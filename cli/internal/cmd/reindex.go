package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/momhq/mom/cli/internal/adapters/storage"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/momhq/mom/cli/internal/ux"
	"github.com/spf13/cobra"
)

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Rebuild the SQLite search index from JSON memory files",
	Long: `Drops and rebuilds the SQLite FTS5 search index from JSON memory files.

The JSON files in .mom/memory/ are the source of truth. The SQLite index
at .mom/cache/index.db is a rebuildable cache. Running 'mom reindex' is
safe and will not affect your memory documents.

Run this command after:
  - Manually editing or deleting memory JSON files
  - Restoring from a backup
  - Upgrading MOM (done automatically by 'mom upgrade')
  - Diagnosing inconsistent search results`,
	RunE: runReindex,
}

func init() {
	reindexCmd.Flags().Bool("all", false, "Reindex all scopes in the hierarchy (repo → org → user)")
}

func runReindex(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool("all")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	scopes := scope.Walk(cwd)
	if len(scopes) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No .mom/ directory found. Run 'mom init' first.")
		return nil
	}

	targetScopes := scopes
	if !all {
		targetScopes = scopes[:1]
	}

	p := ux.NewPrinter(cmd.OutOrStdout())
	showSpinner := ux.IsTTY(cmd.OutOrStdout())

	p.Diamond("reindex")
	p.Blank()

	for _, sc := range targetScopes {
		momDir := sc.Path
		adapter := storage.NewIndexedAdapter(momDir)
		defer adapter.Close() //nolint:errcheck

		var reindexErr error
		doReindex := func() {
			reindexErr = adapter.Reindex()
		}

		if showSpinner {
			sp := ux.NewSpinner(os.Stderr)
			sp.Start(fmt.Sprintf("Reindexing %s", sc.Label))
			doReindex()
			if reindexErr != nil {
				sp.StopFail()
			} else {
				sp.Stop()
			}
		} else {
			doReindex()
		}

		if reindexErr != nil {
			p.Failf("%s: %v", sc.Label, reindexErr)
			continue
		}

		// Count the docs indexed.
		memDir := filepath.Join(momDir, "memory")
		entries, _ := os.ReadDir(memDir)
		var count int
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
				count++
			}
		}

		p.Checkf("%s: %s documents indexed", sc.Label, p.HighlightValue(fmt.Sprintf("%d", count)))
		p.Chevron(shortenPath(momDir))
	}

	p.Blank()
	p.Muted("SQLite index rebuilt. Search results now reflect all memory files.")
	return nil
}
