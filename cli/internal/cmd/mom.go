package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

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
			if isMomProject(momCandidate) {
				return momCandidate, nil
			}
		}

		if leoFallback == "" {
			leoCandidate := filepath.Join(dir, ".leo")
			if info, err := os.Stat(leoCandidate); err == nil && info.IsDir() {
				if isMomProject(leoCandidate) {
					leoFallback = leoCandidate
				}
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

// isMomProject returns true if dir looks like a MOM/LEO project directory
// (has config.yaml, memory/, or index.json). This prevents ~/.mom/cache/
// (created by version check) from being mistaken for a project.
func isMomProject(dir string) bool {
	markers := []string{"config.yaml", "memory"}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return true
		}
	}
	return false
}
