package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/momhq/mom/cli/internal/adapters/runtime"
	"github.com/momhq/mom/cli/internal/adapters/storage"
	"github.com/momhq/mom/cli/internal/config"
	"github.com/momhq/mom/cli/internal/memory"
	"github.com/momhq/mom/cli/internal/scope"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show memory status summary",
	RunE:  runStatus,
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check .mom/ health and diagnose issues",
	RunE:  runDoctor,
}

// runStatus implements `leo status`.
func runStatus(cmd *cobra.Command, args []string) error {
	leoDir, err := findMomDir()
	if err != nil {
		return err
	}

	// Load config — required.
	cfg, err := config.Load(leoDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Load index via IndexedAdapter (write-through + FTS5).
	adapter := storage.NewIndexedAdapter(leoDir)
	defer adapter.Close()
	idx, err := adapter.List()
	if err != nil {
		return fmt.Errorf("loading index: %w", err)
	}

	// Compute totals from index maps.
	totalDocs := 0
	for _, ids := range idx.ByScope {
		totalDocs += len(ids)
	}
	totalTags := len(idx.ByTag)

	// Stale count from raw JSON stats block.
	staleCount := readRawIndexInt(leoDir, "stats", "stale_count")

	// Show enabled runtimes.
	enabledRTs := cfg.EnabledRuntimes()
	if len(enabledRTs) > 0 {
		cmd.Printf("Runtimes:     %s\n", strings.Join(enabledRTs, ", "))
	} else {
		cmd.Printf("Runtimes:     (none)\n")
	}
	commMode := cfg.Communication.Mode
	if commMode == "" {
		commMode = "concise"
	}
	cmd.Printf("Mode:         %s\n", commMode)
	cmd.Printf("Storage:      json\n")
	cmd.Printf("Total docs:   %d\n", totalDocs)
	cmd.Printf("Tags:         %d unique\n", totalTags)
	cmd.Printf("Stale docs:   %d\n", staleCount)

	return nil
}

// printAdapterCapabilities prints the MRP v0 capability summary for each enabled adapter.
func printAdapterCapabilities(cmd *cobra.Command, projectRoot string, cfg *config.Config) {
	enabled := cfg.EnabledRuntimes()
	if len(enabled) == 0 {
		return
	}
	registry := runtime.NewRegistry(projectRoot)
	cmd.Printf("\nAdapter capabilities (MRP v0):\n")
	for _, name := range enabled {
		adapter, ok := registry.Get(name)
		if !ok {
			continue
		}
		cap := adapter.Capabilities()
		adapterName := cap.Name
		if adapterName == "" {
			adapterName = name
		}
		version := cap.Version
		if version == "" {
			version = "unknown"
		}
		cmd.Printf("  Adapter: %s (v%s)\n", adapterName, version)
		if len(cap.Supports) > 0 {
			cmd.Printf("    Supported:    %s\n", strings.Join(cap.Supports, ", "))
		}
		if len(cap.Experimental) > 0 {
			cmd.Printf("    Experimental: %s\n", strings.Join(cap.Experimental, ", "))
		}
	}
}

// printScopesSection prints the active scopes discovered by walk-up from cwd.
func printScopesSection(cmd *cobra.Command, cwd string) {
	scopes := scope.Walk(cwd)
	if len(scopes) == 0 {
		return
	}
	cmd.Printf("\nActive scopes (nearest first):\n")
	for _, s := range scopes {
		cmd.Printf("  %-12s %s  (%d memories)\n",
			s.Label, shortenPath(s.Path), s.MemoryCount())
	}
}

// validateAllDocs reads and validates every .json file in dir.
// label is used for log messages (e.g. "doc", "constraint", "skill").
// Returns (errorCount, set of valid doc IDs on disk).
func validateAllDocs(cmd *cobra.Command, dir string, label string) (int, map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Dir unreadable or missing — already reported.
		return 0, nil
	}

	diskDocIDs := make(map[string]bool)
	errors := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		doc, loadErr := memory.LoadDoc(path)
		if loadErr != nil {
			cmd.Printf("✗ %s %s: %v\n", label, e.Name(), loadErr)
			errors++
			continue
		}

		if valErr := doc.Validate(); valErr != nil {
			cmd.Printf("✗ %s %s: %v\n", label, e.Name(), valErr)
			errors++
			continue
		}

		diskDocIDs[doc.ID] = true
	}

	if errors == 0 && len(diskDocIDs) > 0 {
		cmd.Printf("✔ %ss: all %d valid\n", label, len(diskDocIDs))
	} else if errors > 0 {
		cmd.Printf("✗ %ss: %d failed validation\n", label, errors)
	}

	return errors, diskDocIDs
}

// checkIndexConsistency compares the index to the docs actually on disk.
// Returns true if there are hard failures.
func checkIndexConsistency(cmd *cobra.Command, leoDir string, diskDocIDs map[string]bool) bool {
	adapter := storage.NewIndexedAdapter(leoDir)
	defer adapter.Close()
	idx, err := adapter.List()
	if err != nil {
		cmd.Printf("⚠ index: could not read — %v\n", err)
		return false
	}

	// Collect all IDs referenced in the index.
	indexIDs := make(map[string]bool)
	for _, ids := range idx.ByScope {
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
	indexPath := filepath.Join(leoDir, "index.json")
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
