package autofold

import (
	"context"
	"errors"
	"time"

	"github.com/momhq/mom/services/projection"
)

// FoldFunc executes one project-wide fold pass. Production wires
// projection.RunProjectFold; tests inject a stub.
type FoldFunc func(ctx context.Context, projectID, root string) (projection.RunSummary, error)

// defaultPoll is how often the runner re-evaluates eligibility. Cheap
// (in-memory map scan), so a short period keeps the idle debounce
// responsive without meaningful cost.
const defaultPoll = 30 * time.Second

// Runner drives the auto-fold loop inside the global watch daemon: poll
// the Tracker, and for each eligible project run one fold. Folds are
// executed strictly sequentially across projects — the synthesizer
// shells out to the user's harness CLI, and parallel invocations would
// multiply quota burn and contend on the CLI.
type Runner struct {
	Tracker *Tracker
	// RootFor maps a project id to its project root (registry lookup).
	// Returning ok=false skips the project (no root — cannot fold).
	RootFor func(projectID string) (string, bool)
	Fold    FoldFunc
	Logf    func(format string, args ...any)
	// Poll overrides the eligibility poll period (tests). 0 = default.
	Poll time.Duration
}

// Run blocks until ctx is done, folding eligible projects as they come
// due. Call in its own goroutine.
func (r *Runner) Run(ctx context.Context) {
	poll := r.Poll
	if poll <= 0 {
		poll = defaultPoll
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.RunOnce(ctx)
		}
	}
}

// RunOnce evaluates eligibility and folds every currently-eligible
// project, sequentially. Exposed for tests and for a final drain.
func (r *Runner) RunOnce(ctx context.Context) {
	for _, c := range r.Tracker.Eligible() {
		if ctx.Err() != nil {
			return
		}
		r.foldOne(ctx, c)
	}
}

func (r *Runner) foldOne(ctx context.Context, c Candidate) {
	root, ok := r.RootFor(c.ProjectID)
	if !ok {
		// Not a registered project (e.g. attributed via per-turn cwd to a
		// project the daemon does not watch) — cannot fold without a root.
		// Disarm so it does not re-trigger every poll.
		r.Tracker.MarkSuccess(c.ProjectID)
		r.Logf("autofold %s: no registered root — skipping", c.ProjectID)
		return
	}

	r.Tracker.MarkAttempt(c.ProjectID)
	r.Logf("autofold %s: starting (%s, %d events pending)", c.ProjectID, c.Reason, c.Pending)

	start := time.Now()
	sum, err := r.Fold(ctx, c.ProjectID, root)
	elapsed := time.Since(start).Round(time.Second)

	switch {
	case errors.Is(err, projection.ErrFoldLocked):
		// A manual `mom vault fold` is running. Not a harness failure —
		// no backoff escalation, but respect min-interval before retrying.
		r.Logf("autofold %s: fold lock held (manual fold in progress) — will retry next window", c.ProjectID)
	case err != nil:
		delay := r.Tracker.MarkFailure(c.ProjectID)
		r.Logf("autofold %s: FAILED after %s: %v — backing off %s", c.ProjectID, elapsed, err, delay)
	case !sum.Complete():
		// Systemic harness trouble (usage limit, auth): the watermark
		// advanced as far as it could; back off before finishing the rest.
		delay := r.Tracker.MarkFailure(c.ProjectID)
		r.Logf("autofold %s: partial after %s (offsets %d→%d of %d, pending_synthesis=%v) — backing off %s",
			c.ProjectID, elapsed, sum.FromOffset, sum.FoldedThrough, sum.Head, sum.PendingSynthesis, delay)
	default:
		r.Tracker.MarkSuccess(c.ProjectID)
		r.Logf("autofold %s: completed in %s — %d events folded (offsets %d→%d), %d files written, engine %s",
			c.ProjectID, elapsed, sum.EventsFolded, sum.FromOffset, sum.FoldedThrough, sum.FilesWritten, sum.EngineName)
	}
}
