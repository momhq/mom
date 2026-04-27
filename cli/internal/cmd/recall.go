package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/momhq/mom/cli/internal/adapters/storage"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/momhq/mom/cli/internal/ux"
)

const recallDefaultLimit = 10

var recallCmd = &cobra.Command{
	Use:   "recall <query>",
	Short: "Search memories by tag match and content substring",
	Long: `Search memories by tag match and content substring.

Results are ranked by FTS5 BM25 scoring with landmark boost.

Examples:
  mom recall "authentication"
  mom recall "api" --tags auth,security --limit 5
  mom recall "" --scope repo`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRecall,
}

func init() {
	recallCmd.Flags().StringSlice("tags", nil, "Filter by tags (comma-separated)")
	recallCmd.Flags().String("scope", "", "Restrict to a specific scope (repo/org/user)")
	recallCmd.Flags().Int("limit", recallDefaultLimit, "Maximum results to return")
}

func runRecall(cmd *cobra.Command, args []string) error {
	query := ""
	if len(args) > 0 {
		query = strings.TrimSpace(args[0])
	}

	filterTags, _ := cmd.Flags().GetStringSlice("tags")
	scopeLabel, _ := cmd.Flags().GetString("scope")
	limit, _ := cmd.Flags().GetInt("limit")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	p := ux.NewPrinter(cmd.OutOrStdout())

	scopes := scope.Walk(cwd)
	if len(scopes) == 0 {
		p.Muted("No .mom/ directory found. Run 'mom init' first.")
		return nil
	}

	// Collect scope paths, filtering by label if specified.
	var scopePaths []string
	for _, s := range scopes {
		if scopeLabel == "" || s.Label == scopeLabel {
			scopePaths = append(scopePaths, s.Path)
		}
	}
	if scopeLabel != "" && len(scopePaths) == 0 {
		return fmt.Errorf("no scope with label %q found", scopeLabel)
	}

	// Use IndexedAdapter from the nearest writable scope.
	momDir := scopes[0].Path
	idx := storage.NewIndexedAdapter(momDir)
	defer idx.Close()

	showSpinner := ux.IsTTY(cmd.OutOrStdout())

	var results []storage.SearchResult
	var searchErr error
	doSearch := func() {
		results, searchErr = idx.Search(storage.SearchOptions{
			Query:      query,
			ScopePaths: scopePaths,
			Tags:       filterTags,
			Limit:      limit,
		})
	}

	if showSpinner {
		sp := ux.NewSpinner(os.Stderr)
		sp.Start("Searching")
		doSearch()
		sp.Stop()
	} else {
		doSearch()
	}
	if searchErr != nil {
		return fmt.Errorf("search failed: %w", searchErr)
	}

	if len(results) == 0 {
		if query == "" && len(filterTags) == 0 {
			p.Muted("No memories found.")
		} else {
			p.Muted("No memories matched your query.")
		}
		return nil
	}

	title := "recall"
	if query != "" {
		title = fmt.Sprintf("recall %q", query)
	}
	p.Diamond(fmt.Sprintf("%s — %d results", title, len(results)))
	p.Blank()

	p.Bold(fmt.Sprintf("%-36s  %-10s  %s", "ID", "Score", "Summary"))
	p.Muted(strings.Repeat("─", 80))
	for _, r := range results {
		landmark := ""
		if r.Landmark {
			landmark = p.HighlightValue(" ★")
		}
		p.Textf("%-36s  %s  %s%s",
			truncate(r.ID, 36),
			p.HighlightValue(fmt.Sprintf("%-10.3f", r.Score)),
			truncate(r.Summary, 40),
			landmark,
		)
	}

	return nil
}
