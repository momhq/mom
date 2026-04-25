package watcher

import (
	"encoding/json"
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
