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
		Layer:          "B",
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
	if got.Layer != fm.Layer {
		t.Errorf("Layer: got %q, want %q", got.Layer, fm.Layer)
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
	fm := Frontmatter{Layer: "C", Kind: "episode", Version: 1}
	rendered := RenderFrontmatter(fm)
	if strings.Contains(rendered, "time_range_start") {
		t.Error("zero TimeRangeStart should be omitted")
	}
	if strings.Contains(rendered, "folded_at") {
		t.Error("zero FoldedAt should be omitted")
	}
}

func TestFrontmatterNumericLayerCoerced(t *testing.T) {
	content := "---\nid: abc123\ntype: reference\nlayer: 1\nversion: 1\n---\n\n# Body\n"
	fm, _ := ParseFrontmatter(content)
	if fm.Layer != "B" {
		t.Errorf("Layer: got %q, want %q", fm.Layer, "B")
	}

	rendered := RenderFrontmatter(fm)
	if !strings.Contains(rendered, "layer: B") {
		t.Errorf("rendered frontmatter should contain %q, got %q", "layer: B", rendered)
	}
}

func TestStampProvenanceForcesLayerByPath(t *testing.T) {
	cases := []struct {
		name       string
		path       string
		inputLayer string
		wantLayer  string
		wantTier   string
	}{
		{"reference numeric layer", "reference/architecture.md", "1", "B", "distilled"},
		{"reference garbage layer", "reference/architecture.md", "banana", "B", "distilled"},
		{"conventions numeric layer", "conventions/workflow.md", "1", "B", "distilled"},
		{"episode numeric layer", "episodes/abc123.md", "0", "C", "raw"},
		{"episode garbage layer", "episodes/abc123.md", "banana", "C", "raw"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := "---\nlayer: " + tc.inputLayer + "\n---\n\n# Body\n"
			stamped := stampProvenance(content, "proj", tc.path, []uint64{1, 2, 3}, nil, time.Time{}, time.Time{}, "")
			fm, _ := ParseFrontmatter(stamped)
			if fm.Layer != tc.wantLayer {
				t.Errorf("Layer: got %q, want %q", fm.Layer, tc.wantLayer)
			}
			if fm.AccessTier != tc.wantTier {
				t.Errorf("AccessTier: got %q, want %q", fm.AccessTier, tc.wantTier)
			}
		})
	}
}

func TestLayerForPath(t *testing.T) {
	cases := []struct {
		path      string
		wantLayer string
		wantTier  string
		wantOK    bool
	}{
		{"identity.md", "A", "distilled", true},
		{"reference/architecture.md", "B", "distilled", true},
		{"conventions/workflow.md", "B", "distilled", true},
		{"episodes/abc123.md", "C", "raw", true},
		{"INDEX.md", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			layer, tier, ok := layerForPath(tc.path)
			if layer != tc.wantLayer || tier != tc.wantTier || ok != tc.wantOK {
				t.Errorf("layerForPath(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.path, layer, tier, ok, tc.wantLayer, tc.wantTier, tc.wantOK)
			}
		})
	}
}

func TestFrontmatterMissingBlock(t *testing.T) {
	content := "# Just a heading\n\nNo frontmatter.\n"
	fm, body := ParseFrontmatter(content)
	if fm.Layer != "" || fm.Kind != "" || fm.ID != "" {
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
		Layer:   "B",
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
