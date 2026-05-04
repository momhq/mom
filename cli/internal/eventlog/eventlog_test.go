package eventlog_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/momhq/mom/cli/internal/eventlog"
	"github.com/momhq/mom/cli/internal/vault"
)

// newEventLog opens a fresh Vault in a temp dir and returns an
// EventLog backed by it.
func newEventLog(t *testing.T) (*eventlog.EventLog, *vault.Vault) {
	t.Helper()
	dir := t.TempDir()
	v, err := vault.Open(filepath.Join(dir, "mom.db"))
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })
	return eventlog.New(v), v
}

// T1 (tracer bullet): Log + Query round-trips an event with all
// fields populated. Verifies wiring through the event_log table.
func TestEventLog_LogAndQueryRoundTrip(t *testing.T) {
	el, _ := newEventLog(t)

	when := time.Date(2026, 5, 4, 12, 30, 0, 0, time.UTC)
	in := eventlog.Event{
		EventType: "capture",
		Timestamp: when,
		SessionID: "session-1",
		Payload:   map[string]any{"actor": "claude-code", "trigger_event": "watcher"},
	}
	if err := el.Log(in); err != nil {
		t.Fatalf("Log: %v", err)
	}

	rows, err := el.Query(eventlog.Filter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d: %v", len(rows), rows)
	}
	r := rows[0]
	if r.EventType != "capture" {
		t.Errorf("EventType: got %q, want %q", r.EventType, "capture")
	}
	if !r.Timestamp.Equal(when) {
		t.Errorf("Timestamp: got %v, want %v", r.Timestamp, when)
	}
	if r.SessionID != "session-1" {
		t.Errorf("SessionID: got %q, want %q", r.SessionID, "session-1")
	}
	if actor, _ := r.Payload["actor"].(string); actor != "claude-code" {
		t.Errorf("Payload.actor: got %v", r.Payload["actor"])
	}
	if r.ID == 0 {
		t.Errorf("expected non-zero auto-incremented ID")
	}
}

// logSimple is a fixture helper for tests that need multiple events.
func logSimple(t *testing.T, el *eventlog.EventLog, eventType, sessionID string, when time.Time) {
	t.Helper()
	if err := el.Log(eventlog.Event{
		EventType: eventType,
		SessionID: sessionID,
		Timestamp: when,
	}); err != nil {
		t.Fatalf("Log(%s): %v", eventType, err)
	}
}

