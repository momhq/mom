package projection

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLinkRelated_ChildrenAndSiblings(t *testing.T) {
	mk := func(layer, kind string, sources []uint64, tags []string, title string) string {
		return PrependFrontmatter(Frontmatter{
			Layer: layer, Kind: kind, Version: 1, Sources: sources, Tags: tags,
		}, "# "+title+"\nbody\n")
	}

	files := map[string]string{
		// Two episodes feeding the architecture topic.
		"episodes/e1.md": mk("C", "episode", []uint64{10, 11}, []string{"architecture"}, "Episode 1"),
		"episodes/e2.md": mk("C", "episode", []uint64{12}, []string{"architecture"}, "Episode 2"),
		// Topics: architecture + ledger share the "architecture" tag; voice is unrelated.
		"topics/architecture.md": mk("B", "topic", []uint64{10, 11, 12}, []string{"architecture"}, "Topic: Architecture"),
		"topics/ledger.md":       mk("B", "topic", []uint64{20, 21}, []string{"architecture", "ledger"}, "Topic: Ledger"),
		"topics/voice.md":        mk("B", "topic", []uint64{30}, []string{"voice"}, "Topic: Voice"),
		// Overview rolls up every topic offset.
		"summaries/overview.md": mk("A", "summary", []uint64{10, 11, 12, 20, 21, 30}, []string{"architecture"}, "Project overview"),
	}

	linkRelated(files)

	arch := files["topics/architecture.md"]
	fm, _ := ParseFrontmatter(arch)

	// Children: the two episodes whose offsets are covered by the topic's sources.
	if got := strings.Join(fm.Children, ","); got != "episodes/e1.md,episodes/e2.md" {
		t.Errorf("architecture children = %q, want the two episodes", got)
	}

	// Related body: links the sibling that shares a tag (ledger), not voice.
	if !strings.Contains(arch, relatedHeading) {
		t.Fatalf("architecture topic missing Related section:\n%s", arch)
	}
	if !strings.Contains(arch, "](ledger.md)") {
		t.Errorf("architecture should link sibling ledger.md:\n%s", arch)
	}
	if strings.Contains(arch, "voice.md") {
		t.Errorf("architecture should NOT link the unrelated voice topic:\n%s", arch)
	}
	// Parent overview link, relative from topics/ up to summaries/.
	if !strings.Contains(arch, "](../summaries/overview.md) _(overview)_") {
		t.Errorf("architecture missing parent overview link:\n%s", arch)
	}

	// Overview's children are the three L1 topics (one level down), not episodes.
	ovFm, _ := ParseFrontmatter(files["summaries/overview.md"])
	if got := strings.Join(ovFm.Children, ","); got != "topics/architecture.md,topics/ledger.md,topics/voice.md" {
		t.Errorf("overview children = %q, want the three topics", got)
	}

	// Episodes stay leaves: no Related section.
	if strings.Contains(files["episodes/e1.md"], relatedHeading) {
		t.Errorf("episode should not get a Related section:\n%s", files["episodes/e1.md"])
	}
}

func TestBuildPerFolderIndexes(t *testing.T) {
	files := map[string]string{
		"reference/voice.md": PrependFrontmatter(
			Frontmatter{Type: typeReference, Name: "Voice & tone", Description: "How the product speaks.", Layer: "B", Version: 1},
			"# Voice & tone\nbody\n"),
		"reference/ledger.md": PrependFrontmatter(
			Frontmatter{Type: typeReference, Name: "Ledger", Layer: "B", Version: 1},
			"# Ledger\nThe append-only source of truth. More detail here.\n"),
		"episodes/e1.md": PrependFrontmatter(Frontmatter{Type: typeEpisode, Layer: "C", Version: 1}, "# Episode\n"),
	}

	buildPerFolderIndexes(files)

	idx, ok := files["reference/INDEX.md"]
	if !ok {
		t.Fatalf("reference/INDEX.md not generated")
	}
	fm, _ := ParseFrontmatter(idx)
	if fm.Type != typeIndex {
		t.Errorf("per-folder index should have type=index, got %q", fm.Type)
	}
	// Links are relative to the folder (base filename only) and carry the OKF name.
	if !strings.Contains(idx, "[`voice.md`](voice.md)") || !strings.Contains(idx, "Voice & tone") {
		t.Errorf("reference index missing concept link/name:\n%s", idx)
	}
	if !strings.Contains(idx, "How the product speaks.") {
		t.Errorf("reference index missing OKF description:\n%s", idx)
	}
	// Episodes are provenance — no folder index.
	if _, exists := files["episodes/INDEX.md"]; exists {
		t.Errorf("episodes/ should not get a folder index")
	}
}

