// Package scheduler posts the period summary to every enabled channel
// at each period rollover (every KARMABOT_PERIOD_DAYS days at 00:00 UTC).
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"karmabot/internal/bot"
	"karmabot/internal/storage"
)

// weeklyTopLimit is how many leaders the end-of-period message shows.
const weeklyTopLimit = 5

// Store is the storage surface the scheduler needs.
type Store interface {
	EnabledChannels(ctx context.Context) ([]storage.Channel, error)
	TopByKarma(
		ctx context.Context,
		channelID, week string,
		limit int,
	) ([]storage.KarmaEntry, error)
	PruneDailyGiven(ctx context.Context, day string) error
}

// Poster delivers messages to channels; a reply thread root may be
// given, with an empty root posting a channel-top message.
type Poster interface {
	CreatePost(ctx context.Context, channelID, rootID, message string) error
}

// Scheduler publishes the finished period's top list when the period
// ends. The reset itself is implicit: karma is keyed by period, so new
// karma after the rollover lands in the fresh period automatically.
type Scheduler struct {
	store  Store
	poster Poster
	msgs   *bot.Messages
	period bot.Period
	log    *slog.Logger
	now    func() time.Time
}

// New creates the scheduler; summaries are rendered by msgs in the
// configured language and periods follow the configured length.
func New(
	store Store,
	poster Poster,
	log *slog.Logger,
	msgs *bot.Messages,
	period bot.Period,
) *Scheduler {
	return &Scheduler{
		store:  store,
		poster: poster,
		msgs:   msgs,
		period: period,
		log:    log,
		now:    time.Now,
	}
}

// Run blocks until ctx is cancelled, publishing a summary at every
// period rollover.
func (s *Scheduler) Run(ctx context.Context) {
	for {
		next := s.period.NextReset(s.now())
		s.log.Info("period summary scheduled", "at", next.Format(time.RFC3339))

		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		s.publishFinishedWeek(ctx)
	}
}

func (s *Scheduler) publishFinishedWeek(ctx context.Context) {
	now := s.now()
	finishedPeriod := s.period.PreviousKey(now)

	channels, err := s.store.EnabledChannels(ctx)
	if err != nil {
		s.log.Error("listing channels for period summary", "err", err)
	}

	for _, ch := range channels {
		top, err := s.store.TopByKarma(ctx, ch.ID, finishedPeriod, weeklyTopLimit)
		if err != nil {
			s.log.Error("reading period top", "err", err, "channel_id", ch.ID)
			continue
		}
		if len(top) == 0 {
			continue
		}

		// The summary answers no command, so it starts its own thread.
		if err := s.poster.CreatePost(ctx, ch.ID, "", s.msgs.PeriodTop(top)); err != nil {
			s.log.Error("posting period summary", "err", err, "channel_id", ch.ID)
		}
	}

	if err := s.store.PruneDailyGiven(ctx, bot.DayKey(now)); err != nil {
		s.log.Error("pruning daily usage", "err", err)
	}
}
