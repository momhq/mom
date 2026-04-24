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
	"github.com/momhq/mom/cli/internal/adapters/runtime"
	"github.com/momhq/mom/cli/internal/config"
	"gopkg.in/yaml.v3"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade .mom/ to the latest version (preserves your memory docs)",
	Long: `Upgrades core infrastructure (schema, constraints, skills, runtime files)
to match the installed mom binary. Your documents in .mom/memory/ are never touched.

Use --all to propagate the upgrade to all child scopes in the hierarchy
(org folders and repos beneath the current scope).`,
	RunE: runUpgrade,
}

func init() {
	upgradeCmd.Flags().Bool("dry-run", false, "Show what would change without modifying anything")
	upgradeCmd.Flags().Bool("all", false, "Upgrade all child scopes in the hierarchy")
}

// upgradeAction tracks a single change for reporting.
type upgradeAction struct {
	symbol string // ✔, ⚠, +
	desc   string
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	all, _ := cmd.Flags().GetBool("all")

	momDir, err := findMomDir()
	if err != nil {
		return err
	}

	projectRoot := filepath.Dir(momDir)

	// Upgrade the root scope.
	if err := upgradeSingleDir(cmd, projectRoot, dryRun); err != nil {
		return err
	}

	// Propagate to child scopes if --all.
	if all {
		propagateUpgrade(cmd, projectRoot, dryRun)
	}

	return nil
}

