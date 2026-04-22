package cmd

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	huhspinner "charm.land/huh/v2/spinner"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/momhq/mom/cli/internal/adapters/runtime"
	"github.com/momhq/mom/cli/internal/config"
	"github.com/momhq/mom/cli/internal/transponder"
)

//go:embed schema.json
var embeddedSchema embed.FS

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a .mom/ directory in the current project",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringSlice("runtimes", nil, "AI runtimes to configure (claude, codex, cline)")
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
			cmd.Println()
			if result.ScopeLabel == "user" || result.ScopeLabel == "org" {
				// Multi-repo: bootstrap each child repo that has .mom/.
				if err := bootstrapAllChildRepos(cmd, installDir); err != nil {
					cmd.Printf("  ⚠ multi-repo bootstrap error: %v\n", err)
				}
			} else {
				scanDir := installDir
				if result.BootstrapChoice == "repo" {
					scanDir = cwd
				}
				if err := runBootstrapInline(cmd, scanDir, filepath.Join(installDir, ".mom")); err != nil {
					cmd.Printf("  ⚠ bootstrap scan error: %v\n", err)
				}
			}
		}
		return nil
	}

	// Non-interactive path: use flags/defaults. Always installs at cwd with repo scope.
	runtimes, _ := cmd.Flags().GetStringSlice("runtimes")
	if len(runtimes) == 0 {
		runtimes = []string{"claude"}
	}

	defaults := config.Default()
	return runInitWithConfig(cmd, cwd, force, OnboardingResult{
		Runtimes:   runtimes,
		Language:   defaults.User.Language,
		Mode:       defaults.Communication.Mode,
		InstallDir: cwd,
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
			// .mom/ exists — skip scaffold+config but still honour bootstrap choice.
			alreadyExists = true
		}
	}

	// When .mom/ already exists and --force was not given, skip scaffold+config
	// but still return nil so the caller can run bootstrap if requested.
	if alreadyExists {
		cmd.Println("  .mom/ already exists — skipping scaffold, using existing config.")
		return nil
	}

	showSpinner := isTerminalWriter(cmd.OutOrStdout())

	// ── Phase 1: Scaffold directories ───────────────────────────────────────
	var scaffoldErr error
	doScaffold := func() {
		dirs := []string{
			leoDir,
			filepath.Join(leoDir, "memory"),
			filepath.Join(leoDir, "skills"),
			filepath.Join(leoDir, "constraints"),
			filepath.Join(leoDir, "logs"),
			filepath.Join(leoDir, "telemetry"),
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
		_ = huhspinner.New().Title("Scanning project structure...").Action(doScaffold).Run()
	} else {
		doScaffold()
	}
	if scaffoldErr != nil {
		return scaffoldErr
	}

	// ── Phase 2: Write memory structure ──────────────────────────────────────
	registry := runtime.NewRegistry(cwd)

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

		// Write core constraints.
		constraintsDir := filepath.Join(leoDir, "constraints")
		for name, content := range coreConstraints() {
			path := filepath.Join(constraintsDir, name+".json")
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				kbErr = fmt.Errorf("writing constraint %s: %w", name, err)
				return
			}
		}

		// Write core skills.
		skillsDir := filepath.Join(leoDir, "skills")
		for name, content := range coreSkills() {
			path := filepath.Join(skillsDir, name+".json")
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				kbErr = fmt.Errorf("writing skill %s: %w", name, err)
				return
			}
		}

		if showSpinner {
			time.Sleep(800 * time.Millisecond)
		}
	}

	if showSpinner {
		_ = huhspinner.New().Title("Writing memory structure...").Action(doWriteKB).Run()
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
				runtime.BackupIfNeeded(absPath) //nolint:errcheck
			}

			if err := adapter.GenerateContextFile(runtimeCfg, runtimeConstraints, runtimeSkills, runtimeIdentity); err != nil {
				genErr = err
				return
			}
		}

		if showSpinner {
			time.Sleep(500 * time.Millisecond)
		}
	}

	if showSpinner {
		_ = huhspinner.New().Title("Generating runtime context files...").Action(doGenerate).Run()
	} else {
		doGenerate()
	}
	if genErr != nil {
		return genErr
	}

	// ── Telemetry: emit smoke events ────────────────────────────────────────
	startedAt := time.Now().UTC().Format(time.RFC3339)
	emitter := transponder.New(leoDir, cfg.Telemetry.TelemetryEnabled())
	emitter.EmitSessionEvent(transponder.SessionEvent{
		SessionID: "s-init",
		RepoID:    filepath.Base(cwd),
		Runtime:   cfg.PrimaryRuntime(),
		StartedAt: startedAt,
		Trigger:   "normal",
	})
	emitter.EmitRuntimeHealth(transponder.RuntimeHealth{
		Runtime:       cfg.PrimaryRuntime(),
		TS:            time.Now().UTC().Format(time.RFC3339),
		WrapUpSuccess: true,
		LatencyMS:     0,
	})

	// ── Done ────────────────────────────────────────────────────────────────
	cmd.Println()
	cmd.Println("  ✔ .mom/ structure created")
	for _, rt := range result.Runtimes {
		adapter, ok := registry.Get(rt)
		if !ok {
			continue
		}
		for _, f := range adapter.GeneratedFiles() {
			absPath := filepath.Join(cwd, f)
			if _, statErr := os.Stat(absPath); statErr == nil {
				cmd.Printf("  ✔ %s\n", f)
			}
		}
	}
	// Warn about adapters with experimental MRP features.
	printExperimentalWarnings(cmd, registry, result.Runtimes)

	cmd.Println()
	cmd.Println("MOM is ready. Run 'mom status' to check health.")
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
			if _, err := os.Stat(gitPath); err == nil {
				cmd.Printf("\n  Bootstrapping %s...\n", e.Name())
				if err := runBootstrapInline(cmd, childPath, childLeo); err != nil {
					cmd.Printf("  ⚠ %s: %v\n", e.Name(), err)
				}
			} else {
				// Org folder — recurse into its children.
				if err := bootstrapAllChildRepos(cmd, childPath); err != nil {
					cmd.Printf("  ⚠ %s: %v\n", e.Name(), err)
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

		if childHasRepos {
			// Org folder: init with scope "org" and recurse into repos.
			childResult := parentResult
			childResult.InstallDir = childPath
			childResult.ScopeLabel = "org"
			childResult.BootstrapChoice = "" // bootstrap handled separately
			if err := runInitWithConfig(cmd, childPath, false, childResult); err != nil {
				cmd.Printf("  ⚠ failed to init %s: %v\n", childPath, err)
				continue
			}
			cmd.Printf("  ✔ initialized %s (scope: org)\n", e.Name())

			// Recurse: init repos inside this org folder.
			propagateInit(cmd, childPath, parentResult)
		} else if childHasGit {
			// Repo: init with scope "repo".
			childResult := parentResult
			childResult.InstallDir = childPath
			childResult.ScopeLabel = "repo"
			childResult.BootstrapChoice = "" // bootstrap handled separately
			if err := runInitWithConfig(cmd, childPath, false, childResult); err != nil {
				cmd.Printf("  ⚠ failed to init %s: %v\n", childPath, err)
				continue
			}
			cmd.Printf("  ✔ initialized %s (scope: repo)\n", e.Name())
		}
	}
}

