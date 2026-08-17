package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/momhq/mom/ops/daemon"
	"github.com/momhq/mom/services/autofold"
	"github.com/momhq/mom/services/projection"
	"github.com/momhq/mom/shared/config"
	"github.com/momhq/mom/shared/project"
	"github.com/momhq/mom/storage/librarian"
)

// startAutofold wires the M2 auto-fold loop into the persistent global
// watch daemon: a Tracker armed by the watchers' publish hook, and a
// Runner that executes the same in-process fold as `mom vault fold`
// (projection.RunProjectFold — shared semantics, shared fold lock).
//
// Returns the OnPublish hook to install on every project watcher, or nil
// when auto-folding is disabled or cannot start (the daemon then runs
// exactly as before).
func startAutofold(ctx context.Context, momDir string) func(projectID, harness string) {
	logf := watchDaemonLogf(momDir)

	cfg, err := config.Load(momDir)
	if err != nil {
		logf("autofold: config load failed (%v) — auto-folding disabled", err)
		return nil
	}
	if !cfg.Autofold.AutofoldEnabled() {
		logf("autofold: disabled by config")
		return nil
	}
	ldir, err := librarian.LedgerDir()
	if err != nil {
		logf("autofold: ledger dir (%v) — auto-folding disabled", err)
		return nil
	}

	policy := autofold.Policy{
		Harnesses:   cfg.Autofold.AutofoldHarnesses(),
		Idle:        cfg.Autofold.AutofoldIdle(),
		Backlog:     cfg.Autofold.AutofoldBacklog(),
		MinInterval: cfg.Autofold.AutofoldMinInterval(),
	}
	tracker := autofold.NewTracker(policy, nil)
	runner := &autofold.Runner{
		Tracker: tracker,
		RootFor: registryRootForProject,
		Fold: func(ctx context.Context, projectID, root string) (projection.RunSummary, error) {
			return projection.RunProjectFold(ctx, projection.RunOptions{
				ProjectID:  projectID,
				Root:       root,
				LedgerDir:  ldir,
				Engine:     "auto",
				Model:      cfg.Vault.FoldModel,
				EntryFiles: cfg.EntryFiles(),
				Warn: func(msg string) {
					logf("autofold %s: warning: %s", projectID, msg)
				},
			})
		},
		Logf: logf,
	}

	logf("autofold active: harnesses=%v idle=%s backlog=%d min_interval=%s",
		policy.Harnesses, policy.Idle, policy.Backlog, policy.MinInterval)
	go runner.Run(ctx)

	return func(projectID, harness string) {
		tracker.Observe(projectID, harness, 1)
	}
}

// registryRootForProject resolves a project id back to its registered
// project root by re-reading the daemon registry — kept live so projects
// registered after daemon start fold without a restart. Folds are rare,
// so the fresh read is cheap enough.
func registryRootForProject(projectID string) (string, bool) {
	reg, err := daemon.LoadRegistry()
	if err != nil {
		return "", false
	}
	for projDir := range reg {
		if id, _, _, rerr := project.ResolveProject(projDir); rerr == nil && id == projectID {
			return projDir, true
		}
	}
	return "", false
}

// watchDaemonLogf returns a logger appending timestamped lines to
// <momDir>/logs/watch.log — the same file the project watchers log to,
// so V sees ingestion and auto-folding in one place.
func watchDaemonLogf(momDir string) func(format string, args ...any) {
	logsDir := filepath.Join(momDir, "logs")
	_ = os.MkdirAll(logsDir, 0755)
	logFile := filepath.Join(logsDir, "watch.log")
	return func(format string, args ...any) {
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer f.Close()
		ts := time.Now().UTC().Format(time.RFC3339)
		fmt.Fprintf(f, "%s %s\n", ts, fmt.Sprintf(format, args...))
	}
}
