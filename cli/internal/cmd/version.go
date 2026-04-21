package cmd

import (
	"github.com/spf13/cobra"
)

// Set via ldflags at build time.
var (
	Version = "dev"
	Commit  = "none"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the MOM CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Printf("leo %s (%s)\n", Version, Commit)
	},
}
