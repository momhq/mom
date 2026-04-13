package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage specialist profiles",
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("leo profile list: not yet implemented")
		return nil
	},
}

var profileShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show profile details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("leo profile show %s: not yet implemented\n", args[0])
		return nil
	},
}

var profileUseCmd = &cobra.Command{
	Use:   "use [name]",
	Short: "Set the default profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("leo profile use %s: not yet implemented\n", args[0])
		return nil
	},
}

var profileCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new profile interactively",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("leo profile create %s: not yet implemented\n", args[0])
		return nil
	},
}

func init() {
	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileShowCmd)
	profileCmd.AddCommand(profileUseCmd)
	profileCmd.AddCommand(profileCreateCmd)
}
