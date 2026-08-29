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

	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	client, err := mmclient.New(cfg.MattermostURL, cfg.MattermostToken, log)
	if err != nil {
		return err
	}

	msgs, err := bot.NewMessages(cfg.Language)
	if err != nil {
		return err
	}

	b, err := bot.New(client, store, log, msgs, bot.Limits{
		DailyTotal:     cfg.DailyTotalLimit,
		DailyPerTarget: cfg.DailyPerTargetLimit,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go scheduler.New(store, client, log, msgs).Run(ctx)

	log.Info("karmabot started", "language", cfg.Language)
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
