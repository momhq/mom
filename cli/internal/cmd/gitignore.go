package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/momhq/mom/cli/internal/adapters/runtime"
)

// coreGitIgnorePaths are always added regardless of runtime.
var coreGitIgnorePaths = []string{
	".mom/",
	".mcp.json",
}

// gitIgnoreHeader is the comment placed above MOM entries.
const gitIgnoreHeader = "# MOM (generated at runtime)"

// ensureGitIgnore ensures that .gitignore at projectRoot contains all MOM-generated
// paths for the given runtimes. If .gitignore doesn't exist, it is created.
// Returns the list of entries that were added (empty if all were already present).
func ensureGitIgnore(projectRoot string, registry *runtime.Registry, runtimes []string) ([]string, error) {
	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	// Collect all paths that should be present.
	needed := make([]string, 0, len(coreGitIgnorePaths)+len(runtimes)*3)
	needed = append(needed, coreGitIgnorePaths...)
	for _, rt := range runtimes {
		adapter, ok := registry.Get(rt)
		if !ok {
			continue
		}
		needed = append(needed, adapter.GitIgnorePaths()...)
	}

	// De-duplicate needed paths (e.g. CLAUDE.md from multiple adapters).
	needed = dedup(needed)

	// Read existing .gitignore (may not exist).
	existing, _ := os.ReadFile(gitignorePath)
	existingLines := strings.Split(string(existing), "\n")

	// Build a set of existing entries (trimmed, ignoring comments and blanks).
	existingSet := make(map[string]bool)
	for _, line := range existingLines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			existingSet[trimmed] = true
		}
	}

	// Determine which entries are missing.
	var missing []string
	for _, entry := range needed {
		if !existingSet[entry] {
			missing = append(missing, entry)
		}
	}

	if len(missing) == 0 {
		return nil, nil
	}

	// Build the block to append.
	var block strings.Builder

	// Add a blank line before the header if the file has content and doesn't
	// end with a blank line.
	content := string(existing)
	if len(content) > 0 && !strings.HasSuffix(content, "\n\n") {
		if !strings.HasSuffix(content, "\n") {
			block.WriteString("\n")
		}
		block.WriteString("\n")
	}

	block.WriteString(gitIgnoreHeader)
	block.WriteString("\n")
	for _, entry := range missing {
		block.WriteString(entry)
		block.WriteString("\n")
	}

	// Write / append.
	f, err := os.OpenFile(gitignorePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.WriteString(block.String()); err != nil {
		return nil, err
	}

	return missing, nil
}

// dedup returns a slice with duplicate strings removed, preserving order.
func dedup(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}
