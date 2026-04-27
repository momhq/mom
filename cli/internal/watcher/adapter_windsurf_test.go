package watcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWindsurfAdapter_Name(t *testing.T) {
	a := NewWindsurfAdapter()
	if a.Name() != "windsurf" {
		t.Errorf("expected Name()=windsurf, got %q", a.Name())
	}
}

func TestWindsurfAdapter_ParseLine_UserInput(t *testing.T) {
	a := NewWindsurfAdapter()

	line, _ := json.Marshal(map[string]any{
		"status": "done",
		"type":   "user_input",
		"user_input": map[string]any{
			"user_response": "How do I implement a binary search?",
			"rules_applied": map[string]any{"always_on": []string{"rule.md"}},
		},
	})

	entry, ok := a.ParseLine(line, "traj-abc123")
	if !ok {
		t.Fatal("expected ParseLine to return ok=true for user_input")
	}
	if entry.SessionID != "traj-abc123" {
		t.Errorf("expected session_id=traj-abc123, got %q", entry.SessionID)
	}
	if entry.Text != "How do I implement a binary search?" {
		t.Errorf("unexpected text: %q", entry.Text)
	}
	if entry.Event != "watch-user" {
		t.Errorf("expected event=watch-user, got %q", entry.Event)
	}
	if entry.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestWindsurfAdapter_ParseLine_PlannerResponse(t *testing.T) {
	a := NewWindsurfAdapter()

	line, _ := json.Marshal(map[string]any{
		"status": "done",
		"type":   "planner_response",
		"planner_response": map[string]any{
			"response": "Binary search works by repeatedly halving the search space.",
		},
	})

	entry, ok := a.ParseLine(line, "traj-def456")
	if !ok {
		t.Fatal("expected ParseLine to return ok=true for planner_response")
	}
	if entry.SessionID != "traj-def456" {
		t.Errorf("expected session_id=traj-def456, got %q", entry.SessionID)
	}
	if entry.Text != "Binary search works by repeatedly halving the search space." {
		t.Errorf("unexpected text: %q", entry.Text)
	}
	if entry.Event != "watch-assistant" {
		t.Errorf("expected event=watch-assistant, got %q", entry.Event)
	}
}

func TestWindsurfAdapter_ParseLine_DropCodeAction(t *testing.T) {
	a := NewWindsurfAdapter()

	line, _ := json.Marshal(map[string]any{
		"status": "done",
		"type":   "code_action",
		"code_action": map[string]any{
			"new_content": "def hello(): pass",
			"path":        "/project/hello.py",
		},
	})

	_, ok := a.ParseLine(line, "traj-xxx")
	if ok {
		t.Error("expected code_action to be dropped (ok=false)")
	}
}

func TestWindsurfAdapter_ParseLine_DropCommandAction(t *testing.T) {
	a := NewWindsurfAdapter()

	line, _ := json.Marshal(map[string]any{
		"status": "done",
		"type":   "command_action",
		"command_action": map[string]any{
			"command": "npm test",
			"output":  "All tests passed",
		},
	})

	_, ok := a.ParseLine(line, "traj-xxx")
	if ok {
		t.Error("expected command_action to be dropped (ok=false)")
	}
}

func TestWindsurfAdapter_ParseLine_DropFileHistorySnapshot(t *testing.T) {
	a := NewWindsurfAdapter()

	line, _ := json.Marshal(map[string]any{
		"status": "done",
		"type":   "file-history-snapshot",
	})

	_, ok := a.ParseLine(line, "traj-xxx")
	if ok {
		t.Error("expected file-history-snapshot to be dropped (ok=false)")
	}
}

func TestWindsurfAdapter_ParseLine_DropHookProgress(t *testing.T) {
	a := NewWindsurfAdapter()

	line, _ := json.Marshal(map[string]any{
		"status": "in_progress",
		"type":   "hook_progress",
	})

	_, ok := a.ParseLine(line, "traj-xxx")
	if ok {
		t.Error("expected hook_progress to be dropped (ok=false)")
	}
}

func TestWindsurfAdapter_ParseLine_EmptyUserResponse(t *testing.T) {
	a := NewWindsurfAdapter()

	line, _ := json.Marshal(map[string]any{
		"status": "done",
		"type":   "user_input",
		"user_input": map[string]any{
			"user_response": "   ",
		},
	})

	_, ok := a.ParseLine(line, "traj-xxx")
	if ok {
		t.Error("expected whitespace-only user_response to be dropped (ok=false)")
	}
}

func TestWindsurfAdapter_ParseLine_EmptyPlannerResponse(t *testing.T) {
	a := NewWindsurfAdapter()

	line, _ := json.Marshal(map[string]any{
		"status": "done",
		"type":   "planner_response",
		"planner_response": map[string]any{
			"response": "",
		},
	})

	_, ok := a.ParseLine(line, "traj-xxx")
	if ok {
		t.Error("expected empty planner response to be dropped (ok=false)")
	}
}

func TestWindsurfAdapter_ParseLine_MissingUserInputField(t *testing.T) {
	a := NewWindsurfAdapter()

	// type=user_input but no user_input payload
	line, _ := json.Marshal(map[string]any{
		"status": "done",
		"type":   "user_input",
	})

	_, ok := a.ParseLine(line, "traj-xxx")
	if ok {
		t.Error("expected missing user_input field to be dropped (ok=false)")
	}
}

func TestWindsurfAdapter_ParseLine_MissingPlannerResponseField(t *testing.T) {
	a := NewWindsurfAdapter()

	// type=planner_response but no planner_response payload
	line, _ := json.Marshal(map[string]any{
		"status": "done",
		"type":   "planner_response",
	})

	_, ok := a.ParseLine(line, "traj-xxx")
	if ok {
		t.Error("expected missing planner_response field to be dropped (ok=false)")
	}
}

func TestWindsurfAdapter_ParseLine_InvalidJSON(t *testing.T) {
	a := NewWindsurfAdapter()
	_, ok := a.ParseLine([]byte("{not json"), "traj-xxx")
	if ok {
		t.Error("expected invalid JSON to return ok=false")
	}
}

func TestWindsurfAdapter_ParseLine_EmptyLine(t *testing.T) {
	a := NewWindsurfAdapter()
	_, ok := a.ParseLine([]byte(""), "traj-xxx")
	if ok {
		t.Error("expected empty line to return ok=false")
	}
}

func TestWindsurfAdapter_ParseLine_WhitespaceOnlyLine(t *testing.T) {
	a := NewWindsurfAdapter()
	_, ok := a.ParseLine([]byte("   \t  "), "traj-xxx")
	if ok {
		t.Error("expected whitespace-only line to return ok=false")
	}
}

func TestWindsurfAdapter_ParseSession(t *testing.T) {
	lines := []map[string]any{
		{"type": "planner_response", "status": "done", "planner_response": map[string]any{"response": "I'll fix the bug."}},
		{"type": "planner_response", "status": "done", "planner_response": map[string]any{"response": "Done fixing."}},
		{"type": "code_action", "status": "done", "code_action": map[string]any{"path": "/proj/main.go", "new_content": "package main"}},
		{"type": "code_action", "status": "done", "code_action": map[string]any{"path": "/proj/util.go", "new_content": "package util"}},
		{"type": "command_action", "status": "done", "command_action": map[string]any{"command": "go test ./..."}},
		{"type": "mcp_tool", "status": "done", "mcp_tool": map[string]any{"tool_name": "mom_recall", "result": "..."}},
		{"type": "mcp_tool", "status": "done", "mcp_tool": map[string]any{"tool_name": "create_memory_draft", "result": "ok"}},
		{"type": "user_input", "status": "done", "user_input": map[string]any{"user_response": "looks good"}},
	}

	dir := t.TempDir()
	tp := filepath.Join(dir, "traj-abc.jsonl")
	f, _ := os.Create(tp)
	for _, l := range lines {
		data, _ := json.Marshal(l)
		f.Write(append(data, '\n'))
	}
	f.Close()

	a := NewWindsurfAdapter()
	sl, err := a.ParseSession(tp, "traj-abc")
	if err != nil {
		t.Fatalf("ParseSession error: %v", err)
	}

	if sl.SessionID != "traj-abc" {
		t.Errorf("session_id: got %q, want traj-abc", sl.SessionID)
	}
	if sl.Interactions != 2 {
		t.Errorf("interactions: got %d, want 2 (planner_response count)", sl.Interactions)
	}
	if sl.FilesChanged != 2 {
		t.Errorf("files_changed: got %d, want 2", sl.FilesChanged)
	}
	if sl.MemoriesCreated != 1 {
		t.Errorf("memories_created: got %d, want 1", sl.MemoriesCreated)
	}

	// code_action → codebase_write
	if g, ok := sl.ToolCalls["codebase_write"]; !ok || g.Total != 2 {
		t.Errorf("codebase_write: got %+v, want total=2", g)
	}
	// command_action → system
	if g, ok := sl.ToolCalls["system"]; !ok || g.Total != 1 {
		t.Errorf("system: got %+v, want total=1", g)
	}
	// mom_recall → mom_memory
	if g, ok := sl.ToolCalls["mom_memory"]; !ok || g.Total != 2 {
		t.Errorf("mom_memory: got %+v, want total=2 (mom_recall + create_memory_draft)", g)
	}

	// Timestamps should be set.
	if sl.Started == "" {
		t.Error("expected non-empty Started")
	}
	if sl.Ended == "" {
		t.Error("expected non-empty Ended")
	}
}
