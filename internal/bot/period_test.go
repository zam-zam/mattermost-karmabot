package bot

import (
	"testing"
	"time"
)

func TestWeekKey(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{
			name: "midweek afternoon belongs to that week's monday",
			at:   time.Date(2026, 8, 26, 14, 30, 0, 0, time.UTC), // Wednesday
			want: "2026-08-24",
		},
		{
			// 23:59:59 UTC is Monday 02:59:59 MSK, before the reset hour.
			name: "monday before reset hour belongs to previous week",
			at:   time.Date(2026, 8, 23, 23, 59, 59, 0, time.UTC),
			want: "2026-08-17",
		},
		{
			// 00:00 UTC is Monday 03:00 MSK, exactly the reset hour.
			name: "monday at reset hour starts the new week",
			at:   time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC),
			want: "2026-08-24",
		},
		{
			// Sunday 23:00 UTC is Monday 02:00 MSK, still the old week.
			name: "sunday night belongs to the week ending at monday 03:00",
			at:   time.Date(2026, 8, 30, 23, 0, 0, 0, time.UTC),
			want: "2026-08-24",
		},
		{
			name: "instant already in msk keeps its week",
			at:   time.Date(2026, 8, 28, 12, 0, 0, 0, MSK),
			want: "2026-08-24",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WeekKey(tt.at); got != tt.want {
				t.Errorf("WeekKey(%v) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}

func TestDayKey(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{
			name: "utc midnight is 03:00 msk same day",
			at:   time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC),
			want: "2026-08-28",
		},
		{
			name: "utc 23:30 is next msk day",
			at:   time.Date(2026, 8, 28, 23, 30, 0, 0, time.UTC),
			want: "2026-08-29",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DayKey(tt.at); got != tt.want {
				t.Errorf("DayKey(%v) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}

func TestNextWeeklyReset(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		want time.Time
	}{
		{
			name: "sunday before midnight resets next morning",
			at:   time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC), // Sunday 23:00 MSK
			want: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),  // Monday 03:00 MSK
		},
		{
			// Sunday 23:00 UTC is Monday 02:00 MSK, before the reset hour.
			name: "monday before reset is today",
			at:   time.Date(2026, 8, 23, 23, 0, 0, 0, time.UTC),
			want: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), // 03:00 MSK
		},
		{
			// 03:00:01 UTC is 06:00:01 MSK, just past this week's reset.
			name: "monday at reset jumps a week",
			at:   time.Date(2026, 8, 24, 3, 0, 1, 0, time.UTC),
			want: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "midweek wednesday resets the coming monday",
			at:   time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC), // Wednesday
			want: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextWeeklyReset(tt.at)
			if !got.Equal(tt.want) {
				t.Errorf("NextWeeklyReset(%v) = %v, want %v", tt.at, got, tt.want)
			}
		})
	}
}

func TestPreviousWeekKey(t *testing.T) {
	// At the reset moment itself the finished week is the previous Monday.
	at := time.Date(2026, 8, 31, 3, 0, 0, 0, time.UTC) // Monday 06:00 MSK
	if got := PreviousWeekKey(at); got != "2026-08-24" {
		t.Errorf("PreviousWeekKey(%v) = %q, want %q", at, got, "2026-08-24")
	}
}
