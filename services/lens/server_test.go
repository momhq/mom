package lens

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/momhq/mom/events/envelope"
	"github.com/momhq/mom/shared/archtest"
	"github.com/momhq/mom/storage/ledger"
)

// testLedger returns a fresh Ledger directory for the test.
func testLedger(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// appendEvent appends one event to the Ledger at dir.
func appendEvent(t *testing.T, dir string, ev envelope.Event) {
	t.Helper()
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatalf("ledger.Open: %v", err)
	}
	defer func() { _ = l.Close() }()
	if _, err := l.Append(ev); err != nil {
		t.Fatalf("ledger.Append: %v", err)
	}
}

func newTestServer(t *testing.T, dir string) *Server {
	t.Helper()
	srv, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv
}

func appendTurn(t *testing.T, dir, sessionID, role string, at time.Time, payload map[string]any) {
	t.Helper()
	if payload == nil {
		payload = map[string]any{}
	}
	payload["role"] = role
	appendEvent(t, dir, envelope.Event{
		Type:      envelope.TurnObserved,
		SessionID: sessionID,
		Timestamp: at,
		Payload:   payload,
	})
}

func appendMemory(t *testing.T, dir, sessionID, summary string, at time.Time) {
	t.Helper()
	appendEvent(t, dir, envelope.Event{
		Type:      envelope.MemoryRecord,
		SessionID: sessionID,
		Timestamp: at,
		Payload:   map[string]any{"summary": summary},
	})
}

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

func fetchDetail(t *testing.T, h http.Handler, id string) SessionDetail {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+id, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got SessionDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return got
}

func TestSessions_RenderFromTurnObserved(t *testing.T) {
	dir := testLedger(t)
	at := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	appendTurn(t, dir, "s-new", "user", at, nil)
	appendTurn(t, dir, "s-new", "assistant", at.Add(time.Minute), map[string]any{
		"tool_calls": []any{
			map[string]any{"name": "Read", "category": "codebase_read"},
			map[string]any{"name": "mom recall", "category": "mom_cli", "safe_name": "mom recall"},
		},
		"usage":    map[string]any{"total_tokens": float64(42), "cost_usd": 0.02},
		"model":    "claude-sonnet",
		"provider": "anthropic",
	})

	got := fetchSessions(t, newTestServer(t, dir).Handler())
	if len(got) != 1 {
		t.Fatalf("sessions = %+v", got)
	}
	if got[0].SessionID != "s-new" || got[0].Interactions != 1 || got[0].ToolsTotal != 2 || got[0].TotalTokens != 42 || got[0].ScopeLabel != "central" {
		t.Fatalf("summary = %+v", got[0])
	}
	detail := fetchDetail(t, newTestServer(t, dir).Handler(), "s-new")
	if detail.ToolCalls["mom_cli"].Detail["mom recall"] != 1 || detail.ToolCalls["codebase_read"].Detail["Read"] != 1 {
		t.Fatalf("tool detail missing per-call tool names: %+v", detail.ToolCalls)
	}
}

// Memory-recorded events surface their session with a memory count. The
// Ledger carries no promotion state / landmarks / tags (those lived in the
// retired SQLite vault), so MemoryItem is content + timestamp only.
func TestSessions_MemoryRecordedSessionAppears(t *testing.T) {
	dir := testLedger(t)
	at := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	appendMemory(t, dir, "s-memory", "remember this", at)

	srv := newTestServer(t, dir)
	got := fetchSessions(t, srv.Handler())
	if len(got) != 1 || got[0].SessionID != "s-memory" || got[0].MemoryCount != 1 || got[0].Interactions != 0 {
		t.Fatalf("sessions = %+v", got)
	}
	detail := fetchDetail(t, srv.Handler(), "s-memory")
	if len(detail.Memories) != 1 || detail.Memories[0].Summary != "remember this" {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestLensResponseDoesNotLeakToolInputSentinel(t *testing.T) {
	dir := testLedger(t)
	appendTurn(t, dir, "s-private", "assistant", time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC), map[string]any{
		"tool_calls": []any{
			map[string]any{
				"name":     "Bash",
				"category": "system",
				"input":    map[string]any{"command": "AKIA-SHOULD-NOT-LEAK secrets.env echo boom"},
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/s-private", nil)
	rec := httptest.NewRecorder()
	newTestServer(t, dir).Handler().ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "AKIA") || strings.Contains(rec.Body.String(), "secrets.env") || strings.Contains(rec.Body.String(), "echo boom") {
		t.Fatalf("Lens response leaked sentinel: %s", rec.Body.String())
	}
}

func TestLensStaticHasNoScopeFilter(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(data)
	for _, forbidden := range []string{"v3-scope-row", "activeScope", "?scope=", "Scope</span>", "nav-scope"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("static UI still contains scope filter marker %q", forbidden)
		}
	}
}

func TestLensDoesNotImportLegacyStores(t *testing.T) {
	archtest.AssertNoDirectImport(t, ".",
		"github.com/momhq/mom/storage/memory",
		"github.com/momhq/mom/shared/scope",
		"github.com/momhq/mom/storage/librarian",
		"github.com/momhq/mom/workers/logbook",
	)
}

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
