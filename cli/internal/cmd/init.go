package cmd

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/momhq/mom/cli/internal/adapters/harness"
	"github.com/momhq/mom/cli/internal/config"
	"github.com/momhq/mom/cli/internal/herald"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/momhq/mom/cli/internal/ux"
)

//go:embed schema.json
var embeddedSchema embed.FS

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a .mom/ directory in the current project",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringSlice("runtimes", nil, "AI runtimes to configure (claude, codex, windsurf, pi)")
	initCmd.Flags().Bool("force", false, "Overwrite existing .mom/ directory")
	initCmd.Flags().BoolP("no-interactive", "y", false, "Skip the interactive wizard and use defaults/flags")
}

func runInit(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	noInteractive, _ := cmd.Flags().GetBool("no-interactive")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Run the interactive onboarding wizard unless:
	//   - --no-interactive / -y was passed, OR
	//   - --runtimes was explicitly provided by the user (direct/scripted mode).
	if !noInteractive && !cmd.Flags().Changed("runtimes") {
		result, err := runOnboarding(cmd.InOrStdin(), cmd.OutOrStdout(), cwd)
		if err != nil {
			return err
		}
		installDir := result.InstallDir
		if installDir == "" {
			installDir = cwd
		}
		if err := runInitWithConfig(cmd, installDir, force, result); err != nil {
			return err
		}

		// Propagate: when scope is user/org, initialize child scopes automatically.
		if result.ScopeLabel == "user" || result.ScopeLabel == "org" {
			propagateInit(cmd, installDir, result)
		}

		// Run bootstrap inline if the user opted in (non-interactive -y always skips).
		if result.BootstrapChoice != "" && result.BootstrapChoice != "skip" {
			p := ux.NewPrinter(cmd.OutOrStdout())
			p.Blank()
			if result.ScopeLabel == "user" || result.ScopeLabel == "org" {
				// Multi-repo: bootstrap each child repo that has .mom/.
				if err := bootstrapAllChildRepos(cmd, installDir); err != nil {
					p.Warnf("multi-repo bootstrap error: %v", err)
				}
			} else {
				scanDir := installDir
				if result.BootstrapChoice == "repo" {
					scanDir = cwd
				}
				if err := runBootstrapInline(cmd, scanDir, filepath.Join(installDir, ".mom")); err != nil {
					p.Warnf("bootstrap scan error: %v", err)
				}
			}
		}
		return nil
	}

	// Non-interactive path: use flags/defaults.
	// When --force, look for an existing .mom/ up the directory tree so we
	// regenerate context files in the right place instead of the cwd.
	installDir := cwd
	if force {
		if sc, ok := scope.NearestWritable(cwd); ok {
			installDir = filepath.Dir(sc.Path)
		}
	}

	runtimes, _ := cmd.Flags().GetStringSlice("runtimes")
	if len(runtimes) == 0 {
		runtimes = []string{"claude"}
	}

	defaults := config.Default()
	return runInitWithConfig(cmd, installDir, force, OnboardingResult{
		Runtimes:   runtimes,
		Language:   defaults.User.Language,
		Mode:       defaults.Communication.Mode,
		InstallDir: installDir,
		ScopeLabel: "repo",
	})
}

