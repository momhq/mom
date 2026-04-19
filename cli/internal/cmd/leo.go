package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vmarinogg/leo-core/cli/internal/adapters/storage"
)

// storageDoc is a type alias for use in command implementations.
type storageDoc = storage.Doc

// findLeoDir walks up from cwd to find .leo/ directory.
func findLeoDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(dir, ".leo")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("no .leo/ directory found — run 'leo init' first")
}

// newStorageAdapter creates a JSONAdapter for the current project.
func newStorageAdapter() (*storage.JSONAdapter, error) {
	leoDir, err := findLeoDir()
	if err != nil {
		return nil, err
	}
	return storage.NewJSONAdapter(leoDir), nil
}
