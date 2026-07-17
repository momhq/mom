package watcher

import (
	"time"

	"github.com/momhq/mom/events/envelope"
)

// SessionEvent is a harness-emitted extension event observed on a
// transcript — e.g. OATS type:"event" lines from momOS
// (delegation.spawned, session.titled, session.archived,
// context.compacted, memory.folded, plan/todo updates). Adapters that
// understand such lines implement EventExtractor; the watcher publishes
// each SessionEvent through the Editor as a capture.event.observed
// event.
//
// These events are session-level facts, not conversation: they are
// stored durably in the Ledger (delegation.spawned links a momOS
// manager session to the Claude Code worker session it spawned, which
// MOM ingests separately — the edge enables cross-referencing) but are
// not folded into the markdown vault, mirroring the ADR 0025 treatment
// of non-prose operational events.
type SessionEvent struct {
	SessionID string
	Timestamp time.Time
	Name      string         // harness event name, e.g. "delegation.spawned"
	Payload   map[string]any // harness event payload, carried verbatim
	Harness   string         // writing harness, from the session header

	// ProjectId carries the resolved project identity (ADR 0016), stamped
	// by the watcher at publish time. Empty means "unknown".
	ProjectId string

	// Cwd is the working directory the session header reported. The
	// watcher prefers it for project resolution over the configured
	// ProjectDir (same rule as Turn.Cwd).
	Cwd string
}

// ToPayload renders the SessionEvent into the map[string]any payload of
// the capture.event.observed event appended to the Ledger.
//
// Keys: "event", "payload", "harness", "project_id".
func (e SessionEvent) ToPayload() map[string]any {
	out := map[string]any{
		"event": e.Name,
	}
	if len(e.Payload) > 0 {
		out["payload"] = e.Payload
	}
	if e.Harness != "" {
		out["harness"] = e.Harness
	}
	if e.ProjectId != "" {
		out["project_id"] = e.ProjectId
	}
	return out
}

// Canonical implements editor.Canonicalizer. It exposes SessionEvent as
// a canonical envelope.EventObserved event whose payload is the
// ToPayload() shape.
func (e SessionEvent) Canonical() (envelope.EventType, map[string]any) {
	payload := e.ToPayload()
	if e.SessionID != "" {
		payload["session_id"] = e.SessionID
	}
	return envelope.EventObserved, payload
}
