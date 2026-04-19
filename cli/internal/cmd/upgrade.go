package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	huhspinner "charm.land/huh/v2/spinner"
	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/adapters/runtime"
	"github.com/vmarinogg/leo-core/cli/internal/config"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade .leo/ to the latest version (preserves your KB docs)",
	Long: `Upgrades core infrastructure (schema, constraints, skills, runtime files)
to match the installed leo binary. Your documents in .leo/memory/ are never touched.`,
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

	// ── Phase 0: Migrate legacy kb/ layout to flat layout ───────────────────
	if !dryRun {
		layoutActions, err := migrateKBLayout(leoDir)
		if err != nil {
			return fmt.Errorf("migrating layout: %w", err)
		}
		for _, a := range layoutActions {
			addAction(a.symbol, a.desc)
		}
	} else {
		if _, statErr := os.Stat(filepath.Join(leoDir, "kb")); statErr == nil {
			addAction("⚠", "legacy .leo/kb/ detected — would flatten to new layout (run without --dry-run)")
		}
	}

	// ── Phase 1: Load, migrate, and persist config ───────────────────────────
	cfg, err := config.Load(leoDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	var phase1Err error
	doPhase1 := func() {
		// Back-fill communication.mode if absent (pre-v0.8 installs).
		if cfg.Communication.Mode == "" {
			cfg.Communication.Mode = "concise"
			addAction("✔", "communication.mode set to concise (default)")
		}

		// Persist migrated config (Load auto-migrates v0.6.0/v0.7.0 formats,
		// dropping default_profile and back-filling communication.mode).
		if err := config.Save(leoDir, cfg); err != nil {
			phase1Err = fmt.Errorf("saving config: %w", err)
			return
		}
		addAction("✔", "config.yaml migrated to latest format")

		// Create missing directories. All canonical v0.8 dirs must exist before
		// Phase 2 writes core constraints/skills and the layout migration.
		newDirs := []string{
			filepath.Join(leoDir, "memory"),
			filepath.Join(leoDir, "constraints"),
			filepath.Join(leoDir, "skills"),
			filepath.Join(leoDir, "logs"),
			filepath.Join(leoDir, "telemetry"),
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

		// Remove retired .leo/profiles/ directory (v0.8.0 migration).
		profilesDir := filepath.Join(leoDir, "profiles")
		if _, statErr := os.Stat(profilesDir); statErr == nil {
			if !dryRun {
				if err := os.RemoveAll(profilesDir); err != nil {
					phase1Err = fmt.Errorf("removing profiles/: %w", err)
					return
				}
			}
			addAction("✔", "profiles/ directory removed (retired in v0.8.0)")
		}

		// Remove retired constraint JSON files (idempotent).
		retiredConstraints := []string{
			"delegation-mandatory",
			"think-before-execute",
			"know-what-you-dont-know",
			"peer-review-automatic",
			"token-economy-caveman",
			"token-economy-model-selection",
		}
		constraintsDir := filepath.Join(leoDir, "constraints")
		for _, name := range retiredConstraints {
			path := filepath.Join(constraintsDir, name+".json")
			if _, statErr := os.Stat(path); statErr == nil {
				if !dryRun {
					if err := os.Remove(path); err != nil {
						phase1Err = fmt.Errorf("removing retired constraint %s: %w", name, err)
						return
					}
				}
				addAction("✔", fmt.Sprintf("retired constraint %s removed", name))
			}
		}

		// Remove retired skill JSON files (idempotent).
		retiredSkills := []string{"task-intake"}
		skillsDir := filepath.Join(leoDir, "skills")
		for _, name := range retiredSkills {
			path := filepath.Join(skillsDir, name+".json")
			if _, statErr := os.Stat(path); statErr == nil {
				if !dryRun {
					if err := os.Remove(path); err != nil {
						phase1Err = fmt.Errorf("removing retired skill %s: %w", name, err)
						return
					}
				}
				addAction("✔", fmt.Sprintf("retired skill %s removed", name))
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
		schemaPath := filepath.Join(leoDir, "schema.json")
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
		constraintsDir := filepath.Join(leoDir, "constraints")
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
		skillsDir := filepath.Join(leoDir, "skills")
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

		// Migrate metric → session-log docs.
		docsDir := filepath.Join(leoDir, "memory")
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
		indexPath := filepath.Join(leoDir, "index.json")
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

// migrateKBLayout detects a legacy .leo/kb/ layout and promotes each subdirectory
// one level up to the new flat layout. It is idempotent: if the destination already
// exists it skips that step and reports a conflict rather than overwriting.
func migrateKBLayout(leoDir string) ([]upgradeAction, error) {
	kbDir := filepath.Join(leoDir, "kb")
	if _, err := os.Stat(kbDir); os.IsNotExist(err) {
		// Already on new layout — nothing to do.
		return nil, nil
	}

	var actions []upgradeAction

	type migration struct {
		src  string
		dst  string
		desc string
	}

	moves := []migration{
		{filepath.Join(kbDir, "docs"), filepath.Join(leoDir, "memory"), "kb/docs/ → memory/"},
		{filepath.Join(kbDir, "constraints"), filepath.Join(leoDir, "constraints"), "kb/constraints/ → constraints/"},
		{filepath.Join(kbDir, "skills"), filepath.Join(leoDir, "skills"), "kb/skills/ → skills/"},
		{filepath.Join(kbDir, "logs"), filepath.Join(leoDir, "logs"), "kb/logs/ → logs/"},
	}

	for _, m := range moves {
		if _, err := os.Stat(m.src); os.IsNotExist(err) {
			// Source doesn't exist — skip silently.
			continue
		}
		if _, err := os.Stat(m.dst); err == nil {
			// Destination already exists — partial migration; skip to avoid data loss.
			actions = append(actions, upgradeAction{"⚠", fmt.Sprintf("skipped %s — destination already exists", m.desc)})
			continue
		}
		if err := os.Rename(m.src, m.dst); err != nil {
			return nil, fmt.Errorf("moving %s: %w", m.desc, err)
		}
		actions = append(actions, upgradeAction{"✔", fmt.Sprintf("migrated %s", m.desc)})
	}

	// Move flat files.
	fileMovs := []migration{
		{filepath.Join(kbDir, "index.json"), filepath.Join(leoDir, "index.json"), "kb/index.json → index.json"},
		{filepath.Join(kbDir, "schema.json"), filepath.Join(leoDir, "schema.json"), "kb/schema.json → schema.json"},
	}
	for _, m := range fileMovs {
		if _, err := os.Stat(m.src); os.IsNotExist(err) {
			continue
		}
		if _, err := os.Stat(m.dst); err == nil {
			actions = append(actions, upgradeAction{"⚠", fmt.Sprintf("skipped %s — destination already exists", m.desc)})
			continue
		}
		if err := os.Rename(m.src, m.dst); err != nil {
			return nil, fmt.Errorf("moving %s: %w", m.desc, err)
		}
		actions = append(actions, upgradeAction{"✔", fmt.Sprintf("migrated %s", m.desc)})
	}

	// Remove kb/ if it is now empty (ignoring hidden files like .gitkeep).
	remaining, _ := os.ReadDir(kbDir)
	hasVisible := false
	for _, e := range remaining {
		if !strings.HasPrefix(e.Name(), ".") {
			hasVisible = true
			break
		}
	}
	if !hasVisible {
		if err := os.RemoveAll(kbDir); err != nil {
			actions = append(actions, upgradeAction{"⚠", fmt.Sprintf("could not remove empty kb/: %v", err)})
		} else {
			actions = append(actions, upgradeAction{"✔", "removed empty kb/ directory"})
		}
	}

	if len(actions) > 0 {
		actions = append([]upgradeAction{{"✔", "filesystem layout migrated to v0.8 (kb/ flattened)"}}, actions...)
	}

	return actions, nil
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

	runtimeCfg := buildRuntimeConfig(cfg)
	runtimeConstraints := buildRuntimeConstraints()
	runtimeSkills := buildRuntimeSkills()
	runtimeIdentity := buildRuntimeIdentity()

	for _, rt := range cfg.EnabledRuntimes() {
		adapter, ok := registry.Get(rt)
		if !ok {
			continue
		}
		if err := adapter.GenerateContextFile(runtimeCfg, runtimeConstraints, runtimeSkills, runtimeIdentity); err != nil {
			return fmt.Errorf("generating %s context: %w", rt, err)
		}
	}

	return nil
}
