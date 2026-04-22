package logbook_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/momhq/mom/cli/internal/logbook"
)

// sampleTranscript builds a minimal JSONL transcript for testing.
func sampleTranscript(t *testing.T) string {
	t.Helper()
	lines := []map[string]any{
		// user turn
		{
			"timestamp": "2024-01-15T10:00:00Z",
			"role":      "user",
			"content":   "hello",
		},
		// assistant turn with tool calls
		{
			"timestamp": "2024-01-15T10:00:05Z",
			"role":      "assistant",
			"content": []any{
				map[string]any{
					"type": "tool_use",
					"name": "Read",
					"input": map[string]any{
						"file_path": "/project/main.go",
					},
				},
				map[string]any{
					"type": "tool_use",
					"name": "Grep",
					"input": map[string]any{},
				},
				map[string]any{
					"type": "tool_use",
					"name": "Edit",
					"input": map[string]any{
						"file_path": "/project/main.go",
					},
				},
				map[string]any{
					"type": "tool_use",
					"name": "Write",
					"input": map[string]any{
						"file_path": "/project/new_file.go",
					},
				},
				map[string]any{
					"type": "tool_use",
					"name": "mom_recall",
					"input": map[string]any{},
				},
				map[string]any{
					"type": "tool_use",
					"name": "create_memory_draft",
					"input": map[string]any{},
				},
				map[string]any{
					"type": "tool_use",
					"name": "mom_status",
					"input": map[string]any{},
				},
			},
		},
		// second assistant turn
		{
			"timestamp": "2024-01-15T10:01:00Z",
			"role":      "assistant",
			"content": []any{
				map[string]any{
					"type": "tool_use",
					"name": "Bash",
					"input": map[string]any{},
				},
			},
		},
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "transcript.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, l := range lines {
		if err := enc.Encode(l); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestCategorizeToolCall(t *testing.T) {
	cases := []struct {
		tool     string
		expected string
	}{
		// mom_memory
		{"mom_recall", "mom_memory"},
		{"search_memories", "mom_memory"},
		{"get_memory", "mom_memory"},
		{"create_memory_draft", "mom_memory"},
		{"list_landmarks", "mom_memory"},
		{"mom_record_turn", "mom_memory"},
		// mom_cli
		{"mom_status", "mom_cli"},
		{"mom_draft", "mom_cli"},
		{"mom_log", "mom_cli"},
		// codebase_read
		{"Read", "codebase_read"},
		{"read", "codebase_read"},
		{"Grep", "codebase_read"},
		{"grep", "codebase_read"},
		{"Glob", "codebase_read"},
		{"glob", "codebase_read"},
		{"rg", "codebase_read"},
		// codebase_write
		{"Edit", "codebase_write"},
		{"edit", "codebase_write"},
		{"Write", "codebase_write"},
		{"write", "codebase_write"},
		// system (fallback)
		{"Bash", "system"},
		{"unknown_tool", "system"},
		{"", "system"},
	}

	for _, tc := range cases {
		got := logbook.CategorizeToolCall(tc.tool)
		if got != tc.expected {
			t.Errorf("CategorizeToolCall(%q) = %q, want %q", tc.tool, got, tc.expected)
		}
	}
}

func TestParseTranscript(t *testing.T) {
	path := sampleTranscript(t)
	sessionID := "test-session-123"

	log, err := logbook.ParseTranscript(path, sessionID)
	if err != nil {
		t.Fatalf("ParseTranscript returned error: %v", err)
	}

	if log.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", log.SessionID, sessionID)
	}

	// 2 assistant turns
	if log.Interactions != 2 {
		t.Errorf("Interactions = %d, want 2", log.Interactions)
	}

	// Edit writes to /project/main.go, Write writes to /project/new_file.go → 2 unique files
	if log.FilesChanged != 2 {
		t.Errorf("FilesChanged = %d, want 2", log.FilesChanged)
	}

	// one create_memory_draft call
	if log.MemoriesCreated != 1 {
		t.Errorf("MemoriesCreated = %d, want 1", log.MemoriesCreated)
	}

	// timestamps present and non-empty
	if log.Started == "" {
		t.Error("Started is empty")
	}
	if log.Ended == "" {
		t.Error("Ended is empty")
	}
	if log.Started > log.Ended {
		t.Errorf("Started %q is after Ended %q", log.Started, log.Ended)
	}

	// tool call categories
	checkGroup(t, log, "codebase_read", 2, map[string]int{"Read": 1, "Grep": 1})
	checkGroup(t, log, "codebase_write", 2, map[string]int{"Edit": 1, "Write": 1})
	checkGroup(t, log, "mom_memory", 2, map[string]int{"mom_recall": 1, "create_memory_draft": 1})
	checkGroup(t, log, "mom_cli", 1, map[string]int{"mom_status": 1})
	checkGroup(t, log, "system", 1, map[string]int{"Bash": 1})
}

func checkGroup(t *testing.T, log *logbook.SessionLog, category string, total int, detail map[string]int) {
	t.Helper()
	g, ok := log.ToolCalls[category]
	if !ok {
		t.Errorf("ToolCalls missing category %q", category)
		return
	}
	if g.Total != total {
		t.Errorf("ToolCalls[%q].Total = %d, want %d", category, g.Total, total)
	}
	for tool, count := range detail {
		if g.Detail[tool] != count {
			t.Errorf("ToolCalls[%q].Detail[%q] = %d, want %d", category, tool, g.Detail[tool], count)
		}
	}
}

func TestParseTranscriptEmptyFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "empty.jsonl")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	log, err := logbook.ParseTranscript(path, "empty-session")
	if err != nil {
		t.Fatalf("unexpected error on empty file: %v", err)
	}
	if log.Interactions != 0 {
		t.Errorf("Interactions = %d, want 0", log.Interactions)
	}
	if log.FilesChanged != 0 {
		t.Errorf("FilesChanged = %d, want 0", log.FilesChanged)
	}
	if log.MemoriesCreated != 0 {
		t.Errorf("MemoriesCreated = %d, want 0", log.MemoriesCreated)
	}
	if len(log.ToolCalls) != 0 {
		t.Errorf("ToolCalls = %v, want empty", log.ToolCalls)
	}
	if log.Started == "" {
		t.Error("Started should be non-empty even for empty file (fallback to now)")
	}
}

