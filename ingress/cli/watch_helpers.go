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

	// Do NOT resolve symlinks — global daemon uses the symlink path so
	// brew upgrade / package updates are picked up on restart.

	// Start global daemon if not already running.
	h, err := daemon.StatusGlobal()
	if err == nil && len(h.Services) > 0 && h.Services[0].DaemonRunning {
		// Daemon process is alive, but a running daemon executes the
		// binary it was launched with — `brew upgrade mom` (or any
		// rebuild) leaves the daemon serving the old code. The sentinel
		// recorded at install time tells us whether the binary on disk
		// has moved. Mismatch → unload so the install path below
		// re-spawns against the current binary (ADR-pointer: #338).
		match, _ := daemon.BinaryVersionMatches(bin)
		if match {
			_ = daemon.CleanupLegacy(projectRoot)
			return nil
		}
		_ = daemon.UninstallGlobal()
	}

	if err := daemon.InstallGlobal(daemon.GlobalServiceConfig{MomBinary: bin}); err != nil {
		return fmt.Errorf("installing global daemon: %w", err)
	}
	// Best-effort: record the binary identity so the next ensureGlobal
	// call can detect a future upgrade. Failure to write the sentinel
	// is non-fatal — at worst the next call treats the daemon as stale
	// and reinstalls unnecessarily.
	_ = daemon.RecordBinaryVersion(bin)

	_ = daemon.CleanupLegacy(projectRoot)
	return nil
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
