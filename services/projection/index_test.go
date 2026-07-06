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
			Level: 0, Kind: "episode", Version: 1,
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
		"episodes/aaaa.md":   PrependFrontmatter(Frontmatter{Type: typeEpisode, Level: 0, Version: 1}, "# Episode\n"),
		"reference/voice.md": PrependFrontmatter(Frontmatter{Type: typeReference, Name: "Voice", Level: 1, Version: 1}, "# Voice\n"),
	}
	idx := buildIndex(files, FoldInput{ProjectID: "demo"})
	if strings.Contains(idx, "episodes/aaaa.md") {
		t.Errorf("episodes should be hidden once reference concepts exist:\n%s", idx)
	}
	if !strings.Contains(idx, "reference/voice.md") {
		t.Errorf("router missing the reference concept:\n%s", idx)
	}
}

// TestBuildIndexRoutesICMLayout guards the OKF root router: reference concepts
// are clickable links described by their OKF name, identity is surfaced, and the
// folder routing points at each layer's own index.
func TestBuildIndexRoutesICMLayout(t *testing.T) {
	files := map[string]string{
		"identity.md": PrependFrontmatter(
			Frontmatter{Type: typeIdentity, Name: "Demo", Description: "A demo project.", Level: 2, Version: 1},
			"# Demo\n"),
		"reference/harness-mcp.md": PrependFrontmatter(
			Frontmatter{Type: typeReference, Name: "Harness MCP removal", Level: 1, Version: 1},
			"# Harness MCP removal\nDecision: drop MCP.\n"),
		"reference/INDEX.md":   PrependFrontmatter(Frontmatter{Type: typeIndex, Version: 1}, "# Reference — index\n"),
		"contracts/INDEX.md":   PrependFrontmatter(Frontmatter{Type: typeIndex, Version: 1}, "# Contracts — index\n"),
		"contracts/release.md": PrependFrontmatter(Frontmatter{Type: typeContract, Name: "Release flow", Level: 1, Version: 1}, "# Release flow\n"),
	}

	idx := buildIndex(files, FoldInput{ProjectID: "demo"})

	if !strings.Contains(idx, "[`identity.md`](identity.md)") || !strings.Contains(idx, "A demo project.") {
		t.Errorf("router missing identity with its description:\n%s", idx)
	}
	if !strings.Contains(idx, "[`reference/harness-mcp.md`](reference/harness-mcp.md)") {
		t.Errorf("reference concept not rendered as a markdown link:\n%s", idx)
	}
	if !strings.Contains(idx, "Harness MCP removal") {
		t.Errorf("router did not use the OKF concept name:\n%s", idx)
	}
	// Folder routing points at the per-folder index, not individual files.
	if !strings.Contains(idx, "[`contracts/INDEX.md`](contracts/INDEX.md)") {
		t.Errorf("router missing the contracts folder routing:\n%s", idx)
	}
	if strings.Contains(idx, "the task touches") {
		t.Errorf("router still emits the stale slug-echo hint:\n%s", idx)
	}
}
