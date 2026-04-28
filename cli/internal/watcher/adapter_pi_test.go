package watcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPiAdapter_Name(t *testing.T) {
	if got := NewPiAdapter().Name(); got != "pi" {
		t.Errorf("Name(): got %q, want %q", got, "pi")
	}
}

func TestPiAdapter_ParseLine_UserMessage_StringContent(t *testing.T) {
	a := NewPiAdapter()

	line, _ := json.Marshal(map[string]any{
		"type":      "message",
		"id":        "51d3e8ae",
		"parentId":  "7c38c6c5",
		"timestamp": "2026-04-28T00:12:35.077Z",
		"message": map[string]any{
			"role":    "user",
			"content": "What is the purpose of pi?",
		},
	})

	entry, ok := a.ParseLine(line, "sess-from-filename")
	if !ok {
		t.Fatal("expected ok=true for user message with string content")
	}
	if entry.Text != "What is the purpose of pi?" {
		t.Errorf("Text: got %q", entry.Text)
	}
	if entry.Event != "watch-user" {
		t.Errorf("Event: got %q, want watch-user", entry.Event)
	}
	if entry.SessionID != "sess-from-filename" {
		t.Errorf("SessionID: got %q, want fallback", entry.SessionID)
	}
	if entry.Timestamp != "2026-04-28T00:12:35.077Z" {
		t.Errorf("Timestamp: got %q", entry.Timestamp)
	}
}

func TestPiAdapter_ParseLine_UserMessage_StructuredContent(t *testing.T) {
	a := NewPiAdapter()

	// This mirrors the actual shape pi writes for user prompts:
	// content is an array with a single {type:"text",text:"..."} block.
	line, _ := json.Marshal(map[string]any{
		"type":      "message",
		"id":        "51d3e8ae",
		"parentId":  "7c38c6c5",
		"timestamp": "2026-04-28T00:12:35.077Z",
		"message": map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{
					"type": "text",
					"text": "Read MOM's MCP tool definitions.",
				},
			},
			"timestamp": 1777335155073,
		},
	})

	entry, ok := a.ParseLine(line, "sess")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Text != "Read MOM's MCP tool definitions." {
		t.Errorf("Text: got %q", entry.Text)
	}
}

func TestPiAdapter_ParseLine_AssistantMessage_MixedContent(t *testing.T) {
	a := NewPiAdapter()

	// Real assistant turns interleave text + tool_use + thinking blocks.
	// We must concatenate text blocks in order and drop everything else.
	line, _ := json.Marshal(map[string]any{
		"type":      "message",
		"timestamp": "2026-04-28T00:13:00.000Z",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "thinking", "thinking": "let me check..."},
				map[string]any{"type": "text", "text": "Reading the file."},
				map[string]any{"type": "tool_use", "id": "t1", "name": "Read", "input": map[string]any{"path": "x.go"}},
				map[string]any{"type": "text", "text": "Done."},
			},
		},
	})

	entry, ok := a.ParseLine(line, "sess")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Text != "Reading the file.\nDone." {
		t.Errorf("Text concat: got %q", entry.Text)
	}
	if entry.Event != "watch-assistant" {
		t.Errorf("Event: got %q, want watch-assistant", entry.Event)
	}
}

func TestPiAdapter_ParseLine_DropSessionHeader(t *testing.T) {
	a := NewPiAdapter()
	line, _ := json.Marshal(map[string]any{
		"type":      "session",
		"version":   3,
		"id":        "019dd16c-ca46-776a-b1ce-42cd3c3582e0",
		"timestamp": "2026-04-28T00:11:01.063Z",
		"cwd":       "/Users/x/proj",
	})
	if _, ok := a.ParseLine(line, "s"); ok {
		t.Error("expected session header to be dropped")
	}
}

func TestPiAdapter_ParseLine_DropThinkingLevelChange(t *testing.T) {
	a := NewPiAdapter()
	line, _ := json.Marshal(map[string]any{
		"type":          "thinking_level_change",
		"id":            "301c6f97",
		"timestamp":     "2026-04-28T00:11:01.634Z",
		"thinkingLevel": "off",
	})
	if _, ok := a.ParseLine(line, "s"); ok {
		t.Error("expected thinking_level_change to be dropped")
	}
}

func TestPiAdapter_ParseLine_DropModelChange(t *testing.T) {
	a := NewPiAdapter()
	line, _ := json.Marshal(map[string]any{
		"type":      "model_change",
		"id":        "a3f9699c",
		"timestamp": "2026-04-28T00:11:36.206Z",
		"provider":  "anthropic",
		"modelId":   "claude-opus-4-7",
	})
	if _, ok := a.ParseLine(line, "s"); ok {
		t.Error("expected model_change to be dropped")
	}
}

