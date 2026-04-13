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

var readCmd = &cobra.Command{
	Use:   "read [id]",
	Short: "Read a KB document by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		adapter, err := newStorageAdapter()
		if err != nil {
			return err
		}

		doc, err := adapter.Read(args[0])
		if err != nil {
			return fmt.Errorf("reading %q: %w", args[0], err)
		}

		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}

		cmd.Println(string(data))
		return nil
	},
}

var writeCmd = &cobra.Command{
	Use:   "write [file]",
	Short: "Write a document to the KB",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		adapter, err := newStorageAdapter()
		if err != nil {
			return err
		}

		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		kbDoc, err := parseDoc(data)
		if err != nil {
			return err
		}

		if err := adapter.Write(kbDoc); err != nil {
			return err
		}

		cmd.Printf("✔ Written: %s (type: %s, tags: [%s])\n",
			kbDoc.ID, kbDoc.Type, strings.Join(kbDoc.Tags, ", "))
		return nil
	},
}

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query KB documents by tags, type, or scope",
	RunE: func(cmd *cobra.Command, args []string) error {
		adapter, err := newStorageAdapter()
		if err != nil {
			return err
		}

		tags, _ := cmd.Flags().GetStringSlice("tags")
		docType, _ := cmd.Flags().GetString("type")
		scope, _ := cmd.Flags().GetString("scope")
		lifecycle, _ := cmd.Flags().GetString("lifecycle")

		docs, err := adapter.Query(storageFilter(tags, docType, scope, lifecycle))
		if err != nil {
			return err
		}

		if len(docs) == 0 {
			cmd.Println("No documents found.")
			return nil
		}

		for _, doc := range docs {
			cmd.Printf("%-30s  type:%-10s  tags:[%s]\n",
				doc.ID, doc.Type, strings.Join(doc.Tags, ", "))
		}
		cmd.Printf("\n%d document(s) found.\n", len(docs))
		return nil
	},
}

func init() {
	queryCmd.Flags().StringSlice("tags", nil, "Filter by tags")
	queryCmd.Flags().String("type", "", "Filter by type")
	queryCmd.Flags().String("scope", "", "Filter by scope")
	queryCmd.Flags().String("lifecycle", "", "Filter by lifecycle")
}

var deleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a KB document by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		adapter, err := newStorageAdapter()
		if err != nil {
			return err
		}

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			cmd.Printf("Delete %q? This cannot be undone. Use --force to skip confirmation.\n", args[0])
			return nil
		}

		if err := adapter.Delete(args[0]); err != nil {
			return fmt.Errorf("deleting %q: %w", args[0], err)
		}

		cmd.Printf("✔ Deleted: %s\n", args[0])
		return nil
	},
}

func init() {
	deleteCmd.Flags().Bool("force", false, "Skip confirmation")
}

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

		// Force a full rebuild by writing an empty doc set via BulkWrite(nil)
		// Actually, just trigger rebuildIndex by doing a Read+Write cycle isn't clean.
		// Better: expose Reindex on the adapter. For now, delete and recreate index.
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

// parseDoc unmarshals JSON data into a storage-compatible doc.
func parseDoc(data []byte) (*storageDoc, error) {
	doc := &storageDoc{}
	if err := json.Unmarshal(data, doc); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return doc, nil
}
