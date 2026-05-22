package cli

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/momhq/mom/bus/herald"
	"github.com/momhq/mom/events/editor"
	"github.com/momhq/mom/storage/canonical"
	"github.com/momhq/mom/storage/ledger"
	"github.com/momhq/mom/storage/librarian"
	"github.com/momhq/mom/storage/vault"
	"github.com/momhq/mom/workers/drafter"
)

// recordCanonical is a Canonicalizer that emits a MemoryRecord event
// with the given session and content. Used by the e2e test to drive
// the full Watcher→Editor→Ledger→Crier→Herald→Drafter pipeline without
// reaching into ingress/record (which is the production producer).
type recordCanonical struct {
	sessionID string
	content   map[string]any
}

func (r recordCanonical) Canonical() (herald.EventType, map[string]any) {
	return herald.MemoryRecord, map[string]any{
		"session_id":               r.sessionID,
		"content":                  r.content,
		"summary":                  "e2e",
		"tags":                     []string{"e2e"},
		"provenance_actor":         "test",
		"provenance_source_type":   "manual-draft",
		"provenance_trigger_event": "record",
	}
}

// TestPipeline_EditorLedgerCrierHeraldDrafter exercises the full
// architecture v3 write path: Editor appends to Ledger, Crier projects
// + republishes onto Herald, Drafter subscribes and persists. The
// draft must NOT exist before Crier ticks — that is the behavioural
// proof that Crier (not Editor) is the live publisher.
func TestPipeline_EditorLedgerCrierHeraldDrafter(t *testing.T) {
	dir := t.TempDir()

	// Vault + Librarian.
	v, err := vault.Open(filepath.Join(dir, "mom.db"), canonical.Migrations())
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	defer v.Close()
	lib := librarian.New(v)

	// Ledger.
	led, err := ledger.Open(filepath.Join(dir, "ledger"))
	if err != nil {
		t.Fatalf("ledger.Open: %v", err)
	}
	defer led.Close()

	// Bus + Drafter subscriber.
	bus := herald.NewBus()
	stop := drafter.New(lib).SubscribeAll(bus)
	defer stop()

	// Probe subscriber asserts Crier actually republishes onto the bus.
	// Separates the Crier→Herald step from the downstream Drafter
	// persistence step, so a failure points at the right component.
	var crierPublishes int
	stopProbe := bus.Subscribe(herald.MemoryRecord, func(e herald.Event) {
		crierPublishes++
	})
	defer stopProbe()

	// Editor wired to the Ledger only — no Herald dependency, per
	// architecture v3.
	ed := editor.New(nil, nil).WithLedger(led)

	// Hand a MemoryRecord event to Editor.
	in := recordCanonical{
		sessionID: "11111111-1111-4111-8111-111111111111",
		content:   map[string]any{"text": "e2e draft from the pipeline test"},
	}
	if err := ed.Publish(in, editor.Source{Adapter: "test"}); err != nil {
		t.Fatalf("editor.Publish: %v", err)
	}

	// Critical invariant: Drafter must NOT have persisted yet. The
	// event is on the Ledger but Crier has not projected it onto
	// Herald.
	rowsBefore, err := lib.RecentDrafts(librarian.RecentDraftsFilter{Limit: 10, Since: time.Hour})
	if err != nil {
		t.Fatalf("RecentDrafts pre-Crier: %v", err)
	}
	if len(rowsBefore) != 0 {
		t.Fatalf("draft already exists before Crier ticked: %d rows (Editor must not publish to Herald)", len(rowsBefore))
	}

	// Now drain the Ledger via Crier — projects to vault AND republishes
	// onto bus, which Drafter consumes and turns into a persisted draft.
	if err := drainLedgerToBus(led, lib, bus); err != nil {
		t.Fatalf("drainLedgerToBus: %v", err)
	}

	if crierPublishes != 1 {
		t.Fatalf("Crier published %d MemoryRecord events onto Herald, want 1", crierPublishes)
	}
	rowsAfter, err := lib.RecentDrafts(librarian.RecentDraftsFilter{Limit: 10, Since: time.Hour})
	if err != nil {
		t.Fatalf("RecentDrafts post-Crier: %v", err)
	}
	if len(rowsAfter) != 1 {
		t.Fatalf("post-Crier drafts = %d, want 1 (Drafter should have persisted via Crier's republish)", len(rowsAfter))
	}
	if rowsAfter[0].SessionID != in.sessionID {
		t.Errorf("draft SessionID = %q, want %q", rowsAfter[0].SessionID, in.sessionID)
	}
}
