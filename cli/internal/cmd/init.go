package cmd

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/adapters/runtime"
	"github.com/vmarinogg/leo-core/cli/internal/config"
	"github.com/vmarinogg/leo-core/cli/internal/profiles"
)

//go:embed schema.json
var embeddedSchema embed.FS

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a .leo/ directory in the current project",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().String("runtime", "claude", "AI runtime to configure (claude, cursor, windsurf)")
	initCmd.Flags().Bool("force", false, "Overwrite existing .leo/ directory")
	initCmd.Flags().BoolP("no-interactive", "y", false, "Skip the interactive wizard and use defaults/flags")
}

func runInit(cmd *cobra.Command, args []string) error {
	rt, _ := cmd.Flags().GetString("runtime")
	force, _ := cmd.Flags().GetBool("force")
	noInteractive, _ := cmd.Flags().GetBool("no-interactive")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Run the interactive onboarding wizard unless:
	//   • --no-interactive / -y was passed, OR
	//   • --runtime was explicitly provided by the user (direct/scripted mode).
	if !noInteractive && !cmd.Flags().Changed("runtime") {
		result, err := runOnboarding(cmd.InOrStdin(), cmd.OutOrStdout(), cwd)
		if err != nil {
			return err
		}
		return runInitWithConfig(cmd, cwd, force, result)
	}

	// Non-interactive path: use flags/defaults.
	return runInitWithConfig(cmd, cwd, force, OnboardingResult{
		Runtime:        rt,
		Language:       config.Default().User.Language,
		Mode:           config.Default().User.Mode,
		DefaultProfile: config.Default().User.DefaultProfile,
		Autonomy:       config.Default().User.Autonomy,
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

	cmd.Println("Creating .leo/ structure...")

	// Create directories.
	dirs := []string{
		leoDir,
		filepath.Join(leoDir, "profiles"),
		filepath.Join(leoDir, "kb", "docs"),
		filepath.Join(leoDir, "kb", "skills"),
		filepath.Join(leoDir, "kb", "constraints"),
		filepath.Join(leoDir, "cache"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}

	// Write config.yaml — start from defaults and apply wizard/flag choices.
	cfg := config.Default()
	cfg.Runtime = result.Runtime
	cfg.CoreSource = result.CoreSource
	cfg.User.Language = result.Language
	cfg.User.Mode = result.Mode
	cfg.User.DefaultProfile = result.DefaultProfile
	cfg.User.Autonomy = result.Autonomy

	if err := config.Save(leoDir, &cfg); err != nil {
		return err
	}
	cmd.Println("  ✔ .leo/config.yaml")

	// Write default profiles.
	profilesDir := filepath.Join(leoDir, "profiles")
	for name, p := range profiles.DefaultProfiles() {
		if err := profiles.Save(profilesDir, name, p); err != nil {
			return err
		}
		cmd.Printf("  ✔ .leo/profiles/%s.yaml\n", name)
	}

	// Write schema.json.
	schemaData, err := embeddedSchema.ReadFile("schema.json")
	if err != nil {
		return fmt.Errorf("reading embedded schema: %w", err)
	}
	schemaPath := filepath.Join(leoDir, "kb", "schema.json")
	if err := os.WriteFile(schemaPath, schemaData, 0644); err != nil {
		return fmt.Errorf("writing schema: %w", err)
	}
	cmd.Println("  ✔ .leo/kb/schema.json")

	// Write identity.json.
	identityPath := filepath.Join(leoDir, "identity.json")
	if err := os.WriteFile(identityPath, []byte(defaultIdentity()), 0644); err != nil {
		return fmt.Errorf("writing identity.json: %w", err)
	}
	cmd.Println("  ✔ .leo/identity.json")

	// Write core constraints.
	constraintsDir := filepath.Join(leoDir, "kb", "constraints")
	for name, content := range coreConstraints() {
		path := filepath.Join(constraintsDir, name+".json")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing constraint %s: %w", name, err)
		}
		cmd.Printf("  ✔ .leo/kb/constraints/%s.json\n", name)
	}

	// Write core skills.
	skillsDir := filepath.Join(leoDir, "kb", "skills")
	for name, content := range coreSkills() {
		path := filepath.Join(skillsDir, name+".json")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing skill %s: %w", name, err)
		}
		cmd.Printf("  ✔ .leo/kb/skills/%s.json\n", name)
	}

	// Write index.json with entries for all core docs.
	indexPath := filepath.Join(leoDir, "kb", "index.json")
	indexData, err := buildCoreIndex()
	if err != nil {
		return fmt.Errorf("building index: %w", err)
	}
	if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}
	cmd.Println("  ✔ .leo/kb/index.json")

	// Generate runtime context file.
	if result.Runtime == "claude" {
		adapter := runtime.NewClaudeAdapter(cwd)
		runtimeCfg := runtime.Config{
			Version: cfg.Version,
			Runtime: cfg.Runtime,
			User: runtime.UserConfig{
				Language:       cfg.User.Language,
				Mode:           cfg.User.Mode,
				Autonomy:       cfg.User.Autonomy,
				DefaultProfile: cfg.User.DefaultProfile,
			},
		}

		defaultProfile := profiles.DefaultProfiles()[cfg.User.DefaultProfile]
		runtimeProfile := runtime.Profile{
			Name:             defaultProfile.Name,
			Description:      defaultProfile.Description,
			Focus:            defaultProfile.Focus,
			Tone:             defaultProfile.Tone,
			ContextInjection: defaultProfile.ContextInjection,
		}

		// Build constraints list from core constraints.
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
		// Sort for deterministic output.
		sort.Slice(runtimeConstraints, func(i, j int) bool {
			return runtimeConstraints[i].ID < runtimeConstraints[j].ID
		})

		// Build skills list from core skills.
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

		// Parse identity.
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

		if err := adapter.GenerateContextFile(runtimeCfg, runtimeProfile, runtimeConstraints, runtimeSkills, runtimeIdentity); err != nil {
			return err
		}
		cmd.Println("  ✔ .claude/CLAUDE.md (generated)")
	}

	cmd.Println("\nLeo is ready. Run 'leo status' to check health.")
	return nil
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
		Version     string                 `json:"version"`
		LastRebuilt string                 `json:"last_rebuilt"`
		Stats       IndexStats             `json:"stats"`
		ByTag       map[string][]string    `json:"by_tag"`
		ByType      map[string][]string    `json:"by_type"`
		ByScope     map[string][]string    `json:"by_scope"`
		ByLifecycle map[string][]string    `json:"by_lifecycle"`
		Docs        map[string]IndexEntry  `json:"docs"`
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
