package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vmarinogg/leo-core/cli/internal/adapters/storage"
)

// storageDoc is a type alias for use in command implementations.
type storageDoc = storage.Doc

// findMomDir walks up from cwd to find .mom/ directory, falling back to .leo/ for
// backward compatibility with projects that have not yet migrated.
func findMomDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		// Prefer .mom/ (current name), fall back to .leo/ (legacy name).
		for _, name := range []string{".mom", ".leo"} {
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("no .mom/ directory found — run 'mom init' first")
}

// findLeoDir is deprecated: use findMomDir. Kept for call sites not yet migrated.
func findLeoDir() (string, error) {
	return findMomDir()
}

// newStorageAdapter creates a JSONAdapter for the current project.
func newStorageAdapter() (*storage.JSONAdapter, error) {
	momDir, err := findMomDir()
	if err != nil {
		return nil, err
	}
	return storage.NewJSONAdapter(momDir), nil
}