// T2: Filter.EventType narrows results to events of that type.
func TestEventLog_Query_FiltersByEventType(t *testing.T) {
	el, _ := newEventLog(t)
	now := time.Now().UTC().Round(0)
	logSimple(t, el, "capture", "s1", now)
	logSimple(t, el, "recall", "s1", now)
	logSimple(t, el, "capture", "s1", now.Add(time.Second))

	rows, err := el.Query(eventlog.Filter{EventType: "capture"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 capture events, got %d: %v", len(rows), rows)
	}
	for _, r := range rows {
		if r.EventType != "capture" {
			t.Errorf("got event_type %q, want capture", r.EventType)
		}
	}
}

// T3: Filter.SessionID narrows results to events from that session.
func TestEventLog_Query_FiltersBySessionID(t *testing.T) {
	el, _ := newEventLog(t)
	now := time.Now().UTC().Round(0)
	logSimple(t, el, "capture", "s1", now)
	logSimple(t, el, "capture", "s2", now.Add(time.Second))
	logSimple(t, el, "capture", "s1", now.Add(2*time.Second))

	rows, err := el.Query(eventlog.Filter{SessionID: "s1"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 events for s1, got %d: %v", len(rows), rows)
	}
	for _, r := range rows {
		if r.SessionID != "s1" {
			t.Errorf("got session_id %q, want s1", r.SessionID)
		}
	}
}

// T4: Query returns rows ordered by timestamp descending — most
// recent first. Lens timeline UX expects newest at top.
func TestEventLog_Query_OrdersByTimestampDesc(t *testing.T) {
	el, _ := newEventLog(t)
	base := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	logSimple(t, el, "older", "s1", base)
	logSimple(t, el, "middle", "s1", base.Add(time.Hour))
	logSimple(t, el, "newest", "s1", base.Add(2*time.Hour))

	rows, err := el.Query(eventlog.Filter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	want := []string{"newest", "middle", "older"}
	for i, w := range want {
		if rows[i].EventType != w {
			t.Errorf("position %d: got %q, want %q", i, rows[i].EventType, w)
		}
	}
}

// T5: Filter.Since narrows results to events at or after the cutoff.
func TestEventLog_Query_FiltersBySince(t *testing.T) {
	el, _ := newEventLog(t)
	base := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	logSimple(t, el, "old", "s1", base)
	logSimple(t, el, "kept-1", "s1", base.Add(time.Hour))
	logSimple(t, el, "kept-2", "s1", base.Add(2*time.Hour))

	cutoff := base.Add(30 * time.Minute)
	rows, err := el.Query(eventlog.Filter{Since: cutoff})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 events at or after %v, got %d: %v", cutoff, len(rows), rows)
	}
	for _, r := range rows {
		if r.Timestamp.Before(cutoff) {
			t.Errorf("got event at %v, before cutoff %v", r.Timestamp, cutoff)
		}
	}
}

// counterByCategory finds a Counter by category for assertion lookups.
func counterByCategory(counters []eventlog.Counter, cat string) (eventlog.Counter, bool) {
	for _, c := range counters {
		if c.Category == cat {
			return c, true
		}
	}
	return eventlog.Counter{}, false
}

// T6: IncrementCounter creates a new row on first call (count=1) and
// increments on subsequent calls. last_fired_at advances to the most
// recent call.
func TestEventLog_IncrementCounter_CreatesAndIncrements(t *testing.T) {
	el, _ := newEventLog(t)

	if err := el.IncrementCounter("aws_secret"); err != nil {
		t.Fatalf("first IncrementCounter: %v", err)
	}
	counters, err := el.Counters()
	if err != nil {
		t.Fatalf("Counters: %v", err)
	}
	c, ok := counterByCategory(counters, "aws_secret")
	if !ok {
		t.Fatalf("expected aws_secret counter to exist after first call")
	}
	if c.RedactionCount != 1 {
		t.Errorf("first call: count got %d, want 1", c.RedactionCount)
	}
	firstFired := c.LastFiredAt
	if firstFired.IsZero() {
		t.Errorf("expected last_fired_at to be set on first call")
	}

	// Second + third call increment the same counter.
	if err := el.IncrementCounter("aws_secret"); err != nil {
		t.Fatalf("second IncrementCounter: %v", err)
	}
	if err := el.IncrementCounter("aws_secret"); err != nil {
		t.Fatalf("third IncrementCounter: %v", err)
	}
	counters, _ = el.Counters()
	c, _ = counterByCategory(counters, "aws_secret")
	if c.RedactionCount != 3 {
		t.Errorf("after three calls: count got %d, want 3", c.RedactionCount)
	}
	if !c.LastFiredAt.After(firstFired) && !c.LastFiredAt.Equal(firstFired) {
		t.Errorf("last_fired_at should be >= first call: got %v vs %v", c.LastFiredAt, firstFired)
	}
}

// T7: Counters() returns one row per distinct category.
func TestEventLog_Counters_ReturnsAllCategories(t *testing.T) {
	el, _ := newEventLog(t)

	if err := el.IncrementCounter("aws_secret"); err != nil {
		t.Fatal(err)
	}
	if err := el.IncrementCounter("aws_secret"); err != nil {
		t.Fatal(err)
	}
	if err := el.IncrementCounter("github_pat"); err != nil {
		t.Fatal(err)
	}

	counters, err := el.Counters()
	if err != nil {
		t.Fatalf("Counters: %v", err)
	}
	if len(counters) != 2 {
		t.Fatalf("expected 2 distinct counters, got %d: %v", len(counters), counters)
	}
	if c, _ := counterByCategory(counters, "aws_secret"); c.RedactionCount != 2 {
		t.Errorf("aws_secret count: got %d, want 2", c.RedactionCount)
	}
	if c, _ := counterByCategory(counters, "github_pat"); c.RedactionCount != 1 {
		t.Errorf("github_pat count: got %d, want 1", c.RedactionCount)
	}
}

// T8: Mixed-precision timestamps sort consistently in DESC order.
// RFC3339Nano trims trailing zeros, which makes lexical sort
// incorrect across mixed precisions ("." sorts before "Z" in ASCII).
// The eventlog format is fixed-width nanoseconds so DESC ordering
// matches actual chronology regardless of caller-supplied precision.
func TestEventLog_Query_OrdersConsistentlyAcrossMixedPrecision(t *testing.T) {
	el, _ := newEventLog(t)
	base := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)

	// Caller supplies whole-second timestamp.
	logSimple(t, el, "whole-second", "s1", base)
	// Caller supplies sub-second timestamp 500ms later.
	logSimple(t, el, "half-second-later", "s1", base.Add(500*time.Millisecond))
	// Caller supplies whole-second timestamp 1s later.
	logSimple(t, el, "one-second-later", "s1", base.Add(time.Second))

	rows, err := el.Query(eventlog.Filter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	want := []string{"one-second-later", "half-second-later", "whole-second"}
	for i, w := range want {
		if rows[i].EventType != w {
			t.Errorf("position %d: got %q, want %q", i, rows[i].EventType, w)
		}
	}
}

// T9: Log rejects empty EventType, empty SessionID, and zero
// timestamp is filled in. Locks the input contract documented on
// Event.
func TestEventLog_Log_RejectsMissingRequiredFields(t *testing.T) {
	el, _ := newEventLog(t)

	if err := el.Log(eventlog.Event{SessionID: "s1"}); err == nil {
		t.Errorf("expected error for empty EventType")
	}
	if err := el.Log(eventlog.Event{EventType: "capture"}); err == nil {
		t.Errorf("expected error for empty SessionID")
	}
}

// T10: IncrementCounter rejects empty category.
func TestEventLog_IncrementCounter_RejectsEmptyCategory(t *testing.T) {
	el, _ := newEventLog(t)

	if err := el.IncrementCounter(""); err == nil {
		t.Errorf("expected error for empty category")
	}
}

// T11: nil and empty-map payloads round-trip identically. Both
// serialise to "null" at write time, both come back as nil from
// Query. Removes a subtle distinction the caller would otherwise
// see between "no payload" and "explicitly empty payload".
func TestEventLog_Log_NormalisesEmptyPayload(t *testing.T) {
	el, _ := newEventLog(t)
	base := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)

	if err := el.Log(eventlog.Event{
		EventType: "nil-payload",
		SessionID: "s1",
		Timestamp: base,
		Payload:   nil,
	}); err != nil {
		t.Fatal(err)
	}
	if err := el.Log(eventlog.Event{
		EventType: "empty-map-payload",
		SessionID: "s1",
		Timestamp: base.Add(time.Second),
		Payload:   map[string]any{},
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := el.Query(eventlog.Filter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.Payload != nil {
			t.Errorf("event %q: expected Payload=nil, got %v", r.EventType, r.Payload)
		}
	}
}
