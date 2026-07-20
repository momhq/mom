package ingest_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/momhq/mom/events/editor"
	"github.com/momhq/mom/services/ingest"
	"github.com/momhq/mom/storage/ledger"
)

// openTestLedger opens a temporary ledger for use in tests.
func openTestLedger(t *testing.T) (*ledger.Ledger, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mom-ingest-test-*")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	led, err := ledger.Open(dir)
	if err != nil {
		t.Fatalf("ledger.Open: %v", err)
	}
	t.Cleanup(func() { _ = led.Close() })
	return led, dir
}

// openTestEditor creates an editor wired to the given ledger.
func openTestEditor(led *ledger.Ledger) *editor.Editor {
	return editor.New(nil, nil).WithLedger(led)
}

// postJSON is a helper that fires a POST to the given handler with a JSON body.
func postJSON(t *testing.T, h http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/events", bytes.NewReader(b))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// getJSON is a helper that fires a GET to the handler.
func getJSON(t *testing.T, h http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Host = "localhost"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// TestPost_Single verifies a single operational event can be posted.
func TestPost_Single(t *testing.T) {
	led, _ := openTestLedger(t)
	ed := openTestEditor(led)
	s := ingest.New(ed, led)

	rr := postJSON(t, s.Handler(), map[string]any{
		"type": "operational.message.posted",
		"payload": map[string]any{
			"project_id":  "proj-abc",
			"session_id":  "sess-1",
			"message_id":  "msg-1",
			"channel_id":  "chan-1",
			"sender_type": "agent",
			"sender_id":   "agent-1",
			"sender_name": "Toad",
			"kind":        "chat",
			"body":        "hello",
			"created_at":  "2025-01-01T00:00:00Z",
		},
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Appended int    `json:"appended"`
		Head     uint64 `json:"head"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Appended != 1 {
		t.Errorf("appended: want 1, got %d", resp.Appended)
	}
}

// TestPost_Batch verifies an array of events can be posted.
func TestPost_Batch(t *testing.T) {
	led, _ := openTestLedger(t)
	ed := openTestEditor(led)
	s := ingest.New(ed, led)

	items := []map[string]any{
		{
			"type": "operational.run.started",
			"payload": map[string]any{
				"project_id": "proj-abc",
				"session_id": "sess-2",
				"run_id":     "run-1",
				"agent_id":   "agent-1",
				"channel_id": "chan-1",
				"title":      "Do something",
				"model":      "claude-opus-4",
				"started_at": "2025-01-01T00:00:00Z",
			},
		},
		{
			"type": "operational.run.finished",
			"payload": map[string]any{
				"project_id": "proj-abc",
				"session_id": "sess-2",
				"run_id":     "run-1",
				"status":     "success",
				"ended_at":   "2025-01-01T00:01:00Z",
			},
		},
	}

	rr := postJSON(t, s.Handler(), items)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Appended int `json:"appended"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Appended != 2 {
		t.Errorf("appended: want 2, got %d", resp.Appended)
	}
}

// TestPost_MissingProjectID verifies 400 when project_id is absent.
func TestPost_MissingProjectID(t *testing.T) {
	led, _ := openTestLedger(t)
	ed := openTestEditor(led)
	s := ingest.New(ed, led)

	rr := postJSON(t, s.Handler(), map[string]any{
		"type": "operational.message.posted",
		"payload": map[string]any{
			"session_id": "sess-1",
			"body":       "hello",
		},
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestPost_WrongFamily verifies 400 when the event type is not operational.*.
func TestPost_WrongFamily(t *testing.T) {
	led, _ := openTestLedger(t)
	ed := openTestEditor(led)
	s := ingest.New(ed, led)

	rr := postJSON(t, s.Handler(), map[string]any{
		"type": "capture.turn.observed",
		"payload": map[string]any{
			"project_id": "proj-abc",
			"session_id": "sess-1",
		},
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGet_FromFilter verifies GET /api/ingest/events with from filter.
func TestGet_FromFilter(t *testing.T) {
	led, _ := openTestLedger(t)
	ed := openTestEditor(led)
	s := ingest.New(ed, led)

	// Post 3 events.
	for i := 0; i < 3; i++ {
		rr := postJSON(t, s.Handler(), map[string]any{
			"type": "operational.task.created",
			"payload": map[string]any{
				"project_id":   "proj-abc",
				"session_id":   "sess-1",
				"task_id":      "task-" + string(rune('0'+i)),
				"workspace_id": "ws-1",
				"channel_id":   "chan-1",
				"title":        "Task",
				"status":       "open",
				"acceptance":   "done",
				"created_at":   "2025-01-01T00:00:00Z",
			},
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("post[%d]: %d %s", i, rr.Code, rr.Body.String())
		}
	}

	// GET from offset 1 — should return offsets 1 and 2.
	rr := getJSON(t, s.Handler(), "/api/ingest/events?from=1&project=proj-abc")
	if rr.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Records []struct {
			Offset uint64 `json:"offset"`
		} `json:"records"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Records) != 2 {
		t.Errorf("records: want 2, got %d", len(resp.Records))
	}
	if resp.Records[0].Offset != 1 {
		t.Errorf("first record offset: want 1, got %d", resp.Records[0].Offset)
	}
}

// TestGet_ProjectFilter verifies filtering by project_id.
func TestGet_ProjectFilter(t *testing.T) {
	led, _ := openTestLedger(t)
	ed := openTestEditor(led)
	s := ingest.New(ed, led)

	// Post to two projects.
	for _, proj := range []string{"proj-A", "proj-B", "proj-A"} {
		rr := postJSON(t, s.Handler(), map[string]any{
			"type": "operational.task.updated",
			"payload": map[string]any{
				"project_id": proj,
				"session_id": "sess-1",
				"task_id":    "task-1",
				"status":     "done",
				"updated_at": "2025-01-01T00:00:00Z",
			},
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("post: %d %s", rr.Code, rr.Body.String())
		}
	}

	rr := getJSON(t, s.Handler(), "/api/ingest/events?project=proj-A")
	if rr.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Records []struct {
			Payload map[string]any `json:"payload"`
		} `json:"records"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Records) != 2 {
		t.Errorf("project filter: want 2 records for proj-A, got %d", len(resp.Records))
	}
}

// TestRoundTrip_PostThenGet is the httptest round-trip: POST then GET,
// asserting payload fidelity and offset consistency.
func TestRoundTrip_PostThenGet(t *testing.T) {
	led, _ := openTestLedger(t)
	ed := openTestEditor(led)
	s := ingest.New(ed, led)

	payload := map[string]any{
		"project_id":  "proj-rt",
		"session_id":  "sess-rt",
		"approval_id": "appr-1",
		"status":      "approved",
		"note":        "looks good",
		"resolved_at": "2025-06-01T12:00:00Z",
	}

	rr := postJSON(t, s.Handler(), map[string]any{
		"type":    "operational.approval.resolved",
		"payload": payload,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("post: %d %s", rr.Code, rr.Body.String())
	}

	var postResp struct {
		Appended int    `json:"appended"`
		Head     uint64 `json:"head"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &postResp); err != nil {
		t.Fatalf("decode post: %v", err)
	}
	if postResp.Appended != 1 {
		t.Fatalf("appended: want 1, got %d", postResp.Appended)
	}

	// GET the event back.
	rr2 := getJSON(t, s.Handler(), "/api/ingest/events?project=proj-rt")
	if rr2.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rr2.Code, rr2.Body.String())
	}

	var getResp struct {
		Records []struct {
			Offset  uint64         `json:"offset"`
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		} `json:"records"`
		Head uint64 `json:"head"`
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if len(getResp.Records) != 1 {
		t.Fatalf("get records: want 1, got %d", len(getResp.Records))
	}
	rec := getResp.Records[0]
	if rec.Type != "operational.approval.resolved" {
		t.Errorf("type: want operational.approval.resolved, got %s", rec.Type)
	}
	if rec.Offset != postResp.Head {
		t.Errorf("offset %d != post head %d", rec.Offset, postResp.Head)
	}
	if getResp.Head != postResp.Head {
		t.Errorf("get head %d != post head %d", getResp.Head, postResp.Head)
	}
	// Payload fidelity: project_id, approval_id, status, note.
	if got := rec.Payload["approval_id"]; got != "appr-1" {
		t.Errorf("approval_id: want appr-1, got %v", got)
	}
	if got := rec.Payload["note"]; got != "looks good" {
		t.Errorf("note: want 'looks good', got %v", got)
	}
}

// TestHealth verifies the health endpoint.
func TestHealth(t *testing.T) {
	s := ingest.New(nil, nil)
	rr := getJSON(t, s.Handler(), "/api/ingest/health")
	if rr.Code != http.StatusOK {
		t.Fatalf("health: %d", rr.Code)
	}
	var resp struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Error("ok: want true")
	}
	if resp.Version == "" {
		t.Error("version: want non-empty")
	}
}

// TestHead_Empty verifies GET /api/ingest/head on an empty ledger.
func TestHead_Empty(t *testing.T) {
	led, _ := openTestLedger(t)
	s := ingest.New(nil, led)
	rr := getJSON(t, s.Handler(), "/api/ingest/head")
	if rr.Code != http.StatusOK {
		t.Fatalf("head: %d", rr.Code)
	}
	var resp struct {
		Head uint64 `json:"head"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Head != 0 {
		t.Errorf("head: want 0, got %d", resp.Head)
	}
}