func TestPiAdapter_ParseLine_DropToolOnlyAssistantContent(t *testing.T) {
	a := NewPiAdapter()
	// Assistant turn with no text blocks (only tool_use). No conversational
	// signal → drop, like the Claude adapter does.
	line, _ := json.Marshal(map[string]any{
		"type":      "message",
		"timestamp": "2026-04-28T00:13:00Z",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "tool_use", "id": "t1", "name": "Bash"},
			},
		},
	})
	if _, ok := a.ParseLine(line, "s"); ok {
		t.Error("expected tool-only assistant content to yield ok=false")
	}
}

func TestPiAdapter_ParseLine_DropToolResult(t *testing.T) {
	a := NewPiAdapter()
	// Pi serializes tool results as a "user" role message whose content is
	// a single tool_result block. That's bookkeeping noise — drop it.
	line, _ := json.Marshal(map[string]any{
		"type":      "message",
		"timestamp": "2026-04-28T00:13:01Z",
		"message": map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{
					"type":        "tool_result",
					"tool_use_id": "t1",
					"content":     "stdout...",
				},
			},
		},
	})
	if _, ok := a.ParseLine(line, "s"); ok {
		t.Error("expected pure tool_result message to yield ok=false")
	}
}

func TestPiAdapter_ParseLine_UnknownRole(t *testing.T) {
	a := NewPiAdapter()
	line, _ := json.Marshal(map[string]any{
		"type":      "message",
		"timestamp": "2026-04-28T00:13:00Z",
		"message": map[string]any{
			"role":    "system",
			"content": "ignored",
		},
	})
	if _, ok := a.ParseLine(line, "s"); ok {
		t.Error("expected non-user/assistant role to be dropped")
	}
}

func TestPiAdapter_ParseLine_MissingTimestamp_FallsBackToNow(t *testing.T) {
	a := NewPiAdapter()
	line, _ := json.Marshal(map[string]any{
		"type": "message",
		"message": map[string]any{
			"role":    "user",
			"content": "hi",
		},
	})
	entry, ok := a.ParseLine(line, "s")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Timestamp == "" {
		t.Error("expected fallback timestamp, got empty")
	}
}

func TestPiAdapter_ParseLine_InvalidJSON(t *testing.T) {
	a := NewPiAdapter()
	if _, ok := a.ParseLine([]byte("{not json"), "s"); ok {
		t.Error("expected invalid JSON to yield ok=false")
	}
}

func TestPiAdapter_ParseLine_EmptyLine(t *testing.T) {
	a := NewPiAdapter()
	if _, ok := a.ParseLine([]byte(""), "s"); ok {
		t.Error("expected empty line to yield ok=false")
	}
}

