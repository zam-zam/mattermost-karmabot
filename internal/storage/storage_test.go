package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// eachStore runs a test body against every configured backend: always
// SQLite, plus PostgreSQL when KARMABOT_TEST_POSTGRES_DSN points at a
// throwaway database.
func eachStore(t *testing.T, fn func(t *testing.T, store *Store)) {
	t.Helper()

	t.Run("sqlite", func(t *testing.T) {
		store, err := Open(Options{Path: filepath.Join(t.TempDir(), "test.db")})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { store.Close() })
		fn(t, store)
	})

	dsn := os.Getenv("KARMABOT_TEST_POSTGRES_DSN")
	if dsn == "" {
		return
	}
	t.Run("postgres", func(t *testing.T) {
		store, err := Open(Options{Driver: "postgres", DSN: dsn})
		if err != nil {
			t.Fatalf("Open postgres: %v", err)
		}
		t.Cleanup(func() { store.Close() })
		// The DSN may point at a shared test database; start clean.
		for _, table := range []string{"karma", "daily_given", "channels"} {
			if _, err := store.exec(context.Background(), "DELETE FROM "+table); err != nil {
				t.Fatalf("clearing %s: %v", table, err)
			}
		}
		fn(t, store)
	})
}

func TestChannelLifecycle(t *testing.T) {
	eachStore(t, testChannelLifecycle)
}

func testChannelLifecycle(t *testing.T, store *Store) {
	ctx := context.Background()

	if enabled, err := store.IsChannelEnabled(ctx, "ch1"); err != nil || enabled {
		t.Fatalf("unknown channel should be disabled, got enabled=%v err=%v", enabled, err)
	}

	if err := store.EnableChannel(ctx, Channel{ID: "ch1", TeamID: "t1", Name: "dev"}); err != nil {
		t.Fatalf("EnableChannel: %v", err)
	}
	if enabled, err := store.IsChannelEnabled(ctx, "ch1"); err != nil || !enabled {
		t.Fatalf("after enable: enabled=%v err=%v", enabled, err)
	}

	channels, err := store.EnabledChannels(ctx)
	if err != nil {
		t.Fatalf("EnabledChannels: %v", err)
	}
	if len(channels) != 1 || channels[0].ID != "ch1" || channels[0].Name != "dev" {
		t.Fatalf("EnabledChannels = %+v, want one ch1/dev", channels)
	}

	if err := store.DisableChannel(ctx, "ch1"); err != nil {
		t.Fatalf("DisableChannel: %v", err)
	}
	if enabled, err := store.IsChannelEnabled(ctx, "ch1"); err != nil || enabled {
		t.Fatalf("after disable: enabled=%v err=%v", enabled, err)
	}

	// Re-enable refreshes the display name.
	if err := store.EnableChannel(ctx, Channel{ID: "ch1", TeamID: "t1", Name: "dev-team"}); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	channels, err = store.EnabledChannels(ctx)
	if err != nil || len(channels) != 1 || channels[0].Name != "dev-team" {
		t.Fatalf("after re-enable: %+v err=%v", channels, err)
	}
}

func TestGrantKarmaAccumulatesAndReportsDailyUsage(t *testing.T) {
	eachStore(t, testGrantKarmaAccumulatesAndReportsDailyUsage)
}

func testGrantKarmaAccumulatesAndReportsDailyUsage(t *testing.T, store *Store) {
	ctx := context.Background()

	grant := func(targetID, targetName string) int {
		t.Helper()
		total, err := store.GrantKarma(ctx, Grant{
			ChannelID:      "ch1",
			Week:           "2026-08-24",
			Day:            "2026-08-26",
			GiverID:        "alice",
			TargetID:       targetID,
			TargetUsername: targetName,
		})
		if err != nil {
			t.Fatalf("GrantKarma(%s): %v", targetName, err)
		}
		return total
	}

	if got := grant("bob", "bob"); got != 1 {
		t.Errorf("first grant total = %d, want 1", got)
	}
	if got := grant("bob", "bob"); got != 2 {
		t.Errorf("second grant total = %d, want 2", got)
	}
	grant("carol", "carol")

	total, perTarget, err := store.GivenOnDay(ctx, "ch1", "alice", "2026-08-26")
	if err != nil {
		t.Fatalf("GivenOnDay: %v", err)
	}
	if total != 3 {
		t.Errorf("daily total = %d, want 3", total)
	}
	if perTarget["bob"] != 2 || perTarget["carol"] != 1 {
		t.Errorf("per target = %v, want bob:2 carol:1", perTarget)
	}

	// A different day starts with a clean budget.
	total, _, err = store.GivenOnDay(ctx, "ch1", "alice", "2026-08-27")
	if err != nil {
		t.Fatalf("GivenOnDay next day: %v", err)
	}
	if total != 0 {
		t.Errorf("next day total = %d, want 0", total)
	}
}

