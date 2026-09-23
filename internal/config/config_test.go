package config

import (
	"strings"
	"testing"
)

// setBaseEnv provides the required vars so optional settings can be tested.
func setBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("KARMABOT_MATTERMOST_URL", "https://mattermost.example.com")
	t.Setenv("KARMABOT_MATTERMOST_TOKEN", "token")
}

func TestLoadDefaults(t *testing.T) {
	setBaseEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Language != "en" {
		t.Errorf("Language = %q, want %q", cfg.Language, "en")
	}
	if cfg.DailyTotalLimit != 5 {
		t.Errorf("DailyTotalLimit = %d, want 5", cfg.DailyTotalLimit)
	}
	if cfg.DailyPerTargetLimit != 2 {
		t.Errorf("DailyPerTargetLimit = %d, want 2", cfg.DailyPerTargetLimit)
	}
	if cfg.Period != "week" {
		t.Errorf("Period = %q, want %q", cfg.Period, "week")
	}
	if cfg.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want %q", cfg.Timezone, "UTC")
	}
	if cfg.RolloverTime != "09:00" {
		t.Errorf("RolloverTime = %q, want %q", cfg.RolloverTime, "09:00")
	}
	if cfg.DBDriver != "sqlite" {
		t.Errorf("DBDriver = %q, want %q", cfg.DBDriver, "sqlite")
	}
	if cfg.AdminUsername != "" {
		t.Errorf("AdminUsername = %q, want empty default", cfg.AdminUsername)
	}
}

func TestLoadAdminUsername(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("KARMABOT_ADMIN_USERNAME", "boss")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AdminUsername != "boss" {
		t.Errorf("AdminUsername = %q, want boss", cfg.AdminUsername)
	}
}

func TestLoadPostgresDriver(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("KARMABOT_DB_DRIVER", "postgres")
	t.Setenv("KARMABOT_DB_DSN", "postgres://user:pass@localhost:5432/karmabot")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBDriver != "postgres" || cfg.DBDSN == "" {
		t.Fatalf("DBDriver=%q DBDSN=%q, want postgres with a DSN", cfg.DBDriver, cfg.DBDSN)
	}
}

func TestLoadRejectsBadDBDriver(t *testing.T) {
	tests := []struct {
		name      string
		driver    string
		dsn       string
		wantInErr string
	}{
		{
			name:      "unknown driver",
			driver:    "mysql",
			wantInErr: "KARMABOT_DB_DRIVER",
		},
		{
			name:      "postgres without dsn",
			driver:    "postgres",
			wantInErr: "KARMABOT_DB_DSN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setBaseEnv(t)
			t.Setenv("KARMABOT_DB_DRIVER", tt.driver)
			if tt.dsn != "" {
				t.Setenv("KARMABOT_DB_DSN", tt.dsn)
			}

			_, err := Load()
			if err == nil {
				t.Fatal("Load succeeded with a bad database config, want error")
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantInErr)
			}
		})
	}
}

// TestLoadExplicitZeroMeansUnlimited pins that 0 is a valid, explicit
// choice distinct from the 5/2 defaults.
func TestLoadExplicitZeroMeansUnlimited(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("KARMABOT_DAILY_TOTAL_LIMIT", "0")
	t.Setenv("KARMABOT_DAILY_PER_TARGET_LIMIT", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DailyTotalLimit != 0 {
		t.Errorf("DailyTotalLimit = %d, want 0 (unlimited)", cfg.DailyTotalLimit)
	}
	if cfg.DailyPerTargetLimit != 0 {
		t.Errorf("DailyPerTargetLimit = %d, want 0 (unlimited)", cfg.DailyPerTargetLimit)
	}
}

func TestLoadCustomLimits(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("KARMABOT_DAILY_TOTAL_LIMIT", "7")
	t.Setenv("KARMABOT_DAILY_PER_TARGET_LIMIT", "3")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DailyTotalLimit != 7 {
		t.Errorf("DailyTotalLimit = %d, want 7", cfg.DailyTotalLimit)
	}
	if cfg.DailyPerTargetLimit != 3 {
		t.Errorf("DailyPerTargetLimit = %d, want 3", cfg.DailyPerTargetLimit)
	}
}

func TestLoadRejectsBadLimits(t *testing.T) {
	tests := []struct {
		name      string
		total     string
		perTarget string
		wantInErr string
	}{
		{
			name:      "negative total",
			total:     "-1",
			perTarget: "1",
			wantInErr: "KARMABOT_DAILY_TOTAL_LIMIT",
		},
		{
			name:      "negative per-target",
			total:     "5",
			perTarget: "-1",
			wantInErr: "KARMABOT_DAILY_PER_TARGET_LIMIT",
		},
		{
			name:      "per-target above total",
			total:     "3",
			perTarget: "5",
			wantInErr: "must not exceed",
		},
		{
			name:      "non-numeric total",
			total:     "many",
			perTarget: "2",
			wantInErr: "DAILY_TOTAL_LIMIT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setBaseEnv(t)
			t.Setenv("KARMABOT_DAILY_TOTAL_LIMIT", tt.total)
			t.Setenv("KARMABOT_DAILY_PER_TARGET_LIMIT", tt.perTarget)

			_, err := Load()
			if err == nil {
				t.Fatal("Load succeeded with bad limits, want error")
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantInErr)
			}
		})
	}
}

// TestLoadPeriodSettingsPassThrough pins that the period settings are
// carried through verbatim; their values are parsed and rejected by
// bot.NewPeriod.
func TestLoadPeriodSettingsPassThrough(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("KARMABOT_PERIOD", "month")
	t.Setenv("KARMABOT_TIMEZONE", "Europe/Moscow")
	t.Setenv("KARMABOT_ROLLOVER_TIME", "10:30")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Period != "month" {
		t.Errorf("Period = %q, want month", cfg.Period)
	}
	if cfg.Timezone != "Europe/Moscow" {
		t.Errorf("Timezone = %q, want Europe/Moscow", cfg.Timezone)
	}
	if cfg.RolloverTime != "10:30" {
		t.Errorf("RolloverTime = %q, want 10:30", cfg.RolloverTime)
	}
}
