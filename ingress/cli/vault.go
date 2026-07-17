package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/momhq/mom/services/projection"
	"github.com/momhq/mom/shared/config"
	"github.com/momhq/mom/shared/project"
	"github.com/momhq/mom/shared/ux"
	"github.com/momhq/mom/storage/librarian"
	"github.com/spf13/cobra"
)

var (
	vaultProject   string
	vaultEngine    string
	vaultRoot      string
	vaultChunk     int
	vaultFoldModel string
	vaultParallel  int
)

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Project Ledger memory into a navigable markdown vault (v0.50 experiment)",
	Long: `Project a project's captured Ledger events into a navigable markdown
"vault" (an LLM-wiki) under the project's .mom/vault/, plus a small
always-loaded pointer block in the project's CLAUDE.md.

This is a strictly-additive downstream lane: it only reads the Ledger and
writes project-local markdown. The agent retrieves memory by navigating
those files rather than searching a database.`,
}

var vaultFoldCmd = &cobra.Command{
	Use:   "fold",
	Short: "Fold new Ledger events into the project vault (incremental)",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return runVaultFold(cmd, false) },
}

var vaultRebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild the project vault from Ledger offset 0 (preserves CLAUDE.md human content)",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return runVaultFold(cmd, true) },
}

var vaultStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the vault watermark, last fold time, file count, and pending events",
	Args:  cobra.NoArgs,
	RunE:  runVaultStatus,
}

func init() {
	for _, c := range []*cobra.Command{vaultFoldCmd, vaultRebuildCmd, vaultStatusCmd} {
		c.Flags().StringVar(&vaultProject, "project", "", "Override the resolved project id")
		c.Flags().StringVar(&vaultRoot, "root", "", "Override the project root for output (default: resolved project root)")
	}
	for _, c := range []*cobra.Command{vaultFoldCmd, vaultRebuildCmd} {
		c.Flags().StringVar(&vaultEngine, "engine", "auto", "Synthesis engine: auto | claude | codex | pi")
		c.Flags().IntVar(&vaultChunk, "chunk", 60, "Events per synthesizer call when folding (iterative, full-history coverage)")
		c.Flags().StringVar(&vaultFoldModel, "model", "", "Synthesis model (default: vault.fold_model config, else the engine's cheap default — claude: haiku)")
		c.Flags().IntVar(&vaultParallel, "parallel", 4, "Concurrent synthesis calls for the L0/L1 passes")
	}
	vaultCmd.AddCommand(vaultFoldCmd)
	vaultCmd.AddCommand(vaultRebuildCmd)
	vaultCmd.AddCommand(vaultStatusCmd)
}

// resolveVaultTarget resolves the project id and output root from cwd /
// flags, erroring clearly when the cwd is not in a MOM project.
func resolveVaultTarget() (projectID, root string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	id, sourceFile, found, rerr := project.ResolveProject(cwd)
	if rerr != nil {
		return "", "", rerr
	}

	root = vaultRoot
	if !found {
		if vaultProject == "" || root == "" {
			return "", "", fmt.Errorf(
				"no %s found in this directory or any parent — bind this directory with `mom project bind --id <id>` first (or pass both --project and --root)",
				project.BindFilename)
		}
	} else if root == "" {
		root = filepath.Dir(sourceFile)
	}

	projectID = id
	if vaultProject != "" {
		projectID = vaultProject
	}
	if projectID == "" {
		return "", "", fmt.Errorf("could not resolve a project id; pass --project")
	}
	if root == "" {
		return "", "", fmt.Errorf("could not resolve a project root; pass --root")
	}
	abs, aerr := filepath.Abs(root)
	if aerr != nil {
		return "", "", aerr
	}
	return projectID, abs, nil
}

func ledgerDir() (string, error) {
	return librarian.LedgerDir()
}

