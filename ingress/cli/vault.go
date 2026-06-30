package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/momhq/mom/services/projection"
	"github.com/momhq/mom/shared/project"
	"github.com/momhq/mom/shared/ux"
	"github.com/momhq/mom/storage/librarian"
	"github.com/spf13/cobra"
)

var (
	vaultProject string
	vaultEngine  string
	vaultRoot    string
	vaultChunk   int
	vaultModel   string
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

var vaultGardenCmd = &cobra.Command{
	Use:   "garden",
	Short: "Reorganize the existing vault (merge/dedupe/relink) without inventing memories",
	Args:  cobra.NoArgs,
	RunE:  runVaultGarden,
}

func init() {
	for _, c := range []*cobra.Command{vaultFoldCmd, vaultRebuildCmd, vaultStatusCmd, vaultGardenCmd} {
		c.Flags().StringVar(&vaultProject, "project", "", "Override the resolved project id")
		c.Flags().StringVar(&vaultRoot, "root", "", "Override the project root for output (default: resolved project root)")
	}
	for _, c := range []*cobra.Command{vaultFoldCmd, vaultRebuildCmd} {
		c.Flags().StringVar(&vaultEngine, "engine", "auto", "Synthesis engine: auto | claude | codex | pi")
		c.Flags().IntVar(&vaultChunk, "chunk", 60, "Events per synthesizer call when folding (iterative, full-history coverage)")
	}
	vaultGardenCmd.Flags().StringVar(&vaultModel, "model", "claude-sonnet-4-6", "Model used for the garden reorganization pass")
	vaultCmd.AddCommand(vaultFoldCmd)
	vaultCmd.AddCommand(vaultRebuildCmd)
	vaultCmd.AddCommand(vaultStatusCmd)
	vaultCmd.AddCommand(vaultGardenCmd)
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
	if !rebuild {
		st, found, serr := projection.LoadFoldState(root)
		if serr != nil {
			return serr
		}
		if found {
			fromOffset = st.LastOffset
			existingChunks = st.Chunks
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
	synth, engineName, err := projection.NewSynthesizer(vaultEngine, warn)
	if err != nil {
		return err
	}

	spinner := ux.NewSpinner(cmd.OutOrStdout())
	spinner.Start(fmt.Sprintf("folding %d events with %s", len(read.Events), engineName))

	in := projection.FoldInput{
		ProjectID:      projectID,
		ProjectRoot:    root,
		FromOffset:     fromOffset,
		ToOffset:       read.Head,
		Existing:       existing,
		Events:         read.Events,
		ExistingChunks: existingChunks,
		Engine:         engineName,
		Progress:       spinner.Update,
	}
	chunkSize := vaultChunk
	if chunkSize <= 0 {
		chunkSize = 60
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

	writer := projection.NewWriter(root)
	// On rebuild, prune stale files so the on-disk vault exactly matches the
	// freshly synthesized set (e.g. when the layout changes).
	wres, err := writer.Write(res, read.Head, len(read.Events), rebuild)
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
	p.Chevron(fmt.Sprintf("offsets:       %d → %d", fromOffset, read.Head))
	p.Chevron(fmt.Sprintf("events folded: %d", len(read.Events)))
	p.Chevron(fmt.Sprintf("chunks:        %d (size %d)", chunks, chunkSize))
	p.Chevron(fmt.Sprintf("files written: %d", wres.FilesWritten))
	p.Chevron(fmt.Sprintf("engine:        %s", engineName))
	p.Chevron(fmt.Sprintf("vault:         %s", wres.VaultDir))
	p.Blank()
	p.Checkf("vault written; CLAUDE.md block updated at %s", p.HighlightValue(wres.ClaudePath))
	return nil
}

func runVaultGarden(cmd *cobra.Command, _ []string) error {
	p := ux.NewPrinter(cmd.OutOrStdout())

	projectID, root, err := resolveVaultTarget()
	if err != nil {
		return err
	}

	warn := func(msg string) { fmt.Fprintln(os.Stderr, "warning: "+msg) }
	gres, err := projection.Garden(context.Background(), root, projectID, vaultModel, warn)
	if err != nil {
		// Garden leaves the vault untouched on any failure.
		return fmt.Errorf("garden failed; vault left unchanged: %w", err)
	}

	writer := projection.NewWriter(root)
	wres, err := writer.WritePruned(gres.Result)
	if err != nil {
		return err
	}

	removed := gres.FilesBefore - gres.FilesAfter

	p.Diamond("vault garden")
	p.Blank()
	p.Chevron(fmt.Sprintf("project:       %s", p.HighlightValue(projectID)))
	p.Chevron(fmt.Sprintf("files before:  %d", gres.FilesBefore))
	p.Chevron(fmt.Sprintf("files after:   %d", gres.FilesAfter))
	p.Chevron(fmt.Sprintf("net change:    %d files merged/removed", removed))
	p.Chevron(fmt.Sprintf("files written: %d", wres.FilesWritten))
	p.Chevron(fmt.Sprintf("model:         %s", gres.Model))
	p.Chevron(fmt.Sprintf("vault:         %s", wres.VaultDir))
	p.Blank()
	p.Checkf("vault gardened; CLAUDE.md block updated at %s", p.HighlightValue(wres.ClaudePath))
	return nil
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
