package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/momhq/mom/cli/internal/adapters/storage"
	"github.com/momhq/mom/cli/internal/memory"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/momhq/mom/cli/internal/ux"
)

var promoteCmd = &cobra.Command{
	Use:   "promote <memory-id>",
	Short: "Move a memory doc up to a broader scope",
	Long: `Moves a memory document from the nearest .mom/ to the nearest
ancestor .mom/ that has the specified scope label.

The file is removed from the source scope and written to the target scope.
Provenance is updated to record the promotion (promoted_from tag added).

Symlinks are not followed during walk-up discovery.`,
	Args: cobra.ExactArgs(1),
	RunE: runPromote,
}

var demoteCmd = &cobra.Command{
	Use:   "demote <memory-id>",
	Short: "Move a memory doc down to the nearest (repo) scope",
	Long: `Moves a memory document from an ancestor scope down to the
nearest .mom/ (most specific scope).

The file is removed from the source scope and written to the target scope.
Provenance is updated to record the demotion (demoted_from tag added).

Symlinks are not followed during walk-up discovery.`,
	Args: cobra.ExactArgs(1),
	RunE: runDemote,
}

func init() {
	promoteCmd.Flags().String("to", "", "Target scope label (user, org, workspace, custom)")
	_ = promoteCmd.MarkFlagRequired("to")

	demoteCmd.Flags().String("to", "", "Target scope label (repo, or any closer scope)")
	_ = demoteCmd.MarkFlagRequired("to")
}

// runPromote implements `leo promote <id> --to <scope>`.
func runPromote(cmd *cobra.Command, args []string) error {
	id := args[0]
	toLabel, _ := cmd.Flags().GetString("to")

	if err := scope.ValidateLabel(toLabel); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	scopes := scope.Walk(cwd)
	if len(scopes) == 0 {
		return fmt.Errorf("no .mom/ found — run 'mom init' first")
	}

	// Source: nearest scope.
	src := scopes[0]
	srcDocPath := filepath.Join(src.Path, "memory", id+".json")

	if _, err := os.Stat(srcDocPath); os.IsNotExist(err) {
		return fmt.Errorf("memory %q not found in nearest scope (%s)", id, src.Path)
	}

	// Target: nearest ancestor with the requested label (skip src itself).
	var dst scope.Scope
	found := false
	for _, s := range scopes[1:] {
		if s.Label == toLabel {
			dst = s
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no ancestor scope with label %q found — available scopes: %s",
			toLabel, scopeList(scopes[1:]))
	}

	return moveDoc(srcDocPath, src.Path, dst.Path, id, "promoted-from-"+src.Label, cmd)
}

// runDemote implements `leo demote <id> --to <scope>`.
func runDemote(cmd *cobra.Command, args []string) error {
	id := args[0]
	toLabel, _ := cmd.Flags().GetString("to")

	if err := scope.ValidateLabel(toLabel); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	scopes := scope.Walk(cwd)
	if len(scopes) == 0 {
		return fmt.Errorf("no .mom/ found — run 'mom init' first")
	}

	// Target: nearest scope with the requested label.
	var dst scope.Scope
	dstIdx := -1
	for i, s := range scopes {
		if s.Label == toLabel {
			dst = s
			dstIdx = i
			break
		}
	}
	if dstIdx < 0 {
		return fmt.Errorf("no scope with label %q found — available scopes: %s",
			toLabel, scopeList(scopes))
	}

	// Source: nearest ancestor ABOVE the target that has the doc.
	var src scope.Scope
	found := false
	for _, s := range scopes[dstIdx+1:] {
		candidate := filepath.Join(s.Path, "memory", id+".json")
		if _, err := os.Stat(candidate); err == nil {
			src = s
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("memory %q not found in any scope above %q", id, toLabel)
	}

	srcDocPath := filepath.Join(src.Path, "memory", id+".json")
	return moveDoc(srcDocPath, src.Path, dst.Path, id, "demoted-from-"+src.Label, cmd)
}

// moveDoc reads a doc from srcPath, rewrites provenance, writes to dstLeoDir
// via IndexedAdapter (write-through to SQLite index), then removes the original.
// The provenanceTag is added to the doc's tags.
func moveDoc(srcDocPath, srcLeoDir, dstLeoDir, id, provenanceTag string, cmd *cobra.Command) error {
	doc, err := memory.LoadDoc(srcDocPath)
	if err != nil {
		return fmt.Errorf("reading doc: %w", err)
	}

	// Add provenance tag (idempotent — skip if already present).
	hasTag := false
	for _, t := range doc.Tags {
		if t == provenanceTag {
			hasTag = true
			break
		}
	}
	if !hasTag {
		doc.Tags = append(doc.Tags, provenanceTag)
	}

	// Ensure destination memory dir exists.
	dstMemDir := filepath.Join(dstLeoDir, "memory")
	if err := os.MkdirAll(dstMemDir, 0755); err != nil {
		return fmt.Errorf("creating destination memory dir: %w", err)
	}

	// Write to destination via IndexedAdapter (syncs JSON + SQLite).
	dstIdx := storage.NewIndexedAdapter(dstLeoDir)
	defer dstIdx.Close()

	storageDoc := &storage.Doc{
		ID:             doc.ID,
		Scope:          doc.Scope,
		Tags:           doc.Tags,
		Created:        doc.Created,
		CreatedBy:      doc.CreatedBy,
		SessionID:      doc.SessionID,
		PromotionState: doc.PromotionState,
		Classification: doc.Classification,
		Compartments:   doc.Compartments,
		Provenance:     doc.Provenance,
		Landmark:       doc.Landmark,
		CentralityScore: doc.CentralityScore,
		Content:        doc.Content,
	}
	if err := dstIdx.Write(storageDoc); err != nil {
		return fmt.Errorf("writing doc to destination: %w", err)
	}

	// Remove from source (JSON + SQLite index).
	srcIdx := storage.NewIndexedAdapter(srcLeoDir)
	defer srcIdx.Close()
	if err := srcIdx.Delete(id); err != nil {
		return fmt.Errorf("removing source doc: %w", err)
	}

	mp := ux.NewPrinter(cmd.OutOrStdout())
	mp.Checkf("%s moved: %s → %s", id,
		shortenPath(srcLeoDir), shortenPath(dstLeoDir))
	return nil
}

// scopeList formats a human-readable list of scope labels and paths.
func scopeList(scopes []scope.Scope) string {
	if len(scopes) == 0 {
		return "(none)"
	}
	out := ""
	for i, s := range scopes {
		if i > 0 {
			out += ", "
		}
		out += s.Label + "(" + shortenPath(s.Path) + ")"
	}
	return out
}

// shortenPath replaces the home directory prefix with "~".