func runVaultFold(cmd *cobra.Command, rebuild bool) error {
	p := ux.NewPrinter(cmd.OutOrStdout())

	projectID, root, err := resolveVaultTarget()
	if err != nil {
		return err
	}
	ldir, err := ledgerDir()
	if err != nil {
		return err
	}

	// Determine the watermark and existing chunk map to resume from.
	var fromOffset uint64
	var existingChunks map[string]string
	var resumeSynthesis bool
	if !rebuild {
		st, found, serr := projection.LoadFoldState(root)
		if serr != nil {
			return serr
		}
		if found {
			fromOffset = st.LastOffset
			existingChunks = st.Chunks
			resumeSynthesis = st.PendingSynthesis
		}
	}

	reader := projection.NewReader(ldir, projectID)
	read, err := reader.Read(fromOffset)
	if err != nil {
		return err
	}

	existing := map[string]string{}
	if !rebuild {
		existing, err = projection.LoadExisting(root)
		if err != nil {
			return err
		}
	}

	warn := func(msg string) { fmt.Fprintln(os.Stderr, "warning: "+msg) }
	synth, engineName, err := projection.NewSynthesizer(vaultEngine, resolveFoldModel(), warn)
	if err != nil {
		return err
	}

	chunkSize := vaultChunk
	if chunkSize <= 0 {
		chunkSize = 60
	}

	// Count pending subjects for the resume banner.
	var spinnerMsg string
	switch {
	case resumeSynthesis && len(read.Events) == 0:
		// Count L0 episodes and pending subjects (those without a current concept file).
		l0Count := 0
		for p := range existing {
			if len(p) > len("episodes/") && p[:len("episodes/")] == "episodes/" {
				l0Count++
			}
		}
		spinnerMsg = fmt.Sprintf("resuming interrupted synthesis (%d episodes) with %s", l0Count, engineName)
	case resumeSynthesis:
		spinnerMsg = fmt.Sprintf("resuming interrupted synthesis + %d new events with %s", len(read.Events), engineName)
	default:
		spinnerMsg = fmt.Sprintf("folding %d events with %s", len(read.Events), engineName)
	}

	spinner := ux.NewSpinner(cmd.OutOrStdout())
	spinner.Start(spinnerMsg)

	writer := projection.NewWriter(root)
	in := projection.FoldInput{
		ProjectID:       projectID,
		ProjectRoot:     root,
		FromOffset:      fromOffset,
		ToOffset:        read.Head,
		Existing:        existing,
		Events:          read.Events,
		ExistingChunks:  existingChunks,
		Engine:          engineName,
		Progress:        spinner.Update,
		ResumeSynthesis: resumeSynthesis,
		Parallel:        vaultParallel,
		// Persist every completed synthesis step immediately: a Ctrl-C or
		// crash mid-fold loses at most one call's work, and the next fold
		// resumes from the checkpointed watermark + chunk cache.
		Checkpoint: func(changed, chunks map[string]string, watermark uint64) {
			if cerr := writer.Checkpoint(changed, chunks, watermark); cerr != nil {
				warn(fmt.Sprintf("checkpoint failed (%v); continuing — progress since the last good checkpoint is at risk", cerr))
			}
		},
	}
	chunks := (len(read.Events) + chunkSize - 1) / chunkSize
	if chunks == 0 {
		chunks = 1 // single refresh pass even with no new events
	}

	res, err := projection.Fold(context.Background(), synth, in, chunkSize)
	if err != nil {
		spinner.StopFail()
		return fmt.Errorf("synthesis failed: %w", err)
	}
	spinner.Stop()

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
	prune := rebuild && foldedThrough >= read.Head && !res.PendingSynthesis

	wres, err := writer.Write(res, foldedThrough, len(read.Events), prune)
	if err != nil {
		return err
	}

	verb := "fold"
	if rebuild {
		verb = "rebuild"
	}
	p.Diamond("vault " + verb)
	p.Blank()
	p.Chevron(fmt.Sprintf("project:       %s", p.HighlightValue(projectID)))
	p.Chevron(fmt.Sprintf("offsets:       %d → %d", fromOffset, foldedThrough))
	p.Chevron(fmt.Sprintf("events folded: %d", len(read.Events)))
	p.Chevron(fmt.Sprintf("chunks:        %d (size %d)", chunks, chunkSize))
	p.Chevron(fmt.Sprintf("files written: %d", wres.FilesWritten))
	p.Chevron(fmt.Sprintf("engine:        %s", engineName))
	p.Chevron(fmt.Sprintf("vault:         %s", wres.VaultDir))
	p.Blank()
	if res.PendingSynthesis {
		p.Chevron("pending: L1/L2 synthesis was interrupted — run `mom vault fold` when the harness recovers to complete it")
		p.Blank()
	} else if foldedThrough < read.Head {
		p.Chevron(fmt.Sprintf("partial: synthesis stopped at offset %d of %d — run `mom vault fold` to retry the rest", foldedThrough, read.Head))
		p.Blank()
	}
	if rebuild && !prune {
		p.Chevron("note: rebuild incomplete — existing vault files were KEPT (stale ones are pruned only by a fully completed rebuild)")
		p.Blank()
	}
	p.Checkf("vault written; CLAUDE.md block updated at %s", p.HighlightValue(wres.ClaudePath))
	return nil
}

// resolveFoldModel picks the fold synthesis model: the --model flag wins,
// then the vault.fold_model key in ~/.mom/config.yaml. Empty lets the
// projection factory apply the engine's cheap default (claude: haiku).
func resolveFoldModel() string {
	if vaultFoldModel != "" {
		return vaultFoldModel
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	cfg, err := config.Load(filepath.Join(home, ".mom"))
	if err != nil {
		return ""
	}
	return cfg.Vault.FoldModel
}

func runVaultStatus(cmd *cobra.Command, _ []string) error {
	p := ux.NewPrinter(cmd.OutOrStdout())

	projectID, root, err := resolveVaultTarget()
	if err != nil {
		return err
	}
	ldir, err := ledgerDir()
	if err != nil {
		return err
	}

	st, found, err := projection.LoadFoldState(root)
	if err != nil {
		return err
	}

	var fromOffset uint64
	if found {
		fromOffset = st.LastOffset
	}
	reader := projection.NewReader(ldir, projectID)
	pending, head, err := reader.PendingCount(fromOffset)
	if err != nil {
		return err
	}

	// Count vault files currently on disk.
	files, err := projection.LoadExisting(root)
	if err != nil {
		return err
	}

	p.Diamond("vault status")
	p.Blank()
	p.Chevron(fmt.Sprintf("project:       %s", p.HighlightValue(projectID)))
	p.Chevron(fmt.Sprintf("vault dir:     %s", projection.VaultDir(root)))
	if found {
		p.Chevron(fmt.Sprintf("watermark:     offset %d", st.LastOffset))
		p.Chevron(fmt.Sprintf("last fold:     %s", st.FoldedAt.Format("2006-01-02 15:04:05 MST")))
		p.Chevron(fmt.Sprintf("files on disk: %d", len(files)))
	} else {
		p.Chevron("watermark:     (never folded)")
		p.Chevron("files on disk: 0")
	}
	p.Chevron(fmt.Sprintf("ledger head:   offset %d", head))
	p.Chevron(fmt.Sprintf("pending:       %d new foldable events", pending))
	return nil
}
