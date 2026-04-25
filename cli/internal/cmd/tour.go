package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/momhq/mom/cli/internal/adapters/storage"
	"github.com/momhq/mom/cli/internal/gardener"
	"github.com/momhq/mom/cli/internal/scope"
)

var tourCmd = &cobra.Command{
	Use:   "tour",
	Short: "Show top landmark memories at current scope",
	Long: `Display the top landmark memories — high-centrality docs that sit at
structural crossroads of the memory graph.

Landmarks are computed automatically during
'mom bootstrap' (when doc count >= 100).`,
	RunE: runTour,
}

func init() {
	tourCmd.Flags().String("scope", "", "Target scope label (repo/org/user)")
	tourCmd.Flags().Int("limit", 10, "Maximum landmarks to show")
	tourCmd.Flags().Bool("graph", false, "Generate interactive HTML graph and open in browser")
}

func runTour(cmd *cobra.Command, _ []string) error {
	scopeLabel, _ := cmd.Flags().GetString("scope")
	limit, _ := cmd.Flags().GetInt("limit")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	scopes := scope.Walk(cwd)
	if len(scopes) == 0 {
		cmd.Printf("No .mom/ directory found. Run 'mom init' first.\n")
		return nil
	}

	var targetScope scope.Scope
	graphMode, _ := cmd.Flags().GetBool("graph")
	if scopeLabel != "" {
		found := false
		for _, s := range scopes {
			if s.Label == scopeLabel {
				targetScope = s
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("no scope with label %q found", scopeLabel)
		}
	} else {
		targetScope = scopes[0]
	}

	if graphMode {
		return runTourGraph(cmd, targetScope)
	}

	// Use IndexedAdapter.ListLandmarks() for fast SQLite-backed landmark listing.
	idx := storage.NewIndexedAdapter(targetScope.Path)
	defer idx.Close()

	results, err := idx.ListLandmarks([]string{targetScope.Path}, limit)
	if err != nil {
		cmd.Printf("No landmarks found. Run 'mom bootstrap --path .' first.\n")
		return nil
	}

	if len(results) == 0 {
		cmd.Printf("No landmarks found. Run 'mom bootstrap --path .' first.\n")
		return nil
	}

	cmd.Printf("Landmarks for %s (%s)\n\n", targetScope.Label, shortenPath(targetScope.Path))
	for i, r := range results {
		score := 0.0
		if r.CentralityScore != nil {
			score = *r.CentralityScore
		}
		summary := r.Summary
		if summary == "" {
			summary = r.ID
		}

		cmd.Printf("%2d. %s\n", i+1, r.ID)
		cmd.Printf("    Scope:      %s\n", r.ScopePath)
		cmd.Printf("    Centrality: %.4f\n", score)
		cmd.Printf("    Tags:       %s\n", strings.Join(r.Tags, ", "))
		if summary != r.ID {
			cmd.Printf("    Summary:    %s\n", truncate(summary, 72))
		}
		cmd.Println()
	}

	return nil
}

func runTourGraph(cmd *cobra.Command, targetScope scope.Scope) error {
	memDir := filepath.Join(targetScope.Path, "memory")

	// Build graph data (max tag group size 50 to keep graph readable).
	data, err := gardener.BuildGraphData(memDir, 50)
	if err != nil {
		return fmt.Errorf("building graph data: %w", err)
	}

	if data.Stats.TotalDocs == 0 {
		cmd.Println("No memories found. Run 'mom bootstrap' first.")
		return nil
	}

	// Write HTML to a temp file.
	outPath := filepath.Join(os.TempDir(), "mom-memory-graph.html")
	if err := gardener.WriteGraphHTML(data, outPath); err != nil {
		return fmt.Errorf("writing graph HTML: %w", err)
	}

	cmd.Printf("Graph written to %s\n", outPath)
	cmd.Printf("  %d nodes, %d edges, %d landmarks\n", data.Stats.TotalDocs, data.Stats.TotalEdges, data.Stats.LandmarkCount)

	// Try to open in browser.
	if err := openBrowser(outPath); err != nil {
		cmd.Printf("  Open the file in your browser to view the graph.\n")
	}

	return nil
}

// openBrowser opens a URL in the default browser (cross-platform).
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	default: // linux and others
		return exec.Command("xdg-open", url).Start()
	}
}
