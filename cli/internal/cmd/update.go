package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/adapters/storage"
	"github.com/vmarinogg/leo-core/cli/internal/config"
	"github.com/vmarinogg/leo-core/cli/internal/kb"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Sync core rules and profiles from leo-core into this project",
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().String("source", "", "Path to the leo-core clone")
	updateCmd.Flags().Bool("dry-run", false, "Show what would change without applying")
	updateCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
}

// SyncPlan describes what the update command will do.
type SyncPlan struct {
	DocsToAdd     []SyncItem
	DocsToUpdate  []SyncItem
	DocsUnchanged []string
	SchemaChanged bool
}

// SyncItem represents a single document to be copied.
type SyncItem struct {
	ID         string
	SourcePath string
	TargetPath string
}

func runUpdate(cmd *cobra.Command, args []string) error {
	leoDir, err := findLeoDir()
	if err != nil {
		return err
	}

	cfg, err := config.Load(leoDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Resolve core source.
	source, _ := cmd.Flags().GetString("source")
	if source == "" {
		source = cfg.CoreSource
	}
	if source == "" {
		return fmt.Errorf("no core source configured — use --source or set core_source in .leo/config.yaml")
	}

	source = expandTilde(source)

	// Validate core source.
	kbDocsDir := filepath.Join(source, ".claude", "kb", "docs")
	if _, err := os.Stat(kbDocsDir); err != nil {
		return fmt.Errorf("not a valid leo-core: %s not found", kbDocsDir)
	}

	// Compute sync plan.
	plan, err := computeSyncPlan(source, leoDir)
	if err != nil {
		return fmt.Errorf("computing sync plan: %w", err)
	}

	// Display plan.
	displaySyncPlan(cmd, source, plan)

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dryRun {
		return nil
	}

	// Confirm unless --yes.
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		cmd.Print("\n  Apply changes? [Y/n]: ")
		buf := new(strings.Builder)
		fmt.Fscan(cmd.InOrStdin(), buf)
		answer := strings.TrimSpace(buf.String())
		if answer != "" && strings.ToLower(answer) != "y" {
			return fmt.Errorf("update aborted")
		}
	}

	// Apply plan.
	if err := applySyncPlan(plan, leoDir, source, cmd); err != nil {
		return err
	}

	// Summary.
	cmd.Printf("\n  Done: %d new, %d updated, %d unchanged\n",
		len(plan.DocsToAdd), len(plan.DocsToUpdate), len(plan.DocsUnchanged))

	return nil
}

// computeSyncPlan compares core docs against the project and produces a plan.
func computeSyncPlan(coreSource, leoDir string) (*SyncPlan, error) {
	coreDocsDir := filepath.Join(coreSource, ".claude", "kb", "docs")
	projDocsDir := filepath.Join(leoDir, "kb", "docs")

	entries, err := os.ReadDir(coreDocsDir)
	if err != nil {
		return nil, fmt.Errorf("reading core docs dir: %w", err)
	}

	plan := &SyncPlan{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		srcPath := filepath.Join(coreDocsDir, e.Name())
		coreDoc, err := kb.LoadDoc(srcPath)
		if err != nil {
			// Skip unparseable docs.
			continue
		}

		// Only sync core-scoped docs.
		if coreDoc.Scope != "core" {
			continue
		}

		tgtPath := filepath.Join(projDocsDir, e.Name())

		localDoc, err := kb.LoadDoc(tgtPath)
		if err != nil {
			// Local doesn't exist — add it.
			plan.DocsToAdd = append(plan.DocsToAdd, SyncItem{
				ID:         coreDoc.ID,
				SourcePath: srcPath,
				TargetPath: tgtPath,
			})
			continue
		}

		// Compare updated timestamps.
		if coreDoc.Updated.After(localDoc.Updated) {
			plan.DocsToUpdate = append(plan.DocsToUpdate, SyncItem{
				ID:         coreDoc.ID,
				SourcePath: srcPath,
				TargetPath: tgtPath,
			})
		} else {
			plan.DocsUnchanged = append(plan.DocsUnchanged, coreDoc.ID)
		}
	}

	// Compare schema files.
	coreSchema := filepath.Join(coreSource, ".claude", "kb", "schema.json")
	projSchema := filepath.Join(leoDir, "kb", "schema.json")
	plan.SchemaChanged = !filesEqual(coreSchema, projSchema)

	return plan, nil
}

// applySyncPlan copies files according to the plan and rebuilds the index.
func applySyncPlan(plan *SyncPlan, leoDir, coreSource string, cmd *cobra.Command) error {
	projDocsDir := filepath.Join(leoDir, "kb", "docs")
	if err := os.MkdirAll(projDocsDir, 0755); err != nil {
		return fmt.Errorf("ensuring docs dir exists: %w", err)
	}

	for _, item := range plan.DocsToAdd {
		if err := copyFileContents(item.SourcePath, item.TargetPath); err != nil {
			return fmt.Errorf("copying %s: %w", item.ID, err)
		}
	}

	for _, item := range plan.DocsToUpdate {
		if err := copyFileContents(item.SourcePath, item.TargetPath); err != nil {
			return fmt.Errorf("updating %s: %w", item.ID, err)
		}
	}

	if plan.SchemaChanged {
		coreSchema := filepath.Join(coreSource, ".claude", "kb", "schema.json")
		projSchema := filepath.Join(leoDir, "kb", "schema.json")
		if err := copyFileContents(coreSchema, projSchema); err != nil {
			return fmt.Errorf("updating schema: %w", err)
		}
	}

	// Rebuild index.
	adapter := storage.NewJSONAdapter(leoDir)
	if err := adapter.Reindex(); err != nil {
		cmd.Printf("  warning: index rebuild failed: %v\n", err)
	}

	return nil
}

// displaySyncPlan prints the sync plan to the command output.
func displaySyncPlan(cmd *cobra.Command, source string, plan *SyncPlan) {
	cmd.Printf("\n  Source: %s\n\n", source)
	cmd.Println("  KB docs:")

	for _, item := range plan.DocsToAdd {
		cmd.Printf("    + %-30s (new)\n", item.ID)
	}
	for _, item := range plan.DocsToUpdate {
		cmd.Printf("    ~ %-30s (updated)\n", item.ID)
	}
	for _, id := range plan.DocsUnchanged {
		cmd.Printf("    = %-30s (unchanged)\n", id)
	}

	schemaStatus := "unchanged"
	if plan.SchemaChanged {
		schemaStatus = "updated"
	}
	cmd.Printf("\n  Schema: %s\n", schemaStatus)
	cmd.Printf("\n  Summary: %d new, %d updated, %d unchanged\n",
		len(plan.DocsToAdd), len(plan.DocsToUpdate), len(plan.DocsUnchanged))
}

// filesEqual returns true if both files exist and have identical contents.
func filesEqual(a, b string) bool {
	dataA, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	dataB, err := os.ReadFile(b)
	if err != nil {
		return false
	}
	return bytes.Equal(dataA, dataB)
}
