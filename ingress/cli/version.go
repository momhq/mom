package cli

import (
	"fmt"

	"github.com/momhq/mom/shared/ux"
	"github.com/spf13/cobra"
)

// Set via ldflags at build time.
var (
	Version = "dev"
	Commit  = "none"
)

// shortCommit returns the commit truncated to its short form.
func shortCommit() string {
	if len(Commit) > 7 {
		return Commit[:7]
	}
	return Commit
}

// versionString is the plain "Version (commit)" form used by the bare
// `--version` flag (see root.go).
func versionString() string {
	return fmt.Sprintf("%s (%s)", Version, shortCommit())
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the MOM CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		p := ux.NewPrinter(cmd.OutOrStdout())
		p.Text(fmt.Sprintf("mom %s (%s)", p.HighlightValue(Version), p.MutedText(shortCommit())))
	},
}
