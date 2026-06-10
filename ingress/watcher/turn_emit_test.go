package watcher

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/momhq/mom/events/editor"
	"github.com/momhq/mom/events/envelope"
)

// captureAppender is an in-memory Ledger stand-in: it records every event the
// Editor appends so watcher tests can assert the canonical events produced
// through the v0.50 Watcher → Editor → Ledger path.
type captureAppender struct {
	mu     sync.Mutex
	events []envelope.Event
}

func (c *captureAppender) Append(e envelope.Event) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return uint64(len(c.events)), nil
}

func (c *captureAppender) all() []envelope.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]envelope.Event(nil), c.events...)
}

// captureWatcher builds a Watcher whose Editor appends to an in-memory capture
// appender. projectDir may be unbound (no .mom-project.yaml) to exercise the
// privacy gate.
func captureWatcher(projectDir, momDir, transcriptDir string) (*Watcher, *captureAppender) {
	cap := &captureAppender{}
	ed := editor.New(nil, nil).WithLedger(cap)
	w := &Watcher{
		cfg: Config{
			TranscriptDir: transcriptDir,
			ProjectDir:    projectDir,
			MomDir:        momDir,
			Adapter:       NewClaudeAdapter(),
			Editor:        ed,
			DebounceMs:    300,
		},
		timers:    make(map[string]*time.Timer),
		cursorDir: filepath.Join(momDir, "cache"),
		logFile:   filepath.Join(momDir, "watch.log"),
	}
	_ = os.MkdirAll(w.cursorDir, 0755)
	return w, cap
}

// TestIngestFile_PublishesTurnObserved locks the watcher's v0.30
// emission contract: every parsed turn produces ONE turn.observed
// event on the configured bus, with the rich payload Drafter and
// Logbook consume.
func TestIngestFile_PublishesTurnObserved(t *testing.T) {
	transcriptDir := t.TempDir()
	momDir := t.TempDir()
	projectDir := t.TempDir()
	// Capture only happens for a bound project (privacy gate).
	if err := os.WriteFile(filepath.Join(projectDir, ".mom-project.yaml"),
		[]byte("version: \"1\"\nid: alpha\n"), 0o644); err != nil {
		t.Fatalf("write bind file: %v", err)
	}
	w, cap := captureWatcher(projectDir, momDir, transcriptDir)

	// Two real Claude transcript lines: one user, one assistant with
	// usage + a tool_use block.
	transcriptPath := filepath.Join(transcriptDir, "s-emit.jsonl")
	body := claudeUserTurn + "\n" + claudeAssistantToolUseTurn + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("writing transcript: %v", err)
	}

	w.ingestFile(transcriptPath)

	captured := cap.all()
	if len(captured) != 2 {
		t.Fatalf("got %d turn.observed events, want 2", len(captured))
	}

	// First event: user turn.
	user := captured[0]
	if user.SessionID != "s-test" {
		t.Errorf("user event SessionID = %q, want s-test", user.SessionID)
	}
	if got, _ := user.Payload["role"].(string); got != "user" {
		t.Errorf("user event role = %v, want user", user.Payload["role"])
	}

	// Second event: assistant turn with tool_use.
	asst := captured[1]
	if asst.SessionID != "s-test" {
		t.Errorf("assistant event SessionID = %q, want s-test", asst.SessionID)
	}
	if got, _ := asst.Payload["role"].(string); got != "assistant" {
		t.Errorf("assistant event role = %v, want assistant", asst.Payload["role"])
	}
	tcs, _ := asst.Payload["tool_calls"].([]map[string]any)
	if len(tcs) != 2 {
		t.Errorf("tool_calls len = %d, want 2 (Read + Bash)", len(tcs))
	}
	// The raw Bash command (an AKIA-shaped sentinel in the fixture) rides on
	// the canonical event appended to the Ledger; the fold/lens read paths
	// are responsible for not surfacing it.
	if len(tcs) >= 2 {
		input, _ := tcs[1]["input"].(map[string]any)
		cmd, _ := input["command"].(string)
		if cmd == "" {
			t.Errorf("Bash tool_call.input.command should be preserved on the canonical event, got empty")
		}
	}
}

