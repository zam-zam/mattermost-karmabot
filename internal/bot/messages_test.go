package bot

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// loadLocale decodes one embedded catalog into an id → template map.
func loadLocale(t *testing.T, name string) map[string]string {
	t.Helper()

	raw, err := localeFS.ReadFile(name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	var msgs map[string]string
	if err := json.Unmarshal(raw, &msgs); err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	return msgs
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

// TestLocaleParity keeps the two catalogs in sync: same IDs, and each ID
// carries the same placeholders in both languages.
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

	for id, enText := range en {
		ruText, ok := ru[id]
		if !ok {
			continue
		}
		enPh := placeholders(enText)
		ruPh := placeholders(ruText)
		if strings.Join(enPh, ",") != strings.Join(ruPh, ",") {
			t.Errorf("%q: placeholders differ: en %v, ru %v", id, enPh, ruPh)
		}
	}
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
		"LimitsLine":     "Limits: at most 5 karma per day, of which at most 2 to one person.",
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

	// Regional variants resolve to their base language.
	ru, err := NewMessages("ru-RU")
	if err != nil {
		t.Fatalf("NewMessages(ru-RU): %v", err)
	}
	wantRu := "На этой неделе пока никто не получил карму."
	if got := ru.topEmpty(); got != wantRu {
		t.Errorf("ru-RU topEmpty = %q, want %q", got, wantRu)
	}

	// A well-formed but unsupported language falls back to English.
	de, err := NewMessages("de")
	if err != nil {
		t.Fatalf("NewMessages(de): %v", err)
	}
	if got := de.topEmpty(); got != en.topEmpty() {
		t.Errorf("de topEmpty = %q, want English %q", got, en.topEmpty())
	}

	// Garbage fails fast at startup.
	if _, err := NewMessages("not a language!"); err == nil {
		t.Error("NewMessages(garbage) succeeded, want error")
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
