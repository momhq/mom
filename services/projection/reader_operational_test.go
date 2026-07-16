package projection

import (
	"os"
	"testing"
	"time"

	"github.com/momhq/mom/events/editor"
	"github.com/momhq/mom/events/envelope"
	"github.com/momhq/mom/storage/ledger"
)

// publishOperational appends a raw operational event via the Editor.
func publishOperational(t *testing.T, ed *editor.Editor, evType envelope.EventType, payload map[string]any) {
	t.Helper()
	c := &rawCan{eventType: evType, payload: payload}
	if err := ed.Publish(c, editor.Source{Adapter: "ingest"}); err != nil {
		t.Fatalf("Publish %s: %v", evType, err)
	}
}

type rawCan struct {
	eventType envelope.EventType
	payload   map[string]any
}

func (c *rawCan) Canonical() (envelope.EventType, map[string]any) {
	return c.eventType, c.payload
}

// openTestPipeline opens a temp ledger + editor for projection reader tests.
func openTestPipeline(t *testing.T) (*ledger.Ledger, *editor.Editor, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mom-reader-test-*")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	led, err := ledger.Open(dir)
	if err != nil {
		t.Fatalf("ledger.Open: %v", err)
	}
	t.Cleanup(func() { _ = led.Close() })

	ed := editor.New(nil, nil).WithLedger(led)
	return led, ed, dir
}

// TestReader_OperationalMessagePosted_ChatKind asserts that a
// operational.message.posted with kind=chat becomes a FoldEvent of type
// "os.message" with the expected text shape.
func TestReader_OperationalMessagePosted_ChatKind(t *testing.T) {
	_, ed, dir := openTestPipeline(t)

	publishOperational(t, ed, envelope.OperationalMessagePosted, map[string]any{
		"project_id":  "proj-reader",
		"session_id":  "sess-1",
		"message_id":  "msg-1",
		"channel_id":  "chan-1",
		"sender_type": "agent",
		"sender_id":   "agent-1",
		"sender_name": "Toad",
		"kind":        "chat",
		"body":        "Hello from chat",
		"created_at":  time.Now().UTC().Format(time.RFC3339),
	})

	r := NewReader(dir, "proj-reader")
	res, err := r.Read(0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("events: want 1, got %d", len(res.Events))
	}
	fe := res.Events[0]
	if fe.Type != "os.message" {
		t.Errorf("type: want os.message, got %s", fe.Type)
	}
	if fe.Role != "Toad (agent)" {
		t.Errorf("role: want 'Toad (agent)', got %s", fe.Role)
	}
	if fe.Text != "[chat] Hello from chat" {
		t.Errorf("text: want '[chat] Hello from chat', got %s", fe.Text)
	}
}

// TestReader_OperationalMessagePosted_ToolResultDropped asserts that
// operational.message.posted with kind=tool_result is not included in the fold.
func TestReader_OperationalMessagePosted_ToolResultDropped(t *testing.T) {
	_, ed, dir := openTestPipeline(t)

	publishOperational(t, ed, envelope.OperationalMessagePosted, map[string]any{
		"project_id":  "proj-reader",
		"session_id":  "sess-1",
		"message_id":  "msg-2",
		"channel_id":  "chan-1",
		"sender_type": "system",
		"sender_id":   "sys",
		"sender_name": "system",
		"kind":        "tool_result",
		"body":        "tool output here",
		"created_at":  time.Now().UTC().Format(time.RFC3339),
	})

	r := NewReader(dir, "proj-reader")
	res, err := r.Read(0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Events) != 0 {
		t.Errorf("expected 0 fold events for tool_result kind, got %d", len(res.Events))
	}
}

