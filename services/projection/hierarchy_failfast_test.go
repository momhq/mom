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

// multiSubjectL0Stub emits episodes whose tags rotate across subjectTags so
// collectSubjects produces len(subjectTags) subjects (each appears on every
// episode, satisfying l1SubjectMinEpisodes easily).
type multiSubjectL0Stub struct {
	subjectTags []string
	l0, l1, l2  int
}

func (s *multiSubjectL0Stub) Fold(_ context.Context, in FoldInput) (FoldResult, error) {
	files := map[string]string{}
	switch {
	case has(in.Existing, "_l0_hint"):
		s.l0++
		offs := make([]uint64, len(in.Events))
		for i, e := range in.Events {
			offs[i] = e.Offset
		}
		cid := chunkID(in.ProjectID, offs)
		// Tag every episode with ALL subject tags so each subject has enough episodes.
		files[episodesDir+"/"+cid+".md"] = PrependFrontmatter(
			Frontmatter{Type: typeEpisode, Level: 0, Version: 1, Sources: offs, Tags: s.subjectTags},
			"# Episode\n")
	case has(in.Existing, "_l1_hint"):
		s.l1++
		// If an existing concept path is present in Existing (update-in-place),
		// use it. Otherwise extract the slug from the _l1_hint text.
		for k := range in.Existing {
			if (strings.HasPrefix(k, referenceDir+"/") || strings.HasPrefix(k, contractsDir+"/")) &&
				!strings.HasSuffix(k, "/"+indexFileName) {
				files[k] = PrependFrontmatter(
					Frontmatter{Type: typeReference, Level: 1, Version: 1}, "# concept\n")
				return FoldResult{Files: files}, nil
			}
		}
		// New subject: derive slug from the hint text.
		slug := subjectSlugFromHint(in.Existing["_l1_hint"])
		files[referenceDir+"/"+slug+".md"] = PrependFrontmatter(
			Frontmatter{Type: typeReference, Level: 1, Version: 1}, "# concept\n")
	case has(in.Existing, "_l2_hint"):
		s.l2++
		files[identityFile] = PrependFrontmatter(
			Frontmatter{Type: typeIdentity, Level: 2, Version: 1}, "# identity\n")
	}
	return FoldResult{Files: files}, nil
}

// subjectSlugFromHint extracts the subject slug from the _l1_hint text, which
// contains `Write EXACTLY ONE file about the subject "<name>".`. Used by the
// stub to infer which subject an L1 call is about when all episodes share the
// same tags.
func subjectSlugFromHint(hint string) string {
	const marker = `about the subject "`
	idx := strings.Index(hint, marker)
	if idx < 0 {
		return "unknown"
	}
	rest := hint[idx+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return "unknown"
	}
	name := rest[:end]
	return strings.ReplaceAll(name, " ", "-")
}

// l1AbortAfterNStub fails the (n+1)th L1 call with a systemic InvokeError.
// L0 and L2 always succeed (via multiSubjectL0Stub).
type l1AbortAfterNStub struct {
	multiSubjectL0Stub
	okL1 int // number of L1 calls to allow before aborting
}

func (s *l1AbortAfterNStub) Fold(ctx context.Context, in FoldInput) (FoldResult, error) {
	if has(in.Existing, "_l1_hint") {
		if s.l1 >= s.okL1 {
			// Fail this call systemically.
			s.l1++
			return FoldResult{}, &InvokeError{Err: errors.New("usage limit reached")}
		}
	}
	return s.multiSubjectL0Stub.Fold(ctx, in)
}

