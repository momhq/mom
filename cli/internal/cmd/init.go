package cmd

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	huhspinner "charm.land/huh/v2/spinner"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/adapters/runtime"
	"github.com/vmarinogg/leo-core/cli/internal/config"
)

//go:embed schema.json
var embeddedSchema embed.FS

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a .leo/ directory in the current project",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringSlice("runtimes", nil, "AI runtimes to configure (claude, codex, cline)")
	initCmd.Flags().Bool("force", false, "Overwrite existing .leo/ directory")
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
		return runInitWithConfig(cmd, cwd, force, result)
	}

	// Non-interactive path: use flags/defaults.
	runtimes, _ := cmd.Flags().GetStringSlice("runtimes")
	if len(runtimes) == 0 {
		runtimes = []string{"claude"}
	}

	defaults := config.Default()
	return runInitWithConfig(cmd, cwd, force, OnboardingResult{
		Runtimes: runtimes,
		Language: defaults.User.Language,
		Mode:     defaults.User.Mode,
		Autonomy: defaults.User.Autonomy,
	})
}

// runInitWithConfig performs the actual directory and file creation using the
// resolved configuration from either the wizard or flag defaults.
func runInitWithConfig(cmd *cobra.Command, cwd string, force bool, result OnboardingResult) error {
	leoDir := filepath.Join(cwd, ".leo")

	// Check if already initialized.
	if _, err := os.Stat(leoDir); err == nil && !force {
		return fmt.Errorf(".leo/ already exists — use --force to overwrite")
	}

	showSpinner := isTerminalWriter(cmd.OutOrStdout())

	// ── Phase 1: Scaffold directories ───────────────────────────────────────
	var scaffoldErr error
	doScaffold := func() {
		dirs := []string{
			leoDir,
			filepath.Join(leoDir, "kb", "docs"),
			filepath.Join(leoDir, "kb", "skills"),
			filepath.Join(leoDir, "kb", "constraints"),
			filepath.Join(leoDir, "kb", "logs"),
			filepath.Join(leoDir, "cache"),
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

	// ── Phase 2: Write knowledge base ───────────────────────────────────────
	registry := runtime.NewRegistry(cwd)

	var kbErr error
	doWriteKB := func() {
		// Build runtime config from selected runtimes.
		runtimesCfg := make(map[string]config.RuntimeConfig)
		for _, rt := range result.Runtimes {
			adapter, ok := registry.Get(rt)
			if !ok {
				continue
			}
			runtimesCfg[rt] = config.RuntimeConfig{
				Enabled: true,
				Tiers:   adapter.DefaultTierMapping(),
			}
		}

		// Infer communication.mode from the onboarding mode selection.
		commMode := result.Mode
		if commMode == "" {
			commMode = "concise"
		}

		// Write config.yaml.
		cfg := config.Config{
			Version:    "1",
			CoreSource: result.CoreSource,
			Runtimes:   runtimesCfg,
			User: config.UserConfig{
				Language: result.Language,
				Mode:     result.Mode,
				Autonomy: result.Autonomy,
			},
			Communication: config.CommunicationConfig{
				Mode: commMode,
			},
			KB: config.Default().KB,
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
		schemaPath := filepath.Join(leoDir, "kb", "schema.json")
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
		constraintsDir := filepath.Join(leoDir, "kb", "constraints")
		for name, content := range coreConstraints() {
			path := filepath.Join(constraintsDir, name+".json")
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				kbErr = fmt.Errorf("writing constraint %s: %w", name, err)
				return
			}
		}

		// Write core skills.
		skillsDir := filepath.Join(leoDir, "kb", "skills")
		for name, content := range coreSkills() {
			path := filepath.Join(skillsDir, name+".json")
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				kbErr = fmt.Errorf("writing skill %s: %w", name, err)
				return
			}
		}

		// Write index.json with entries for all core docs.
		indexPath := filepath.Join(leoDir, "kb", "index.json")
		indexData, err := buildCoreIndex()
		if err != nil {
			kbErr = fmt.Errorf("building index: %w", err)
			return
		}
		if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
			kbErr = fmt.Errorf("writing index: %w", err)
			return
		}

		if showSpinner {
			time.Sleep(800 * time.Millisecond)
		}
	}

	if showSpinner {
		_ = huhspinner.New().Title("Writing knowledge base...").Action(doWriteKB).Run()
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

	// ── Done ────────────────────────────────────────────────────────────────
	cmd.Println()
	cmd.Println("  ✔ .leo/ structure created")
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
	cmd.Println()
	cmd.Println("L.E.O. is ready. Run 'leo status' to check health.")
	return nil
}

// isTerminalWriter returns true if w is connected to a terminal.
func isTerminalWriter(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(f.Fd())
	}
	return false
}

