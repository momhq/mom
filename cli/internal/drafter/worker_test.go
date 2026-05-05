package drafter_test

import (
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/momhq/mom/cli/internal/drafter"
	"github.com/momhq/mom/cli/internal/herald"
	"github.com/momhq/mom/cli/internal/librarian"
	"github.com/momhq/mom/cli/internal/logbook"
	"github.com/momhq/mom/cli/internal/vault"
)

// openWorker opens a fresh vault + librarian and returns a Drafter
// Worker plus the underlying Librarian for read-back assertions.
func openWorker(t *testing.T) (*drafter.Worker, *librarian.Librarian) {
	t.Helper()
	dir := t.TempDir()
	migs := append(librarian.Migrations(), logbook.Migrations()...)
	v, err := vault.Open(filepath.Join(dir, "mom.db"), migs)
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })
	lib := librarian.New(v)
	return drafter.NewWorker(lib), lib
}

// TestSubscribeTurnObserved_PersistsCleanMemory locks the happy path:
// substantive turn → memory persists → op.memory.created fires.
func TestSubscribeTurnObserved_PersistsCleanMemory(t *testing.T) {
	w, lib := openWorker(t)
	bus := herald.NewBus()
	defer w.SubscribeTurnObserved(bus)()

	var created atomic.Int64
	bus.Subscribe("op.memory.created", func(e herald.Event) { created.Add(1) })

	bus.Publish(herald.Event{
		Type:      herald.TurnObserved,
		SessionID: "s-1",
		Payload: map[string]any{
			"role":    "assistant",
			"text":    "I'll deploy postgres canary now and verify the connection pool",
			"harness": "claude-code",
			"model":   "claude-sonnet-4-6",
		},
	})

	if got := created.Load(); got != 1 {
		t.Fatalf("op.memory.created fired %d times, want 1", got)
	}
	rows, _ := lib.SearchMemories(librarian.SearchFilter{SessionID: "s-1", Limit: 10})
	if len(rows) != 1 {
		t.Fatalf("got %d memories, want 1", len(rows))
	}
	m := rows[0].Memory
	if !strings.Contains(m.Content, "deploy postgres canary") {
		t.Errorf("Content lost the original text: %q", m.Content)
	}
	if m.ProvenanceTriggerEvent != "watcher" {
		t.Errorf("ProvenanceTriggerEvent = %q, want watcher", m.ProvenanceTriggerEvent)
	}
	if m.ProvenanceSourceType != "transcript-extraction" {
		t.Errorf("ProvenanceSourceType = %q, want transcript-extraction", m.ProvenanceSourceType)
	}
	if m.ProvenanceActor != "claude-code" {
		t.Errorf("ProvenanceActor = %q, want claude-code (from harness)", m.ProvenanceActor)
	}
}

// TestSubscribeTurnObserved_RedactsAndBumpsAudit is the privacy
// contract end-to-end: a turn containing an AWS key is persisted as
// [REDACTED] in the memory AND the filter_audit category counter is
// bumped. The matched secret never appears in either row.
func TestSubscribeTurnObserved_RedactsAndBumpsAudit(t *testing.T) {
	w, lib := openWorker(t)
	bus := herald.NewBus()
	defer w.SubscribeTurnObserved(bus)()

	var redacted atomic.Int64
	bus.Subscribe("op.memory.redacted", func(e herald.Event) { redacted.Add(1) })

	bus.Publish(herald.Event{
		Type:      herald.TurnObserved,
		SessionID: "s-1",
		Payload: map[string]any{
			"role":    "assistant",
			"text":    "Why isn't AKIA1234567890ABCDEF working in this region?",
			"harness": "claude-code",
		},
	})

	if got := redacted.Load(); got != 1 {
		t.Fatalf("op.memory.redacted fired %d times, want 1", got)
	}

	rows, _ := lib.SearchMemories(librarian.SearchFilter{SessionID: "s-1", Limit: 10})
	if len(rows) != 1 {
		t.Fatalf("got %d memories, want 1 (redacted-and-persist)", len(rows))
	}
	m := rows[0].Memory
	if strings.Contains(m.Content, "AKIA1234567890ABCDEF") {
		t.Errorf("AWS key survived in memory content: %q", m.Content)
	}
	if !strings.Contains(m.Content, "[REDACTED]") {
		t.Errorf("expected [REDACTED] marker in content: %q", m.Content)
	}
	if !strings.Contains(m.Content, "working in this region") {
		t.Errorf("surrounding context lost: %q", m.Content)
	}

	// filter_audit row exists and was bumped to 1.
	auditRows, err := lib.FilterAuditCounts()
	if err != nil {
		t.Fatalf("FilterAuditCounts: %v", err)
	}
	if len(auditRows) != 1 {
		t.Fatalf("got %d filter_audit rows, want 1", len(auditRows))
	}
	if auditRows[0].Category != "aws_key" || auditRows[0].RedactionCount != 1 {
		t.Errorf("filter_audit row = %+v, want category=aws_key count=1", auditRows[0])
	}
}

