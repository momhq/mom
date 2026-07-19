package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/momhq/mom/events/editor"
	"github.com/momhq/mom/services/ingest"
	"github.com/momhq/mom/storage/ledger"

	"github.com/fsnotify/fsnotify"
	"github.com/momhq/mom/ingress/watcher"
	"github.com/momhq/mom/ops/daemon"
	"github.com/momhq/mom/shared/config"
	"github.com/momhq/mom/shared/ux"
	"github.com/momhq/mom/storage/librarian"
	"github.com/spf13/cobra"
)

var (
	watchStatus  bool
	watchSweep   bool
	watchGlobal  bool
	ingestPort   int
	noIngest     bool
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch harness transcripts and ingest turns automatically",
	Long: `Starts a filesystem watcher on a harness transcript directory and
appends new conversation turns to the central Ledger at $HOME/.mom/ledger/
without hook overhead. Capture happens only for directories bound to a MOM
project (.mom-project.yaml).

Supported harnesses:
  claude    — ~/.claude/projects/ (default)
  codex     — ~/.codex/sessions/ (or $CODEX_HOME/sessions/)
  pi        — ~/.pi/agent/sessions/

Each session's JSONL transcript is tailed incrementally.
Cursor files in .mom/cache/ track the last ingested byte offset per session,
so restarts are safe and idempotent.

The watcher runs in the foreground. Use Ctrl-C to stop.`,
	Args:          cobra.NoArgs,
	RunE:          runWatch,
	SilenceUsage:  true,
	SilenceErrors: false,
}

func init() {
	watchCmd.Flags().BoolVar(&watchStatus, "status", false,
		"Show watch cursors and ingested sessions, then exit")
	watchCmd.Flags().BoolVar(&watchSweep, "sweep", false,
		"One-shot mode: catch up on unprocessed transcripts and exit")
	watchCmd.Flags().BoolVar(&watchGlobal, "global", false,
		"Run as a single global daemon watching all registered projects")
	_ = watchCmd.Flags().MarkHidden("global")
	watchCmd.Flags().IntVar(&ingestPort, "ingest-port", ingest.DefaultPort,
		"TCP port for the operational event ingress server (global mode only)")
	watchCmd.Flags().BoolVar(&noIngest, "no-ingest", false,
		"Disable the operational event ingress server (global mode only)")
}

func runWatch(cmd *cobra.Command, _ []string) error {
	// Global mode doesn't need a project-local .mom/ — handle it first.
	if watchGlobal {
		return runWatchGlobal(watchSweep)
	}

	cwd, _ := os.Getwd()
	if envDir := os.Getenv("MOM_PROJECT_DIR"); envDir != "" {
		cwd = envDir
	}
	projectDir, momDir, err := resolveMomContext(cwd)
	if err != nil {
		return err
	}

	if watchStatus {
		return runWatchStatus(momDir)
	}

	p := ux.NewPrinter(os.Stderr)

	// Config-driven multi-harness mode. Manual per-harness overrides were kept
	// out of the v1 public CLI; init/upgrade own harness configuration.
	momCfg, err := config.Load(momDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	sources := buildWatcherSources(momCfg, projectDir)
	if len(sources) == 0 {
		return fmt.Errorf("no watcher-capable harnesses enabled in config")
	}

	// Build the write pipeline once: watcher → Editor → Ledger.
	pipe := openLedgerPipeline()

	// Sweep mode: one-shot catch-up and exit.
	if watchSweep {
		w, err := watcher.New(watcher.Config{
			ProjectDir: projectDir,
			MomDir:     momDir,
			Sources:    sources,
			SweepOnly:  true,
			Editor:     pipe.ed,
		})
		if err != nil {
			return fmt.Errorf("creating watcher: %w", err)
		}
		sessions, turns := w.Sweep()
		if sessions > 0 {
			p.Checkf("sweep: %s sessions, %s turns",
				p.HighlightValue(fmt.Sprintf("%d", sessions)),
				p.HighlightValue(fmt.Sprintf("%d", turns)))
		} else {
			p.Muted("sweep: nothing new")
		}
		return nil
	}

	w, err := watcher.New(watcher.Config{
		ProjectDir: projectDir,
		MomDir:     momDir,
		Sources:    sources,
		DebounceMs: 300,
		Editor:     pipe.ed,
	})
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}

	// Print startup info.
	harnessNames := make([]string, len(sources))
	for i, src := range sources {
		harnessNames[i] = src.Harness
	}
	p.Diamond(fmt.Sprintf("watch [%s]", strings.Join(harnessNames, ", ")))
	for rt, dir := range w.TranscriptDirs() {
		p.Chevron(fmt.Sprintf("%s: %s", rt, dir))
	}
	if path, err := librarian.LedgerDir(); err == nil {
		p.Chevron(fmt.Sprintf("ledger: %s", path))
	}
	p.Muted("press Ctrl-C to stop")
	p.Blank()

	if err := w.Run(); err != nil {
		return fmt.Errorf("watcher stopped: %w", err)
	}
	return nil
}

