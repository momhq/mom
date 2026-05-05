package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/momhq/mom/cli/internal/config"
	"github.com/momhq/mom/cli/internal/daemon"
	"github.com/momhq/mom/cli/internal/drafter"
	"github.com/momhq/mom/cli/internal/herald"
	"github.com/momhq/mom/cli/internal/librarian"
	"github.com/momhq/mom/cli/internal/logbook"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/momhq/mom/cli/internal/ux"
	"github.com/momhq/mom/cli/internal/vault"
	"github.com/momhq/mom/cli/internal/watcher"
	"github.com/spf13/cobra"
)

var (
	watchTranscriptDir string
	watchDebounceMs    int
	watchStatus        bool
	watchHarness       string
	watchSweep         bool
	watchInstall       bool
	watchUninstall     bool
	watchGlobal        bool
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch runtime transcripts and ingest turns automatically",
	Long: `Starts a filesystem watcher on a runtime transcript directory and
ingests new conversation turns into .mom/raw/ without MCP calls or hook overhead.

Supported runtimes:
  claude    — ~/.claude/projects/ (default)
  windsurf  — ~/.windsurf/transcripts/
  pi        — ~/.pi/agent/sessions/

Each session's JSONL transcript is tailed incrementally.
Cursor files in .mom/raw/ track the last ingested byte offset per session,
so restarts are safe and idempotent.

The watcher runs in the foreground. Use Ctrl-C to stop.`,
	RunE:          runWatch,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	watchCmd.Flags().StringVar(&watchHarness, "harness", "claude",
		`Harness to watch: "claude" (default), "windsurf", or "pi"`)
	watchCmd.Flags().StringVar(&watchHarness, "runtime", "claude",
		`deprecated: use --harness`)
	_ = watchCmd.Flags().MarkDeprecated("runtime", "use --harness instead")
	watchCmd.Flags().StringVar(&watchTranscriptDir, "dir", "",
		"Transcript directory to watch (overrides the runtime default)")
	watchCmd.Flags().IntVar(&watchDebounceMs, "debounce", 300,
		"Milliseconds to wait after a write event before reading (debounce)")
	watchCmd.Flags().BoolVar(&watchStatus, "status", false,
		"Show watch cursors and ingested sessions, then exit")
	watchCmd.Flags().BoolVar(&watchSweep, "sweep", false,
		"One-shot mode: catch up on unprocessed transcripts and exit")
	watchCmd.Flags().BoolVar(&watchInstall, "install", false,
		"Install system daemon and periodic sweep timer for background recording")
	watchCmd.Flags().BoolVar(&watchUninstall, "uninstall", false,
		"Remove system daemon and periodic sweep timer")
	watchCmd.Flags().BoolVar(&watchGlobal, "global", false,
		"Run as a single global daemon watching all registered projects")
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
	sc, ok := scope.NearestWritable(cwd)
	if !ok {
		return fmt.Errorf("no .mom/ found from %q — run mom init first", cwd)
	}
	momDir := sc.Path

	if watchStatus {
		return runWatchStatus(momDir)
	}

	p := ux.NewPrinter(os.Stderr)

	if watchInstall {
		return runWatchInstall(momDir, p)
	}
	if watchUninstall {
		return runWatchUninstall(momDir, p)
	}

	projectDir := filepath.Dir(momDir)

	// Open the central vault once for this watch process. Worker is
	// shared across the per-project buses below.
	workers := openCentralWorkers()

	// Build watcher sources: if --harness is explicitly set, use single source;
	// otherwise read config and watch all enabled harnesses.
	var sources []watcher.Source
	harnessExplicit := cmd.Flags().Changed("harness") || cmd.Flags().Changed("runtime")

	if harnessExplicit {
		// Manual single-Harness mode.
		transcriptDir := watchTranscriptDir
		var adapter watcher.Adapter

		switch watchHarness {
		case "windsurf":
			adapter = &watcher.WindsurfAdapter{ProjectDir: projectDir}
		case "pi":
			adapter = watcher.NewPiAdapter()
		case "claude", "":
			adapter = watcher.NewClaudeAdapter()
		default:
			return fmt.Errorf("unknown harness %q — supported: claude, windsurf, pi", watchHarness)
		}
		if transcriptDir == "" {
			transcriptDir = harnessTranscriptDir(watchHarness)
		}

		sources = []watcher.Source{{
			Harness:       watchHarness,
			TranscriptDir: transcriptDir,
			Adapter:       adapter,
		}}
	} else {
		// Config-driven multi-Harness mode (daemon default).
		momCfg, err := config.Load(momDir)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		sources = buildWatcherSources(momCfg, projectDir)
		if len(sources) == 0 {
			return fmt.Errorf("no watcher-capable runtimes enabled in config")
		}
	}

	// Sweep mode: one-shot catch-up and exit.
	if watchSweep {
		adapterMap := make(map[string]watcher.Adapter, len(sources))
		for _, src := range sources {
			adapterMap[src.Harness] = src.Adapter
		}
		bus := newProjectBus(momDir, adapterMap, workers)
		w, err := watcher.New(watcher.Config{
			ProjectDir: projectDir,
			MomDir:     momDir,
			Sources:    sources,
			SweepOnly:  true,
			Bus:        bus,
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

	// Herald event bus: watcher publishes RecordAppended events,
	// Logbook and Drafter subscribe as downstream processors.
	adapterMap := make(map[string]watcher.Adapter, len(sources))
	for _, src := range sources {
		adapterMap[src.Harness] = src.Adapter
	}
	bus := newProjectBus(momDir, adapterMap, workers)

	w, err := watcher.New(watcher.Config{
		ProjectDir: projectDir,
		MomDir:     momDir,
		Sources:    sources,
		DebounceMs: watchDebounceMs,
		Bus:        bus,
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
	p.Chevron(fmt.Sprintf("target: %s/raw/", momDir))
	p.Muted("press Ctrl-C to stop")
	p.Blank()

	if err := w.Run(); err != nil {
		return fmt.Errorf("watcher stopped: %w", err)
	}
	return nil
}

// newProjectBus creates a Herald event bus with Logbook and Drafter
// subscribers wired for a given momDir. Used by both single-project
// and global watch modes. adapters maps Harness name → Adapter for
// Harness-specific logbook parsing.
//
// `lb` is the central-vault Logbook worker (one per process); if
// non-nil, it is subscribed to TurnObserved events on this bus. nil
// means the central vault could not be opened — the bus still
// functions for the legacy RecordAppended subscribers below.
//
// Two wiring tiers coexist while #240 is in flight:
//
//  1. Legacy path: v1 logbook.ParseTranscript + v1 drafter.Process,
//     subscribed to RecordAppended. Writes session-*.json + draft
//     memory files under momDir. Stays operational through #240.
//  2. New path: logbook.Worker subscribed to TurnObserved,
//     persisting metadata projections through Librarian into the
//     central vault at $HOME/.mom/mom.db. Drafter joins this path
//     in #240 PR 2.
func newProjectBus(momDir string, adapters map[string]watcher.Adapter, workers centralWorkers) *herald.Bus {
	bus := herald.NewBus()
	workers.AttachToBus(bus)

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

		// Use Harness-specific parser when available, fall back to Claude format.
		var sessionLog *logbook.SessionLog
		var err error
		if rt, ok := e.Payload["runtime"].(string); ok {
			if adapter, ok := adapters[rt]; ok {
				if sp, ok := adapter.(watcher.SessionParser); ok {
					sessionLog, err = sp.ParseSession(tp, sid)
				}
			}
		}
		if sessionLog == nil && err == nil {
			sessionLog, err = logbook.ParseTranscript(tp, sid)
		}
		if err != nil || sessionLog == nil {
			return
		}
		outPath := filepath.Join(logsDir, fmt.Sprintf("session-%s.json", sid))
		data, _ := json.MarshalIndent(sessionLog, "", "  ")
		_ = os.WriteFile(outPath, append(data, '\n'), 0644)
	})

	// v1 file-based drafter (RecordAppended → .mom/memory/*.json) was
	// retired in #240 PR 3. Drafter now consumes turn.observed via
	// centralWorkers.AttachToBus and persists into the central vault.

	return bus
}

// runWatchGlobal runs the global watch daemon: watches all registered projects.
func runWatchGlobal(sweepOnly bool) error {
	reg, err := daemon.LoadRegistry()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	// Open the central vault ONCE for the entire global daemon. The
	// same Logbook worker is shared across every per-project bus
	// below — no N-vault-handle leak in multi-project mode.
	workers := openCentralWorkers()

	if sweepOnly {
		p := ux.NewPrinter(os.Stderr)
		totalSessions, totalTurns := 0, 0
		// Sweep all registered projects and exit.
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
			adapterMap := make(map[string]watcher.Adapter, len(sources))
			for _, src := range sources {
				adapterMap[src.Harness] = src.Adapter
			}
			bus := newProjectBus(entry.MomDir, adapterMap, workers)
			w, err := watcher.New(watcher.Config{
				ProjectDir: projDir,
				MomDir:     entry.MomDir,
				Sources:    sources,
				SweepOnly:  true,
				Bus:        bus,
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

	// Persistent watch mode: one watcher per registered project.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

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
		adapterMap := make(map[string]watcher.Adapter, len(sources))
		for _, src := range sources {
			adapterMap[src.Harness] = src.Adapter
		}
		bus := newProjectBus(entry.MomDir, adapterMap, workers)
		w, err := watcher.New(watcher.Config{
			ProjectDir: projDir,
			MomDir:     entry.MomDir,
			Sources:    sources,
			DebounceMs: 300,
			Bus:        bus,
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

// runWatchStatus prints cursor files in .mom/raw/ for inspection.
func runWatchStatus(momDir string) error {
	p := ux.NewPrinter(os.Stderr)
	rawDir := filepath.Join(momDir, "raw")
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		if os.IsNotExist(err) {
			p.Warn(fmt.Sprintf("no raw dir at %s — nothing recorded yet", rawDir))
			return nil
		}
		return fmt.Errorf("reading raw dir: %w", err)
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
			cf := filepath.Join(rawDir, e.Name())
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

// centralWorkers bundles the two Herald subscribers that need a
// Librarian: Drafter (filter pipeline + memory persistence) and
// Logbook (operational stream). Returned together because they share
// the same Vault — we open the vault once per process and use it for
// both.
type centralWorkers struct {
	drafter *drafter.Drafter
	logbook *logbook.Worker
}

// AttachToBus subscribes both workers to the given bus with the
// correct topic set:
//
//   - Drafter consumes turn.observed and memory.record (write path)
//   - Logbook consumes turn.observed (privacy-projected audit) AND
//     op.memory.created / op.memory.redacted / op.memory.dropped
//     (Drafter's outcome events, persisted as audit rows)
//
// No-op when the workers are nil — openCentralWorkers returns a zero
// value when vault.Open fails. The bus continues to function for
// legacy v1 subscribers in that case.
//
// Encapsulating both subscriptions here is the single place a future
// "what does Logbook record for this bus?" change needs to land.
func (cw centralWorkers) AttachToBus(bus *herald.Bus) {
	if cw.drafter != nil {
		cw.drafter.SubscribeAll(bus)
	}
	if cw.logbook != nil {
		cw.logbook.SubscribeTurnObserved(bus)
		cw.logbook.SubscribeAll(bus,
			herald.OpMemoryCreated,
			herald.OpMemoryRedacted,
			herald.OpMemoryDropped,
		)
	}
}

// openCentralWorkers opens the central vault at $HOME/.mom/mom.db,
// runs migrations, and constructs the workers bound to it. Returns
// zero values + logs to stderr on any failure (HOME resolution,
// MkdirAll, vault.Open) — callers can still use the bus for legacy
// subscribers.
//
// Called once per process, NOT per project. The same workers are
// subscribed to every project's bus by newProjectBus; SQLite WAL +
// the librarian/vault concurrency contract keep this safe across
// goroutines.
//
// The vault stays open for the process's lifetime. The runtime owns
// the lifecycle; on shutdown the OS reclaims the handle. A future
// refactor should plumb an explicit Close, but for alpha this is
// acceptable.
func openCentralWorkers() centralWorkers {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: cannot resolve $HOME: %v — central workers not wired\n", err)
		return centralWorkers{}
	}
	momHome := filepath.Join(home, ".mom")
	if err := os.MkdirAll(momHome, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "watch: cannot create %s: %v — central workers not wired\n", momHome, err)
		return centralWorkers{}
	}
	dbPath := filepath.Join(momHome, "mom.db")
	migs := append(librarian.Migrations(), logbook.Migrations()...)
	v, err := vault.Open(dbPath, migs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: vault.Open %s: %v — central workers not wired\n", dbPath, err)
		return centralWorkers{}
	}
	lib := librarian.New(v)
	return centralWorkers{
		drafter: drafter.New(lib),
		logbook: logbook.New(lib),
	}
}
