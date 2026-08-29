// Package scheduler posts the weekly karma summary to every enabled
// channel at the Monday 03:00 MSK rollover.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"karmabot/internal/bot"
	"karmabot/internal/storage"
)

// weeklyTopLimit is how many leaders the end-of-week message shows.
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

// Poster delivers messages to channels.
type Poster interface {
	CreatePost(ctx context.Context, channelID, message string) error
}

// Scheduler publishes the finished week's top list when the period ends.
// The reset itself is implicit: karma is keyed by week, so new karma
// after the rollover lands in the fresh week automatically.
type Scheduler struct {
	store  Store
	poster Poster
	log    *slog.Logger
	now    func() time.Time
}

// New creates the scheduler.
func New(store Store, poster Poster, log *slog.Logger) *Scheduler {
	return &Scheduler{
		store:  store,
		poster: poster,
		log:    log,
		now:    time.Now,
	}
}

// Run blocks until ctx is cancelled, publishing a summary at every weekly
// rollover.
func (s *Scheduler) Run(ctx context.Context) {
	for {
		next := bot.NextWeeklyReset(s.now())
		s.log.Info("weekly summary scheduled", "at", next.Format(time.RFC3339))

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
	finishedWeek := bot.PreviousWeekKey(now)

	channels, err := s.store.EnabledChannels(ctx)
	if err != nil {
		s.log.Error("listing channels for weekly summary", "err", err)
	}

	for _, ch := range channels {
		top, err := s.store.TopByKarma(ctx, ch.ID, finishedWeek, weeklyTopLimit)
		if err != nil {
			s.log.Error("reading weekly top", "err", err, "channel_id", ch.ID)
			continue
		}
		if len(top) == 0 {
			continue
		}

		if err := s.poster.CreatePost(ctx, ch.ID, bot.FormatWeeklyTop(top)); err != nil {
			s.log.Error("posting weekly summary", "err", err, "channel_id", ch.ID)
		}
	}

	if err := s.store.PruneDailyGiven(ctx, bot.DayKey(now)); err != nil {
		s.log.Error("pruning daily usage", "err", err)
	}
}