// TestPendingSynthesisResumeAfterL1Abort verifies the full resume lifecycle:
//
//  1. An initial fold completes L0 but aborts L1 after the first subject due to
//     a systemic harness failure. FoldResult.PendingSynthesis must be true.
//
//  2. A follow-up fold with zero new events and ResumeSynthesis=true (simulating
//     what the CLI does when fold-state.PendingSynthesis is set) runs the L1 pass
//     against the existing episodes, completes the remaining subjects, and
//     produces PendingSynthesis=false.
func TestPendingSynthesisResumeAfterL1Abort(t *testing.T) {
	subjects := []string{"alpha", "bravo", "charlie"}
	// 6 events → chunkSize 1 → 6 episodes; every episode has all 3 tags →
	// collectSubjects produces 3 subjects.
	var events []FoldEvent
	for i := 1; i <= 6; i++ {
		events = append(events, makeMemoryEvent(uint64(i), "s", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), fmt.Sprintf("m%d", i), nil))
	}

	// ── Step 1: fold that aborts L1 after the first subject. ────────────────
	abortStub := &l1AbortAfterNStub{
		multiSubjectL0Stub: multiSubjectL0Stub{subjectTags: subjects},
		okL1:               1, // allow exactly one L1 call, then abort
	}
	hs := &HierarchySynth{inner: abortStub, l1Threshold: 1, l2Threshold: 1000}

	res1, err := FoldHierarchical(context.Background(), hs, FoldInput{
		ProjectID: "demo",
		Events:    events,
		ToOffset:  6,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}

	// L0 must have run for all 6 events.
	if abortStub.l0 != 6 {
		t.Errorf("want 6 L0 calls, got %d", abortStub.l0)
	}
	// L1 was aborted after 2 attempts (1 ok + 1 systemic fail).
	if abortStub.l1 != 2 {
		t.Errorf("want 2 L1 attempts (1 ok + 1 fail), got %d", abortStub.l1)
	}
	// PendingSynthesis must be true since L0 completed but L1 was aborted.
	if !res1.PendingSynthesis {
		t.Error("want PendingSynthesis=true after L1 abort, got false")
	}
	// FoldedThrough must still be the full watermark (L0 completed fine).
	if res1.FoldedThrough != 6 {
		t.Errorf("want FoldedThrough=6 (L0 complete), got %d", res1.FoldedThrough)
	}
	// Exactly one concept file was written (the first subject that succeeded).
	conceptCount := 0
	for p := range res1.Files {
		if strings.HasPrefix(p, referenceDir+"/") && !strings.HasSuffix(p, "/"+indexFileName) {
			conceptCount++
		}
	}
	if conceptCount != 1 {
		t.Errorf("want 1 concept file written before abort, got %d (files: %v)", conceptCount, fileKeys(res1.Files))
	}

	// ── Step 2: resume fold — zero new events, ResumeSynthesis=true. ────────
	// Use a working stub for the resume.
	workingStub := &multiSubjectL0Stub{subjectTags: subjects}
	hs2 := &HierarchySynth{inner: workingStub, l1Threshold: 1, l2Threshold: 1}

	res2, err := FoldHierarchical(context.Background(), hs2, FoldInput{
		ProjectID:       "demo",
		ToOffset:        6,
		Existing:        res1.Files,
		ExistingChunks:  res1.Chunks,
		ResumeSynthesis: true,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}

	// PendingSynthesis must clear when the fold completes without systemic abort.
	if res2.PendingSynthesis {
		t.Error("want PendingSynthesis=false after successful resume, got true")
	}
	// L0 must NOT have been called during the resume (all chunks cached).
	if workingStub.l0 != 0 {
		t.Errorf("want 0 L0 calls on resume (all chunks cached), got %d", workingStub.l0)
	}
	// All 3 subjects must now have concept files.
	conceptCount2 := 0
	for p := range res2.Files {
		if strings.HasPrefix(p, referenceDir+"/") && !strings.HasSuffix(p, "/"+indexFileName) {
			conceptCount2++
		}
	}
	if conceptCount2 != 3 {
		t.Errorf("want 3 concept files after resume (one per subject), got %d (files: %v)", conceptCount2, fileKeys(res2.Files))
	}
	// identity.md must be present (L2 threshold is 1).
	if _, ok := res2.Files[identityFile]; !ok {
		t.Errorf("want identity.md after resume, missing (files: %v)", fileKeys(res2.Files))
	}
}
