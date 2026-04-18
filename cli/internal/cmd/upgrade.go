package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	huhspinner "charm.land/huh/v2/spinner"
	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/adapters/runtime"
	"github.com/vmarinogg/leo-core/cli/internal/config"
	"github.com/vmarinogg/leo-core/cli/internal/profiles"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade .leo/ to the latest version (preserves your KB docs)",
	Long: `Upgrades core infrastructure (schema, constraints, skills, profiles, runtime files)
to match the installed leo binary. Your documents in .leo/kb/docs/ are never touched.`,
	RunE: runUpgrade,
}

func init() {
	upgradeCmd.Flags().Bool("dry-run", false, "Show what would change without modifying anything")
}

// upgradeAction tracks a single change for reporting.
type upgradeAction struct {
	symbol string // ✔, ⚠, +
	desc   string
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	leoDir, err := findLeoDir()
	if err != nil {
		return err
	}

	projectRoot := filepath.Dir(leoDir)
	showSpinner := isTerminalWriter(cmd.OutOrStdout())

	var actions []upgradeAction
	addAction := func(symbol, desc string) {
		actions = append(actions, upgradeAction{symbol, desc})
	}

	// ── Phase 1: Load and migrate config ────────────────────────────────────
	cfg, err := config.Load(leoDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	var phase1Err error
	doPhase1 := func() {
		// Persist migrated config (Load auto-migrates v0.6.0 format).
		if err := config.Save(leoDir, cfg); err != nil {
			phase1Err = fmt.Errorf("saving config: %w", err)
			return
		}
		addAction("✔", "config.yaml migrated to latest format")

		// Create missing directories.
		newDirs := []string{
			filepath.Join(leoDir, "kb", "logs"),
			filepath.Join(leoDir, "cache"),
		}
		for _, d := range newDirs {
			if _, err := os.Stat(d); os.IsNotExist(err) {
				if !dryRun {
					if err := os.MkdirAll(d, 0755); err != nil {
						phase1Err = fmt.Errorf("creating %s: %w", d, err)
						return
					}
				}
				rel, _ := filepath.Rel(projectRoot, d)
				addAction("+", fmt.Sprintf("created directory %s", rel))
			}
		}

		if showSpinner {
			time.Sleep(500 * time.Millisecond)
		}
	}

	if showSpinner {
		_ = huhspinner.New().Title("Checking configuration...").Action(doPhase1).Run()
	} else {
		doPhase1()
	}
	if phase1Err != nil {
		return phase1Err
	}

	// ── Phase 2: Update core KB docs ────────────────────────────────────────
	var phase2Err error
	doPhase2 := func() {
		// Update schema.json.
		schemaData, err := embeddedSchema.ReadFile("schema.json")
		if err != nil {
			phase2Err = fmt.Errorf("reading embedded schema: %w", err)
			return
		}
		schemaPath := filepath.Join(leoDir, "kb", "schema.json")
		if changed := fileChanged(schemaPath, schemaData); changed {
			if !dryRun {
				if err := os.WriteFile(schemaPath, schemaData, 0644); err != nil {
					phase2Err = fmt.Errorf("writing schema: %w", err)
					return
				}
			}
			addAction("✔", "schema.json updated")
		}

		// Update identity.json.
		identityPath := filepath.Join(leoDir, "identity.json")
		identityBytes := []byte(defaultIdentity())
		if changed := fileChanged(identityPath, identityBytes); changed {
			if !dryRun {
				if err := os.WriteFile(identityPath, identityBytes, 0644); err != nil {
					phase2Err = fmt.Errorf("writing identity.json: %w", err)
					return
				}
			}
			addAction("✔", "identity.json updated")
		}

		// Update core constraints.
		constraintsDir := filepath.Join(leoDir, "kb", "constraints")
		for name, content := range coreConstraints() {
			path := filepath.Join(constraintsDir, name+".json")
			if changed := fileChanged(path, []byte(content)); changed {
				if !dryRun {
					if err := os.WriteFile(path, []byte(content), 0644); err != nil {
						phase2Err = fmt.Errorf("writing constraint %s: %w", name, err)
						return
					}
				}
				addAction("✔", fmt.Sprintf("constraint %s updated", name))
			}
		}

		// Update core skills.
		skillsDir := filepath.Join(leoDir, "kb", "skills")
		for name, content := range coreSkills() {
			path := filepath.Join(skillsDir, name+".json")
			if changed := fileChanged(path, []byte(content)); changed {
				if !dryRun {
					if err := os.WriteFile(path, []byte(content), 0644); err != nil {
						phase2Err = fmt.Errorf("writing skill %s: %w", name, err)
						return
					}
				}
				addAction("✔", fmt.Sprintf("skill %s updated", name))
			}
		}

		// Add new profiles (don't overwrite existing customizations).
		profilesDir := filepath.Join(leoDir, "profiles")
		for name, p := range profiles.DefaultProfiles() {
			path := filepath.Join(profilesDir, name+".yaml")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if !dryRun {
					if err := profiles.Save(profilesDir, name, p); err != nil {
						phase2Err = fmt.Errorf("writing profile %s: %w", name, err)
						return
					}
				}
				addAction("+", fmt.Sprintf("profile %s added", name))
			}
		}

		// Migrate metric → session-log docs.
		docsDir := filepath.Join(leoDir, "kb", "docs")
		migrated := migrateMetricDocs(docsDir, dryRun)
		for _, docID := range migrated {
			addAction("✔", fmt.Sprintf("doc %s migrated metric → session-log", docID))
		}

		if showSpinner {
			time.Sleep(700 * time.Millisecond)
		}
	}

	if showSpinner {
		_ = huhspinner.New().Title("Updating knowledge base...").Action(doPhase2).Run()
	} else {
		doPhase2()
	}
	if phase2Err != nil {
		return phase2Err
	}

	// ── Phase 3: Rebuild index and regenerate runtime files ─────────────────
	var phase3Err error
	doPhase3 := func() {
		// Rebuild index.
		indexPath := filepath.Join(leoDir, "kb", "index.json")
		if !dryRun {
			indexData, err := buildCoreIndex()
			if err != nil {
				phase3Err = fmt.Errorf("building index: %w", err)
				return
			}
			if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
				phase3Err = fmt.Errorf("writing index: %w", err)
				return
			}
		}
		addAction("✔", "index.json rebuilt")

		// Regenerate runtime context files.
		if !dryRun {
			if err := regenerateRuntimeFiles(projectRoot, leoDir, cfg); err != nil {
				phase3Err = err
				return
			}
		}
		for _, rt := range cfg.EnabledRuntimes() {
			addAction("✔", fmt.Sprintf("runtime %s context file regenerated", rt))
		}

		if showSpinner {
			time.Sleep(500 * time.Millisecond)
		}
	}

	if showSpinner {
		_ = huhspinner.New().Title("Regenerating runtime files...").Action(doPhase3).Run()
	} else {
		doPhase3()
	}
	if phase3Err != nil {
		return phase3Err
	}

	// ── Report ──────────────────────────────────────────────────────────────
	cmd.Println()
	if dryRun {
		cmd.Println("  Dry run — no changes made. Would apply:")
	} else {
		cmd.Println("  Upgrade complete:")
	}
	cmd.Println()
	for _, a := range actions {
		cmd.Printf("  %s %s\n", a.symbol, a.desc)
	}
	if len(actions) == 0 {
		cmd.Println("  Everything is already up to date.")
	}
	cmd.Println()
	if !dryRun {
		cmd.Println("L.E.O. is up to date. Run 'leo doctor' to verify health.")
	}
	return nil
}

