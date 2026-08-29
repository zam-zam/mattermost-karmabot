package bot

import "time"

// DayKey returns the UTC calendar day of t as YYYY-MM-DD. Daily karma
// budgets refresh whenever this key changes.
func DayKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// WeekKey returns the Monday date of the weekly karma period containing t.
// A period runs from Monday 00:00 UTC.
func WeekKey(t time.Time) string {
	return weekStart(t).Format("2006-01-02")
}

// PreviousWeekKey returns the week key of the period that ended most
// recently before t — the week whose summary the bot posts at the rollover.
func PreviousWeekKey(t time.Time) string {
	return WeekKey(t.AddDate(0, 0, -7))
}

// NextWeeklyReset returns the next Monday 00:00 UTC strictly after t.
func NextWeeklyReset(t time.Time) time.Time {
	next := weekStart(t)
	if !next.After(t) {
		next = next.AddDate(0, 0, 7)
	}
	return next
}

// weekStart returns the Monday 00:00 UTC beginning the weekly period
// containing t.
func weekStart(t time.Time) time.Time {
	utc := t.UTC()
	daysFromMonday := (int(utc.Weekday()) + 6) % 7

	monday := time.Date(
		utc.Year(),
		utc.Month(),
		utc.Day(),
		0,
		0,
		0,
		0,
		time.UTC,
	)
	return monday.AddDate(0, 0, -daysFromMonday)
}
