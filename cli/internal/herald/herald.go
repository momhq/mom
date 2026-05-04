package herald

import (
	"fmt"
	"os"
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
type Event struct {
	Type      EventType
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

// Publish dispatches an event of eventType with the given payload to
// all registered handlers. Handlers fire synchronously in registration
// order. A panic in one handler is recovered and logged to stderr; the
// remaining handlers still fire. The panicking handler stays registered.
//
// stdout is reserved for JSON-RPC output by the MCP server, so all
// recovered-panic logging goes to stderr.
func (b *Bus) Publish(eventType EventType, payload map[string]any) {
	b.mu.RLock()
	hs := b.entries[eventType]
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

	event := Event{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}

	for _, h := range handlers {
		invoke(eventType, event, h)
	}
}

func invoke(eventType EventType, event Event, h Handler) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "herald: handler for %q panicked: %v\n", eventType, r)
		}
	}()
	h(event)
}