func TestTopByKarmaOrdersAndLimits(t *testing.T) {
	eachStore(t, testTopByKarmaOrdersAndLimits)
}

func testTopByKarmaOrdersAndLimits(t *testing.T, store *Store) {
	ctx := context.Background()

	scores := []struct {
		id, name string
		times    int
	}{
		{"u1", "alice", 3},
		{"u2", "bob", 5},
		{"u3", "carol", 1},
		{"u4", "dave", 3},
	}
	for _, s := range scores {
		for range s.times {
			if _, err := store.GrantKarma(ctx, Grant{
				ChannelID:      "ch1",
				Week:           "2026-08-24",
				Day:            "2026-08-26",
				GiverID:        "giver",
				TargetID:       s.id,
				TargetUsername: s.name,
			}); err != nil {
				t.Fatalf("GrantKarma(%s): %v", s.name, err)
			}
		}
	}

	top, err := store.TopByKarma(ctx, "ch1", "2026-08-24", 10)
	if err != nil {
		t.Fatalf("TopByKarma: %v", err)
	}
	wantOrder := []string{"bob", "alice", "dave", "carol"}
	if len(top) != len(wantOrder) {
		t.Fatalf("top len = %d, want %d", len(top), len(wantOrder))
	}
	for i, name := range wantOrder {
		if top[i].Username != name {
			t.Errorf("top[%d] = %s, want %s", i, top[i].Username, name)
		}
	}

	top2, err := store.TopByKarma(ctx, "ch1", "2026-08-24", 2)
	if err != nil || len(top2) != 2 {
		t.Fatalf("TopByKarma limit 2: len=%d err=%v", len(top2), err)
	}

	// Other weeks and channels are isolated.
	empty, err := store.TopByKarma(ctx, "ch1", "2026-08-31", 10)
	if err != nil || len(empty) != 0 {
		t.Fatalf("other week: len=%d err=%v", len(empty), err)
	}
}

func TestDisableKeepsKarmaAndUserKarmaByChannel(t *testing.T) {
	eachStore(t, testDisableKeepsKarmaAndUserKarmaByChannel)
}

func testDisableKeepsKarmaAndUserKarmaByChannel(t *testing.T, store *Store) {
	ctx := context.Background()

	for _, ch := range []string{"ch1", "ch2"} {
		if err := store.EnableChannel(ctx, Channel{ID: ch, TeamID: "t1", Name: ch + "-name"}); err != nil {
			t.Fatalf("EnableChannel(%s): %v", ch, err)
		}
	}
	for range 2 {
		if _, err := store.GrantKarma(ctx, Grant{
			ChannelID:      "ch1",
			Week:           "2026-08-24",
			Day:            "2026-08-26",
			GiverID:        "giver",
			TargetID:       "bob",
			TargetUsername: "bob",
		}); err != nil {
			t.Fatalf("GrantKarma: %v", err)
		}
	}
	if _, err := store.GrantKarma(ctx, Grant{
		ChannelID:      "ch2",
		Week:           "2026-08-24",
		Day:            "2026-08-26",
		GiverID:        "giver",
		TargetID:       "bob",
		TargetUsername: "bob",
	}); err != nil {
		t.Fatalf("GrantKarma ch2: %v", err)
	}

	if err := store.DisableChannel(ctx, "ch2"); err != nil {
		t.Fatalf("DisableChannel: %v", err)
	}

	// Disabled channels still appear in the user's DM report: the karma
	// exists and the bot's message names the channel.
	got, err := store.UserKarmaByChannel(ctx, "bob", "2026-08-24")
	if err != nil {
		t.Fatalf("UserKarmaByChannel: %v", err)
	}
	if len(got) != 2 || got[0].ChannelName != "ch1-name" || got[0].Karma != 2 {
		t.Fatalf("UserKarmaByChannel = %+v, want ch1-name:2 then ch2-name:1", got)
	}

	// Another week shows nothing.
	empty, err := store.UserKarmaByChannel(ctx, "bob", "2026-08-31")
	if err != nil || len(empty) != 0 {
		t.Fatalf("other week: len=%d err=%v", len(empty), err)
	}
}