// TestSubscribeTurnObserved_BumpsAuditFromToolInputSecret locks the
// "secrets in tool_input still bump filter_audit" rule. Drafter does
// not persist tool_input in memory content, but it still scans
// inputs for secrets so the lens panel reflects what the filter
// caught.
func TestSubscribeTurnObserved_BumpsAuditFromToolInputSecret(t *testing.T) {
	w, lib := openWorker(t)
	bus := herald.NewBus()
	defer w.SubscribeTurnObserved(bus)()

	bus.Publish(herald.Event{
		Type:      herald.TurnObserved,
		SessionID: "s-1",
		Payload: map[string]any{
			"role":    "assistant",
			"text":    "Running a deploy command now to verify the pipeline",
			"harness": "claude-code",
			"tool_calls": []map[string]any{
				{
					"name":     "Bash",
					"category": "system",
					"input":    map[string]any{"command": "AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF aws s3 ls"},
				},
			},
		},
	})

	rows, _ := lib.SearchMemories(librarian.SearchFilter{SessionID: "s-1", Limit: 10})
	if len(rows) != 1 {
		t.Fatalf("got %d memories, want 1", len(rows))
	}
	if strings.Contains(rows[0].Memory.Content, "AKIA1234567890ABCDEF") {
		t.Errorf("memory content leaked the AWS key from tool_input: %q", rows[0].Memory.Content)
	}

	// filter_audit fired even though the secret was in tool_input,
	// not in the prose. At minimum aws_key fired; env_assignment
	// also matches "AWS_ACCESS_KEY_ID=…" so it may or may not be
	// counted (regex order). Either way one of them must be present.
	auditRows, _ := lib.FilterAuditCounts()
	if len(auditRows) == 0 {
		t.Errorf("no filter_audit bumps despite a secret in tool_input")
	}
}

// TestSubscribeTurnObserved_DropsNoise locks the soft-filter path:
// a bare "ok" turn produces no memory and emits op.memory.dropped.
func TestSubscribeTurnObserved_DropsNoise(t *testing.T) {
	w, lib := openWorker(t)
	bus := herald.NewBus()
	defer w.SubscribeTurnObserved(bus)()

	var dropped atomic.Int64
	bus.Subscribe("op.memory.dropped", func(e herald.Event) { dropped.Add(1) })

	bus.Publish(herald.Event{
		Type:      herald.TurnObserved,
		SessionID: "s-1",
		Payload: map[string]any{
			"role": "user",
			"text": "ok",
		},
	})

	if got := dropped.Load(); got != 1 {
		t.Errorf("op.memory.dropped fired %d times, want 1", got)
	}
	rows, _ := lib.SearchMemories(librarian.SearchFilter{SessionID: "s-1", Limit: 10})
	if len(rows) != 0 {
		t.Errorf("got %d memories, want 0 (soft filter should have dropped)", len(rows))
	}
}

