package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/kb"
)

var exportCmd = &cobra.Command{
	Use:   "export [path]",
	Short: "Export KB to a portable format",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runExport,
}

var importCmd = &cobra.Command{
	Use:   "import [path]",
	Short: "Import KB from a portable format",
	Args:  cobra.ExactArgs(1),
	RunE:  runImport,
}

func init() {
	exportCmd.Flags().String("output", "", "Output directory path (default: ./leo-export)")
	importCmd.Flags().Bool("merge", false, "Merge: keep existing docs, add new, skip conflicts (default)")
	importCmd.Flags().Bool("replace", false, "Replace: back up current KB, then replace entirely")
}

// runExport implements the leo export command.
func runExport(cmd *cobra.Command, args []string) error {
	leoDir, err := findLeoDir()
	if err != nil {
		return err
	}

	// Determine output directory.
	outputFlag, _ := cmd.Flags().GetString("output")
	var outputDir string
	if outputFlag != "" {
		outputDir = outputFlag
	} else if len(args) > 0 {
		outputDir = filepath.Join(args[0], "leo-export")
	} else {
		// Default: ./leo-export relative to cwd.
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting cwd: %w", err)
		}
		outputDir = filepath.Join(cwd, "leo-export")
	}

	docsOutputDir := filepath.Join(outputDir, "docs")
	if err := os.MkdirAll(docsOutputDir, 0755); err != nil {
		return fmt.Errorf("creating output docs dir: %w", err)
	}

	// Copy all docs.
	srcDocsDir := filepath.Join(leoDir, "kb", "docs")
	entries, err := os.ReadDir(srcDocsDir)
	if err != nil {
		return fmt.Errorf("reading docs dir: %w", err)
	}

	var count int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		src := filepath.Join(srcDocsDir, e.Name())
		dst := filepath.Join(docsOutputDir, e.Name())
		if err := copyFileContents(src, dst); err != nil {
			return fmt.Errorf("copying doc %s: %w", e.Name(), err)
		}
		count++
	}

	// Copy index.json.
	srcIndex := filepath.Join(leoDir, "kb", "index.json")
	dstIndex := filepath.Join(outputDir, "index.json")
	if err := copyFileContents(srcIndex, dstIndex); err != nil {
		return fmt.Errorf("copying index.json: %w", err)
	}

	// Copy schema.json if it exists.
	srcSchema := filepath.Join(leoDir, "kb", "schema.json")
	if _, err := os.Stat(srcSchema); err == nil {
		dstSchema := filepath.Join(outputDir, "schema.json")
		if err := copyFileContents(srcSchema, dstSchema); err != nil {
			return fmt.Errorf("copying schema.json: %w", err)
		}
	}

	cmd.Printf("Exported %d document(s) to %s\n", count, outputDir)
	return nil
}

// runImport implements the leo import command.
func runImport(cmd *cobra.Command, args []string) error {
	importPath := args[0]

	replaceMode, _ := cmd.Flags().GetBool("replace")
	// mergeMode is the default; --merge flag is explicit but same behavior.
	// replaceMode takes priority if set.

	leoDir, err := findLeoDir()
	if err != nil {
		return err
	}

	srcDocsDir := filepath.Join(importPath, "docs")
	if _, err := os.Stat(srcDocsDir); err != nil {
		return fmt.Errorf("import path must contain a docs/ directory: %w", err)
	}

	destDocsDir := filepath.Join(leoDir, "kb", "docs")

	if replaceMode {
		// Back up current KB to .leo/kb/backup-{timestamp}/.
		timestamp := time.Now().UTC().Format("20060102-150405")
		backupDir := filepath.Join(leoDir, "kb", "backup-"+timestamp)
		backupDocsDir := filepath.Join(backupDir, "docs")
		if err := os.MkdirAll(backupDocsDir, 0755); err != nil {
			return fmt.Errorf("creating backup dir: %w", err)
		}

		// Copy all existing docs to backup.
		existingEntries, err := os.ReadDir(destDocsDir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("reading existing docs: %w", err)
		}
		for _, e := range existingEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			src := filepath.Join(destDocsDir, e.Name())
			dst := filepath.Join(backupDocsDir, e.Name())
			if err := copyFileContents(src, dst); err != nil {
				return fmt.Errorf("backing up %s: %w", e.Name(), err)
			}
		}

		// Copy index.json to backup if it exists.
		srcIdx := filepath.Join(leoDir, "kb", "index.json")
		if _, err := os.Stat(srcIdx); err == nil {
			copyFileContents(srcIdx, filepath.Join(backupDir, "index.json")) //nolint:errcheck
		}

		// Remove all existing docs.
		for _, e := range existingEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			os.Remove(filepath.Join(destDocsDir, e.Name()))
		}
	}

	// Read import source docs.
	srcEntries, err := os.ReadDir(srcDocsDir)
	if err != nil {
		return fmt.Errorf("reading import docs dir: %w", err)
	}

	if err := os.MkdirAll(destDocsDir, 0755); err != nil {
		return fmt.Errorf("creating dest docs dir: %w", err)
	}

	var imported, skipped, errCount int

	for _, e := range srcEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		srcPath := filepath.Join(srcDocsDir, e.Name())

		// Validate the doc.
		kbDoc, err := kb.LoadDoc(srcPath)
		if err != nil {
			cmd.Printf("  error: %s: cannot parse: %v\n", e.Name(), err)
			errCount++
			continue
		}
		if err := kbDoc.Validate(); err != nil {
			cmd.Printf("  invalid: %s: %v\n", e.Name(), err)
			errCount++
			continue
		}

		dstPath := filepath.Join(destDocsDir, e.Name())

		if !replaceMode {
			// Merge mode: skip if already exists.
			if _, err := os.Stat(dstPath); err == nil {
				skipped++
				continue
			}
		}

		if err := copyFileContents(srcPath, dstPath); err != nil {
			cmd.Printf("  error: copying %s: %v\n", e.Name(), err)
			errCount++
			continue
		}
		imported++
	}

	// Rebuild the index after import.
	adapter, err := newStorageAdapter()
	if err != nil {
		return fmt.Errorf("creating adapter for reindex: %w", err)
	}

	// Read all docs and BulkWrite to rebuild index.
	allEntries, _ := os.ReadDir(destDocsDir)
	var docs []*storageDoc
	for _, e := range allEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		doc, err := adapter.Read(id)
		if err != nil {
			continue
		}
		docs = append(docs, doc)
	}
	if len(docs) > 0 {
		if err := adapter.BulkWrite(docs); err != nil {
			cmd.Printf("  warning: index rebuild failed: %v\n", err)
		}
	} else {
		// No docs — write an empty index by removing and letting it be recreated.
		indexPath := filepath.Join(leoDir, "kb", "index.json")
		emptyIdx := map[string]any{
			"version": "1", "last_rebuilt": time.Now().UTC().Format(time.RFC3339),
			"by_tag": map[string]any{}, "by_type": map[string]any{},
			"by_scope": map[string]any{}, "by_lifecycle": map[string]any{},
		}
		if data, err := json.MarshalIndent(emptyIdx, "", "  "); err == nil {
			os.WriteFile(indexPath, append(data, '\n'), 0644)
		}
	}

	cmd.Printf("Import complete: %d imported, %d skipped, %d error(s)\n",
		imported, skipped, errCount)
	return nil
}

// copyFileContents copies a file byte-for-byte from src to dst.
func copyFileContents(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
