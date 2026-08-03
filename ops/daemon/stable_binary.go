package daemon

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// StableBinaryPath returns the fixed location the global daemon's launchd
// plist / systemd unit is always registered against: ~/.mom/bin/mom.
// Registering against a fixed path — rather than os.Executable() — keeps the
// daemon alive across callers whose own executable path is not stable (e.g.
// a Tauri sidecar binary that changes on every app update); registering
// against a moving path orphans the daemon on the next update.
func StableBinaryPath() (string, error) {
	dir, err := RegistryDir()
	if err != nil {
		return "", err
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("creating stable bin dir: %w", err)
	}
	return filepath.Join(binDir, "mom"), nil
}

// ReconcileStableBinary copies currentBinary to the stable path when it is
// newer than what's installed there (or nothing is installed yet), and
// reports whether a copy happened. Callers use the returned bool to decide
// whether the running daemon needs restarting.
//
// This also covers the Homebrew install flow without special-casing it:
// `brew upgrade` re-points /opt/homebrew/bin/mom at a new Cellar binary with
// a newer mtime, so the next reconcile copies the upgraded binary into the
// stable path exactly like any other caller.
func ReconcileStableBinary(currentBinary string) (stablePath string, copied bool, err error) {
	stablePath, err = StableBinaryPath()
	if err != nil {
		return "", false, err
	}

	resolved, err := filepath.EvalSymlinks(currentBinary)
	if err != nil {
		return "", false, fmt.Errorf("resolving %s: %w", currentBinary, err)
	}

	// Already running from the stable path itself (e.g. the daemon process
	// re-checking its own binary) — nothing to reconcile.
	if resolved == stablePath {
		return stablePath, false, nil
	}

	curInfo, err := os.Stat(resolved)
	if err != nil {
		return "", false, fmt.Errorf("stat %s: %w", resolved, err)
	}

	if stableInfo, statErr := os.Stat(stablePath); statErr == nil && !curInfo.ModTime().After(stableInfo.ModTime()) {
		return stablePath, false, nil
	}

	if err := copyBinary(resolved, stablePath); err != nil {
		return "", false, fmt.Errorf("copying %s to %s: %w", resolved, stablePath, err)
	}
	return stablePath, true, nil
}

// copyBinary writes src's contents to dst via a temp file + rename so a
// concurrent daemon restart never observes a partially-written binary.
func copyBinary(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
