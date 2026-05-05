// Package drafter has two surfaces in v0.30. This file contains the
// Worker — the Herald subscriber that consumes turn.observed and
// memory.record events, runs the filter pipeline, and persists
// memories through Librarian. The legacy batch processor in
// drafter.go (read .mom/raw/, write .mom/memory/*.json) coexists
// during the cleanup transition and is removed in #240 PR 3.
package drafter

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/momhq/mom/cli/internal/herald"
	"github.com/momhq/mom/cli/internal/librarian"
)

// MemoryRecordEventType mirrors the constant defined by the MCP
// package so Drafter doesn't have to import mcp (which would create
// an import cycle). Kept in sync by convention; if either side
// changes the string, the integration test fails first.
const MemoryRecordEventType herald.EventType = "memory.record"

// Worker subscribes to turn.observed and memory.record on Herald,
// runs the filter pipeline (turn.observed only), and persists memories
// through Librarian. It also bumps filter_audit on hard-filter fires
// and emits op.memory.* events for Logbook to record.
//
// One Worker per process. Same Librarian-via-Vault concurrency
// guarantees apply as everywhere else (SQLite WAL + the librarian
// boundary).
type Worker struct {
	lib *librarian.Librarian
	now func() time.Time
}

// NewWorker returns a Worker bound to the given Librarian. Named
// NewWorker (not New) while the legacy batch-processor's drafter.New
// constructor is still alive in this package; it gets renamed to the
// canonical drafter.New in #240 PR 3 once the v1 surface is deleted.
func NewWorker(lib *librarian.Librarian) *Worker {
	return &Worker{lib: lib, now: func() time.Time { return time.Now().UTC() }}
}

// SubscribeAll wires the worker to both event types it consumes.
// Returns a single unsubscribe that detaches both.
func (w *Worker) SubscribeAll(bus *herald.Bus) func() {
	stopTurns := w.SubscribeTurnObserved(bus)
	stopRecord := w.SubscribeRecord(bus)
	return func() {
		stopTurns()
		stopRecord()
	}
}

// SubscribeTurnObserved registers the watcher-driven capture path.
// Each turn flows through the soft filter (noise drop), then the
// hard filter (secret redact-and-persist). filter_audit counters
// bump on every distinct category that fired, regardless of whether
// the memory ends up persisted.
func (w *Worker) SubscribeTurnObserved(bus *herald.Bus) func() {
	return bus.Subscribe(herald.TurnObserved, func(e herald.Event) {
		if e.SessionID == "" {
			fmt.Fprintf(os.Stderr, "drafter: drop %q event with empty session_id\n", e.Type)
			return
		}
		if err := w.processTurn(bus, e); err != nil {
			fmt.Fprintf(os.Stderr, "drafter: process turn (session=%s): %v\n", e.SessionID, err)
		}
	})
}

// SubscribeRecord registers the explicit-write path from mom_record.
// Filters are bypassed — the user's explicitness wins over MOM's
// heuristics per ADR 0014. Provenance fields come from the event
// payload (set by the MCP handler).
func (w *Worker) SubscribeRecord(bus *herald.Bus) func() {
	return bus.Subscribe(MemoryRecordEventType, func(e herald.Event) {
		if e.SessionID == "" {
			fmt.Fprintf(os.Stderr, "drafter: drop %q event with empty session_id\n", e.Type)
			return
		}
		if err := w.processRecord(bus, e); err != nil {
			fmt.Fprintf(os.Stderr, "drafter: process record (session=%s): %v\n", e.SessionID, err)
		}
	})
}

// processTurn applies the filter pipeline to a turn.observed event
// and persists the resulting memory if it survives.
func (w *Worker) processTurn(bus *herald.Bus, e herald.Event) error {
	role, _ := e.Payload["role"].(string)
	text, _ := e.Payload["text"].(string)
	model, _ := e.Payload["model"].(string)
	provider, _ := e.Payload["provider"].(string)
	harness, _ := e.Payload["harness"].(string)

	tcs := tcsFromPayload(e.Payload["tool_calls"])

	// Soft filter — drop silently if the turn is noise.
	soft := softTurn{
		Role:                   role,
		Text:                   text,
		ToolCount:              len(tcs),
		CodebaseWriteToolCount: countCategory(tcs, "codebase_write"),
	}
	if isNoise(soft) {
		bus.Publish(herald.Event{
			Type:      "op.memory.dropped",
			SessionID: e.SessionID,
			Payload: map[string]any{
				"reason":  "soft_filter",
				"role":    role,
				"harness": harness,
			},
		})
		return nil
	}

	// Hard filter — redact secrets in text AND in any tool_input
	// values. Bump filter_audit per distinct category that fired.
	redactedText, textCats := redactSecrets(text)
	categories := map[string]struct{}{}
	for _, c := range textCats {
		categories[c] = struct{}{}
	}
	for _, tc := range tcs {
		// Tool inputs are not persisted in the memory, but if a
		// secret-shaped value is in there, we still want filter_audit
		// to know the filter fired for that turn — useful for the
		// "is the filter catching anything?" lens panel.
		if tc.Input == nil {
			continue
		}
		blob, err := json.Marshal(tc.Input)
		if err != nil {
			continue
		}
		_, cats := redactSecrets(string(blob))
		for _, c := range cats {
			categories[c] = struct{}{}
		}
	}
	for cat := range categories {
		if err := w.lib.IncrementFilterAudit(cat); err != nil {
			fmt.Fprintf(os.Stderr, "drafter: filter_audit bump %q: %v\n", cat, err)
		}
	}

	// Build the persisted content. v0.30 stores only the redacted
	// text under $.text — Lens consumes the FTS5 trigger that
	// extracts that key. tool_input never lands here.
	contentBytes, err := json.Marshal(map[string]any{"text": redactedText})
	if err != nil {
		return fmt.Errorf("marshal content: %w", err)
	}

	// Provenance: turn observed by the watcher.
	actor := harness
	if actor == "" {
		actor = "watcher"
	}
	id, err := w.lib.Insert(librarian.InsertMemory{
		Content:                string(contentBytes),
		SessionID:              e.SessionID,
		ProvenanceActor:        actor,
		ProvenanceSourceType:   "transcript-extraction",
		ProvenanceTriggerEvent: "watcher",
		CreatedAt:              extractCreatedAt(e.Payload),
	})
	if err != nil {
		return fmt.Errorf("insert memory: %w", err)
	}

	// Emit op.memory.created or op.memory.redacted depending on
	// whether the hard filter fired.
	opType := herald.EventType("op.memory.created")
	if len(categories) > 0 {
		opType = "op.memory.redacted"
	}
	bus.Publish(herald.Event{
		Type:      opType,
		SessionID: e.SessionID,
		Payload: map[string]any{
			"memory_id":  id,
			"role":       role,
			"harness":    harness,
			"provider":   provider,
			"model":      model,
			"categories": categoriesSlice(categories),
		},
	})
	return nil
}

