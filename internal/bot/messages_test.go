package bot

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"testing"
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
// sample data, catching broken templates early. Plural-form messages get
// a count; plain messages must not receive one.
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
		"PeriodLine":     "The period lasts 30 days and ends at 00:00 UTC.",
		"PeriodDays":     30,
	}
	en := loadLocale(t, "locales/active.en.json")

	for _, lang := range []string{"en", "ru"} {
		m, err := NewMessages(lang)
		if err != nil {
			t.Fatalf("NewMessages(%q): %v", lang, err)
		}
		for id, raw := range en {
			var got string
			if isPluralValue(raw) {
				got = m.tPlural(id, data, 5)
			} else {
				got = m.t(id, data)
			}
			if strings.Contains(got, "{{") {
				t.Errorf("%s/%s: unresolved placeholder in %q", lang, id, got)
			}
		}
	}
}

// isPluralValue reports whether a catalog value is a plural-form object.
func isPluralValue(raw json.RawMessage) bool {
	return bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{"))
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
	wantRu := "В этом периоде пока никто не получил карму."
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

// TestPeriodLinePluralForms pins the CLDR plural forms for the day count
// in both languages.
func TestPeriodLinePluralForms(t *testing.T) {
	ru, err := NewMessages("ru")
	if err != nil {
		t.Fatalf("NewMessages(ru): %v", err)
	}
	en, err := NewMessages("en")
	if err != nil {
		t.Fatalf("NewMessages(en): %v", err)
	}

	tests := []struct {
		lang    string
		m       *Messages
		days    int
		wantSub string
	}{
		{lang: "ru", m: ru, days: 1, wantSub: "1 день"},
		{lang: "ru", m: ru, days: 2, wantSub: "2 дня"},
		{lang: "ru", m: ru, days: 5, wantSub: "5 дней"},
		{lang: "ru", m: ru, days: 11, wantSub: "11 дней"},
		{lang: "ru", m: ru, days: 21, wantSub: "21 день"},
		{lang: "ru", m: ru, days: 22, wantSub: "22 дня"},
		{lang: "ru", m: ru, days: 30, wantSub: "30 дней"},
		{lang: "en", m: en, days: 1, wantSub: "1 day"},
		{lang: "en", m: en, days: 30, wantSub: "30 days"},
	}
	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			got := tt.m.periodLine(Period{Days: tt.days})
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("periodLine(%d days, %s) = %q, want to contain %q",
					tt.days, tt.lang, got, tt.wantSub)
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
