package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/momhq/mom/cli/internal/gardener"
	"github.com/momhq/mom/cli/internal/memory"
)

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Rebuild the memory index from docs",
	RunE: func(cmd *cobra.Command, args []string) error {
		landmarksFlag, _ := cmd.Flags().GetBool("landmarks")

		leoDir, err := findMomDir()
		if err != nil {
			return err
		}

		// Read all docs, write them back to trigger index rebuild.
		docsDir := filepath.Join(leoDir, "memory")
		entries, err := os.ReadDir(docsDir)
		if err != nil {
			return fmt.Errorf("reading docs dir: %w", err)
		}

		adapter, _ := newStorageAdapter()
		var count int
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			count++
		}

		// Force a full rebuild by deleting and recreating the index.
		indexPath := filepath.Join(leoDir, "index.json")
		os.Remove(indexPath)

		// Read all docs and bulk-write to rebuild.
		var docs []*storageDoc
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			doc, err := adapter.Read(strings.TrimSuffix(e.Name(), ".json"))
			if err != nil {
				cmd.Printf("  ⚠ skipping %s: %v\n", e.Name(), err)
				continue
			}
			docs = append(docs, doc)
		}

		if len(docs) > 0 {
			if err := adapter.BulkWrite(docs); err != nil {
				return fmt.Errorf("rebuilding index: %w", err)
			}
		}

		cmd.Printf("✔ Index rebuilt: %d documents indexed.\n", count)

		if landmarksFlag {
			if count < gardener.MinDocsForLandmarks {
				cmd.Printf("⚠ Landmark computation skipped: %d/%d memories (below threshold).\n",
					count, gardener.MinDocsForLandmarks)
			} else {
				n, err := gardener.ComputeLandmarks(docsDir, 2.0)
				if err != nil {
					cmd.Printf("⚠ Landmark computation failed: %v\n", err)
				} else {
					landmarkCount := countLandmarks(docsDir)
					cmd.Printf("✔ Computed landmarks: %d docs marked as landmarks out of %d total.\n",
						landmarkCount, count)
					_ = n
				}
			}
		}

		return nil
	},
}

var validateCmd = &cobra.Command{
	Use:   "validate [file]",
	Short: "Validate memory documents against the schema",
	RunE: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")

		if all {
			return validateAll(cmd)
		}

		if len(args) == 0 {
			return fmt.Errorf("provide a file path or use --all")
		}

		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		doc := &memory.Doc{}
		if err := json.Unmarshal(data, doc); err != nil {
			return fmt.Errorf("parsing JSON: %w", err)
		}

		if err := doc.Validate(); err != nil {
			cmd.Printf("✗ %s: %v\n", args[0], err)
			return err
		}

		cmd.Printf("✔ %s: valid\n", args[0])
		return nil
	},
}

func init() {
	reindexCmd.Flags().Bool("landmarks", false, "Compute landmark centrality scores after reindexing")
	validateCmd.Flags().Bool("all", false, "Validate all memory documents")
}

// countLandmarks returns the number of docs with landmark=true in memDir.
func countLandmarks(memDir string) int {
	entries, _ := os.ReadDir(memDir)
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		doc, err := memory.LoadDoc(filepath.Join(memDir, e.Name()))
		if err != nil {
			continue
		}
		if doc.Landmark {
			n++
		}
	}
	return n
}

func validateAll(cmd *cobra.Command) error {
	leoDir, err := findMomDir()
	if err != nil {
		return err
	}

	docsDir := filepath.Join(leoDir, "memory")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		return fmt.Errorf("reading docs dir: %w", err)
	}

	var errors int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		path := filepath.Join(docsDir, e.Name())
		doc, err := memory.LoadDoc(path)
		if err != nil {
			cmd.Printf("✗ %s: %v\n", e.Name(), err)
			errors++
			continue
		}

		if err := doc.Validate(); err != nil {
			cmd.Printf("✗ %s: %v\n", e.Name(), err)
			errors++
			continue
		}

		cmd.Printf("✔ %s\n", e.Name())
	}

	if errors > 0 {
		return fmt.Errorf("%d document(s) failed validation", errors)
	}

	cmd.Println("\nAll documents valid.")
	return nil
}
