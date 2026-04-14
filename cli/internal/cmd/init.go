package cmd

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

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
		Language:       config.Default().Owner.Language,
		Mode:           config.Default().Owner.Mode,
		DefaultProfile: config.Default().Owner.DefaultProfile,
		Autonomy:       config.Default().Owner.Autonomy,
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
	cfg.Owner.Language = result.Language
	cfg.Owner.Mode = result.Mode
	cfg.Owner.DefaultProfile = result.DefaultProfile
	cfg.Owner.Autonomy = result.Autonomy

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

	// Write empty index.
	indexPath := filepath.Join(leoDir, "kb", "index.json")
	emptyIndex := []byte(`{
  "version": "1",
  "last_rebuilt": "",
  "stats": {"total_docs": 0, "total_tags": 0, "docs_by_type": {}, "stale_count": 0, "most_connected_tag": ""},
  "by_tag": {},
  "by_type": {},
  "by_scope": {},
  "by_lifecycle": {}
}
`)
	if err := os.WriteFile(indexPath, emptyIndex, 0644); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}
	cmd.Println("  ✔ .leo/kb/index.json")

	// Generate runtime context file.
	if result.Runtime == "claude" {
		adapter := runtime.NewClaudeAdapter(cwd)
		runtimeCfg := runtime.Config{
			Version: cfg.Version,
			Runtime: cfg.Runtime,
			Owner: runtime.OwnerConfig{
				Language:       cfg.Owner.Language,
				Mode:           cfg.Owner.Mode,
				Autonomy:       cfg.Owner.Autonomy,
				DefaultProfile: cfg.Owner.DefaultProfile,
			},
		}

		defaultProfile := profiles.DefaultProfiles()[cfg.Owner.DefaultProfile]
		runtimeProfile := runtime.Profile{
			Name:             defaultProfile.Name,
			Description:      defaultProfile.Description,
			Focus:            defaultProfile.Focus,
			Tone:             defaultProfile.Tone,
			ContextInjection: defaultProfile.ContextInjection,
		}

		if err := adapter.GenerateContextFile(runtimeCfg, runtimeProfile, nil); err != nil {
			return err
		}
		cmd.Println("  ✔ .claude/CLAUDE.md (generated)")
	}

	cmd.Println("\nLeo is ready. Run 'leo status' to check health.")
	return nil
}
