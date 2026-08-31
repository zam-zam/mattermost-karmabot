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
	if cfg.PeriodDays != 30 {
		t.Errorf("PeriodDays = %d, want 30", cfg.PeriodDays)
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

func TestLoadRejectsBadPeriod(t *testing.T) {
	for _, value := range []string{"0", "366", "many"} {
		t.Run(value, func(t *testing.T) {
			setBaseEnv(t)
			t.Setenv("KARMABOT_PERIOD_DAYS", value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load succeeded with a bad period, want error")
			}
			if !strings.Contains(err.Error(), "KARMABOT_PERIOD_DAYS") {
				t.Errorf("error %q does not mention KARMABOT_PERIOD_DAYS", err)
			}
		})
	}
}
