package projection

import (
	"context"
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

func TestBuildTimelineTwoMonths(t *testing.T) {
	events := []FoldEvent{
		makeMemoryEvent(1, "sess-a", time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC), "jan memory", nil),
		makeMemoryEvent(2, "sess-b", time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC), "feb memory", nil),
	}
	out := buildTimeline("test-project", events)
	if len(out) != 2 {
		t.Fatalf("expected 2 timeline files, got %d: %v", len(out), keys(out))
	}
	for path, content := range out {
		if !strings.HasPrefix(path, "timeline/") {
			t.Errorf("unexpected path: %s", path)
		}
		fm, _ := ParseFrontmatter(content)
		if fm.Level != 1 {
			t.Errorf("%s: expected level 1, got %d", path, fm.Level)
		}
		if fm.Kind != "timeline" {
			t.Errorf("%s: expected kind timeline, got %q", path, fm.Kind)
		}
		if len(fm.Sources) == 0 {
			t.Errorf("%s: expected sources, got none", path)
		}
	}
}

func TestDeterministicFoldFrontmatter(t *testing.T) {
	events := []FoldEvent{
		makeMemoryEvent(10, "sess-a", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), "a decision", []string{"arch"}),
		makeMemoryEvent(11, "sess-a", time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), "another decision", []string{"arch"}),
	}
	synth := NewDeterministicSynth()
	res, err := synth.Fold(context.Background(), FoldInput{
		ProjectID: "test-proj",
		Events:    events,
	})
	if err != nil {
		t.Fatalf("Fold error: %v", err)
	}
	for path, content := range res.Files {
		fm, _ := ParseFrontmatter(content)
		if fm.Level != 1 {
			t.Errorf("%s: expected level 1, got %d", path, fm.Level)
		}
		if fm.Version != 1 {
			t.Errorf("%s: expected version 1, got %d", path, fm.Version)
		}
		if len(fm.Sources) == 0 {
			t.Errorf("%s: expected sources, got none", path)
		}
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

func TestBuildIndexEpisodesHiddenOnceTopicsExist(t *testing.T) {
	files := map[string]string{
		"episodes/aaaa.md": PrependFrontmatter(Frontmatter{Level: 0, Kind: "episode", Version: 1}, "# Episode\n"),
		"topics/voice.md":  PrependFrontmatter(Frontmatter{Level: 1, Kind: "topic", Version: 1}, "# Topic\n"),
	}
	idx := buildIndex(files, FoldInput{ProjectID: "demo"})
	if strings.Contains(idx, "episodes/aaaa.md") {
		t.Errorf("episodes should be hidden once L1 topics exist:\n%s", idx)
	}
	if !strings.Contains(idx, "topics/voice.md") {
		t.Errorf("router missing the topic file:\n%s", idx)
	}
}

// TestBuildIndexLinksAndTitles guards the router improvements: every entry is a
// clickable markdown link (not a bare path), and a topic's description is drawn
// from its synthesized H1 title rather than echoing the slug.
func TestBuildIndexLinksAndTitles(t *testing.T) {
	files := map[string]string{
		"topics/harness-mcp.md": PrependFrontmatter(
			Frontmatter{Level: 1, Kind: "topic", Version: 1},
			"# Topic: Harness MCP removal → vault-first context\nDecision: drop MCP.\n"),
		"topics/raw-slug.md": PrependFrontmatter(
			Frontmatter{Level: 1, Kind: "topic", Version: 1}, "no title here\n"),
		"summaries/overview.md": PrependFrontmatter(
			Frontmatter{Level: 2, Kind: "summary", Version: 1}, "# Project overview\nstuff\n"),
	}

	idx := buildIndex(files, FoldInput{ProjectID: "demo"})

	// Linked path, not a bare code span.
	if !strings.Contains(idx, "[`topics/harness-mcp.md`](topics/harness-mcp.md)") {
		t.Errorf("topic path not rendered as a markdown link:\n%s", idx)
	}
	// Description comes from the H1 title (prefix stripped), not the slug.
	if !strings.Contains(idx, "Harness MCP removal → vault-first context") {
		t.Errorf("router did not use the synthesized topic title:\n%s", idx)
	}
	if strings.Contains(idx, "the task touches") {
		t.Errorf("router still emits the stale slug-echo hint:\n%s", idx)
	}
	// A file without a leading H1 falls back to the humanized slug.
	if !strings.Contains(idx, "| raw slug |") {
		t.Errorf("missing humanized-slug fallback for title-less topic:\n%s", idx)
	}
}

func keys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
