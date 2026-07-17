package watcher

import (
	"strings"
	"testing"
)

// Test fixtures: representative OATS (oats/1) transcript JSONL lines,
// modeled on a real momOS-written session. Real transcripts are
// single-line JSON per record (JSONL); fixtures follow that shape so
// they round-trip through the watcher's line-by-line reader correctly.

const oatsSessionHeaderLine = `{"schema":"oats/1","type":"session","id":"d3446c23-427f-485d-b183-3ccdc08f6b8b","harness":"momos","harness_version":"0.1.0","timestamp":"2026-07-17T22:36:39.056Z","cwd":"/Users/x/momhq/mom-os","platform":"macos","project":{"root":"/Users/x/momhq/mom-os","name":"mom-os"},"model":{"provider":"anthropic","id":"claude-sonnet-4-6"}}`

const oatsUserTurn = `{"schema":"oats/1","type":"message","seq":1,"turn":1,"session_id":"d3446c23-427f-485d-b183-3ccdc08f6b8b","timestamp":"2026-07-17T22:36:39.069Z","role":"user","content":[{"type":"text","text":"Tell me about this project"}],"x_momos_ui":{"id":"wlrRwN9TXqRLUBoU"}}`

const oatsAssistantToolCallTurn = `{"schema":"oats/1","type":"message","seq":3,"turn":1,"session_id":"d3446c23-427f-485d-b183-3ccdc08f6b8b","timestamp":"2026-07-17T22:36:54.758Z","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"tool_call","id":"toolu_1","name":"Read","arguments":{"file_path":"/Users/x/momhq/mom-os/README.md"}},{"type":"text","text":"Reading the project README."},{"type":"tool_call","id":"toolu_2","name":"Bash","arguments":{"command":"go test ./..."}}],"usage":{"input_tokens":1240,"output_tokens":38,"cache_read_tokens":800,"cache_write_tokens":0},"x_momos_partial":true}`

const oatsToolResultLine = `{"schema":"oats/1","type":"message","seq":4,"turn":1,"session_id":"d3446c23-427f-485d-b183-3ccdc08f6b8b","timestamp":"2026-07-17T22:36:54.766Z","role":"tool","call_id":"toolu_1","name":"Read","is_error":false,"content":[{"type":"text","text":"# mom-os"}],"x_momos_synthetic":true}`

const oatsEventLine = `{"schema":"oats/1","type":"event","seq":2,"turn":1,"session_id":"d3446c23-427f-485d-b183-3ccdc08f6b8b","timestamp":"2026-07-17T22:36:40.504Z","event":"delegation.spawned","payload":{"backend":"claude-code","child_session_id":"d66f90d2"}}`

const oatsUnknownTypeLine = `{"schema":"oats/1","type":"plan_state","seq":5,"session_id":"d3446c23-427f-485d-b183-3ccdc08f6b8b","timestamp":"2026-07-17T22:36:55.000Z","steps":["a","b"]}`

// headeredOats returns an adapter that has already seen the session
// header for the given session key, as happens when the watcher reads a
// transcript file from line 1.
func headeredOats(t *testing.T, sessionKey string) *OatsAdapter {
	t.Helper()
	a := NewOatsAdapter()
	if _, ok := a.ExtractTurn([]byte(oatsSessionHeaderLine), sessionKey); ok {
		t.Fatal("session header line must not produce a Turn")
	}
	return a
}

func TestOatsAdapter_ExtractTurn_UserText(t *testing.T) {
	a := headeredOats(t, "s-key")
	turn, ok := a.ExtractTurn([]byte(oatsUserTurn), "s-key")
	if !ok {
		t.Fatal("expected ok=true for user turn")
	}
	if turn.SessionID != "d3446c23-427f-485d-b183-3ccdc08f6b8b" {
		t.Errorf("SessionID = %q, want header session id (from line)", turn.SessionID)
	}
	if turn.Role != "user" {
		t.Errorf("Role = %q, want user", turn.Role)
	}
	if turn.Text != "Tell me about this project" {
		t.Errorf("Text = %q", turn.Text)
	}
	if len(turn.ToolCalls) != 0 {
		t.Errorf("user turn should have no tool calls, got %v", turn.ToolCalls)
	}
	if turn.Usage != nil {
		t.Errorf("user turn should have no usage, got %+v", turn.Usage)
	}
}

