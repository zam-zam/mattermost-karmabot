package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"karmabot/internal/storage"
)

type sentPost struct {
	channelID string
	rootID    string
	message   string
}

type fakeClient struct {
	me       *model.User
	channels map[string]*model.Channel
	users    map[string]*model.User
	posts    []sentPost
}

func (f *fakeClient) Me() *model.User { return f.me }

func (f *fakeClient) GetChannel(
	_ context.Context,
	channelID string,
) (*model.Channel, error) {
	ch, ok := f.channels[channelID]
	if !ok {
		return nil, fmt.Errorf("no channel %s", channelID)
	}
	return ch, nil
}

func (f *fakeClient) GetUserByUsername(
	_ context.Context,
	username string,
) (*model.User, error) {
	user, ok := f.users[username]
	if !ok {
		return nil, fmt.Errorf("no user %s", username)
	}
	return user, nil
}

func (f *fakeClient) CreatePost(
	_ context.Context,
	channelID, rootID, message string,
) error {
	f.posts = append(f.posts,
		sentPost{channelID: channelID, rootID: rootID, message: message})
	return nil
}

func (f *fakeClient) lastReply() string {
	if len(f.posts) == 0 {
		return ""
	}
	return f.posts[len(f.posts)-1].message
}

func (f *fakeClient) lastRoot() string {
	if len(f.posts) == 0 {
		return ""
	}
	return f.posts[len(f.posts)-1].rootID
}

func newTestBot(t *testing.T, lang string) (*Bot, *fakeClient) {
	t.Helper()

	store, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	client := &fakeClient{
		me: &model.User{Id: "bot-1", Username: "karmabot"},
		channels: map[string]*model.Channel{
			"ch1": {Id: "ch1", TeamId: "t1", Name: "dev", DisplayName: "dev", Type: model.ChannelTypeOpen},
			"ch2": {Id: "ch2", TeamId: "t1", Name: "ops", DisplayName: "ops", Type: model.ChannelTypeOpen},
			"dm":  {Id: "dm", Type: model.ChannelTypeDirect},
		},
		users: map[string]*model.User{
			"alice": {Id: "u-alice", Username: "alice"},
			"bob":   {Id: "u-bob", Username: "bob"},
			"carol": {Id: "u-carol", Username: "carol"},
			"dave":  {Id: "u-dave", Username: "dave"},
			"eve":   {Id: "u-eve", Username: "eve"},
		},
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	msgs, err := NewMessages(lang)
	if err != nil {
		t.Fatalf("NewMessages(%q): %v", lang, err)
	}
	b, err := New(client, store, log, Options{
		Messages: msgs,
		Limits:   Limits{DailyTotal: 5, DailyPerTarget: 2},
		Period:   Period{Days: 7},
	})
	if err != nil {
		t.Fatalf("bot.New: %v", err)
	}
	// Wednesday 2026-08-26 so period and day keys are stable in tests.
	b.now = func() time.Time {
		return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	}
	return b, client
}

var saySeq int

// say delivers a message as userID via a synthetic "posted" event,
// returning the generated post ID so tests can assert thread roots.
func say(t *testing.T, b *Bot, channelID, userID, message string) string {
	t.Helper()

	saySeq++
	postID := fmt.Sprintf("post-%d", saySeq)
	raw, err := json.Marshal(&model.Post{
		Id:        postID,
		ChannelId: channelID,
		UserId:    userID,
		Message:   message,
	})
	if err != nil {
		t.Fatalf("marshaling post: %v", err)
	}

	event := model.NewWebSocketEvent(model.WebsocketEventPosted, "", channelID, userID, nil, "")
	// Add mutates in place, unlike SetData which returns a copy.
	event.Add("post", string(raw))
	b.HandleEvent(context.Background(), event)
	return postID
}

func startKarma(t *testing.T, b *Bot, channelID string) {
	t.Helper()
	say(t, b, channelID, "u-alice", "@karmabot start")
}

func wantContains(t *testing.T, got, want, name string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("%s: reply %q does not contain %q", name, got, want)
	}
}

