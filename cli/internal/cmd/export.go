package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/momhq/mom/cli/internal/memory"
)

var exportCmd = &cobra.Command{
	Use:   "export [path]",
	Short: "Export memory to a portable format",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runExport,
}

var importCmd = &cobra.Command{
	Use:   "import [path]",
	Short: "Import memory from a portable format",
	Args:  cobra.ExactArgs(1),
	RunE:  runImport,
}

func init() {
	exportCmd.Flags().String("output", "", "Output directory path (default: ./mom-export)")
	importCmd.Flags().Bool("merge", false, "Merge: keep existing docs, add new, skip conflicts (default)")
	importCmd.Flags().Bool("replace", false, "Replace: back up current memory, then replace entirely")
}

// runExport implements the leo export command.
func runExport(cmd *cobra.Command, args []string) error {
	leoDir, err := findMomDir()
	if err != nil {
		return err
	}

	// Determine output directory.
	outputFlag, _ := cmd.Flags().GetString("output")
	var outputDir string
	if outputFlag != "" {
		outputDir = outputFlag
	} else if len(args) > 0 {
		outputDir = filepath.Join(args[0], "mom-export")
	} else {
		// Default: ./mom-export relative to cwd.
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting cwd: %w", err)
		}
		outputDir = filepath.Join(cwd, "mom-export")
	}

	docsOutputDir := filepath.Join(outputDir, "docs")
	if err := os.MkdirAll(docsOutputDir, 0755); err != nil {
		return fmt.Errorf("creating output docs dir: %w", err)
	}

	// Copy all docs.
	srcDocsDir := filepath.Join(leoDir, "memory")
	docCount, err := copyJSONDir(srcDocsDir, docsOutputDir)
	if err != nil {
		return fmt.Errorf("copying docs: %w", err)
	}

	// Copy constraints.
	constraintsCount := 0
	srcConstraints := filepath.Join(leoDir, "constraints")
	if dirExists(srcConstraints) {
		dstConstraints := filepath.Join(outputDir, "constraints")
		if err := os.MkdirAll(dstConstraints, 0755); err != nil {
			return fmt.Errorf("creating constraints dir: %w", err)
		}
		constraintsCount, err = copyJSONDir(srcConstraints, dstConstraints)
		if err != nil {
			return fmt.Errorf("copying constraints: %w", err)
		}
	}

	// Copy skills.
	skillsCount := 0
	srcSkills := filepath.Join(leoDir, "skills")
	if dirExists(srcSkills) {
		dstSkills := filepath.Join(outputDir, "skills")
		if err := os.MkdirAll(dstSkills, 0755); err != nil {
			return fmt.Errorf("creating skills dir: %w", err)
		}
		skillsCount, err = copyJSONDir(srcSkills, dstSkills)
		if err != nil {
			return fmt.Errorf("copying skills: %w", err)
		}
	}

	// Copy schema.json if it exists.
	srcSchema := filepath.Join(leoDir, "schema.json")
	if _, err := os.Stat(srcSchema); err == nil {
		dstSchema := filepath.Join(outputDir, "schema.json")
		if err := copyFileContents(srcSchema, dstSchema); err != nil {
			return fmt.Errorf("copying schema.json: %w", err)
		}
	}

	// Copy identity.json if it exists.
	srcIdentity := filepath.Join(leoDir, "identity.json")
	if _, err := os.Stat(srcIdentity); err == nil {
		dstIdentity := filepath.Join(outputDir, "identity.json")
		if err := copyFileContents(srcIdentity, dstIdentity); err != nil {
			return fmt.Errorf("copying identity.json: %w", err)
		}
	}

	// Copy config.yaml if it exists.
	srcConfig := filepath.Join(leoDir, "config.yaml")
	if _, err := os.Stat(srcConfig); err == nil {
		dstConfig := filepath.Join(outputDir, "config.yaml")
		if err := copyFileContents(srcConfig, dstConfig); err != nil {
			return fmt.Errorf("copying config.yaml: %w", err)
		}
	}

	cmd.Printf("Exported to %s: %d docs, %d constraints, %d skills\n",
		outputDir, docCount, constraintsCount, skillsCount)
	return nil
}

// runImport implements the leo import command.
func runImport(cmd *cobra.Command, args []string) error {
	importPath := args[0]

	replaceMode, _ := cmd.Flags().GetBool("replace")

	leoDir, err := findMomDir()
	if err != nil {
		return err
	}

	srcDocsDir := filepath.Join(importPath, "docs")
	if _, err := os.Stat(srcDocsDir); err != nil {
		return fmt.Errorf("import path must contain a docs/ directory: %w", err)
	}

	destDocsDir := filepath.Join(leoDir, "memory")

	if replaceMode {
		// Back up current memory to .mom/backup-{timestamp}/.
		timestamp := time.Now().UTC().Format("20060102-150405")
		backupDir := filepath.Join(leoDir, "backup-"+timestamp)
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
		kbDoc, err := memory.LoadDoc(srcPath)
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

	// Import constraints if present.
	srcConstraints := filepath.Join(importPath, "constraints")
	if dirExists(srcConstraints) {
		destConstraints := filepath.Join(leoDir, "constraints")
		if err := os.MkdirAll(destConstraints, 0755); err != nil {
			return fmt.Errorf("create constraints dir: %w", err)
		}
		importDirFiles(srcConstraints, destConstraints, ".json", replaceMode)
	}

	// Import skills if present.
	srcSkills := filepath.Join(importPath, "skills")
	if dirExists(srcSkills) {
		destSkills := filepath.Join(leoDir, "skills")
		if err := os.MkdirAll(destSkills, 0755); err != nil {
			return fmt.Errorf("create skills dir: %w", err)
		}
		importDirFiles(srcSkills, destSkills, ".json", replaceMode)
	}

	// Import identity.json if present.
	srcIdentity := filepath.Join(importPath, "identity.json")
	if _, err := os.Stat(srcIdentity); err == nil {
		dstIdentity := filepath.Join(leoDir, "identity.json")
		copyFileContents(srcIdentity, dstIdentity) //nolint:errcheck
	}

	// Import config.yaml if present.
	srcConfig := filepath.Join(importPath, "config.yaml")
	if _, err := os.Stat(srcConfig); err == nil {
		dstConfig := filepath.Join(leoDir, "config.yaml")
		copyFileContents(srcConfig, dstConfig) //nolint:errcheck
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

// dirExists returns true if the path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// copyJSONDir copies all .json files from src to dst. Returns count.
func copyJSONDir(src, dst string) (int, error) {
	return copyDirByExt(src, dst, ".json")
}

// copyDirByExt copies all files with the given extension from src to dst.
func copyDirByExt(src, dst, ext string) (int, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return 0, fmt.Errorf("reading dir: %w", err)
	}

	var count int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) {
			continue
		}
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if err := copyFileContents(srcPath, dstPath); err != nil {
			return count, fmt.Errorf("copying %s: %w", e.Name(), err)
		}
		count++
	}
	return count, nil
}

// importDirFiles copies files with the given extension from src to dst.
// In merge mode (replaceMode=false), existing files are skipped.
func importDirFiles(src, dst, ext string, replaceMode bool) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) {
			continue
		}
		dstPath := filepath.Join(dst, e.Name())
		if !replaceMode {
			if _, err := os.Stat(dstPath); err == nil {
				continue // skip existing
			}
		}
		srcPath := filepath.Join(src, e.Name())
		copyFileContents(srcPath, dstPath) //nolint:errcheck
	}
}
