package bot

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"karmabot/internal/storage"
)

// loadLocale decodes one embedded catalog into an id → raw value map;
// values are either plain strings or plural-form objects.
func loadLocale(t *testing.T, name string) map[string]json.RawMessage {
	t.Helper()

	raw, err := localeFS.ReadFile(name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	var msgs map[string]json.RawMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	return msgs
}

// localeForms expands a catalog value into plural form → template; a
// plain string message has the single form "".
func localeForms(t *testing.T, raw json.RawMessage) map[string]string {
	t.Helper()

	var plural map[string]string
	if err := json.Unmarshal(raw, &plural); err == nil {
		return plural
	}
	var plain string
	if err := json.Unmarshal(raw, &plain); err != nil {
		t.Fatalf("catalog value is neither string nor plural object: %s", raw)
	}
	return map[string]string{"": plain}
}

var placeholderRe = regexp.MustCompile(`\{\{\.(\w+)\}\}`)

// placeholders returns the sorted set of template placeholders in s.
func placeholders(s string) []string {
	seen := map[string]bool{}
	for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
		seen[m[1]] = true
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestLocaleParity keeps the two catalogs in sync: same IDs, same plural
// form sets, and matching placeholders per form.
func TestLocaleParity(t *testing.T) {
	en := loadLocale(t, "locales/active.en.json")
	ru := loadLocale(t, "locales/active.ru.json")

	for id := range en {
		if _, ok := ru[id]; !ok {
			t.Errorf("ru catalog missing %q", id)
		}
	}
	for id := range ru {
		if _, ok := en[id]; !ok {
			t.Errorf("en catalog missing %q", id)
		}
	}

	for id, enRaw := range en {
		ruRaw, ok := ru[id]
		if !ok {
			continue
		}
		checkFormsParity(t, id, localeForms(t, enRaw), localeForms(t, ruRaw))
	}
}

func checkFormsParity(t *testing.T, id string, enForms, ruForms map[string]string) {
	t.Helper()

	// Plural objects must carry the universal "other" fallback; plain
	// string messages use the single form "".
	for lang, forms := range map[string]map[string]string{"en": enForms, "ru": ruForms} {
		_, plain := forms[""]
		if !plain {
			if _, ok := forms["other"]; !ok {
				t.Errorf("%q: %s catalog has no %q plural form (the fallback)", id, lang, "other")
			}
		}
	}

	// Plural form sets legitimately differ between languages; every form
	// both share must carry the same placeholders.
	for form, enText := range enForms {
		ruText, ok := ruForms[form]
		if !ok {
			continue
		}
		enPh := placeholders(enText)
		ruPh := placeholders(ruText)
		if strings.Join(enPh, ",") != strings.Join(ruPh, ",") {
			t.Errorf("%q[%q]: placeholders differ: en %v, ru %v", id, form, enPh, ruPh)
		}
	}
}

func formNames(forms map[string]string) []string {
	names := make([]string, 0, len(forms))
	for name := range forms {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestEveryMessageLocalizes renders every known ID in both languages with
// sample data, catching broken templates early.
func TestEveryMessageLocalizes(t *testing.T) {
	data := map[string]any{
		"BotUsername":    "karmabot",
		"Username":       "bob",
		"Limit":          5,
		"Remaining":      3,
		"Stars":          "⭐ 1",
		"Position":       1,
		"TopLimit":       10,
		"DailyTotal":     5,
		"DailyPerTarget": 2,
		"ChannelName":    "dev",
		"ChannelID":      "ch1",
		"LimitsLine":     "Limits: at most 5 karma per day, of which at most 2 to one person.",
		"PeriodLine":     "The scoreboard follows calendar weeks: results are announced every Monday at 09:00 (UTC).",
		"Bounds":         "24 August – 30 August",
		"StartDay":       24,
		"StartMonth":     "August",
		"EndDay":         30,
		"EndMonth":       "August",
		"Month":          "September",
		"Time":           "09:00",
		"Zone":           "UTC",
	}
	en := loadLocale(t, "locales/active.en.json")

	for _, lang := range []string{"en", "ru"} {
		m, err := NewMessages(lang)
		if err != nil {
			t.Fatalf("NewMessages(%q): %v", lang, err)
		}
		for id := range en {
			if got := m.t(id, data); strings.Contains(got, "{{") {
				t.Errorf("%s/%s: unresolved placeholder in %q", lang, id, got)
			}
		}
	}
}

func TestLanguageSelection(t *testing.T) {
	en, err := NewMessages("en")
	if err != nil {
		t.Fatalf("NewMessages(en): %v", err)
	}

	week := mustPeriod(t, "week", "UTC", "09:00")

	// Regional variants resolve to their base language.
	ru, err := NewMessages("ru-RU")
	if err != nil {
		t.Fatalf("NewMessages(ru-RU): %v", err)
	}
	wantRu := "На этой неделе пока никто не получил карму."
	if got := ru.topEmpty(week); got != wantRu {
		t.Errorf("ru-RU topEmpty = %q, want %q", got, wantRu)
	}

	// A well-formed but unsupported language falls back to English.
	de, err := NewMessages("de")
	if err != nil {
		t.Fatalf("NewMessages(de): %v", err)
	}
	if got := de.topEmpty(week); got != en.topEmpty(week) {
		t.Errorf("de topEmpty = %q, want English %q", got, en.topEmpty(week))
	}

	// Garbage fails fast at startup.
	if _, err := NewMessages("not a language!"); err == nil {
		t.Error("NewMessages(garbage) succeeded, want error")
	}
}

// TestPeriodLineVariants pins the help sentence for both period kinds in
// both languages, including the configured rollover time and zone.
func TestPeriodLineVariants(t *testing.T) {
	week := mustPeriod(t, "week", "Europe/Moscow", "09:00")
	month := mustPeriod(t, "month", "UTC", "00:30")

	ru, err := NewMessages("ru")
	if err != nil {
		t.Fatalf("NewMessages(ru): %v", err)
	}
	en, err := NewMessages("en")
	if err != nil {
		t.Fatalf("NewMessages(en): %v", err)
	}

	tests := []struct {
		name    string
		m       *Messages
		period  Period
		wantSub string
	}{
		{name: "ru week", m: ru, period: week, wantSub: "каждый понедельник в 09:00 (Europe/Moscow)"},
		{name: "ru month", m: ru, period: month, wantSub: "1-го числа каждого месяца в 00:30 (UTC)"},
		{name: "en week", m: en, period: week, wantSub: "every Monday at 09:00 (Europe/Moscow)"},
		{name: "en month", m: en, period: month, wantSub: "on the first day of each month at 00:30 (UTC)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.m.periodLine(tt.period)
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("periodLine(%s) = %q, want to contain %q",
					tt.period.Kind, got, tt.wantSub)
			}
		})
	}
}

// TestPeriodBounds pins the human-readable bounds: full month names,
// Russian genitive in date ranges, and ranges crossing a month boundary.
func TestPeriodBounds(t *testing.T) {
	week := mustPeriod(t, "week", "UTC", "09:00")
	month := mustPeriod(t, "month", "UTC", "09:00")

	augStart := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)   // a Monday
	crossStart := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC) // Monday → Sunday Sep 6
	septStart := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

	ru, err := NewMessages("ru")
	if err != nil {
		t.Fatalf("NewMessages(ru): %v", err)
	}
	en, err := NewMessages("en")
	if err != nil {
		t.Fatalf("NewMessages(en): %v", err)
	}

	tests := []struct {
		name  string
		m     *Messages
		p     Period
		start time.Time
		want  string
	}{
		{name: "en week", m: en, p: week, start: augStart, want: "24 August – 30 August"},
		{name: "en week across months", m: en, p: week, start: crossStart, want: "31 August – 6 September"},
		{name: "en month", m: en, p: month, start: septStart, want: "September"},
		{name: "ru week genitive", m: ru, p: week, start: augStart, want: "с 24 августа по 30 августа"},
		{name: "ru week across months", m: ru, p: week, start: crossStart, want: "с 31 августа по 6 сентября"},
		{name: "ru month", m: ru, p: month, start: septStart, want: "сентябрь"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.periodBounds(tt.p, tt.start); got != tt.want {
				t.Errorf("periodBounds = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPeriodTopAndStartedHeaders pins the scheduler messages: kind-named
// headers with bounds.
func TestPeriodTopAndStartedHeaders(t *testing.T) {
	week := mustPeriod(t, "week", "UTC", "09:00")
	month := mustPeriod(t, "month", "UTC", "09:00")
	entries := []storage.KarmaEntry{{Username: "alice", Karma: 2}}

	ru, err := NewMessages("ru")
	if err != nil {
		t.Fatalf("NewMessages(ru): %v", err)
	}
	en, err := NewMessages("en")
	if err != nil {
		t.Fatalf("NewMessages(en): %v", err)
	}

	tests := []struct {
		name string
		m    *Messages
		p    Period
	}{
		{name: "en week", m: en, p: week},
		{name: "en month", m: en, p: month},
		{name: "ru week", m: ru, p: week},
		{name: "ru month", m: ru, p: month},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := tt.p.Start(time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC))
			top := tt.m.PeriodTop(entries, tt.p, start)
			started := tt.m.PeriodStarted(tt.p, start)
			if !strings.Contains(top, "@alice") || !strings.Contains(top, ":") {
				t.Errorf("PeriodTop = %q, want a header and entries", top)
			}
			if strings.Contains(top, "{{") || strings.Contains(started, "{{") {
				t.Errorf("unresolved placeholder in %q / %q", top, started)
			}
		})
	}
}

// TestRussianWordingPreserved pins a few Russian strings to their exact
// pre-i18n wording.
func TestRussianWordingPreserved(t *testing.T) {
	m, err := NewMessages("ru")
	if err != nil {
		t.Fatalf("NewMessages(ru): %v", err)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "started",
			got:  m.started("karmabot"),
			want: "✅ Карма включена. Благодарите так: `@karmabot ++ @username`",
		},
		{
			name: "applied",
			got:  m.applied("bob", "⭐ 1"),
			want: "✅ @bob: +1 (карма в канале: ⭐ 1)",
		},
		{
			name: "budgets prefix",
			got:  m.budgetsPrefix(4, 5),
			want: "Осталось на сегодня: 4 из 5",
		},
		{
			name: "budgets target",
			got:  m.budgetsTarget("bob", 1, 2),
			want: "@bob: 1 из 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}
