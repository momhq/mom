// Package logbook parses Claude Code transcript files and extracts
// session-level observability metrics (interactions, tool calls, file changes).
package logbook

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"
)

// SessionLog holds the extracted metrics for one session.
type SessionLog struct {
	SessionID       string               `json:"session_id"`
	Started         string               `json:"started"`
	Ended           string               `json:"ended"`
	Interactions    int                  `json:"interactions"`
	FilesChanged    int                  `json:"files_changed"`
	MemoriesCreated int                  `json:"memories_created"`
	ToolCalls       map[string]ToolGroup `json:"tool_calls"`
}

// ToolGroup aggregates tool call counts within a category.
type ToolGroup struct {
	Total  int            `json:"total"`
	Detail map[string]int `json:"detail"`
}

// NormalizeToolName strips runtime-specific prefixes from tool names.
// Claude Code prefixes MCP tools with "mcp__<server>__" (e.g. "mcp__mom__mom_recall").
func NormalizeToolName(toolName string) string {
	if strings.HasPrefix(toolName, "mcp__") {
		// Strip "mcp__<server>__" → bare tool name.
		if i := strings.Index(toolName[5:], "__"); i >= 0 {
			return toolName[5+i+2:]
		}
	}
	return toolName
}

// CategorizeToolCall returns the category for a given tool name.
// Handles both bare names (Windsurf) and MCP-prefixed names (Claude Code).
func CategorizeToolCall(toolName string) string {
	name := NormalizeToolName(toolName)
	switch {
	case isMemoryTool(name):
		return "mom_memory"
	case isMomCLI(name):
		return "mom_cli"
	case isCodebaseRead(name):
		return "codebase_read"
	case isCodebaseWrite(name):
		return "codebase_write"
	default:
		return "system"
	}
}

func isMemoryTool(name string) bool {
	return name == "mom_recall" || name == "search_memories" || name == "get_memory" ||
		name == "create_memory_draft" || name == "list_landmarks" || name == "mom_record_turn"
}

func isMomCLI(name string) bool {
	return name == "mom_status" || name == "mom_draft" || name == "mom_log"
}

func isCodebaseRead(name string) bool {
	return name == "Read" || name == "read" || name == "Grep" || name == "grep" ||
		name == "Glob" || name == "glob" || name == "rg"
}

func isCodebaseWrite(name string) bool {
	return name == "Edit" || name == "edit" || name == "Write" || name == "write"
}

// ParseTranscript reads a JSONL transcript file and extracts session metrics.
// The transcript format is Claude Code's JSONL with entries containing tool calls.
func ParseTranscript(transcriptPath, sessionID string) (*SessionLog, error) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	log := &SessionLog{
		SessionID: sessionID,
		ToolCalls: make(map[string]ToolGroup),
	}

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var firstTS, lastTS string
	filesChanged := make(map[string]bool)

	for scanner.Scan() {
		line := scanner.Bytes()
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		// Track timestamps.
		if ts, ok := entry["timestamp"].(string); ok {
			if firstTS == "" {
				firstTS = ts
			}
			lastTS = ts
		}

		// Claude Code nests role/content inside "message"; unwrap if present.
		msg := entry
		if m, ok := entry["message"].(map[string]any); ok {
			msg = m
		}

		// Count interactions (assistant messages).
		if role, _ := msg["role"].(string); role == "assistant" {
			log.Interactions++
		}

		// Extract tool calls from content array.
		if content, ok := msg["content"].([]any); ok {
			for _, item := range content {
				block, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if blockType, _ := block["type"].(string); blockType != "tool_use" {
					continue
				}
				toolName, _ := block["name"].(string)
				if toolName == "" {
					continue
				}

				normalized := NormalizeToolName(toolName)
				category := CategorizeToolCall(toolName)
				group := log.ToolCalls[category]
				group.Total++
				if group.Detail == nil {
					group.Detail = make(map[string]int)
				}
				group.Detail[toolName]++
				log.ToolCalls[category] = group

				// Track unique files changed.
				if isCodebaseWrite(normalized) {
					if input, ok := block["input"].(map[string]any); ok {
						if fp, ok := input["file_path"].(string); ok && fp != "" {
							filesChanged[fp] = true
						}
					}
				}

				// Track memory creation.
				if normalized == "create_memory_draft" {
					log.MemoriesCreated++
				}
			}
		}
	}

	if firstTS == "" {
		firstTS = time.Now().UTC().Format(time.RFC3339)
	}
	if lastTS == "" {
		lastTS = firstTS
	}

	log.Started = firstTS
	log.Ended = lastTS
	log.FilesChanged = len(filesChanged)

	return log, nil
}
