package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vmarinogg/leo-core/cli/internal/adapters/storage"
)

// storageDoc is a type alias for use in command implementations.
type storageDoc = storage.Doc

// findMomDir walks up from cwd to find .mom/ directory.
// Falls back to .leo/ for backward compatibility (v0.10/v0.11 transition).
// If .leo/ is found, a migration warning is printed.
func findMomDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	var leoFallback string

	for {
		momCandidate := filepath.Join(dir, ".mom")
		if info, err := os.Stat(momCandidate); err == nil && info.IsDir() {
			return momCandidate, nil
		}

		if leoFallback == "" {
			leoCandidate := filepath.Join(dir, ".leo")
			if info, err := os.Stat(leoCandidate); err == nil && info.IsDir() {
				leoFallback = leoCandidate
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if leoFallback != "" {
		fmt.Fprintf(os.Stderr, "Warning: Found .leo/ — run 'mom upgrade' to migrate to .mom/\n")
		return leoFallback, nil
	}

	return "", fmt.Errorf("no .mom/ directory found — run 'mom init' first")
}

// newStorageAdapter creates a JSONAdapter for the current project.
func newStorageAdapter() (*storage.JSONAdapter, error) {
	momDir, err := findMomDir()
	if err != nil {
		return nil, err
	}
	return storage.NewJSONAdapter(momDir), nil
}