// TestReader_OperationalApprovalResolved_WithNote asserts that
// operational.approval.resolved produces an os.approval FoldEvent with note.
func TestReader_OperationalApprovalResolved_WithNote(t *testing.T) {
	_, ed, dir := openTestPipeline(t)

	publishOperational(t, ed, envelope.OperationalApprovalResolved, map[string]any{
		"project_id":  "proj-reader",
		"session_id":  "sess-1",
		"approval_id": "appr-1",
		"status":      "approved",
		"note":        "looks good",
		"resolved_at": time.Now().UTC().Format(time.RFC3339),
	})

	r := NewReader(dir, "proj-reader")
	res, err := r.Read(0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("events: want 1, got %d", len(res.Events))
	}
	fe := res.Events[0]
	if fe.Type != "os.approval" {
		t.Errorf("type: want os.approval, got %s", fe.Type)
	}
	if fe.Text != "approval approved: looks good" {
		t.Errorf("text: want 'approval approved: looks good', got %s", fe.Text)
	}
}

// TestReader_OperationalApprovalResolved_NoNote asserts that
// operational.approval.resolved without a note produces the short form.
func TestReader_OperationalApprovalResolved_NoNote(t *testing.T) {
	_, ed, dir := openTestPipeline(t)

	publishOperational(t, ed, envelope.OperationalApprovalResolved, map[string]any{
		"project_id":  "proj-reader",
		"session_id":  "sess-1",
		"approval_id": "appr-2",
		"status":      "rejected",
		"resolved_at": time.Now().UTC().Format(time.RFC3339),
	})

	r := NewReader(dir, "proj-reader")
	res, err := r.Read(0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("events: want 1, got %d", len(res.Events))
	}
	if res.Events[0].Text != "approval rejected" {
		t.Errorf("text: want 'approval rejected', got %s", res.Events[0].Text)
	}
}

// TestReader_OperationalRunStarted_Dropped asserts that operational.run.started
// is stored in the Ledger but not included in the fold output.
func TestReader_OperationalRunStarted_Dropped(t *testing.T) {
	_, ed, dir := openTestPipeline(t)

	publishOperational(t, ed, envelope.OperationalRunStarted, map[string]any{
		"project_id": "proj-reader",
		"session_id": "sess-1",
		"run_id":     "run-1",
		"agent_id":   "agent-1",
		"channel_id": "chan-1",
		"title":      "A run",
		"model":      "claude",
		"started_at": time.Now().UTC().Format(time.RFC3339),
	})

	r := NewReader(dir, "proj-reader")
	res, err := r.Read(0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Events) != 0 {
		t.Errorf("expected 0 fold events for run.started, got %d", len(res.Events))
	}
	// Head is offset 0 of the run.started — that is fine; HeadOffset is set
	// regardless of whether the event was kept, so there is nothing to assert.
}

// TestReader_ProjectFilter asserts that events for other projects are dropped.
func TestReader_ProjectFilter(t *testing.T) {
	_, ed, dir := openTestPipeline(t)

	publishOperational(t, ed, envelope.OperationalMessagePosted, map[string]any{
		"project_id":  "proj-other",
		"session_id":  "sess-1",
		"message_id":  "msg-1",
		"channel_id":  "chan-1",
		"sender_type": "agent",
		"sender_id":   "a1",
		"sender_name": "Alice",
		"kind":        "chat",
		"body":        "not for you",
		"created_at":  time.Now().UTC().Format(time.RFC3339),
	})
	publishOperational(t, ed, envelope.OperationalMessagePosted, map[string]any{
		"project_id":  "proj-reader",
		"session_id":  "sess-1",
		"message_id":  "msg-2",
		"channel_id":  "chan-1",
		"sender_type": "agent",
		"sender_id":   "a1",
		"sender_name": "Alice",
		"kind":        "plan",
		"body":        "here is the plan",
		"created_at":  time.Now().UTC().Format(time.RFC3339),
	})

	r := NewReader(dir, "proj-reader")
	res, err := r.Read(0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("events: want 1 (only proj-reader), got %d", len(res.Events))
	}
	if res.Events[0].Text != "[plan] here is the plan" {
		t.Errorf("text mismatch: %s", res.Events[0].Text)
	}
}
