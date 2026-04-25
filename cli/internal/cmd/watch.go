package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/momhq/mom/cli/internal/adapters/storage"
	"github.com/momhq/mom/cli/internal/drafter"
	"github.com/momhq/mom/cli/internal/herald"
	"github.com/momhq/mom/cli/internal/logbook"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/momhq/mom/cli/internal/ux"
	"github.com/momhq/mom/cli/internal/watcher"
	"github.com/spf13/cobra"
)

var (
	watchTranscriptDir string
	watchDebounceMs    int
	watchStatus        bool
	watchRuntime       string
)

// defaultTranscriptDirs maps runtime name to its default transcript directory.
var defaultTranscriptDirs = map[string]string{
	"claude":   "~/.claude/projects/",
	"windsurf": "~/.windsurf/transcripts/",
}

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch runtime transcripts and ingest turns automatically",
	Long: `Starts a filesystem watcher on a runtime transcript directory and
ingests new conversation turns into .mom/raw/ without MCP calls or hook overhead.

Supported runtimes:
  claude    — ~/.claude/projects/ (default)
  windsurf  — ~/.windsurf/transcripts/

Each session's JSONL transcript is tailed incrementally.
Cursor files in .mom/raw/ track the last ingested byte offset per session,
so restarts are safe and idempotent.

The watcher runs in the foreground. Use Ctrl-C to stop.`,
	RunE:          runWatch,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	watchCmd.Flags().StringVar(&watchRuntime, "runtime", "claude",
		`Runtime to watch: "claude" (default) or "windsurf"`)
	watchCmd.Flags().StringVar(&watchTranscriptDir, "dir", "",
		"Transcript directory to watch (overrides the runtime default)")
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

	// Resolve adapter and transcript directory from --runtime flag.
	var adapter watcher.Adapter
	transcriptDir := watchTranscriptDir

	// ProjectDir is the directory containing .mom/ — used to scope
	// transcript ingestion to the matching project subdirectory.
	projectDir := filepath.Dir(momDir)

	switch watchRuntime {
	case "windsurf":
		adapter = &watcher.WindsurfAdapter{ProjectDir: projectDir}
		if transcriptDir == "" {
			transcriptDir = defaultTranscriptDirs["windsurf"]
		}
	case "claude", "":
		adapter = watcher.NewClaudeAdapter()
		if transcriptDir == "" {
			transcriptDir = defaultTranscriptDirs["claude"]
		}
	default:
		return fmt.Errorf("unknown runtime %q — supported: claude, windsurf", watchRuntime)
	}

	// Herald event bus: watcher publishes RecordAppended events,
	// Logbook and Drafter subscribe as downstream processors.
	bus := herald.NewBus()

	// Logbook: parse transcript → write session metrics to .mom/logs/.
	bus.Subscribe(herald.RecordAppended, func(e herald.Event) {
		tp, _ := e.Payload["transcript_path"].(string)
		sid, _ := e.Payload["session_id"].(string)
		md, _ := e.Payload["mom_dir"].(string)
		if tp == "" || sid == "" || md == "" {
			return
		}
		logsDir := filepath.Join(md, "logs")
		_ = os.MkdirAll(logsDir, 0755)

		sessionLog, err := logbook.ParseTranscript(tp, sid)
		if err != nil {
			return
		}
		outPath := filepath.Join(logsDir, fmt.Sprintf("session-%s.json", sid))
		data, _ := json.MarshalIndent(sessionLog, "", "  ")
		_ = os.WriteFile(outPath, append(data, '\n'), 0644)
	})

	// Drafter: process raw → write draft memories to .mom/memory/.
	bus.Subscribe(herald.RecordAppended, func(e herald.Event) {
		md, _ := e.Payload["mom_dir"].(string)
		if md == "" {
			return
		}
		rawDir := filepath.Join(md, "raw")
		memDir := filepath.Join(md, "memory")
		_ = os.MkdirAll(memDir, 0755)

		since := lastDraftTime(md)
		vocabFn := func() []string { return collectVocab(memDir) }

		d := drafter.New(rawDir, memDir, vocabFn)
		drafts, err := d.Process(since)
		if err != nil || len(drafts) == 0 {
			return
		}

		idx := storage.NewIndexedAdapter(md)
		defer idx.Close()

		for _, dr := range drafts {
			_ = writeDraftDoc(idx, memDir, dr)
		}
		updateDraftMarker(md)
	})

	cfg := watcher.Config{
		TranscriptDir: transcriptDir,
		ProjectDir:    projectDir,
		MomDir:        momDir,
		Adapter:       adapter,
		DebounceMs:    watchDebounceMs,
		Bus:           bus,
	}

	w, err := watcher.New(cfg)
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}

	p := ux.NewPrinter(os.Stderr)
	p.Diamond(fmt.Sprintf("watch [%s]", watchRuntime))
	p.Chevron(fmt.Sprintf("source: %s", w.TranscriptDir()))
	p.Chevron(fmt.Sprintf("target: %s/raw/", momDir))
	p.Muted("press Ctrl-C to stop")
	p.Blank()

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
