package projection

import (
	"strings"
	"testing"
	"time"
)

func makeMemoryEvent(offset uint64, sessionID string, t time.Time, text string, tags []string) FoldEvent {
	return FoldEvent{
		Offset:    offset,
		Type:      string(memoryType),
		SessionID: sessionID,
		CreatedAt: t,
		Role:      "user",
		Text:      text,
		Tags:      tags,
	}
}

func TestBuildIndexEpisodeFallback(t *testing.T) {
	mkEpisode := func(start, end string) string {
		ts, _ := time.Parse(time.RFC3339, start)
		te, _ := time.Parse(time.RFC3339, end)
		return PrependFrontmatter(Frontmatter{
			Layer: "C", Kind: "episode", Version: 1,
			TimeRangeStart: ts, TimeRangeEnd: te,
		}, "# Episode\n")
	}
	files := map[string]string{
		"episodes/aaaa.md": mkEpisode("2026-05-01T00:00:00Z", "2026-05-02T00:00:00Z"),
		"episodes/bbbb.md": mkEpisode("2026-06-01T00:00:00Z", "2026-06-03T00:00:00Z"),
	}

	idx := buildIndex(files, FoldInput{ProjectID: "demo"})

	if strings.Contains(idx, "no files yet") {
		t.Fatal("router reported 'no files yet' while episodes exist")
	}
	for _, p := range []string{"episodes/aaaa.md", "episodes/bbbb.md"} {
		if !strings.Contains(idx, p) {
			t.Errorf("router missing episode %s:\n%s", p, idx)
		}
	}
	if !strings.Contains(idx, "session memory from 2026-06-01 to 2026-06-03") {
		t.Errorf("missing date-range hint for newer episode:\n%s", idx)
	}
	// Newest episode (June) must be routed before the older one (May).
	if strings.Index(idx, "episodes/bbbb.md") > strings.Index(idx, "episodes/aaaa.md") {
		t.Errorf("episodes not ordered newest-first:\n%s", idx)
	}
}

func TestBuildIndexEpisodesHiddenOnceReferenceExists(t *testing.T) {
	files := map[string]string{
		"episodes/aaaa.md":   PrependFrontmatter(Frontmatter{Type: typeEpisode, Layer: "C", Version: 1}, "# Episode\n"),
		"reference/voice.md": PrependFrontmatter(Frontmatter{Type: typeReference, Name: "Voice", Layer: "B", Version: 1}, "# Voice\n"),
	}
	idx := buildIndex(files, FoldInput{ProjectID: "demo"})
	if strings.Contains(idx, "episodes/aaaa.md") {
		t.Errorf("episodes should never be individually enumerated:\n%s", idx)
	}
	if strings.Contains(idx, "reference/voice.md") {
		t.Errorf("root router should not enumerate individual concepts:\n%s", idx)
	}
	if !strings.Contains(idx, "reference/INDEX.md") {
		t.Errorf("router missing the reference folder routing:\n%s", idx)
	}
}

// TestBuildIndexRoutesICMLayout guards the OKF root router: it is
// routing-ONLY — identity is surfaced with its description, and each
// populated layer points at its own per-folder INDEX, with no individual
// concept enumerated.
func TestBuildIndexRoutesICMLayout(t *testing.T) {
	files := map[string]string{
		"identity.md": PrependFrontmatter(
			Frontmatter{Type: typeIdentity, Name: "Demo", Description: "A demo project.", Layer: "A", Version: 1},
			"# Demo\n"),
		"reference/harness-mcp.md": PrependFrontmatter(
			Frontmatter{Type: typeReference, Name: "Harness MCP removal", Layer: "B", Version: 1},
			"# Harness MCP removal\nDecision: drop MCP.\n"),
		"reference/INDEX.md":     PrependFrontmatter(Frontmatter{Type: typeIndex, Version: 1}, "# Reference — index\n"),
		"conventions/INDEX.md":   PrependFrontmatter(Frontmatter{Type: typeIndex, Version: 1}, "# Conventions — index\n"),
		"conventions/release.md": PrependFrontmatter(Frontmatter{Type: typeConvention, Name: "Release flow", Layer: "B", Version: 1}, "# Release flow\n"),
	}

	idx := buildIndex(files, FoldInput{ProjectID: "demo"})

	if !strings.Contains(idx, "[`identity.md`](identity.md)") || !strings.Contains(idx, "A demo project.") {
		t.Errorf("router missing identity with its description:\n%s", idx)
	}
	// Folder routing points at the per-folder index, never individual concepts.
	if !strings.Contains(idx, "[`reference/INDEX.md`](reference/INDEX.md)") {
		t.Errorf("router missing the reference folder routing:\n%s", idx)
	}
	if !strings.Contains(idx, "[`conventions/INDEX.md`](conventions/INDEX.md)") {
		t.Errorf("router missing the conventions folder routing:\n%s", idx)
	}
	if strings.Contains(idx, "reference/harness-mcp.md") || strings.Contains(idx, "Harness MCP removal") {
		t.Errorf("root router must not enumerate individual concepts:\n%s", idx)
	}
	if strings.Contains(idx, "conventions/release.md") {
		t.Errorf("root router must not enumerate individual concepts:\n%s", idx)
	}
}
