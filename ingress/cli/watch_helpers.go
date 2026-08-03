package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/momhq/mom/ingress/harness"
	"github.com/momhq/mom/ingress/watcher"
	"github.com/momhq/mom/ops/daemon"
	"github.com/momhq/mom/shared/config"
	"github.com/momhq/mom/shared/pathutil"
	"github.com/momhq/mom/storage/librarian"
)

// harnessTranscriptDir resolves a Harness's default transcript directory via
// its TranscriptSource implementation. Returns "" if the Harness is unknown
// or has no transcript source.
func harnessTranscriptDir(name string) string {
	reg := harness.NewRegistry("")
	h, ok := reg.Get(name)
	if !ok {
		return ""
	}
	if ts, ok := h.(harness.TranscriptSource); ok {
		return ts.DefaultTranscriptDir()
	}
	return ""
}

func resolveMomContext(cwd string) (projectDir string, momDir string, err error) {
	cwd = pathutil.CanonicalDir(cwd)
	centralDir, err := librarian.Dir()
	if err != nil {
		return "", "", err
	}
	if _, err := os.Stat(filepath.Join(centralDir, "config.yaml")); err != nil {
		return "", "", fmt.Errorf("no MOM configuration found from %q — run mom init first", cwd)
	}
	return cwd, centralDir, nil
}

// ensureGlobalDaemon registers the project in the global watch registry and
// ensures the single global daemon is running. Also cleans up legacy per-project agents.
//
// Registry write (prune + register) always runs — including under tests and
// under MOM_NO_DAEMON=1 — because the registry file is the source of user
// intent and the running daemon hot-loads it via fsnotify. Only the daemon
// install / launchctl side effects are skipped under the test/no-daemon gate.
func ensureGlobalDaemon(projectRoot, momDir string, harnesses []string) error {
	projectRoot = pathutil.CanonicalDir(projectRoot)

	// ADR 0016: require an explicit project binding before adding this
	// directory to the daemon registry. Without this, running `mom init`
	// or `mom upgrade` from $HOME (or any unrelated cwd) would silently
	// promote that directory into a permanently-watched project.
	if _, err := os.Stat(filepath.Join(projectRoot, ".mom-project.yaml")); err != nil {
		return fmt.Errorf("refusing to watch %s: no .mom-project.yaml binding (run `mom project bind <id>`)", projectRoot)
	}

	// Prune stale pre-v0.40 registry entries before registering this project.
	_, _ = daemon.PruneInvalidRegistry()

	// Register this project in the global registry.
	if err := daemon.RegisterProject(projectRoot, momDir, harnesses); err != nil {
		return fmt.Errorf("registering project: %w", err)
	}

	if os.Getenv("MOM_NO_DAEMON") == "1" {
		return nil
	}

	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving binary path: %w", err)
	}

	// Skip daemon install when running inside `go test`.
	if strings.HasSuffix(bin, ".test") || strings.Contains(bin, "/_test/") {
		return nil
	}

	// Install/update the stable copy the daemon is always registered
	// against — never os.Executable() directly, which orphans the daemon
	// once the caller's own path stops being stable (e.g. a Tauri sidecar
	// path that changes on every app update). A Homebrew install
	// reconciles the same way: `brew upgrade` leaves a newer binary behind
	// /opt/homebrew/bin/mom, so the next reconcile copies it in like any
	// other caller.
	stableBin, copied, err := daemon.ReconcileStableBinary(bin)
	if err != nil {
		return fmt.Errorf("reconciling stable binary: %w", err)
	}

	// Start global daemon if not already running.
	h, err := daemon.StatusGlobal()
	if err == nil && len(h.Services) > 0 && h.Services[0].DaemonRunning {
		// Daemon process is alive, but a running daemon executes the
		// binary it was launched with. If the stable copy didn't change,
		// the daemon is already serving the current code.
		if !copied {
			_ = daemon.CleanupLegacy(projectRoot)
			return nil
		}
		// The stable copy just changed — restart in place so the daemon
		// picks up the new binary (ADR-pointer: #338).
		if err := daemon.RestartGlobal(); err != nil {
			return fmt.Errorf("restarting global daemon: %w", err)
		}
		_ = daemon.CleanupLegacy(projectRoot)
		return nil
	}

	if err := daemon.InstallGlobal(daemon.GlobalServiceConfig{MomBinary: stableBin}); err != nil {
		return fmt.Errorf("installing global daemon: %w", err)
	}

	_ = daemon.CleanupLegacy(projectRoot)
	return nil
}