// TestSubscribeRecord_BypassesFiltersAndStampsProvenance locks the
// explicit-write path: mom_record events skip filtering entirely,
// even when their content would normally be redacted, and the
// provenance fields come from the event payload.
func TestSubscribeRecord_BypassesFiltersAndStampsProvenance(t *testing.T) {
	w, lib := openWorker(t)
	bus := herald.NewBus()
	defer w.SubscribeRecord(bus)()

	var created atomic.Int64
	bus.Subscribe("op.memory.created", func(e herald.Event) { created.Add(1) })

	bus.Publish(herald.Event{
		Type:      drafter.MemoryRecordEventType,
		SessionID: "s-1",
		Payload: map[string]any{
			"summary": "deploy notes",
			"content": map[string]any{
				"text": "I learned that deploying with AKIA1234567890ABCDEF requires explicit region",
			},
			"tags":                     []string{"deploy", "aws"},
			"provenance_actor":         "claude-code",
			"provenance_source_type":   "manual-draft",
			"provenance_trigger_event": "record",
		},
	})

	if got := created.Load(); got != 1 {
		t.Fatalf("op.memory.created fired %d times, want 1", got)
	}
	rows, _ := lib.SearchMemories(librarian.SearchFilter{SessionID: "s-1", Limit: 10})
	if len(rows) != 1 {
		t.Fatalf("got %d memories, want 1", len(rows))
	}
	m := rows[0].Memory
	// CRITICAL: filter bypass — the secret-shaped string MUST
	// survive verbatim because the user explicitly asked to record
	// this turn. ADR 0014 says explicitness wins over heuristics.
	if !strings.Contains(m.Content, "AKIA1234567890ABCDEF") {
		t.Errorf("explicit-write content was redacted; the user override must bypass filters\n  content: %q", m.Content)
	}
	if m.ProvenanceTriggerEvent != "record" {
		t.Errorf("ProvenanceTriggerEvent = %q, want record", m.ProvenanceTriggerEvent)
	}
	if m.ProvenanceSourceType != "manual-draft" {
		t.Errorf("ProvenanceSourceType = %q, want manual-draft", m.ProvenanceSourceType)
	}
	if m.ProvenanceActor != "claude-code" {
		t.Errorf("ProvenanceActor = %q, want claude-code", m.ProvenanceActor)
	}

	// Tags linked.
	tagged, _ := lib.MemoriesByTag("deploy")
	if len(tagged) != 1 || tagged[0] != m.ID {
		t.Errorf("MemoriesByTag(deploy) = %v, want [%q]", tagged, m.ID)
	}

	// filter_audit must NOT have fired — bypass means no audit bump
	// either.
	auditRows, _ := lib.FilterAuditCounts()
	if len(auditRows) != 0 {
		t.Errorf("filter_audit bumped on bypass path: %+v", auditRows)
	}
}

