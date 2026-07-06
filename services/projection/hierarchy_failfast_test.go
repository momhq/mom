package projection

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// failingSynth fails every L0 call whose window contains an offset in bad;
// other calls behave like icmStub's L0 branch. L1/L2 always succeed.
type failingSynth struct {
	bad   map[uint64]bool
	calls int
}

func (s *failingSynth) Fold(_ context.Context, in FoldInput) (FoldResult, error) {
	s.calls++
	files := map[string]string{}
	switch {
	case has(in.Existing, "_l0_hint"):
		offs := make([]uint64, len(in.Events))
		for i, e := range in.Events {
			if s.bad[e.Offset] {
				return FoldResult{}, errors.New("synthetic failure")
			}
			offs[i] = e.Offset
		}
		cid := chunkID(in.ProjectID, offs)
		files["episodes/"+cid+".md"] = PrependFrontmatter(
			Frontmatter{Type: typeEpisode, Level: 0, Version: 1, Sources: offs, Tags: []string{"arch"}}, "# Episode\n")
	case has(in.Existing, "_l1_hint"):
		files[referenceDir+"/arch.md"] = PrependFrontmatter(
			Frontmatter{Type: typeReference, Name: "arch", Level: 1, Version: 1}, "# arch\n")
	case has(in.Existing, "_l2_hint"):
		files[identityFile] = PrependFrontmatter(
			Frontmatter{Type: typeIdentity, Name: "Demo", Level: 2, Version: 1}, "# Demo\n")
	}
	return FoldResult{Files: files}, nil
}

// TestFoldHierarchicalStopsWatermarkAtFailure guards the no-fallback contract:
// when an L0 window keeps failing even after bisection, the fold stops and
// FoldedThrough points at the end of the last consecutive successful window —
// never past the failed events.
func TestFoldHierarchicalStopsWatermarkAtFailure(t *testing.T) {
	var events []FoldEvent
	for i := 1; i <= 60; i++ {
		events = append(events, makeMemoryEvent(uint64(i), "s", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), fmt.Sprintf("m%d", i), nil))
	}
	// Offset 25 poisons every window containing it, at every bisection depth.
	stub := &failingSynth{bad: map[uint64]bool{25: true}}
	hs := &HierarchySynth{inner: stub, l1Threshold: 1000, l2Threshold: 1000}

	res, err := FoldHierarchical(context.Background(), hs, FoldInput{ProjectID: "demo", Events: events, ToOffset: 60}, 20)
	if err != nil {
		t.Fatal(err)
	}

	// Chunk 1 (1–20) succeeds; chunk 2 (21–40) fails, bisects to (21–30 fails,
	// too small to split at min 10)... the consecutive frontier stays at 20.
	if res.FoldedThrough != 20 {
		t.Errorf("want FoldedThrough=20 (last consecutive success), got %d", res.FoldedThrough)
	}
	// Nothing past the failure may be synthesized (fail-fast, no gaps).
	for p, c := range res.Files {
		if !strings.HasPrefix(p, "episodes/") {
			continue
		}
		fm, _ := ParseFrontmatter(c)
		for _, o := range fm.Sources {
			if o > 20 {
				t.Errorf("episode %s covers offset %d beyond the failed window", p, o)
			}
		}
	}
}

// TestFoldHierarchicalBisectRecoversPartialWindow verifies bisection: a window
// whose failure is caused by its size (fails at 20, succeeds at 10) is split
// and both halves are folded — no events lost, full watermark reached.
func TestFoldHierarchicalBisectRecoversPartialWindow(t *testing.T) {
	var events []FoldEvent
	for i := 1; i <= 20; i++ {
		events = append(events, makeMemoryEvent(uint64(i), "s", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), fmt.Sprintf("m%d", i), nil))
	}
	stub := &sizeLimitedSynth{maxEvents: 10}
	hs := &HierarchySynth{inner: stub, l1Threshold: 1000, l2Threshold: 1000}

	res, err := FoldHierarchical(context.Background(), hs, FoldInput{ProjectID: "demo", Events: events, ToOffset: 20}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.FoldedThrough != 20 {
		t.Errorf("want FoldedThrough=20 after bisection, got %d", res.FoldedThrough)
	}
	episodes := 0
	for p := range res.Files {
		if strings.HasPrefix(p, "episodes/") {
			episodes++
		}
	}
	if episodes != 2 {
		t.Errorf("want 2 episodes (one per bisected half), got %d", episodes)
	}
}