// fileChanged returns true if the file at path doesn't exist or differs from data.
func fileChanged(path string, data []byte) bool {
	existing, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	return string(existing) != string(data)
}

// migrateMetricDocs finds docs with type "metric" and migrates them to "session-log".
func migrateMetricDocs(docsDir string, dryRun bool) []string {
	var migrated []string

	entries, err := os.ReadDir(docsDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(docsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var doc map[string]interface{}
		if err := json.Unmarshal(data, &doc); err != nil {
			continue
		}

		docType, ok := doc["type"].(string)
		if !ok || docType != "metric" {
			continue
		}

		docID, _ := doc["id"].(string)
		if !dryRun {
			doc["type"] = "session-log"
			updated, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				continue
			}
			os.WriteFile(path, append(updated, '\n'), 0644) //nolint:errcheck
		}
		migrated = append(migrated, docID)
	}

	return migrated
}

// regenerateRuntimeFiles rebuilds all runtime context files from the current config.
func regenerateRuntimeFiles(projectRoot, leoDir string, cfg *config.Config) error {
	registry := runtime.NewRegistry(projectRoot)

	runtimeCfg := runtime.Config{
		Version: cfg.Version,
		User: runtime.UserConfig{
			Language:       cfg.User.Language,
			Mode:           cfg.User.Mode,
			Autonomy:       cfg.User.Autonomy,
			DefaultProfile: cfg.User.DefaultProfile,
		},
	}

	defaultProfile := profiles.DefaultProfiles()[cfg.User.DefaultProfile]
	if defaultProfile == nil {
		defaultProfile = profiles.DefaultProfiles()["general-manager"]
	}
	runtimeProfile := runtime.Profile{
		Name:             defaultProfile.Name,
		Description:      defaultProfile.Description,
		Focus:            defaultProfile.Focus,
		Tone:             defaultProfile.Tone,
		ContextInjection: defaultProfile.ContextInjection,
	}

	var runtimeConstraints []runtime.Constraint
	for id := range coreConstraints() {
		var doc struct {
			Summary string `json:"summary"`
		}
		json.Unmarshal([]byte(coreConstraints()[id]), &doc) //nolint:errcheck
		runtimeConstraints = append(runtimeConstraints, runtime.Constraint{
			ID:      id,
			Summary: doc.Summary,
		})
	}
	sort.Slice(runtimeConstraints, func(i, j int) bool {
		return runtimeConstraints[i].ID < runtimeConstraints[j].ID
	})

	var runtimeSkills []runtime.Skill
	for id := range coreSkills() {
		var doc struct {
			Summary string `json:"summary"`
		}
		json.Unmarshal([]byte(coreSkills()[id]), &doc) //nolint:errcheck
		runtimeSkills = append(runtimeSkills, runtime.Skill{
			ID:      id,
			Summary: doc.Summary,
		})
	}
	sort.Slice(runtimeSkills, func(i, j int) bool {
		return runtimeSkills[i].ID < runtimeSkills[j].ID
	})

	var identityData struct {
		What        string   `json:"what"`
		Philosophy  string   `json:"philosophy"`
		Constraints []string `json:"constraints"`
	}
	json.Unmarshal([]byte(defaultIdentity()), &identityData) //nolint:errcheck
	runtimeIdentity := &runtime.Identity{
		What:        identityData.What,
		Philosophy:  identityData.Philosophy,
		Constraints: identityData.Constraints,
	}

	for _, rt := range cfg.EnabledRuntimes() {
		adapter, ok := registry.Get(rt)
		if !ok {
			continue
		}
		if err := adapter.GenerateContextFile(runtimeCfg, runtimeProfile, runtimeConstraints, runtimeSkills, runtimeIdentity); err != nil {
			return fmt.Errorf("generating %s context: %w", rt, err)
		}
	}

	return nil
}