// upgradeSingleDir runs the full upgrade pipeline on a single .mom/ directory.
func upgradeSingleDir(cmd *cobra.Command, projectRoot string, dryRun bool) error {
	momDir := filepath.Join(projectRoot, ".mom")
	showSpinner := isTerminalWriter(cmd.OutOrStdout())

	// Check if this dir has a .mom/ at all.
	if _, err := os.Stat(momDir); os.IsNotExist(err) {
		// Try .leo/ fallback.
		leoDir := filepath.Join(projectRoot, ".leo")
		if _, err := os.Stat(leoDir); os.IsNotExist(err) {
			return nil // not a MOM project, skip silently
		}
		momDir = leoDir
	}

	if !isMomProject(momDir) {
		return nil
	}

	var actions []upgradeAction
	addAction := func(symbol, desc string) {
		actions = append(actions, upgradeAction{symbol, desc})
	}

	// ── Phase -1: Migrate .leo/ → .mom/ (v0.10 path migration) ─────────────
	isLegacyLeoDir := filepath.Base(momDir) == ".leo"
	if isLegacyLeoDir {
		if dryRun {
			addAction("⚠", fmt.Sprintf("would migrate %s → %s (run without --dry-run)", momDir, filepath.Join(projectRoot, ".mom")))
		} else {
			pathActions, err := migrateLeoToMom(momDir)
			if err != nil {
				return fmt.Errorf("path migration: %w", err)
			}
			for _, a := range pathActions {
				addAction(a.symbol, a.desc)
			}
			momDir = filepath.Join(projectRoot, ".mom")
		}
	}

	leoDir := momDir

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
			addAction("⚠", "legacy .mom/kb/ detected — would flatten to new layout (run without --dry-run)")
		}
	}

	// ── Phase 1: Load, migrate, and persist config ───────────────────────────
	cfg, err := config.Load(leoDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	var phase1Err error
	doPhase1 := func() {
		if cfg.Communication.Mode == "" {
			cfg.Communication.Mode = "concise"
			addAction("✔", "communication.mode set to concise (default)")
		}

		if err := config.Save(leoDir, cfg); err != nil {
			phase1Err = fmt.Errorf("saving config: %w", err)
			return
		}
		addAction("✔", "config.yaml migrated to latest format")

		if scrubbed, changed, err := scrubDeadConfigFields(leoDir); err != nil {
			phase1Err = fmt.Errorf("scrubbing dead config fields: %w", err)
			return
		} else if changed {
			if !dryRun {
				configPath := filepath.Join(leoDir, "config.yaml")
				if err := os.WriteFile(configPath, scrubbed, 0644); err != nil {
					phase1Err = fmt.Errorf("writing scrubbed config: %w", err)
					return
				}
			}
			addAction("✔", "removed retired fields (tiers, autonomy) from config.yaml")
		}

		newDirs := []string{
			filepath.Join(leoDir, "memory"),
			filepath.Join(leoDir, "constraints"),
			filepath.Join(leoDir, "skills"),
			filepath.Join(leoDir, "logs"),
			filepath.Join(leoDir, "cache"),
			filepath.Join(leoDir, "raw"),
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

	// ── Phase 2: Update core memory docs ──────────────────────────────────────
	var phase2Err error
	doPhase2 := func() {
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

		docsDir := filepath.Join(leoDir, "memory")
		migrated := migrateMetricDocs(docsDir, dryRun)
		for _, docID := range migrated {
			addAction("✔", fmt.Sprintf("doc %s migrated metric → session-log", docID))
		}

		migratedPatterns := migrateFactASTDocs(docsDir, dryRun)
		for _, docID := range migratedPatterns {
			addAction("✔", fmt.Sprintf("doc %s migrated fact → pattern", docID))
		}

		if showSpinner {
			time.Sleep(700 * time.Millisecond)
		}
	}

	if showSpinner {
		_ = huhspinner.New().Title("Updating memory structure...").Action(doPhase2).Run()
	} else {
		doPhase2()
	}
	if phase2Err != nil {
		return phase2Err
	}

	// ── Phase 3: Rebuild index and regenerate runtime files ─────────────────
	var phase3Err error
	doPhase3 := func() {
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

	// ── Phase 4: Update .gitignore ──────────────────────────────────────────
	if !dryRun {
		registry := runtime.NewRegistry(projectRoot)
		enabledRTs := cfg.EnabledRuntimes()
		if added, gitErr := ensureGitIgnore(projectRoot, registry, enabledRTs); gitErr != nil {
			addAction("⚠", fmt.Sprintf(".gitignore: %v", gitErr))
		} else if len(added) > 0 {
			addAction("✔", fmt.Sprintf(".gitignore updated (%d entries added)", len(added)))
		}
	}

	// ── Report ──────────────────────────────────────────────────────────────
	home, _ := os.UserHomeDir()
	display := projectRoot
	if strings.HasPrefix(display, home) {
		display = "~" + display[len(home):]
	}

	cmd.Println()
	if dryRun {
		cmd.Printf("  [%s] Dry run — no changes made. Would apply:\n", display)
	} else {
		cmd.Printf("  [%s] Upgrade complete:\n", display)
	}
	cmd.Println()
	for _, a := range actions {
		cmd.Printf("  %s %s\n", a.symbol, a.desc)
	}
	if len(actions) == 0 {
		cmd.Println("  Everything is already up to date.")
	}
	cmd.Println()

	return nil
}

// propagateUpgrade walks child directories and upgrades each .mom/ found.
// Follows the same pattern as propagateInit: org folders (containing repos)
// are upgraded and recursed into; repos (with .git/) are upgraded.
func propagateUpgrade(cmd *cobra.Command, rootDir string, dryRun bool) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		childPath := filepath.Join(rootDir, e.Name())
		childMom := filepath.Join(childPath, ".mom")

		// Only upgrade dirs that have .mom/.
		if _, statErr := os.Stat(childMom); statErr != nil {
			continue
		}
		if !isMomProject(childMom) {
			continue
		}

		if err := upgradeSingleDir(cmd, childPath, dryRun); err != nil {
			home, _ := os.UserHomeDir()
			display := childPath
			if strings.HasPrefix(display, home) {
				display = "~" + display[len(home):]
			}
			cmd.Printf("  ⚠ failed to upgrade %s: %v\n", display, err)
			continue
		}

		// Recurse into org folders (dirs that contain repos).
		if containsGitRepos(childPath) {
			propagateUpgrade(cmd, childPath, dryRun)
		}
	}
}

// fileChanged returns true if the file at path doesn't exist or differs from data.
func fileChanged(path string, data []byte) bool {
	existing, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	return string(existing) != string(data)
}

// migrateKBLayout detects a legacy .mom/kb/ layout and promotes each subdirectory
// one level up to the new flat layout. It is idempotent: if the destination already
// exists it skips that step and reports a conflict rather than overwriting.
func migrateKBLayout(leoDir string) ([]upgradeAction, error) {
	kbDir := filepath.Join(leoDir, "kb")
	if _, err := os.Stat(kbDir); os.IsNotExist(err) {
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

// migrateFactASTDocs finds docs with type "fact" that carry an "ast" or "bootstrap"
// tag (written by the cartographer before pattern was a first-class type) and
// converts them to type "pattern". Plain fact docs without those tags are untouched.
func migrateFactASTDocs(docsDir string, dryRun bool) []string {
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
		if !ok || docType != "fact" {
			continue
		}

		tags, _ := doc["tags"].([]interface{})
		hasASTTag := false
		for _, tag := range tags {
			if s, ok := tag.(string); ok && (s == "ast" || s == "bootstrap") {
				hasASTTag = true
				break
			}
		}
		if !hasASTTag {
			continue
		}

		docID, _ := doc["id"].(string)
		if !dryRun {
			doc["type"] = "pattern"
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

		// Register MCP server config and hooks for all adapters.
		if err := adapter.RegisterMCP(); err != nil {
			return fmt.Errorf("registering %s MCP config: %w", rt, err)
		}
		if adapter.SupportsHooks() {
			if err := adapter.RegisterHooks(runtime.HooksForRuntime(rt)); err != nil {
				return fmt.Errorf("registering %s hooks: %w", rt, err)
			}
		}
	}

	return nil
}

// scrubDeadConfigFields reads config.yaml from leoDir, removes the retired
// "tiers" key from every runtime block and the "autonomy" key from the user
// block, and returns the cleaned bytes plus a changed flag. It does nothing
// when the keys are already absent.
//
// The scrub operates on the raw YAML node tree so that comments and
// formatting are preserved as much as possible.
func scrubDeadConfigFields(leoDir string) (scrubbed []byte, changed bool, err error) {
	configPath := filepath.Join(leoDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, false, fmt.Errorf("reading config: %w", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, false, fmt.Errorf("parsing config: %w", err)
	}
	if root.Kind == 0 || len(root.Content) == 0 {
		return data, false, nil
	}

	doc := root.Content[0] // document node wraps a mapping node
	if doc.Kind != yaml.MappingNode {
		return data, false, nil
	}

	changed = removeKeyFromMapping(doc, "runtimes", func(runtimesNode *yaml.Node) {
		if runtimesNode.Kind != yaml.MappingNode {
			return
		}
		for i := 1; i < len(runtimesNode.Content); i += 2 {
			rtVal := runtimesNode.Content[i]
			if rtVal.Kind == yaml.MappingNode {
				if removeYAMLKey(rtVal, "tiers") {
					changed = true
				}
			}
		}
	}) || changed

	if removeYAMLKey(findMappingValue(doc, "user"), "autonomy") {
		changed = true
	}

	if !changed {
		return data, false, nil
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return nil, false, fmt.Errorf("marshaling scrubbed config: %w", err)
	}
	return out, true, nil
}

// removeKeyFromMapping visits the value node for key in a YAML mapping node,
// calling fn with the value node. Returns true if the key was found (fn may
// set the outer changed flag).
func removeKeyFromMapping(mapping *yaml.Node, key string, fn func(*yaml.Node)) bool {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			fn(mapping.Content[i+1])
			return true
		}
	}
	return false
}

// findMappingValue returns the value node for key in a YAML mapping node, or nil.
func findMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// removeYAMLKey removes the key+value pair for key from a YAML mapping node.
// Returns true if the key was present and removed.
func removeYAMLKey(mapping *yaml.Node, key string) bool {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return true
		}
	}
	return false
}

// migrateLeoToMom copies a .leo/ directory to .mom/ at the same level.
// The .leo/ directory is preserved (not deleted) — users can remove it manually
// or it will be removed in v0.12. Returns actions describing what was done.
// If .mom/ already exists, the migration is skipped.
func migrateLeoToMom(leoDir string) ([]upgradeAction, error) {
	parent := filepath.Dir(leoDir)
	momDir := filepath.Join(parent, ".mom")

	if _, err := os.Stat(momDir); err == nil {
		return nil, nil
	}

	var actions []upgradeAction

	if err := copyDirRecursive(leoDir, momDir); err != nil {
		return nil, fmt.Errorf("copying .leo/ to .mom/: %w", err)
	}

	actions = append(actions, upgradeAction{"✔", fmt.Sprintf("migrated %s → %s", leoDir, momDir)})
	actions = append(actions, upgradeAction{"⚠", ".leo/ preserved — remove it manually after verifying .mom/ works"})

	return actions, nil
}

// copyDirRecursive recursively copies src directory to dst.
func copyDirRecursive(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(target, data, info.Mode())
	})
}
