package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "leo",
	Short: "LEO — Living Ecosystem Orchestrator",
	Long:  "A living knowledge infrastructure where humans and agents think, decide, and evolve together.",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(readCmd)
	rootCmd.AddCommand(writeCmd)
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(profileCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(reindexCmd)
	rootCmd.AddCommand(validateCmd)
}