func TestStartStopTopFlow(t *testing.T) {
	b, client := newTestBot(t, "ru")

	startKarma(t, b, "ch1")
	wantContains(t, client.lastReply(), "Карма включена", "start")

	say(t, b, "ch1", "u-bob", "@karmabot ++ @alice")
	say(t, b, "ch1", "u-bob", "@karmabot top")
	wantContains(t, client.lastReply(), "Топ-10", "top header")
	wantContains(t, client.lastReply(), "1. @alice — ⭐ 1", "top entry")

	say(t, b, "ch1", "u-alice", "@karmabot stop")
	wantContains(t, client.lastReply(), "Карма выключена", "stop")

	say(t, b, "ch1", "u-alice", "@karmabot top")
	wantContains(t, client.lastReply(), "не включена", "top after stop")

	say(t, b, "ch1", "u-alice", "@karmabot start")
	say(t, b, "ch1", "u-alice", "@karmabot top")
	wantContains(t, client.lastReply(), "1. @alice — ⭐ 1", "top resumes with data")
}

func TestGrantAppliesAndReportsBudgets(t *testing.T) {
	b, client := newTestBot(t, "ru")
	startKarma(t, b, "ch1")

	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	reply := client.lastReply()
	wantContains(t, reply, "✅ @bob: +1 (карма в канале: ⭐ 1)", "applied")
	wantContains(t, reply, "Осталось на сегодня: 4 из 5", "total budget")
	wantContains(t, reply, "@bob: 1 из 2", "target budget")

	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	reply = client.lastReply()
	wantContains(t, reply, "✅ @bob: +1 (карма в канале: ⭐⭐ 2)", "second applied")
	wantContains(t, reply, "@bob: 0 из 2", "target budget exhausted")

	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	reply = client.lastReply()
	wantContains(t, reply, "этому человеку на сегодня хватит", "per-target limit")
	wantContains(t, reply, "3 из 5", "total budget intact")
}

func TestGrantDailyTotalLimit(t *testing.T) {
	b, client := newTestBot(t, "ru")
	startKarma(t, b, "ch1")

	// bob spends his budget of 5 across three targets.
	say(t, b, "ch1", "u-bob", "@karmabot ++ @alice @carol") // 2 used
	say(t, b, "ch1", "u-bob", "@karmabot ++ @alice @carol") // 4 used
	say(t, b, "ch1", "u-bob", "@karmabot ++ @dave")         // 5 used
	wantContains(t, client.lastReply(), "Осталось на сегодня: 0 из 5", "budget spent")

	say(t, b, "ch1", "u-bob", "@karmabot ++ @eve")
	wantContains(t, client.lastReply(), "лимит исчерпан", "total limit hit")
	wantContains(t, client.lastReply(), "@eve: 2 из 2", "eve budget untouched")
}

func TestGrantBlocksSelfAndUnknownUser(t *testing.T) {
	b, client := newTestBot(t, "ru")
	startKarma(t, b, "ch1")

	say(t, b, "ch1", "u-alice", "@karmabot ++ @alice")
	wantContains(t, client.lastReply(), "себе карму повысить нельзя", "self")

	say(t, b, "ch1", "u-alice", "@karmabot ++ @ghost")
	wantContains(t, client.lastReply(), "Пользователь @ghost не найден", "unknown user")
}

func TestGrantMultipleTargetsOneReply(t *testing.T) {
	b, client := newTestBot(t, "ru")
	startKarma(t, b, "ch1")

	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob Спасибо за помощь! @carol")
	reply := client.lastReply()
	wantContains(t, reply, "✅ @bob: +1 (карма в канале: ⭐ 1)", "bob applied")
	wantContains(t, reply, "✅ @carol: +1 (карма в канале: ⭐ 1)", "carol applied")
	wantContains(t, reply, "Осталось на сегодня: 3 из 5", "combined budget")
}

func TestGrantInStoppedChannel(t *testing.T) {
	b, client := newTestBot(t, "ru")

	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	wantContains(t, client.lastReply(), "не включена", "not started")
}