// TestOpMemoryEvents_PersistedThroughLogbook locks the F1 fix: when
// Logbook subscribes to op.memory.* alongside Drafter, every Drafter
// outcome lands as an op_events row. This is the audit-stream half
// of the capture pipeline; without it Lens has no "memory was
// created/redacted/dropped" timeline.
func TestOpMemoryEvents_PersistedThroughLogbook(t *testing.T) {
	w, lib := openWorker(t)
	bus := herald.NewBus()
	defer w.SubscribeAll(bus)()

	// Logbook listens for op.memory.* events on the same bus.
	lb := logbook.New(lib)
	defer lb.SubscribeAll(bus,
		herald.OpMemoryCreated,
		herald.OpMemoryRedacted,
		herald.OpMemoryDropped,
	)()

	// One clean turn → op.memory.created
	bus.Publish(herald.Event{
		Type:      herald.TurnObserved,
		SessionID: "s",
		Payload: map[string]any{
			"role":    "assistant",
			"text":    "deploy postgres canary, set the connection pool to 50",
			"harness": "claude-code",
		},
	})
	// One redacted turn → op.memory.redacted
	bus.Publish(herald.Event{
		Type:      herald.TurnObserved,
		SessionID: "s",
		Payload: map[string]any{
			"role":    "assistant",
			"text":    "AKIA1234567890ABCDEF leaked into the deploy step somehow",
			"harness": "claude-code",
		},
	})
	// One ack-noise turn → op.memory.dropped
	bus.Publish(herald.Event{
		Type:      herald.TurnObserved,
		SessionID: "s",
		Payload: map[string]any{
			"role": "user",
			"text": "ok",
		},
	})

	rows, err := lib.QueryOpEvents(librarian.OpEventFilter{SessionID: "s", Limit: 100})
	if err != nil {
		t.Fatalf("QueryOpEvents: %v", err)
	}
	// Expect: 2 turn.observed (the non-noise ones) + 3 op.memory.*
	// (created, redacted, dropped). The noise turn doesn't produce a
	// turn.observed projection because Logbook subscribes to
	// turn.observed too, but we publish all three turns and Logbook
	// records every one as turn.observed AND every Drafter outcome.
	gotTypes := map[string]int{}
	for _, r := range rows {
		gotTypes[r.EventType]++
	}
	if gotTypes["op.memory.created"] != 1 {
		t.Errorf("op.memory.created count = %d, want 1", gotTypes["op.memory.created"])
	}
	if gotTypes["op.memory.redacted"] != 1 {
		t.Errorf("op.memory.redacted count = %d, want 1", gotTypes["op.memory.redacted"])
	}
	if gotTypes["op.memory.dropped"] != 1 {
		t.Errorf("op.memory.dropped count = %d, want 1", gotTypes["op.memory.dropped"])
	}
}

// TestProcessRecord_AtomicMemoryAndTags locks the F5 fix: a record
// event with valid tags persists the memory + every tag edge in one
// transaction. Read-back asserts the memory exists AND every tag is
// linked.
func TestProcessRecord_AtomicMemoryAndTags(t *testing.T) {
	w, lib := openWorker(t)
	bus := herald.NewBus()
	defer w.SubscribeRecord(bus)()

	bus.Publish(herald.Event{
		Type:      herald.MemoryRecord,
		SessionID: "s",
		Payload: map[string]any{
			"content":                  map[string]any{"text": "deploy plan"},
			"tags":                     []string{"deploy", "postgres", "canary"},
			"provenance_actor":         "claude-code",
			"provenance_source_type":   "manual-draft",
			"provenance_trigger_event": "record",
		},
	})

	rows, _ := lib.SearchMemories(librarian.SearchFilter{SessionID: "s", Limit: 10})
	if len(rows) != 1 {
		t.Fatalf("got %d memories, want 1", len(rows))
	}
	memID := rows[0].ID

	for _, tag := range []string{"deploy", "postgres", "canary"} {
		ids, err := lib.MemoriesByTag(tag)
		if err != nil {
			t.Fatalf("MemoriesByTag(%q): %v", tag, err)
		}
		if len(ids) != 1 || ids[0] != memID {
			t.Errorf("MemoriesByTag(%q) = %v, want [%q]", tag, ids, memID)
		}
	}
}

func TestSubscribeAll_WiresBothEventTypes(t *testing.T) {
	w, lib := openWorker(t)
	bus := herald.NewBus()
	defer w.SubscribeAll(bus)()

	bus.Publish(herald.Event{
		Type:      herald.TurnObserved,
		SessionID: "s-1",
		Payload: map[string]any{
			"role":    "user",
			"text":    "deploy postgres canary, set the connection pool to 50",
			"harness": "claude-code",
		},
	})
	bus.Publish(herald.Event{
		Type:      drafter.MemoryRecordEventType,
		SessionID: "s-1",
		Payload: map[string]any{
			"content":                map[string]any{"text": "explicit save"},
			"provenance_actor":       "mcp",
			"provenance_source_type": "manual-draft",
			"provenance_trigger_event": "record",
		},
	})

	rows, _ := lib.SearchMemories(librarian.SearchFilter{SessionID: "s-1", Limit: 10})
	if len(rows) != 2 {
		t.Errorf("got %d memories, want 2 (one from each path)", len(rows))
	}
}
