// Package crier projects canonical events from the Ledger (ADR 0021)
// into the Vault via Librarian (ADR 0022). Crier is the sole projector:
// archtests forbid it from importing storage/vault directly, and an
// archtest in #369 forbids services from reading the Ledger.
//
// Semantics are at-least-once + idempotent. Crier reads the Ledger
// from checkpoint+1, applies each event via librarian.ApplyLedgerEvent
// (which writes the projection and advances the checkpoint in a
// single transaction), and continues.
//
// The MVP exposes a one-shot Replay that drains the Ledger from
// checkpoint to head. A daemon-loop variant (Run-with-poll) is a
// follow-up — Replay is sufficient for #368 (replay tests) and for
// the v0.50 wire-up.
package crier

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/momhq/mom/storage/ledger"
	"github.com/momhq/mom/storage/librarian"
)

// Projector is the Librarian-shaped surface Crier needs. Implemented
// by storage/librarian.Librarian (production); a test double in
// crier_test.go covers idempotency and error paths.
type Projector interface {
	GetCrierCheckpoint() (librarian.CrierCheckpoint, error)
	SetCrierCheckpoint(offset int64) error
	ApplyLedgerEvent(offset int64, eventType, sessionID string, createdAt time.Time, payload map[string]any) (applied bool, err error)
}

// LedgerReader is the Ledger-shaped surface Crier needs. Implemented
// by storage/ledger.Ledger.
type LedgerReader interface {
	Iterate(from uint64) *ledger.Iter
	HeadOffset() (uint64, bool)
}

// Publisher is the live-fan-out surface Crier uses after each
// successful projection. Architecture v3: Crier is Herald's sole
// publisher; subscribers (Drafter, Logbook) react to Crier-published
// events. The interface is string-typed so events/crier does not have
// to import bus/herald (archtest invariant). Production wiring adapts
// *herald.Bus to this surface in ingress.
//
// A nil Publisher is accepted: Crier still projects to Librarian but
// performs no live fan-out. Used by tests that exercise projection
// without the bus, and by transitional builds.
type Publisher interface {
	Publish(eventType, sessionID string, ts time.Time, payload map[string]any)
}

// PublisherFunc adapts an ordinary function to the Publisher interface
// (http.HandlerFunc-style). Production wiring uses it to forward into
// *herald.Bus without defining an adapter struct at the call site:
//
//	pub := crier.PublisherFunc(func(t, s string, ts time.Time, p map[string]any) {
//	    bus.Publish(herald.Event{Type: herald.EventType(t), SessionID: s,
//	        Timestamp: ts, Payload: p})
//	})
type PublisherFunc func(eventType, sessionID string, ts time.Time, payload map[string]any)

// Publish calls f(eventType, sessionID, ts, payload).
func (f PublisherFunc) Publish(eventType, sessionID string, ts time.Time, payload map[string]any) {
	f(eventType, sessionID, ts, payload)
}

// Crier reads from a LedgerReader and projects into a Projector.
// Construct via New; call Replay to drain the Ledger up to its head.
type Crier struct {
	led LedgerReader
	pro Projector
	pub Publisher
}

// New constructs a Crier wired to led, pro, and pub. pub may be nil —
// in that case Crier projects to the Vault but does not republish onto
// Herald.
func New(led LedgerReader, pro Projector, pub Publisher) *Crier {
	return &Crier{led: led, pro: pro, pub: pub}
}

// Stats summarizes a Replay run.
type Stats struct {
	// Applied is the number of events that produced a NEW projection
	// (the row did not already exist).
	Applied int
	// Skipped is the number of events that were re-applied no-op-ly:
	// offset <= checkpoint, or the op_events UNIQUE index caught a
	// duplicate.
	Skipped int
	// LastOffset is the offset of the most recently processed event,
	// or the prior checkpoint when no events were available.
	LastOffset int64
}