// sizeLimitedSynth fails any L0 window larger than maxEvents — simulating a
// window that is too big for one synthesis call.
type sizeLimitedSynth struct{ maxEvents int }

func (s *sizeLimitedSynth) Fold(_ context.Context, in FoldInput) (FoldResult, error) {
	if !has(in.Existing, "_l0_hint") {
		return FoldResult{Files: map[string]string{}}, nil
	}
	if len(in.Events) > s.maxEvents {
		return FoldResult{}, errors.New("window too large")
	}
	offs := make([]uint64, len(in.Events))
	for i, e := range in.Events {
		offs[i] = e.Offset
	}
	cid := chunkID(in.ProjectID, offs)
	return FoldResult{Files: map[string]string{
		"episodes/" + cid + ".md": PrependFrontmatter(
			Frontmatter{Type: typeEpisode, Level: 0, Version: 1, Sources: offs, Tags: []string{"arch"}}, "# Episode\n"),
	}}, nil
}

// TestFoldHierarchicalL1SkipsUnchangedSubjects guards incremental L1: a second
// fold over the same vault (no new events for a subject) must not re-call L1
// for that subject, because the concept file's content-addressed id already
// matches its episode set.
func TestFoldHierarchicalL1SkipsUnchangedSubjects(t *testing.T) {
	var events []FoldEvent
	for i := 1; i <= 10; i++ {
		events = append(events, makeMemoryEvent(uint64(i), "s", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "m", nil))
	}
	stub := &refStub{}
	hs := &HierarchySynth{inner: stub, l1Threshold: 5, l2Threshold: 1}

	res1, err := FoldHierarchical(context.Background(), hs, FoldInput{ProjectID: "demo", Events: events, ToOffset: 10}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stub.l1 == 0 {
		t.Fatal("first fold produced no L1 calls")
	}

	// Second fold: same vault, no new events.
	l1Before, l2Before := stub.l1, stub.l2
	_, err = FoldHierarchical(context.Background(), hs, FoldInput{
		ProjectID:      "demo",
		Existing:       res1.Files,
		ExistingChunks: res1.Chunks,
		ToOffset:       10,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stub.l1 != l1Before {
		t.Errorf("second fold re-ran L1 for unchanged subjects: %d → %d calls", l1Before, stub.l1)
	}
	if stub.l2 != l2Before {
		t.Errorf("second fold re-ran L2 with no L1 change: %d → %d calls", l2Before, stub.l2)
	}
}

// refStub writes the subject's concept at its slug path (reference/arch.md),
// the way the L1 prompt instructs a real model to.
type refStub struct{ icmStub }

func (s *refStub) Fold(ctx context.Context, in FoldInput) (FoldResult, error) {
	if has(in.Existing, "_l1_hint") {
		s.l1++
		return FoldResult{Files: map[string]string{
			referenceDir + "/arch.md": PrependFrontmatter(
				Frontmatter{Type: typeReference, Name: "arch", Level: 1, Version: 1}, "# arch\n"),
		}}, nil
	}
	return s.icmStub.Fold(ctx, in)
}

// contractStub classifies its single subject as a contract, exercising the
// contracts/ path end-to-end.
type contractStub struct{ icmStub }

func (s *contractStub) Fold(ctx context.Context, in FoldInput) (FoldResult, error) {
	if has(in.Existing, "_l1_hint") {
		s.l1++
		return FoldResult{Files: map[string]string{
			contractsDir + "/arch.md": PrependFrontmatter(
				Frontmatter{Type: typeContract, Name: "arch", Level: 1, Version: 1}, "# arch\n"),
		}}, nil
	}
	return s.icmStub.Fold(ctx, in)
}

// TestFoldHierarchicalContractClassification verifies a subject the model
// classifies as a contract lands in contracts/, is indexed, and is skipped as
// unchanged on the next fold (the skip check covers both folders).
func TestFoldHierarchicalContractClassification(t *testing.T) {
	var events []FoldEvent
	for i := 1; i <= 10; i++ {
		events = append(events, makeMemoryEvent(uint64(i), "s", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "m", nil))
	}
	stub := &contractStub{}
	hs := &HierarchySynth{inner: stub, l1Threshold: 5, l2Threshold: 1000}

	res1, err := FoldHierarchical(context.Background(), hs, FoldInput{ProjectID: "demo", Events: events, ToOffset: 10}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res1.Files[contractsDir+"/arch.md"]; !ok {
		t.Fatalf("contract concept not written; files: %v", fileKeys(res1.Files))
	}
	if _, ok := res1.Files[contractsDir+"/"+indexFileName]; !ok {
		t.Errorf("contracts folder missing its OKF index")
	}

	l1Before := stub.l1
	_, err = FoldHierarchical(context.Background(), hs, FoldInput{
		ProjectID:      "demo",
		Existing:       res1.Files,
		ExistingChunks: res1.Chunks,
		ToOffset:       10,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stub.l1 != l1Before {
		t.Errorf("unchanged contract subject was re-synthesized: %d → %d calls", l1Before, stub.l1)
	}
}

// systemicSynth succeeds for the first okCalls L0 windows, then fails every
// call with an InvokeError — simulating a usage limit hit mid-fold.
type systemicSynth struct {
	okCalls int
	calls   int
}

func (s *systemicSynth) Fold(_ context.Context, in FoldInput) (FoldResult, error) {
	if !has(in.Existing, "_l0_hint") {
		// Any L1/L2 call during the outage is a bug — fail loudly.
		s.calls++
		return FoldResult{}, &InvokeError{Err: errors.New("usage limit reached")}
	}
	s.calls++
	if s.calls > s.okCalls {
		return FoldResult{}, &InvokeError{Err: errors.New("usage limit reached")}
	}
	offs := make([]uint64, len(in.Events))
	for i, e := range in.Events {
		offs[i] = e.Offset
	}
	cid := chunkID(in.ProjectID, offs)
	return FoldResult{Files: map[string]string{
		"episodes/" + cid + ".md": PrependFrontmatter(
			Frontmatter{Type: typeEpisode, Level: 0, Version: 1, Sources: offs, Tags: []string{"arch"}}, "# Episode\n"),
	}}, nil
}

// TestFoldHierarchicalSystemicAbort guards the circuit breaker: a harness
// process failure (usage limit) aborts the fold WITHOUT bisecting the window
// or attempting L1/L2 — one failing call, not dozens of doomed ones.
func TestFoldHierarchicalSystemicAbort(t *testing.T) {
	var events []FoldEvent
	for i := 1; i <= 40; i++ {
		events = append(events, makeMemoryEvent(uint64(i), "s", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "m", nil))
	}
	stub := &systemicSynth{okCalls: 1}
	hs := &HierarchySynth{inner: stub, l1Threshold: 1, l2Threshold: 1}

	res, err := FoldHierarchical(context.Background(), hs, FoldInput{ProjectID: "demo", Events: events, ToOffset: 40}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.FoldedThrough != 20 {
		t.Errorf("want FoldedThrough=20, got %d", res.FoldedThrough)
	}
	// Exactly 2 calls: the successful chunk 1 and the single failed chunk 2.
	// No bisection retries, no L1, no L2 during the outage.
	if stub.calls != 2 {
		t.Errorf("want 2 synthesis calls (success + single systemic failure), got %d", stub.calls)
	}
}

func fileKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
