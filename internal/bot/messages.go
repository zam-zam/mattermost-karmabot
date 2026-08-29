package bot

import (
	"fmt"
	"strconv"
	"strings"

	"karmabot/internal/storage"
)

// All user-facing text is Russian; commands themselves are English.
const (
	msgStarted       = "✅ Карма включена. Благодарите так: `@%s ++ @username`"
	msgStopped       = "⏸ Карма выключена. Статистика сохранена — `start` продолжит текущую неделю."
	msgNotStarted    = "❗ Карма в этом канале не включена. Напишите `@%s start`."
	msgSelfKarma     = "😏 @%s: себе карму повысить нельзя."
	msgNoTargets     = "Укажите, кого благодарите: `@%s ++ @username`"
	msgUserNotFound  = "🤔 Пользователь @%s не найден."
	msgDailyTotalMax = "❌ @%s: на сегодня лимит исчерпан — всего не больше %d кармы в день."
	msgPerTargetMax  = "❌ @%s: этому человеку на сегодня хватит — не больше %d кармы в день."
	msgApplied       = "✅ @%s: +1 (карма в канале: %s)"
	msgGrantFailed   = "⚠️ @%s: не получилось начислить карму — попробуйте позже."
	msgInternal      = "⚠️ Что-то сломалось на моей стороне. Попробуйте ещё раз позже."
	msgBudgetsPrefix = "Осталось на сегодня: %d из %d"
	msgBudgetsTarget = "@%s: %d из %d"
	msgNoNegative    = "🙂 Карму нельзя понижать — только `++`."
	msgTopHeader     = "🏅 Топ-%d за неделю:"
	msgTopEmpty      = "На этой неделе пока никто не получил карму."
	msgWeeklyHeader  = "🏁 Итоги недели:"
	msgDMHeader      = "Твоя карма за текущую неделю:"
	msgDMEmpty       = "На этой неделе у тебя пока нет кармы."
	msgDMUnknown     = "Не понял команду. Доступно: `karma` — твоя карма, `help` — справка."
)

func helpMessage(botUsername string) string {
	lines := []string{
		"🤖 Карма-бот",
		"",
		"Команды:",
		fmt.Sprintf("• `@%s start` — включить карму в канале", botUsername),
		fmt.Sprintf("• `@%s stop` — выключить карму в канале", botUsername),
		fmt.Sprintf("• `@%s ++ @user [@user2 …]` — дать карму (+1 каждому)", botUsername),
		fmt.Sprintf("• `@%s top` — топ-10 за неделю", botUsername),
		fmt.Sprintf("• `@%s help` — справка", botUsername),
		"",
		fmt.Sprintf("Лимиты: не больше %d кармы в день, из них не больше %d одному человеку.",
			dailyTotalLimit, dailyPerTargetLimit),
		"Неделя завершается в понедельник в 00:00 UTC — перед этим бот объявит итоги.",
		"В личных сообщениях: `karma` — твоя карма по каналам.",
	}
	return strings.Join(lines, "\n")
}

// karmaStars renders a score as one star per point followed by the total,
// e.g. 3 → "⭐⭐⭐ 3".
func karmaStars(karma int) string {
	stars := strings.Repeat("⭐", karma)
	if karma == 0 {
		return "0"
	}
	return stars + " " + strconv.Itoa(karma)
}

// formatRanked renders a numbered scoreboard, e.g. the weekly top.
func formatRanked(header string, entries []storage.KarmaEntry) string {
	lines := make([]string, 0, len(entries)+1)
	lines = append(lines, header)
	for i, e := range entries {
		lines = append(lines, fmt.Sprintf("%d. @%s — %s", i+1, e.Username, karmaStars(e.Karma)))
	}
	return strings.Join(lines, "\n")
}

// FormatWeeklyTop renders the end-of-week message the scheduler posts to
// each channel.
func FormatWeeklyTop(entries []storage.KarmaEntry) string {
	return formatRanked(msgWeeklyHeader, entries)
}

func formatDMReport(entries []storage.ChannelKarma) string {
	if len(entries) == 0 {
		return msgDMEmpty
	}
	lines := make([]string, 0, len(entries)+1)
	lines = append(lines, msgDMHeader)
	for _, e := range entries {
		lines = append(lines, fmt.Sprintf("• %s — %s", e.ChannelName, karmaStars(e.Karma)))
	}
	return strings.Join(lines, "\n")
}
