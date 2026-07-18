package projection

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrFoldLocked reports that another fold (a concurrent `mom vault fold`
// or the daemon's auto-fold) already holds the project's fold lock.
// Callers should surface it and try again later, never bypass it.
var ErrFoldLocked = errors.New("another fold is already running for this project")

// FoldLock is an exclusive, per-project, cross-process fold lock. It
// guards the vault write path (fold state, vault files, CLAUDE.md block)
// so a daemon auto-fold and a manual `mom vault fold` can never interleave
// on the same project. Same flock mechanism as the daemon registry lock
// (ops/daemon/registry.go).
type FoldLock struct {
	f *os.File
}

// foldLockPath returns <root>/.mom/.fold.lock. The lock lives beside the
// vault (not inside it) so a rebuild's prune can never delete it.
func foldLockPath(root string) string {
	return filepath.Join(root, ".mom", ".fold.lock")
}

// AcquireFoldLock takes the project fold lock without blocking. Returns
// ErrFoldLocked when another process holds it. Callers must Release.
func AcquireFoldLock(root string) (*FoldLock, error) {
	path := foldLockPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir for fold lock: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening fold lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrFoldLocked
		}
		return nil, fmt.Errorf("acquiring fold lock: %w", err)
	}
	return &FoldLock{f: f}, nil
}

// Release drops the lock. Safe to call once; the flock dies with the fd.
func (l *FoldLock) Release() {
	if l == nil || l.f == nil {
		return
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
	l.f = nil
}
