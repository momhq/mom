package cmd

import (
	"bufio"
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
	Short: "Sync core rules from mom-core into this project",
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

	ConstraintsToAdd     []SyncItem
	ConstraintsToUpdate  []SyncItem
	ConstraintsUnchanged []string

	SkillsToAdd     []SyncItem
	SkillsToUpdate  []SyncItem
	SkillsUnchanged []string

	IdentityChanged bool

	ProfilesToAdd     []SyncItem
	ProfileConflicts  []SyncItem // exist locally, content differs from core
	ProfilesUnchanged []string
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

	// Validate core source — accept both old (kb/docs) and new (memory) layouts.
	kbDocsDir := filepath.Join(source, ".leo", "memory")
	if _, err := os.Stat(kbDocsDir); err != nil {
		// Fall back to legacy layout for backward compatibility.
		kbDocsDir = filepath.Join(source, ".leo", "kb", "docs")
		if _, err := os.Stat(kbDocsDir); err != nil {
			return fmt.Errorf("not a valid leo-core: %s not found", kbDocsDir)
		}
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
	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !yes {
		cmd.Print("\n  Apply changes? [Y/n]: ")
		answer := ""
		if scanner.Scan() {
			answer = strings.TrimSpace(scanner.Text())
		}
		if answer != "" && strings.ToLower(answer) != "y" {
			return fmt.Errorf("update aborted")
		}
	}

	// Resolve profile conflicts.
	var profilesToReplace []SyncItem
	conflictsKept := 0
	if len(plan.ProfileConflicts) > 0 {
		if yes {
			// Safe default with --yes: keep all local.
			for _, item := range plan.ProfileConflicts {
				cmd.Printf("  Conflict: %s — keeping local (safe default with --yes)\n", item.ID)
			}
			conflictsKept = len(plan.ProfileConflicts)
		} else {
			// Interactive: prompt per conflict.
			for _, item := range plan.ProfileConflicts {
				cmd.Printf("  Profile %q differs from core. Replace with core version? [y/N]: ", item.ID)
				answer := ""
				if scanner.Scan() {
					answer = strings.TrimSpace(scanner.Text())
				}
				if strings.ToLower(answer) == "y" {
					profilesToReplace = append(profilesToReplace, item)
				} else {
					conflictsKept++
				}
			}
		}
	}

	// Apply plan.
	if err := applySyncPlan(plan, leoDir, source, cmd, profilesToReplace); err != nil {
		return err
	}

	// Summary.
	totalNew := len(plan.DocsToAdd) + len(plan.ProfilesToAdd) + len(plan.ConstraintsToAdd) + len(plan.SkillsToAdd)
	totalUpdated := len(plan.DocsToUpdate) + len(plan.ConstraintsToUpdate) + len(plan.SkillsToUpdate)
	totalUnchanged := len(plan.DocsUnchanged) + len(plan.ProfilesUnchanged) + len(plan.ConstraintsUnchanged) + len(plan.SkillsUnchanged)
	totalConflicts := len(plan.ProfileConflicts)
	conflictsReplaced := len(profilesToReplace)
	cmd.Printf("\n  Done: %d new, %d updated, %d unchanged, %d conflicts (%d replaced, %d kept)\n",
		totalNew, totalUpdated, totalUnchanged, totalConflicts, conflictsReplaced, conflictsKept)

	return nil
}

// resolveCoreDocsDir returns the memory dir from coreSource, falling back to
// the legacy kb/docs layout for cores that have not yet been migrated.
func resolveCoreDocsDir(coreSource string) string {
	newPath := filepath.Join(coreSource, ".leo", "memory")
	if _, err := os.Stat(newPath); err == nil {
		return newPath
	}
	return filepath.Join(coreSource, ".leo", "kb", "docs")
}

// computeSyncPlan compares core docs against the project and produces a plan.
func computeSyncPlan(coreSource, leoDir string) (*SyncPlan, error) {
	coreDocsDir := resolveCoreDocsDir(coreSource)
	projDocsDir := filepath.Join(leoDir, "memory")

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

	// Compare constraints.
	coreConstraintsDir := filepath.Join(coreSource, ".leo", "constraints")
	if _, err := os.Stat(coreConstraintsDir); err != nil {
		coreConstraintsDir = filepath.Join(coreSource, ".leo", "kb", "constraints")
	}
	projConstraintsDir := filepath.Join(leoDir, "constraints")
	if constraintEntries, err := os.ReadDir(coreConstraintsDir); err == nil {
		for _, e := range constraintEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			srcPath := filepath.Join(coreConstraintsDir, e.Name())
			tgtPath := filepath.Join(projConstraintsDir, e.Name())
			name := strings.TrimSuffix(e.Name(), ".json")

			if filesEqual(srcPath, tgtPath) {
				plan.ConstraintsUnchanged = append(plan.ConstraintsUnchanged, name)
			} else if _, err := os.Stat(tgtPath); err != nil {
				plan.ConstraintsToAdd = append(plan.ConstraintsToAdd, SyncItem{
					ID: name, SourcePath: srcPath, TargetPath: tgtPath,
				})
			} else {
				plan.ConstraintsToUpdate = append(plan.ConstraintsToUpdate, SyncItem{
					ID: name, SourcePath: srcPath, TargetPath: tgtPath,
				})
			}
		}
	}

	// Compare skills.
	coreSkillsDir := filepath.Join(coreSource, ".leo", "skills")
	if _, err := os.Stat(coreSkillsDir); err != nil {
		coreSkillsDir = filepath.Join(coreSource, ".leo", "kb", "skills")
	}
	projSkillsDir := filepath.Join(leoDir, "skills")
	if skillEntries, err := os.ReadDir(coreSkillsDir); err == nil {
		for _, e := range skillEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			srcPath := filepath.Join(coreSkillsDir, e.Name())
			tgtPath := filepath.Join(projSkillsDir, e.Name())
			name := strings.TrimSuffix(e.Name(), ".json")

			if filesEqual(srcPath, tgtPath) {
				plan.SkillsUnchanged = append(plan.SkillsUnchanged, name)
			} else if _, err := os.Stat(tgtPath); err != nil {
				plan.SkillsToAdd = append(plan.SkillsToAdd, SyncItem{
					ID: name, SourcePath: srcPath, TargetPath: tgtPath,
				})
			} else {
				plan.SkillsToUpdate = append(plan.SkillsToUpdate, SyncItem{
					ID: name, SourcePath: srcPath, TargetPath: tgtPath,
				})
			}
		}
	}

	// Compare identity.json.
	coreIdentity := filepath.Join(coreSource, ".leo", "identity.json")
	projIdentity := filepath.Join(leoDir, "identity.json")
	plan.IdentityChanged = !filesEqual(coreIdentity, projIdentity)

	// Compare schema files.
	coreSchema := filepath.Join(coreSource, ".leo", "schema.json")
	if _, err := os.Stat(coreSchema); err != nil {
		coreSchema = filepath.Join(coreSource, ".leo", "kb", "schema.json")
	}
	projSchema := filepath.Join(leoDir, "schema.json")
	plan.SchemaChanged = !filesEqual(coreSchema, projSchema)

	// Compare profile files.
	coreProfilesDir := filepath.Join(coreSource, ".leo", "profiles")
	projProfilesDir := filepath.Join(leoDir, "profiles")

	profileEntries, err := os.ReadDir(coreProfilesDir)
	if err == nil {
		for _, e := range profileEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}

			srcPath := filepath.Join(coreProfilesDir, e.Name())
			tgtPath := filepath.Join(projProfilesDir, e.Name())
			name := strings.TrimSuffix(e.Name(), ".yaml")

			if filesEqual(srcPath, tgtPath) {
				plan.ProfilesUnchanged = append(plan.ProfilesUnchanged, name)
			} else if _, err := os.Stat(tgtPath); err != nil {
				// Local doesn't exist — add it.
				plan.ProfilesToAdd = append(plan.ProfilesToAdd, SyncItem{
					ID:         name,
					SourcePath: srcPath,
					TargetPath: tgtPath,
				})
			} else {
				// Exists but different — update it.
				plan.ProfileConflicts = append(plan.ProfileConflicts, SyncItem{
					ID:         name,
					SourcePath: srcPath,
					TargetPath: tgtPath,
				})
			}
		}
	}

	return plan, nil
}

