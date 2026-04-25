// Package watcher provides filesystem-based transcript ingestion for MOM.
// It watches Claude Code transcript directories and normalizes entries to
// RawEntry format compatible with the existing drafter pipeline.
package watcher

import (
	"github.com/momhq/mom/cli/internal/recorder"
)

// Adapter parses runtime-specific transcript lines into RawEntry values.
// Each runtime (Claude Code, Windsurf, Codex) has its own adapter.
type Adapter interface {
	// Name returns the adapter's runtime identifier.
	Name() string

	// ParseLine parses a single JSONL line from a transcript file.
	// Returns (entry, true) if the line yields a recordable entry,
	// (zero, false) if the line should be skipped (tool_use, metadata, etc.).
	ParseLine(line []byte, sessionID string) (recorder.RawEntry, bool)
}
