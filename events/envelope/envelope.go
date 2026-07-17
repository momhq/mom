// Package envelope defines MOM's canonical event envelope — the on-the-wire
// shape every captured event takes between Ingress, the Editor, and the Ledger.
//
// In v0.50 there is no event bus. The Editor canonicalizes a parsed turn into
// an Event and appends it to the Ledger (the source of truth); the projection
// fold reads Events back from the Ledger. This package is a pure value type —
// no behavior, no pub/sub.
package envelope

import "time"

// EventType identifies a category of event using the family.subject.verb
// taxonomy (ADR 0018).
type EventType string

const (
	// TurnObserved is the source-of-truth event the watcher records for every
	// parsed turn. Its payload carries the full structured turn — role, text,
	// tool_calls, usage, model, provider.
	TurnObserved EventType = "capture.turn.observed"

	// MemoryRecord is an explicit memory-write event. Retained so historical
	// Ledger records still decode; the v0.50 capture-only model has no live
	// producer for it.
	MemoryRecord EventType = "capture.memory.recorded"

	// EventObserved records a harness-emitted extension event line observed
	// on a transcript (e.g. OATS type:"event" lines — delegation.spawned,
	// session.titled). Published by the watcher for adapters implementing
	// EventExtractor. Durable in the Ledger; not folded into the vault.
	EventObserved EventType = "capture.event.observed"

	// Operational event types — published by the Toad OS ingest surface
	// (services/ingest) via the shared Editor + Ledger pipeline. These make
	// MOM's ledger the canonical company record for agent-centric operations.

	// OperationalMessagePosted records a message posted in a channel or thread.
	OperationalMessagePosted EventType = "operational.message.posted"

	// OperationalRunStarted records the start of an agent run.
	OperationalRunStarted EventType = "operational.run.started"

	// OperationalRunFinished records the terminal state of an agent run.
	OperationalRunFinished EventType = "operational.run.finished"

	// OperationalApprovalRequested records a request for human approval.
	OperationalApprovalRequested EventType = "operational.approval.requested"

	// OperationalApprovalResolved records the resolution of a pending approval.
	OperationalApprovalResolved EventType = "operational.approval.resolved"

	// OperationalTaskCreated records a new task in the company task ledger.
	OperationalTaskCreated EventType = "operational.task.created"

	// OperationalTaskUpdated records a status or ownership change on a task.
	OperationalTaskUpdated EventType = "operational.task.updated"

	// OperationalWorkspaceRegistered records a new workspace being registered.
	OperationalWorkspaceRegistered EventType = "operational.workspace.registered"
)

// Event is the canonical envelope appended to the Ledger.
//
// Type and SessionID are first-class fields — producers set them, consumers
// read them directly. Timestamp is the event's wall-clock time (the Ledger
// additionally stamps a durable AppendedAt). Payload carries the per-type
// fields; the envelope is type-agnostic.
type Event struct {
	Type      EventType
	SessionID string
	Timestamp time.Time
	Payload   map[string]any
}
