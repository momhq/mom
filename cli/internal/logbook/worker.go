// File worker.go contains Logbook's v0.30 surface: a worker that
// subscribes to operational events on Herald and persists them through
// Librarian into the op_events table. The legacy transcript-parsing
// surface in this package (logbook.go and friends) is a separate
// concern used by lens, watcher, and cmd; the two coexist while v1
// callers migrate.
package logbook

import (
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
// The handler reads session_id from event.Payload["session_id"] when
// present; if missing, the row is skipped (the API requires non-empty
// session_id and a programming-error event without one should not
// silently land in the stream).
func (w *Worker) Subscribe(bus *herald.Bus, eventType herald.EventType) func() {
	return bus.Subscribe(eventType, func(e herald.Event) {
		sessionID := stringField(e.Payload, "session_id")
		if sessionID == "" {
			return
		}
		_ = w.Log(string(e.Type), sessionID, e.Payload)
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

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
