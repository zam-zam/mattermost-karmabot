package bot

import (
	"embed"
	"fmt"
	"strconv"
	"strings"
	"time"

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
	tag language.Tag
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
	return &Messages{loc: i18n.NewLocalizer(bundle, best.String()), tag: best}, nil
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

// topHeader renders the leaderboard header for the period containing now.
func (m *Messages) topHeader(limit int, p Period, now time.Time) string {
	return m.t(p.kindID("topHeaderWeek", "topHeaderMonth"), map[string]any{
		"TopLimit": limit,
		"Bounds":   m.periodBounds(p, p.Start(now)),
	})
}

func (m *Messages) topEmpty(p Period) string {
	return m.t(p.kindID("topEmptyWeek", "topEmptyMonth"), nil)
}

// PeriodTop renders the end-of-period message the scheduler posts to
// each channel; start is the finished period's rollover instant.
func (m *Messages) PeriodTop(entries []storage.KarmaEntry, p Period, start time.Time) string {
	return m.formatRanked(
		m.t(p.kindID("weekResultsHeader", "monthResultsHeader"),
			map[string]any{"Bounds": m.periodBounds(p, start)}),
		entries,
	)
}

// PeriodStarted renders the announcement of a freshly begun period.
func (m *Messages) PeriodStarted(p Period, start time.Time) string {
	return m.t(p.kindID("weekStarted", "monthStarted"),
		map[string]any{"Bounds": m.periodBounds(p, start)})
}

func (m *Messages) dmHeader(p Period, now time.Time) string {
	return m.t(p.kindID("dmHeaderWeek", "dmHeaderMonth"), map[string]any{
		"Bounds": m.periodBounds(p, p.Start(now)),
	})
}

func (m *Messages) dmEmpty(p Period) string {
	return m.t(p.kindID("dmEmptyWeek", "dmEmptyMonth"), nil)
}

func (m *Messages) dmUnknown() string {
	return m.t("dmUnknown", nil)
}

func (m *Messages) notAdmin() string {
	return m.t("notAdmin", nil)
}

func (m *Messages) statusHeader() string {
	return m.t("statusHeader", nil)
}

func (m *Messages) statusEmpty() string {
	return m.t("statusEmpty", nil)
}

func (m *Messages) statusEntry(channelName, channelID string) string {
	return m.t("statusEntry", map[string]any{
		"ChannelName": channelName,
		"ChannelID":   channelID,
	})
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

func (m *Messages) help(botUsername string, limits Limits, period Period) string {
	return m.t("help", map[string]any{
		"BotUsername": botUsername,
		"LimitsLine":  m.limitsLine(limits),
		"PeriodLine":  m.periodLine(period),
	})
}

// periodLine renders the help sentence about the period cadence and
// rollover time.
func (m *Messages) periodLine(period Period) string {
	return m.t(period.kindID("periodLineWeek", "periodLineMonth"), map[string]any{
		"Time": period.Rollover(),
		"Zone": period.Loc.String(),
	})
}

// periodBounds renders the human-readable bounds of the period starting
// at start: a date range for weeks, the month name for months.
func (m *Messages) periodBounds(p Period, start time.Time) string {
	local := start.In(p.Loc)
	if p.Kind == KindMonth {
		return m.t("monthBounds", map[string]any{
			"Month": m.monthName(local.Month(), false),
		})
	}
	end := p.End(start).In(p.Loc)
	return m.t("weekBounds", map[string]any{
		"StartDay":   local.Day(),
		"StartMonth": m.monthName(local.Month(), true),
		"EndDay":     end.Day(),
		"EndMonth":   m.monthName(end.Month(), true),
	})
}

// monthName localizes a month name, in the genitive case when the active
// language's date ranges call for it (Russian: «25 августа»).
func (m *Messages) monthName(month time.Month, genitive bool) string {
	id := fmt.Sprintf("month%d", month)
	if genitive && m.tag == language.Russian {
		id = fmt.Sprintf("monthGen%d", month)
	}
	return m.t(id, nil)
}

// limitsLine renders the help sentence describing the daily limits,
// adapting to which of them are set.
func (m *Messages) limitsLine(limits Limits) string {
	switch {
	case limits.DailyTotal > 0 && limits.DailyPerTarget > 0:
		return m.t("limitsLineBoth", map[string]any{
			"DailyTotal":     limits.DailyTotal,
			"DailyPerTarget": limits.DailyPerTarget,
		})
	case limits.DailyTotal > 0:
		return m.t("limitsLineTotal", map[string]any{"DailyTotal": limits.DailyTotal})
	case limits.DailyPerTarget > 0:
		return m.t("limitsLinePerTarget", map[string]any{
			"DailyPerTarget": limits.DailyPerTarget,
		})
	default:
		return m.t("limitsLineNone", nil)
	}
}

// top renders the current period's leaderboard for a channel.
func (m *Messages) top(limit int, p Period, now time.Time, entries []storage.KarmaEntry) string {
	return m.formatRanked(m.topHeader(limit, p, now), entries)
}

// dmReport renders the direct-message summary of a user's karma per
// channel for the period containing now.
func (m *Messages) dmReport(entries []storage.ChannelKarma, p Period, now time.Time) string {
	if len(entries) == 0 {
		return m.dmEmpty(p)
	}
	lines := m.dmLines(entries, p, now)
	return strings.Join(lines, "\n")
}

// status renders the admin's list of channels with karma enabled.
func (m *Messages) status(channels []storage.Channel) string {
	if len(channels) == 0 {
		return m.statusEmpty()
	}
	lines := make([]string, 0, len(channels)+1)
	lines = append(lines, m.statusHeader())
	for _, ch := range channels {
		lines = append(lines, m.statusEntry(ch.Name, ch.ID))
	}
	return strings.Join(lines, "\n")
}

func (m *Messages) dmLines(entries []storage.ChannelKarma, p Period, now time.Time) []string {
	lines := make([]string, 0, len(entries)+1)
	lines = append(lines, m.dmHeader(p, now))
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
