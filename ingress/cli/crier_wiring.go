package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/momhq/mom/bus/herald"
	"github.com/momhq/mom/events/crier"
	"github.com/momhq/mom/events/editor"
	"github.com/momhq/mom/storage/ledger"
	"github.com/momhq/mom/storage/librarian"
)

// crierPollInterval is the cadence Crier.Run polls the Ledger. 100 ms
// keeps draft-creation latency below human-perceptible thresholds
// while bounding wakeups when the Ledger is idle. Tunable if it shows
// up in syscall traces.
const crierPollInterval = 100 * time.Millisecond

// publisherFromBus returns a crier.Publisher that republishes onto bus.
// This is the only place ingress/cli bridges the architectural seam:
// crier.go is herald-free (archtest invariant); the herald.Event
// envelope is reconstructed here at the wiring layer.
func publisherFromBus(bus *herald.Bus) crier.Publisher {
	if bus == nil {
		return nil
	}
	return crier.PublisherFunc(func(eventType, sessionID string, ts time.Time, payload map[string]any) {
		bus.Publish(herald.Event{
			Type:      herald.EventType(eventType),
			SessionID: sessionID,
			Timestamp: ts,
			Payload:   payload,
		})
	})
}

// newCrier returns a Crier wired to project from the Ledger into the
// central vault (via Librarian) and republish on the given bus.
// Returns nil when led or lib is nil so callers degrade gracefully.
func newCrier(led *ledger.Ledger, lib *librarian.Librarian, bus *herald.Bus) *crier.Crier {
	if led == nil || lib == nil {
		return nil
	}
	return crier.New(led, lib, publisherFromBus(bus))
}

// drainLedgerToBus runs a one-shot Crier.Replay: every record on the
// Ledger past the current checkpoint is projected into the vault and
// republished onto bus. Used by short-lived CLI commands (mom record)
// that must see their write reflected on the bus before the process
// exits. Returns nil when there is nothing to drain.
func drainLedgerToBus(led *ledger.Ledger, lib *librarian.Librarian, bus *herald.Bus) error {
	c := newCrier(led, lib, bus)
	if c == nil {
		return nil
	}
	_, err := c.Replay()
	return err
}

// startCrierDaemon is the long-lived counterpart to drainLedgerToBus.
// It calls SkipBackfill once (so the existing op_events rows from the
// legacy bus path are not re-projected) and then runs Crier.Run on a
// goroutine until ctx is cancelled. The returned channel closes when
// the goroutine has exited, so callers can sequence "Crier-stopped"
// before "Drafter.FlushAll".
//
// A nil Crier returns an already-closed channel — the caller sees the
// same "no Crier" behaviour it would see today with no ledger.
func startCrierDaemon(ctx context.Context, c *crier.Crier) <-chan struct{} {
	done := make(chan struct{})
	if c == nil {
		close(done)
		return done
	}
	skipped, head, err := c.SkipBackfill()
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "crier: skip-backfill: %v\n", err)
	case skipped:
		fmt.Fprintf(os.Stderr, "crier: skipped backfill, checkpoint advanced to offset %d\n", head)
	}
	go func() {
		defer close(done)
		if err := c.Run(ctx, crierPollInterval); err != nil {
			fmt.Fprintf(os.Stderr, "crier: run stopped: %v\n", err)
		}
	}()
	return done
}

// writePipeline groups the handles every write surface shares with
// Crier: the Editor that appends to the central Ledger, the Herald
// bus subscribers (Drafter, Logbook) listen on, and the Crier that
// bridges Ledger → Herald. One per daemon process.
type writePipeline struct {
	ed    *editor.Editor
	bus   *herald.Bus
	crier *crier.Crier
}

// openWritePipeline is the v0.50 wiring entry point: it opens the
// central Ledger, builds the Editor (Ledger-only, no Herald), creates
// the bus with workers attached, and constructs the single shared
// Crier. Returns a pipeline with nil ed/crier when the Ledger or
// Librarian is unavailable; production callers check before using.
func openWritePipeline(workers centralWorkers) *writePipeline {
	led := openCentralLedger()
	bus := herald.NewBus()
	workers.AttachToBus(bus)
	p := &writePipeline{bus: bus}
	if led != nil {
		p.ed = editor.New(nil, nil).WithLedger(led)
	}
	p.crier = newCrier(led, workers.lib, bus)
	return p
}
