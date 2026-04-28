// Package watcher provides filesystem-based transcript ingestion for MOM.
// It watches Claude Code transcript directories and normalizes entries to
// RawEntry format compatible with the existing drafter pipeline.
package watcher

import (
	"github.com/momhq/mom/cli/internal/logbook"
	"github.com/momhq/mom/cli/internal/recorder"
)

// Adapter parses Harness-specific transcript lines into RawEntry values.
// Each Harness (Claude Code, Windsurf, Pi) has its own adapter.
type Adapter interface {
	// Name returns the adapter's Harness identifier.
	Name() string

	// ParseLine parses a single JSONL line from a transcript file.
	// Returns (entry, true) if the line yields a recordable entry,
	// (zero, false) if the line should be skipped (tool_use, metadata, etc.).
	ParseLine(line []byte, sessionID string) (recorder.RawEntry, bool)
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
