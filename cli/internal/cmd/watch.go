package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/momhq/mom/cli/internal/scope"
	"github.com/momhq/mom/cli/internal/watcher"
	"github.com/spf13/cobra"
)

var (
	watchTranscriptDir string
	watchDebounceMs    int
	watchStatus        bool
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch Claude Code transcripts and ingest turns automatically",
	Long: `Starts a filesystem watcher on the Claude Code transcript directory
(default: ~/.claude/projects/) and ingests new conversation turns into
.mom/raw/ without MCP calls or hook overhead.

Each Claude Code session's JSONL transcript is tailed incrementally.
Cursor files in .mom/raw/ track the last ingested byte offset per session,
so restarts are safe and idempotent.

The watcher runs in the foreground. Use Ctrl-C to stop.`,
	RunE:          runWatch,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	watchCmd.Flags().StringVar(&watchTranscriptDir, "dir", "~/.claude/projects/",
		"Transcript directory to watch (Claude Code: ~/.claude/projects/)")
	watchCmd.Flags().IntVar(&watchDebounceMs, "debounce", 300,
		"Milliseconds to wait after a write event before reading (debounce)")
	watchCmd.Flags().BoolVar(&watchStatus, "status", false,
		"Show watch cursors and ingested sessions, then exit")
}

func runWatch(cmd *cobra.Command, _ []string) error {
	cwd, _ := os.Getwd()
	if envDir := os.Getenv("MOM_PROJECT_DIR"); envDir != "" {
		cwd = envDir
	}
	sc, ok := scope.NearestWritable(cwd)
	if !ok {
		return fmt.Errorf("no .mom/ found from %q — run mom init first", cwd)
	}
	momDir := sc.Path

	if watchStatus {
		return runWatchStatus(momDir)
	}

	cfg := watcher.Config{
		TranscriptDir: watchTranscriptDir,
		MomDir:        momDir,
		Adapter:       watcher.NewClaudeAdapter(),
		DebounceMs:    watchDebounceMs,
	}

	w, err := watcher.New(cfg)
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}

	fmt.Fprintf(os.Stderr, "watching %s → %s/raw/\n", watchTranscriptDir, momDir)
	fmt.Fprintf(os.Stderr, "press Ctrl-C to stop\n")

	if err := w.Run(); err != nil {
		return fmt.Errorf("watcher stopped: %w", err)
	}
	return nil
}

// runWatchStatus prints cursor files in .mom/raw/ for inspection.
func runWatchStatus(momDir string) error {
	rawDir := filepath.Join(momDir, "raw")
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "no raw dir at %s — nothing recorded yet\n", rawDir)
			return nil
		}
		return fmt.Errorf("reading raw dir: %w", err)
	}

	var cursors []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".watch-cursor-") {
			sid := strings.TrimPrefix(e.Name(), ".watch-cursor-")
			cf := filepath.Join(rawDir, e.Name())
			data, err := os.ReadFile(cf)
			if err != nil {
				continue
			}
			cursors = append(cursors, fmt.Sprintf("  %s: %s bytes", sid, strings.TrimSpace(string(data))))
		}
	}

	if len(cursors) == 0 {
		fmt.Fprintf(os.Stderr, "no watch cursors found — watcher has not run yet\n")
		return nil
	}

	fmt.Fprintf(os.Stderr, "watch cursors (%d sessions):\n", len(cursors))
	for _, c := range cursors {
		fmt.Fprintln(os.Stderr, c)
	}
	return nil
}
