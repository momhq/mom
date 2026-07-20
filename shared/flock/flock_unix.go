//go:build !windows

package flock

import (
	"errors"
	"os"
	"syscall"
)

// lockFile takes a BSD advisory lock (flock) on f. When nonblocking, a
// contended lock surfaces as ErrLocked.
func lockFile(f *os.File, nonblocking bool) error {
	how := syscall.LOCK_EX
	if nonblocking {
		how |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		if nonblocking && (errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)) {
			return ErrLocked
		}
		return err
	}
	return nil
}

func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
