package watcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/momhq/mom/events/envelope"
)

// oatsCaptureWatcher builds a Watcher wired to an OatsAdapter and an
// in-memory capture appender (sibling of captureWatcher, which is
// Claude-wired).
func oatsCaptureWatcher(projectDir, momDir, transcriptDir string) (*Watcher, *captureAppender) {
	w, cap := captureWatcher(projectDir, momDir, transcriptDir)
	w.cfg.Adapter = NewOatsAdapter()
	return w, cap
}

func bindProject(t *testing.T, projectDir, id string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(projectDir, ".mom-project.yaml"),
		[]byte("version: \"1\"\nid: "+id+"\n"), 0o644); err != nil {
		t.Fatalf("write bind file: %v", err)
	}
}

// oatsHeaderWithCwd builds a session header whose cwd points at the
// given (bound) project directory — per-turn cwd resolution gates
// capture on the cwd the header reports, exactly as in production.
func oatsHeaderWithCwd(cwd string) string {
	return `{"schema":"oats/1","type":"session","id":"d3446c23-427f-485d-b183-3ccdc08f6b8b","harness":"momos","harness_version":"0.1.0","timestamp":"2026-07-17T22:36:39.056Z","cwd":"` + cwd + `","platform":"macos","model":{"provider":"anthropic","id":"claude-sonnet-4-6"}}`
}

// The watcher publishes one capture.event.observed per extension event
// line, alongside the turn.observed events, through the same Editor
// pipeline and privacy gate.
func TestIngestFile_PublishesEventObserved_OATS(t *testing.T) {
	transcriptDir := t.TempDir()
	momDir := t.TempDir()
	projectDir := t.TempDir()
	bindProject(t, projectDir, "alpha")
	w, cap := oatsCaptureWatcher(projectDir, momDir, transcriptDir)

	transcriptPath := filepath.Join(transcriptDir, "2026-07-17T22-36-39-d3446c23.jsonl")
	body := oatsHeaderWithCwd(projectDir) + "\n" + oatsUserTurn + "\n" + oatsEventFixture + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	w.ingestFile(transcriptPath)

	captured := cap.all()
	if len(captured) != 2 {
		t.Fatalf("got %d events, want 2 (turn.observed + event.observed)", len(captured))
	}
	turnEv := captured[0]
	if turnEv.Type != envelope.TurnObserved {
		t.Errorf("captured[0].Type = %q, want %q", turnEv.Type, envelope.TurnObserved)
	}
	ev := captured[1]
	if ev.Type != envelope.EventObserved {
		t.Fatalf("captured[1].Type = %q, want %q", ev.Type, envelope.EventObserved)
	}
	if got, _ := ev.Payload["event"].(string); got != "delegation.spawned" {
		t.Errorf("payload[event] = %v, want delegation.spawned", ev.Payload["event"])
	}
	if got, _ := ev.Payload["harness"].(string); got != "momos" {
		t.Errorf("payload[harness] = %v, want momos", ev.Payload["harness"])
	}
	inner, _ := ev.Payload["payload"].(map[string]any)
	if got, _ := inner["child_session_id"].(string); got != "d66f90d2" {
		t.Errorf("payload.payload.child_session_id = %v, want d66f90d2 (manager→worker edge)", inner)
	}
	if ev.SessionID != "d3446c23-427f-485d-b183-3ccdc08f6b8b" {
		t.Errorf("SessionID = %q, want session id from line", ev.SessionID)
	}
}

// Extension events obey the same privacy gate as turns: unbound
// directories publish nothing.
func TestIngestFile_SkipsEventWhenUnbound(t *testing.T) {
	transcriptDir := t.TempDir()
	momDir := t.TempDir()
	projectDir := t.TempDir() // no .mom-project.yaml

	w, cap := oatsCaptureWatcher(projectDir, momDir, transcriptDir)

	transcriptPath := filepath.Join(transcriptDir, "2026-07-17T22-36-39-d3446c23.jsonl")
	body := oatsSessionHeaderLine + "\n" + oatsEventFixture + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	w.ingestFile(transcriptPath)

	if captured := cap.all(); len(captured) != 0 {
		t.Fatalf("got %d events, want 0 (unbound capture must be skipped)", len(captured))
	}
}

