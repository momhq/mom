package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mom",
	Short: "MOM — Memory Oriented Machine",
	Long:  "A living knowledge infrastructure where humans and agents think, decide, and evolve together.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if warning := checkVersionCache(); warning != "" {
			fmt.Fprintln(os.Stderr, warning)
			fmt.Fprintln(os.Stderr)
		}
		refreshVersionCacheAsync()
	},
}

func Execute() error {
	// Hide cobra's auto-generated completion command.
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	return rootCmd.Execute()
}

func init() {
	// Enable the bare `mom --version` flag alongside the `version` subcommand.
	rootCmd.Version = versionString()
	rootCmd.SetVersionTemplate("mom {{.Version}}\n")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(ingestCmd)
	rootCmd.AddCommand(demoCmd)
	rootCmd.AddCommand(lensCmd)
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(vaultCmd)
}
