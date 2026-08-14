package watcher

import (
	"encoding/json"
	"strings"
	"time"
)

// DroidAdapter parses Factory Droid (https://factory.ai) JSONL session
// transcripts.
//
// Droid writes one JSON object per line to
//
//	~/.factory/sessions/<project-slug>/<session-uuid>.jsonl
//
// The project slug uses the same "/" → "-" convention as Claude Code and
// Codex (confirmed against real on-disk transcripts), so the existing
// projectSlug() scoping logic in watcher.go applies unchanged — no
// ProjectScoper override needed.
//
// Line schema (confirmed against real transcripts on disk):
//
//	{"type":"session_start","id":"...","title":"...","owner":"...","version":2,"cwd":"..."}
//	{
//	  "type":      "message",
//	  "id":        "<uuid>",
//	  "parentId":  "<uuid>",
//	  "timestamp": "2026-04-28T00:11:01.063Z",
//	  "message": {
//	    "role":    "user" | "assistant",
//	    "content": [ {type:"text",text:"..."} | {type:"tool_use",id,name,input} |
//	                 {type:"tool_result",tool_use_id,content} | {type:"thinking",...} | {type:"image",...} ]
//	  }
//	}
//
// Unlike Claude Code, a Droid tool_result's "content" is always a plain
// string (never an array of blocks), and carries no "is_error" field on
// any transcript observed — IsError is left false. Model/usage are not
// present on the transcript line; they live in a sibling
// <session-uuid>.settings.json file this adapter does not read.
//
// The first message of every session is a synthetic context injection
// (system reminders, tool/skill/subagent catalogs) written as an ordinary
// role:"user" message, but with "visibility":"llm_only" set on the
// message object — confirmed via a real `droid exec` invocation. This is
// dropped, not ingested as a user turn.
type DroidAdapter struct{}

// NewDroidAdapter returns a new DroidAdapter.
func NewDroidAdapter() *DroidAdapter {
	return &DroidAdapter{}
}

func (a *DroidAdapter) Name() string { return "droid" }

type droidTranscriptLine struct {
	Type      string       `json:"type"`
	Timestamp string       `json:"timestamp"`
	Message   droidMessage `json:"message"`
}

type droidMessage struct {
	Role       string `json:"role"`
	Content    []any  `json:"content"`
	Visibility string `json:"visibility"`
}

// ExtractTurn implements Adapter. Returns the rich per-turn shape the
// Editor canonicalizes into a turn.observed event.
func (a *DroidAdapter) ExtractTurn(line []byte, sessionID string) (Turn, bool) {
	line = trimLine(line)
	if len(line) == 0 {
		return Turn{}, false
	}

	var tl droidTranscriptLine
	if err := json.Unmarshal(line, &tl); err != nil {
		return Turn{}, false
	}

	// Drop everything except conversational message lines (session_start
	// carries no turn).
	if tl.Type != "message" {
		return Turn{}, false
	}
	if tl.Message.Role != "user" && tl.Message.Role != "assistant" {
		return Turn{}, false
	}
	// The session-start context injection (system reminders, tool/skill
	// catalogs) is written as an ordinary role:"user" message but tagged
	// visibility:"llm_only" — confirmed on a real transcript. It is Droid's
	// own scaffolding, not something the human said; ingesting it would
	// flood the Ledger with the entire system prompt on every session.
	if tl.Message.Visibility == "llm_only" {
		return Turn{}, false
	}

	turn := Turn{
		SessionID: sessionID,
		Role:      tl.Message.Role,
		Harness:   "droid",
	}

	// Timestamp: prefer the line's timestamp so catch-up reads preserve
	// the turn's real time. Mirrors the Claude/Pi adapters.
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

	turn.Text, turn.ToolCalls, turn.ToolResults = walkDroidContent(tl.Message.Content)

	// A user line whose only content is tool_result blocks is Droid
	// returning results, not the human speaking — surface it as a
	// role:"tool" turn, mirroring the Claude adapter's convention.
	if turn.Role == "user" && turn.Text == "" && len(turn.ToolCalls) == 0 && len(turn.ToolResults) > 0 {
		turn.Role = "tool"
	}

	if turn.Text == "" && len(turn.ToolCalls) == 0 && len(turn.ToolResults) == 0 {
		return Turn{}, false
	}

	return turn, true
}

// walkDroidContent traverses message.content (always an array of blocks
// for Droid — no bare-string form like Pi) and returns:
//   - the concatenated text from text-typed blocks
//   - the tool calls extracted from tool_use-typed blocks
//   - the tool results extracted from tool_result-typed blocks
//
// Other block types (thinking, image) are ignored, matching the Claude
// adapter's behaviour.
func walkDroidContent(content []any) (string, []ToolCall, []ToolResult) {
	if len(content) == 0 {
		return "", nil, nil
	}
	var (
		textParts []string
		tcs       []ToolCall
		trs       []ToolResult
	)
	for _, item := range content {
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
			// Droid's tool_result content is always a plain string on
			// every transcript observed (unlike Claude's array-of-blocks
			// form), and carries no is_error field.
			content, _ := m["content"].(string)
			trs = append(trs, ToolResult{
				CallID:  callID,
				Content: truncateToolResult(content),
				IsError: false,
			})
		}
	}
	return strings.Join(textParts, "\n"), tcs, trs
}
