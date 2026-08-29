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
	MattermostURL   string `envconfig:"MATTERMOST_URL" required:"true"`
	MattermostToken string `envconfig:"MATTERMOST_TOKEN" required:"true"`
	DBPath          string `envconfig:"DB_PATH" default:"./data/karmabot.db"`
	LogLevel        string `envconfig:"LOG_LEVEL" default:"info"`
}

// Load reads the optional .env file and then processes environment variables.
func Load() (Config, error) {
	_ = godotenv.Load()

	var cfg Config
	if err := envconfig.Process("karmabot", &cfg); err != nil {
		return Config{}, fmt.Errorf("processing environment config: %w", err)
	}
	return cfg, nil
}
