package lens

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeJSON writes v as JSON to path, creating parent dirs as needed.
func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// fixtureScope creates a .mom/ scope with logs/ and memory/ subdirs.
func fixtureScope(t *testing.T) string {
	t.Helper()
	momDir := filepath.Join(t.TempDir(), ".mom")
	for _, sub := range []string{"logs", "memory"} {
		if err := os.MkdirAll(filepath.Join(momDir, sub), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	return momDir
}

func writeSessionLog(t *testing.T, momDir, sessionID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	log := map[string]any{
		"session_id":   sessionID,
		"started":      now,
		"ended":        now,
		"interactions": 1,
		"tool_calls":   map[string]any{},
	}
	writeJSON(t, filepath.Join(momDir, "logs", "session-"+sessionID+".json"), log)
}

func writeMemoryDoc(t *testing.T, momDir, id, sessionID string) {
	t.Helper()
	doc := map[string]any{
		"id":         id,
		"scope":      "project",
		"tags":       []string{},
		"created":    time.Now().UTC(),
		"created_by": "test",
		"session_id": sessionID,
		"content":    map[string]any{},
	}
	writeJSON(t, filepath.Join(momDir, "memory", id+".json"), doc)
}

// fetchSessions hits GET /api/sessions and returns the parsed result.
func fetchSessions(t *testing.T, h http.Handler) []SessionSummary {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got []SessionSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return got
}

// TestSessions_PicksUpNewMemoriesWithoutRestart locks down bug #1 + #2:
// memories created after Server.New() must show up on subsequent requests.
func TestSessions_PicksUpNewMemoriesWithoutRestart(t *testing.T) {
	momDir := fixtureScope(t)
	const sid = "sess-aaa"

	writeSessionLog(t, momDir, sid)
	writeMemoryDoc(t, momDir, "mem-1", sid)

	srv, err := New([]ScopeEntry{{Label: "test", Path: momDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got := fetchSessions(t, srv.Handler())
	if len(got) != 1 || got[0].MemoryCount != 1 {
		t.Fatalf("first read: want 1 session with 1 memory, got %+v", got)
	}

	// Memory created AFTER server start — must be visible on next request.
	writeMemoryDoc(t, momDir, "mem-2", sid)

	got = fetchSessions(t, srv.Handler())
	if len(got) != 1 || got[0].MemoryCount != 2 {
		t.Fatalf("second read: want 1 session with 2 memories, got %+v", got)
	}
}

// TestMeta_ReflectsLiveMemoryCount locks down bug #1 for /api/meta.
func TestMeta_ReflectsLiveMemoryCount(t *testing.T) {
	momDir := fixtureScope(t)
	writeMemoryDoc(t, momDir, "mem-1", "sess-x")

	srv, err := New([]ScopeEntry{{Label: "test", Path: momDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	get := func() MetaResponse {
		req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		var m MetaResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &m)
		return m
	}

	if m := get(); m.TotalMemories != 1 {
		t.Fatalf("first meta: want 1 memory, got %d", m.TotalMemories)
	}

	writeMemoryDoc(t, momDir, "mem-2", "sess-y")

	if m := get(); m.TotalMemories != 2 {
		t.Fatalf("second meta: want 2 memories, got %d", m.TotalMemories)
	}
}

// TestListenWithFallback_BumpsToNextPort locks down bug #4:
// when the preferred port is taken, the helper picks a free nearby port.
func TestListenWithFallback_BumpsToNextPort(t *testing.T) {
	occupier, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("seed listen: %v", err)
	}
	defer occupier.Close()

	taken := occupier.Addr().(*net.TCPAddr).Port

	ln, err := ListenWithFallback("127.0.0.1", taken, 5)
	if err != nil {
		t.Fatalf("ListenWithFallback: %v", err)
	}
	defer ln.Close()

	got := ln.Addr().(*net.TCPAddr).Port
	if got == taken {
		t.Fatalf("expected fallback port, got the occupied one (%d)", got)
	}
	if got < taken+1 || got > taken+5 {
		t.Fatalf("expected port in [%d, %d], got %d", taken+1, taken+5, got)
	}
}

// TestListenWithFallback_ZeroAttemptsFailsCleanly: explicit-port behavior
// (attempts=0 means no fallback, fail loud if taken).
func TestListenWithFallback_ZeroAttemptsFailsCleanly(t *testing.T) {
	occupier, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("seed listen: %v", err)
	}
	defer occupier.Close()

	taken := occupier.Addr().(*net.TCPAddr).Port

	if _, err := ListenWithFallback("127.0.0.1", taken, 0); err == nil {
		t.Fatalf("expected error when port is taken and attempts=0")
	}
}
