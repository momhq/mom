package watcher

import (
	"encoding/json"
	"strings"
	"time"
)

// ClaudeAdapter parses Claude Code JSONL transcript lines.
// Claude Code writes one JSON object per line with the schema:
//
//	{ type, message: { role, content, model, usage }, timestamp, sessionId, uuid, cwd, gitBranch, isSidechain }
//
// We keep only type=="user" and type=="assistant" entries; everything else
// (system, hook_progress) is dropped. tool_result content blocks (Claude
// writes them on type=="user" lines) are extracted as ToolResults;
// result-only lines surface as role:"tool" turns.
type ClaudeAdapter struct{}

// NewClaudeAdapter returns a new ClaudeAdapter.
func NewClaudeAdapter() *ClaudeAdapter {
	return &ClaudeAdapter{}
}

func (a *ClaudeAdapter) Name() string { return "claude" }

// claudeTranscriptLine is the minimal subset of a Claude Code JSONL line
// that the adapter needs to inspect.
type claudeTranscriptLine struct {
	Type        string        `json:"type"`
	Message     claudeMessage `json:"message"`
	Timestamp   string        `json:"timestamp"`
	SessionID   string        `json:"sessionId"`
	Cwd         string        `json:"cwd"`
	IsSidechain bool          `json:"isSidechain"`
}

type claudeMessage struct {
	Role       string       `json:"role"`
	Model      string       `json:"model,omitempty"`
	Content    any          `json:"content"` // string or []claudeContentItem
	Usage      *claudeUsage `json:"usage,omitempty"`
	StopReason string       `json:"stop_reason,omitempty"`
}

type claudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// ExtractTurn implements Adapter. Returns the rich per-turn shape the
// Editor canonicalizes into a `turn.observed` event and appends to the
// Ledger. Raw text and tool inputs are carried on the event; read paths
// (fold, lens) decide what to surface.
func (a *ClaudeAdapter) ExtractTurn(line []byte, sessionID string) (Turn, bool) {
	line = trimLine(line)
	if len(line) == 0 {
		return Turn{}, false
	}
	var tl claudeTranscriptLine
	if err := json.Unmarshal(line, &tl); err != nil {
		return Turn{}, false
	}
	if tl.Type != "user" && tl.Type != "assistant" {
		return Turn{}, false
	}
	if tl.IsSidechain {
		return Turn{}, false
	}

	turn := Turn{
		SessionID: tl.SessionID,
		Role:      tl.Type,
		Model:     tl.Message.Model,
		Provider:  "anthropic",
		Harness:   "claude-code",
		Cwd:       tl.Cwd,
	}
	if turn.SessionID == "" {
		turn.SessionID = sessionID
	}

	// Timestamp: prefer line's, fall back to now.
	if tl.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, tl.Timestamp); err == nil {
			turn.Timestamp = t
		} else if t, err := time.Parse(time.RFC3339, tl.Timestamp); err == nil {
			turn.Timestamp = t
		}
	}
	if turn.Timestamp.IsZero() {
		turn.Timestamp = time.Now().UTC()
	}

	// Text + tool calls + tool results: walk the structured content blocks.
	turn.Text, turn.ToolCalls, turn.ToolResults = walkClaudeContent(tl.Message.Content)

	// A user line whose only content is tool_result blocks is the harness
	// returning results, not the human speaking — surface it as a
	// role:"tool" turn so consumers can attribute it correctly.
	if turn.Role == "user" && turn.Text == "" && len(turn.ToolCalls) == 0 && len(turn.ToolResults) > 0 {
		turn.Role = "tool"
	}

	// Usage: lift from message.usage if present.
	if tl.Message.Usage != nil {
		u := &Usage{
			InputTokens:      tl.Message.Usage.InputTokens,
			OutputTokens:     tl.Message.Usage.OutputTokens,
			CacheReadTokens:  tl.Message.Usage.CacheReadInputTokens,
			CacheWriteTokens: tl.Message.Usage.CacheCreationInputTokens,
			StopReason:       tl.Message.StopReason,
		}
		u.TotalTokens = u.InputTokens + u.OutputTokens
		turn.Usage = u
	}

	// Drop turns that carry nothing at all (no text, no tool calls, no
	// tool results). They carry no signal for the Ledger or the vault.
	if turn.Text == "" && len(turn.ToolCalls) == 0 && len(turn.ToolResults) == 0 {
		return Turn{}, false
	}

	return turn, true
}

// walkClaudeContent traverses message.content (string or array of
// blocks) and returns:
//   - the concatenated text from text-typed blocks
//   - the tool calls extracted from tool_use-typed blocks (with
//     pre-computed Category)
//   - the tool results extracted from tool_result-typed blocks
//     (content truncated, error state preserved)
//
// Other block types (image, etc.) are ignored.
func walkClaudeContent(content any) (string, []ToolCall, []ToolResult) {
	if content == nil {
		return "", nil, nil
	}
	if s, ok := content.(string); ok {
		return strings.TrimSpace(s), nil, nil
	}
	items, ok := content.([]any)
	if !ok {
		return "", nil, nil
	}
	var (
		textParts []string
		tcs       []ToolCall
		trs       []ToolResult
	)
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t, _ := m["type"].(string)
		switch t {
		case "text":
			if text, _ := m["text"].(string); text != "" {
				textParts = append(textParts, text)
			}
		case "tool_use":
			name, _ := m["name"].(string)
			if name == "" {
				continue
			}
			input, _ := m["input"].(map[string]any)
			category, safeName := CategorizeObservedToolCall(name, input)
			tcs = append(tcs, ToolCall{
				Name:     name,
				SafeName: safeName,
				Input:    input,
				Category: category,
			})
		case "tool_result":
			callID, _ := m["tool_use_id"].(string)
			isError, _ := m["is_error"].(bool)
			trs = append(trs, ToolResult{
				CallID:  callID,
				Content: truncateToolResult(claudeToolResultText(m["content"])),
				IsError: isError,
			})
		}
	}
	return strings.Join(textParts, "\n"), tcs, trs
}

// claudeToolResultText flattens a tool_result block's content — a plain
// string or an array of text-typed blocks — to text.
func claudeToolResultText(content any) string {
	if content == nil {
		return ""
	}
	if s, ok := content.(string); ok {
		return s
	}
	items, ok := content.([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t != "text" {
			continue
		}
		if text, _ := m["text"].(string); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// trimLine removes leading/trailing whitespace from a byte slice.
func trimLine(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
