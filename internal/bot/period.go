package bot

import (
	"fmt"
	"time"
)

// periodAnchor is a Monday at 00:00 UTC. Periods are counted from it in
// whole-day multiples, so with Days=7 the keys match the historical
// Monday-based week keys exactly.
var periodAnchor = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// Period is the karma scoreboard cycle: it starts every Days days at
// 00:00 UTC, counted from a fixed anchor. Changing Days starts a fresh
// scoreboard; rows stored under the old segmentation are kept but no
// longer read.
type Period struct {
	Days int
}

// validate rejects periods outside the supported range.
func (p Period) validate() error {
	if p.Days < 1 || p.Days > 365 {
		return fmt.Errorf("period must be between 1 and 365 days, got %d", p.Days)
	}
	return nil
}

// DayKey returns the UTC calendar day of t as YYYY-MM-DD. Daily karma
// budgets refresh whenever this key changes.
func DayKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// Key returns the start date of the period containing t as YYYY-MM-DD.
func (p Period) Key(t time.Time) string {
	return p.start(t).Format("2006-01-02")
}

// PreviousKey returns the key of the period that ended most recently
// before t — the one whose summary the bot posts at the rollover.
func (p Period) PreviousKey(t time.Time) string {
	return p.start(t).AddDate(0, 0, -p.Days).Format("2006-01-02")
}

// NextReset returns the next period start strictly after t. At an exact
// period start the next one is a full cycle out.
func (p Period) NextReset(t time.Time) time.Time {
	next := p.start(t)
	if !next.After(t) {
		next = next.AddDate(0, 0, p.Days)
	}
	return next
}

// KeyAligned reports whether a stored key belongs to this period's
// segmentation; misaligned keys were written under a different Days.
func (p Period) KeyAligned(key string) bool {
	date, err := time.Parse("2006-01-02", key)
	if err != nil {
		return false
	}
	elapsed := int(date.Sub(periodAnchor).Hours() / 24)
	return elapsed%p.Days == 0
}

// start returns the period-start instant containing t, computed in
// closed form: one subtraction, one division — no loops, and the cost
// does not depend on Days or the distance from the anchor.
func (p Period) start(t time.Time) time.Time {
	utc := t.UTC()
	dayStart := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	elapsed := int(dayStart.Sub(periodAnchor).Hours() / 24)
	return periodAnchor.AddDate(0, 0, (elapsed/p.Days)*p.Days)
}
