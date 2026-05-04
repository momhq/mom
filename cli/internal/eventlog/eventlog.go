// Package eventlog is the v0.30 operational telemetry layer over the
// SQLite vault. It owns the event_log table (append-only event stream,
// consumed by mom lens for the activity timeline) and the filter_audit
// table (per-category counters incremented by the capture filter
// pipeline). Separate from store — substance vs operations.
package eventlog

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/momhq/mom/cli/internal/vault"
)

// EventLog is the operational telemetry surface backed by a vault.
type EventLog struct {
	v *vault.Vault
}

// New returns an EventLog backed by the given vault.
func New(v *vault.Vault) *EventLog {
	return &EventLog{v: v}
}

// Event is the input shape for Log. Zero values are treated as
// reasonable defaults: empty Timestamp becomes time.Now().UTC(),
// nil Payload is stored as JSON null.
type Event struct {
	EventType string
	Timestamp time.Time
	SessionID string
	Payload   map[string]any
}

// Filter narrows a Query. Zero values mean "no filter on this
// dimension". Limit defaults to 100 if zero.
type Filter struct {
	EventType string
	SessionID string
	Since     time.Time
	Limit     int
}

// Row is the output shape from Query. ID is the autoincrement primary
// key; Timestamp is parsed back from the stored RFC3339Nano string.
type Row struct {
	ID        int64
	EventType string
	Timestamp time.Time
	SessionID string
	Payload   map[string]any
}

// Counter is the output shape for Counters(). Tracks one row per
// filter_audit category.
type Counter struct {
	Category       string
	RedactionCount int64
	LastFiredAt    time.Time
}

// Log appends an event to event_log. The row's auto-increment ID is
// not surfaced to the caller — use Query to retrieve persisted events.
func (e *EventLog) Log(ev Event) error {
	if ev.EventType == "" {
		return fmt.Errorf("eventlog.Log: EventType is required")
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC().Round(0)
	}
	payloadJSON, err := json.Marshal(ev.Payload)
	if err != nil {
		return fmt.Errorf("eventlog.Log: marshal payload: %w", err)
	}
	return e.v.Tx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO event_log (event_type, timestamp, session_id, payload)
			 VALUES (?, ?, ?, ?)`,
			ev.EventType,
			ev.Timestamp.Format(time.RFC3339Nano),
			ev.SessionID,
			string(payloadJSON),
		)
		return err
	})
}

// Query returns event_log rows matching the filter, ordered by
// timestamp descending (most recent first). Limit defaults to 100.
func (e *EventLog) Query(f Filter) ([]Row, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	var (
		sb   strings.Builder
		args []any
	)
	sb.WriteString(`SELECT id, event_type, timestamp, session_id, payload
		FROM event_log WHERE 1=1`)
	if f.EventType != "" {
		sb.WriteString(` AND event_type = ?`)
		args = append(args, f.EventType)
	}
	if f.SessionID != "" {
		sb.WriteString(` AND session_id = ?`)
		args = append(args, f.SessionID)
	}
	if !f.Since.IsZero() {
		sb.WriteString(` AND timestamp >= ?`)
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	sb.WriteString(` ORDER BY timestamp DESC, id DESC LIMIT ?`)
	args = append(args, limit)

	var rows []Row
	err := e.v.Query(sb.String(), args, func(rs *sql.Rows) error {
		for rs.Next() {
			var (
				r           Row
				ts, payload string
			)
			if err := rs.Scan(&r.ID, &r.EventType, &ts, &r.SessionID, &payload); err != nil {
				return err
			}
			t, err := time.Parse(time.RFC3339Nano, ts)
			if err != nil {
				return fmt.Errorf("parse timestamp: %w", err)
			}
			r.Timestamp = t
			if payload != "" && payload != "null" {
				if err := json.Unmarshal([]byte(payload), &r.Payload); err != nil {
					return fmt.Errorf("unmarshal payload: %w", err)
				}
			}
			rows = append(rows, r)
		}
		return nil
	})
	return rows, err
}

// IncrementCounter atomically creates or increments the filter_audit
// row for the given category and updates last_fired_at to now.
func (e *EventLog) IncrementCounter(category string) error {
	if category == "" {
		return fmt.Errorf("eventlog.IncrementCounter: category is required")
	}
	now := time.Now().UTC().Round(0).Format(time.RFC3339Nano)
	return e.v.Tx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO filter_audit (category, redaction_count, last_fired_at)
			 VALUES (?, 1, ?)
			 ON CONFLICT(category) DO UPDATE SET
			   redaction_count = redaction_count + 1,
			   last_fired_at = excluded.last_fired_at`,
			category, now,
		)
		return err
	})
}

// Counters returns all filter_audit rows ordered by category.
func (e *EventLog) Counters() ([]Counter, error) {
	var counters []Counter
	err := e.v.Query(
		`SELECT category, redaction_count, last_fired_at
		 FROM filter_audit ORDER BY category`,
		nil,
		func(rs *sql.Rows) error {
			for rs.Next() {
				var (
					c             Counter
					lastFiredText sql.NullString
				)
				if err := rs.Scan(&c.Category, &c.RedactionCount, &lastFiredText); err != nil {
					return err
				}
				if lastFiredText.Valid && lastFiredText.String != "" {
					t, err := time.Parse(time.RFC3339Nano, lastFiredText.String)
					if err != nil {
						return fmt.Errorf("parse last_fired_at: %w", err)
					}
					c.LastFiredAt = t
				}
				counters = append(counters, c)
			}
			return nil
		},
	)
	return counters, err
}