// runInitWithConfig performs the actual directory and file creation using the
// resolved configuration from either the wizard or flag defaults.
// cwd is the directory where .mom/ will be created (may differ from os.Getwd()
// when the user chose a parent install location during onboarding).
func runInitWithConfig(cmd *cobra.Command, cwd string, force bool, result OnboardingResult) error {
	leoDir := filepath.Join(cwd, ".mom")

	// Check if already initialized.
	alreadyExists := false
	if _, err := os.Stat(leoDir); err == nil {
		if !force {
			alreadyExists = true
		}
	}

	p := ux.NewPrinter(cmd.OutOrStdout())

	// When .mom/ already exists: update config with new runtimes, regenerate
	// runtime files, and reinstall daemon — but skip scaffold.
	if alreadyExists {
		return runReinit(cmd, cwd, leoDir, result, p)
	}

	showSpinner := ux.IsTTY(cmd.OutOrStdout())

	// ── Phase 1: Scaffold directories ───────────────────────────────────────
	var scaffoldErr error
	doScaffold := func() {
		dirs := []string{
			leoDir,
			filepath.Join(leoDir, "memory"),
			filepath.Join(leoDir, "skills"),
			filepath.Join(leoDir, "constraints"),
			filepath.Join(leoDir, "logs"),
			filepath.Join(leoDir, "cache"),
			filepath.Join(leoDir, "raw"),
		}
		for _, d := range dirs {
			if err := os.MkdirAll(d, 0755); err != nil {
				scaffoldErr = fmt.Errorf("creating %s: %w", d, err)
				return
			}
		}
		if showSpinner {
			time.Sleep(600 * time.Millisecond)
		}
	}

	if showSpinner {
		sp := ux.NewSpinner(os.Stderr)
		sp.Start("Scanning project structure")
		doScaffold()
		sp.Stop()
	} else {
		doScaffold()
	}
	if scaffoldErr != nil {
		return scaffoldErr
	}

	// ── Phase 2: Write memory structure ──────────────────────────────────────
	registry := harness.NewRegistry(cwd)

	var kbErr error
	doWriteKB := func() {
		// Build runtime config from selected runtimes.
		runtimesCfg := make(map[string]config.RuntimeConfig)
		for _, rt := range result.Runtimes {
			_, ok := registry.Get(rt)
			if !ok {
				continue
			}
			runtimesCfg[rt] = config.RuntimeConfig{Enabled: true}
		}

		// Infer communication.mode from the onboarding mode selection.
		commMode := result.Mode
		if commMode == "" {
			commMode = "concise"
		}

		// Determine scope label — default to "repo" for backward compat.
		scopeLabel := result.ScopeLabel
		if scopeLabel == "" {
			scopeLabel = "repo"
		}

		// Write config.yaml.
		cfg := config.Config{
			Version:    "1",
			CoreSource: result.CoreSource,
			Scope:      scopeLabel,
			Runtimes:   runtimesCfg,
			User: config.UserConfig{
				Language: result.Language,
			},
			Communication: config.CommunicationConfig{
				Mode: commMode,
			},
			Memory: config.Default().Memory,
		}

		if err := config.Save(leoDir, &cfg); err != nil {
			kbErr = err
			return
		}

		// Write schema.json.
		schemaData, err := embeddedSchema.ReadFile("schema.json")
		if err != nil {
			kbErr = fmt.Errorf("reading embedded schema: %w", err)
			return
		}
		schemaPath := filepath.Join(leoDir, "schema.json")
		if err := os.WriteFile(schemaPath, schemaData, 0644); err != nil {
			kbErr = fmt.Errorf("writing schema: %w", err)
			return
		}

		// Write identity.json.
		identityPath := filepath.Join(leoDir, "identity.json")
		if err := os.WriteFile(identityPath, []byte(defaultIdentity()), 0644); err != nil {
			kbErr = fmt.Errorf("writing identity.json: %w", err)
			return
		}

		// Write core constraints — skip if a parent scope already provides them.
		if !parentScopeHasDir(cwd, "constraints") {
			constraintsDir := filepath.Join(leoDir, "constraints")
			for name, content := range coreConstraints() {
				path := filepath.Join(constraintsDir, name+".json")
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					kbErr = fmt.Errorf("writing constraint %s: %w", name, err)
					return
				}
			}
		}

		// Write core skills — skip if a parent scope already provides them.
		if !parentScopeHasDir(cwd, "skills") {
			skillsDir := filepath.Join(leoDir, "skills")
			for name, content := range coreSkills() {
				path := filepath.Join(skillsDir, name+".json")
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					kbErr = fmt.Errorf("writing skill %s: %w", name, err)
					return
				}
			}
		}

		if showSpinner {
			time.Sleep(800 * time.Millisecond)
		}
	}

	if showSpinner {
		sp := ux.NewSpinner(os.Stderr)
		sp.Start("Writing memory structure")
		doWriteKB()
		sp.Stop()
	} else {
		doWriteKB()
	}
	if kbErr != nil {
		return kbErr
	}

	// Re-load config for runtime generation.
	cfg, err := config.Load(leoDir)
	if err != nil {
		return fmt.Errorf("loading config after write: %w", err)
	}

	// ── Phase 3: Generate runtime context files ────────────────────────────
	var genErr error
	doGenerate := func() {
		runtimeCfg := buildRuntimeConfig(cfg)

		// Build constraints list from core constraints.
		runtimeConstraints := buildRuntimeConstraints()
		runtimeSkills := buildRuntimeSkills()
		runtimeIdentity := buildRuntimeIdentity()

		// Generate context files for all selected runtimes.
		for _, rt := range result.Runtimes {
			adapter, ok := registry.Get(rt)
			if !ok {
				continue
			}

			// Backup existing files if needed.
			for _, relPath := range adapter.GeneratedFiles() {
				absPath := filepath.Join(cwd, relPath)
				harness.BackupIfNeeded(absPath) //nolint:errcheck
			}

			if err := adapter.GenerateContextFile(runtimeCfg, runtimeConstraints, runtimeSkills, runtimeIdentity); err != nil {
				genErr = err
				return
			}

			// Register MCP server config and hooks. The Adapter interface
			// covers all Harnesses uniformly; each adapter knows its own hook
			// set via HooksForHarness(name).
			if err := adapter.RegisterMCP(); err != nil {
				genErr = err
				return
			}
			if adapter.SupportsHooks() {
				if err := adapter.RegisterHooks(harness.HooksForHarness(rt)); err != nil {
					genErr = err
					return
				}
			}
		}

		if showSpinner {
			time.Sleep(500 * time.Millisecond)
		}
	}

	if showSpinner {
		sp := ux.NewSpinner(os.Stderr)
		sp.Start("Generating runtime context files")
		doGenerate()
		sp.Stop()
	} else {
		doGenerate()
	}
	if genErr != nil {
		return genErr
	}

	// ── Phase 3.5: Register with global watch daemon ────────────────────────
	if err := ensureGlobalDaemon(cwd, leoDir, result.Runtimes); err != nil {
		p.Warnf("watch daemon: %v", err)
	} else {
		p.Check("watch daemon installed")
	}

	// ── Phase 4: Update .gitignore ──────────────────────────────────────────
	if added, gitErr := ensureGitIgnore(cwd, registry, result.Runtimes); gitErr != nil {
		p.Warnf(".gitignore: %v", gitErr)
	} else if len(added) > 0 {
		p.Checkf(".gitignore updated (%d entries added)", len(added))
	}

	// ── Telemetry: emit smoke events ────────────────────────────────────────
	startedAt := time.Now().UTC().Format(time.RFC3339)
	emitter := herald.New(leoDir, cfg.Telemetry.TelemetryEnabled())
	emitter.EmitSessionEvent(herald.SessionEvent{
		SessionID: "s-init",
		RepoID:    filepath.Base(cwd),
		Runtime:   cfg.PrimaryRuntime(),
		StartedAt: startedAt,
		Trigger:   "normal",
	})
	emitter.EmitRuntimeHealth(herald.RuntimeHealth{
		Runtime:       cfg.PrimaryRuntime(),
		TS:            time.Now().UTC().Format(time.RFC3339),
		WrapUpSuccess: true,
		LatencyMS:     0,
	})

	// ── Done ────────────────────────────────────────────────────────────────
	p.Blank()
	p.Check(".mom/ structure created")
	for _, rt := range result.Runtimes {
		adapter, ok := registry.Get(rt)
		if !ok {
			continue
		}
		for _, f := range adapter.GeneratedFiles() {
			absPath := filepath.Join(cwd, f)
			if _, statErr := os.Stat(absPath); statErr == nil {
				p.Check(f)
			}
		}
	}
	p.Blank()
	p.Textf("MOM is ready. Run %s to check health.", p.HighlightCmd("mom status"))
	return nil
}