// refreshGlobalDaemonIfStale restarts the global watch daemon when the mom
// binary on disk no longer matches the one the daemon launched with. The
// daemon is registered against the stable path (see StableBinaryPath), so
// once it's running, ReconcileStableBinary here always finds running ==
// stable and this is a no-op — an installed daemon never observes a stale
// binary via the sweep. Actual `make install` / `brew upgrade` pickup
// happens on the next foreground `mom` invocation (running from the
// non-stable path), which reconciles and restarts the daemon. This sweep
// path only self-heals a crashed/not-yet-running daemon.
func refreshGlobalDaemonIfStale() {
	if os.Getenv("MOM_NO_DAEMON") == "1" {
		return
	}
	bin, err := os.Executable()
	if err != nil {
		return
	}
	if strings.HasSuffix(bin, ".test") || strings.Contains(bin, "/_test/") {
		return
	}
	stableBin, copied, err := daemon.ReconcileStableBinary(bin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mom] reconciling stable binary: %v\n", err)
		return
	}
	h, err := daemon.StatusGlobal()
	running := err == nil && len(h.Services) > 0 && h.Services[0].DaemonRunning
	if !running {
		// Not running at all (crashed, or a past failed refresh left it
		// uninstalled) — install it fresh. Safe: there is no live service for
		// this process to boot out from under itself.
		fmt.Fprintln(os.Stderr, "[mom] global watch daemon not running; installing it")
		if err := daemon.InstallGlobal(daemon.GlobalServiceConfig{MomBinary: stableBin}); err != nil {
			fmt.Fprintf(os.Stderr, "[mom] daemon install failed: %v\n", err)
		}
		return
	}
	if !copied {
		return
	}
	// Restart IN PLACE — never uninstall/reinstall here: this code also runs
	// from the launchd sweep-timer's own process, and booting out the timer
	// service kills that process mid-refresh, leaving everything uninstalled.
	fmt.Fprintln(os.Stderr, "[mom] binary changed on disk; restarting global watch daemon")
	if err := daemon.RestartGlobal(); err != nil {
		fmt.Fprintf(os.Stderr, "[mom] daemon restart failed: %v\n", err)
	}
}

// buildWatcherSources builds watcher.Source entries from config for all
// watcher-capable Harnesses.
func buildWatcherSources(cfg *config.Config, projectDir string) []watcher.Source {
	var sources []watcher.Source
	for _, rt := range cfg.EnabledHarnesses() {
		var (
			override string
			adapter  watcher.Adapter
		)
		switch rt {
		case "claude":
			override = cfg.Watcher.TranscriptDir
			adapter = watcher.NewClaudeAdapter()
		case "pi":
			override = cfg.Watcher.PiTranscriptDir
			adapter = watcher.NewPiAdapter()
		case "codex":
			override = cfg.Watcher.CodexTranscriptDir
			adapter = watcher.NewCodexAdapter()
		default:
			continue
		}
		dir := override
		if dir == "" {
			dir = harnessTranscriptDir(rt)
		}
		if dir == "" {
			continue
		}
		sources = append(sources, watcher.Source{
			Harness:       rt,
			TranscriptDir: dir,
			Adapter:       adapter,
		})
	}

	// OATS (Open Agent Transcript Standard) is harness-agnostic: any
	// conformant writer (momOS today, others later) lands sessions under a
	// single shared root, and the adapter attributes each session to the
	// harness named in its line-1 header. It is therefore not gated on
	// EnabledHarnesses — the source is added whenever the transcript root
	// exists on disk.
	if dir, ok := oatsTranscriptDir(cfg); ok {
		sources = append(sources, watcher.Source{
			Harness:       "oats",
			TranscriptDir: dir,
			Adapter:       watcher.NewOatsAdapter(),
		})
	}
	return sources
}

// defaultOatsTranscriptDir is the OATS standard transcript root.
const defaultOatsTranscriptDir = "~/.transcripts/"

// oatsTranscriptDir returns the configured (or default) OATS transcript
// root and whether it exists as a directory. The unexpanded form is
// returned — watcher.New performs tilde expansion itself, matching how
// the other harness sources are passed through.
func oatsTranscriptDir(cfg *config.Config) (string, bool) {
	dir := cfg.Watcher.OatsTranscriptDir
	if dir == "" {
		dir = defaultOatsTranscriptDir
	}
	resolved := dir
	if strings.HasPrefix(resolved, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		resolved = filepath.Join(home, resolved[1:])
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return "", false
	}
	return dir, true
}
