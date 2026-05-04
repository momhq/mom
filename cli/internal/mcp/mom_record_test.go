package mcp

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/momhq/mom/cli/internal/herald"
)

// recordingSubscriber attaches to the bus, captures the single event
// it sees (if any), and returns it. Tests use it to assert the
// handler's published event shape.
type recordingSubscriber struct {
	captured atomic.Value // herald.Event
	count    atomic.Int64
}

func (rs *recordingSubscriber) attach(bus *herald.Bus) func() {
	return bus.Subscribe(MemoryRecordEventType, func(e herald.Event) {
		rs.captured.Store(e)
		rs.count.Add(1)
	})
}

func (rs *recordingSubscriber) get() (herald.Event, bool) {
	v := rs.captured.Load()
	if v == nil {
		return herald.Event{}, false
	}
	return v.(herald.Event), true
}

func newSrvWithSubscriber(t *testing.T) (*Server, *recordingSubscriber) {
	t.Helper()
	srv := New(t.TempDir())
	rs := &recordingSubscriber{}
	t.Cleanup(rs.attach(srv.Bus()))
	return srv, rs
}

// ── happy path ────────────────────────────────────────────────────────────────

func TestMomRecord_PublishesEventWithNormalizedTags(t *testing.T) {
	srv, rs := newSrvWithSubscriber(t)

	res, err := srv.toolMomRecord(map[string]any{
		"session_id": "s-1",
		"summary":    "deploy notes",
		"content":    map[string]any{"text": "deploy postgres canary"},
		"tags":       []any{"v0.30", "MCP"},
		"actor":      "claude-code",
	})
	if err != nil {
		t.Fatalf("toolMomRecord: %v", err)
	}
	if res.IsError {
		t.Fatalf("got IsError=true: %+v", res)
	}

	got, ok := rs.get()
	if !ok {
		t.Fatal("no event published")
	}
	if got.Type != MemoryRecordEventType {
		t.Errorf("event type = %q, want %q", got.Type, MemoryRecordEventType)
	}
	if got.SessionID != "s-1" {
		t.Errorf("session_id = %q, want s-1", got.SessionID)
	}
	if _, dup := got.Payload["session_id"]; dup {
		t.Error("session_id was duplicated into payload bag; should live only on the envelope")
	}
	tags, _ := got.Payload["tags"].([]string)
	want := []string{"v0-30", "mcp"} // normalised
	if len(tags) != len(want) {
		t.Fatalf("tags = %v, want %v", tags, want)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Errorf("tags[%d] = %q, want %q", i, tags[i], want[i])
		}
	}

	// Provenance stamps locked.
	if got.Payload["provenance_trigger_event"] != "record" {
		t.Errorf("trigger_event = %v", got.Payload["provenance_trigger_event"])
	}
	if got.Payload["provenance_source_type"] != "manual-draft" {
		t.Errorf("source_type = %v", got.Payload["provenance_source_type"])
	}
	if got.Payload["provenance_actor"] != "claude-code" {
		t.Errorf("actor = %v", got.Payload["provenance_actor"])
	}
}

func TestMomRecord_DefaultsActorToMCP(t *testing.T) {
	srv, rs := newSrvWithSubscriber(t)
	_, err := srv.toolMomRecord(map[string]any{
		"session_id": "s",
		"content":    map[string]any{"text": "hi"},
	})
	if err != nil {
		t.Fatalf("toolMomRecord: %v", err)
	}
	got, _ := rs.get()
	if got.Payload["provenance_actor"] != "mcp" {
		t.Errorf("default actor = %v, want mcp", got.Payload["provenance_actor"])
	}
}

// ── validation ────────────────────────────────────────────────────────────────

