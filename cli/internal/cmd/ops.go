package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/adapters/storage"
	"github.com/vmarinogg/leo-core/cli/internal/config"
	"github.com/vmarinogg/leo-core/cli/internal/kb"
	"gopkg.in/yaml.v3"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show KB status summary",
	RunE:  runStatus,
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check .leo/ health and diagnose issues",
	RunE:  runDoctor,
}

// runStatus implements `leo status`.
func runStatus(cmd *cobra.Command, args []string) error {
	leoDir, err := findLeoDir()
	if err != nil {
		return err
	}

	// Load config — required.
	cfg, err := config.Load(leoDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Load index.
	adapter := storage.NewJSONAdapter(leoDir)
	idx, err := adapter.List()
	if err != nil {
		return fmt.Errorf("loading index: %w", err)
	}

	// Compute totals from index maps.
	totalDocs := 0
	for _, ids := range idx.ByType {
		totalDocs += len(ids)
	}
	totalTags := len(idx.ByTag)

	// Build docs-by-type map.
	docsByType := make(map[string]int, len(idx.ByType))
	for t, ids := range idx.ByType {
		docsByType[t] = len(ids)
	}

	// Stale count from raw JSON stats block.
	staleCount := readRawIndexInt(leoDir, "stats", "stale_count")

	cmd.Printf("Runtime:      %s\n", cfg.Runtime)
	cmd.Printf("Storage:      json\n")
	cmd.Printf("Total docs:   %d\n", totalDocs)
	cmd.Printf("Tags:         %d unique\n", totalTags)
	cmd.Printf("Stale docs:   %d\n", staleCount)

	if len(docsByType) > 0 {
		cmd.Printf("Docs by type:\n")
		for t, count := range docsByType {
			cmd.Printf("  %-15s %d\n", t, count)
		}
	}

	return nil
}

// runDoctor implements `leo doctor`.
func runDoctor(cmd *cobra.Command, args []string) error {
	leoDir, err := findLeoDir()
	if err != nil {
		cmd.Printf("✗ .leo/ directory: not found — run 'leo init' first\n")
		return err
	}

	failed := false

	// Check 1: .leo/ exists and is writable.
	if err := checkDirWritable(leoDir); err != nil {
		cmd.Printf("✗ .leo/ directory: %v\n", err)
		failed = true
	} else {
		cmd.Printf("✔ .leo/ directory: exists and writable\n")
	}

	// Check 2: config.yaml is valid.
	cfg, cfgErr := config.Load(leoDir)
	if cfgErr != nil {
		cmd.Printf("✗ config.yaml: %v\n", cfgErr)
		failed = true
	} else {
		cmd.Printf("✔ config.yaml: valid (runtime: %s)\n", cfg.Runtime)
	}

	// Check 3: KB docs dir exists.
	docsDir := filepath.Join(leoDir, "kb", "docs")
	if _, statErr := os.Stat(docsDir); statErr != nil {
		cmd.Printf("✗ kb/docs/: %v\n", statErr)
		failed = true
	} else {
		cmd.Printf("✔ kb/docs/: exists\n")
	}

	// Check 4: All docs pass schema validation.
	docErrors, diskDocIDs := validateAllDocs(cmd, docsDir)
	if docErrors > 0 {
		failed = true
	}

	// Check 5: Index consistency — no orphan files, no orphan entries.
	if orphanFail := checkIndexConsistency(cmd, leoDir, diskDocIDs); orphanFail {
		failed = true
	}

	// Check 6: All profiles are valid YAML.
	profilesDir := filepath.Join(leoDir, "profiles")
	if profileFail := checkProfiles(cmd, profilesDir); profileFail {
		failed = true
	}

	if failed {
		return fmt.Errorf("one or more doctor checks failed")
	}

	return nil
}

// validateAllDocs reads and validates every .json file in docsDir.
// Returns (errorCount, set of valid doc IDs on disk).
func validateAllDocs(cmd *cobra.Command, docsDir string) (int, map[string]bool) {
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		// Docs dir unreadable — check 3 already reported it.
		return 0, nil
	}

	diskDocIDs := make(map[string]bool)
	errors := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		path := filepath.Join(docsDir, e.Name())
		doc, loadErr := kb.LoadDoc(path)
		if loadErr != nil {
			cmd.Printf("✗ doc %s: %v\n", e.Name(), loadErr)
			errors++
			continue
		}

		if valErr := doc.Validate(); valErr != nil {
			cmd.Printf("✗ doc %s: %v\n", e.Name(), valErr)
			errors++
			continue
		}

		diskDocIDs[doc.ID] = true
	}

	if errors == 0 {
		cmd.Printf("✔ schema validation: all docs valid\n")
	} else {
		cmd.Printf("✗ schema validation: %d doc(s) failed\n", errors)
	}

	return errors, diskDocIDs
}

