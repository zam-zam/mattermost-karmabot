package bot

import (
	"embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"karmabot/internal/storage"
)

//go:embed locales/*.json
var localeFS embed.FS

// supportedMatcher maps the configured language onto the languages the bot
// actually speaks; English is the fallback for anything else.
var supportedMatcher = language.NewMatcher([]language.Tag{
	language.English,
	language.Russian,
})

// localeFiles lists the embedded catalogs, default language first.
var localeFiles = []string{
	"locales/active.en.json",
	"locales/active.ru.json",
}

// Messages renders user-facing text in the configured language. Commands
// themselves are English-only.
type Messages struct {
	loc *i18n.Localizer
}

// NewMessages builds the message catalog for lang, e.g. "en" or "ru"
// (regional variants like "ru-RU" are accepted). A well-formed language
// outside the supported set falls back to English; unparseable input is an
// error.
func NewMessages(lang string) (*Messages, error) {
	tag, err := language.Parse(lang)
	if err != nil {
		return nil, fmt.Errorf("parsing language %q: %w", lang, err)
	}

	bundle := i18n.NewBundle(language.English)
	for _, name := range localeFiles {
		raw, err := localeFS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("reading embedded locale %s: %w", name, err)
		}
		if _, err := bundle.ParseMessageFileBytes(raw, name); err != nil {
			return nil, fmt.Errorf("parsing locale %s: %w", name, err)
		}
	}

	best, _, _ := supportedMatcher.Match(tag)
	return &Messages{loc: i18n.NewLocalizer(bundle, best.String())}, nil
}

// t localizes one message. MustLocalize is safe because the locale parity
// test guarantees every ID exists in the default catalog.
func (m *Messages) t(id string, data map[string]any) string {
	return m.loc.MustLocalize(&i18n.LocalizeConfig{
		MessageID:    id,
		TemplateData: data,
	})
}

func (m *Messages) started(botUsername string) string {
	return m.t("started", map[string]any{"BotUsername": botUsername})
}

func (m *Messages) stopped() string {
	return m.t("stopped", nil)
}

func (m *Messages) notStarted(botUsername string) string {
	return m.t("notStarted", map[string]any{"BotUsername": botUsername})
}

func (m *Messages) selfKarma(username string) string {
	return m.t("selfKarma", map[string]any{"Username": username})
}

func (m *Messages) noTargets(botUsername string) string {
	return m.t("noTargets", map[string]any{"BotUsername": botUsername})
}

func (m *Messages) userNotFound(username string) string {
	return m.t("userNotFound", map[string]any{"Username": username})
}

func (m *Messages) dailyTotalMax(username string, limit int) string {
	return m.t("dailyTotalMax", map[string]any{
		"Username": username,
		"Limit":    limit,
	})
}

func (m *Messages) perTargetMax(username string, limit int) string {
	return m.t("perTargetMax", map[string]any{
		"Username": username,
		"Limit":    limit,
	})
}

func (m *Messages) applied(username, stars string) string {
	return m.t("applied", map[string]any{
		"Username": username,
		"Stars":    stars,
	})
}

func (m *Messages) grantFailed(username string) string {
	return m.t("grantFailed", map[string]any{"Username": username})
}

func (m *Messages) internalError() string {
	return m.t("internalError", nil)
}

func (m *Messages) budgetsPrefix(remaining, limit int) string {
	return m.t("budgetsPrefix", map[string]any{
		"Remaining": remaining,
		"Limit":     limit,
	})
}

func (m *Messages) budgetsTarget(username string, remaining, limit int) string {
	return m.t("budgetsTarget", map[string]any{
		"Username":  username,
		"Remaining": remaining,
		"Limit":     limit,
	})
}

func (m *Messages) noNegative() string {
	return m.t("noNegative", nil)
}

func (m *Messages) topHeader(limit int) string {
	return m.t("topHeader", map[string]any{"TopLimit": limit})
}

func (m *Messages) topEmpty() string {
	return m.t("topEmpty", nil)
}

func (m *Messages) weeklyHeader() string {
	return m.t("weeklyHeader", nil)
}

func (m *Messages) dmHeader() string {
	return m.t("dmHeader", nil)
}

func (m *Messages) dmEmpty() string {
	return m.t("dmEmpty", nil)
}

func (m *Messages) dmUnknown() string {
	return m.t("dmUnknown", nil)
}

func (m *Messages) rankedEntry(position int, username, stars string) string {
	return m.t("rankedEntry", map[string]any{
		"Position": position,
		"Username": username,
		"Stars":    stars,
	})
}

func (m *Messages) dmEntry(channelName, stars string) string {
	return m.t("dmEntry", map[string]any{
		"ChannelName": channelName,
		"Stars":       stars,
	})
}

func (m *Messages) help(botUsername string, dailyTotal, dailyPerTarget int) string {
	return m.t("help", map[string]any{
		"BotUsername":    botUsername,
		"DailyTotal":     dailyTotal,
		"DailyPerTarget": dailyPerTarget,
	})
}

// top renders the current week's leaderboard for a channel.
func (m *Messages) top(limit int, entries []storage.KarmaEntry) string {
	return m.formatRanked(m.topHeader(limit), entries)
}

// WeeklyTop renders the end-of-week message the scheduler posts to each
// channel.
func (m *Messages) WeeklyTop(entries []storage.KarmaEntry) string {
	return m.formatRanked(m.weeklyHeader(), entries)
}

// dmReport renders the direct-message summary of a user's karma per
// channel.
func (m *Messages) dmReport(entries []storage.ChannelKarma) string {
	if len(entries) == 0 {
		return m.dmEmpty()
	}
	return strings.Join(m.dmLines(entries), "\n")
}

func (m *Messages) dmLines(entries []storage.ChannelKarma) []string {
	lines := make([]string, 0, len(entries)+1)
	lines = append(lines, m.dmHeader())
	for _, e := range entries {
		lines = append(lines, m.dmEntry(e.ChannelName, karmaStars(e.Karma)))
	}
	return lines
}

func (m *Messages) formatRanked(header string, entries []storage.KarmaEntry) string {
	lines := make([]string, 0, len(entries)+1)
	lines = append(lines, header)
	for i, e := range entries {
		lines = append(lines, m.rankedEntry(i+1, e.Username, karmaStars(e.Karma)))
	}
	return strings.Join(lines, "\n")
}

// karmaStars renders a score as one star per point followed by the total,
// e.g. 3 → "⭐⭐⭐ 3". Locale-neutral: emoji and digits only.
func karmaStars(karma int) string {
	stars := strings.Repeat("⭐", karma)
	if karma == 0 {
		return "0"
	}
	return stars + " " + strconv.Itoa(karma)
}