// runWatchGlobal runs the global watch daemon: watches all registered projects.
func runWatchGlobal(sweepOnly bool) error {
	// Sweeps run constantly (Stop/SessionEnd hooks, launchd timer), which
	// makes them the natural place to notice that the mom binary on disk has
	// been swapped under the running daemon (make install, brew upgrade) —
	// a stale daemon keeps executing the old image, silently running
	// yesterday's autofold. Restart it against the current binary.
	if sweepOnly {
		refreshGlobalDaemonIfStale()
	}
	if _, err := daemon.PruneInvalidRegistry(); err != nil {
		return fmt.Errorf("pruning registry: %w", err)
	}
	reg, err := daemon.LoadRegistry()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	// Open the write pipeline ONCE: the Ledger + Editor are shared
	// across every project. All projects write to the same central
	// Ledger via the Editor's durable-append path.
	pipe := openLedgerPipeline()

	// Start the ingest server inside this process, sharing the pipeline.
	// Only in persistent (non-sweep) mode; the server needs a long-lived process.
	if !sweepOnly && !noIngest && pipe.ed != nil && pipe.led != nil {
		ingestSrv := ingest.New(pipe.ed, pipe.led)
		ln, err := ingest.ListenWithFallback("", ingestPort, 10)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mom] ingest: bind port: %v — ingress disabled\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[mom] ingest: listening on %s\n", ln.Addr())
			go func() {
				if err := http.Serve(ln, ingestSrv.Handler()); err != nil {
					fmt.Fprintf(os.Stderr, "[mom] ingest: server stopped: %v\n", err)
				}
			}()
		}
	}

	if sweepOnly {
		p := ux.NewPrinter(os.Stderr)
		totalSessions, totalTurns := 0, 0
		// Sweep all registered projects, then drain the Ledger once.
		for projDir, entry := range reg {
			cfg, err := config.Load(entry.MomDir)
			if err != nil {
				p.Warn(fmt.Sprintf("sweep %s: config: %v", projDir, err))
				continue
			}
			sources := buildWatcherSources(cfg, projDir)
			if len(sources) == 0 {
				continue
			}
			w, err := watcher.New(watcher.Config{
				ProjectDir: projDir,
				MomDir:     entry.MomDir,
				Sources:    sources,
				SweepOnly:  true,
				Editor:     pipe.ed,
			})
			if err != nil {
				p.Warn(fmt.Sprintf("sweep %s: %v", projDir, err))
				continue
			}
			sessions, turns := w.Sweep()
			totalSessions += sessions
			totalTurns += turns
			if sessions > 0 {
				p.Checkf("sweep %s: %s sessions, %s turns",
					filepath.Base(projDir),
					p.HighlightValue(fmt.Sprintf("%d", sessions)),
					p.HighlightValue(fmt.Sprintf("%d", turns)))
			}
		}
		if totalSessions == 0 {
			p.Muted("sweep: nothing new across all projects")
		}
		return nil
	}

	// Persistent watch mode: one watcher per registered project, all
	// sharing the daemon-wide pipeline.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Auto-fold (M2): the daemon owns the fold trigger. Ingestion arms
	// the tracker via the OnPublish hook; the runner folds eligible
	// projects with the same in-process path as `mom vault fold`.
	var onPublish func(projectID, harness string)
	if centralDir, derr := librarian.Dir(); derr == nil {
		onPublish = startAutofold(ctx, centralDir)
	}

	type runningWatcher struct {
		cancel context.CancelFunc
	}
	var mu sync.Mutex
	watchers := make(map[string]*runningWatcher)

	startProject := func(projDir string, entry daemon.RegistryEntry) {
		cfg, err := config.Load(entry.MomDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mom] watch %s: config: %v\n", projDir, err)
			return
		}
		sources := buildWatcherSources(cfg, projDir)
		if len(sources) == 0 {
			return
		}
		w, err := watcher.New(watcher.Config{
			ProjectDir: projDir,
			MomDir:     entry.MomDir,
			Sources:    sources,
			DebounceMs: 300,
			Editor:     pipe.ed,
			OnPublish:  onPublish,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mom] watch %s: %v\n", projDir, err)
			return
		}

		wCtx, wCancel := context.WithCancel(ctx)
		mu.Lock()
		watchers[projDir] = &runningWatcher{cancel: wCancel}
		mu.Unlock()

		go func() {
			if err := w.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "[mom] watch %s stopped: %v\n", projDir, err)
			}
		}()

		go func() {
			<-wCtx.Done()
			w.Stop() //nolint:errcheck
		}()
	}

	// Start watchers for all currently registered projects.
	for projDir, entry := range reg {
		startProject(projDir, entry)
	}

	fmt.Fprintf(os.Stderr, "[mom] global daemon: watching %d projects\n", len(reg))

	// Watch the registry file for changes (add/remove projects).
	regPath, err := daemon.RegistryPath()
	if err != nil {
		return fmt.Errorf("registry path: %w", err)
	}
	regDir := filepath.Dir(regPath)

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsnotify watcher: %w", err)
	}
	defer fw.Close()

	if err := fw.Add(regDir); err != nil {
		return fmt.Errorf("watching registry dir: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			for _, rw := range watchers {
				rw.cancel()
			}
			mu.Unlock()
			return nil

		case ev, ok := <-fw.Events:
			if !ok {
				return nil
			}
			if filepath.Base(ev.Name) != "watch-registry.json" {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			newReg, err := daemon.LoadRegistry()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[mom] reload registry: %v\n", err)
				continue
			}

			mu.Lock()
			// Stop watchers for removed projects.
			for projDir, rw := range watchers {
				if _, exists := newReg[projDir]; !exists {
					rw.cancel()
					delete(watchers, projDir)
					fmt.Fprintf(os.Stderr, "[mom] unregistered: %s\n", projDir)
				}
			}
			// Start watchers for new projects.
			for projDir, entry := range newReg {
				if _, exists := watchers[projDir]; !exists {
					startProject(projDir, entry)
					fmt.Fprintf(os.Stderr, "[mom] registered: %s\n", projDir)
				}
			}
			mu.Unlock()

		case err, ok := <-fw.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "[mom] fsnotify error: %v\n", err)
		}
	}
}