// TestIngestFile_PublishesNothingWithoutBus locks the inverse: when
// the watcher has no Bus configured, ingestion still works (legacy
// raw writer) but no events are appended. Catches a future regression
// where ingestion couples to a publish path that must always be present.
func TestIngestFile_PublishesNothingWithoutEditor(t *testing.T) {
	transcriptDir := t.TempDir()
	momDir := t.TempDir()

	w := &Watcher{
		cfg: Config{
			TranscriptDir: transcriptDir,
			MomDir:        momDir,
			Adapter:       NewClaudeAdapter(),
			DebounceMs:    300,
			// Editor intentionally nil
		},
		timers:    make(map[string]*time.Timer),
		cursorDir: filepath.Join(momDir, "cache"),
		logFile:   filepath.Join(momDir, "watch.log"),
	}
	_ = os.MkdirAll(w.cursorDir, 0755)

	transcriptPath := filepath.Join(transcriptDir, "s-no-editor.jsonl")
	if err := os.WriteFile(transcriptPath, []byte(claudeUserTurn+"\n"), 0o644); err != nil {
		t.Fatalf("writing transcript: %v", err)
	}

	// Should not panic; parsing still works and returns the turn count.
	count := w.ingestFile(transcriptPath)
	if count == 0 {
		t.Errorf("expected ingest count > 0, got 0")
	}
}

// TestIngestFile_PublishesOneEventPerTurn uses a watcher with three
// turns to verify the per-turn-not-per-batch contract.
func TestIngestFile_PublishesOneEventPerTurn(t *testing.T) {
	transcriptDir := t.TempDir()
	momDir := t.TempDir()
	projectDir := t.TempDir()
	// Capture only happens for a bound project (privacy gate).
	if err := os.WriteFile(filepath.Join(projectDir, ".mom-project.yaml"),
		[]byte("version: \"1\"\nid: alpha\n"), 0o644); err != nil {
		t.Fatalf("write bind file: %v", err)
	}
	w, cap := captureWatcher(projectDir, momDir, transcriptDir)

	transcriptPath := filepath.Join(transcriptDir, "s-three.jsonl")
	body := claudeUserTurn + "\n" + claudeAssistantTextTurn + "\n" + claudeAssistantToolUseTurn + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("writing transcript: %v", err)
	}

	w.ingestFile(transcriptPath)

	if got := len(cap.all()); got != 3 {
		t.Errorf("turn.observed appended %d times, want 3 (one per turn)", got)
	}
}

// TestTurn_ToPayload_EmitsProjectId verifies ProjectId rides on the
// Herald payload when set (per ADR 0016).
func TestTurn_ToPayload_EmitsProjectId(t *testing.T) {
	turn := Turn{
		SessionID: "s",
		Role:      "user",
		Text:      "hi",
		ProjectId: "alpha",
	}
	p := turn.ToPayload()
	if p["project_id"] != "alpha" {
		t.Errorf("payload[project_id] = %v, want alpha", p["project_id"])
	}
}

// When ProjectId is empty, the payload omits the key entirely (so
// downstream consumers can tell "stamped as alpha" from "no stamp").
func TestTurn_ToPayload_OmitsProjectIdWhenEmpty(t *testing.T) {
	turn := Turn{SessionID: "s", Role: "user", Text: "hi"}
	p := turn.ToPayload()
	if _, ok := p["project_id"]; ok {
		t.Errorf("payload should omit project_id when empty, got %v", p["project_id"])
	}
}