func TestNegativeKarmaRejected(t *testing.T) {
	b, client := newTestBot(t, "ru")
	startKarma(t, b, "ch1")

	say(t, b, "ch1", "u-alice", "@karmabot -- @bob")
	wantContains(t, client.lastReply(), "нельзя понижать", "negative")
}

func TestUnknownCommandShowsHelp(t *testing.T) {
	b, client := newTestBot(t, "ru")

	say(t, b, "ch1", "u-alice", "@karmabot dance")
	wantContains(t, client.lastReply(), "Карма-бот", "unknown verb")

	say(t, b, "ch1", "u-alice", "@karmabot")
	wantContains(t, client.lastReply(), "Карма-бот", "bare mention")

	say(t, b, "ch1", "u-alice", "@karmabot help")
	wantContains(t, client.lastReply(), "топ-10", "help")
}

func TestDirectMessages(t *testing.T) {
	b, client := newTestBot(t, "ru")
	startKarma(t, b, "ch1")
	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")

	say(t, b, "dm", "u-bob", "karma")
	reply := client.lastReply()
	wantContains(t, reply, "Твоя карма за текущий период", "dm header")
	wantContains(t, reply, "• dev — ⭐ 1", "dm entry")

	say(t, b, "dm", "u-bob", "help")
	wantContains(t, client.lastReply(), "Карма-бот", "dm help")

	say(t, b, "dm", "u-bob", "что?")
	wantContains(t, client.lastReply(), "Не понял команду", "dm unknown")
}

func TestIgnoredMessages(t *testing.T) {
	b, client := newTestBot(t, "ru")

	before := len(client.posts)

	// Bot's own posts must not re-trigger commands.
	say(t, b, "ch1", "bot-1", "@karmabot ++ @alice")

	// Mention mid-message is not a command.
	say(t, b, "ch1", "u-alice", "спасибо @karmabot, что существуешь")

	// A similar username is not the bot.
	say(t, b, "ch1", "u-alice", "@karmabot2 ++ @bob")

	// Non-posted events are dropped silently.
	b.HandleEvent(context.Background(), nil)

	if after := len(client.posts); after != before {
		t.Errorf("bot replied to ignored messages: %d -> %d posts", before, after)
	}
}

func TestStripBotMention(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		wantOK   bool
		wantRest string
	}{
		{name: "plain", message: "@karmabot start", wantOK: true, wantRest: "start"},
		{name: "case-insensitive", message: "@KarmaBot start", wantOK: true, wantRest: "start"},
		{name: "padding", message: "  @karmabot   top  ", wantOK: true, wantRest: "top"},
		{name: "similar name rejected", message: "@karmabot2 ++ @bob", wantOK: false},
		{name: "mid-message rejected", message: "hi @karmabot ++ @bob", wantOK: false},
		{name: "no mention", message: "++ @bob", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rest, ok := stripBotMention(tt.message, "karmabot")
			if ok != tt.wantOK || (ok && rest != tt.wantRest) {
				t.Errorf("stripBotMention(%q) = (%q, %v), want (%q, %v)",
					tt.message, rest, ok, tt.wantRest, tt.wantOK)
			}
		})
	}
}

func TestKarmaStars(t *testing.T) {
	tests := []struct {
		karma int
		want  string
	}{
		{karma: 0, want: "0"},
		{karma: 1, want: "⭐ 1"},
		{karma: 3, want: "⭐⭐⭐ 3"},
	}

	for _, tt := range tests {
		if got := karmaStars(tt.karma); got != tt.want {
			t.Errorf("karmaStars(%d) = %q, want %q", tt.karma, got, tt.want)
		}
	}
}

