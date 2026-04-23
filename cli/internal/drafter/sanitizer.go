package drafter

import (
	"encoding/json"
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