// runReinit handles `mom init` on an already-initialized project.
// Updates runtimes in config, regenerates context files, and reinstalls daemon.
func runReinit(cmd *cobra.Command, cwd, leoDir string, result OnboardingResult, p *ux.Printer) error {
	cfg, err := config.Load(leoDir)
	if err != nil {
		// Corrupt or missing config — fall back to informational message.
		p.Muted(".mom/ already exists — run with --force to reinitialize from scratch.")
		return nil
	}

	// Merge new runtimes into existing config.
	changed := false
	for _, rt := range result.Runtimes {
		if _, exists := cfg.Runtimes[rt]; !exists {
			cfg.Runtimes[rt] = config.RuntimeConfig{Enabled: true}
			changed = true
		}
	}

	if !changed {
		// Still register with global daemon even if runtimes unchanged.
		if err := ensureGlobalDaemon(cwd, leoDir, cfg.EnabledRuntimes()); err != nil {
			p.Warnf("watch daemon: %v", err)
		}
		p.Muted(".mom/ already configured with selected runtimes — nothing to update.")
		return nil
	}

	// Save updated config.
	if err := config.Save(leoDir, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	// Regenerate runtime context files for all enabled runtimes.
	registry := harness.NewRegistry(cwd)
	runtimeCfg := buildRuntimeConfig(cfg)
	runtimeConstraints := buildRuntimeConstraints()
	runtimeSkills := buildRuntimeSkills()
	runtimeIdentity := buildRuntimeIdentity()

	for _, rt := range cfg.EnabledRuntimes() {
		adapter, ok := registry.Get(rt)
		if !ok {
			continue
		}

		for _, relPath := range adapter.GeneratedFiles() {
			absPath := filepath.Join(cwd, relPath)
			harness.BackupIfNeeded(absPath) //nolint:errcheck
		}

		if err := adapter.GenerateContextFile(runtimeCfg, runtimeConstraints, runtimeSkills, runtimeIdentity); err != nil {
			p.Warnf("generating %s context: %v", rt, err)
			continue
		}

		// Register MCP + hooks generically. Same pattern as upgrade.go and
		// the fresh-init path above; covers any adapter implementing the
		// harness.Adapter interface, including pi.
		_ = adapter.RegisterMCP()
		if adapter.SupportsHooks() {
			_ = adapter.RegisterHooks(harness.HooksForHarness(rt))
		}
	}

	// Register with global watch daemon (updated runtimes).
	if err := ensureGlobalDaemon(cwd, leoDir, cfg.EnabledRuntimes()); err != nil {
		p.Warnf("watch daemon: %v", err)
	} else {
		p.Check("watch daemon updated")
	}

	// Update .gitignore.
	if added, gitErr := ensureGitIgnore(cwd, registry, cfg.EnabledRuntimes()); gitErr != nil {
		p.Warnf(".gitignore: %v", gitErr)
	} else if len(added) > 0 {
		p.Checkf(".gitignore updated (%d entries added)", len(added))
	}

	p.Blank()
	p.Check("configuration updated")
	for _, rt := range result.Runtimes {
		if adapter, ok := registry.Get(rt); ok {
			for _, f := range adapter.GeneratedFiles() {
				absPath := filepath.Join(cwd, f)
				if _, statErr := os.Stat(absPath); statErr == nil {
					p.Check(f)
				}
			}
		}
	}
	return nil
}

// bootstrapAllChildRepos walks rootDir recursively, finds every directory that
// has .mom/, and runs bootstrapInline for each one. Org folders (scope: org)
// are skipped because they don't contain source code directly.
func bootstrapAllChildRepos(cmd *cobra.Command, rootDir string) error {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		childPath := filepath.Join(rootDir, e.Name())
		childLeo := filepath.Join(childPath, ".mom")

		// If this child has .mom/ and .git/, it's a repo — bootstrap it.
		gitPath := filepath.Join(childPath, ".git")
		if _, err := os.Stat(childLeo); err == nil {
			bp := ux.NewPrinter(cmd.OutOrStdout())
			if _, err := os.Stat(gitPath); err == nil {
				bp.Blank()
				bp.Textf("Bootstrapping %s...", e.Name())
				if err := runBootstrapInline(cmd, childPath, childLeo); err != nil {
					bp.Warnf("%s: %v", e.Name(), err)
				}
			} else {
				// Org folder — recurse into its children.
				if err := bootstrapAllChildRepos(cmd, childPath); err != nil {
					bp.Warnf("%s: %v", e.Name(), err)
				}
			}
		}
	}
	return nil
}

