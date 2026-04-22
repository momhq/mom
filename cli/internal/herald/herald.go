package herald

import (
	"sync"
	"time"
)

// EventType identifies a category of bus event.
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

// Bus is a thread-safe pub/sub event bus.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]Handler
}

// NewBus returns an empty Bus ready for use.
func NewBus() *Bus {
	return &Bus{subscribers: make(map[EventType][]Handler)}
}

// Subscribe registers handler to receive events of eventType.
// Multiple handlers per type are supported; each receives its own copy of the event.
func (b *Bus) Subscribe(eventType EventType, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[eventType] = append(b.subscribers[eventType], handler)
}

// Publish dispatches an event of eventType with the given payload to all
// registered handlers. If no handlers are subscribed it is a no-op.
// Handlers are called synchronously in registration order.
func (b *Bus) Publish(eventType EventType, payload map[string]any) {
	b.mu.RLock()
	handlers := make([]Handler, len(b.subscribers[eventType]))
	copy(handlers, b.subscribers[eventType])
	b.mu.RUnlock()

	if len(handlers) == 0 {
		return
	}

	event := Event{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}

	for _, h := range handlers {
		h(event)
	}
}