func TestExtractMentions(t *testing.T) {
	got := extractMentions("++ @bob @carol! @bob (и @karmabot)", "karmabot")
	want := []string{"bob", "carol"}
	if len(got) != len(want) {
		t.Fatalf("extractMentions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("extractMentions[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

// TestEnglishReplies runs the main flows in the default language so both
// catalogs stay covered end to end.
func TestEnglishReplies(t *testing.T) {
	b, client := newTestBot(t, "en")

	startKarma(t, b, "ch1")
	wantContains(t, client.lastReply(), "Karma enabled", "start")

	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	reply := client.lastReply()
	wantContains(t, reply, "✅ @bob: +1 (channel karma: ⭐ 1)", "applied")
	wantContains(t, reply, "Left today: 4 of 5", "total budget")
	wantContains(t, reply, "@bob: 1 of 2", "target budget")

	say(t, b, "ch1", "u-alice", "@karmabot top")
	wantContains(t, client.lastReply(), "Top-10", "top header")
	wantContains(t, client.lastReply(), "1. @bob — ⭐ 1", "top entry")

	say(t, b, "ch1", "u-alice", "@karmabot help")
	wantContains(t, client.lastReply(), "Karma bot", "help")
	wantContains(t, client.lastReply(), "top-10", "help top line")

	say(t, b, "dm", "u-bob", "karma")
	reply = client.lastReply()
	wantContains(t, reply, "Your karma this period", "dm header")
	wantContains(t, reply, "• dev — ⭐ 1", "dm entry")

	say(t, b, "dm", "u-bob", "what?")
	wantContains(t, client.lastReply(), "Didn't understand", "dm unknown")
}

// TestConfigurableLimits proves the daily budgets come from configuration,
// not the 5/2 defaults the other tests pin.
func TestConfigurableLimits(t *testing.T) {
	b, client := newTestBot(t, "en")
	b.limits = Limits{DailyTotal: 3, DailyPerTarget: 1}
	startKarma(t, b, "ch1")

	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	reply := client.lastReply()
	wantContains(t, reply, "✅ @bob: +1 (channel karma: ⭐ 1)", "applied")
	wantContains(t, reply, "Left today: 2 of 3", "total budget")
	wantContains(t, reply, "@bob: 0 of 1", "target budget")

	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	wantContains(t, client.lastReply(), "that's enough for this person today", "per-target hit")
}

func TestNewRejectsInvalidLimits(t *testing.T) {
	tests := []struct {
		name   string
		limits Limits
	}{
		{name: "negative total", limits: Limits{DailyTotal: -1, DailyPerTarget: 1}},
		{name: "negative per-target", limits: Limits{DailyTotal: 5, DailyPerTarget: -2}},
		{name: "per-target above total", limits: Limits{DailyTotal: 3, DailyPerTarget: 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeClient{
				me: &model.User{Id: "bot-1", Username: "karmabot"},
			}
			opts := Options{
				Messages: &Messages{},
				Limits:   tt.limits,
				Period:   Period{Days: 7},
			}
			if _, err := New(client, nil, nil, opts); err == nil {
				t.Error("New succeeded with invalid limits, want error")
			}
		})
	}
}

func TestNewRejectsInvalidPeriodAndNilMessages(t *testing.T) {
	client := &fakeClient{
		me: &model.User{Id: "bot-1", Username: "karmabot"},
	}

	for _, days := range []int{0, 366} {
		opts := Options{
			Messages: &Messages{},
			Limits:   Limits{DailyTotal: 5, DailyPerTarget: 2},
			Period:   Period{Days: days},
		}
		if _, err := New(client, nil, nil, opts); err == nil {
			t.Errorf("New with Period{Days:%d} succeeded, want error", days)
		}
	}

	opts := Options{
		Messages: nil,
		Limits:   Limits{DailyTotal: 5, DailyPerTarget: 2},
		Period:   Period{Days: 7},
	}
	if _, err := New(client, nil, nil, opts); err == nil {
		t.Error("New with nil Messages succeeded, want error")
	}
}

// TestNewAcceptsUnlimitedLimits pins the zero-means-unlimited contract:
// either limit may be 0 on its own or both at once.
func TestNewAcceptsUnlimitedLimits(t *testing.T) {
	for _, limits := range []Limits{
		{},
		{DailyPerTarget: 2},
		{DailyTotal: 5},
	} {
		client := &fakeClient{
			me: &model.User{Id: "bot-1", Username: "karmabot"},
		}
		opts := Options{
			Messages: &Messages{},
			Limits:   limits,
			Period:   Period{Days: 7},
		}
		if _, err := New(client, nil, nil, opts); err != nil {
			t.Errorf("New(%+v): %v", limits, err)
		}
	}
}

// TestUnlimitedLimits proves zero limits never cap granting and add no
// budget footer to replies.
func TestUnlimitedLimits(t *testing.T) {
	b, client := newTestBot(t, "en")
	b.limits = Limits{}
	startKarma(t, b, "ch1")

	for range 3 {
		say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	}
	reply := client.lastReply()
	wantContains(t, reply, "✅ @bob: +1 (channel karma: ⭐⭐⭐ 3)", "third grant applied")
	if strings.Contains(reply, "Left today") {
		t.Errorf("unlimited reply shows a budget footer: %q", reply)
	}
}

// TestUnlimitedTotalWithPerTargetLimit caps only the per-target budget:
// no total line in the footer, and the per-target limit still binds.
func TestUnlimitedTotalWithPerTargetLimit(t *testing.T) {
	b, client := newTestBot(t, "ru")
	b.limits = Limits{DailyPerTarget: 2}
	startKarma(t, b, "ch1")

	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	reply := client.lastReply()
	wantContains(t, reply, "✅ @bob: +1 (карма в канале: ⭐⭐ 2)", "second grant applied")
	wantContains(t, reply, "@bob: 0 из 2", "per-target budget")
	if strings.Contains(reply, "Осталось на сегодня") {
		t.Errorf("unlimited total still shows the total budget: %q", reply)
	}

	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	wantContains(t, client.lastReply(), "этому человеку на сегодня хватит", "per-target hit")
}

// TestThirtyDayPeriodFlow proves a longer period keeps karma across week
// boundaries: karma granted on 2026-08-26 is still on the scoreboard on
// 2026-09-10, which belongs to a different week but the same 30-day
// period (2026-08-18 … 2026-09-17).
func TestThirtyDayPeriodFlow(t *testing.T) {
	b, client := newTestBot(t, "en")
	b.period = Period{Days: 30}
	startKarma(t, b, "ch1")

	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	say(t, b, "ch1", "u-alice", "@karmabot top")
	wantContains(t, client.lastReply(), "1. @bob — ⭐ 1", "top in the same period")

	b.now = func() time.Time {
		return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	}
	say(t, b, "ch1", "u-alice", "@karmabot top")
	wantContains(t, client.lastReply(), "1. @bob — ⭐ 1", "top persists across the week boundary")

	say(t, b, "dm", "u-bob", "karma")
	wantContains(t, client.lastReply(), "• dev — ⭐ 1", "dm report persists")
}

// TestRepliesGoToCommandThread pins that every command answer is posted
// as a reply in the command's thread.
func TestRepliesGoToCommandThread(t *testing.T) {
	b, client := newTestBot(t, "en")

	id := say(t, b, "ch1", "u-alice", "@karmabot start")
	if got := client.lastRoot(); got != id {
		t.Errorf("start reply root = %q, want %q", got, id)
	}

	id = say(t, b, "ch1", "u-alice", "@karmabot help")
	if got := client.lastRoot(); got != id {
		t.Errorf("help reply root = %q, want %q", got, id)
	}

	say(t, b, "ch1", "u-alice", "@karmabot ++ @bob")
	id = say(t, b, "ch1", "u-alice", "@karmabot top")
	if got := client.lastRoot(); got != id {
		t.Errorf("top reply root = %q, want %q", got, id)
	}

	id = say(t, b, "dm", "u-alice", "help")
	if got := client.lastRoot(); got != id {
		t.Errorf("dm reply root = %q, want %q", got, id)
	}
}

func TestThreadRoot(t *testing.T) {
	// A channel-top command starts a thread under itself.
	if got := threadRoot(&model.Post{Id: "p1"}); got != "p1" {
		t.Errorf("channel-top post root = %q, want p1", got)
	}
	// A command sent inside a thread keeps that thread.
	if got := threadRoot(&model.Post{Id: "p2", RootId: "p1"}); got != "p1" {
		t.Errorf("in-thread post root = %q, want p1", got)
	}
}