// The source harness is attributed from the session header's `harness`
// field — the adapter is a generic OATS reader and must not hardcode any
// single writer.
func TestOatsAdapter_ExtractTurn_AttributesHarnessFromHeader(t *testing.T) {
	a := headeredOats(t, "s-key")
	turn, ok := a.ExtractTurn([]byte(oatsUserTurn), "s-key")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if turn.Harness != "momos" {
		t.Errorf("Harness = %q, want momos (from header, not hardcoded)", turn.Harness)
	}
	if turn.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic (from header model.provider)", turn.Provider)
	}
}

// The header's cwd must be stamped on every Turn so the watcher can
// resolve the correct project_id — ~/.transcripts/ is shared across
// projects (Codex-adapter parity).
func TestOatsAdapter_ExtractTurn_StampsHeaderCwd(t *testing.T) {
	a := headeredOats(t, "s-key")
	turn, ok := a.ExtractTurn([]byte(oatsUserTurn), "s-key")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if turn.Cwd != "/Users/x/momhq/mom-os" {
		t.Errorf("Turn.Cwd = %q, want header cwd (needed for project_id resolution)", turn.Cwd)
	}
}

// When the header was never seen (watcher resumed mid-file from a
// cursor), the adapter attributes generically to "oats" rather than
// inventing a writer, and leaves cwd empty.
func TestOatsAdapter_ExtractTurn_FallsBackWithoutHeader(t *testing.T) {
	a := NewOatsAdapter()
	turn, ok := a.ExtractTurn([]byte(oatsUserTurn), "s-key")
	if !ok {
		t.Fatal("expected ok=true even without a cached header")
	}
	if turn.Harness != "oats" {
		t.Errorf("Harness = %q, want oats fallback", turn.Harness)
	}
	if turn.Provider != "" {
		t.Errorf("Provider = %q, want empty (unknown without header)", turn.Provider)
	}
	if turn.Cwd != "" {
		t.Errorf("Cwd = %q, want empty (adapter must not synthesise cwd)", turn.Cwd)
	}
}

func TestOatsAdapter_ExtractTurn_AssistantWithToolCallsAndUsage(t *testing.T) {
	a := headeredOats(t, "s-key")
	turn, ok := a.ExtractTurn([]byte(oatsAssistantToolCallTurn), "s-key")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if turn.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", turn.Role)
	}
	if turn.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q (per-message model must win)", turn.Model)
	}
	if turn.Text != "Reading the project README." {
		t.Errorf("Text = %q (text blocks concatenated, tool_call blocks excluded)", turn.Text)
	}
	if len(turn.ToolCalls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(turn.ToolCalls))
	}
	read := turn.ToolCalls[0]
	if read.Name != "Read" {
		t.Errorf("ToolCalls[0].Name = %q, want Read", read.Name)
	}
	if read.Category != "codebase_read" {
		t.Errorf("ToolCalls[0].Category = %q, want codebase_read", read.Category)
	}
	if got, _ := read.Input["file_path"].(string); got != "/Users/x/momhq/mom-os/README.md" {
		t.Errorf("Read.Input.file_path = %q (full arguments must be preserved)", got)
	}
	bash := turn.ToolCalls[1]
	if bash.Name != "Bash" {
		t.Errorf("ToolCalls[1].Name = %q, want Bash", bash.Name)
	}
	if bash.Category != "system" {
		t.Errorf("ToolCalls[1].Category = %q, want system", bash.Category)
	}
	if turn.Usage == nil {
		t.Fatal("Usage should be non-nil for assistant turn with usage block")
	}
	if turn.Usage.InputTokens != 1240 {
		t.Errorf("InputTokens = %d, want 1240", turn.Usage.InputTokens)
	}
	if turn.Usage.OutputTokens != 38 {
		t.Errorf("OutputTokens = %d, want 38", turn.Usage.OutputTokens)
	}
	if turn.Usage.CacheReadTokens != 800 {
		t.Errorf("CacheReadTokens = %d, want 800", turn.Usage.CacheReadTokens)
	}
	if turn.Usage.TotalTokens != 1278 {
		t.Errorf("TotalTokens = %d, want 1278", turn.Usage.TotalTokens)
	}
}

