// Package storage persists karmabot state in a single SQLite database.
// Karma and daily budgets are scoped by channel, so every channel's stats
// are isolated even though they share one file.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Channel identifies a Mattermost channel with karma tracking enabled.
type Channel struct {
	ID     string
	TeamID string
	Name   string
}

// KarmaEntry is one user's score within a channel for one week.
type KarmaEntry struct {
	UserID   string
	Username string
	Karma    int
}

// ChannelKarma is one user's current score in one channel.
type ChannelKarma struct {
	ChannelName string
	Karma       int
}

// Grant records one karma point for the target and its cost against the
// giver's daily budget.
type Grant struct {
	ChannelID      string
	Week           string
	Day            string
	GiverID        string
	TargetID       string
	TargetUsername string
}

// Store wraps the SQLite database holding channels, weekly karma and
// daily budget usage.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS channels (
	channel_id TEXT PRIMARY KEY,
	team_id    TEXT NOT NULL,
	name       TEXT NOT NULL,
	enabled    INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS karma (
	channel_id TEXT NOT NULL,
	user_id    TEXT NOT NULL,
	username   TEXT NOT NULL,
	week       TEXT NOT NULL,
	karma      INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (channel_id, user_id, week)
);

CREATE TABLE IF NOT EXISTS daily_given (
	channel_id TEXT NOT NULL,
	from_user  TEXT NOT NULL,
	to_user    TEXT NOT NULL,
	day        TEXT NOT NULL,
	amount     INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (channel_id, from_user, to_user, day)
);
`

// Open creates or opens the database at path, applying the schema.
func Open(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data directory %s: %w", dir, err)
	}

	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}
	// SQLite allows one writer; a single connection also rules out
	// SQLITE_BUSY between the event loop and the scheduler goroutine.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// EnableChannel turns karma tracking on for the channel, storing its
// current display name; an already-known channel is re-enabled.
func (s *Store) EnableChannel(ctx context.Context, ch Channel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO channels (channel_id, team_id, name, enabled, created_at)
		VALUES (?, ?, ?, 1, ?)
		ON CONFLICT(channel_id) DO UPDATE SET
			enabled = 1,
			team_id = excluded.team_id,
			name = excluded.name`,
		ch.ID, ch.TeamID, ch.Name, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("enabling channel %s: %w", ch.ID, err)
	}
	return nil
}

// DisableChannel pauses karma tracking; collected karma is preserved.
func (s *Store) DisableChannel(ctx context.Context, channelID string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE channels SET enabled = 0 WHERE channel_id = ?",
		channelID)
	if err != nil {
		return fmt.Errorf("disabling channel %s: %w", channelID, err)
	}
	return nil
}

// IsChannelEnabled reports whether karma tracking is on for the channel.
func (s *Store) IsChannelEnabled(ctx context.Context, channelID string) (bool, error) {
	var enabled bool
	err := s.db.QueryRowContext(ctx,
		"SELECT enabled FROM channels WHERE channel_id = ?",
		channelID).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking channel %s: %w", channelID, err)
	}
	return enabled, nil
}

