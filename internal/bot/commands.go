package bot

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"

	"karmabot/internal/storage"
)

// Command verbs; commands are English-only by design.
const (
	verbStart = "start"
	verbStop  = "stop"
	verbTop   = "top"
	verbHelp  = "help"
	verbGrant = "++"
)

var mentionRe = regexp.MustCompile(`@([A-Za-z0-9._-]+)`)

// handleChannel dispatches a channel message that starts with a bot
// mention. Messages that merely mention the bot mid-text are ignored.
func (b *Bot) handleChannel(
	ctx context.Context,
	channel *model.Channel,
	post *model.Post,
) {
	rest, ok := stripBotMention(post.Message, b.botUsername)
	if !ok {
		return
	}

	verb, args := splitVerb(rest)
	switch verb {
	case verbStart:
		b.cmdStart(ctx, channel)
	case verbStop:
		b.cmdStop(ctx, channel.Id)
	case verbTop:
		b.cmdTop(ctx, channel.Id)
	case verbGrant:
		b.cmdGrant(ctx, channel.Id, post, args)
	case "--":
		b.reply(ctx, channel.Id, msgNoNegative)
	default:
		// Includes a bare mention and unknown verbs: show the cheat sheet.
		b.reply(ctx, channel.Id, helpMessage(b.botUsername))
	}
}

// handleDirect dispatches a direct message to the bot.
func (b *Bot) handleDirect(ctx context.Context, post *model.Post) {
	switch strings.ToLower(strings.TrimSpace(post.Message)) {
	case "karma":
		b.cmdMyKarma(ctx, post.ChannelId, post.UserId)
	case "help":
		b.reply(ctx, post.ChannelId, helpMessage(b.botUsername))
	default:
		b.reply(ctx, post.ChannelId, msgDMUnknown)
	}
}

func (b *Bot) cmdStart(ctx context.Context, channel *model.Channel) {
	name := channel.DisplayName
	if name == "" {
		name = channel.Name
	}

	err := b.store.EnableChannel(ctx, storage.Channel{
		ID:     channel.Id,
		TeamID: channel.TeamId,
		Name:   name,
	})
	if err != nil {
		b.internal(ctx, channel.Id, err, "enabling channel")
		return
	}
	b.reply(ctx, channel.Id, fmt.Sprintf(msgStarted, b.botUsername))
}

func (b *Bot) cmdStop(ctx context.Context, channelID string) {
	if err := b.store.DisableChannel(ctx, channelID); err != nil {
		b.internal(ctx, channelID, err, "disabling channel")
		return
	}
	b.reply(ctx, channelID, msgStopped)
}

func (b *Bot) cmdTop(ctx context.Context, channelID string) {
	enabled, err := b.store.IsChannelEnabled(ctx, channelID)
	if err != nil {
		b.internal(ctx, channelID, err, "checking channel state")
		return
	}
	if !enabled {
		b.reply(ctx, channelID, fmt.Sprintf(msgNotStarted, b.botUsername))
		return
	}

	entries, err := b.store.TopByKarma(ctx, channelID, WeekKey(b.now()), topLimit)
	if err != nil {
		b.internal(ctx, channelID, err, "reading top karma")
		return
	}
	if len(entries) == 0 {
		b.reply(ctx, channelID, msgTopEmpty)
		return
	}
	b.reply(ctx, channelID, formatRanked(fmt.Sprintf(msgTopHeader, topLimit), entries))
}

// resolvedTarget pairs a mention with its user, for budgets and replies.
type resolvedTarget struct {
	username string
	userID   string
}

