package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/momhq/mom/cli/internal/gardener"
	"github.com/momhq/mom/cli/internal/memory"
	"github.com/momhq/mom/cli/internal/scope"
	"github.com/momhq/mom/cli/internal/ux"
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

	p := ux.NewPrinter(cmd.OutOrStdout())

	scopes := scope.Walk(cwd)
	if len(scopes) == 0 {
		p.Muted("No .mom/ directory found. Run 'mom init' first.")
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
		p.Muted("No landmarks found. Run 'mom bootstrap --path .' first.")
		return nil
	}

	type landmarkEntry struct {
		doc   *memory.Doc
		score float64
	}

	var landmarks []landmarkEntry
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		doc, err := memory.LoadDoc(filepath.Join(memDir, e.Name()))
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
		p.Muted("No landmarks found. Run 'mom bootstrap --path .' first.")
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

	p.Bold(fmt.Sprintf("Landmarks for %s (%s)", targetScope.Label, shortenPath(targetScope.Path)))
	p.Blank()
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

		p.Diamond(fmt.Sprintf("%2d. %s", i+1, doc.ID))
		w := 14
		p.KeyValue("    Scope", doc.Scope, w)
		p.KeyValue("    Centrality", fmt.Sprintf("%.4f", lm.score), w)
		p.KeyValue("    Tags", strings.Join(doc.Tags, ", "), w)
		if summary != doc.ID {
			p.KeyValue("    Summary", truncate(summary, 72), w)
		}
		p.Blank()
	}

	return nil
}

func runTourGraph(cmd *cobra.Command, targetScope scope.Scope) error {
	gp := ux.NewPrinter(cmd.OutOrStdout())
	memDir := filepath.Join(targetScope.Path, "memory")

	// Build graph data (max tag group size 50 to keep graph readable).
	data, err := gardener.BuildGraphData(memDir, 50)
	if err != nil {
		return fmt.Errorf("building graph data: %w", err)
	}

	if data.Stats.TotalDocs == 0 {
		gp.Muted("No memories found. Run 'mom bootstrap' first.")
		return nil
	}

	// Write HTML to a temp file.
	outPath := filepath.Join(os.TempDir(), "mom-memory-graph.html")
	if err := gardener.WriteGraphHTML(data, outPath); err != nil {
		return fmt.Errorf("writing graph HTML: %w", err)
	}

	gp.Checkf("Graph written to %s", outPath)
	gp.Muted(fmt.Sprintf("  %d nodes, %d edges, %d landmarks", data.Stats.TotalDocs, data.Stats.TotalEdges, data.Stats.LandmarkCount))

	// Try to open in browser.
	if err := openBrowser(outPath); err != nil {
		gp.Muted("  Open the file in your browser to view the graph.")
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
