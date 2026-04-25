package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/momhq/mom/cli/internal/adapters/storage"
	"github.com/momhq/mom/cli/internal/scope"
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
		// Default: reindex only the nearest (writable) scope.
		targetScopes = scopes[:1]
	}

	for _, sc := range targetScopes {
		fmt.Fprintf(cmd.OutOrStdout(), "Reindexing %s (%s)...\n", sc.Label, sc.Path)

		momDir := sc.Path
		adapter := storage.NewIndexedAdapter(momDir)
		defer adapter.Close() //nolint:errcheck

		if err := adapter.Reindex(); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "! Failed to reindex %s: %v\n", sc.Label, err)
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

		fmt.Fprintf(cmd.OutOrStdout(), "✓ Reindexed %s: %d documents\n", sc.Label, count)
	}

	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "SQLite index rebuilt. Search results will now reflect all memory files.")
	return nil
}