// A watcher restart between two writes of the same session must not
// degrade attribution: the second ingest resumes from a mid-file cursor
// with a cold adapter cache, and PrimeSession re-reads the header.
func TestIngestFile_ResumePrimesHeaderAfterRestart(t *testing.T) {
	transcriptDir := t.TempDir()
	momDir := t.TempDir()
	projectDir := t.TempDir()
	bindProject(t, projectDir, "alpha")

	transcriptPath := filepath.Join(transcriptDir, "2026-07-17T22-36-39-d3446c23.jsonl")
	body := oatsHeaderWithCwd(projectDir) + "\n" + oatsUserTurn + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	// First watcher ingests header + first turn; cursor advances past both.
	w1, cap1 := oatsCaptureWatcher(projectDir, momDir, transcriptDir)
	w1.ingestFile(transcriptPath)
	if got := len(cap1.all()); got != 1 {
		t.Fatalf("first ingest: got %d events, want 1", got)
	}

	// Session continues: another user turn lands.
	more := `{"type":"message","role":"user","session_id":"d3446c23-427f-485d-b183-3ccdc08f6b8b","timestamp":"2026-07-17T22:40:00.000Z","content":[{"type":"text","text":"and one more thing"}]}` + "\n"
	f, err := os.OpenFile(transcriptPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("append transcript: %v", err)
	}
	if _, err := f.WriteString(more); err != nil {
		t.Fatalf("append transcript: %v", err)
	}
	_ = f.Close()

	// Second watcher (fresh adapter — simulates a restart) shares the
	// cursor dir, so it resumes mid-file with a cold header cache.
	w2, cap2 := oatsCaptureWatcher(projectDir, momDir, transcriptDir)
	w2.ingestFile(transcriptPath)

	captured := cap2.all()
	if len(captured) != 1 {
		t.Fatalf("resume ingest: got %d events, want 1", len(captured))
	}
	if got, _ := captured[0].Payload["harness"].(string); got != "momos" {
		t.Errorf("payload[harness] = %v, want momos (PrimeSession must restore the header on resume)", captured[0].Payload["harness"])
	}
}

// The OnPublish hook fires once per published turn AND per published
// session event, with the resolved project id and source harness — the
// daemon's auto-fold tracker depends on this contract.
func TestIngestFile_OnPublishHookFires(t *testing.T) {
	transcriptDir := t.TempDir()
	momDir := t.TempDir()
	projectDir := t.TempDir()
	bindProject(t, projectDir, "alpha")
	w, _ := oatsCaptureWatcher(projectDir, momDir, transcriptDir)

	type obs struct{ project, harness string }
	var observed []obs
	w.cfg.OnPublish = func(projectID, harness string) {
		observed = append(observed, obs{projectID, harness})
	}

	transcriptPath := filepath.Join(transcriptDir, "2026-07-17T22-36-39-d3446c23.jsonl")
	body := oatsHeaderWithCwd(projectDir) + "\n" + oatsUserTurn + "\n" + oatsEventFixture + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	w.ingestFile(transcriptPath)

	if len(observed) != 2 {
		t.Fatalf("OnPublish fired %d times, want 2 (turn + event)", len(observed))
	}
	for i, o := range observed {
		if o.project != "alpha" {
			t.Errorf("observed[%d].project = %q, want alpha", i, o.project)
		}
		if o.harness != "momos" {
			t.Errorf("observed[%d].harness = %q, want momos", i, o.harness)
		}
	}
}

// Tool results ride on the canonical turn.observed payload under
// "tool_results", with name/call_id/content/is_error.
func TestTurn_ToPayload_EmitsToolResults(t *testing.T) {
	turn := Turn{
		SessionID: "s",
		Role:      "tool",
		ToolResults: []ToolResult{{
			Name:    "Bash",
			CallID:  "toolu_1",
			Content: "exit status 1",
			IsError: true,
		}},
	}
	p := turn.ToPayload()
	trs, _ := p["tool_results"].([]map[string]any)
	if len(trs) != 1 {
		t.Fatalf("payload[tool_results] = %v, want 1 entry", p["tool_results"])
	}
	if trs[0]["name"] != "Bash" || trs[0]["call_id"] != "toolu_1" {
		t.Errorf("tool_results[0] = %v, want name/call_id preserved", trs[0])
	}
	if trs[0]["content"] != "exit status 1" {
		t.Errorf("tool_results[0][content] = %v", trs[0]["content"])
	}
	if trs[0]["is_error"] != true {
		t.Errorf("tool_results[0][is_error] = %v, want true", trs[0]["is_error"])
	}
}

// When ToolResults is empty, the payload omits the key entirely.
func TestTurn_ToPayload_OmitsToolResultsWhenEmpty(t *testing.T) {
	turn := Turn{SessionID: "s", Role: "user", Text: "hi"}
	p := turn.ToPayload()
	if _, ok := p["tool_results"]; ok {
		t.Errorf("payload should omit tool_results when empty, got %v", p["tool_results"])
	}
}