// cmdGrant gives +1 karma to every mentioned user, enforcing the daily
// limits and replying once with per-target results and the remaining
// budgets.
func (b *Bot) cmdGrant(
	ctx context.Context,
	channelID string,
	post *model.Post,
	args string,
) {
	enabled, err := b.store.IsChannelEnabled(ctx, channelID)
	if err != nil {
		b.internal(ctx, channelID, err, "checking channel state")
		return
	}
	if !enabled {
		b.reply(ctx, channelID, fmt.Sprintf(msgNotStarted, b.botUsername))
		return
	}

	targets := extractMentions(args, b.botUsername)
	if len(targets) == 0 {
		b.reply(ctx, channelID, fmt.Sprintf(msgNoTargets, b.botUsername))
		return
	}

	now := b.now()
	week := WeekKey(now)
	day := DayKey(now)

	totalGiven, perTarget, err := b.store.GivenOnDay(ctx, channelID, post.UserId, day)
	if err != nil {
		b.internal(ctx, channelID, err, "reading daily usage")
		return
	}

	lines := []string{}
	resolved := []resolvedTarget{}
	for _, name := range targets {
		user, err := b.client.GetUserByUsername(ctx, name)
		if err != nil {
			lines = append(lines, fmt.Sprintf(msgUserNotFound, name))
			continue
		}
		if user.Id == post.UserId {
			lines = append(lines, fmt.Sprintf(msgSelfKarma, user.Username))
			continue
		}

		resolved = append(resolved, resolvedTarget{
			username: user.Username,
			userID:   user.Id,
		})

		switch {
		case totalGiven >= dailyTotalLimit:
			lines = append(lines,
				fmt.Sprintf(msgDailyTotalMax, user.Username, dailyTotalLimit))
		case perTarget[user.Id] >= dailyPerTargetLimit:
			lines = append(lines,
				fmt.Sprintf(msgPerTargetMax, user.Username, dailyPerTargetLimit))
		default:
			newTotal, err := b.store.GrantKarma(ctx, storage.Grant{
				ChannelID:      channelID,
				Week:           week,
				Day:            day,
				GiverID:        post.UserId,
				TargetID:       user.Id,
				TargetUsername: user.Username,
			})
			if err != nil {
				b.log.Error("granting karma", "err", err, "channel_id", channelID)
				lines = append(lines, fmt.Sprintf(msgGrantFailed, user.Username))
				continue
			}
			totalGiven++
			perTarget[user.Id]++
			lines = append(lines, fmt.Sprintf(msgApplied, user.Username, karmaStars(newTotal)))
		}
	}

	lines = append(lines, "", b.budgetFooter(totalGiven, resolved, perTarget))
	b.reply(ctx, channelID, strings.Join(lines, "\n"))
}

// budgetFooter renders the giver's remaining daily budget: the total and
// the share for each resolved target.
func (b *Bot) budgetFooter(
	totalGiven int,
	targets []resolvedTarget,
	perTarget map[string]int,
) string {
	parts := []string{
		fmt.Sprintf(msgBudgetsPrefix, dailyTotalLimit-totalGiven, dailyTotalLimit),
	}
	for _, t := range targets {
		used := perTarget[t.userID]
		parts = append(parts,
			fmt.Sprintf(msgBudgetsTarget, t.username, dailyPerTargetLimit-used, dailyPerTargetLimit))
	}
	return strings.Join(parts, " · ")
}

func (b *Bot) cmdMyKarma(ctx context.Context, channelID, userID string) {
	entries, err := b.store.UserKarmaByChannel(ctx, userID, WeekKey(b.now()))
	if err != nil {
		b.internal(ctx, channelID, err, "reading user karma")
		return
	}
	b.reply(ctx, channelID, formatDMReport(entries))
}

// stripBotMention splits "‹@bot …rest›" when the message starts with the
// bot mention, returning the command text. Mention lengths are computed
// on the original string because strings.ToLower can change byte length.
func stripBotMention(message, botUsername string) (string, bool) {
	trimmed := strings.TrimSpace(message)

	token := trimmed
	if i := strings.IndexAny(trimmed, " \t\n"); i >= 0 {
		token = trimmed[:i]
	}
	if !strings.EqualFold(token, "@"+botUsername) {
		return "", false
	}
	return strings.TrimSpace(trimmed[len(token):]), true
}

// splitVerb returns the lowercase first word and the remaining text.
func splitVerb(rest string) (string, string) {
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", ""
	}
	verb := strings.ToLower(fields[0])
	return verb, strings.TrimSpace(strings.Join(fields[1:], " "))
}

// extractMentions returns unique mentioned usernames in order of
// appearance, skipping the bot itself.
func extractMentions(text, botUsername string) []string {
	seen := map[string]bool{}
	targets := []string{}

	for _, m := range mentionRe.FindAllStringSubmatch(text, -1) {
		name := m[1]
		isBot := strings.EqualFold(name, botUsername)
		if isBot || seen[name] {
			continue
		}
		seen[name] = true
		targets = append(targets, name)
	}
	return targets
}
