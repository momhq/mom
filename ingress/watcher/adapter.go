// Package watcher provides filesystem-based transcript ingestion for MOM.
// It watches Harness transcript directories and appends structured
// turn.observed events to the Ledger through the Editor.
package watcher

// Adapter parses Harness-specific transcript lines into Turn values.
// Each Harness (Claude Code, Codex, Pi) has its own adapter.
type Adapter interface {
	// Name returns the adapter's Harness identifier.
	Name() string

	// ExtractTurn parses a single JSONL line and returns the rich
	// per-turn shape the Editor canonicalizes into a turn.observed event.
	// Returns (zero, false) for lines that do not produce a meaningful
	// turn (tool_result, system messages, sidechain entries, malformed
	// JSON).
	//
	// The returned Turn carries raw text and tool inputs; the fold and
	// lens read paths are responsible for not surfacing sensitive inputs.
	ExtractTurn(line []byte, sessionID string) (Turn, bool)
}

// EventExtractor is optionally implemented by adapters whose Harness
// interleaves session-level extension event lines in its transcripts
// (e.g. OATS type:"event" lines — delegation.spawned, session.titled).
// The watcher calls ExtractEvent for lines that did not produce a Turn
// and publishes each SessionEvent as a capture.event.observed event.
type EventExtractor interface {
	// ExtractEvent parses a single JSONL line into a SessionEvent.
	// Returns (zero, false) for lines that are not extension events.
	ExtractEvent(line []byte, sessionID string) (SessionEvent, bool)
}

// SessionPrimer is optionally implemented by adapters whose per-session
// attribution state lives on the transcript's first line (e.g. the OATS
// session header). The watcher calls PrimeSession before ingesting a
// file from a non-zero cursor offset, so a process restart (cold
// in-memory cache) never degrades attribution: the adapter re-reads the
// header it would otherwise have missed.
type SessionPrimer interface {
	PrimeSession(path string, sessionID string)
}

// ProjectFilter is optionally implemented by adapters that need to
// filter transcripts by project (e.g. harnesses that use a flat
// transcript directory with no per-project subdirectories).
type ProjectFilter interface {
	// BelongsToProject reads a transcript file and returns true if it
	// belongs to the adapter's configured project directory.
	BelongsToProject(path string) bool
}

// ToolCategorizer is optionally implemented by adapters that know how to
// bucket their Harness's tool names into Lens categories.
type ToolCategorizer interface {
	CategorizeTool(toolName string) string
}

// ProjectScoper is optionally implemented by adapters whose Harness uses a
// non-default project-slug convention for its per-project transcript
// subdirectory. The default convention (claude/codex) is
// strings.ReplaceAll(path, "/", "-"); pi (for example) uses
// "--<path-with-separators-as-dashes>--".
//
// When implemented, the watcher uses this method instead of the default
// projectSlug() to locate the scoped transcript subdirectory.
type ProjectScoper interface {
	// ProjectSlug returns the per-project subdirectory name this adapter's
	// Harness would create under its base transcript directory for the given
	// absolute project path.
	ProjectSlug(projectDir string) string
}