// TestLinkRelated_ICMTypeSiblings verifies that two files carrying the ICM
// `type:` field (no legacy `kind:`) are correctly recognised as siblings when
// they share a tag.
func TestLinkRelated_ICMTypeSiblings(t *testing.T) {
	mkICM := func(typ string, sources []uint64, tags []string, title string) string {
		return PrependFrontmatter(Frontmatter{
			Type: typ, Layer: "B", Version: 1, Sources: sources, Tags: tags,
		}, "# "+title+"\nbody\n")
	}

	files := map[string]string{
		"reference/auth.md":    mkICM(typeReference, []uint64{1, 2}, []string{"auth", "security"}, "Auth"),
		"reference/session.md": mkICM(typeReference, []uint64{3, 4}, []string{"auth", "session"}, "Session"),
		"contracts/release.md": mkICM(typeContract, []uint64{5, 6}, []string{"release"}, "Release"),
	}

	linkRelated(files)

	auth := files["reference/auth.md"]
	// auth and session share the "auth" tag and have the same type:reference.
	if !strings.Contains(auth, relatedHeading) {
		t.Fatalf("auth.md missing Related section (ICM type-based sibling):\n%s", auth)
	}
	if !strings.Contains(auth, "](session.md)") {
		t.Errorf("auth.md should link sibling session.md (same type, shared tag):\n%s", auth)
	}
	// release.md is type:contract — must NOT appear as a sibling of auth.md.
	if strings.Contains(auth, "release.md") {
		t.Errorf("auth.md should NOT link cross-type release.md:\n%s", auth)
	}
}

func TestLinkRelated_Idempotent(t *testing.T) {
	mk := func(p string) map[string]string {
		return map[string]string{
			"topics/a.md": PrependFrontmatter(Frontmatter{Layer: "B", Kind: "topic", Version: 1, Sources: []uint64{1}, Tags: []string{"x"}}, "# A\nbody\n"),
			"topics/b.md": PrependFrontmatter(Frontmatter{Layer: "B", Kind: "topic", Version: 1, Sources: []uint64{2}, Tags: []string{"x"}}, "# B\nbody\n"),
		}
	}
	files := mk("")
	linkRelated(files)
	once := files["topics/a.md"]
	linkRelated(files)
	twice := files["topics/a.md"]
	if once != twice {
		t.Errorf("linkRelated not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
	if c := strings.Count(twice, relatedHeading); c != 1 {
		t.Errorf("want exactly one Related section after re-fold, got %d:\n%s", c, twice)
	}
}

// TestLinkRelated_OffsetOverlapSiblings mirrors the real ICM vault shape:
// every concept carries only its own unique slug as a tag, so tag overlap is
// impossible — relatedness must come from shared source offsets.
func TestLinkRelated_OffsetOverlapSiblings(t *testing.T) {
	mk := func(typ string, sources []uint64, tag, title string) string {
		return PrependFrontmatter(Frontmatter{
			Type: typ, Name: title, Layer: "B", Version: 1, Sources: sources, Tags: []string{tag},
		}, "# "+title+"\nbody\n")
	}
	files := map[string]string{
		// fold and release co-occur in offsets 10-12; voice is disjoint.
		"reference/fold.md":    mk(typeReference, []uint64{10, 11, 12, 13}, "fold", "Fold"),
		"contracts/release.md": mk(typeContract, []uint64{11, 12, 20}, "release", "Release"),
		"reference/voice.md":   mk(typeReference, []uint64{30, 31}, "voice", "Voice"),
	}

	linkRelated(files)

	fold := files["reference/fold.md"]
	if !strings.Contains(fold, relatedHeading) {
		t.Fatalf("fold.md missing Related section despite offset overlap:\n%s", fold)
	}
	// Cross-type links are wanted: the release contract shares fold's windows.
	if !strings.Contains(fold, "](../contracts/release.md)") {
		t.Errorf("fold.md should link co-occurring release.md:\n%s", fold)
	}
	if strings.Contains(fold, "voice.md") {
		t.Errorf("fold.md should NOT link disjoint voice.md:\n%s", fold)
	}
	// And the link is mutual.
	if !strings.Contains(files["contracts/release.md"], "](../reference/fold.md)") {
		t.Errorf("release.md should link back to fold.md:\n%s", files["contracts/release.md"])
	}
}

// TestFoldStampsIdentityProvenance: identity.md gets machine-stamped sources
// (union of the concept layer) so concept files can link it as their parent.
func TestFoldStampsIdentityProvenance(t *testing.T) {
	var events []FoldEvent
	for i := 1; i <= 10; i++ {
		events = append(events, makeMemoryEvent(uint64(i), "s", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "m", nil))
	}
	stub := &refStub{}
	hs := &HierarchySynth{inner: stub, l1Threshold: 5, l2Threshold: 1}

	res, err := FoldHierarchical(context.Background(), hs, FoldInput{ProjectID: "demo", Events: events, ToOffset: 10}, 1)
	if err != nil {
		t.Fatal(err)
	}
	fm, _ := ParseFrontmatter(res.Files[identityFile])
	if len(fm.Sources) == 0 {
		t.Fatalf("identity.md has no stamped sources:\n%s", res.Files[identityFile])
	}
	if fm.ID == "" {
		t.Errorf("identity.md missing content-addressed id")
	}
	// The concept file links identity as its parent overview.
	ref := res.Files[referenceDir+"/arch.md"]
	if !strings.Contains(ref, "](../identity.md) _(overview)_") {
		t.Errorf("concept missing parent overview link to identity:\n%s", ref)
	}
}
