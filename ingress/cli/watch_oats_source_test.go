package cli

import (
	"path/filepath"
	"testing"

	"github.com/momhq/mom/shared/config"
)

// The OATS source is harness-agnostic: it must be added regardless of
// which harnesses are enabled, as long as the transcript root exists.
func TestBuildWatcherSources_IncludesOatsWhenRootExists(t *testing.T) {
	cfg := config.Default()
	root := t.TempDir()
	cfg.Watcher.OatsTranscriptDir = root

	sources := buildWatcherSources(&cfg, "/tmp/project")

	for _, s := range sources {
		if s.Harness == "oats" {
			if s.TranscriptDir != root {
				t.Errorf("oats source TranscriptDir = %q, want %q", s.TranscriptDir, root)
			}
			if s.Adapter.Name() != "oats" {
				t.Errorf("oats source Adapter.Name() = %q, want oats", s.Adapter.Name())
			}
			return
		}
	}
	t.Fatalf("expected an oats source in buildWatcherSources, got %d sources (none oats)", len(sources))
}

// When the OATS transcript root does not exist, no source is added — the
// watcher must not watch a directory no OATS writer has created yet.
func TestBuildWatcherSources_SkipsOatsWhenRootMissing(t *testing.T) {
	cfg := config.Default()
	cfg.Watcher.OatsTranscriptDir = filepath.Join(t.TempDir(), "does-not-exist")

	sources := buildWatcherSources(&cfg, "/tmp/project")

	for _, s := range sources {
		if s.Harness == "oats" {
			t.Fatalf("oats source added for missing root %q", cfg.Watcher.OatsTranscriptDir)
		}
	}
}
