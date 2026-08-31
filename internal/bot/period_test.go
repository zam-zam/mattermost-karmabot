package bot

import (
	"testing"
	"time"
)

// Period arithmetic is pinned to 2026-08-24, a Monday exactly 966 days
// after the 2024-01-01 anchor.

func TestPeriodKeySevenDaysMatchesWeeklyKeys(t *testing.T) {
	weekly := Period{Days: 7}
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
			name: "sunday night belongs to the closing week",
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
			name: "non-utc zone is converted to utc",
			at:   time.Date(2026, 8, 31, 1, 0, 0, 0, time.FixedZone("+5", 5*60*60)),
			want: "2026-08-24",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := weekly.Key(tt.at); got != tt.want {
				t.Errorf("Key(%v) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}

func TestPeriodKeyThirtyDays(t *testing.T) {
	monthly := Period{Days: 30}
	// 2026-08-24 is day 966 after the anchor; 966/30 = 32 periods of 30
	// days puts the current period start 6 days earlier, on 2026-08-18,
	// and the next rollover 24 days later, on 2026-09-17.
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{
			name: "mid-period day",
			at:   time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
			want: "2026-08-18",
		},
		{
			name: "first day of the period",
			at:   time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC),
			want: "2026-08-18",
		},
		{
			name: "day before the period start belongs to the previous one",
			at:   time.Date(2026, 8, 17, 23, 59, 59, 0, time.UTC),
			want: "2026-07-19",
		},
		{
			name: "last day of the period",
			at:   time.Date(2026, 9, 16, 23, 0, 0, 0, time.UTC),
			want: "2026-08-18",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := monthly.Key(tt.at); got != tt.want {
				t.Errorf("Key(%v) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}

func TestPeriodKeyOneDayMatchesDayKey(t *testing.T) {
	daily := Period{Days: 1}
	at := time.Date(2026, 8, 26, 23, 30, 0, 0, time.UTC)
	if got, want := daily.Key(at), DayKey(at); got != want {
		t.Errorf("Key(%v) = %q, want DayKey %q", at, got, want)
	}
}

func TestPeriodNextReset(t *testing.T) {
	tests := []struct {
		name   string
		period Period
		at     time.Time
		want   time.Time
	}{
		{
			name:   "thirty-day period resets at the period end",
			period: Period{Days: 30},
			at:     time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
			want:   time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			// At the rollover instant itself the next reset is a full
			// cycle out.
			name:   "exact period start jumps a cycle",
			period: Period{Days: 30},
			at:     time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC),
			want:   time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "last second before rollover resets in one second",
			period: Period{Days: 7},
			at:     time.Date(2026, 8, 30, 23, 59, 59, 0, time.UTC),
			want:   time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.period.NextReset(tt.at)
			if !got.Equal(tt.want) {
				t.Errorf("NextReset(%v) = %v, want %v", tt.at, got, tt.want)
			}
		})
	}
}

func TestPeriodPreviousKey(t *testing.T) {
	// At a Monday rollover the finished week is the previous Monday.
	weekly := Period{Days: 7}
	at := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	if got := weekly.PreviousKey(at); got != "2026-08-24" {
		t.Errorf("PreviousKey(%v) = %q, want %q", at, got, "2026-08-24")
	}

	monthly := Period{Days: 30}
	if got := monthly.PreviousKey(at); got != "2026-07-19" {
		t.Errorf("PreviousKey(%v) = %q, want %q", at, got, "2026-07-19")
	}
}

func TestPeriodKeyAligned(t *testing.T) {
	tests := []struct {
		name   string
		period Period
		key    string
		want   bool
	}{
		{name: "weekly key under seven days", period: Period{Days: 7}, key: "2026-08-24", want: true},
		{name: "weekly key under thirty days", period: Period{Days: 30}, key: "2026-08-24", want: false},
		{name: "thirty-day key under thirty days", period: Period{Days: 30}, key: "2026-08-18", want: true},
		{name: "thirty-day key under seven days", period: Period{Days: 7}, key: "2026-08-18", want: false},
		{name: "not a date", period: Period{Days: 7}, key: "garbage", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.period.KeyAligned(tt.key); got != tt.want {
				t.Errorf("KeyAligned(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestPeriodValidate(t *testing.T) {
	for _, days := range []int{0, -7, 366} {
		if err := (Period{Days: days}).validate(); err == nil {
			t.Errorf("validate(%d) succeeded, want error", days)
		}
	}
	for _, days := range []int{1, 7, 30, 365} {
		if err := (Period{Days: days}).validate(); err != nil {
			t.Errorf("validate(%d): %v", days, err)
		}
	}
}

// BenchmarkPeriodKey documents the per-call cost of period math: closed-
// form arithmetic, independent of Days.
func BenchmarkPeriodKey(b *testing.B) {
	monthly := Period{Days: 30}
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

	b.ReportAllocs()
	for b.Loop() {
		_ = monthly.Key(now)
	}
}
