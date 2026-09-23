package bot

import (
	"strings"
	"testing"
	"time"
	// Embed the IANA database so zone tests pass on hosts without
	// /usr/share/zoneinfo.
	_ "time/tzdata"
)

// mustPeriod builds a Period, failing the test on bad inputs.
func mustPeriod(t *testing.T, kind, tz, rollover string) Period {
	t.Helper()
	p, err := NewPeriod(kind, tz, rollover)
	if err != nil {
		t.Fatalf("NewPeriod(%q, %q, %q): %v", kind, tz, rollover, err)
	}
	return p
}

func utc(t *testing.T, y int, m time.Month, d, hh, mm, ss int) time.Time {
	t.Helper()
	return time.Date(y, m, d, hh, mm, ss, 0, time.UTC)
}

// TestWeekKeyUTCPinsMondayBoundaries pins week keys around the Monday
// 09:00 UTC rollover. 2026-08-24 is a Monday.
func TestWeekKeyUTCPinsMondayBoundaries(t *testing.T) {
	p := mustPeriod(t, "week", "UTC", "09:00")

	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "midweek", at: utc(t, 2026, 8, 26, 12, 0, 0), want: "2026-08-24"},
		{name: "sunday night", at: utc(t, 2026, 8, 30, 23, 0, 0), want: "2026-08-24"},
		{name: "monday before rollover", at: utc(t, 2026, 8, 24, 8, 59, 0), want: "2026-08-17"},
		{name: "monday at rollover", at: utc(t, 2026, 8, 24, 9, 0, 0), want: "2026-08-24"},
		{name: "monday after rollover", at: utc(t, 2026, 8, 24, 9, 1, 0), want: "2026-08-24"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.Key(tt.at); got != tt.want {
				t.Errorf("Key(%s) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}

// TestWeekKeyFollowsConfiguredZone proves the week boundary is computed
// in the configured location, not in UTC: Monday 02:00 UTC is still
// Sunday 22:00 in New York, where the week has not rolled over yet.
func TestWeekKeyFollowsConfiguredZone(t *testing.T) {
	p := mustPeriod(t, "week", "America/New_York", "09:00")

	// Monday 2026-08-24 02:00 UTC is Sunday 2026-08-23 22:00 in New
	// York, so the week of August 17 is still running there; computing
	// in UTC would say August 24.
	at := utc(t, 2026, 8, 24, 2, 0, 0)
	if got := p.Key(at); got != "2026-08-17" {
		t.Errorf("Key(UTC instant that is still Sunday in New York) = %q, want 2026-08-17", got)
	}
}

// TestMonthKeyUTCPinsFirstDayBoundaries pins month keys around the 1st
// 09:00 UTC rollover, including December → January.
func TestMonthKeyUTCPinsFirstDayBoundaries(t *testing.T) {
	p := mustPeriod(t, "month", "UTC", "09:00")

	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "mid-month", at: utc(t, 2026, 9, 10, 12, 0, 0), want: "2026-09-01"},
		{name: "first day before rollover", at: utc(t, 2026, 9, 1, 8, 0, 0), want: "2026-08-01"},
		{name: "first day at rollover", at: utc(t, 2026, 9, 1, 9, 0, 0), want: "2026-09-01"},
		{name: "last day of month", at: utc(t, 2026, 9, 30, 23, 0, 0), want: "2026-09-01"},
		{name: "december into january", at: utc(t, 2026, 12, 31, 23, 0, 0), want: "2026-12-01"},
		{name: "new year day", at: utc(t, 2027, 1, 1, 10, 0, 0), want: "2027-01-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.Key(tt.at); got != tt.want {
				t.Errorf("Key(%s) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}

// TestStartReturnsRolloverInstant pins the exact instant a period begins.
func TestStartReturnsRolloverInstant(t *testing.T) {
	week := mustPeriod(t, "week", "UTC", "09:00")
	want := utc(t, 2026, 8, 24, 9, 0, 0)
	if got := week.Start(utc(t, 2026, 8, 26, 12, 0, 0)); !got.Equal(want) {
		t.Errorf("week Start = %s, want %s", got, want)
	}

	month := mustPeriod(t, "month", "UTC", "09:00")
	want = utc(t, 2026, 9, 1, 9, 0, 0)
	if got := month.Start(utc(t, 2026, 9, 10, 12, 0, 0)); !got.Equal(want) {
		t.Errorf("month Start = %s, want %s", got, want)
	}
}

// TestNextResetJumpsFromExactRollover pins that the next rollover is
// strictly after t, a full segment out at an exact rollover instant.
func TestNextResetJumpsFromExactRollover(t *testing.T) {
	week := mustPeriod(t, "week", "UTC", "09:00")
	tests := []struct {
		name string
		at   time.Time
		want time.Time
	}{
		{
			name: "last second of the week",
			at:   utc(t, 2026, 8, 30, 23, 59, 59),
			want: utc(t, 2026, 8, 31, 9, 0, 0),
		},
		{
			name: "exact rollover jumps a week",
			at:   utc(t, 2026, 8, 24, 9, 0, 0),
			want: utc(t, 2026, 8, 31, 9, 0, 0),
		},
		{
			name: "minute before rollover",
			at:   utc(t, 2026, 8, 24, 8, 59, 0),
			want: utc(t, 2026, 8, 24, 9, 0, 0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := week.NextReset(tt.at); !got.Equal(tt.want) {
				t.Errorf("NextReset(%s) = %s, want %s", tt.at, got, tt.want)
			}
		})
	}

	month := mustPeriod(t, "month", "UTC", "09:00")
	if got, want := month.NextReset(utc(t, 2026, 9, 30, 23, 0, 0)),
		utc(t, 2026, 10, 1, 9, 0, 0); !got.Equal(want) {
		t.Errorf("month NextReset = %s, want %s", got, want)
	}
	if got, want := month.NextReset(utc(t, 2026, 9, 1, 9, 0, 0)),
		utc(t, 2026, 10, 1, 9, 0, 0); !got.Equal(want) {
		t.Errorf("month NextReset from exact rollover = %s, want %s", got, want)
	}
}

// TestPreviousKeyCoversYearBoundary pins the key of the period whose
// results are posted at the rollover.
func TestPreviousKeyCoversYearBoundary(t *testing.T) {
	week := mustPeriod(t, "week", "UTC", "09:00")
	if got := week.PreviousKey(utc(t, 2026, 8, 26, 12, 0, 0)); got != "2026-08-17" {
		t.Errorf("week PreviousKey = %q, want 2026-08-17", got)
	}

	month := mustPeriod(t, "month", "UTC", "09:00")
	if got := month.PreviousKey(utc(t, 2026, 9, 10, 12, 0, 0)); got != "2026-08-01" {
		t.Errorf("month PreviousKey = %q, want 2026-08-01", got)
	}
	if got := month.PreviousKey(utc(t, 2027, 1, 5, 12, 0, 0)); got != "2026-12-01" {
		t.Errorf("month PreviousKey across new year = %q, want 2026-12-01", got)
	}
}

// TestEndReturnsLastDayOfPeriod pins the display-only end dates: Sunday
// for weeks, the last calendar day for months.
func TestEndReturnsLastDayOfPeriod(t *testing.T) {
	week := mustPeriod(t, "week", "UTC", "09:00")
	weekStart := utc(t, 2026, 8, 24, 9, 0, 0)
	if got := week.End(weekStart); got.In(time.UTC).Format("2006-01-02") != "2026-08-30" {
		t.Errorf("week End = %s, want 2026-08-30", got)
	}

	month := mustPeriod(t, "month", "UTC", "09:00")
	septStart := utc(t, 2026, 9, 1, 9, 0, 0)
	if got := month.End(septStart); got.In(time.UTC).Format("2006-01-02") != "2026-09-30" {
		t.Errorf("month End (September) = %s, want 2026-09-30", got)
	}
	febStart := utc(t, 2028, 2, 1, 9, 0, 0) // 2028 is a leap year
	if got := month.End(febStart); got.In(time.UTC).Format("2006-01-02") != "2028-02-29" {
		t.Errorf("month End (leap February) = %s, want 2028-02-29", got)
	}
}

// TestMoscowZoneUTCDrift pins that the same wall clock in Moscow maps to
// shifted UTC instants: the rollover is 09:00 local, 06:00 UTC.
func TestMoscowZoneUTCDrift(t *testing.T) {
	p := mustPeriod(t, "week", "Europe/Moscow", "09:00")

	// Monday 08:59 Moscow is Monday 05:59 UTC: still the previous week.
	at := utc(t, 2026, 8, 24, 5, 59, 0)
	if got := p.Key(at); got != "2026-08-17" {
		t.Errorf("Key before Moscow rollover = %q, want 2026-08-17", got)
	}
	// Monday 09:00 Moscow is Monday 06:00 UTC: the new week begins.
	at = utc(t, 2026, 8, 24, 6, 0, 0)
	if got := p.Key(at); got != "2026-08-24" {
		t.Errorf("Key at Moscow rollover = %q, want 2026-08-24", got)
	}
	if got, want := p.NextReset(at), utc(t, 2026, 8, 31, 6, 0, 0); !got.Equal(want) {
		t.Errorf("NextReset at Moscow rollover = %s, want %s", got, want)
	}
}

// TestBerlinDSTKeepsWallClock proves rollovers stay at 09:00 local time
// across the spring-forward and fall-back transitions (Europe/Berlin:
// March 29 and October 25, 2026).
func TestBerlinDSTKeepsWallClock(t *testing.T) {
	p := mustPeriod(t, "week", "Europe/Berlin", "09:00")

	// Spring forward: the week of March 23 starts at CET 09:00, the next
	// rollover is Monday March 30 at CEST 09:00 — 07:00 UTC.
	got := p.NextReset(utc(t, 2026, 3, 25, 12, 0, 0))
	if want := utc(t, 2026, 3, 30, 7, 0, 0); !got.Equal(want) {
		t.Errorf("NextReset across spring forward = %s, want %s", got, want)
	}

	// Fall back: the next rollover after October 21 is Monday October 26
	// at CET 09:00 — 08:00 UTC.
	got = p.NextReset(utc(t, 2026, 10, 21, 12, 0, 0))
	if want := utc(t, 2026, 10, 26, 8, 0, 0); !got.Equal(want) {
		t.Errorf("NextReset across fall back = %s, want %s", got, want)
	}

	month := mustPeriod(t, "month", "Europe/Berlin", "09:00")
	novStart := month.Start(utc(t, 2026, 11, 3, 12, 0, 0))
	if want := utc(t, 2026, 11, 1, 8, 0, 0); !novStart.Equal(want) {
		t.Errorf("November Start after fall back = %s, want %s", novStart, want)
	}
}

// TestDayKeyFollowsZone pins that daily budget days flip at local
// midnight in the configured zone.
func TestDayKeyFollowsZone(t *testing.T) {
	msk := mustPeriod(t, "week", "Europe/Moscow", "09:00")
	// 2026-08-26 21:00 UTC is already August 27 in Moscow.
	at := utc(t, 2026, 8, 26, 21, 0, 0)
	if got := msk.DayKey(at); got != "2026-08-27" {
		t.Errorf("Moscow DayKey = %q, want 2026-08-27", got)
	}

	utcP := mustPeriod(t, "week", "UTC", "09:00")
	if got := utcP.DayKey(at); got != "2026-08-26" {
		t.Errorf("UTC DayKey = %q, want 2026-08-26", got)
	}
}

func TestKeyAligned(t *testing.T) {
	week := mustPeriod(t, "week", "UTC", "09:00")
	month := mustPeriod(t, "month", "UTC", "09:00")

	tests := []struct {
		name  string
		p     Period
		key   string
		align bool
	}{
		{name: "week monday key", p: week, key: "2026-08-24", align: true},
		{name: "week non-monday key", p: week, key: "2026-08-25", align: false},
		{name: "week first of month is not a monday key", p: week, key: "2026-09-01", align: false},
		{name: "month first day key", p: month, key: "2026-09-01", align: true},
		{name: "month mid-month key", p: month, key: "2026-08-24", align: false},
		{name: "garbage key", p: week, key: "week-32", align: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.KeyAligned(tt.key); got != tt.align {
				t.Errorf("KeyAligned(%q) = %v, want %v", tt.key, got, tt.align)
			}
		})
	}
}

func TestNewPeriod(t *testing.T) {
	if _, err := NewPeriod("week", "UTC", "09:00"); err != nil {
		t.Errorf("NewPeriod(week): %v", err)
	}
	if _, err := NewPeriod("month", "Europe/Moscow", "00:00"); err != nil {
		t.Errorf("NewPeriod(month): %v", err)
	}

	p, err := NewPeriod("week", "UTC", "09:05")
	if err != nil {
		t.Fatalf("NewPeriod: %v", err)
	}
	if p.Rollover() != "09:05" {
		t.Errorf("Rollover() = %q, want 09:05", p.Rollover())
	}
	if p.Loc.String() != "UTC" {
		t.Errorf("Loc = %q, want UTC", p.Loc.String())
	}
}

func TestNewPeriodRejectsBadValues(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		tz        string
		rollover  string
		wantInErr string
	}{
		{name: "unknown kind", kind: "daily", tz: "UTC", rollover: "09:00", wantInErr: "KARMABOT_PERIOD"},
		{name: "empty kind", kind: "", tz: "UTC", rollover: "09:00", wantInErr: "KARMABOT_PERIOD"},
		{name: "unknown zone", kind: "week", tz: "Mars/Olympus", rollover: "09:00", wantInErr: "KARMABOT_TIMEZONE"},
		{name: "hour out of range", kind: "week", tz: "UTC", rollover: "25:00", wantInErr: "KARMABOT_ROLLOVER_TIME"},
		{name: "not a time", kind: "week", tz: "UTC", rollover: "morning", wantInErr: "KARMABOT_ROLLOVER_TIME"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPeriod(tt.kind, tt.tz, tt.rollover)
			if err == nil {
				t.Fatal("NewPeriod succeeded with bad values, want error")
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantInErr)
			}
		})
	}
}

func TestPeriodValidate(t *testing.T) {
	valid := mustPeriod(t, "week", "UTC", "09:00")
	if err := valid.validate(); err != nil {
		t.Errorf("validate(valid) = %v, want nil", err)
	}

	tests := []struct {
		name   string
		period Period
	}{
		{name: "unknown kind", period: Period{Kind: "daily", Loc: time.UTC}},
		{name: "nil location", period: Period{Kind: KindWeek}},
		{name: "rollover hour out of range", period: Period{Kind: KindWeek, Loc: time.UTC, RolloverHour: 24}},
		{name: "rollover minute out of range", period: Period{Kind: KindMonth, Loc: time.UTC, RolloverMinute: 60}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.period.validate(); err == nil {
				t.Error("validate succeeded on an invalid period, want error")
			}
		})
	}
}