// checkIndexConsistency compares the index to the docs actually on disk.
// Returns true if there are hard failures.
func checkIndexConsistency(cmd *cobra.Command, leoDir string, diskDocIDs map[string]bool) bool {
	adapter := storage.NewJSONAdapter(leoDir)
	idx, err := adapter.List()
	if err != nil {
		cmd.Printf("⚠ index: could not read — %v\n", err)
		return false
	}

	// Collect all IDs referenced in the index.
	indexIDs := make(map[string]bool)
	for _, ids := range idx.ByType {
		for _, id := range ids {
			indexIDs[id] = true
		}
	}

	// Orphan index entries: referenced in index but file is gone.
	orphanEntries := 0
	for id := range indexIDs {
		if diskDocIDs != nil && !diskDocIDs[id] {
			cmd.Printf("⚠ index: orphan entry — %q not on disk\n", id)
			orphanEntries++
		}
	}

	// Orphan files: on disk but not in index.
	orphanFiles := 0
	for id := range diskDocIDs {
		if !indexIDs[id] {
			cmd.Printf("⚠ index: orphan file — %q not in index\n", id)
			orphanFiles++
		}
	}

	if orphanEntries > 0 || orphanFiles > 0 {
		cmd.Printf("✗ index consistency: %d orphan entries, %d orphan files\n", orphanEntries, orphanFiles)
		return true
	}

	cmd.Printf("✔ index consistency: ok\n")
	return false
}

// checkProfiles validates all YAML files in profilesDir.
// Returns true if any profile fails validation.
func checkProfiles(cmd *cobra.Command, profilesDir string) bool {
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		cmd.Printf("⚠ profiles/: directory not found\n")
		return false
	}

	failed := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		path := filepath.Join(profilesDir, e.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			cmd.Printf("✗ profile %s: %v\n", e.Name(), readErr)
			failed = true
			continue
		}

		var v map[string]any
		if unmarshalErr := yaml.Unmarshal(data, &v); unmarshalErr != nil {
			cmd.Printf("✗ profile %s: invalid YAML — %v\n", e.Name(), unmarshalErr)
			failed = true
			continue
		}

		cmd.Printf("✔ profile %s: valid\n", e.Name())
	}

	return failed
}

// checkDirWritable verifies a directory exists and is writable.
func checkDirWritable(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("not found: %v", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory")
	}

	tmp := filepath.Join(dir, ".write-check")
	if err := os.WriteFile(tmp, []byte("ok"), 0644); err != nil {
		return fmt.Errorf("not writable: %v", err)
	}
	os.Remove(tmp)

	return nil
}

// readRawIndexInt reads a nested integer from the raw index JSON.
func readRawIndexInt(leoDir string, keys ...string) int {
	indexPath := filepath.Join(leoDir, "kb", "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return 0
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0
	}

	var node any = raw
	for _, key := range keys {
		m, ok := node.(map[string]any)
		if !ok {
			return 0
		}
		node = m[key]
	}

	switch v := node.(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}