// TestIngestFile_StampsProjectIdFromBindFile verifies that when the
// watcher's ProjectDir contains a .mom-project.yaml, every published
// Turn carries the resolved project_id (per ADR 0016).
func TestIngestFile_StampsProjectIdFromBindFile(t *testing.T) {
	transcriptDir := t.TempDir()
	momDir := t.TempDir()
	projectDir := t.TempDir()
	// Write .mom-project.yaml at project root.
	if err := os.WriteFile(filepath.Join(projectDir, ".mom-project.yaml"),
		[]byte("version: \"1\"\nid: alpha\n"), 0o644); err != nil {
		t.Fatalf("write bind file: %v", err)
	}

	w, cap := captureWatcher(projectDir, momDir, transcriptDir)

	transcriptPath := filepath.Join(transcriptDir, "s-pid.jsonl")
	if err := os.WriteFile(transcriptPath, []byte(claudeUserTurn+"\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	w.ingestFile(transcriptPath)

	captured := cap.all()
	if len(captured) != 1 {
		t.Fatalf("got %d events, want 1", len(captured))
	}
	got, _ := captured[0].Payload["project_id"].(string)
	if got != "alpha" {
		t.Errorf("payload project_id = %q, want alpha", got)
	}
}

// TestIngestFile_StampsProjectIdFromPerTurnCwd_Claude verifies that a
// Claude transcript line's per-turn `cwd` wins project_id resolution
// over the watcher's startup ProjectDir. This is the contract that
// keeps claude-code captures correctly scoped even when no
// project-scoped watcher is registered (sibling to the Codex
// `turn_context.cwd` path).
func TestIngestFile_StampsProjectIdFromPerTurnCwd_Claude(t *testing.T) {
	transcriptDir := t.TempDir()
	momDir := t.TempDir()
	watcherProjectDir := t.TempDir() // intentionally unbound
	boundDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(boundDir, ".mom-project.yaml"),
		[]byte("version: \"1\"\nid: logbook\n"), 0o644); err != nil {
		t.Fatalf("write bind file: %v", err)
	}

	w, cap := captureWatcher(watcherProjectDir, momDir, transcriptDir)

	line := `{"type":"user","sessionId":"s-cwd","timestamp":"2026-05-05T12:00:00.000Z","isSidechain":false,"cwd":"` + boundDir + `","message":{"role":"user","content":"hi"}}`
	transcriptPath := filepath.Join(transcriptDir, "s-cwd.jsonl")
	if err := os.WriteFile(transcriptPath, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	w.ingestFile(transcriptPath)

	captured := cap.all()
	if len(captured) != 1 {
		t.Fatalf("got %d events, want 1", len(captured))
	}
	got, _ := captured[0].Payload["project_id"].(string)
	if got != "logbook" {
		t.Errorf("payload project_id = %q, want logbook (resolved from per-turn cwd, not from watcher ProjectDir)", got)
	}
}

// When no project is bound (no .mom-project.yaml resolvable from the
// watcher's ProjectDir or the turn's cwd), the watcher SKIPS capture
// entirely to protect privacy — nothing is published to the bus. The
// user is told about binding status at session start via the context
// block.
func TestIngestFile_SkipsTurnWhenUnbound(t *testing.T) {
	transcriptDir := t.TempDir()
	momDir := t.TempDir()
	projectDir := t.TempDir() // no .mom-project.yaml

	w, cap := captureWatcher(projectDir, momDir, transcriptDir)

	transcriptPath := filepath.Join(transcriptDir, "s-unbound.jsonl")
	_ = os.WriteFile(transcriptPath, []byte(claudeUserTurn+"\n"), 0o644)
	w.ingestFile(transcriptPath)

	if captured := cap.all(); len(captured) != 0 {
		t.Fatalf("got %d events, want 0 (unbound capture must be skipped)", len(captured))
	}
}

// TestTurn_ToPayload_EmitsHarness locks the contract that watcher
// adapters' harness identity (claude-code, codex, pi, …) rides on the
// herald payload so Logbook and Drafter can attribute the turn to the
// right harness. Pre-#340 ToPayload silently dropped this field.
func TestTurn_ToPayload_EmitsHarness(t *testing.T) {
	turn := Turn{
		SessionID: "s",
		Role:      "user",
		Text:      "hi",
		Harness:   "pi",
	}
	p := turn.ToPayload()
	if p["harness"] != "pi" {
		t.Errorf("payload[harness] = %v, want pi", p["harness"])
	}
}

// When Harness is empty, payload omits the key — keeps the bus clean
// for paths that don't carry harness information.
func TestTurn_ToPayload_OmitsHarnessWhenEmpty(t *testing.T) {
	turn := Turn{SessionID: "s", Role: "user", Text: "hi"}
	p := turn.ToPayload()
	if _, ok := p["harness"]; ok {
		t.Errorf("payload should omit harness when empty, got %v", p["harness"])
	}
}
