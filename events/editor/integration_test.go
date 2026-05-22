package editor_test

import (
	"path/filepath"
	"testing"

	"github.com/momhq/mom/bus/herald"
	"github.com/momhq/mom/events/editor"
	"github.com/momhq/mom/storage/ledger"
)

// TestPublish_E2E_RealLedger appends through a real Ledger driver and
// confirms the on-disk record matches what Editor canonicalized. Per
// architecture v3, the Ledger is Editor's only side effect: Crier
// projects from there into the Vault and republishes onto Herald.
func TestPublish_E2E_RealLedger(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	led, err := ledger.Open(dir)
	if err != nil {
		t.Fatalf("ledger.Open: %v", err)
	}
	defer led.Close()

	e := editor.New(nil, nil).WithLedger(led)

	in := staticInput{
		eventType: "capture.turn.observed",
		payload:   map[string]any{"session_id": "e2e", "text": "hello world", "role": "user"},
	}
	if err := e.Publish(in, editor.Source{Adapter: "claude-code"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	rec, err := led.Read(0)
	if err != nil {
		t.Fatalf("ledger.Read(0): %v", err)
	}
	if rec.Event.Type != herald.TurnObserved {
		t.Errorf("ledger Type = %q, want %q", rec.Event.Type, herald.TurnObserved)
	}
	if got := rec.Event.Payload["text"]; got != "hello world" {
		t.Errorf("ledger payload text = %v, want hello world", got)
	}
	if got := rec.Event.Payload["provenance_actor"]; got != "claude-code" {
		t.Errorf("ledger payload provenance_actor = %v, want claude-code", got)
	}
}
