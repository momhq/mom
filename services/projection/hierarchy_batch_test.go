package projection

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// icmStub is a fake inner synthesizer that emits ICM concepts based on the
// work-item hint, so we can drive FoldHierarchical without an LLM.
type icmStub struct{ l0, l1, l2 int }

func (s *icmStub) Fold(_ context.Context, in FoldInput) (FoldResult, error) {
	files := map[string]string{}
	switch {
	case has(in.Existing, "_l0_hint"):
		s.l0++
		offs := make([]uint64, len(in.Events))
		for i, e := range in.Events {
			offs[i] = e.Offset
		}
		cid := chunkID(in.ProjectID, offs)
		// Every episode carries the same subject tag so L1 groups them into one
		// subject.
		files["episodes/"+cid+".md"] = PrependFrontmatter(
			Frontmatter{Type: typeEpisode, Level: 0, Version: 1, Sources: offs, Tags: []string{"arch"}}, "# Episode\n")
	case has(in.Existing, "_l1_hint"):
		s.l1++
		name := fmt.Sprintf("subject-%d", s.l1)
		files[referenceDir+"/"+name+".md"] = PrependFrontmatter(
			Frontmatter{Type: typeReference, Name: name, Level: 1, Version: 1, Sources: []uint64{uint64(s.l1)}},
			"# "+name+"\n")
	case has(in.Existing, "_l2_hint"):
		s.l2++
		files[identityFile] = PrependFrontmatter(
			Frontmatter{Type: typeIdentity, Name: "Demo", Level: 2, Version: 1}, "# Demo\n")
	}
	return FoldResult{Files: files}, nil
}

func has(m map[string]string, k string) bool { _, ok := m[k]; return ok }

// TestHierarchicalL1SubjectOriented verifies that L1 synthesizes one call per
// subject (grouped from L0 tags), not one giant call over every episode, and
// that the ICM layout — reference/ + identity.md, no chronological layer — is
// produced.
func TestHierarchicalL1SubjectOriented(t *testing.T) {
	var events []FoldEvent
	for i := 1; i <= 25; i++ {
		events = append(events, makeMemoryEvent(uint64(i), "s", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "m", nil))
	}
	stub := &icmStub{}
	hs := &HierarchySynth{inner: stub, l1Threshold: 5, l2Threshold: 1}

	// chunkSize 1 → one episode per event → 25 episodes, all tagged "arch".
	res, err := FoldHierarchical(context.Background(), hs, FoldInput{ProjectID: "demo", Events: events}, 1)
	if err != nil {
		t.Fatal(err)
	}

	if stub.l0 != 25 {
		t.Errorf("want 25 L0 calls, got %d", stub.l0)
	}
	// All episodes share one subject → exactly one L1 call, not one per episode.
	if stub.l1 != 1 {
		t.Errorf("want 1 L1 subject call, got %d", stub.l1)
	}
	if stub.l2 != 1 {
		t.Errorf("want 1 L2 call, got %d", stub.l2)
	}
	if _, ok := res.Files[identityFile]; !ok {
		t.Errorf("missing identity.md")
	}
	hasRef := false
	for p := range res.Files {
		if strings.HasPrefix(p, referenceDir+"/") && !strings.HasSuffix(p, indexFileName) {
			hasRef = true
		}
		if strings.HasPrefix(p, "dev-log/") || strings.HasPrefix(p, "timeline/") || strings.HasPrefix(p, "summaries/") {
			t.Errorf("unexpected chronological/legacy file: %s", p)
		}
	}
	if !hasRef {
		t.Errorf("no reference concept produced")
	}
}
