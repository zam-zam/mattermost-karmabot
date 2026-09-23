// Package scheduler posts the finished period's results and the new
// period's announcement to every enabled channel at each period rollover.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"karmabot/internal/bot"
	"karmabot/internal/storage"
)

// periodTopLimit is how many leaders the results message shows.
const periodTopLimit = 5

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

// Scheduler publishes the finished period's top list and announces the
// new period when the period ends. The reset itself is implicit: karma is
// keyed by period, so new karma after the rollover lands in the fresh
// period automatically.
type Scheduler struct {
	store  Store
	poster Poster
	msgs   *bot.Messages
	period bot.Period
	log    *slog.Logger
	now    func() time.Time
}

// New creates the scheduler; messages are rendered by msgs in the
// configured language and periods follow the configured segmentation.
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

// Run blocks until ctx is cancelled, publishing at every period
// rollover.
func (s *Scheduler) Run(ctx context.Context) {
	for {
		next := s.period.NextReset(s.now())
		s.log.Info("period rollover scheduled", "at", next.Format(time.RFC3339))

		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		s.publishFinishedPeriod(ctx)
	}
}

// publishFinishedPeriod posts, per enabled channel, the finished
// period's results (only when there is anything to show) and then the
// new period's announcement with its bounds, and prunes stale daily
// usage rows.
func (s *Scheduler) publishFinishedPeriod(ctx context.Context) {
	now := s.now()
	finishedStart := s.period.PreviousStart(now)
	finishedKey := s.period.PreviousKey(now)
	newStart := s.period.Start(now)

	channels, err := s.store.EnabledChannels(ctx)
	if err != nil {
		s.log.Error("listing channels for period results", "err", err)
	}

	for _, ch := range channels {
		top, err := s.store.TopByKarma(ctx, ch.ID, finishedKey, periodTopLimit)
		if err != nil {
			s.log.Error("reading period top", "err", err, "channel_id", ch.ID)
			continue
		}
		if len(top) > 0 {
			// The results answer no command, so they start their own thread.
			if err := s.poster.CreatePost(ctx, ch.ID, "",
				s.msgs.PeriodTop(top, s.period, finishedStart)); err != nil {
				s.log.Error("posting period results", "err", err, "channel_id", ch.ID)
			}
		}

		if err := s.poster.CreatePost(ctx, ch.ID, "",
			s.msgs.PeriodStarted(s.period, newStart)); err != nil {
			s.log.Error("posting period announcement", "err", err, "channel_id", ch.ID)
		}
	}

	if err := s.store.PruneDailyGiven(ctx, s.period.DayKey(now)); err != nil {
		s.log.Error("pruning daily usage", "err", err)
	}
}
