package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordFreshCursor(t *testing.T) {
	momDir := t.TempDir()
	transcriptFile := filepath.Join(t.TempDir(), "transcript.txt")

	content := "Hello from the transcript\nThis is a conversation turn."
	if err := os.WriteFile(transcriptFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		SessionID:      "sess-001",
		TranscriptPath: transcriptFile,
		Cwd:            "/some/project",
		HookEventName:  "stop",
	}

	if err := Record(momDir, input); err != nil {
		t.Fatalf("Record returned error: %v", err)
	}

	// Verify raw dir was created.
	rawDir := filepath.Join(momDir, "raw")
	if _, err := os.Stat(rawDir); err != nil {
		t.Fatalf("raw dir not created: %v", err)
	}

	// Verify a daily JSONL file was created.
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		t.Fatal(err)
	}
	var jsonlFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			jsonlFiles = append(jsonlFiles, e.Name())
		}
	}
	if len(jsonlFiles) == 0 {
		t.Fatal("no JSONL file created")
	}

	// Read and parse the entry.
	data, err := os.ReadFile(filepath.Join(rawDir, jsonlFiles[0]))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(lines))
	}

	var entry RawEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("invalid JSON entry: %v", err)
	}
	if entry.SessionID != "sess-001" {
		t.Errorf("expected session_id 'sess-001', got %q", entry.SessionID)
	}
	if entry.Event != "stop" {
		t.Errorf("expected event 'stop', got %q", entry.Event)
	}
	if !strings.Contains(entry.Text, "Hello from the transcript") {
		t.Errorf("entry text missing expected content, got: %q", entry.Text)
	}
	if entry.Timestamp == "" {
		t.Error("timestamp is empty")
	}
}

func TestRecordExistingCursor(t *testing.T) {
	momDir := t.TempDir()
	transcriptFile := filepath.Join(t.TempDir(), "transcript.txt")

	firstContent := "First turn content."
	if err := os.WriteFile(transcriptFile, []byte(firstContent), 0644); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		SessionID:      "sess-002",
		TranscriptPath: transcriptFile,
		Cwd:            "/project",
		HookEventName:  "stop",
	}

	// First call — records everything.
	if err := Record(momDir, input); err != nil {
		t.Fatal(err)
	}

	// Append new content.
	secondContent := "\nSecond turn content."
	f, err := os.OpenFile(transcriptFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(secondContent) //nolint:errcheck
	f.Close()

	// Second call — should only record new content.
	if err := Record(momDir, input); err != nil {
		t.Fatal(err)
	}

	rawDir := filepath.Join(momDir, "raw")
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		t.Fatal(err)
	}
	var jsonlFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			jsonlFiles = append(jsonlFiles, e.Name())
		}
	}
	if len(jsonlFiles) == 0 {
		t.Fatal("no JSONL file found")
	}

	data, err := os.ReadFile(filepath.Join(rawDir, jsonlFiles[0]))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(lines), lines)
	}

	var entry2 RawEntry
	if err := json.Unmarshal([]byte(lines[1]), &entry2); err != nil {
		t.Fatalf("invalid JSON entry: %v", err)
	}
	if !strings.Contains(entry2.Text, "Second turn") {
		t.Errorf("second entry should contain new content, got: %q", entry2.Text)
	}
	if strings.Contains(entry2.Text, "First turn") {
		t.Error("second entry should not contain first turn content")
	}
}

func TestRecordIdempotent(t *testing.T) {
	momDir := t.TempDir()
	transcriptFile := filepath.Join(t.TempDir(), "transcript.txt")

	content := "Idempotent test content."
	if err := os.WriteFile(transcriptFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		SessionID:      "sess-003",
		TranscriptPath: transcriptFile,
		Cwd:            "/project",
		HookEventName:  "stop",
	}

	// Call twice — no new content added.
	if err := Record(momDir, input); err != nil {
		t.Fatal(err)
	}
	if err := Record(momDir, input); err != nil {
		t.Fatal(err)
	}

	rawDir := filepath.Join(momDir, "raw")
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		t.Fatal(err)
	}
	var jsonlFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			jsonlFiles = append(jsonlFiles, e.Name())
		}
	}
	if len(jsonlFiles) == 0 {
		t.Fatal("no JSONL file found")
	}

	data, err := os.ReadFile(filepath.Join(rawDir, jsonlFiles[0]))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 entry (idempotent), got %d", len(lines))
	}
}

func TestRecordMissingTranscript(t *testing.T) {
	momDir := t.TempDir()

	input := HookInput{
		SessionID:      "sess-004",
		TranscriptPath: "/nonexistent/path/transcript.txt",
		Cwd:            "/project",
		HookEventName:  "stop",
	}

	// Must not return error — logs internally.
	if err := Record(momDir, input); err != nil {
		t.Fatalf("Record should not return error for missing transcript, got: %v", err)
	}
}

func TestRecordCreatesRawDirectory(t *testing.T) {
	momDir := t.TempDir()
	transcriptFile := filepath.Join(t.TempDir(), "transcript.txt")
	os.WriteFile(transcriptFile, []byte("content"), 0644) //nolint:errcheck

	input := HookInput{
		SessionID:      "sess-005",
		TranscriptPath: transcriptFile,
		HookEventName:  "stop",
	}

	// raw/ should not exist before.
	rawDir := filepath.Join(momDir, "raw")
	if _, err := os.Stat(rawDir); err == nil {
		t.Fatal("raw dir should not exist before Record")
	}

	if err := Record(momDir, input); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(rawDir); err != nil {
		t.Fatalf("raw dir should exist after Record: %v", err)
	}
}

func TestRecordCursorUpdated(t *testing.T) {
	momDir := t.TempDir()
	transcriptFile := filepath.Join(t.TempDir(), "transcript.txt")

	content := "Some content for cursor test."
	if err := os.WriteFile(transcriptFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		SessionID:      "sess-006",
		TranscriptPath: transcriptFile,
		HookEventName:  "stop",
	}

	if err := Record(momDir, input); err != nil {
		t.Fatal(err)
	}

	// Cursor file should exist.
	cursorFile := filepath.Join(momDir, "raw", ".cursor")
	data, err := os.ReadFile(cursorFile)
	if err != nil {
		t.Fatalf("cursor file not created: %v", err)
	}

	var cursor Cursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		t.Fatalf("invalid cursor JSON: %v", err)
	}
	if cursor.SessionID != "sess-006" {
		t.Errorf("expected session_id 'sess-006', got %q", cursor.SessionID)
	}
	expectedOffset := int64(len(content))
	if cursor.LastOffset != expectedOffset {
		t.Errorf("expected offset %d, got %d", expectedOffset, cursor.LastOffset)
	}
	if cursor.LastTimestamp == "" {
		t.Error("cursor timestamp is empty")
	}
}
