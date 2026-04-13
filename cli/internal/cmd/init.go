package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a .leo/ directory in the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("leo init: not yet implemented")
		return nil
	},
}

func init() {
	initCmd.Flags().String("runtime", "claude", "AI runtime to configure (claude, cursor, windsurf)")
}
