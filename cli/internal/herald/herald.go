package herald

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"
)

// EventType identifies a category of bus event. v0.30 callers may use a
// plain string literal; v1 callers use the predefined constants below.
type EventType string

const (
	SessionStart     EventType = "session-start"
	SessionEnd       EventType = "session-end"
	TurnComplete     EventType = "turn-complete"
	ToolUse          EventType = "tool-use"
	CompactTriggered EventType = "compact-triggered"
	MemoryCreated    EventType = "memory-created"
	MemoryPromoted   EventType = "memory-promoted"
	MemorySearched   EventType = "memory-searched"
	MemoryDeleted    EventType = "memory-deleted"
	RecordAppended   EventType = "record-appended"
	ConfigChanged    EventType = "config-changed"
	Error            EventType = "error"
)

// Event is a single message on the bus.
//
// Type and SessionID are first-class envelope fields — producers set
// them explicitly, consumers read them directly. SessionID is required
// for any event a worker (Logbook, Drafter) will persist; programming-
// error empty values are caught at the worker boundary, not buried
// inside the payload.
//
// Timestamp is set by Publish (not by producers) using wall-clock UTC.
// Any value a caller assigns is overwritten — the bus is the only
// authority for "when this event happened on the wire." Historical
// times for replay or import belong on the persisted row, not on the
// envelope.
//
// Payload carries per-event-type fields. The bus is type-agnostic; the
// payload contract is defined by each producer/consumer pair (see
// e.g. mcp.MemoryRecordPayload).
type Event struct {
	Type      EventType
	SessionID string
	Timestamp time.Time
	Payload   map[string]any
}

// Handler is a function that processes an Event.
type Handler func(Event)

// Bus is the v0.30 in-process pub/sub event bus. It connects event
// producers (watcher, MCP handlers, CLI) to event consumers (Drafter,
// Logbook, Cartographer, Lens). Bus has no knowledge of Vault or
// Librarian — it is a pure router. Persistence is the subscriber's job.
//
// Bus is safe for concurrent use.
type Bus struct {
	mu      sync.RWMutex
	nextID  uint64
	entries map[EventType]map[uint64]Handler
}

// NewBus returns an empty Bus ready for use.
func NewBus() *Bus {
	return &Bus{entries: make(map[EventType]map[uint64]Handler)}
}

// Subscribe registers handler to receive events of eventType. Multiple
// handlers per type are supported; each receives its own copy of the
// event when Publish fires.
//
// The returned function deregisters this specific handler. It is
// idempotent — calling it more than once is a no-op and does not affect
// other subscribers.
func (b *Bus) Subscribe(eventType EventType, handler Handler) func() {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	if b.entries[eventType] == nil {
		b.entries[eventType] = make(map[uint64]Handler)
	}
	b.entries[eventType][id] = handler
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if hs, ok := b.entries[eventType]; ok {
				delete(hs, id)
				if len(hs) == 0 {
					delete(b.entries, eventType)
				}
			}
		})
	}
}

// Publish dispatches the event to all handlers registered for its
// type. Handlers fire synchronously in registration order. A panic in
// one handler is recovered and logged to stderr; the remaining
// handlers still fire. The panicking handler stays registered.
//
// Publish always stamps Timestamp with wall-clock UTC, overwriting any
// value the caller may have set. The bus is the sole authority for
// event timing.
//
// stdout is reserved for JSON-RPC output by the MCP server, so all
// recovered-panic logging goes to stderr.
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	hs := b.entries[e.Type]
	if len(hs) == 0 {
		b.mu.RUnlock()
		return
	}
	// Snapshot handlers so we hold no lock while invoking them.
	handlers := make([]Handler, 0, len(hs))
	for _, h := range hs {
		handlers = append(handlers, h)
	}
	b.mu.RUnlock()

	e.Timestamp = time.Now().UTC()

	for _, h := range handlers {
		invoke(e.Type, e, h)
	}
}

func invoke(eventType EventType, event Event, h Handler) {
	defer func() {
		if r := recover(); r != nil {
			// Include the goroutine stack so a recovered panic is
			// debuggable from a single log line. One-line panics are
			// brittle when the failure mode is "which subscriber blew
			// up and why."
			fmt.Fprintf(os.Stderr, "herald: handler for %q panicked: %v\n%s\n", eventType, r, debug.Stack())
		}
	}()
	h(event)
}
