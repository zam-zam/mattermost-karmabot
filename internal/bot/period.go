package bot

import (
	"fmt"
	"time"
)

// PeriodKind selects the scoreboard segmentation: calendar weeks or
// calendar months.
type PeriodKind string

const (
	// KindWeek segments the scoreboard by calendar weeks starting Monday.
	KindWeek PeriodKind = "week"
	// KindMonth segments the scoreboard by calendar months starting on
	// the 1st.
	KindMonth PeriodKind = "month"
)

// Period is the karma scoreboard cycle: calendar weeks or months in a
// fixed location. A period runs from one rollover instant to the next:
// the same wall-clock time (RolloverHour:RolloverMinute in Loc) on the
// segment's first day — Monday for weeks, the 1st for months. Keys are
// the segment's start date, so switching the segmentation starts a fresh
// scoreboard; rows stored under the old segmentation are kept but no
// longer read.
type Period struct {
	Kind           PeriodKind
	Loc            *time.Location
	RolloverHour   int
	RolloverMinute int
}

// NewPeriod builds a Period from the KARMABOT_PERIOD, KARMABOT_TIMEZONE
// and KARMABOT_ROLLOVER_TIME settings, naming the variables in errors.
func NewPeriod(kind, timezone, rollover string) (Period, error) {
	switch PeriodKind(kind) {
	case KindWeek, KindMonth:
	default:
		return Period{}, fmt.Errorf(
			"KARMABOT_PERIOD must be %q or %q, got %q",
			KindWeek, KindMonth, kind,
		)
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return Period{}, fmt.Errorf(
			"KARMABOT_TIMEZONE must be an IANA zone name such as Europe/Moscow: %w",
			err,
		)
	}

	clock, err := time.Parse("15:04", rollover)
	if err != nil {
		return Period{}, fmt.Errorf(
			"KARMABOT_ROLLOVER_TIME must be a 24-hour HH:MM time: %w",
			err,
		)
	}

	return Period{
		Kind:           PeriodKind(kind),
		Loc:            loc,
		RolloverHour:   clock.Hour(),
		RolloverMinute: clock.Minute(),
	}, nil
}

// validate rejects periods that could not have come from NewPeriod.
func (p Period) validate() error {
	switch p.Kind {
	case KindWeek, KindMonth:
	default:
		return fmt.Errorf("period kind must be %q or %q, got %q",
			KindWeek, KindMonth, p.Kind)
	}
	if p.Loc == nil {
		return fmt.Errorf("period location must not be nil")
	}
	if p.RolloverHour < 0 || p.RolloverHour > 23 ||
		p.RolloverMinute < 0 || p.RolloverMinute > 59 {
		return fmt.Errorf("period rollover time out of range: %s", p.Rollover())
	}
	return nil
}

// Rollover returns the wall-clock rollover time as HH:MM.
func (p Period) Rollover() string {
	return fmt.Sprintf("%02d:%02d", p.RolloverHour, p.RolloverMinute)
}

// kindID picks weekID for weeks and monthID for months; message catalogs
// key period texts by kind.
func (p Period) kindID(weekID, monthID string) string {
	if p.Kind == KindMonth {
		return monthID
	}
	return weekID
}

// Key returns the start date of the period containing t as YYYY-MM-DD.
func (p Period) Key(t time.Time) string {
	return p.Start(t).In(p.Loc).Format("2006-01-02")
}

// DayKey returns the calendar day of t in the period's location as
// YYYY-MM-DD. Daily karma budgets refresh whenever this key changes.
func (p Period) DayKey(t time.Time) string {
	return t.In(p.Loc).Format("2006-01-02")
}

// Start returns the rollover instant at which the period containing t
// began.
func (p Period) Start(t time.Time) time.Time {
	start := p.segmentStart(t)
	if start.After(t) {
		start = p.shiftSegment(start, -1)
	}
	return start
}

// PreviousStart returns the start of the period that ended most recently
// before t — the one whose results the bot posts at the rollover.
func (p Period) PreviousStart(t time.Time) time.Time {
	return p.shiftSegment(p.Start(t), -1)
}

// PreviousKey returns the key of the period that ended most recently
// before t.
func (p Period) PreviousKey(t time.Time) string {
	return p.PreviousStart(t).In(p.Loc).Format("2006-01-02")
}

// NextReset returns the next rollover strictly after t. At an exact
// rollover instant the next one is a full segment out.
func (p Period) NextReset(t time.Time) time.Time {
	return p.shiftSegment(p.Start(t), 1)
}

// KeyAligned reports whether a stored key belongs to this period's
// segmentation; misaligned keys were written under a different one.
func (p Period) KeyAligned(key string) bool {
	date, err := time.Parse("2006-01-02", key)
	if err != nil {
		return false
	}
	if p.Kind == KindMonth {
		return date.Day() == 1
	}
	return date.Weekday() == time.Monday
}

// End returns the last calendar day of the period that starts at start;
// used for display only. Noon keeps the date stable across DST shifts.
func (p Period) End(start time.Time) time.Time {
	next := p.shiftSegment(start, 1)
	local := next.In(p.Loc)
	return time.Date(local.Year(), local.Month(), local.Day()-1, 12, 0, 0, 0, p.Loc)
}

// segmentStart returns the candidate segment start for t's own segment,
// ignoring whether the rollover time has already passed that day.
func (p Period) segmentStart(t time.Time) time.Time {
	local := t.In(p.Loc)
	day := 1
	if p.Kind == KindWeek {
		day = local.Day() - (int(local.Weekday())+6)%7 // back to Monday
	}
	return p.atRollover(local.Year(), local.Month(), day)
}

// shiftSegment moves a segment start by whole segments, rebuilding the
// wall-clock rollover time so DST transitions never shift it.
func (p Period) shiftSegment(start time.Time, segments int) time.Time {
	local := start.In(p.Loc)
	if p.Kind == KindWeek {
		return p.atRollover(local.Year(), local.Month(), local.Day()+7*segments)
	}
	return p.atRollover(local.Year(), local.Month()+time.Month(segments), 1)
}

// atRollover builds the rollover instant on the given calendar date;
// time.Date normalizes out-of-range values and DST edge cases.
func (p Period) atRollover(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, p.RolloverHour, p.RolloverMinute, 0, 0, p.Loc)
}
