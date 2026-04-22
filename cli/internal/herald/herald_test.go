package herald

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// readDir wraps os.ReadDir and returns the entries (ignoring errors gracefully).
func readDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

// readJSONLFile reads all JSONL lines from path, decoding each as map[string]any.
func readJSONLFile(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("parse line: %v", err)
		}
		out = append(out, m)
	}
	return out
}

// ── EventType constants ───────────────────────────────────────────────────────

func TestEventTypeConstants_AllDefined(t *testing.T) {
	types := []EventType{
		SessionStart,
		SessionEnd,
		TurnComplete,
		ToolUse,
		CompactTriggered,
		MemoryCreated,
		MemoryPromoted,
		MemorySearched,
		MemoryDeleted,
		RecordAppended,
		ConfigChanged,
		Error,
	}
	if len(types) != 12 {
		t.Fatalf("expected 12 event type constants, got %d", len(types))
	}
}

func TestEventTypeConstants_Unique(t *testing.T) {
	types := []EventType{
		SessionStart,
		SessionEnd,
		TurnComplete,
		ToolUse,
		CompactTriggered,
		MemoryCreated,
		MemoryPromoted,
		MemorySearched,
		MemoryDeleted,
		RecordAppended,
		ConfigChanged,
		Error,
	}
	seen := make(map[EventType]bool)
	for _, et := range types {
		if seen[et] {
			t.Errorf("duplicate event type: %q", et)
		}
		seen[et] = true
	}
}

// ── Subscribe + Publish ───────────────────────────────────────────────────────

func TestPublish_DeliversToHandler(t *testing.T) {
	bus := NewBus()
	var received []Event

	bus.Subscribe(SessionStart, func(e Event) {
		received = append(received, e)
	})

	payload := map[string]any{"session_id": "s-123"}
	bus.Publish(SessionStart, payload)

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	ev := received[0]
	if ev.Type != SessionStart {
		t.Errorf("expected type %q, got %q", SessionStart, ev.Type)
	}
	if ev.Payload["session_id"] != "s-123" {
		t.Errorf("expected session_id %q, got %v", "s-123", ev.Payload["session_id"])
	}
	if ev.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestPublish_MultipleSubscribersReceiveSameEvent(t *testing.T) {
	bus := NewBus()
	var mu sync.Mutex
	var count int

	for i := 0; i < 3; i++ {
		bus.Subscribe(MemoryCreated, func(e Event) {
			mu.Lock()
			count++
			mu.Unlock()
		})
	}

	bus.Publish(MemoryCreated, map[string]any{"id": "m-001"})

	if count != 3 {
		t.Errorf("expected 3 handler calls, got %d", count)
	}
}

func TestPublish_UnsubscribedEventType_NoOp(t *testing.T) {
	bus := NewBus()
	// No subscribers registered.
	// Must not panic, must not error.
	bus.Publish(ConfigChanged, map[string]any{"key": "telemetry"})
}

func TestPublish_SetsTimestampUTC(t *testing.T) {
	bus := NewBus()
	var ev Event
	bus.Subscribe(TurnComplete, func(e Event) { ev = e })

	before := time.Now().UTC()
	bus.Publish(TurnComplete, nil)
	after := time.Now().UTC()

	if ev.Timestamp.Before(before) || ev.Timestamp.After(after) {
		t.Errorf("timestamp %v not in expected range [%v, %v]", ev.Timestamp, before, after)
	}
	if ev.Timestamp.Location() != time.UTC {
		t.Errorf("expected UTC, got %v", ev.Timestamp.Location())
	}
}

func TestPublish_NilPayload_NoOp(t *testing.T) {
	bus := NewBus()
	var received Event
	bus.Subscribe(Error, func(e Event) { received = e })
	bus.Publish(Error, nil)
	if received.Payload != nil {
		t.Errorf("expected nil payload, got %v", received.Payload)
	}
}

// ── Concurrent safety ─────────────────────────────────────────────────────────

func TestPublish_ConcurrentSafety(t *testing.T) {
	bus := NewBus()
	var count atomic.Int64

	bus.Subscribe(ToolUse, func(e Event) {
		count.Add(1)
	})

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			bus.Publish(ToolUse, map[string]any{"tool": "read"})
		}()
	}
	wg.Wait()

	if count.Load() != goroutines {
		t.Errorf("expected %d events, got %d", goroutines, count.Load())
	}
}

func TestSubscribe_ConcurrentWithPublish(t *testing.T) {
	bus := NewBus()
	var wg sync.WaitGroup

	// Concurrent subscribes and publishes must not race.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Subscribe(MemorySearched, func(e Event) {})
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(MemorySearched, nil)
		}()
	}
	wg.Wait()
}

// ── TelemetrySubscriber ───────────────────────────────────────────────────────

func TestTelemetrySubscriber_WritesJSONLOnEvent(t *testing.T) {
	momDir := t.TempDir()
	bus := NewBus()
	ts := NewTelemetrySubscriber(momDir, true)
	ts.Register(bus)

	// Publish a known event; TelemetrySubscriber should write it.
	bus.Publish(SessionStart, map[string]any{
		"session_id": "s-test",
		"runtime":    "claude-code",
	})

	// Read back — there should be at least one JSONL file.
	telPath := momDir + "/telemetry"
	entries, err := readDir(telPath)
	if err != nil {
		t.Fatalf("cannot read telemetry dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one JSONL file, got none")
	}

	lines := readJSONLFile(t, telPath+"/"+entries[0].Name())
	if len(lines) == 0 {
		t.Fatal("expected at least one line in JSONL file")
	}

	first := lines[0]
	if len(first) == 0 {
		t.Error("JSONL line is empty map")
	}
}

func TestTelemetrySubscriber_DisabledWritesNothing(t *testing.T) {
	momDir := t.TempDir()
	bus := NewBus()
	ts := NewTelemetrySubscriber(momDir, false)
	ts.Register(bus)

	bus.Publish(SessionEnd, map[string]any{"session_id": "s-off"})

	telDir := momDir + "/telemetry"
	entries, _ := readDir(telDir)
	if len(entries) != 0 {
		t.Errorf("disabled subscriber wrote %d file(s), expected 0", len(entries))
	}
}
