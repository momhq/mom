package crier_test

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momhq/mom/bus/herald"
	"github.com/momhq/mom/events/crier"
	"github.com/momhq/mom/storage/canonical"
	"github.com/momhq/mom/storage/ledger"
	"github.com/momhq/mom/storage/librarian"
	"github.com/momhq/mom/storage/vault"
)

// openLibrarian opens a fresh vault under t.TempDir() with the full
// canonical migration set (including migration 6 which crier needs).
func openLibrarian(t *testing.T) (*librarian.Librarian, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "mom.db")
	v, err := vault.Open(dbPath, canonical.Migrations())
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	lib := librarian.New(v)
	return lib, func() { _ = v.Close() }
}

func openLedger(t *testing.T) *ledger.Ledger {
	t.Helper()
	l, err := ledger.Open(filepath.Join(t.TempDir(), "ledger"))
	if err != nil {
		t.Fatalf("ledger.Open: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func ev(typ herald.EventType, session string) herald.Event {
	return herald.Event{
		Type:      typ,
		SessionID: session,
		Timestamp: time.Now().UTC(),
		Payload: map[string]any{
			"session_id": session,
			"role":       "user",
		},
	}
}

func TestReplay_AppliesNewEvents(t *testing.T) {
	lib, closeFn := openLibrarian(t)
	defer closeFn()
	led := openLedger(t)

	// Append 3 events to the Ledger.
	for _, s := range []string{"s1", "s2", "s3"} {
		if _, err := led.Append(ev(herald.TurnObserved, s)); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	c := crier.New(led, lib, nil)
	stats, err := c.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if stats.Applied != 3 {
		t.Errorf("Applied = %d, want 3", stats.Applied)
	}
	if stats.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", stats.Skipped)
	}
	if stats.LastOffset != 2 {
		t.Errorf("LastOffset = %d, want 2", stats.LastOffset)
	}
}

func TestReplay_IsIdempotentOnRerun(t *testing.T) {
	lib, closeFn := openLibrarian(t)
	defer closeFn()
	led := openLedger(t)
	for i := 0; i < 5; i++ {
		_, _ = led.Append(ev(herald.TurnObserved, "session"))
	}

	c := crier.New(led, lib, nil)
	first, err := c.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if first.Applied != 5 {
		t.Fatalf("first Replay: Applied = %d, want 5", first.Applied)
	}

	// Rerun: nothing new should apply.
	second, err := c.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if second.Applied != 0 {
		t.Errorf("second Replay: Applied = %d, want 0 (idempotent)", second.Applied)
	}
}

func TestReplay_ResumesAfterCheckpoint(t *testing.T) {
	lib, closeFn := openLibrarian(t)
	defer closeFn()
	led := openLedger(t)

	for i := 0; i < 3; i++ {
		_, _ = led.Append(ev(herald.TurnObserved, "session"))
	}
	c := crier.New(led, lib, nil)
	if _, err := c.Replay(); err != nil {
		t.Fatal(err)
	}

	// Append 2 more events after the first replay.
	for i := 0; i < 2; i++ {
		_, _ = led.Append(ev(herald.TurnObserved, "session"))
	}
	stats, err := c.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Applied != 2 {
		t.Errorf("Applied = %d, want 2 (only new events)", stats.Applied)
	}
	if stats.LastOffset != 4 {
		t.Errorf("LastOffset = %d, want 4", stats.LastOffset)
	}
}

func TestReplay_EmptyLedgerIsOk(t *testing.T) {
	lib, closeFn := openLibrarian(t)
	defer closeFn()
	led := openLedger(t)

	c := crier.New(led, lib, nil)
	stats, err := c.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Applied != 0 || stats.Skipped != 0 {
		t.Errorf("empty replay: stats = %+v, want zero", stats)
	}
}

func TestReplay_ProjectsCorrectVaultState(t *testing.T) {
	lib, closeFn := openLibrarian(t)
	defer closeFn()
	led := openLedger(t)

	// Append one event.
	_, _ = led.Append(ev(herald.OpMemoryCreated, "session-vault-state"))

	c := crier.New(led, lib, nil)
	if _, err := c.Replay(); err != nil {
		t.Fatal(err)
	}

	// Verify the op_event row landed via librarian.QueryOpEvents.
	rows, err := lib.QueryOpEvents(librarian.OpEventFilter{
		SessionID: "session-vault-state",
	})
	if err != nil {
		t.Fatalf("QueryOpEvents: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].EventType != string(herald.OpMemoryCreated) {
		t.Errorf("EventType = %q, want %q", rows[0].EventType, herald.OpMemoryCreated)
	}
}

// heraldAdapter wraps *herald.Bus so it satisfies crier.Publisher
// without forcing crier.go to import the bus package (archtest
// invariant: events/crier does not import bus/herald).
type heraldAdapter struct{ b *herald.Bus }

func (h heraldAdapter) Publish(eventType, sessionID string, ts time.Time, payload map[string]any) {
	h.b.Publish(herald.Event{
		Type:      herald.EventType(eventType),
		SessionID: sessionID,
		Timestamp: ts,
		Payload:   payload,
	})
}

func TestReplay_PublishesToHeraldOnApply(t *testing.T) {
	lib, closeFn := openLibrarian(t)
	defer closeFn()
	led := openLedger(t)

	bus := herald.NewBus()
	var received []herald.Event
	stop := bus.Subscribe(herald.TurnObserved, func(e herald.Event) {
		received = append(received, e)
	})
	defer stop()

	if _, err := led.Append(ev(herald.TurnObserved, "s-pub")); err != nil {
		t.Fatal(err)
	}

	c := crier.New(led, lib, heraldAdapter{b: bus})
	if _, err := c.Replay(); err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 {
		t.Fatalf("herald received %d events, want 1", len(received))
	}
	if received[0].SessionID != "s-pub" {
		t.Errorf("SessionID = %q, want s-pub", received[0].SessionID)
	}
	if received[0].Type != herald.TurnObserved {
		t.Errorf("Type = %q, want %q", received[0].Type, herald.TurnObserved)
	}
}

func TestReplay_IdempotentRerunDoesNotRepublish(t *testing.T) {
	lib, closeFn := openLibrarian(t)
	defer closeFn()
	led := openLedger(t)

	bus := herald.NewBus()
	var received []herald.Event
	stop := bus.Subscribe(herald.TurnObserved, func(e herald.Event) {
		received = append(received, e)
	})
	defer stop()

	_, _ = led.Append(ev(herald.TurnObserved, "s-once"))

	c := crier.New(led, lib, heraldAdapter{b: bus})
	if _, err := c.Replay(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Replay(); err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 {
		t.Fatalf("herald received %d events across two Replays, want 1 (republish guarded by applied bool)", len(received))
	}
}

// TestSkipBackfill_AdvancesCheckpointPastLedgerHead is the v0.50
// cutover behaviour: when Crier first wakes up against a Ledger that
// already contains records (those events were projected by the legacy
// bus path), it must NOT re-replay them. SkipBackfill jumps the
// checkpoint to the current head so live operation continues without
// duplicate op_events rows or duplicate Herald publishes.
func TestSkipBackfill_AdvancesCheckpointPastLedgerHead(t *testing.T) {
	lib, closeFn := openLibrarian(t)
	defer closeFn()
	led := openLedger(t)

	// Pre-populate three Ledger records — simulating the production
	// ledger that already has thousands.
	for _, s := range []string{"old-1", "old-2", "old-3"} {
		if _, err := led.Append(ev(herald.TurnObserved, s)); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	c := crier.New(led, lib, nil)
	skipped, head, err := c.SkipBackfill()
	if err != nil {
		t.Fatalf("SkipBackfill: %v", err)
	}
	if !skipped {
		t.Fatal("SkipBackfill returned skipped=false on a fresh checkpoint with non-empty ledger")
	}
	if head != 2 {
		t.Errorf("head = %d, want 2 (zero-indexed last offset of 3 records)", head)
	}

	// Replay must NOT apply the three pre-existing records.
	stats, err := c.Replay()
	if err != nil {
		t.Fatalf("Replay after SkipBackfill: %v", err)
	}
	if stats.Applied != 0 {
		t.Fatalf("Applied = %d, want 0 (SkipBackfill should make the pre-existing records invisible)", stats.Applied)
	}
}

func TestSkipBackfill_EmptyLedgerLeavesCheckpointFresh(t *testing.T) {
	lib, closeFn := openLibrarian(t)
	defer closeFn()
	led := openLedger(t)

	c := crier.New(led, lib, nil)
	skipped, _, err := c.SkipBackfill()
	if err != nil {
		t.Fatalf("SkipBackfill: %v", err)
	}
	if skipped {
		t.Fatal("SkipBackfill returned skipped=true on an empty ledger")
	}

	// Append one record AFTER SkipBackfill — it must project normally.
	_, _ = led.Append(ev(herald.TurnObserved, "first"))
	stats, err := c.Replay()
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if stats.Applied != 1 {
		t.Errorf("Applied = %d, want 1 (skip on empty ledger must not block first record)", stats.Applied)
	}
}

func TestSkipBackfill_IdempotentOnSecondCall(t *testing.T) {
	lib, closeFn := openLibrarian(t)
	defer closeFn()
	led := openLedger(t)
	for i := 0; i < 2; i++ {
		_, _ = led.Append(ev(herald.TurnObserved, "x"))
	}
	c := crier.New(led, lib, nil)

	if _, _, err := c.SkipBackfill(); err != nil {
		t.Fatal(err)
	}
	// Second call is a no-op — checkpoint already past head.
	skipped, _, err := c.SkipBackfill()
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("second SkipBackfill rolled the checkpoint back (must be a no-op)")
	}
}

// TestRun_PollsAndShutsDown asserts Crier.Run polls the Ledger on the
// supplied interval and republishes new events onto Herald, then
// returns cleanly when ctx is cancelled. The contract is the daemon
// loop production wiring uses (long-lived watcher daemon, MCP server).
func TestRun_PollsAndShutsDown(t *testing.T) {
	lib, closeFn := openLibrarian(t)
	defer closeFn()
	led := openLedger(t)

	bus := herald.NewBus()
	var received atomic.Int64
	stop := bus.Subscribe(herald.TurnObserved, func(e herald.Event) {
		received.Add(1)
	})
	defer stop()

	c := crier.New(led, lib, heraldAdapter{b: bus})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- c.Run(ctx, 5*time.Millisecond)
	}()

	// Append after Run has started — daemon loop must pick it up.
	if _, err := led.Append(ev(herald.TurnObserved, "s-runtime")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for received.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if received.Load() < 1 {
		t.Fatalf("Run did not republish appended event within 2s (received=%d)", received.Load())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("Run returned %v, want nil or context.Canceled", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Run did not return within 1s of ctx.Cancel")
	}
}

func TestReplay_FullReprojectionFromOffsetZero(t *testing.T) {
	dir := t.TempDir()
	// Use a stable ledger dir so we can reopen.
	ledDir := filepath.Join(dir, "ledger")
	led, err := ledger.Open(ledDir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		_, _ = led.Append(ev(herald.TurnObserved, "repro"))
	}
	_ = led.Close()

	// Open fresh vault + reopen ledger; replay from scratch.
	v, err := vault.Open(filepath.Join(dir, "mom.db"), canonical.Migrations())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	lib := librarian.New(v)

	led2, err := ledger.Open(ledDir)
	if err != nil {
		t.Fatal(err)
	}
	defer led2.Close()

	c := crier.New(led2, lib, nil)
	stats, err := c.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Applied != 4 {
		t.Fatalf("full reprojection: Applied = %d, want 4", stats.Applied)
	}

	// Vault state is exactly what Crier projected.
	rows, _ := lib.QueryOpEvents(librarian.OpEventFilter{SessionID: "repro"})
	if len(rows) != 4 {
		t.Errorf("rows = %d, want 4", len(rows))
	}
}
