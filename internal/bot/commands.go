package bot

import (
	"context"
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
		b.reply(ctx, channel.Id, b.msg.noNegative())
	default:
		// Includes a bare mention and unknown verbs: show the cheat sheet.
		b.reply(ctx, channel.Id,
			b.msg.help(b.botUsername, b.limits, b.period))
	}
}

// handleDirect dispatches a direct message to the bot.
func (b *Bot) handleDirect(ctx context.Context, post *model.Post) {
	switch strings.ToLower(strings.TrimSpace(post.Message)) {
	case "karma":
		b.cmdMyKarma(ctx, post.ChannelId, post.UserId)
	case "help":
		b.reply(ctx, post.ChannelId,
			b.msg.help(b.botUsername, b.limits, b.period))
	default:
		b.reply(ctx, post.ChannelId, b.msg.dmUnknown())
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
	b.reply(ctx, channel.Id, b.msg.started(b.botUsername))
}

func (b *Bot) cmdStop(ctx context.Context, channelID string) {
	if err := b.store.DisableChannel(ctx, channelID); err != nil {
		b.internal(ctx, channelID, err, "disabling channel")
		return
	}
	b.reply(ctx, channelID, b.msg.stopped())
}

func (b *Bot) cmdTop(ctx context.Context, channelID string) {
	enabled, err := b.store.IsChannelEnabled(ctx, channelID)
	if err != nil {
		b.internal(ctx, channelID, err, "checking channel state")
		return
	}
	if !enabled {
		b.reply(ctx, channelID, b.msg.notStarted(b.botUsername))
		return
	}

	entries, err := b.store.TopByKarma(ctx, channelID, b.period.Key(b.now()), topLimit)
	if err != nil {
		b.internal(ctx, channelID, err, "reading top karma")
		return
	}
	if len(entries) == 0 {
		b.reply(ctx, channelID, b.msg.topEmpty())
		return
	}
	b.reply(ctx, channelID, b.msg.top(topLimit, entries))
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
		b.reply(ctx, channelID, b.msg.notStarted(b.botUsername))
		return
	}

	targets := extractMentions(args, b.botUsername)
	if len(targets) == 0 {
		b.reply(ctx, channelID, b.msg.noTargets(b.botUsername))
		return
	}

	now := b.now()
	week := b.period.Key(now)
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
			lines = append(lines, b.msg.userNotFound(name))
			continue
		}
		if user.Id == post.UserId {
			lines = append(lines, b.msg.selfKarma(user.Username))
			continue
		}

		resolved = append(resolved, resolvedTarget{
			username: user.Username,
			userID:   user.Id,
		})

		switch {
		case b.limits.totalExhausted(totalGiven):
			lines = append(lines, b.msg.dailyTotalMax(user.Username, b.limits.DailyTotal))
		case b.limits.perTargetExhausted(perTarget[user.Id]):
			lines = append(lines,
				b.msg.perTargetMax(user.Username, b.limits.DailyPerTarget))
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
				lines = append(lines, b.msg.grantFailed(user.Username))
				continue
			}
			totalGiven++
			perTarget[user.Id]++
			lines = append(lines, b.msg.applied(user.Username, karmaStars(newTotal)))
		}
	}

	if footer := b.budgetFooter(totalGiven, resolved, perTarget); footer != "" {
		lines = append(lines, "", footer)
	}
	b.reply(ctx, channelID, strings.Join(lines, "\n"))
}

// budgetFooter renders the giver's remaining daily budget: the total and
// the share for each resolved target. Unlimited budgets are omitted, so
// the footer is empty when nothing is limited.
func (b *Bot) budgetFooter(
	totalGiven int,
	targets []resolvedTarget,
	perTarget map[string]int,
) string {
	parts := []string{}
	if b.limits.DailyTotal > 0 {
		parts = append(parts,
			b.msg.budgetsPrefix(b.limits.DailyTotal-totalGiven, b.limits.DailyTotal))
	}
	if b.limits.DailyPerTarget > 0 {
		for _, t := range targets {
			used := perTarget[t.userID]
			parts = append(parts,
				b.msg.budgetsTarget(t.username, b.limits.DailyPerTarget-used, b.limits.DailyPerTarget))
		}
	}
	return strings.Join(parts, " · ")
}

func (b *Bot) cmdMyKarma(ctx context.Context, channelID, userID string) {
	entries, err := b.store.UserKarmaByChannel(ctx, userID, b.period.Key(b.now()))
	if err != nil {
		b.internal(ctx, channelID, err, "reading user karma")
		return
	}
	b.reply(ctx, channelID, b.msg.dmReport(entries))
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