func TestMomRecord_RejectsEmptySessionID(t *testing.T) {
	srv, rs := newSrvWithSubscriber(t)
	_, err := srv.toolMomRecord(map[string]any{
		"session_id": "",
		"content":    map[string]any{"text": "x"},
	})
	if err == nil {
		t.Fatal("expected error for empty session_id, got nil")
	}
	if !strings.Contains(err.Error(), "session_id") {
		t.Errorf("error %q should mention session_id", err)
	}
	// No event published.
	if rs.count.Load() != 0 {
		t.Errorf("event count = %d, want 0 (validation rejected before publish)", rs.count.Load())
	}
}

func TestMomRecord_RejectsMissingContent(t *testing.T) {
	srv, rs := newSrvWithSubscriber(t)
	_, err := srv.toolMomRecord(map[string]any{
		"session_id": "s",
	})
	if err == nil {
		t.Fatal("expected error for missing content")
	}
	if rs.count.Load() != 0 {
		t.Errorf("event count = %d, want 0", rs.count.Load())
	}
}

func TestMomRecord_RejectsEmptyContent(t *testing.T) {
	srv, rs := newSrvWithSubscriber(t)
	_, err := srv.toolMomRecord(map[string]any{
		"session_id": "s",
		"content":    map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	if rs.count.Load() != 0 {
		t.Errorf("event count = %d, want 0", rs.count.Load())
	}
}

func TestMomRecord_RejectsNonObjectContent(t *testing.T) {
	srv, rs := newSrvWithSubscriber(t)
	_, err := srv.toolMomRecord(map[string]any{
		"session_id": "s",
		"content":    "this is a string, not an object",
	})
	if err == nil {
		t.Fatal("expected error for string content")
	}
	if rs.count.Load() != 0 {
		t.Errorf("event count = %d, want 0", rs.count.Load())
	}
}

// TestMomRecord_RejectsTagsThatNormaliseToEmpty locks the lesson from
// the previous attempt: a memory + entity edge was persisted and THEN
// UpsertTag("") failed downstream, leaving an orphan memory. The fix
// is to validate post-normalisation tag emptiness BEFORE publishing.
func TestMomRecord_RejectsTagsThatNormaliseToEmpty(t *testing.T) {
	cases := []struct {
		name string
		tags []any
	}{
		{"all-punctuation", []any{"!!!"}},
		{"all-whitespace", []any{"   "}},
		{"mixed-some-empty", []any{"deploy", "  ", "ok"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, rs := newSrvWithSubscriber(t)
			_, err := srv.toolMomRecord(map[string]any{
				"session_id": "s",
				"content":    map[string]any{"text": "x"},
				"tags":       c.tags,
			})
			if err == nil {
				t.Fatal("expected error for normalise-empty tag")
			}
			if !strings.Contains(err.Error(), "normalises to empty") {
				t.Errorf("error %q should mention normalisation", err)
			}
			// CRITICAL: NO event must have been published.
			if rs.count.Load() != 0 {
				t.Errorf("event count = %d, want 0 — orphan-row regression", rs.count.Load())
			}
		})
	}
}

func TestMomRecord_RejectsMixedTypeTags(t *testing.T) {
	srv, _ := newSrvWithSubscriber(t)
	_, err := srv.toolMomRecord(map[string]any{
		"session_id": "s",
		"content":    map[string]any{"text": "x"},
		"tags":       []any{"deploy", 42},
	})
	if err == nil {
		t.Fatal("expected error for non-string tag element")
	}
}

// ── architectural / integration ──────────────────────────────────────────────

func TestServer_BusIsAccessibleAndDistinctPerInstance(t *testing.T) {
	a := New(t.TempDir())
	b := New(t.TempDir())
	if a.Bus() == nil || b.Bus() == nil {
		t.Fatal("Server.Bus() returned nil")
	}
	if a.Bus() == b.Bus() {
		t.Fatal("two Servers share the same Bus pointer")
	}
}

func TestServer_SetBusReplacesTheBus(t *testing.T) {
	srv := New(t.TempDir())
	custom := herald.NewBus()
	srv.SetBus(custom)
	if srv.Bus() != custom {
		t.Fatal("SetBus did not replace the bus")
	}
}