// EnabledChannels lists channels with karma tracking currently on.
func (s *Store) EnabledChannels(ctx context.Context) ([]Channel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT channel_id, team_id, name FROM channels WHERE enabled = 1`)
	if err != nil {
		return nil, fmt.Errorf("listing enabled channels: %w", err)
	}
	defer rows.Close()

	channels := []Channel{}
	for rows.Next() {
		var ch Channel
		if err := rows.Scan(&ch.ID, &ch.TeamID, &ch.Name); err != nil {
			return nil, fmt.Errorf("scanning channel row: %w", err)
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

// GrantKarma adds one point for the target and one unit to the giver's
// daily usage in a single transaction, returning the target's new weekly
// total.
func (s *Store) GrantKarma(ctx context.Context, g Grant) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning grant transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO karma (channel_id, user_id, username, week, karma)
		VALUES (?, ?, ?, ?, 1)
		ON CONFLICT(channel_id, user_id, week) DO UPDATE SET
			karma = karma + 1,
			username = excluded.username`,
		g.ChannelID, g.TargetID, g.TargetUsername, g.Week)
	if err != nil {
		return 0, fmt.Errorf("adding karma: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO daily_given (channel_id, from_user, to_user, day, amount)
		VALUES (?, ?, ?, ?, 1)
		ON CONFLICT(channel_id, from_user, to_user, day) DO UPDATE SET
			amount = amount + 1`,
		g.ChannelID, g.GiverID, g.TargetID, g.Day)
	if err != nil {
		return 0, fmt.Errorf("recording daily usage: %w", err)
	}

	var total int
	err = tx.QueryRowContext(ctx, `
		SELECT karma FROM karma
		WHERE channel_id = ? AND user_id = ? AND week = ?`,
		g.ChannelID, g.TargetID, g.Week).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("reading new karma total: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing grant transaction: %w", err)
	}
	return total, nil
}

// GivenOnDay returns how much karma the giver has already spent in the
// channel that day: the total and the split per target user ID.
func (s *Store) GivenOnDay(
	ctx context.Context,
	channelID, giverID, day string,
) (int, map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT to_user, amount FROM daily_given
		WHERE channel_id = ? AND from_user = ? AND day = ?`,
		channelID, giverID, day)
	if err != nil {
		return 0, nil, fmt.Errorf("reading daily usage: %w", err)
	}
	defer rows.Close()

	total := 0
	perTarget := map[string]int{}
	for rows.Next() {
		var targetID string
		var amount int
		if err := rows.Scan(&targetID, &amount); err != nil {
			return 0, nil, fmt.Errorf("scanning daily usage row: %w", err)
		}
		total += amount
		perTarget[targetID] = amount
	}
	return total, perTarget, rows.Err()
}

// TopByKarma returns the channel's highest scores for the week, capped at
// limit.
func (s *Store) TopByKarma(
	ctx context.Context,
	channelID, week string,
	limit int,
) ([]KarmaEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, username, karma FROM karma
		WHERE channel_id = ? AND week = ? AND karma > 0
		ORDER BY karma DESC, username ASC
		LIMIT ?`,
		channelID, week, limit)
	if err != nil {
		return nil, fmt.Errorf("reading top karma: %w", err)
	}
	defer rows.Close()

	entries := []KarmaEntry{}
	for rows.Next() {
		var e KarmaEntry
		if err := rows.Scan(&e.UserID, &e.Username, &e.Karma); err != nil {
			return nil, fmt.Errorf("scanning top karma row: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// UserKarmaByChannel lists the user's non-zero scores for the week across
// all channels, highest first.
func (s *Store) UserKarmaByChannel(
	ctx context.Context,
	userID, week string,
) ([]ChannelKarma, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.name, k.karma FROM karma k
		JOIN channels c ON c.channel_id = k.channel_id
		WHERE k.user_id = ? AND k.week = ? AND k.karma > 0
		ORDER BY k.karma DESC`,
		userID, week)
	if err != nil {
		return nil, fmt.Errorf("reading user karma by channel: %w", err)
	}
	defer rows.Close()

	entries := []ChannelKarma{}
	for rows.Next() {
		var e ChannelKarma
		if err := rows.Scan(&e.ChannelName, &e.Karma); err != nil {
			return nil, fmt.Errorf("scanning user karma row: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// KarmaWeekCounts returns the number of karma rows per stored period
// key, used at startup to detect rows written under a different period
// length.
func (s *Store) KarmaWeekCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT week, COUNT(*) FROM karma GROUP BY week`)
	if err != nil {
		return nil, fmt.Errorf("counting karma per week: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var week string
		var count int
		if err := rows.Scan(&week, &count); err != nil {
			return nil, fmt.Errorf("scanning karma week count: %w", err)
		}
		counts[week] = count
	}
	return counts, rows.Err()
}

// PruneDailyGiven deletes daily budget rows strictly older than the given
// day key.
func (s *Store) PruneDailyGiven(ctx context.Context, day string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM daily_given WHERE day < ?",
		day)
	if err != nil {
		return fmt.Errorf("pruning daily usage: %w", err)
	}
	return nil
}