// Table-driven: lines that must never produce a Turn.
func TestOatsAdapter_ExtractTurn_SkippedLines(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"session header", oatsSessionHeaderLine},
		{"tool result (role tool — Turn model has no tool-result slot)", oatsToolResultLine},
		{"momos event line (unknown type per OATS §2)", oatsEventLine},
		{"unknown extension type", oatsUnknownTypeLine},
		{"malformed JSON", `{"type":"message","role":"user","content":`},
		{"not JSON at all", `not valid json`},
		{"empty line", ``},
		{"whitespace only", `   `},
		{"message with unrecognized role", `{"type":"message","role":"system","content":[{"type":"text","text":"x"}]}`},
		{"message with no text and no tool calls", `{"type":"message","role":"user","content":[{"type":"image","url":"https://x/y.png"}]}`},
		{"content as unexpected scalar", `{"type":"message","role":"user","content":"plain string"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := headeredOats(t, "s-key")
			if turn, ok := a.ExtractTurn([]byte(tt.line), "s-key"); ok {
				t.Fatalf("line should be skipped, got Turn %+v", turn)
			}
		})
	}
}

func TestOatsAdapter_ExtractTurn_FallsBackToSessionIDArg(t *testing.T) {
	line := `{"type":"message","role":"user","content":[{"type":"text","text":"hi"}]}`
	a := NewOatsAdapter()
	turn, ok := a.ExtractTurn([]byte(line), "s-from-filename")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if turn.SessionID != "s-from-filename" {
		t.Errorf("SessionID = %q, want fallback s-from-filename", turn.SessionID)
	}
}

// Golden momOS-style transcript: header + user + event + assistant with
// tool_call + synthesized tool results, fed through the adapter in file
// order exactly as the watcher's line reader would.
func TestOatsAdapter_ExtractTurn_GoldenMomosTranscript(t *testing.T) {
	transcript := strings.Join([]string{
		oatsSessionHeaderLine,
		oatsUserTurn,
		oatsEventLine,
		oatsAssistantToolCallTurn,
		oatsToolResultLine,
		oatsUnknownTypeLine,
	}, "\n")

	a := NewOatsAdapter()
	var turns []Turn
	for _, line := range strings.Split(transcript, "\n") {
		if turn, ok := a.ExtractTurn([]byte(line), "2026-07-17T22-36-39-d3446c23"); ok {
			turns = append(turns, turn)
		}
	}

	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2 (user + assistant; header/event/tool/unknown skipped)", len(turns))
	}
	if turns[0].Role != "user" || turns[1].Role != "assistant" {
		t.Errorf("roles = [%s, %s], want [user, assistant]", turns[0].Role, turns[1].Role)
	}
	for i, turn := range turns {
		if turn.Harness != "momos" {
			t.Errorf("turns[%d].Harness = %q, want momos (attributed from header)", i, turn.Harness)
		}
		if turn.Cwd != "/Users/x/momhq/mom-os" {
			t.Errorf("turns[%d].Cwd = %q, want header cwd", i, turn.Cwd)
		}
		if turn.Timestamp.IsZero() {
			t.Errorf("turns[%d].Timestamp is zero, want line timestamp", i)
		}
	}
	if turns[1].Timestamp.Format("2006-01-02T15:04:05") != "2026-07-17T22:36:54" {
		t.Errorf("assistant Timestamp = %v, want line timestamp preserved", turns[1].Timestamp)
	}
}

// ProjectSlug must follow the OATS convention (lowercase basename,
// alphanumeric and '-' only, max 64 chars) — not the Claude/Codex
// full-path slug — so the watcher's scoping finds the real subdirectory.
func TestOatsAdapter_ProjectSlug(t *testing.T) {
	a := NewOatsAdapter()
	tests := []struct {
		dir  string
		want string
	}{
		{"/Users/x/momhq/mom-os", "mom-os"},
		{"/Users/x/projects/My-App", "my-app"},
		{"/Users/x/projects/dotted.name", "dotted-name"},
		{"/Users/x/projects/under_score", "under-score"},
		{"/Users/x/" + strings.Repeat("a", 80), strings.Repeat("a", 64)},
	}
	for _, tt := range tests {
		if got := a.ProjectSlug(tt.dir); got != tt.want {
			t.Errorf("ProjectSlug(%q) = %q, want %q", tt.dir, got, tt.want)
		}
	}
}
