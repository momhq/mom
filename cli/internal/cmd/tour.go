package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/kb"
	"github.com/vmarinogg/leo-core/cli/internal/scope"
)

var tourCmd = &cobra.Command{
	Use:   "tour",
	Short: "Show top landmark memories at current scope",
	Long: `Display the top landmark memories — high-centrality docs that sit at
structural crossroads of the knowledge base.

Landmarks are computed by 'leo reindex --landmarks' or automatically during
'leo bootstrap' (when doc count >= 100).`,
	RunE: runTour,
}

func init() {
	tourCmd.Flags().String("scope", "", "Target scope label (repo/org/user)")
	tourCmd.Flags().Int("limit", 10, "Maximum landmarks to show")
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
		cmd.Printf("No .leo/ directory found. Run 'leo init' first.\n")
		return nil
	}

	var targetScope scope.Scope
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

	memDir := filepath.Join(targetScope.Path, "memory")
	entries, err := os.ReadDir(memDir)
	if err != nil {
		cmd.Printf("No landmarks found. Run 'leo reindex --landmarks' first.\n")
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
		cmd.Printf("No landmarks found. Run 'leo reindex --landmarks' first.\n")
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
