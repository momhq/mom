package watcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClaudeAdapter_ParseLine_UserTurn(t *testing.T) {
	a := NewClaudeAdapter()
	ts := time.Now().UTC().Format(time.RFC3339)

	line, _ := json.Marshal(map[string]any{
		"type":      "user",
		"sessionId": "sess-123",
		"timestamp": ts,
		"message": map[string]any{
			"role":    "user",
			"content": "What is the purpose of the watcher?",
		},
	})

	entry, ok := a.ParseLine(line, "fallback-id")
	if !ok {
		t.Fatal("expected ParseLine to return ok=true for user turn")
	}
	if entry.SessionID != "sess-123" {
		t.Errorf("expected session_id=sess-123, got %q", entry.SessionID)
	}
	if entry.Text != "What is the purpose of the watcher?" {
		t.Errorf("unexpected text: %q", entry.Text)
	}
	if entry.Event != "watch-user" {
		t.Errorf("expected event=watch-user, got %q", entry.Event)
	}
}

func TestClaudeAdapter_ParseLine_AssistantTurn_StructuredContent(t *testing.T) {
	a := NewClaudeAdapter()
	ts := time.Now().UTC().Format(time.RFC3339)

	line, _ := json.Marshal(map[string]any{
		"type":      "assistant",
		"sessionId": "sess-456",
		"timestamp": ts,
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "text", "text": "I process transcripts."},
				map[string]any{"type": "tool_use", "id": "t1", "name": "Read"},
			},
		},
	})

	entry, ok := a.ParseLine(line, "fallback")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Text != "I process transcripts." {
		t.Errorf("unexpected text: %q", entry.Text)
	}
	if entry.Event != "watch-assistant" {
		t.Errorf("expected event=watch-assistant, got %q", entry.Event)
	}
}

func TestClaudeAdapter_ParseLine_DropToolUse(t *testing.T) {
	a := NewClaudeAdapter()
	line, _ := json.Marshal(map[string]any{
		"type":      "tool_use",
		"sessionId": "sess-789",
		"message":   map[string]any{},
	})

	_, ok := a.ParseLine(line, "sess-789")
	if ok {
		t.Error("expected tool_use line to be dropped")
	}
}

func TestClaudeAdapter_ParseLine_DropSystem(t *testing.T) {
	a := NewClaudeAdapter()
	line, _ := json.Marshal(map[string]any{
		"type":    "system",
		"message": map[string]any{"role": "system", "content": "System prompt"},
	})

	_, ok := a.ParseLine(line, "s")
	if ok {
		t.Error("expected system line to be dropped")
	}
}

func TestClaudeAdapter_ParseLine_DropSidechain(t *testing.T) {
	a := NewClaudeAdapter()
	line, _ := json.Marshal(map[string]any{
		"type":        "assistant",
		"sessionId":   "sub-1",
		"isSidechain": true,
		"message": map[string]any{
			"role":    "assistant",
			"content": "Subagent output",
		},
	})

	_, ok := a.ParseLine(line, "sub-1")
	if ok {
		t.Error("expected sidechain (subagent) turn to be dropped")
	}
}

func TestClaudeAdapter_ParseLine_EmptyContent(t *testing.T) {
	a := NewClaudeAdapter()
	line, _ := json.Marshal(map[string]any{
		"type":      "assistant",
		"sessionId": "sess-empty",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "tool_use", "id": "t2", "name": "Bash"},
			},
		},
	})

	_, ok := a.ParseLine(line, "sess-empty")
	if ok {
		t.Error("expected all-tool_use content to yield ok=false (no text)")
	}
}

func TestClaudeAdapter_ParseLine_FallbackSessionID(t *testing.T) {
	a := NewClaudeAdapter()
	// No sessionId in line — should fall back to the argument.
	line, _ := json.Marshal(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": "Hello",
		},
	})

	entry, ok := a.ParseLine(line, "from-filename")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.SessionID != "from-filename" {
		t.Errorf("expected fallback session_id=from-filename, got %q", entry.SessionID)
	}
}

