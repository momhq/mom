package projection

import (
	"context"
	"strings"
	"testing"
	"time"
)

// docThresholdStub is a fake inner synthesizer whose L0 pass tags document
// episodes with doc:<slug> (mirroring buildDocumentCaptureInput) and whose L1
// pass writes a reference/<slug>.md concept for document subjects, so we can
// drive FoldHierarchical below l1Threshold without an LLM.
type docThresholdStub struct{ l0, l1 int }

func (s *docThresholdStub) Fold(_ context.Context, in FoldInput) (FoldResult, error) {
	files := map[string]string{}
	switch {
	case has(in.Existing, "_l0_hint"):
		s.l0++
		offs := make([]uint64, len(in.Events))
		for i, e := range in.Events {
			offs[i] = e.Offset
		}
		cid := chunkID(in.ProjectID, offs)
		tags := []string{"arch"}
		if len(in.Events) > 0 && in.Events[0].SourceClass == "document" {
			tags = []string{"doc:" + tagSlug(in.Events[0].DocTitle)}
		}
		files["episodes/"+cid+".md"] = PrependFrontmatter(
			Frontmatter{Type: typeEpisode, Layer: "C", Version: 1, Sources: offs, Tags: tags}, "# Episode\n")
	case has(in.Existing, "_l1_hint"):
		s.l1++
		var slug string
		isDoc := false
		for p, c := range in.Existing {
			if !strings.HasPrefix(p, episodesDir+"/") {
				continue
			}
			fm, _ := ParseFrontmatter(c)
			for _, t := range fm.Tags {
				if bookSlug, ok := docTagSlug(t); ok {
					slug, isDoc = bookSlug, true
				}
			}
		}
		if isDoc {
			files[referenceDir+"/"+slug+".md"] = PrependFrontmatter(
				Frontmatter{Type: typeReference, Subtype: "document", Name: slug, Layer: "B", Version: 1, Sources: []uint64{1}},
				"# "+slug+"\n")
		} else {
			files[referenceDir+"/subject.md"] = PrependFrontmatter(
				Frontmatter{Type: typeReference, Name: "subject", Layer: "B", Version: 1, Sources: []uint64{1}},
				"# subject\n")
		}
	}
	return FoldResult{Files: files}, nil
}

func makeDocumentEvent(offset uint64, docTitle string, t time.Time) FoldEvent {
	return FoldEvent{
		Offset:       offset,
		Type:         "capture.document_chapter.observed",
		CreatedAt:    t,
		Text:         "chapter text",
		SourceClass:  "document",
		DocID:        "doc1",
		DocTitle:     docTitle,
		ChapterIndex: int(offset),
	}
}

// TestHierarchicalDocumentBypassesGlobalThreshold proves that a fold with
// ONLY document episodes — below l1Threshold — still synthesizes the
// document's reference/ concept: the flagship single-book-ingest story must
// not silently produce zero concepts.
func TestHierarchicalDocumentBypassesGlobalThreshold(t *testing.T) {
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events := []FoldEvent{
		makeDocumentEvent(1, "My Book", when),
		makeDocumentEvent(2, "My Book", when),
	}
	stub := &docThresholdStub{}
	hs := &HierarchySynth{inner: stub, l1Threshold: 5, l2Threshold: 100}

	res, err := FoldHierarchical(context.Background(), hs, FoldInput{ProjectID: "demo", Events: events}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stub.l0 != 2 {
		t.Errorf("want 2 L0 calls, got %d", stub.l0)
	}
	if stub.l1 != 1 {
		t.Fatalf("want 1 L1 document call despite being below l1Threshold, got %d", stub.l1)
	}
	c, ok := res.Files[referenceDir+"/my-book.md"]
	if !ok {
		t.Fatalf("missing reference/my-book.md concept; files: %v", keysOf(res.Files))
	}
	fm, _ := ParseFrontmatter(c)
	if fm.Subtype != "document" {
		t.Errorf("want subtype:document, got %q", fm.Subtype)
	}
}

// TestHierarchicalTranscriptStaysGatedByThreshold proves the fix is scoped to
// document subjects only: a transcript-only fold below l1Threshold still
// produces zero concepts (no regression on ordinary folds).
func TestHierarchicalTranscriptStaysGatedByThreshold(t *testing.T) {
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events := []FoldEvent{
		makeMemoryEvent(1, "s", when, "m1", nil),
		makeMemoryEvent(2, "s", when, "m2", nil),
	}
	stub := &docThresholdStub{}
	hs := &HierarchySynth{inner: stub, l1Threshold: 5, l2Threshold: 100}

	res, err := FoldHierarchical(context.Background(), hs, FoldInput{ProjectID: "demo", Events: events}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stub.l0 != 2 {
		t.Errorf("want 2 L0 calls, got %d", stub.l0)
	}
	if stub.l1 != 0 {
		t.Errorf("want 0 L1 calls below threshold with no document episodes, got %d", stub.l1)
	}
	for p := range res.Files {
		if strings.HasPrefix(p, referenceDir+"/") && !strings.HasSuffix(p, indexFileName) {
			t.Errorf("unexpected concept file below threshold: %s", p)
		}
	}
}

func keysOf(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
