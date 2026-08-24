// Package autofold implements the global watch daemon's automatic vault
// folding (M2): the daemon observes ingestion activity per project and,
// when a project has pending work from an enabled harness and has gone
// idle (or accumulated a large backlog), runs the same in-process fold
// as `mom vault fold` (projection.RunProjectFold).
//
// Harness gating applies to the TRIGGER only: activity from a harness
// outside the allowlist never starts a fold, but a fold pass itself is
// project-wide and synthesizes whatever pending events exist, whichever
// harness produced them. That is intended.
package autofold

import (
	"sync"
	"time"
)

// Policy is the eligibility policy (materialized from config defaults —
// see shared/config.AutofoldConfig).
type Policy struct {
	// Harnesses is the trigger allowlist. Only ingestion attributed to
	// one of these harness names arms a project.
	Harnesses []string
	// Idle is the quiet period after the last ingested event before a
	// fold fires.
	Idle time.Duration
	// Backlog triggers a fold regardless of idleness once this many
	// events were ingested since the last successful fold.
	Backlog int
	// MinInterval is the minimum spacing between fold attempts for the
	// same project (quota protection — folds shell out to the user's
	// harness CLI).
	MinInterval time.Duration
}

// maxBackoffShift caps the exponential failure backoff at
// MinInterval * 2^maxBackoffShift (16x).
const maxBackoffShift = 4

// projectState is the per-project trigger state.
type projectState struct {
	lastEvent   time.Time // last ingested event (any harness)
	pending     int       // events ingested since the last successful fold
	armed       bool      // pending activity from an enabled harness exists
	lastAttempt time.Time // last fold attempt (success or failure)
	failures    int       // consecutive fold failures
	notBefore   time.Time // failure backoff gate
}

// Tracker is the eligibility/debounce state machine. It is clock-injected
// for tests and safe for concurrent use (the watcher publishes from
// multiple goroutines; the fold loop polls).
type Tracker struct {
	mu       sync.Mutex
	policy   Policy
	now      func() time.Time
	enabled  map[string]bool
	projects map[string]*projectState
}

// NewTracker builds a Tracker. now may be nil (wall clock).
func NewTracker(policy Policy, now func() time.Time) *Tracker {
	if now == nil {
		now = time.Now
	}
	enabled := make(map[string]bool, len(policy.Harnesses))
	for _, h := range policy.Harnesses {
		enabled[h] = true
	}
	return &Tracker{
		policy:   policy,
		now:      now,
		enabled:  enabled,
		projects: map[string]*projectState{},
	}
}

// Observe records n ingested events for a project, attributed to a
// harness. Called by the watcher's publish hook for every turn and
// session event appended to the Ledger.
func (t *Tracker) Observe(projectID, harness string, n int) {
	if projectID == "" || n <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.state(projectID)
	st.lastEvent = t.now()
	st.pending += n
	if t.enabled[harness] {
		st.armed = true
	}
}

// Eligible returns the projects that should fold now, with the reason
// ("idle" or "backlog") for the log line. A project is eligible when:
//
//	armed (pending activity from an enabled harness)
//	AND (idle >= policy.Idle OR pending >= policy.Backlog)
//	AND last attempt was >= policy.MinInterval ago
//	AND the failure backoff window has passed
func (t *Tracker) Eligible() []Candidate {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	var out []Candidate
	for id, st := range t.projects {
		if !st.armed {
			continue
		}
		reason := ""
		switch {
		case st.pending >= t.policy.Backlog:
			reason = "backlog"
		case now.Sub(st.lastEvent) >= t.policy.Idle:
			reason = "idle"
		default:
			continue
		}
		if !st.lastAttempt.IsZero() && now.Sub(st.lastAttempt) < t.policy.MinInterval {
			continue
		}
		if now.Before(st.notBefore) {
			continue
		}
		out = append(out, Candidate{ProjectID: id, Reason: reason, Pending: st.pending})
	}
	return out
}

// Candidate is one fold-eligible project.
type Candidate struct {
	ProjectID string
	Reason    string // "idle" | "backlog"
	Pending   int
}

// MarkAttempt records that a fold attempt started for a project. It
// stamps the min-interval clock regardless of the outcome.
func (t *Tracker) MarkAttempt(projectID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state(projectID).lastAttempt = t.now()
}

// MarkSuccess records a fully-completed fold: pending work is consumed,
// the project disarms until new enabled-harness activity arrives, and
// the failure backoff resets.
func (t *Tracker) MarkSuccess(projectID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.state(projectID)
	st.pending = 0
	st.armed = false
	st.failures = 0
	st.notBefore = time.Time{}
}

// MarkFailure records a failed (or partial/systemic) fold and returns the
// backoff delay applied: MinInterval * 2^(failures-1), capped at
// MinInterval * maxBackoffFactor. Pending state is kept so the project
// re-triggers after the backoff — retry happens only on the next
// eligibility window, never in a tight loop (the synthesizer shells out
// to the user's harness CLI; hammering a logged-out or rate-limited CLI
// helps no one).
func (t *Tracker) MarkFailure(projectID string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.state(projectID)
	st.failures++
	shift := st.failures - 1
	if shift > maxBackoffShift { // caps the delay; guards shift overflow too
		shift = maxBackoffShift
	}
	delay := t.policy.MinInterval * time.Duration(1<<shift)
	st.notBefore = t.now().Add(delay)
	return delay
}

func (t *Tracker) state(projectID string) *projectState {
	st, ok := t.projects[projectID]
	if !ok {
		st = &projectState{}
		t.projects[projectID] = st
	}
	return st
}