// propagateInit initializes .mom/ in child directories when the parent scope
// is user or org. Org folders (dirs containing repos) get scope "org", and
// repos (dirs with .git/) get scope "repo". Already-initialized dirs are skipped.
func propagateInit(cmd *cobra.Command, rootDir string, parentResult OnboardingResult) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		childPath := filepath.Join(rootDir, e.Name())
		childLeo := filepath.Join(childPath, ".mom")

		// Skip if already initialized.
		if _, statErr := os.Stat(childLeo); statErr == nil {
			continue
		}

		childHasGit := false
		if info, statErr := os.Stat(filepath.Join(childPath, ".git")); statErr == nil && info.IsDir() {
			childHasGit = true
		}
		childHasRepos := containsGitRepos(childPath)

		pp := ux.NewPrinter(cmd.OutOrStdout())
		if childHasRepos {
			// Org folder: init with scope "org" and recurse into repos.
			childResult := parentResult
			childResult.InstallDir = childPath
			childResult.ScopeLabel = "org"
			childResult.BootstrapChoice = "" // bootstrap handled separately
			if err := runInitWithConfig(cmd, childPath, false, childResult); err != nil {
				pp.Warnf("failed to init %s: %v", childPath, err)
				continue
			}
			pp.Checkf("initialized %s (scope: org)", e.Name())

			// Recurse: init repos inside this org folder.
			propagateInit(cmd, childPath, parentResult)
		} else if childHasGit {
			// Repo: init with scope "repo".
			childResult := parentResult
			childResult.InstallDir = childPath
			childResult.ScopeLabel = "repo"
			childResult.BootstrapChoice = "" // bootstrap handled separately
			if err := runInitWithConfig(cmd, childPath, false, childResult); err != nil {
				pp.Warnf("failed to init %s: %v", childPath, err)
				continue
			}
			pp.Checkf("initialized %s (scope: repo)", e.Name())
		}
	}
}


