package autofold

import (
	"testing"
	"time"
)

// fakeClock is a manually-advanced clock for deterministic eligibility
// tests.
type fakeClock struct {
	t time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time              { return c.t }
func (c *fakeClock) advance(d time.Duration)     { c.t = c.t.Add(d) }

func testPolicy() Policy {
	return Policy{
		Harnesses:   []string{"momos"},
		Idle:        10 * time.Minute,
		Backlog:     200,
		MinInterval: 30 * time.Minute,
	}
}

func eligibleIDs(t *Tracker) []string {
	var out []string
	for _, c := range t.Eligible() {
		out = append(out, c.ProjectID)
	}
	return out
}

// Enabled-harness activity arms a project; after the idle window it is
// eligible.
func TestTracker_MomosActivityTriggersAfterIdle(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)

	tr.Observe("mom-os", "momos", 3)

	if got := eligibleIDs(tr); len(got) != 0 {
		t.Fatalf("eligible immediately after activity = %v, want none (idle debounce)", got)
	}
	clk.advance(9 * time.Minute)
	if got := eligibleIDs(tr); len(got) != 0 {
		t.Fatalf("eligible at 9m idle = %v, want none (threshold is 10m)", got)
	}
	clk.advance(1 * time.Minute)
	cands := tr.Eligible()
	if len(cands) != 1 || cands[0].ProjectID != "mom-os" {
		t.Fatalf("eligible at 10m idle = %v, want [mom-os]", cands)
	}
	if cands[0].Reason != "idle" {
		t.Errorf("Reason = %q, want idle", cands[0].Reason)
	}
}

// Activity only from harnesses OUTSIDE the allowlist never arms the
// project — the per-harness gate is the point of M2.
func TestTracker_ClaudeOnlyActivityDoesNotTrigger(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)

	tr.Observe("logbook", "claude-code", 500)
	clk.advance(2 * time.Hour)

	if got := eligibleIDs(tr); len(got) != 0 {
		t.Fatalf("eligible = %v, want none (claude-code is not an enabled trigger harness)", got)
	}
}

// One enabled-harness event arms the project; the backlog/idle math then
// counts ALL harnesses' events — the fold is project-wide by design.
func TestTracker_MixedActivityArmsViaEnabledHarness(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)

	tr.Observe("mom-os", "claude-code", 150)
	tr.Observe("mom-os", "momos", 1)
	clk.advance(10 * time.Minute)

	cands := tr.Eligible()
	if len(cands) != 1 {
		t.Fatalf("eligible = %v, want mom-os (armed by one momos event)", cands)
	}
	if cands[0].Pending != 151 {
		t.Errorf("Pending = %d, want 151 (all harnesses counted)", cands[0].Pending)
	}
}

// The backlog cap fires without waiting for idleness.
func TestTracker_BacklogTriggersWithoutIdle(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)

	tr.Observe("mom-os", "momos", 200)
	clk.advance(time.Second) // still far from idle

	cands := tr.Eligible()
	if len(cands) != 1 {
		t.Fatalf("eligible = %v, want mom-os (backlog >= 200)", cands)
	}
	if cands[0].Reason != "backlog" {
		t.Errorf("Reason = %q, want backlog", cands[0].Reason)
	}
}

// A successful fold consumes pending work and disarms the project until
// new enabled-harness activity arrives.
func TestTracker_SuccessDisarmsUntilNewActivity(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)

	tr.Observe("mom-os", "momos", 5)
	clk.advance(10 * time.Minute)
	tr.MarkAttempt("mom-os")
	tr.MarkSuccess("mom-os")

	clk.advance(2 * time.Hour)
	if got := eligibleIDs(tr); len(got) != 0 {
		t.Fatalf("eligible after success with no new activity = %v, want none", got)
	}

	// New momos activity re-arms; eligible after idle + min-interval.
	tr.Observe("mom-os", "momos", 1)
	clk.advance(10 * time.Minute)
	if got := eligibleIDs(tr); len(got) != 1 {
		t.Fatalf("eligible after re-arm = %v, want [mom-os]", got)
	}
}

// The min interval spaces fold attempts even when the project stays
// eligible (quota protection).
func TestTracker_MinIntervalSpacesAttempts(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)

	tr.Observe("mom-os", "momos", 1)
	clk.advance(10 * time.Minute)
	tr.MarkAttempt("mom-os")
	tr.MarkSuccess("mom-os")

	// New activity right away; idle passes at +10m, but the attempt was
	// only 10m ago — blocked until 30m since the attempt.
	tr.Observe("mom-os", "momos", 1)
	clk.advance(10 * time.Minute)
	if got := eligibleIDs(tr); len(got) != 0 {
		t.Fatalf("eligible 20m after last attempt = %v, want none (min interval 30m)", got)
	}
	clk.advance(20 * time.Minute) // now 30m since the attempt
	if got := eligibleIDs(tr); len(got) != 1 {
		t.Fatalf("eligible 30m after last attempt = %v, want [mom-os]", got)
	}
}

// Consecutive failures back off exponentially (1x, 2x, 4x, 8x, 16x the
// min interval, capped) and a success resets the ladder.
func TestTracker_FailureBackoffEscalatesAndResets(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)
	mi := testPolicy().MinInterval

	wants := []time.Duration{mi, 2 * mi, 4 * mi, 8 * mi, 16 * mi, 16 * mi}
	for i, want := range wants {
		if got := tr.MarkFailure("mom-os"); got != want {
			t.Fatalf("failure %d backoff = %s, want %s", i+1, got, want)
		}
	}

	tr.MarkSuccess("mom-os")
	if got := tr.MarkFailure("mom-os"); got != mi {
		t.Errorf("backoff after success reset = %s, want %s (ladder must reset)", got, mi)
	}
}

// While backed off, the project is not eligible even though idle/backlog
// conditions hold; after the backoff window it is again.
func TestTracker_BackoffGatesEligibility(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)

	tr.Observe("mom-os", "momos", 1)
	clk.advance(10 * time.Minute)
	tr.MarkAttempt("mom-os")
	delay := tr.MarkFailure("mom-os") // 30m backoff

	clk.advance(delay - time.Minute)
	if got := eligibleIDs(tr); len(got) != 0 {
		t.Fatalf("eligible during backoff = %v, want none", got)
	}
	clk.advance(2 * time.Minute)
	if got := eligibleIDs(tr); len(got) != 1 {
		t.Fatalf("eligible after backoff = %v, want [mom-os] (pending state kept across failures)", got)
	}
}

// Projects are tracked independently.
func TestTracker_ProjectsIndependent(t *testing.T) {
	clk := newFakeClock()
	tr := NewTracker(testPolicy(), clk.now)

	tr.Observe("mom-os", "momos", 1)
	tr.Observe("logbook", "claude-code", 1)
	clk.advance(10 * time.Minute)

	got := eligibleIDs(tr)
	if len(got) != 1 || got[0] != "mom-os" {
		t.Fatalf("eligible = %v, want only mom-os", got)
	}
}
