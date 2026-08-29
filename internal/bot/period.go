package bot

import "time"

// MSK is Moscow time: a fixed UTC+3 offset, no daylight saving.
var MSK = time.FixedZone("MSK", 3*60*60)

// resetHour is the hour (MSK) at which the weekly period rolls over.
const resetHour = 3

// DayKey returns the MSK calendar day of t as YYYY-MM-DD. Daily karma
// budgets refresh whenever this key changes.
func DayKey(t time.Time) string {
	return t.In(MSK).Format("2006-01-02")
}

// WeekKey returns the Monday date of the weekly karma period containing t.
// A period runs from Monday 03:00 MSK; a Monday before 03:00 still belongs
// to the previous week.
func WeekKey(t time.Time) string {
	return weekStart(t).Format("2006-01-02")
}

// PreviousWeekKey returns the week key of the period that ended most
// recently before t — the week whose summary the bot posts at the rollover.
func PreviousWeekKey(t time.Time) string {
	return WeekKey(t.AddDate(0, 0, -7))
}

// NextWeeklyReset returns the next Monday 03:00 MSK strictly after t.
func NextWeeklyReset(t time.Time) time.Time {
	local := t.In(MSK)
	daysFromMonday := (int(local.Weekday()) + 6) % 7
	daysToMonday := (7 - daysFromMonday) % 7

	next := time.Date(
		local.Year(),
		local.Month(),
		local.Day(),
		resetHour,
		0,
		0,
		0,
		MSK,
	).AddDate(0, 0, daysToMonday)
	if !next.After(t) {
		next = next.AddDate(0, 0, 7)
	}
	return next
}

func weekStart(t time.Time) time.Time {
	local := t.In(MSK)
	daysFromMonday := (int(local.Weekday()) + 6) % 7

	monday := time.Date(
		local.Year(),
		local.Month(),
		local.Day(),
		0,
		0,
		0,
		0,
		MSK,
	).AddDate(0, 0, -daysFromMonday)
	if local.Hour() < resetHour {
		monday = monday.AddDate(0, 0, -7)
	}
	return monday
}
