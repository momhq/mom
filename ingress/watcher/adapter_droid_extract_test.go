package watcher

import (
	"testing"
)

// Fixture lines mirror a real ~/.factory/sessions/<slug>/<uuid>.jsonl
// transcript observed on disk (session_start header, plain text turns,
// tool_use, tool_result-only user turn).
const droidSessionStartLine = `{"type":"session_start","id":"5e90e310-629a-48fc-8864-dd92f3e17b77","title":"could you use mgrep skill?","owner":"khunglong","version":2,"cwd":"/Users/khunglong/proj"}`

const droidUserMessage = `{"type":"message","id":"c0842aa6","timestamp":"2026-05-05T12:00:00.441Z","message":{"role":"user","content":[{"type":"text","text":"deploy postgres canary"}]}}`

const droidAssistantMessage = `{"type":"message","id":"5b5ca656","timestamp":"2026-05-05T12:00:01.909Z","parentId":"c0842aa6","message":{"role":"assistant","content":[{"type":"text","text":"I'll deploy now."},{"type":"tool_use","id":"call_5odyrh9sh3a","name":"Bash","input":{"command":"ls"}}]}}`

const droidToolResultMessage = `{"type":"message","id":"f2ea6894","timestamp":"2026-05-05T12:00:02.923Z","parentId":"5b5ca656","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_5odyrh9sh3a","content":"file1\nfile2"}]}}`

// Real session-start context injection observed via a live `droid exec`
// call: an ordinary role:"user" message whose only marker is
// visibility:"llm_only" on the message object.
const droidContextMessage = `{"type":"message","id":"context-d58c7cd4","timestamp":"2026-08-14T04:19:35.496Z","message":{"role":"user","content":[{"type":"text","text":"<system-reminder>tool catalog...</system-reminder>"}],"visibility":"llm_only"}}`

func TestDroidAdapter_ExtractTurn_UserMessage(t *testing.T) {
	a := NewDroidAdapter()
	turn, ok := a.ExtractTurn([]byte(droidUserMessage), "s-droid")
	if !ok {
		t.Fatal("expected ok=true for user message")
	}
	if turn.SessionID != "s-droid" {
		t.Errorf("SessionID = %q, want s-droid", turn.SessionID)
	}
	if turn.Role != "user" {
		t.Errorf("Role = %q, want user", turn.Role)
	}
	if turn.Text != "deploy postgres canary" {
		t.Errorf("Text = %q", turn.Text)
	}
	if turn.Harness != "droid" {
		t.Errorf("Harness = %q, want droid", turn.Harness)
	}
}

func TestDroidAdapter_ExtractTurn_AssistantWithToolUse(t *testing.T) {
	a := NewDroidAdapter()
	turn, ok := a.ExtractTurn([]byte(droidAssistantMessage), "s-droid")
	if !ok {
		t.Fatal("expected ok=true for assistant message")
	}
	if turn.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", turn.Role)
	}
	if turn.Text != "I'll deploy now." {
		t.Errorf("Text = %q", turn.Text)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(turn.ToolCalls))
	}
	if turn.ToolCalls[0].Name != "Bash" {
		t.Errorf("ToolCalls[0].Name = %q, want Bash", turn.ToolCalls[0].Name)
	}
}

// A user line whose only content is a tool_result block is Droid
// returning results, not the human speaking — must surface as role:"tool".
func TestDroidAdapter_ExtractTurn_ToolResultOnlyBecomesToolRole(t *testing.T) {
	a := NewDroidAdapter()
	turn, ok := a.ExtractTurn([]byte(droidToolResultMessage), "s-droid")
	if !ok {
		t.Fatal("expected ok=true for tool_result message")
	}
	if turn.Role != "tool" {
		t.Errorf("Role = %q, want tool", turn.Role)
	}
	if len(turn.ToolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(turn.ToolResults))
	}
	if turn.ToolResults[0].CallID != "call_5odyrh9sh3a" {
		t.Errorf("CallID = %q, want call_5odyrh9sh3a", turn.ToolResults[0].CallID)
	}
	if turn.ToolResults[0].Content != "file1\nfile2" {
		t.Errorf("Content = %q", turn.ToolResults[0].Content)
	}
	if turn.ToolResults[0].IsError {
		t.Error("IsError should default to false (Droid transcripts carry no is_error field)")
	}
}

func TestDroidAdapter_ExtractTurn_DropsLLMOnlyContextMessage(t *testing.T) {
	a := NewDroidAdapter()
	if _, ok := a.ExtractTurn([]byte(droidContextMessage), "s-droid"); ok {
		t.Error("expected ok=false for visibility:llm_only context message")
	}
}

func TestDroidAdapter_ExtractTurn_DropsSessionStartLine(t *testing.T) {
	a := NewDroidAdapter()
	if _, ok := a.ExtractTurn([]byte(droidSessionStartLine), "s-droid"); ok {
		t.Error("expected ok=false for session_start line")
	}
}

func TestDroidAdapter_ExtractTurn_RejectsMalformedJSON(t *testing.T) {
	a := NewDroidAdapter()
	if _, ok := a.ExtractTurn([]byte(`not-json`), "s-droid"); ok {
		t.Error("expected ok=false for malformed JSON")
	}
}

// Line timestamp must win over time.Now() so catch-up reads preserve the
// turn's real time (mirrors the Claude/Pi/Codex adapters).
func TestDroidAdapter_ExtractTurn_PrefersLineTimestamp(t *testing.T) {
	a := NewDroidAdapter()
	turn, ok := a.ExtractTurn([]byte(droidUserMessage), "s-droid")
	if !ok {
		t.Fatal("expected ok=true")
	}
	want := "2026-05-05T12:00:00.441Z"
	if got := turn.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z"); got != want {
		t.Errorf("Timestamp = %q, want %q (line timestamp must win over time.Now())", got, want)
	}
}

func TestDroidAdapter_Name(t *testing.T) {
	a := NewDroidAdapter()
	if a.Name() != "droid" {
		t.Errorf("Name() = %q, want droid", a.Name())
	}
}
