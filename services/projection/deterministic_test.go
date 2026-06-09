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

func keys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