// buildRuntimeConfig converts a config.Config to a runtime.Config.
func buildRuntimeConfig(cfg *config.Config) runtime.Config {
	commMode := cfg.Communication.Mode
	if commMode == "" {
		commMode = cfg.User.Mode // fallback to user.mode for pre-v0.8 configs
	}
	if commMode == "" {
		commMode = "concise"
	}
	return runtime.Config{
		Version: cfg.Version,
		User: runtime.UserConfig{
			Language:          cfg.User.Language,
			Mode:              cfg.User.Mode,
			Autonomy:          cfg.User.Autonomy,
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

// buildCoreIndex builds an index.json that includes all core constraint and skill docs.
func buildCoreIndex() ([]byte, error) {
	type IndexEntry struct {
		ID        string   `json:"id"`
		Type      string   `json:"type"`
		Boot      bool     `json:"boot"`
		Summary   string   `json:"summary"`
		Lifecycle string   `json:"lifecycle"`
		Scope     string   `json:"scope"`
		Tags      []string `json:"tags"`
		Path      string   `json:"path"`
	}

	type IndexStats struct {
		TotalDocs        int            `json:"total_docs"`
		TotalTags        int            `json:"total_tags"`
		DocsByType       map[string]int `json:"docs_by_type"`
		StaleCount       int            `json:"stale_count"`
		MostConnectedTag string         `json:"most_connected_tag"`
	}

	type Index struct {
		Version     string                `json:"version"`
		LastRebuilt string                `json:"last_rebuilt"`
		Stats       IndexStats            `json:"stats"`
		ByTag       map[string][]string   `json:"by_tag"`
		ByType      map[string][]string   `json:"by_type"`
		ByScope     map[string][]string   `json:"by_scope"`
		ByLifecycle map[string][]string   `json:"by_lifecycle"`
		Docs        map[string]IndexEntry `json:"docs"`
	}

	idx := Index{
		Version:     "1",
		LastRebuilt: "",
		Stats: IndexStats{
			TotalDocs:        0,
			TotalTags:        0,
			DocsByType:       map[string]int{},
			StaleCount:       0,
			MostConnectedTag: "",
		},
		ByTag:       map[string][]string{},
		ByType:      map[string][]string{},
		ByScope:     map[string][]string{},
		ByLifecycle: map[string][]string{},
		Docs:        map[string]IndexEntry{},
	}

	addDoc := func(rawJSON string, pathPrefix string) error {
		var doc struct {
			ID        string   `json:"id"`
			Type      string   `json:"type"`
			Boot      bool     `json:"boot"`
			Summary   string   `json:"summary"`
			Lifecycle string   `json:"lifecycle"`
			Scope     string   `json:"scope"`
			Tags      []string `json:"tags"`
		}
		if err := json.Unmarshal([]byte(rawJSON), &doc); err != nil {
			return err
		}

		entry := IndexEntry{
			ID:        doc.ID,
			Type:      doc.Type,
			Boot:      doc.Boot,
			Summary:   doc.Summary,
			Lifecycle: doc.Lifecycle,
			Scope:     doc.Scope,
			Tags:      doc.Tags,
			Path:      pathPrefix + doc.ID + ".json",
		}

		idx.Docs[doc.ID] = entry
		idx.Stats.TotalDocs++
		idx.Stats.DocsByType[doc.Type]++
		idx.ByType[doc.Type] = append(idx.ByType[doc.Type], doc.ID)
		idx.ByScope[doc.Scope] = append(idx.ByScope[doc.Scope], doc.ID)
		idx.ByLifecycle[doc.Lifecycle] = append(idx.ByLifecycle[doc.Lifecycle], doc.ID)

		for _, tag := range doc.Tags {
			idx.ByTag[tag] = append(idx.ByTag[tag], doc.ID)
		}
		return nil
	}

	for _, content := range coreConstraints() {
		if err := addDoc(content, "kb/constraints/"); err != nil {
			return nil, fmt.Errorf("indexing constraint: %w", err)
		}
	}

	for _, content := range coreSkills() {
		if err := addDoc(content, "kb/skills/"); err != nil {
			return nil, fmt.Errorf("indexing skill: %w", err)
		}
	}

	// Count unique tags.
	idx.Stats.TotalTags = len(idx.ByTag)

	// Find most connected tag.
	maxCount := 0
	for tag, ids := range idx.ByTag {
		if len(ids) > maxCount {
			maxCount = len(ids)
			idx.Stats.MostConnectedTag = tag
		}
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
