package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

var vaultLintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Walk-test the vault: reachability, no payload in routers",
	Args:  cobra.NoArgs,
	RunE:  runVaultLint,
}

func init() {
	for _, c := range []*cobra.Command{vaultFoldCmd, vaultRebuildCmd, vaultStatusCmd, vaultLintCmd} {
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
	vaultCmd.AddCommand(vaultLintCmd)
}

// runVaultLint walk-tests the resolved project's vault and reports every
// finding. Exits non-zero when any finding is present — a lint failure is a
// CI-catchable signal, not just an FYI.
func runVaultLint(cmd *cobra.Command, _ []string) error {
	p := ux.NewPrinter(cmd.OutOrStdout())

	projectID, root, err := resolveVaultTarget()
	if err != nil {
		return err
	}

	findings, err := projection.LintVault(root)
	if err != nil {
		return err
	}

	p.Diamond("vault lint")
	p.Blank()
	p.Chevron(fmt.Sprintf("project: %s", p.HighlightValue(projectID)))
	if len(findings) == 0 {
		p.Blank()
		p.Checkf("no findings")
		return nil
	}
	p.Blank()
	for _, f := range findings {
		p.Chevron(fmt.Sprintf("[%s] %s — %s", f.Rule, f.Path, f.Detail))
	}
	return fmt.Errorf("vault lint: %d finding(s)", len(findings))
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

	spinner := ux.NewSpinner(cmd.OutOrStdout())
	spinner.Start("folding with " + vaultEngine)

	// The fold itself is shared with the daemon's auto-fold
	// (projection.RunProjectFold): same watermark/resume semantics, same
	// per-project fold lock, same checkpointing.
	sum, err := projection.RunProjectFold(context.Background(), projection.RunOptions{
		ProjectID:  projectID,
		Root:       root,
		LedgerDir:  ldir,
		Rebuild:    rebuild,
		Engine:     vaultEngine,
		Model:      resolveFoldModel(),
		ChunkSize:  vaultChunk,
		Parallel:   vaultParallel,
		EntryFiles: resolveEntryFiles(),
		Progress:   spinner.Update,
		Warn:       func(msg string) { fmt.Fprintln(os.Stderr, "warning: "+msg) },
	})
	if err != nil {
		spinner.StopFail()
		if errors.Is(err, projection.ErrFoldLocked) {
			return fmt.Errorf("another fold is already running for this project (daemon auto-fold or another `mom vault fold`) — try again when it finishes")
		}
		return err
	}
	spinner.Stop()

	verb := "fold"
	if rebuild {
		verb = "rebuild"
	}
	p.Diamond("vault " + verb)
	p.Blank()
	p.Chevron(fmt.Sprintf("project:       %s", p.HighlightValue(projectID)))
	p.Chevron(fmt.Sprintf("offsets:       %d → %d", sum.FromOffset, sum.FoldedThrough))
	p.Chevron(fmt.Sprintf("events folded: %d", sum.EventsFolded))
	p.Chevron(fmt.Sprintf("chunks:        %d (size %d)", sum.ChunksTotal, vaultChunk))
	p.Chevron(fmt.Sprintf("files written: %d", sum.FilesWritten))
	p.Chevron(fmt.Sprintf("engine:        %s", sum.EngineName))
	p.Chevron(fmt.Sprintf("vault:         %s", sum.VaultDir))
	p.Blank()
	if sum.PendingSynthesis {
		p.Chevron("pending: L1/L2 synthesis was interrupted — run `mom vault fold` when the harness recovers to complete it")
		p.Blank()
	} else if sum.Partial {
		p.Chevron(fmt.Sprintf("partial: synthesis stopped at offset %d of %d — run `mom vault fold` to retry the rest", sum.FoldedThrough, sum.Head))
		p.Blank()
	}
	if rebuild && !sum.Pruned {
		p.Chevron("note: rebuild incomplete — existing vault files were KEPT (stale ones are pruned only by a fully completed rebuild)")
		p.Blank()
	}
	p.Checkf("vault written; context block updated at %s", p.HighlightValue(strings.Join(sum.EntryPaths, ", ")))
	return nil
}

// resolveEntryFiles maps the enabled harnesses to the entry files that carry
// the managed context block. Falls back to CLAUDE.md when config is
// unavailable.
func resolveEntryFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	cfg, err := config.Load(filepath.Join(home, ".mom"))
	if err != nil {
		return nil
	}
	return cfg.EntryFiles()
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
