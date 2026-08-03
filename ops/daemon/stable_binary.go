package daemon

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
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

// ReconcileStableBinary copies currentBinary to the stable path when its
// content differs from what's installed there (or nothing is installed
// yet), and reports whether a copy happened. Callers use the returned bool
// to decide whether the running daemon needs restarting.
//
// This also covers the Homebrew install flow without special-casing it:
// `brew upgrade` re-points /opt/homebrew/bin/mom at a new Cellar binary.
// Homebrew bottles preserve the original build mtime, so a newer-mtime
// check would miss the upgrade — comparing content instead catches it
// regardless of mtime.
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

	if stableInfo, statErr := os.Stat(stablePath); statErr == nil {
		same, err := sameContent(resolved, stablePath, curInfo, stableInfo)
		if err != nil {
			return "", false, fmt.Errorf("comparing %s and %s: %w", resolved, stablePath, err)
		}
		if same {
			return stablePath, false, nil
		}
	}

	if err := copyBinary(resolved, stablePath, curInfo.ModTime()); err != nil {
		return "", false, fmt.Errorf("copying %s to %s: %w", resolved, stablePath, err)
	}
	return stablePath, true, nil
}

// sameContent reports whether src and dst have identical contents, using
// size as a cheap first filter before hashing.
func sameContent(src, dst string, srcInfo, dstInfo os.FileInfo) (bool, error) {
	if srcInfo.Size() != dstInfo.Size() {
		return false, nil
	}
	srcSum, err := sha256File(src)
	if err != nil {
		return false, err
	}
	dstSum, err := sha256File(dst)
	if err != nil {
		return false, err
	}
	return srcSum == dstSum, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// copyBinary writes src's contents to dst via a unique per-writer temp file
// + rename so concurrent reconciles (e.g. init and the watch-sweep racing
// after an upgrade) never interleave writes or observe a partially-written
// binary. The dest mtime is stamped to srcMtime so any residual mtime logic
// downstream compares source-vs-source rather than source-vs-copy-time.
func copyBinary(src, dst string, srcMtime time.Time) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.CreateTemp(filepath.Dir(dst), "mom-*.tmp")
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer os.Remove(tmp) // best-effort cleanup; no-op once renamed away

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Chmod(0o755); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chtimes(tmp, srcMtime, srcMtime); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	return os.Chmod(dst, 0o755)
}
