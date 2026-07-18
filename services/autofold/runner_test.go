package autofold

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/momhq/mom/services/projection"
)

func discardLogf(string, ...any) {}

// A successful fold marks the project folded (disarmed) and logs
// completion.
func TestRunner_SuccessfulFoldDisarms(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)
	tr.Observe("mom-os", "momos", 3)
	clk.advance(10 * time.Minute)

	var folded []string
	r := &Runner{
		Tracker: tr,
		RootFor: func(id string) (string, bool) { return "/tmp/" + id, true },
		Fold: func(_ context.Context, projectID, root string) (projection.RunSummary, error) {
			folded = append(folded, projectID+"@"+root)
			return projection.RunSummary{EventsFolded: 3, FoldedThrough: 10, Head: 10}, nil
		},
		Logf: discardLogf,
	}
	r.RunOnce(context.Background())

	if len(folded) != 1 || folded[0] != "mom-os@/tmp/mom-os" {
		t.Fatalf("folded = %v, want one fold of mom-os at its root", folded)
	}
	clk.advance(2 * time.Hour)
	if got := eligibleIDs(tr); len(got) != 0 {
		t.Fatalf("eligible after successful fold = %v, want none (disarmed)", got)
	}
}

// A failed fold backs off: the project does not re-trigger within the
// backoff window, and does re-trigger after it.
func TestRunner_FailedFoldBacksOff(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)
	tr.Observe("mom-os", "momos", 1)
	clk.advance(10 * time.Minute)

	calls := 0
	r := &Runner{
		Tracker: tr,
		RootFor: func(id string) (string, bool) { return "/tmp/" + id, true },
		Fold: func(context.Context, string, string) (projection.RunSummary, error) {
			calls++
			return projection.RunSummary{}, fmt.Errorf("claude CLI exited 1: usage limit reached")
		},
		Logf: discardLogf,
	}

	r.RunOnce(context.Background())
	if calls != 1 {
		t.Fatalf("fold calls = %d, want 1", calls)
	}

	// Immediately after failure: not eligible (backoff 30m).
	r.RunOnce(context.Background())
	if calls != 1 {
		t.Fatalf("fold calls after immediate re-poll = %d, want 1 (no retry loop on CLI failure)", calls)
	}

	clk.advance(31 * time.Minute)
	r.RunOnce(context.Background())
	if calls != 2 {
		t.Fatalf("fold calls after backoff window = %d, want 2 (retry on next eligibility window)", calls)
	}

	// Second consecutive failure doubles the backoff: 60m.
	clk.advance(45 * time.Minute)
	r.RunOnce(context.Background())
	if calls != 2 {
		t.Fatalf("fold calls at 45m after second failure = %d, want 2 (backoff doubled to 60m)", calls)
	}
	clk.advance(20 * time.Minute)
	r.RunOnce(context.Background())
	if calls != 3 {
		t.Fatalf("fold calls after doubled backoff = %d, want 3", calls)
	}
}

// A partial fold (systemic harness trouble stopped the watermark short)
// is treated like a failure for backoff purposes.
func TestRunner_PartialFoldBacksOff(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)
	tr.Observe("mom-os", "momos", 1)
	clk.advance(10 * time.Minute)

	calls := 0
	r := &Runner{
		Tracker: tr,
		RootFor: func(id string) (string, bool) { return "/tmp/" + id, true },
		Fold: func(context.Context, string, string) (projection.RunSummary, error) {
			calls++
			return projection.RunSummary{FoldedThrough: 5, Head: 10, Partial: true}, nil
		},
		Logf: discardLogf,
	}
	r.RunOnce(context.Background())
	r.RunOnce(context.Background())
	if calls != 1 {
		t.Fatalf("fold calls = %d, want 1 (partial fold must back off, not retry-loop)", calls)
	}
}

// A fold blocked by the per-project lock (manual `mom vault fold` in
// flight) is NOT a harness failure: no backoff escalation; the runner
// retries on the next eligibility window after min-interval.
func TestRunner_LockContentionRetriesWithoutEscalation(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)
	tr.Observe("mom-os", "momos", 1)
	clk.advance(10 * time.Minute)

	locked := true
	calls := 0
	r := &Runner{
		Tracker: tr,
		RootFor: func(id string) (string, bool) { return "/tmp/" + id, true },
		Fold: func(context.Context, string, string) (projection.RunSummary, error) {
			calls++
			if locked {
				return projection.RunSummary{}, projection.ErrFoldLocked
			}
			return projection.RunSummary{FoldedThrough: 10, Head: 10}, nil
		},
		Logf: discardLogf,
	}

	r.RunOnce(context.Background())
	if calls != 1 {
		t.Fatalf("fold calls = %d, want 1", calls)
	}

	// Min interval still applies (MarkAttempt was stamped)…
	clk.advance(10 * time.Minute)
	r.RunOnce(context.Background())
	if calls != 1 {
		t.Fatalf("fold calls within min-interval = %d, want 1", calls)
	}
	// …but no exponential escalation: eligible right after min-interval.
	locked = false
	clk.advance(21 * time.Minute)
	r.RunOnce(context.Background())
	if calls != 2 {
		t.Fatalf("fold calls after min-interval = %d, want 2 (lock contention must not escalate backoff)", calls)
	}
	if got := errors.Is(projection.ErrFoldLocked, projection.ErrFoldLocked); !got {
		t.Fatal("sanity: ErrFoldLocked identity")
	}
}

// A project with no registered root cannot fold; it is disarmed instead
// of re-triggering every poll.
func TestRunner_UnregisteredProjectDisarmed(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)
	tr.Observe("stray", "momos", 1)
	clk.advance(10 * time.Minute)

	calls := 0
	r := &Runner{
		Tracker: tr,
		RootFor: func(string) (string, bool) { return "", false },
		Fold: func(context.Context, string, string) (projection.RunSummary, error) {
			calls++
			return projection.RunSummary{}, nil
		},
		Logf: discardLogf,
	}
	r.RunOnce(context.Background())
	r.RunOnce(context.Background())
	if calls != 0 {
		t.Fatalf("fold calls = %d, want 0 (no root, nothing to fold)", calls)
	}
	if got := eligibleIDs(tr); len(got) != 0 {
		t.Fatalf("eligible = %v, want none (disarmed)", got)
	}
}

// Multiple eligible projects fold sequentially in one RunOnce pass —
// never in parallel (CLI quota).
func TestRunner_FoldsSequentially(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)
	tr.Observe("a", "momos", 1)
	tr.Observe("b", "momos", 1)
	clk.advance(10 * time.Minute)

	inFlight := 0
	maxInFlight := 0
	calls := 0
	r := &Runner{
		Tracker: tr,
		RootFor: func(id string) (string, bool) { return "/tmp/" + id, true },
		Fold: func(context.Context, string, string) (projection.RunSummary, error) {
			inFlight++
			if inFlight > maxInFlight {
				maxInFlight = inFlight
			}
			calls++
			inFlight--
			return projection.RunSummary{}, nil
		},
		Logf: discardLogf,
	}
	r.RunOnce(context.Background())
	if calls != 2 {
		t.Fatalf("fold calls = %d, want 2", calls)
	}
	if maxInFlight != 1 {
		t.Fatalf("max in-flight folds = %d, want 1 (strictly sequential)", maxInFlight)
	}
}