// runWatchStatus prints watcher cursor files for inspection.
func runWatchStatus(momDir string) error {
	p := ux.NewPrinter(os.Stderr)
	cursorDir := filepath.Join(momDir, "cache")
	entries, err := os.ReadDir(cursorDir)
	if err != nil {
		if os.IsNotExist(err) {
			p.Warn(fmt.Sprintf("no cache dir at %s — watcher has not run yet", cursorDir))
			return nil
		}
		return fmt.Errorf("reading cache dir: %w", err)
	}

	type cursor struct {
		sid    string
		offset string
	}
	var cursors []cursor
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".watch-cursor-") {
			sid := strings.TrimPrefix(e.Name(), ".watch-cursor-")
			cf := filepath.Join(cursorDir, e.Name())
			data, err := os.ReadFile(cf)
			if err != nil {
				continue
			}
			cursors = append(cursors, cursor{sid: sid, offset: strings.TrimSpace(string(data))})
		}
	}

	if len(cursors) == 0 {
		p.Warn("no watch cursors found — watcher has not run yet")
		return nil
	}

	p.Diamond("watch cursors")
	p.Muted(fmt.Sprintf("%d sessions", len(cursors)))
	p.Blank()
	for _, c := range cursors {
		p.Chevron(fmt.Sprintf("%s: %s bytes", c.sid, c.offset))
	}
	return nil
}

// ledgerPipeline is the v0.50 write path: the watcher publishes parsed
// turns through the Editor, which canonicalizes and durably appends them
// to the central Ledger. One per watch process, shared across projects.
type ledgerPipeline struct {
	ed  *editor.Editor
	led *ledger.Ledger
}

// openLedgerPipeline opens the central Ledger and builds the Editor that
// appends to it. Returns a pipeline with nil ed/led when the Ledger is
// unavailable (logged to stderr); production callers degrade gracefully
// without aborting startup.
func openLedgerPipeline() ledgerPipeline {
	led := openCentralLedger()
	p := ledgerPipeline{led: led}
	if led != nil {
		p.ed = editor.New(nil, nil).WithLedger(led)
	}
	return p
}

// openCentralLedger opens the Ledger at $HOME/.mom/ledger/ (per
// ADR 0021). Returns the concrete *ledger.Ledger for the Editor's
// append path (LedgerAppender). Returns nil on failure so callers
// degrade gracefully without aborting startup.
func openCentralLedger() *ledger.Ledger {
	dir, err := librarian.LedgerDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: librarian.LedgerDir: %v — ledger not wired\n", err)
		return nil
	}
	led, err := ledger.Open(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: ledger.Open: %v — ledger not wired\n", err)
		return nil
	}
	return led
}
