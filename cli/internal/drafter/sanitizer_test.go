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

func TestSanitizeTurns_WindsurfUserInput(t *testing.T) {
	turn := rawTurn{
		Text: `{"status":"done","type":"user_input","user_input":{"rules_applied":{"always_on":["mom.md"]},"user_response":"the API deadline is May 15th"}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result))
	}
	if result[0].Text != "the API deadline is May 15th" {
		t.Errorf("expected user response, got: %s", result[0].Text)
	}
}

func TestSanitizeTurns_WindsurfPlannerResponse(t *testing.T) {
	turn := rawTurn{
		Text: `{"planner_response":{"response":"API deadline of May 15th has been recorded."},"status":"done","type":"planner_response"}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result))
	}
	if result[0].Text != "API deadline of May 15th has been recorded." {
		t.Errorf("expected planner response, got: %s", result[0].Text)
	}
}

func TestSanitizeTurns_WindsurfMCPToolDropped(t *testing.T) {
	turn := rawTurn{
		Text: `{"mcp_tool":{"tool_name":"mom_status","arguments":"{}","result":"..."},"status":"done","type":"mcp_tool"}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 0 {
		t.Errorf("expected 0 turns (mcp_tool should be dropped), got %d", len(result))
	}
}

func TestSanitizeTurns_WindsurfMixedSession(t *testing.T) {
	turn := rawTurn{
		Text: `{"status":"done","type":"user_input","user_input":{"user_response":"fix the auth bug"}}
{"mcp_tool":{"tool_name":"mom_recall","arguments":"{}","result":"..."},"status":"done","type":"mcp_tool"}
{"planner_response":{"response":"I found and fixed the auth bug in middleware.go"},"status":"done","type":"planner_response"}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result))
	}
	if result[0].Text != "fix the auth bug\nI found and fixed the auth bug in middleware.go" {
		t.Errorf("expected user+planner text, got: %s", result[0].Text)
	}
}

func TestSanitizeTurns_CodexUserMessage(t *testing.T) {
	turn := rawTurn{
		Text: `{"timestamp":"2026-04-23T17:16:58Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"What is MOM about?"}]}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result))
	}
	if result[0].Text != "What is MOM about?" {
		t.Errorf("expected user text, got: %s", result[0].Text)
	}
}

func TestSanitizeTurns_CodexAssistantMessage(t *testing.T) {
	turn := rawTurn{
		Text: `{"timestamp":"2026-04-23T17:17:00Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"MOM is a persistent memory layer for AI agents."}]}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result))
	}
	if result[0].Text != "MOM is a persistent memory layer for AI agents." {
		t.Errorf("expected assistant text, got: %s", result[0].Text)
	}
}

func TestSanitizeTurns_CodexReasoningDropped(t *testing.T) {
	turn := rawTurn{
		Text: `{"timestamp":"2026-04-23T17:17:00Z","type":"response_item","payload":{"type":"reasoning","content":[]}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 0 {
		t.Errorf("expected 0 turns (reasoning should be dropped), got %d", len(result))
	}
}

func TestSanitizeTurns_CodexFunctionCallDropped(t *testing.T) {
	turn := rawTurn{
		Text: `{"timestamp":"2026-04-23T17:17:00Z","type":"response_item","payload":{"type":"function_call","call_id":"call1","name":"shell","arguments":"{}"}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 0 {
		t.Errorf("expected 0 turns (function_call should be dropped), got %d", len(result))
	}
}

func TestSanitizeTurns_CodexDeveloperDropped(t *testing.T) {
	turn := rawTurn{
		Text: `{"timestamp":"2026-04-23T17:16:58Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"system instructions"}]}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 0 {
		t.Errorf("expected 0 turns (developer role should be dropped), got %d", len(result))
	}
}

func TestSanitizeTurns_CodexMetadataDropped(t *testing.T) {
	turn := rawTurn{
		Text: `{"timestamp":"2026-04-23T17:16:58Z","type":"session_meta","payload":{"id":"abc","cwd":"/tmp"}}
{"timestamp":"2026-04-23T17:16:58Z","type":"event_msg","payload":{"type":"turn_started"}}
{"timestamp":"2026-04-23T17:16:58Z","type":"turn_context","payload":{}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 0 {
		t.Errorf("expected 0 turns (metadata should be dropped), got %d", len(result))
	}
}

func TestSanitizeTurns_CodexMixedSession(t *testing.T) {
	turn := rawTurn{
		Text: `{"type":"session_meta","payload":{"id":"abc"}}
{"type":"event_msg","payload":{"type":"turn_started"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"What is MOM?"}]}}
{"type":"response_item","payload":{"type":"reasoning","content":[]}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"MOM is a memory layer."}]}}
{"type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{}"}}
{"type":"event_msg","payload":{"type":"turn_completed"}}`,
	}
	result := sanitizeTurns([]rawTurn{turn})
	if len(result) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result))
	}
	expected := "What is MOM?\nMOM is a memory layer."
	if result[0].Text != expected {
		t.Errorf("expected %q, got: %s", expected, result[0].Text)
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
