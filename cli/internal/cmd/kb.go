package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/kb"
)

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Rebuild the KB index from docs",
	RunE: func(cmd *cobra.Command, args []string) error {
		leoDir, err := findLeoDir()
		if err != nil {
			return err
		}

		// Read all docs, write them back to trigger index rebuild.
		docsDir := filepath.Join(leoDir, "kb", "docs")
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
		indexPath := filepath.Join(leoDir, "kb", "index.json")
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
		return nil
	},
}

var validateCmd = &cobra.Command{
	Use:   "validate [file]",
	Short: "Validate KB documents against the schema",
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

		doc := &kb.Doc{}
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
	validateCmd.Flags().Bool("all", false, "Validate all KB documents")
}

func validateAll(cmd *cobra.Command) error {
	leoDir, err := findLeoDir()
	if err != nil {
		return err
	}

	docsDir := filepath.Join(leoDir, "kb", "docs")
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
		doc, err := kb.LoadDoc(path)
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
