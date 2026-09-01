// Package config loads karmabot settings from the environment and .env file.
package config

import (
	"fmt"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config holds all karmabot settings. Variables are prefixed with KARMABOT_
// (e.g. KARMABOT_MATTERMOST_URL). Values from the real environment take
// precedence over the .env file.
type Config struct {
	MattermostURL       string `envconfig:"MATTERMOST_URL" required:"true"`
	MattermostToken     string `envconfig:"MATTERMOST_TOKEN" required:"true"`
	DBDriver            string `envconfig:"DB_DRIVER" default:"sqlite"`
	DBPath              string `envconfig:"DB_PATH" default:"./data/karmabot.db"`
	DBDSN               string `envconfig:"DB_DSN"`
	LogLevel            string `envconfig:"LOG_LEVEL" default:"info"`
	Language            string `envconfig:"LANGUAGE" default:"en"`
	DailyTotalLimit     int    `envconfig:"DAILY_TOTAL_LIMIT" default:"5"`
	DailyPerTargetLimit int    `envconfig:"DAILY_PER_TARGET_LIMIT" default:"2"`
	PeriodDays          int    `envconfig:"PERIOD_DAYS" default:"7"`
}

// Load reads the optional .env file and then processes environment variables.
func Load() (Config, error) {
	_ = godotenv.Load()

	var cfg Config
	if err := envconfig.Process("karmabot", &cfg); err != nil {
		return Config{}, fmt.Errorf("processing environment config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// validate checks the settings envconfig can't express: negative limits
// or a per-target limit that could never bind.
func (c Config) validate() error {
	switch c.DBDriver {
	case "sqlite", "postgres":
	case "":
		return fmt.Errorf("KARMABOT_DB_DRIVER must be sqlite or postgres, got empty")
	default:
		return fmt.Errorf("KARMABOT_DB_DRIVER must be sqlite or postgres, got %q", c.DBDriver)
	}
	if c.DBDriver == "postgres" && c.DBDSN == "" {
		return fmt.Errorf("KARMABOT_DB_DSN is required when KARMABOT_DB_DRIVER is postgres")
	}
	if c.DailyTotalLimit < 0 {
		return fmt.Errorf(
			"KARMABOT_DAILY_TOTAL_LIMIT must be 0 (unlimited) or a positive number, got %d",
			c.DailyTotalLimit,
		)
	}
	if c.DailyPerTargetLimit < 0 {
		return fmt.Errorf(
			"KARMABOT_DAILY_PER_TARGET_LIMIT must be 0 (unlimited) or a positive number, got %d",
			c.DailyPerTargetLimit,
		)
	}
	if c.DailyTotalLimit > 0 && c.DailyPerTargetLimit > c.DailyTotalLimit {
		return fmt.Errorf(
			"KARMABOT_DAILY_PER_TARGET_LIMIT (%d) must not exceed KARMABOT_DAILY_TOTAL_LIMIT (%d)",
			c.DailyPerTargetLimit,
			c.DailyTotalLimit,
		)
	}
	if c.PeriodDays < 1 || c.PeriodDays > 365 {
		return fmt.Errorf(
			"KARMABOT_PERIOD_DAYS must be between 1 and 365, got %d",
			c.PeriodDays,
		)
	}
	return nil
}
