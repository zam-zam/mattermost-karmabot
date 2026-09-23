package scheduler

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"karmabot/internal/bot"
	"karmabot/internal/storage"
)

type fakeStore struct {
	channels []storage.Channel
	// tops maps "channelID|week" to the leaderboard for that period.
	tops map[string][]storage.KarmaEntry

	prunedDay    string
	requestedKey string
}

func (f *fakeStore) EnabledChannels(context.Context) ([]storage.Channel, error) {
	return f.channels, nil
}

func (f *fakeStore) TopByKarma(
	_ context.Context,
	channelID, week string,
	_ int,
) ([]storage.KarmaEntry, error) {
	f.requestedKey = week
	return f.tops[channelID+"|"+week], nil
}

func (f *fakeStore) PruneDailyGiven(_ context.Context, day string) error {
	f.prunedDay = day
	return nil
}

type fakePoster struct {
	posts []sentMessage
}

type sentMessage struct {
	channelID string
	message   string
}

func (f *fakePoster) CreatePost(
	_ context.Context,
	channelID, _, message string,
) error {
	f.posts = append(f.posts, sentMessage{channelID: channelID, message: message})
	return nil
}

func (f *fakePoster) messagesFor(channelID string) []string {
	msgs := []string{}
	for _, p := range f.posts {
		if p.channelID == channelID {
			msgs = append(msgs, p.message)
		}
	}
	return msgs
}

// newTestScheduler wires a scheduler over fakes with a frozen clock.
func newTestScheduler(
	t *testing.T,
	lang string,
	period bot.Period,
	store *fakeStore,
	now time.Time,
) (*Scheduler, *fakePoster) {
	t.Helper()

	msgs, err := bot.NewMessages(lang)
	if err != nil {
		t.Fatalf("bot.NewMessages(%q): %v", lang, err)
	}
	poster := &fakePoster{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := New(store, poster, log, msgs, period)
	s.now = func() time.Time { return now }
	return s, poster
}

func mustPeriod(t *testing.T, kind, tz, rollover string) bot.Period {
	t.Helper()
	p, err := bot.NewPeriod(kind, tz, rollover)
	if err != nil {
		t.Fatalf("bot.NewPeriod(%q, %q, %q): %v", kind, tz, rollover, err)
	}
	return p
}

// TestPublishFinishedPeriodWeek pins the two posts at a weekly rollover:
// the finished week's results with its bounds, then the new week's
// announcement. A channel with an empty finished week gets only the
// announcement.
func TestPublishFinishedPeriodWeek(t *testing.T) {
	// Monday 2026-08-31, one second after the 09:00 UTC rollover.
	now := time.Date(2026, 8, 31, 9, 0, 1, 0, time.UTC)
	store := &fakeStore{
		channels: []storage.Channel{{ID: "ch1"}, {ID: "ch2"}},
		tops: map[string][]storage.KarmaEntry{
			// The finished week started Monday 2026-08-24.
			"ch1|2026-08-24": {{Username: "alice", Karma: 3}, {Username: "bob", Karma: 1}},
		},
	}
	s, poster := newTestScheduler(t, "en", mustPeriod(t, "week", "UTC", "09:00"), store, now)

	s.publishFinishedPeriod(context.Background())

	if store.requestedKey != "2026-08-24" {
		t.Errorf("results requested for key %q, want 2026-08-24", store.requestedKey)
	}
	if store.prunedDay != "2026-08-31" {
		t.Errorf("pruned day = %q, want 2026-08-31", store.prunedDay)
	}

	ch1 := poster.messagesFor("ch1")
	if len(ch1) != 2 {
		t.Fatalf("ch1 got %d posts, want 2 (results + announcement): %v", len(ch1), ch1)
	}
	wantContains(t, ch1[0], "🏁 Weekly results (24 August – 30 August):", "results header")
	wantContains(t, ch1[0], "1. @alice — ⭐⭐⭐ 3", "results entry")
	wantContains(t, ch1[1], "🆕 New week: 31 August – 6 September.", "new week announcement")

	ch2 := poster.messagesFor("ch2")
	if len(ch2) != 1 {
		t.Fatalf("empty ch2 got %d posts, want only the announcement: %v", len(ch2), ch2)
	}
	wantContains(t, ch2[0], "🆕 New week: 31 August – 6 September.", "announcement without results")
}

// TestPublishFinishedPeriodMonth pins the monthly rollover posts,
// including the Russian wording and genitive month names.
func TestPublishFinishedPeriodMonth(t *testing.T) {
	// October 1, one second after the 09:00 UTC rollover.
	now := time.Date(2026, 10, 1, 9, 0, 1, 0, time.UTC)
	store := &fakeStore{
		channels: []storage.Channel{{ID: "ch1"}},
		tops: map[string][]storage.KarmaEntry{
			"ch1|2026-09-01": {{Username: "alice", Karma: 2}},
		},
	}
	s, poster := newTestScheduler(t, "ru", mustPeriod(t, "month", "UTC", "09:00"), store, now)

	s.publishFinishedPeriod(context.Background())

	ch1 := poster.messagesFor("ch1")
	if len(ch1) != 2 {
		t.Fatalf("ch1 got %d posts, want 2: %v", len(ch1), ch1)
	}
	wantContains(t, ch1[0], "🏁 Итоги месяца (сентябрь):", "results header")
	wantContains(t, ch1[0], "1. @alice — ⭐⭐ 2", "results entry")
	wantContains(t, ch1[1], "🆕 Новый месяц: октябрь.", "new month announcement")

	if store.prunedDay != "2026-10-01" {
		t.Errorf("pruned day = %q, want 2026-10-01", store.prunedDay)
	}
}

// TestPublishFinishedPeriodRussianWeek pins the Russian weekly wording
// with genitive month names in the date range.
func TestPublishFinishedPeriodRussianWeek(t *testing.T) {
	now := time.Date(2026, 8, 31, 9, 0, 1, 0, time.UTC)
	store := &fakeStore{
		channels: []storage.Channel{{ID: "ch1"}},
		tops: map[string][]storage.KarmaEntry{
			"ch1|2026-08-24": {{Username: "bob", Karma: 1}},
		},
	}
	s, poster := newTestScheduler(t, "ru", mustPeriod(t, "week", "UTC", "09:00"), store, now)

	s.publishFinishedPeriod(context.Background())

	ch1 := poster.messagesFor("ch1")
	if len(ch1) != 2 {
		t.Fatalf("ch1 got %d posts, want 2: %v", len(ch1), ch1)
	}
	wantContains(t, ch1[0], "🏁 Итоги недели (с 24 августа по 30 августа):", "results header")
	wantContains(t, ch1[1], "🆕 Новая неделя: с 31 августа по 6 сентября.", "new week announcement")
}

func wantContains(t *testing.T, got, want, name string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("%s: message %q does not contain %q", name, got, want)
	}
}
