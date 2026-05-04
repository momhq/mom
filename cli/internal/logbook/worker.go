// File worker.go contains Logbook's v0.30 surface: a worker that
// subscribes to operational events on Herald and persists them through
// Librarian into the op_events table. The legacy transcript-parsing
// surface in this package (logbook.go and friends) is a separate
// concern used by lens, watcher, and cmd; the two coexist while v1
// callers migrate.
package logbook

import (
	"fmt"
	"os"

	"github.com/momhq/mom/cli/internal/herald"
	"github.com/momhq/mom/cli/internal/librarian"
)

// Worker subscribes to operational events on Herald and persists them
// through Librarian. Worker does NOT touch the Vault directly (that
// rule is owned by Librarian); the package-level architecture test
// asserts the import graph.
type Worker struct {
	lib *librarian.Librarian
}

// New returns a Worker backed by the given Librarian. Callers wire it
// to a Herald bus by calling Subscribe for each event type they want
// recorded.
func New(lib *librarian.Librarian) *Worker {
	return &Worker{lib: lib}
}

// Log writes an operational event directly through Librarian, without
// going via Herald. Useful for tests, for synchronous-write call paths
// (e.g., upgrade), and as the implementation hook used by Subscribe.
//
// EventType and SessionID are required; empty inputs are rejected at
// the API boundary by Librarian.
func (w *Worker) Log(eventType, sessionID string, payload map[string]any) error {
	_, err := w.lib.InsertOpEvent(librarian.OpEvent{
		EventType: eventType,
		SessionID: sessionID,
		Payload:   payload,
	})
	return err
}

// Subscribe registers a handler on the bus that persists every matching
// event. Returns the unsubscribe func from Herald — callers may detach
// the worker without retaining the Bus reference.
//
// SessionID comes from the Event envelope (e.SessionID), set by the
// producer. An empty SessionID is a programming-error event and is
// skipped with a stderr log — the schema NOT NULL constraint would
// reject it anyway, but losing it silently in the audit substrate is
// the bigger sin. A persistence failure (closed vault, FK error, disk
// full) is also logged to stderr; we do not silently drop audit data.
func (w *Worker) Subscribe(bus *herald.Bus, eventType herald.EventType) func() {
	return bus.Subscribe(eventType, func(e herald.Event) {
		if e.SessionID == "" {
			fmt.Fprintf(os.Stderr, "logbook: drop %q event with empty session_id\n", e.Type)
			return
		}
		if err := w.Log(string(e.Type), e.SessionID, e.Payload); err != nil {
			fmt.Fprintf(os.Stderr, "logbook: persist %q failed: %v\n", e.Type, err)
		}
	})
}

// SubscribeAll wires the worker to every listed event type and returns
// a single unsubscribe that detaches them all.
func (w *Worker) SubscribeAll(bus *herald.Bus, eventTypes ...herald.EventType) func() {
	unsubs := make([]func(), 0, len(eventTypes))
	for _, t := range eventTypes {
		unsubs = append(unsubs, w.Subscribe(bus, t))
	}
	return func() {
		for _, u := range unsubs {
			u()
		}
	}
}

// Query reads back rows from the operational stream through Librarian.
func (w *Worker) Query(filter librarian.OpEventFilter) ([]librarian.OpEvent, error) {
	return w.lib.QueryOpEvents(filter)
}
