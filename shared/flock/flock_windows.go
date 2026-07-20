//go:build windows

package flock

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive byte-range lock via LockFileEx on Windows. A
// contended nonblocking lock returns ERROR_LOCK_VIOLATION, mapped to
// ErrLocked. The lock covers the whole file (max range).
func lockFile(f *os.File, nonblocking bool) error {
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if nonblocking {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	ol := new(windows.Overlapped)
	err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, ^uint32(0), ^uint32(0), ol)
	if err != nil {
		if nonblocking && err == windows.ERROR_LOCK_VIOLATION {
			return ErrLocked
		}
		return err
	}
	return nil
}

func unlockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, ^uint32(0), ^uint32(0), ol)
}
