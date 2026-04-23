package drafter

import (
	"encoding/json"
	"regexp"
	"strings"
)

// sanitizeTurns filters raw turn text to keep only conversational content.
// Each raw turn's Text field may contain full Claude Code transcript JSONL
// (one JSON object per line). The sanitizer:
//  1. Parses each line as JSON
//  2. Keeps only lines where type is "assistant" or "user"
//  3. Extracts text content from message.content[] items where type=="text"
//  4. Drops tool_use, tool_result, progress, system lines
//  5. Falls back to keeping raw text if no structured content is found
func sanitizeTurns(turns []rawTurn) []rawTurn {
	var result []rawTurn
	for _, t := range turns {
		cleaned := sanitizeText(t.Text)
		if cleaned == "" {
			continue // drop turns with no extractable text
		}
		t.Text = cleaned
		result = append(result, t)
	}
	return result
}

// sanitizeText processes a single turn's text content.
// If the text contains JSONL transcript lines, it extracts only text content.
// If it's plain text (not JSONL), it returns it as-is.
func sanitizeText(text string) string {
	lines := strings.Split(text, "\n")

	var extracted []string
	hasStructured := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to parse as JSON
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			// Not JSON — could be plain text. Keep it.
			extracted = append(extracted, line)
			continue
		}

		hasStructured = true

		// Check top-level type
		msgType, _ := obj["type"].(string)

		// Windsurf format: user_input, planner_response, mcp_tool
		switch msgType {
		case "user_input":
			if ui, ok := obj["user_input"].(map[string]any); ok {
				if resp, ok := ui["user_response"].(string); ok && resp != "" {
					extracted = append(extracted, resp)
				}
			}
			continue
		case "planner_response":
			if pr, ok := obj["planner_response"].(map[string]any); ok {
				if resp, ok := pr["response"].(string); ok && resp != "" {
					extracted = append(extracted, resp)
				}
			}
			continue
		case "mcp_tool":
			// Drop MCP tool calls/results (internal machinery)
			continue
		case "response_item":
			// Codex format: payload.type == "message" with role and content[]
			payload, _ := obj["payload"].(map[string]any)
			if payload == nil {
				continue
			}
			pType, _ := payload["type"].(string)
			role, _ := payload["role"].(string)
			if pType != "message" || (role != "user" && role != "assistant") {
				// Drop reasoning, function_call, function_call_output, developer
				continue
			}
			if content, ok := payload["content"].([]any); ok {
				for _, item := range content {
					if m, ok := item.(map[string]any); ok {
						ct, _ := m["type"].(string)
						if ct == "output_text" || ct == "input_text" {
							if text, _ := m["text"].(string); text != "" {
								extracted = append(extracted, text)
							}
						}
					}
				}
			}
			continue
		case "event_msg", "session_meta", "turn_context":
			// Codex metadata — drop
			continue
		}

		// Claude Code format: assistant, user
		if msgType != "assistant" && msgType != "user" {
			// Drop progress, result, system, etc.
			continue
		}

		// Extract text from message.content[]
		texts := extractTextContent(obj)
		if len(texts) > 0 {
			extracted = append(extracted, texts...)
		}
	}

	// If we found structured JSON but extracted nothing, the turn was all tool_use/metadata
	if hasStructured && len(extracted) == 0 {
		return ""
	}

	return strings.Join(extracted, "\n")
}

// sanitizeForTags applies a stricter cleaning pass on text before tag extraction.
// Removes XML tags, code blocks, file paths, URLs, and other noise that would
// pollute RAKE/BM25 tag generation while being fine in content.
func sanitizeForTags(text string) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Drop XML-style tags (system prompts, env context)
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
			continue
		}
		// Drop lines that are mostly a file path
		if looksLikePath(line) {
			continue
		}
		// Drop lines that are code (indented or common code patterns)
		if looksLikeCode(line) {
			continue
		}
		// Drop URLs
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			continue
		}
		// Strip inline XML tags
		line = xmlTagRe.ReplaceAllString(line, " ")
		// Strip markdown formatting
		line = strings.ReplaceAll(line, "```", "")
		line = strings.ReplaceAll(line, "**", "")
		line = strings.ReplaceAll(line, "##", "")
		// Collapse whitespace
		line = strings.Join(strings.Fields(line), " ")
		if len(line) > 2 {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

var xmlTagRe = regexp.MustCompile(`<[^>]+>`)

// looksLikePath returns true if the line is dominated by a file path.
func looksLikePath(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "./") {
		return true
	}
	// Lines with multiple path separators relative to length
	slashes := strings.Count(trimmed, "/")
	if slashes >= 3 && float64(slashes)/float64(len(trimmed)) > 0.05 {
		return true
	}
	return false
}

// looksLikeCode returns true if the line appears to be code rather than prose.
func looksLikeCode(line string) bool {
	trimmed := strings.TrimSpace(line)
	// Common code patterns
	codeIndicators := []string{
		"func ", "import ", "package ", "return ", "if err",
		"var ", "const ", "type ", "fmt.", "os.", "json.",
		"func(", "map[", "[]", ":=", "!=", "==",
		"```", "{}", "();",
	}
	for _, indicator := range codeIndicators {
		if strings.Contains(trimmed, indicator) {
			return true
		}
	}
	return false
}

// extractTextContent pulls text strings from a transcript message object.
// Handles the nested structure: obj.message.content[].text where content[].type=="text"
// Also handles flat text fields directly on the object.
func extractTextContent(obj map[string]any) []string {
	var texts []string

	// Try obj.message.content[] (Claude Code format)
	if msg, ok := obj["message"].(map[string]any); ok {
		if content, ok := msg["content"].([]any); ok {
			for _, item := range content {
				if m, ok := item.(map[string]any); ok {
					if t, _ := m["type"].(string); t == "text" {
						if text, _ := m["text"].(string); text != "" {
							texts = append(texts, text)
						}
					}
				}
			}
		}
	}

	// Try obj.content[] directly (alternative format)
	if len(texts) == 0 {
		if content, ok := obj["content"].([]any); ok {
			for _, item := range content {
				if m, ok := item.(map[string]any); ok {
					if t, _ := m["type"].(string); t == "text" {
						if text, _ := m["text"].(string); text != "" {
							texts = append(texts, text)
						}
					}
				}
			}
		}
	}

	// Try obj.content as string directly (simple format)
	if len(texts) == 0 {
		if content, ok := obj["content"].(string); ok && content != "" {
			texts = append(texts, content)
		}
	}

	// Try obj.text directly (simplest format)
	if len(texts) == 0 {
		if text, ok := obj["text"].(string); ok && text != "" {
			texts = append(texts, text)
		}
	}

	return texts
}
