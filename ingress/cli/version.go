package cli

import (
	"fmt"

	"github.com/momhq/mom/shared/buildinfo"
	"github.com/momhq/mom/shared/ux"
	"github.com/spf13/cobra"
)

// shortCommit returns the commit truncated to its short form.
func shortCommit() string {
	if len(buildinfo.Commit) > 7 {
		return buildinfo.Commit[:7]
	}
	return buildinfo.Commit
}

// versionString is the plain "Version (commit)" form used by the bare
// `--version` flag (see root.go).
func versionString() string {
	return fmt.Sprintf("%s (%s)", buildinfo.Version, shortCommit())
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the MOM CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		p := ux.NewPrinter(cmd.OutOrStdout())
		p.Text(fmt.Sprintf("mom %s (%s)", p.HighlightValue(buildinfo.Version), p.MutedText(shortCommit())))
	},
}
