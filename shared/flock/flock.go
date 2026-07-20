// Package flock is a tiny cross-platform advisory file-lock seam. It backs
// every cross-process exclusive lock in MOM (the daemon registry lock and
// the per-project fold lock), so the lock mechanism lives in ONE place with
// per-OS implementations behind build tags instead of syscall.Flock scattered
// across packages (which broke the Windows build).
package flock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrLocked reports that another process already holds the lock (only
// returned by TryLock, never by the blocking Lock).
var ErrLocked = errors.New("file is locked by another process")

// Lock is a held advisory lock. Release with Unlock; the OS also drops it
// when the process exits.
type Handle struct {
	f *os.File
}

// Lock blocks until it acquires an exclusive lock on path, creating the file
// if needed. The parent directory must exist.
func Lock(path string) (*Handle, error) {
	f, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f, false); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquiring lock %s: %w", path, err)
	}
	return &Handle{f: f}, nil
}

// TryLock attempts to acquire the lock without blocking. It returns ErrLocked
// when another process holds it.
func TryLock(path string) (*Handle, error) {
	f, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f, true); err != nil {
		_ = f.Close()
		if errors.Is(err, ErrLocked) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("acquiring lock %s: %w", path, err)
	}
	return &Handle{f: f}, nil
}

// Unlock releases the lock. Safe to call once; a nil or already-released lock
// is a no-op.
func (l *Handle) Unlock() {
	if l == nil || l.f == nil {
		return
	}
	_ = unlockFile(l.f)
	_ = l.f.Close()
	l.f = nil
}

func openLockFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir for lock: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening lock %s: %w", path, err)
	}
	return f, nil
}
