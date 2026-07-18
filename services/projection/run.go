package projection

import (
	"context"
	"fmt"
)

// RunOptions parametrizes one in-process fold pass. It is the shared
// entry point behind both `mom vault fold`/`rebuild` (ingress/cli/vault.go)
// and the global watch daemon's auto-fold (services/autofold) — the two
// callers must never diverge in fold semantics.
type RunOptions struct {
	ProjectID string
	Root      string // project root; the vault lives at <root>/.mom/vault
	LedgerDir string
	Rebuild   bool // fold from offset 0 and prune on full completion

	Engine    string // "auto" (default) | "claude" | "codex" | "pi"
	Model     string // empty = engine's cheap default (claude: haiku)
	ChunkSize int    // events per synthesizer call; default 60
	Parallel  int    // concurrent synthesis calls for L0/L1; default 4

	Progress func(string) // optional step labels (spinner, log)
	Warn     func(string) // optional non-fatal warnings
}

// RunSummary reports what a fold pass did — everything the CLI summary
// and the daemon's watch.log lines need.
type RunSummary struct {
	EngineName    string
	FromOffset    uint64
	Head          uint64
	FoldedThrough uint64
	EventsFolded  int
	ChunksTotal   int
	FilesWritten  int
	VaultDir      string
	ClaudePath    string
	// PendingSynthesis is true when L1/L2 was aborted by a systemic
	// harness failure (usage limit, auth) — the next fold completes it.
	PendingSynthesis bool
	// Partial is true when synthesis stopped before the read head — the
	// next fold retries the remaining window.
	Partial bool
	// Pruned is true when a fully-completed rebuild deleted stale files.
	Pruned bool
}

// Complete reports whether the fold consumed its whole window with no
// interrupted synthesis. Callers treating partial folds as failures
// (the daemon's backoff) key off this.
func (s RunSummary) Complete() bool { return !s.Partial && !s.PendingSynthesis }

// RunProjectFold executes one fold pass for a project, holding the
// per-project fold lock for its whole duration: watermark load → Ledger
// read → synthesis (with per-step checkpoints) → vault write. Returns
// ErrFoldLocked without doing anything when another fold (manual or
// daemon) is already running for the same project.
func RunProjectFold(ctx context.Context, opts RunOptions) (RunSummary, error) {
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = defaultChunkSize
	}
	if opts.Parallel <= 0 {
		opts.Parallel = 4
	}
	if opts.Engine == "" {
		opts.Engine = "auto"
	}
	warn := opts.Warn
	if warn == nil {
		warn = func(string) {}
	}

	lock, err := AcquireFoldLock(opts.Root)
	if err != nil {
		return RunSummary{}, err
	}
	defer lock.Release()

	// Determine the watermark and existing chunk map to resume from.
	var fromOffset uint64
	var existingChunks map[string]string
	if !opts.Rebuild {
		st, found, serr := LoadFoldState(opts.Root)
		if serr != nil {
			return RunSummary{}, serr
		}
		if found {
			fromOffset = st.LastOffset
			existingChunks = st.Chunks
		}
	}

	reader := NewReader(opts.LedgerDir, opts.ProjectID)
	read, err := reader.Read(fromOffset)
	if err != nil {
		return RunSummary{}, err
	}

	existing := map[string]string{}
	if !opts.Rebuild {
		existing, err = LoadExisting(opts.Root)
		if err != nil {
			return RunSummary{}, err
		}
	}

	synth, engineName, err := NewSynthesizer(opts.Engine, opts.Model, warn)
	if err != nil {
		return RunSummary{}, err
	}

	writer := NewWriter(opts.Root)
	in := FoldInput{
		ProjectID:      opts.ProjectID,
		ProjectRoot:    opts.Root,
		FromOffset:     fromOffset,
		ToOffset:       read.Head,
		Existing:       existing,
		Events:         read.Events,
		ExistingChunks: existingChunks,
		Engine:         engineName,
		Progress:       opts.Progress,
		Parallel:       opts.Parallel,
		// Persist every completed synthesis step immediately: a Ctrl-C or
		// crash mid-fold loses at most one call's work, and the next fold
		// resumes from the checkpointed watermark + chunk cache.
		Checkpoint: func(changed, chunks map[string]string, watermark uint64) {
			if cerr := writer.Checkpoint(changed, chunks, watermark); cerr != nil {
				warn(fmt.Sprintf("checkpoint failed (%v); continuing — progress since the last good checkpoint is at risk", cerr))
			}
		},
	}
	chunks := (len(read.Events) + opts.ChunkSize - 1) / opts.ChunkSize
	if chunks == 0 {
		chunks = 1 // single refresh pass even with no new events
	}

	res, err := Fold(ctx, synth, in, opts.ChunkSize)
	if err != nil {
		return RunSummary{}, fmt.Errorf("synthesis failed: %w", err)
	}

	// Persist the watermark the synthesis actually reached — on a partial
	// fold that is behind the read head, so the next fold retries the
	// remaining window instead of skipping it. 0 is a real value here (a
	// rebuild whose very first window failed): promoting it to read.Head
	// would mark every event folded when NONE was, silently skipping them
	// forever — that is how fallout-overseer's fold-state got poisoned.
	foldedThrough := res.FoldedThrough

	// Prune (delete files absent from the fresh set) ONLY when the rebuild
	// fully completed: L0 consumed the whole window and L1/L2 were not
	// interrupted. An aborted rebuild has a near-empty fresh set — pruning
	// on it deletes the entire existing vault (this destroyed a 2500-episode
	// vault when a rebuild ran against a logged-out harness CLI).
	prune := opts.Rebuild && foldedThrough >= read.Head && !res.PendingSynthesis

	wres, err := writer.Write(res, foldedThrough, len(read.Events), prune)
	if err != nil {
		return RunSummary{}, err
	}

	return RunSummary{
		EngineName:       engineName,
		FromOffset:       fromOffset,
		Head:             read.Head,
		FoldedThrough:    foldedThrough,
		EventsFolded:     len(read.Events),
		ChunksTotal:      chunks,
		FilesWritten:     wres.FilesWritten,
		VaultDir:         wres.VaultDir,
		ClaudePath:       wres.ClaudePath,
		PendingSynthesis: res.PendingSynthesis,
		Partial:          foldedThrough < read.Head,
		Pruned:           prune,
	}, nil
}