func TestKarmaWeekCounts(t *testing.T) {
	eachStore(t, testKarmaWeekCounts)
}

func testKarmaWeekCounts(t *testing.T, store *Store) {
	ctx := context.Background()

	// Rows are keyed by (channel, user, week), so counts follow distinct
	// users per week, not repeated grants.
	grants := []struct {
		week    string
		targets []string
	}{
		{week: "2026-08-24", targets: []string{"bob", "carol"}},
		{week: "2026-08-17", targets: []string{"bob"}},
		{week: "2026-08-18", targets: []string{"bob", "carol", "dave"}},
	}
	for _, g := range grants {
		for _, target := range g.targets {
			if _, err := store.GrantKarma(ctx, Grant{
				ChannelID:      "ch1",
				Week:           g.week,
				Day:            "2026-08-26",
				GiverID:        "alice",
				TargetID:       target,
				TargetUsername: target,
			}); err != nil {
				t.Fatalf("GrantKarma(%s, %s): %v", g.week, target, err)
			}
		}
	}

	counts, err := store.KarmaWeekCounts(ctx)
	if err != nil {
		t.Fatalf("KarmaWeekCounts: %v", err)
	}
	want := map[string]int{
		"2026-08-24": 2,
		"2026-08-17": 1,
		"2026-08-18": 3,
	}
	if len(counts) != len(want) {
		t.Fatalf("KarmaWeekCounts = %v, want %v", counts, want)
	}
	for week, count := range want {
		if counts[week] != count {
			t.Errorf("counts[%q] = %d, want %d", week, counts[week], count)
		}
	}
}

func TestPruneDailyGiven(t *testing.T) {
	eachStore(t, testPruneDailyGiven)
}

func testPruneDailyGiven(t *testing.T, store *Store) {
	ctx := context.Background()

	grants := []struct{ day string }{
		{"2026-08-20"},
		{"2026-08-26"},
	}
	for _, g := range grants {
		if _, err := store.GrantKarma(ctx, Grant{
			ChannelID:      "ch1",
			Week:           "2026-08-24",
			Day:            g.day,
			GiverID:        "alice",
			TargetID:       "bob",
			TargetUsername: "bob",
		}); err != nil {
			t.Fatalf("GrantKarma(%s): %v", g.day, err)
		}
	}

	if err := store.PruneDailyGiven(ctx, "2026-08-26"); err != nil {
		t.Fatalf("PruneDailyGiven: %v", err)
	}

	old, _, err := store.GivenOnDay(ctx, "ch1", "alice", "2026-08-20")
	if err != nil || old != 0 {
		t.Fatalf("old day after prune: total=%d err=%v", old, err)
	}
	current, _, err := store.GivenOnDay(ctx, "ch1", "alice", "2026-08-26")
	if err != nil || current != 1 {
		t.Fatalf("current day after prune: total=%d err=%v", current, err)
	}
}

func TestOpenRejectsUnknownDriver(t *testing.T) {
	if _, err := Open(Options{Driver: "mysql"}); err == nil {
		t.Fatal("Open with unknown driver should fail")
	}
}

func TestRebindPostgres(t *testing.T) {
	got := rebindPostgres("INSERT INTO karma VALUES (?, ?, ?) ON CONFLICT DO NOTHING")
	want := "INSERT INTO karma VALUES ($1, $2, $3) ON CONFLICT DO NOTHING"
	if got != want {
		t.Fatalf("rebindPostgres = %q, want %q", got, want)
	}
	if got := rebindPostgres("SELECT 1"); got != "SELECT 1" {
		t.Fatalf("rebindPostgres without placeholders = %q", got)
	}
}