func TestClaudeAdapter_ParseLine_InvalidJSON(t *testing.T) {
	a := NewClaudeAdapter()
	_, ok := a.ParseLine([]byte("{not json"), "s")
	if ok {
		t.Error("expected invalid JSON to return ok=false")
	}
}

func TestClaudeAdapter_ParseLine_EmptyLine(t *testing.T) {
	a := NewClaudeAdapter()
	_, ok := a.ParseLine([]byte(""), "s")
	if ok {
		t.Error("expected empty line to return ok=false")
	}
}

func TestClaudeAdapter_Name(t *testing.T) {
	a := NewClaudeAdapter()
	if a.Name() != "claude" {
		t.Errorf("expected Name()=claude, got %q", a.Name())
	}
}

func TestClaudeAdapter_ParseSession(t *testing.T) {
	// Build a realistic Claude Code transcript JSONL.
	// Real format: role/content are nested inside "message", not at top level.
	// MCP tools use "mcp__mom__" prefix.
	lines := []map[string]any{
		{
			"type":      "assistant",
			"timestamp": "2026-01-15T10:00:00Z",
			"message": map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "text", "text": "Let me read that file."},
					map[string]any{"type": "tool_use", "name": "Read", "id": "t1", "input": map[string]any{"file_path": "/foo/bar.go"}},
				},
			},
		},
		{
			"type":      "assistant",
			"timestamp": "2026-01-15T10:01:00Z",
			"message": map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "text", "text": "I'll edit it."},
					map[string]any{"type": "tool_use", "name": "Edit", "id": "t2", "input": map[string]any{"file_path": "/foo/bar.go"}},
				},
			},
		},
		{
			"type":      "assistant",
			"timestamp": "2026-01-15T10:02:00Z",
			"message": map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "tool_use", "name": "mcp__mom__create_memory_draft", "id": "t3", "input": map[string]any{}},
				},
			},
		},
		{
			"type":      "assistant",
			"timestamp": "2026-01-15T10:03:00Z",
			"message": map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "tool_use", "name": "mcp__mom__mom_recall", "id": "t4", "input": map[string]any{}},
				},
			},
		},
	}

	dir := t.TempDir()
	tp := filepath.Join(dir, "sess-test.jsonl")
	f, _ := os.Create(tp)
	for _, l := range lines {
		data, _ := json.Marshal(l)
		f.Write(append(data, '\n'))
	}
	f.Close()

	a := NewClaudeAdapter()
	sl, err := a.ParseSession(tp, "sess-test")
	if err != nil {
		t.Fatalf("ParseSession error: %v", err)
	}
	if sl.SessionID != "sess-test" {
		t.Errorf("session_id: got %q, want sess-test", sl.SessionID)
	}
	if sl.Interactions != 4 {
		t.Errorf("interactions: got %d, want 4", sl.Interactions)
	}
	if sl.Started != "2026-01-15T10:00:00Z" {
		t.Errorf("started: got %q, want 2026-01-15T10:00:00Z", sl.Started)
	}
	if sl.FilesChanged != 1 {
		t.Errorf("files_changed: got %d, want 1", sl.FilesChanged)
	}
	if sl.MemoriesCreated != 1 {
		t.Errorf("memories_created: got %d, want 1", sl.MemoriesCreated)
	}
	if g, ok := sl.ToolCalls["codebase_read"]; !ok || g.Total != 1 {
		t.Errorf("codebase_read: got %+v, want total=1", g)
	}
	if g, ok := sl.ToolCalls["codebase_write"]; !ok || g.Total != 1 {
		t.Errorf("codebase_write: got %+v, want total=1", g)
	}
	// MCP-prefixed tools should be categorized correctly.
	if g, ok := sl.ToolCalls["mom_memory"]; !ok || g.Total != 2 {
		t.Errorf("mom_memory: got %+v, want total=2 (create_memory_draft + mom_recall)", g)
	}
}
