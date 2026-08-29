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
			// The last second of Sunday still belongs to the closing week.
			name: "sunday night belongs to the week ending at monday midnight",
			at:   time.Date(2026, 8, 30, 23, 59, 59, 0, time.UTC),
			want: "2026-08-24",
		},
		{
			// Monday 00:00 UTC is exactly the rollover instant.
			name: "monday midnight starts the new week",
			at:   time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC),
			want: "2026-08-24",
		},
		{
			name: "just after monday midnight keeps the new week",
			at:   time.Date(2026, 8, 24, 0, 0, 1, 0, time.UTC),
			want: "2026-08-24",
		},
		{
			// 01:00 at UTC+5 is Sunday 20:00 UTC: still the closing week.
			name: "non-utc zone is converted to utc before dating the week",
			at:   time.Date(2026, 8, 31, 1, 0, 0, 0, time.FixedZone("+5", 5*60*60)),
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
			name: "utc midnight starts the day",
			at:   time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC),
			want: "2026-08-28",
		},
		{
			name: "utc 23:30 stays on the same utc day",
			at:   time.Date(2026, 8, 28, 23, 30, 0, 0, time.UTC),
			want: "2026-08-28",
		},
		{
			// 01:30 at UTC+3 is 22:30 UTC on the previous calendar date.
			name: "non-utc zone is converted to utc",
			at:   time.Date(2026, 8, 29, 1, 30, 0, 0, time.FixedZone("+3", 3*60*60)),
			want: "2026-08-28",
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
			name: "sunday evening resets at monday midnight",
			at:   time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC), // Sunday
			want: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			// The last second of Sunday still aims at the imminent midnight.
			name: "sunday 23:59:59 resets in one second",
			at:   time.Date(2026, 8, 30, 23, 59, 59, 0, time.UTC),
			want: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			// At the rollover instant itself the next reset is a week out.
			name: "monday midnight jumps a week",
			at:   time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC),
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
	// At the Monday 00:00 UTC rollover itself the finished week is the
	// previous Monday.
	at := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	if got := PreviousWeekKey(at); got != "2026-08-24" {
		t.Errorf("PreviousWeekKey(%v) = %q, want %q", at, got, "2026-08-24")
	}
}
