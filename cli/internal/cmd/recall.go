package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/momhq/mom/cli/internal/memory"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/momhq/mom/cli/internal/ux"
)

const (
	recallLandmarkBoost = 0.3
	recallDefaultLimit  = 10
)

var recallCmd = &cobra.Command{
	Use:   "recall <query>",
	Short: "Search memories by tag match and content substring",
	Long: `Search memories by tag match and content substring.

Results are ranked by:
  - Tag match count
  - Substring match in summary/content
  - Landmark boost (+0.3) for landmark memories

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

type recallResult struct {
	doc   *memory.Doc
	score float64
}

func runRecall(cmd *cobra.Command, args []string) error {
	query := ""
	if len(args) > 0 {
		query = strings.ToLower(strings.TrimSpace(args[0]))
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

	// Filter by scope label if specified.
	var targetScopes []scope.Scope
	if scopeLabel != "" {
		for _, s := range scopes {
			if s.Label == scopeLabel {
				targetScopes = append(targetScopes, s)
				break
			}
		}
		if len(targetScopes) == 0 {
			return fmt.Errorf("no scope with label %q found", scopeLabel)
		}
	} else {
		targetScopes = scopes
	}

	// Build filter tag set for O(1) lookup.
	filterTagSet := make(map[string]bool, len(filterTags))
	for _, t := range filterTags {
		filterTagSet[strings.TrimSpace(t)] = true
	}

	var results []recallResult

	for _, s := range targetScopes {
		memDir := filepath.Join(s.Path, "memory")
		entries, err := os.ReadDir(memDir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			doc, err := memory.LoadDoc(filepath.Join(memDir, e.Name()))
			if err != nil {
				continue
			}

			score := scoreRecall(doc, query, filterTagSet)
			if score <= 0 {
				continue
			}

			results = append(results, recallResult{doc: doc, score: score})
		}
	}

	if len(results) == 0 {
		if query == "" && len(filterTags) == 0 {
			p.Muted("No memories found.")
		} else {
			p.Muted("No memories matched your query.")
		}
		return nil
	}

	// Sort by score descending, then by ID for deterministic output.
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].doc.ID < results[j].doc.ID
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	p.Bold(fmt.Sprintf("%-36s  %-10s  %s", "ID", "Score", "Summary"))
	p.Text(strings.Repeat("-", 80))
	for _, r := range results {
		landmark := ""
		if r.doc.Landmark {
			landmark = p.HighlightValue(" ★")
		}
		summary := r.doc.Summary
		if summary == "" {
			if s, ok := r.doc.Content["summary"].(string); ok {
				summary = s
			}
		}
		p.Textf("%-36s  %s  %s%s",
			truncate(r.doc.ID, 36),
			p.HighlightValue(fmt.Sprintf("%-10.3f", r.score)),
			truncate(summary, 40),
			landmark,
		)
	}

	return nil
}

// scoreRecall computes a relevance score for a doc given a query and tag filters.
// Returns 0 if tag filters are set but not matched (exclude from results).
func scoreRecall(doc *memory.Doc, query string, filterTags map[string]bool) float64 {
	// If tag filter specified, doc must match ALL filter tags.
	if len(filterTags) > 0 {
		docTagSet := make(map[string]bool, len(doc.Tags))
		for _, t := range doc.Tags {
			docTagSet[t] = true
		}
		for tag := range filterTags {
			if !docTagSet[tag] {
				return 0
			}
		}
	}

	var score float64

	// Tag match count (query substring match against doc tags).
	if query != "" {
		for _, tag := range doc.Tags {
			if strings.Contains(strings.ToLower(tag), query) {
				score += 1.0
			}
		}
	}

	// Substring match in summary and content.
	if query != "" {
		summary := strings.ToLower(doc.Summary)
		if summary == "" {
			if s, ok := doc.Content["summary"].(string); ok {
				summary = strings.ToLower(s)
			}
		}
		if strings.Contains(summary, query) {
			score += 1.5
		}

		// Search content values.
		for _, v := range doc.Content {
			if s, ok := v.(string); ok {
				if strings.Contains(strings.ToLower(s), query) {
					score += 0.5
					break
				}
			}
		}
	}

	// If no query and no filter tags, return a base score so all docs surface.
	if query == "" && len(filterTags) == 0 {
		score = 0.1
	}

	// Apply landmark boost.
	if doc.Landmark {
		score += recallLandmarkBoost
	}

	// If tag filter matched but no query was given, show all matching docs.
	if query == "" && len(filterTags) > 0 && score == recallLandmarkBoost {
		score = 1.0 + recallLandmarkBoost
	} else if query == "" && len(filterTags) > 0 {
		score = 1.0
	}

	// A doc with filter tags set but no query match still surfaces (with base score).
	if score == 0 && len(filterTags) > 0 {
		return 0 // tag filter required and doc tags matched but no query match → still include
	}
	if score == 0 && query != "" {
		return 0 // query set but no match
	}

	return score
}
