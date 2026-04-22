// Package recorder appends raw conversation turns to .mom/raw/ JSONL files.
// It is driven by Claude Code hooks (PostToolUse, Stop, PreCompact, Clear)
// and an MCP fallback tool. All errors are logged — never propagated —
// so the hook always exits 0.
package recorder

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// HookInput is the JSON payload received from Claude Code hooks on stdin.
type HookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
}

// RawEntry is one line in the .mom/raw/ JSONL file.
type RawEntry struct {
	Timestamp string `json:"timestamp"`
	Event     string `json:"event"`     // "stop", "pre-compact", "clear"
	Text      string `json:"text"`
	SessionID string `json:"session_id"`
}

// Cursor tracks the last recorded position per session.
type Cursor struct {
	SessionID     string `json:"session_id"`
	LastOffset    int64  `json:"last_offset"`
	LastTimestamp string `json:"last_timestamp"`
}

// Record reads the transcript file and appends new content to .mom/raw/.
// It is idempotent — uses a cursor file to track what's been recorded.
// Errors are logged to .mom/logs/record.log but never returned.
func Record(momDir string, input HookInput) error {
	rawDir := filepath.Join(momDir, "raw")

	// 1. Ensure .mom/raw/ exists.
	if err := os.MkdirAll(rawDir, 0755); err != nil {
		logError(momDir, fmt.Errorf("creating raw dir: %w", err))
		return nil
	}

	// 2. Read cursor from .mom/raw/.cursor (JSON).
	cursorFile := filepath.Join(rawDir, ".cursor")
	cursor := readCursor(cursorFile, input.SessionID)

	// 3. Open transcript_path, seek to cursor.LastOffset.
	f, err := os.Open(input.TranscriptPath)
	if err != nil {
		logError(momDir, fmt.Errorf("opening transcript %q: %w", input.TranscriptPath, err))
		return nil
	}
	defer f.Close()

	if cursor.LastOffset > 0 {
		if _, err := f.Seek(cursor.LastOffset, io.SeekStart); err != nil {
			logError(momDir, fmt.Errorf("seeking transcript: %w", err))
			return nil
		}
	}

	// 4. Read new content from that offset.
	reader := bufio.NewReader(f)
	newContent, err := io.ReadAll(reader)
	if err != nil {
		logError(momDir, fmt.Errorf("reading transcript: %w", err))
		return nil
	}

	// 5. If no new content, return nil.
	if len(newContent) == 0 {
		return nil
	}

	// 6. Write new entry to .mom/raw/<YYYY-MM-DD>.jsonl.
	now := time.Now().UTC()
	dailyFile := filepath.Join(rawDir, now.Format("2006-01-02")+".jsonl")

	event := input.HookEventName
	if event == "" {
		event = "stop"
	}

	entry := RawEntry{
		Timestamp: now.Format(time.RFC3339),
		Event:     event,
		Text:      string(newContent),
		SessionID: input.SessionID,
	}

	line, err := json.Marshal(entry)
	if err != nil {
		logError(momDir, fmt.Errorf("marshaling entry: %w", err))
		return nil
	}

	df, err := os.OpenFile(dailyFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logError(momDir, fmt.Errorf("opening daily file: %w", err))
		return nil
	}
	if _, err := df.Write(append(line, '\n')); err != nil {
		df.Close()
		logError(momDir, fmt.Errorf("writing entry: %w", err))
		return nil
	}
	df.Close()

	// 7. Update cursor with new offset.
	newOffset := cursor.LastOffset + int64(len(newContent))
	updatedCursor := Cursor{
		SessionID:     input.SessionID,
		LastOffset:    newOffset,
		LastTimestamp: now.Format(time.RFC3339),
	}
	if err := writeCursor(cursorFile, updatedCursor); err != nil {
		logError(momDir, fmt.Errorf("writing cursor: %w", err))
	}

	return nil
}

// readCursor reads the cursor file for the given session, or returns a zero cursor.
func readCursor(cursorFile, sessionID string) Cursor {
	data, err := os.ReadFile(cursorFile)
	if err != nil {
		// File doesn't exist — fresh start.
		return Cursor{SessionID: sessionID}
	}
	var c Cursor
	if err := json.Unmarshal(data, &c); err != nil {
		return Cursor{SessionID: sessionID}
	}
	// If session changed, reset offset.
	if c.SessionID != sessionID {
		return Cursor{SessionID: sessionID}
	}
	return c
}

// writeCursor persists the cursor to disk atomically.
func writeCursor(cursorFile string, c Cursor) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(cursorFile, data, 0644)
}

// logError appends an error message to .mom/logs/record.log, best-effort.
func logError(momDir string, err error) {
	logsDir := filepath.Join(momDir, "logs")
	_ = os.MkdirAll(logsDir, 0755)
	logFile := filepath.Join(logsDir, "record.log")
	f, ferr := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if ferr != nil {
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(f, "%s recorder error: %v\n", ts, err)
}