// buildRuntimeConfig converts a config.Config to a harness.Config.
// Autonomy was retired from the persisted config in v0.9.0 (#74);
// the generated context files still include the autonomy section using
// the "balanced" default so the runtime retains the behavioral directive.
func buildRuntimeConfig(cfg *config.Config) harness.Config {
	commMode := cfg.Communication.Mode
	if commMode == "" {
		commMode = "concise"
	}
	delivery := cfg.Delivery
	if delivery == "" {
		delivery = "mcp"
	}
	return harness.Config{
		Version: cfg.Version,
		User: harness.UserConfig{
			Language:          cfg.User.Language,
			Autonomy:          "balanced",
			CommunicationMode: commMode,
		},
		Delivery: delivery,
	}
}

// buildRuntimeConstraints extracts constraint summaries from coreConstraints().
func buildRuntimeConstraints() []harness.Constraint {
	var runtimeConstraints []harness.Constraint
	for id := range coreConstraints() {
		var doc struct {
			Summary string `json:"summary"`
		}
		json.Unmarshal([]byte(coreConstraints()[id]), &doc) //nolint:errcheck
		runtimeConstraints = append(runtimeConstraints, harness.Constraint{
			ID:      id,
			Summary: doc.Summary,
		})
	}
	sort.Slice(runtimeConstraints, func(i, j int) bool {
		return runtimeConstraints[i].ID < runtimeConstraints[j].ID
	})
	return runtimeConstraints
}

// buildRuntimeSkills extracts skill summaries from coreSkills().
func buildRuntimeSkills() []harness.Skill {
	var runtimeSkills []harness.Skill
	for id := range coreSkills() {
		var doc struct {
			Summary string `json:"summary"`
		}
		json.Unmarshal([]byte(coreSkills()[id]), &doc) //nolint:errcheck
		runtimeSkills = append(runtimeSkills, harness.Skill{
			ID:      id,
			Summary: doc.Summary,
		})
	}
	sort.Slice(runtimeSkills, func(i, j int) bool {
		return runtimeSkills[i].ID < runtimeSkills[j].ID
	})
	return runtimeSkills
}

// buildRuntimeIdentity parses the identity JSON into a harness.Identity.
func buildRuntimeIdentity() *harness.Identity {
	var identityData struct {
		What        string   `json:"what"`
		Philosophy  string   `json:"philosophy"`
		Constraints []string `json:"constraints"`
	}
	json.Unmarshal([]byte(defaultIdentity()), &identityData) //nolint:errcheck
	return &harness.Identity{
		What:        identityData.What,
		Philosophy:  identityData.Philosophy,
		Constraints: identityData.Constraints,
	}
}

// parentScopeHasDir walks up from dir looking for a parent .mom/ directory that
// contains the given subdirectory (e.g. "constraints" or "skills") with at least
// one .json file. This allows child scopes to inherit from a parent scope
// instead of duplicating files. Only real parent directories are checked — dir
// itself is skipped.
func parentScopeHasDir(dir, subdir string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		home = string(filepath.Separator)
	}

	current := dir
	for {
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root.
			break
		}
		current = parent

		candidate := filepath.Join(current, ".mom", subdir)
		if hasJSONFiles(candidate) {
			return true
		}

		// Stop after processing $HOME (same boundary as scope.Walk).
		if current == home {
			break
		}
	}
	return false
}

// hasJSONFiles returns true if dir exists and contains at least one .json file.
func hasJSONFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			return true
		}
	}
	return false
}