// Replay drains the Ledger from checkpoint+1 to the current head and
// returns a Stats summary. Errors during a single projection abort
// the Replay (returning a partial Stats); the next Replay call
// resumes from the last successful checkpoint.
//
// Replay holds no exclusive lock on the Ledger or the Librarian —
// concurrent producers may continue appending while Crier reads.
// Iter.Next is safe against in-flight appends because each segment
// read uses its own file handle and stops at the first incomplete
// record.
func (c *Crier) Replay() (Stats, error) {
	if c.led == nil || c.pro == nil {
		return Stats{}, errors.New("crier: not wired (led=nil or pro=nil)")
	}
	cp, err := c.pro.GetCrierCheckpoint()
	if err != nil {
		return Stats{}, fmt.Errorf("crier: get checkpoint: %w", err)
	}
	stats := Stats{LastOffset: cp.Offset}
	startFrom := uint64(0)
	if cp.Offset >= 0 {
		startFrom = uint64(cp.Offset + 1)
	}
	it := c.led.Iterate(startFrom)
	defer it.Close()
	for {
		rec, ok := it.Next()
		if !ok {
			break
		}
		applied, err := c.pro.ApplyLedgerEvent(
			int64(rec.Offset),
			string(rec.Event.Type),
			rec.Event.SessionID,
			rec.AppendedAt,
			rec.Event.Payload,
		)
		if err != nil {
			return stats, fmt.Errorf("crier: apply offset %d: %w", rec.Offset, err)
		}
		if applied {
			stats.Applied++
			// Live fan-out (architecture v3): Crier republishes the
			// canonical event onto Herald so bus subscribers (Drafter,
			// Logbook) react. Guarded by applied so an idempotent
			// re-Replay does not duplicate-publish — subscribers see
			// each event exactly once per Crier-side projection.
			if c.pub != nil {
				c.pub.Publish(
					string(rec.Event.Type),
					rec.Event.SessionID,
					rec.AppendedAt,
					rec.Event.Payload,
				)
			}
		} else {
			stats.Skipped++
		}
		stats.LastOffset = int64(rec.Offset)
	}
	if err := it.Err(); err != nil {
		return stats, fmt.Errorf("crier: iterate: %w", err)
	}
	return stats, nil
}

// SkipBackfill advances the checkpoint to the current Ledger head when
// (and only when) the checkpoint is -1 (the fresh-install marker). It
// is the v0.50 cutover path: on production deployments the op_events
// table already contains rows projected via the legacy bus path
// (ledger_offset NULL). Re-projecting them on Crier's first run would
// create duplicates. Returns (skipped, head, err):
//
//   - skipped=true, head=N: checkpoint was -1; advanced to head N.
//   - skipped=false, head=-1: checkpoint was already advanced, no-op.
//   - skipped=false, head=-1, err set: real failure.
//
// Safe to call from any startup path before Run/Replay. Callers
// holding a non-empty checkpoint never have their state rolled back.
func (c *Crier) SkipBackfill() (skipped bool, head int64, err error) {
	if c.led == nil || c.pro == nil {
		return false, -1, errors.New("crier: not wired (led=nil or pro=nil)")
	}
	cp, err := c.pro.GetCrierCheckpoint()
	if err != nil {
		return false, -1, fmt.Errorf("crier: get checkpoint: %w", err)
	}
	if cp.Offset >= 0 {
		// Already past the fresh marker — keep moving forward via Replay.
		return false, -1, nil
	}
	headOffset, ok := c.led.HeadOffset()
	if !ok {
		// Ledger empty; nothing to skip over. Leave checkpoint at -1
		// so the first appended record (offset 0) is projected normally.
		return false, -1, nil
	}
	if err := c.pro.SetCrierCheckpoint(int64(headOffset)); err != nil {
		return false, -1, fmt.Errorf("crier: set checkpoint: %w", err)
	}
	return true, int64(headOffset), nil
}

// Run drives Crier as a long-lived daemon: it calls Replay on a
// poll interval until ctx is cancelled. Returns nil on clean shutdown
// (context.Canceled is normalised to nil). A Replay error propagates
// and stops the loop — the caller decides whether to restart.
//
// interval is clamped to a 1 ms floor; 0 means "use a sensible default
// of 50 ms" so callers that pass an untouched zero-value still get a
// responsive loop. Production wiring picks an interval that trades
// freshness for syscall load.
func (c *Crier) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 50 * time.Millisecond
	}
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	t := time.NewTimer(0) // fire immediately on the first iteration
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
		if _, err := c.Replay(); err != nil {
			return err
		}
		t.Reset(interval)
	}
}