func TestParseTranscriptMissingFile(t *testing.T) {
	_, err := logbook.ParseTranscript("/nonexistent/path/transcript.jsonl", "sess")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestParseTranscriptToolCounting(t *testing.T) {
	// Build a transcript with repeated tool calls to verify counting accuracy.
	lines := []map[string]any{
		{
			"timestamp": "2024-01-15T09:00:00Z",
			"role":      "assistant",
			"content": []any{
				map[string]any{"type": "tool_use", "name": "Read", "input": map[string]any{}},
				map[string]any{"type": "tool_use", "name": "Read", "input": map[string]any{}},
				map[string]any{"type": "tool_use", "name": "Read", "input": map[string]any{}},
				map[string]any{"type": "tool_use", "name": "Grep", "input": map[string]any{}},
				map[string]any{"type": "tool_use", "name": "Bash", "input": map[string]any{}},
				map[string]any{"type": "tool_use", "name": "Bash", "input": map[string]any{}},
			},
		},
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "counting.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, l := range lines {
		enc.Encode(l)
	}
	f.Close()

	log, err := logbook.ParseTranscript(path, "count-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	readGroup := log.ToolCalls["codebase_read"]
	if readGroup.Total != 4 {
		t.Errorf("codebase_read.Total = %d, want 4", readGroup.Total)
	}
	if readGroup.Detail["Read"] != 3 {
		t.Errorf("codebase_read.Detail[Read] = %d, want 3", readGroup.Detail["Read"])
	}
	if readGroup.Detail["Grep"] != 1 {
		t.Errorf("codebase_read.Detail[Grep] = %d, want 1", readGroup.Detail["Grep"])
	}

	sysGroup := log.ToolCalls["system"]
	if sysGroup.Total != 2 {
		t.Errorf("system.Total = %d, want 2", sysGroup.Total)
	}
	if sysGroup.Detail["Bash"] != 2 {
		t.Errorf("system.Detail[Bash] = %d, want 2", sysGroup.Detail["Bash"])
	}

	if log.Interactions != 1 {
		t.Errorf("Interactions = %d, want 1", log.Interactions)
	}
}
