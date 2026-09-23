// Command karmabot runs a karma bot for a Mattermost server: teammates
// thank each other with @karmabot ++ and the bot tracks weekly per-channel
// scores with daily limits.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	// Embed the IANA timezone database so KARMABOT_TIMEZONE works in
	// containers whose base image ships no /usr/share/zoneinfo.
	_ "time/tzdata"

	"karmabot/internal/bot"
	"karmabot/internal/config"
	"karmabot/internal/mmclient"
	"karmabot/internal/scheduler"
	"karmabot/internal/storage"
)

func main() {
	if err := run(); err != nil {
		slog.Error("karmabot exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)

	store, err := storage.Open(storage.Options{
		Driver: cfg.DBDriver,
		Path:   cfg.DBPath,
		DSN:    cfg.DBDSN,
	})
	if err != nil {
		return err
	}
	defer store.Close()

	period, err := bot.NewPeriod(cfg.Period, cfg.Timezone, cfg.RolloverTime)
	if err != nil {
		return err
	}
	if _, ok := os.LookupEnv("KARMABOT_PERIOD_DAYS"); ok {
		log.Warn("KARMABOT_PERIOD_DAYS is no longer supported; configure KARMABOT_PERIOD=week|month instead")
	}
	warnForeignKarma(store, period, log)

	client, err := mmclient.New(cfg.MattermostURL, cfg.MattermostToken, log)
	if err != nil {
		return err
	}

	msgs, err := bot.NewMessages(cfg.Language)
	if err != nil {
		return err
	}

	b, err := bot.New(client, store, log, bot.Options{
		Messages: msgs,
		Limits: bot.Limits{
			DailyTotal:     cfg.DailyTotalLimit,
			DailyPerTarget: cfg.DailyPerTargetLimit,
		},
		Period:        period,
		AdminUsername: cfg.AdminUsername,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go scheduler.New(store, client, log, msgs, period).Run(ctx)

	log.Info("karmabot started",
		"language", cfg.Language,
		"period_kind", period.Kind,
		"timezone", period.Loc.String(),
		"rollover", period.Rollover(),
	)
	events := client.Events(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Info("karmabot stopped")
			return nil
		case event := <-events:
			b.HandleEvent(ctx, event)
		}
	}
}

func newLogger(level string) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}

	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

// warnForeignKarma logs karma rows whose keys don't belong to the current
// period segmentation, e.g. after changing KARMABOT_PERIOD on an
// existing database. Such rows are ignored but kept in storage.
func warnForeignKarma(store *storage.Store, period bot.Period, log *slog.Logger) {
	counts, err := store.KarmaWeekCounts(context.Background())
	if err != nil {
		log.Error("counting stored karma periods", "err", err)
		return
	}

	rows, periods := 0, 0
	for week, count := range counts {
		if !period.KeyAligned(week) {
			rows += count
			periods++
		}
	}
	if rows > 0 {
		log.Warn("karma rows from a different period segmentation are ignored (kept in database)",
			"rows", rows,
			"periods", periods,
			"period_kind", period.Kind,
		)
	}
}
