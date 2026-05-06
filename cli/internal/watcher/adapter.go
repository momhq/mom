// Package watcher provides filesystem-based transcript ingestion for MOM.
// It watches Harness transcript directories and emits structured Turn
// events on Herald for downstream Drafter and Logbook subscribers.
package watcher

import (
	"github.com/momhq/mom/cli/internal/logbook"
)

// Adapter parses Harness-specific transcript lines into Turn values.
// Each Harness (Claude Code, Windsurf, Pi) has its own adapter.
type Adapter interface {
	// Name returns the adapter's Harness identifier.
	Name() string

	// ExtractTurn parses a single JSONL line and returns the rich
	// per-turn shape consumed by Drafter (filter pipeline) and
	// Logbook (metadata projection). Returns (zero, false) for lines
	// that do not produce a meaningful turn (tool_result, system
	// messages, sidechain entries, malformed JSON).
	//
	// The returned Turn carries raw text and tool inputs. Drafter
	// applies the redaction pipeline; Logbook projects to a
	// privacy-safe metadata shape (no text, no inputs) before
	// persisting. The full Turn never lands on disk.
	ExtractTurn(line []byte, sessionID string) (Turn, bool)
}

// SessionParser is optionally implemented by adapters that provide
// Harness-specific logbook parsing. Falls back to logbook.ParseTranscript
// (Claude Code format) when not implemented.
type SessionParser interface {
	ParseSession(transcriptPath, sessionID string) (*logbook.SessionLog, error)
}

// ProjectFilter is optionally implemented by adapters that need to
// filter transcripts by project (e.g. Windsurf, which uses a flat
// transcript directory with no per-project subdirectories).
type ProjectFilter interface {
	// BelongsToProject reads a transcript file and returns true if it
	// belongs to the adapter's configured project directory.
	BelongsToProject(path string) bool
}

// ToolCategorizer is optionally implemented by adapters that know how to
// bucket their Harness's tool names into logbook categories. Falls back to
// logbook.categorizeTool when not implemented or when an empty string is
// returned for an unknown tool.
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