// TestPiAdapter_ParseSession_Integration writes a realistic pi session JSONL
// (mirroring the actual shape under ~/.pi/agent/sessions/) and verifies the
// adapter's ParseSession produces sensible logbook metrics, including pi's
// per-turn model/provider/usage metadata.
func TestPiAdapter_ParseSession_Integration(t *testing.T) {
	lines := []map[string]any{
		// Session header — should be ignored by the parser.
		{
			"type":      "session",
			"version":   3,
			"id":        "019dd16c",
			"timestamp": "2026-04-28T00:11:01.063Z",
			"cwd":       "/Users/x/proj",
		},
		// Out-of-band events — also ignored.
		{
			"type":          "thinking_level_change",
			"timestamp":     "2026-04-28T00:11:01.634Z",
			"thinkingLevel": "off",
		},
		{
			"type":      "model_change",
			"timestamp": "2026-04-28T00:11:36.206Z",
			"provider":  "anthropic",
			"modelId":   "claude-opus-4-7",
		},
		// User prompt.
		{
			"type":      "message",
			"timestamp": "2026-04-28T00:12:00Z",
			"message": map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "Find the watcher."},
				},
			},
		},
		// Assistant turn 1: text + toolCall (read) + usage block.
		{
			"type":      "message",
			"timestamp": "2026-04-28T00:12:30Z",
			"message": map[string]any{
				"role":       "assistant",
				"model":      "claude-opus-4-7",
				"provider":   "anthropic",
				"stopReason": "tool_use",
				"content": []any{
					map[string]any{"type": "text", "text": "Reading."},
					map[string]any{
						"type":      "toolCall",
						"id":        "t1",
						"name":      "read",
						"arguments": map[string]any{"path": "watcher.go"},
					},
				},
				"usage": map[string]any{
					"input":       6,
					"output":      107,
					"cacheRead":   0,
					"cacheWrite":  4504,
					"totalTokens": 4617,
					"cost": map[string]any{
						"input":      0.00003,
						"output":     0.002675,
						"cacheRead":  0,
						"cacheWrite": 0.02815,
						"total":      0.030855,
					},
				},
			},
		},
		// Assistant turn 2: edit + create_memory_draft + another usage block.
		{
			"type":      "message",
			"timestamp": "2026-04-28T00:13:00Z",
			"message": map[string]any{
				"role":       "assistant",
				"model":      "claude-opus-4-7",
				"provider":   "anthropic",
				"stopReason": "end_turn",
				"content": []any{
					map[string]any{
						"type":      "toolCall",
						"id":        "t2",
						"name":      "edit",
						"arguments": map[string]any{"file_path": "/foo/bar.go"},
					},
					map[string]any{
						"type":      "toolCall",
						"id":        "t3",
						"name":      "create_memory_draft",
						"arguments": map[string]any{},
					},
				},
				"usage": map[string]any{
					"input":       12,
					"output":      50,
					"cacheRead":   1000,
					"cacheWrite":  0,
					"totalTokens": 1062,
					"cost":        map[string]any{"total": 0.001},
				},
			},
		},
		// Tool result message — own role, no toolCall blocks. Should not be
		// counted as an interaction.
		{
			"type":      "message",
			"timestamp": "2026-04-28T00:13:01Z",
			"message": map[string]any{
				"role": "toolResult",
				"content": []any{
					map[string]any{"type": "text", "text": "ok"},
				},
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sess-test.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		data, _ := json.Marshal(l)
		f.Write(append(data, '\n'))
	}
	f.Close()

	a := NewPiAdapter()
	sl, err := a.ParseSession(path, "sess-test")
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}

	// Core counts.
	if sl.SessionID != "sess-test" {
		t.Errorf("SessionID: got %q", sl.SessionID)
	}
	if sl.Interactions != 2 {
		t.Errorf("Interactions: got %d, want 2", sl.Interactions)
	}
	if sl.FilesChanged != 1 {
		t.Errorf("FilesChanged: got %d, want 1", sl.FilesChanged)
	}
	if sl.MemoriesCreated != 1 {
		t.Errorf("MemoriesCreated: got %d, want 1", sl.MemoriesCreated)
	}

	// Tool categorization — these all go through pi's "toolCall" path.
	if g, ok := sl.ToolCalls["codebase_read"]; !ok || g.Total != 1 {
		t.Errorf("codebase_read: got %+v, want total=1", g)
	}
	if g, ok := sl.ToolCalls["codebase_write"]; !ok || g.Total != 1 {
		t.Errorf("codebase_write: got %+v, want total=1", g)
	}
	if g, ok := sl.ToolCalls["mom_memory"]; !ok || g.Total != 1 {
		t.Errorf("mom_memory: got %+v, want total=1", g)
	}

	// Pi-specific metadata.
	if sl.Model != "claude-opus-4-7" {
		t.Errorf("Model: got %q", sl.Model)
	}
	if sl.Provider != "anthropic" {
		t.Errorf("Provider: got %q", sl.Provider)
	}
	if sl.Usage == nil {
		t.Fatal("Usage: nil, want aggregated usage block")
	}
	if sl.Usage.InputTokens != 18 {
		t.Errorf("Usage.InputTokens: got %d, want 18", sl.Usage.InputTokens)
	}
	if sl.Usage.OutputTokens != 157 {
		t.Errorf("Usage.OutputTokens: got %d, want 157", sl.Usage.OutputTokens)
	}
	if sl.Usage.CacheReadTokens != 1000 {
		t.Errorf("Usage.CacheReadTokens: got %d, want 1000", sl.Usage.CacheReadTokens)
	}
	if sl.Usage.CacheWriteTokens != 4504 {
		t.Errorf("Usage.CacheWriteTokens: got %d, want 4504", sl.Usage.CacheWriteTokens)
	}
	if sl.Usage.TotalTokens != 5679 {
		t.Errorf("Usage.TotalTokens: got %d, want 5679", sl.Usage.TotalTokens)
	}
	// Cost is float — compare with epsilon.
	wantCost := 0.030855 + 0.001
	if sl.Usage.CostUSD < wantCost-1e-9 || sl.Usage.CostUSD > wantCost+1e-9 {
		t.Errorf("Usage.CostUSD: got %v, want ~%v", sl.Usage.CostUSD, wantCost)
	}
	if sl.Usage.StopReasons["tool_use"] != 1 || sl.Usage.StopReasons["end_turn"] != 1 {
		t.Errorf("Usage.StopReasons: got %+v", sl.Usage.StopReasons)
	}
}

// TestPiAdapter_ParseSession_NoUsage covers a session with no assistant
// usage blocks (e.g. a transcript that only contains user prompts before
// crash). Usage should be nil; tool counts should still work.
func TestPiAdapter_ParseSession_NoUsage(t *testing.T) {
	lines := []map[string]any{
		{
			"type":      "message",
			"timestamp": "2026-04-28T00:12:00Z",
			"message": map[string]any{
				"role":    "user",
				"content": []any{map[string]any{"type": "text", "text": "hi"}},
			},
		},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "u.jsonl")
	f, _ := os.Create(path)
	for _, l := range lines {
		data, _ := json.Marshal(l)
		f.Write(append(data, '\n'))
	}
	f.Close()

	sl, err := NewPiAdapter().ParseSession(path, "u")
	if err != nil {
		t.Fatal(err)
	}
	if sl.Usage != nil {
		t.Errorf("Usage: got %+v, want nil", sl.Usage)
	}
	if sl.Interactions != 0 {
		t.Errorf("Interactions: got %d, want 0", sl.Interactions)
	}
}
