package projection

import (
	"strings"
	"testing"
	"time"
)

func TestFrontmatterRoundTrip(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	fm := Frontmatter{
		ID:             "abc123def456",
		Level:          1,
		Kind:           "topic",
		Sources:        []uint64{10, 20, 30},
		Tags:           []string{"jwt", "auth"},
		TimeRangeStart: now,
		TimeRangeEnd:   now.Add(24 * time.Hour),
		FoldedAt:       now,
		Version:        1,
		Children:       []string{"episodes/abc.md", "episodes/def.md"},
	}
	body := "# Topic: JWT\n\nSome content.\n"

	combined := PrependFrontmatter(fm, body)
	got, gotBody := ParseFrontmatter(combined)

	if got.ID != fm.ID {
		t.Errorf("ID: got %q, want %q", got.ID, fm.ID)
	}
	if got.Level != fm.Level {
		t.Errorf("Level: got %d, want %d", got.Level, fm.Level)
	}
	if got.Kind != fm.Kind {
		t.Errorf("Kind: got %q, want %q", got.Kind, fm.Kind)
	}
	if len(got.Sources) != len(fm.Sources) {
		t.Errorf("Sources len: got %d, want %d", len(got.Sources), len(fm.Sources))
	}
	if len(got.Tags) != len(fm.Tags) || got.Tags[0] != fm.Tags[0] {
		t.Errorf("Tags: got %v, want %v", got.Tags, fm.Tags)
	}
	if !got.TimeRangeStart.Equal(fm.TimeRangeStart) {
		t.Errorf("TimeRangeStart: got %v, want %v", got.TimeRangeStart, fm.TimeRangeStart)
	}
	if !got.FoldedAt.Equal(fm.FoldedAt) {
		t.Errorf("FoldedAt: got %v, want %v", got.FoldedAt, fm.FoldedAt)
	}
	if got.Version != fm.Version {
		t.Errorf("Version: got %d, want %d", got.Version, fm.Version)
	}
	if len(got.Children) != 2 || got.Children[0] != "episodes/abc.md" {
		t.Errorf("Children: got %v", got.Children)
	}
	if gotBody != body {
		t.Errorf("body: got %q, want %q", gotBody, body)
	}
}

func TestFrontmatterZeroTimesOmitted(t *testing.T) {
	fm := Frontmatter{Level: 0, Kind: "episode", Version: 1}
	rendered := RenderFrontmatter(fm)
	if strings.Contains(rendered, "time_range_start") {
		t.Error("zero TimeRangeStart should be omitted")
	}
	if strings.Contains(rendered, "folded_at") {
		t.Error("zero FoldedAt should be omitted")
	}
}

func TestFrontmatterMissingBlock(t *testing.T) {
	content := "# Just a heading\n\nNo frontmatter.\n"
	fm, body := ParseFrontmatter(content)
	if fm.Level != 0 || fm.Kind != "" || fm.ID != "" {
		t.Errorf("expected zero-value fm, got %+v", fm)
	}
	if body != content {
		t.Errorf("body should equal original content")
	}
}

func TestChunkIDDeterminism(t *testing.T) {
	id1 := chunkID("proj", []uint64{1, 2, 3})
	id2 := chunkID("proj", []uint64{1, 2, 3})
	if id1 != id2 {
		t.Errorf("chunkID not deterministic: %q != %q", id1, id2)
	}
	if len(id1) != 16 {
		t.Errorf("chunkID length: got %d, want 16", len(id1))
	}
}

func TestChunkIDSortInvariance(t *testing.T) {
	a := chunkID("proj", []uint64{3, 1, 2})
	b := chunkID("proj", []uint64{1, 2, 3})
	if a != b {
		t.Errorf("chunkID should be sort-invariant: %q != %q", a, b)
	}
}

func TestChunkIDProjectIsolation(t *testing.T) {
	a := chunkID("proj-a", []uint64{1, 2, 3})
	b := chunkID("proj-b", []uint64{1, 2, 3})
	if a == b {
		t.Errorf("different project IDs should produce different chunk IDs")
	}
}

func TestSourcesRangeCompression(t *testing.T) {
	fm := Frontmatter{
		Type:    "reference",
		Name:    "demo",
		Level:   1,
		Version: 1,
		Sources: []uint64{1, 2, 3, 4, 7, 9, 10, 11, 20},
	}
	out := RenderFrontmatter(fm)
	if !strings.Contains(out, "sources: [1-4, 7, 9-11, 20]") {
		t.Errorf("consecutive runs not compressed:\n%s", out)
	}
	parsed, _ := ParseFrontmatter(out + "body\n")
	want := []uint64{1, 2, 3, 4, 7, 9, 10, 11, 20}
	if len(parsed.Sources) != len(want) {
		t.Fatalf("round-trip lost offsets: %v", parsed.Sources)
	}
	for i, o := range want {
		if parsed.Sources[i] != o {
			t.Errorf("offset %d: want %d, got %d", i, o, parsed.Sources[i])
		}
	}
	// chunkID must be identical whether computed from expanded or round-tripped offsets.
	if chunkID("p", want) != chunkID("p", parsed.Sources) {
		t.Errorf("chunkID changed across range round-trip")
	}
}

func TestEnsureTitleStampsH1(t *testing.T) {
	fm := Frontmatter{Name: "Fold model configuration"}
	got := ensureTitle(fm, "- bullet one\n- bullet two\n")
	if !strings.HasPrefix(got, "# Fold model configuration\n\n- bullet one") {
		t.Errorf("title not stamped:\n%s", got)
	}
	// Idempotent: an existing H1 is kept, not duplicated.
	if ensureTitle(fm, got) != got {
		t.Errorf("ensureTitle not idempotent")
	}
	// No name → body unchanged.
	if ensureTitle(Frontmatter{}, "- b\n") != "- b\n" {
		t.Errorf("body changed despite empty name")
	}
}
