package cmd

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
		}
		refreshVersionCacheAsync()
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(reindexCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(promoteCmd)
	rootCmd.AddCommand(demoteCmd)
	rootCmd.AddCommand(bootstrapCmd)
	rootCmd.AddCommand(recallCmd)
	rootCmd.AddCommand(tourCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(recordCmd)
}
