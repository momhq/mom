package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show KB status summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("leo status: not yet implemented")
		return nil
	},
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check .leo/ health and diagnose issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("leo doctor: not yet implemented")
		return nil
	},
}

var exportCmd = &cobra.Command{
	Use:   "export [path]",
	Short: "Export KB to a portable format",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("leo export: not yet implemented")
		return nil
	},
}

var importCmd = &cobra.Command{
	Use:   "import [path]",
	Short: "Import KB from a portable format",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("leo import %s: not yet implemented\n", args[0])
		return nil
	},
}