// processRecord persists a memory.record event verbatim — no filters,
// no redaction, no filter_audit bumps. The user's explicit-write
// override per ADR 0014.
func (w *Worker) processRecord(bus *herald.Bus, e herald.Event) error {
	rawContent, _ := e.Payload["content"].(map[string]any)
	if len(rawContent) == 0 {
		return fmt.Errorf("memory.record event has empty content")
	}

	contentBytes, err := json.Marshal(rawContent)
	if err != nil {
		return fmt.Errorf("marshal content: %w", err)
	}

	summary, _ := e.Payload["summary"].(string)
	actor, _ := e.Payload["provenance_actor"].(string)
	if actor == "" {
		actor = "mcp"
	}
	source, _ := e.Payload["provenance_source_type"].(string)
	if source == "" {
		source = "manual-draft"
	}
	trigger, _ := e.Payload["provenance_trigger_event"].(string)
	if trigger == "" {
		trigger = "record"
	}

	id, err := w.lib.Insert(librarian.InsertMemory{
		Content:                string(contentBytes),
		Summary:                summary,
		SessionID:              e.SessionID,
		ProvenanceActor:        actor,
		ProvenanceSourceType:   source,
		ProvenanceTriggerEvent: trigger,
	})
	if err != nil {
		return fmt.Errorf("insert memory: %w", err)
	}

	// Tags: link each provided tag through Librarian. Tag names
	// arrive already normalised (the mom_record handler does that).
	for _, name := range tagsFromPayload(e.Payload["tags"]) {
		tagID, err := w.lib.UpsertTag(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "drafter: UpsertTag %q for record %s: %v\n", name, id, err)
			continue
		}
		if err := w.lib.LinkTag(id, tagID); err != nil {
			fmt.Fprintf(os.Stderr, "drafter: LinkTag memory=%s tag=%s: %v\n", id, tagID, err)
		}
	}

	bus.Publish(herald.Event{
		Type:      "op.memory.created",
		SessionID: e.SessionID,
		Payload: map[string]any{
			"memory_id": id,
			"trigger":   trigger,
			"actor":     actor,
		},
	})
	return nil
}

// ── helpers: payload extraction ──────────────────────────────────────────────

// payloadToolCall mirrors watcher.ToolCall as a plain shape we can
// build from map[string]any without importing watcher (which would
// drag the whole transcript-parsing surface into Drafter).
type payloadToolCall struct {
	Name     string
	Input    map[string]any
	Category string
}

func tcsFromPayload(v any) []payloadToolCall {
	switch tcs := v.(type) {
	case []map[string]any:
		out := make([]payloadToolCall, 0, len(tcs))
		for _, tc := range tcs {
			out = append(out, payloadToolCallFromMap(tc))
		}
		return out
	case []any:
		out := make([]payloadToolCall, 0, len(tcs))
		for _, item := range tcs {
			tc, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, payloadToolCallFromMap(tc))
		}
		return out
	}
	return nil
}

func payloadToolCallFromMap(m map[string]any) payloadToolCall {
	name, _ := m["name"].(string)
	cat, _ := m["category"].(string)
	input, _ := m["input"].(map[string]any)
	return payloadToolCall{Name: name, Input: input, Category: cat}
}

func countCategory(tcs []payloadToolCall, cat string) int {
	n := 0
	for _, tc := range tcs {
		if tc.Category == cat {
			n++
		}
	}
	return n
}

func tagsFromPayload(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, x := range s {
			if str, ok := x.(string); ok && str != "" {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func extractCreatedAt(payload map[string]any) time.Time {
	if v, ok := payload["created_at"]; ok {
		if t, ok := v.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

func categoriesSlice(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	return out
}
