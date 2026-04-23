package drafter

import (
	"testing"
)

func TestSanitizeTurns_StripsToolUse(t *testing.T) {
	// Turn with mixed text + tool_use content
	turn := rawTurn{
		Text: `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Here is the fix"},{"type":"tool_use","id":"tu1","name":"Read","input":{"path":"/foo"}}]}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result))
	}
	if result[0].Text != "Here is the fix" {
		t.Errorf("expected cleaned text, got: %s", result[0].Text)
	}
}

func TestSanitizeTurns_StripsProgress(t *testing.T) {
	// Progress and system lines should be dropped
	turn := rawTurn{
		Text: `{"type":"progress","data":{"status":"running"}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done"}]}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result))
	}
	if result[0].Text != "Done" {
		t.Errorf("expected 'Done', got: %s", result[0].Text)
	}
}

func TestSanitizeTurns_PreservesUserText(t *testing.T) {
	turn := rawTurn{
		Text: `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Please fix the bug"}]}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result))
	}
	if result[0].Text != "Please fix the bug" {
		t.Errorf("expected user text, got: %s", result[0].Text)
	}
}

func TestSanitizeTurns_EmptyAfterSanitize(t *testing.T) {
	// Turn with only tool_use — should be dropped
	turn := rawTurn{
		Text: `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Write","input":{}}]}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 0 {
		t.Errorf("expected 0 turns, got %d", len(result))
	}
}

func TestSanitizeTurns_MalformedJSON(t *testing.T) {
	// Garbage lines should be kept as-is (fallback)
	turn := rawTurn{
		Text: "this is just plain text, not JSON",
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result))
	}
	if result[0].Text != "this is just plain text, not JSON" {
		t.Errorf("expected raw text preserved, got: %s", result[0].Text)
	}
}

func TestSanitizeTurns_FallbackKeepsRaw(t *testing.T) {
	// Unrecognized JSON format — keep raw
	turn := rawTurn{
		Text: `{"custom_field": "some value", "data": 123}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	// Unrecognized format with no type field should be dropped (no type == not assistant/user)
	// hasStructured=true, type is empty string != "assistant"/"user", so extracted is empty -> dropped
	if len(result) != 0 {
		t.Errorf("expected 0 turns (unknown structured JSON with no text), got %d", len(result))
	}
}

func TestSanitizeTurns_MultipleContentItems(t *testing.T) {
	turn := rawTurn{
		Text: `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"First part"},{"type":"text","text":"Second part"},{"type":"tool_use","id":"x","name":"Bash","input":{}}]}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result))
	}
	if result[0].Text != "First part\nSecond part" {
		t.Errorf("expected joined text, got: %s", result[0].Text)
	}
}

func TestSanitizeTurns_PreservesMetadata(t *testing.T) {
	// Verify that non-Text fields (Timestamp, SessionID, etc.) are preserved
	turn := rawTurn{
		Timestamp: "2026-04-22T12:00:00Z",
		Event:     "stop",
		SessionID: "session-123",
		Text:      `{"type":"user","content":[{"type":"text","text":"hello"}]}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result))
	}
	if result[0].Timestamp != "2026-04-22T12:00:00Z" {
		t.Error("timestamp not preserved")
	}
	if result[0].SessionID != "session-123" {
		t.Error("session ID not preserved")
	}
}