// printExperimentalWarnings prints a warning for each adapter that carries
// experimental MRP v0 events — informing the user that capture may be less reliable.
func printExperimentalWarnings(cmd *cobra.Command, reg *runtime.Registry, runtimes []string) {
	for _, name := range runtimes {
		adapter, ok := reg.Get(name)
		if !ok {
			continue
		}
		cap := adapter.Capabilities()
		if len(cap.Experimental) == 0 {
			continue
		}
		adapterLabel := cap.Name
		if adapterLabel == "" {
			adapterLabel = name
		}
		version := cap.Version
		if version == "" {
			version = "unknown"
		}
		cmd.Printf("  ⚠ %s adapter installed (v%s)\n", adapterLabel, version)
		cmd.Printf("    Experimental: %s\n", strings.Join(cap.Experimental, ", "))
		cmd.Printf("    These events are emitted best-effort; capture may fire less reliably.\n")
	}
}

// isTerminalWriter returns true if w is connected to a terminal.
func isTerminalWriter(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(f.Fd())
	}
	return false
}

// buildRuntimeConfig converts a config.Config to a runtime.Config.
// Autonomy was retired from the persisted config in v0.9.0 (#74);
// the generated context files still include the autonomy section using
// the "balanced" default so the runtime retains the behavioral directive.
func buildRuntimeConfig(cfg *config.Config) runtime.Config {
	commMode := cfg.Communication.Mode
	if commMode == "" {
		commMode = "concise"
	}
	return runtime.Config{
		Version: cfg.Version,
		User: runtime.UserConfig{
			Language:          cfg.User.Language,
			Autonomy:          "balanced",
			CommunicationMode: commMode,
		},
	}
}

// buildRuntimeConstraints extracts constraint summaries from coreConstraints().
func buildRuntimeConstraints() []runtime.Constraint {
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
	return runtimeConstraints
}

// buildRuntimeSkills extracts skill summaries from coreSkills().
func buildRuntimeSkills() []runtime.Skill {
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
	return runtimeSkills
}

// buildRuntimeIdentity parses the identity JSON into a runtime.Identity.
func buildRuntimeIdentity() *runtime.Identity {
	var identityData struct {
		What        string   `json:"what"`
		Philosophy  string   `json:"philosophy"`
		Constraints []string `json:"constraints"`
	}
	json.Unmarshal([]byte(defaultIdentity()), &identityData) //nolint:errcheck
	return &runtime.Identity{
		What:        identityData.What,
		Philosophy:  identityData.Philosophy,
		Constraints: identityData.Constraints,
	}
}
