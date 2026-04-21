package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/gardener"
	"github.com/vmarinogg/leo-core/cli/internal/kb"
	"github.com/vmarinogg/leo-core/cli/internal/scope"
)

var tourCmd = &cobra.Command{
	Use:   "tour",
	Short: "Show top landmark memories at current scope",
	Long: `Display the top landmark memories — high-centrality docs that sit at
structural crossroads of the memory graph.

Landmarks are computed by 'mom reindex --landmarks' or automatically during
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

	memDir := filepath.Join(targetScope.Path, "memory")
	entries, err := os.ReadDir(memDir)
	if err != nil {
		cmd.Printf("No landmarks found. Run 'mom reindex --landmarks' first.\n")
		return nil
	}

	type landmarkEntry struct {
		doc   *kb.Doc
		score float64
	}

	var landmarks []landmarkEntry
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		doc, err := kb.LoadDoc(filepath.Join(memDir, e.Name()))
		if err != nil {
			continue
		}
		if !doc.Landmark {
			continue
		}
		score := 0.0
		if doc.CentralityScore != nil {
			score = *doc.CentralityScore
		}
		landmarks = append(landmarks, landmarkEntry{doc: doc, score: score})
	}

	if len(landmarks) == 0 {
		cmd.Printf("No landmarks found. Run 'mom reindex --landmarks' first.\n")
		return nil
	}

	sort.Slice(landmarks, func(i, j int) bool {
		if landmarks[i].score != landmarks[j].score {
			return landmarks[i].score > landmarks[j].score
		}
		return landmarks[i].doc.ID < landmarks[j].doc.ID
	})

	if limit > 0 && len(landmarks) > limit {
		landmarks = landmarks[:limit]
	}

	cmd.Printf("Landmarks for %s (%s)\n\n", targetScope.Label, shortenPath(targetScope.Path))
	for i, lm := range landmarks {
		doc := lm.doc
		summary := doc.Summary
		if summary == "" {
			if s, ok := doc.Content["summary"].(string); ok {
				summary = s
			}
		}
		if summary == "" {
			summary = doc.ID
		}

		cmd.Printf("%2d. %s\n", i+1, doc.ID)
		cmd.Printf("    Type:       %s\n", doc.Type)
		cmd.Printf("    Centrality: %.4f\n", lm.score)
		cmd.Printf("    Tags:       %s\n", strings.Join(doc.Tags, ", "))
		if summary != doc.ID {
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
	outPath := filepath.Join(os.TempDir(), "leo-memory-graph.html")
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

// openBrowser opens a URL in the default browser.
func openBrowser(url string) error {
	return exec.Command("open", url).Start()
}
