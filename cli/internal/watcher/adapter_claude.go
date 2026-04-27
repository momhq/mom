package watcher

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/momhq/mom/cli/internal/logbook"
	"github.com/momhq/mom/cli/internal/recorder"
)

// ClaudeAdapter parses Claude Code JSONL transcript lines.
// Claude Code writes one JSON object per line with the schema:
//
//	{ type, message: { role, content, model, usage }, timestamp, sessionId, uuid, cwd, gitBranch, isSidechain }
//
// We keep only type=="user" and type=="assistant" entries; everything else
// (tool_use, tool_result, system, hook_progress) is dropped.
type ClaudeAdapter struct{}

// NewClaudeAdapter returns a new ClaudeAdapter.
func NewClaudeAdapter() *ClaudeAdapter {
	return &ClaudeAdapter{}
}

func (a *ClaudeAdapter) Name() string { return "claude" }

// ParseSession implements SessionParser by delegating to logbook.ParseTranscript.
func (a *ClaudeAdapter) ParseSession(transcriptPath, sessionID string) (*logbook.SessionLog, error) {
	return logbook.ParseTranscript(transcriptPath, sessionID)
}

// claudeTranscriptLine is the minimal subset of a Claude Code JSONL line
// that the adapter needs to inspect.
type claudeTranscriptLine struct {
	Type      string         `json:"type"`
	Message   claudeMessage  `json:"message"`
	Timestamp string         `json:"timestamp"`
	SessionID string         `json:"sessionId"`
	IsSidechain bool         `json:"isSidechain"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []claudeContentItem
	Usage   *claudeUsage `json:"usage,omitempty"`
}

type claudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// ParseLine implements Adapter. It parses one JSONL line and returns a
// RawEntry if the line contains user or assistant conversational content.
func (a *ClaudeAdapter) ParseLine(line []byte, sessionID string) (recorder.RawEntry, bool) {
	line = trimLine(line)
	if len(line) == 0 {
		return recorder.RawEntry{}, false
	}

	var tl claudeTranscriptLine
	if err := json.Unmarshal(line, &tl); err != nil {
		return recorder.RawEntry{}, false
	}

	// Only keep user and assistant turns; drop everything else.
	if tl.Type != "user" && tl.Type != "assistant" {
		return recorder.RawEntry{}, false
	}

	// Skip subagent turns (isSidechain == true) — too noisy for Phase 1.
	if tl.IsSidechain {
		return recorder.RawEntry{}, false
	}

	// Extract text from message.content.
	text := extractClaudeContent(tl.Message.Content)
	if text == "" {
		return recorder.RawEntry{}, false
	}

	// Resolve session ID: prefer the line's sessionId field, fall back to
	// the filename-derived sessionID passed by the watcher.
	sid := tl.SessionID
	if sid == "" {
		sid = sessionID
	}

	// Resolve timestamp: prefer the line's timestamp, fall back to now.
	ts := tl.Timestamp
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}

	return recorder.RawEntry{
		Timestamp: ts,
		Event:     "watch-" + tl.Type,
		Text:      text,
		SessionID: sid,
	}, true
}

// extractClaudeContent converts message.content (string or []contentItem) to plain text.
func extractClaudeContent(content any) string {
	if content == nil {
		return ""
	}

	// Fast path: plain string content.
	if s, ok := content.(string); ok {
		return strings.TrimSpace(s)
	}

	// Structured content: []{"type":"text","text":"..."}
	// JSON unmarshal gives []interface{} for arrays.
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
		t, _ := m["type"].(string)
		if t != "text" {
			continue // skip tool_use, tool_result, image, etc.
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