// applySyncPlan copies files according to the plan and rebuilds the index.
// profilesToReplace contains only the conflict profiles the user chose to replace.
func applySyncPlan(plan *SyncPlan, leoDir, coreSource string, cmd *cobra.Command, profilesToReplace []SyncItem) error {
	projDocsDir := filepath.Join(leoDir, "memory")
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

	// Copy constraints.
	constraintsDir := filepath.Join(leoDir, "constraints")
	if len(plan.ConstraintsToAdd)+len(plan.ConstraintsToUpdate) > 0 {
		if err := os.MkdirAll(constraintsDir, 0755); err != nil {
			return fmt.Errorf("ensuring constraints dir exists: %w", err)
		}
		for _, item := range plan.ConstraintsToAdd {
			if err := copyFileContents(item.SourcePath, item.TargetPath); err != nil {
				return fmt.Errorf("copying constraint %s: %w", item.ID, err)
			}
		}
		for _, item := range plan.ConstraintsToUpdate {
			if err := copyFileContents(item.SourcePath, item.TargetPath); err != nil {
				return fmt.Errorf("updating constraint %s: %w", item.ID, err)
			}
		}
	}

	// Copy skills.
	skillsDir := filepath.Join(leoDir, "skills")
	if len(plan.SkillsToAdd)+len(plan.SkillsToUpdate) > 0 {
		if err := os.MkdirAll(skillsDir, 0755); err != nil {
			return fmt.Errorf("ensuring skills dir exists: %w", err)
		}
		for _, item := range plan.SkillsToAdd {
			if err := copyFileContents(item.SourcePath, item.TargetPath); err != nil {
				return fmt.Errorf("copying skill %s: %w", item.ID, err)
			}
		}
		for _, item := range plan.SkillsToUpdate {
			if err := copyFileContents(item.SourcePath, item.TargetPath); err != nil {
				return fmt.Errorf("updating skill %s: %w", item.ID, err)
			}
		}
	}

	// Copy identity.json.
	if plan.IdentityChanged {
		coreIdentity := filepath.Join(coreSource, ".leo", "identity.json")
		projIdentity := filepath.Join(leoDir, "identity.json")
		if err := copyFileContents(coreIdentity, projIdentity); err != nil {
			return fmt.Errorf("updating identity.json: %w", err)
		}
	}

	if plan.SchemaChanged {
		coreSchema := filepath.Join(coreSource, ".leo", "schema.json")
		if _, err := os.Stat(coreSchema); err != nil {
			coreSchema = filepath.Join(coreSource, ".leo", "kb", "schema.json")
		}
		projSchema := filepath.Join(leoDir, "schema.json")
		if err := copyFileContents(coreSchema, projSchema); err != nil {
			return fmt.Errorf("updating schema: %w", err)
		}
	}

	// Copy profiles.
	if len(plan.ProfilesToAdd)+len(profilesToReplace) > 0 {
		projProfilesDir := filepath.Join(leoDir, "profiles")
		if err := os.MkdirAll(projProfilesDir, 0755); err != nil {
			return fmt.Errorf("ensuring profiles dir exists: %w", err)
		}

		for _, item := range plan.ProfilesToAdd {
			if err := copyFileContents(item.SourcePath, item.TargetPath); err != nil {
				return fmt.Errorf("copying profile %s: %w", item.ID, err)
			}
		}

		for _, item := range profilesToReplace {
			if err := copyFileContents(item.SourcePath, item.TargetPath); err != nil {
				return fmt.Errorf("replacing profile %s: %w", item.ID, err)
			}
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

	cmd.Println("\n  Constraints:")
	for _, item := range plan.ConstraintsToAdd {
		cmd.Printf("    + %-30s (new)\n", item.ID)
	}
	for _, item := range plan.ConstraintsToUpdate {
		cmd.Printf("    ~ %-30s (updated)\n", item.ID)
	}
	for _, name := range plan.ConstraintsUnchanged {
		cmd.Printf("    = %-30s (unchanged)\n", name)
	}

	cmd.Println("\n  Skills:")
	for _, item := range plan.SkillsToAdd {
		cmd.Printf("    + %-30s (new)\n", item.ID)
	}
	for _, item := range plan.SkillsToUpdate {
		cmd.Printf("    ~ %-30s (updated)\n", item.ID)
	}
	for _, name := range plan.SkillsUnchanged {
		cmd.Printf("    = %-30s (unchanged)\n", name)
	}

	cmd.Println("\n  Profiles:")
	for _, item := range plan.ProfilesToAdd {
		cmd.Printf("    + %-30s (new)\n", item.ID)
	}
	for _, item := range plan.ProfileConflicts {
		cmd.Printf("    ? %-30s (conflict — local differs)\n", item.ID)
	}
	for _, name := range plan.ProfilesUnchanged {
		cmd.Printf("    = %-30s (unchanged)\n", name)
	}

	identityStatus := "unchanged"
	if plan.IdentityChanged {
		identityStatus = "updated"
	}
	cmd.Printf("\n  Identity: %s\n", identityStatus)

	schemaStatus := "unchanged"
	if plan.SchemaChanged {
		schemaStatus = "updated"
	}
	cmd.Printf("  Schema: %s\n", schemaStatus)

	totalNew := len(plan.DocsToAdd) + len(plan.ProfilesToAdd) + len(plan.ConstraintsToAdd) + len(plan.SkillsToAdd)
	totalUpdated := len(plan.DocsToUpdate) + len(plan.ConstraintsToUpdate) + len(plan.SkillsToUpdate)
	totalUnchanged := len(plan.DocsUnchanged) + len(plan.ProfilesUnchanged) + len(plan.ConstraintsUnchanged) + len(plan.SkillsUnchanged)
	totalConflicts := len(plan.ProfileConflicts)
	cmd.Printf("\n  Summary: %d new, %d updated, %d unchanged, %d conflicts\n",
		totalNew, totalUpdated, totalUnchanged, totalConflicts)
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
