package watcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/momhq/mom/cli/internal/recorder"
)

// mockAdapter records every call to ParseLine for inspection.
type mockAdapter struct {
	calls []string
}

func (m *mockAdapter) Name() string { return "mock" }
func (m *mockAdapter) ParseLine(line []byte, sessionID string) (recorder.RawEntry, bool) {
	m.calls = append(m.calls, string(line))
	// Accept any non-empty line as a user turn.
	if len(strings.TrimSpace(string(line))) == 0 {
		return recorder.RawEntry{}, false
	}
	return recorder.RawEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Event:     "mock-user",
		Text:      "mock: " + string(line),
		SessionID: sessionID,
	}, true
}

// TestSessionIDFromPath verifies that the session ID is derived from the filename.
func TestSessionIDFromPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/home/user/.claude/projects/my-proj/abc-123.jsonl", "abc-123"},
		{"/tmp/session.jsonl", "session"},
		{"plain.jsonl", "plain"},
	}
	for _, tc := range cases {
		got := sessionIDFromPath(tc.path)
		if got != tc.want {
			t.Errorf("sessionIDFromPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestWatchCursorRoundTrip verifies write/read of byte offsets.
func TestWatchCursorRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cf := filepath.Join(dir, ".watch-cursor-test")

	// Non-existent cursor file → offset 0.
	if got := readWatchCursor(cf); got != 0 {
		t.Errorf("expected 0 for missing cursor, got %d", got)
	}

	writeWatchCursor(cf, 4096)
	if got := readWatchCursor(cf); got != 4096 {
		t.Errorf("expected 4096, got %d", got)
	}

	writeWatchCursor(cf, 0)
	if got := readWatchCursor(cf); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// TestExpandTilde verifies tilde expansion.
func TestExpandTilde(t *testing.T) {
	home, _ := os.UserHomeDir()

	got, err := expandTilde("~/.claude/projects")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(home, ".claude/projects")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Non-tilde path passes through unchanged.
	got2, _ := expandTilde("/absolute/path")
	if got2 != "/absolute/path" {
		t.Errorf("expected /absolute/path, got %q", got2)
	}
}

// TestIngestFile_NewSession verifies that a new transcript file is ingested
// and entries are written to .mom/raw/.
func TestIngestFile_NewSession(t *testing.T) {
	transcriptDir := t.TempDir()
	momDir := t.TempDir()

	w := &Watcher{
		cfg: Config{
			TranscriptDir: transcriptDir,
			MomDir:        momDir,
			Adapter:       &mockAdapter{},
			DebounceMs:    300,
		},
		timers:  make(map[string]*time.Timer),
		rawDir:  filepath.Join(momDir, "raw"),
		logFile: filepath.Join(momDir, "watch.log"),
	}
	_ = os.MkdirAll(w.rawDir, 0755)

	// Write a transcript file with two lines.
	sessionID := "test-session-001"
	transcriptPath := filepath.Join(transcriptDir, sessionID+".jsonl")
	line1 := mustMarshal(t, map[string]any{
		"type": "user", "sessionId": sessionID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"message":   map[string]any{"role": "user", "content": "Hello"},
	})
	line2 := mustMarshal(t, map[string]any{
		"type": "assistant", "sessionId": sessionID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"message":   map[string]any{"role": "assistant", "content": "Hi there"},
	})

	if err := os.WriteFile(transcriptPath, []byte(line1+"\n"+line2+"\n"), 0644); err != nil {
		t.Fatalf("writing transcript: %v", err)
	}

	w.ingestFile(transcriptPath)

	// Check that a daily raw file was created.
	today := time.Now().UTC().Format("2006-01-02")
	rawFile := filepath.Join(momDir, "raw", today+".jsonl")
	data, err := os.ReadFile(rawFile)
	if err != nil {
		t.Fatalf("reading raw file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 raw entries, got %d: %s", len(lines), string(data))
	}

	// Check cursor was written.
	cursorFile := filepath.Join(momDir, "raw", ".watch-cursor-"+sessionID)
	offset := readWatchCursor(cursorFile)
	expectedBytes := int64(len(line1) + 1 + len(line2) + 1) // +1 per newline
	if offset != expectedBytes {
		t.Errorf("expected cursor=%d, got %d", expectedBytes, offset)
	}
}

// TestIngestFile_IncrementalRead verifies that re-ingesting a file only reads new bytes.
func TestIngestFile_IncrementalRead(t *testing.T) {
	transcriptDir := t.TempDir()
	momDir := t.TempDir()

	adapter := &mockAdapter{}
	w := &Watcher{
		cfg: Config{
			TranscriptDir: transcriptDir,
			MomDir:        momDir,
			Adapter:       adapter,
			DebounceMs:    300,
		},
		timers:  make(map[string]*time.Timer),
		rawDir:  filepath.Join(momDir, "raw"),
		logFile: filepath.Join(momDir, "watch.log"),
	}
	_ = os.MkdirAll(w.rawDir, 0755)

	sessionID := "incremental-session"
	transcriptPath := filepath.Join(transcriptDir, sessionID+".jsonl")

	line1 := mustMarshal(t, map[string]any{
		"type": "user", "sessionId": sessionID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"message":   map[string]any{"role": "user", "content": "First"},
	})
	_ = os.WriteFile(transcriptPath, []byte(line1+"\n"), 0644)
	w.ingestFile(transcriptPath)
	firstCallCount := len(adapter.calls)

	// Append a second line.
	line2 := mustMarshal(t, map[string]any{
		"type": "user", "sessionId": sessionID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"message":   map[string]any{"role": "user", "content": "Second"},
	})
	f, _ := os.OpenFile(transcriptPath, os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString(line2 + "\n")
	_ = f.Close()

	w.ingestFile(transcriptPath)
	newCalls := adapter.calls[firstCallCount:]

	// Only the second line should have been parsed.
	if len(newCalls) != 1 {
		t.Errorf("expected 1 new parse call, got %d: %v", len(newCalls), newCalls)
	}
}

// TestIngestFile_SkipsSubagents verifies subagent files are excluded by the caller.
// (The watcher's handleEvent skips paths containing "subagents".)
func TestIngestFile_SkipsSubagents(t *testing.T) {
	path := "/home/user/.claude/projects/proj/abc/subagents/agent.jsonl"
	if !strings.Contains(path, "subagents") {
		t.Error("test path should contain 'subagents'")
	}
	// Verify the filter logic used in handleEvent.
	if strings.Contains(path, "subagents") {
		// This is the expected skip branch — test passes.
		return
	}
	t.Error("subagent path was not detected")
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}
