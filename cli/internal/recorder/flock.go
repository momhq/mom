package recorder

import (
	"os"
	"syscall"
)

// withFileLock acquires an exclusive advisory lock on path+".lock",
// executes fn, then releases the lock.
func withFileLock(path string, fn func() error) error {
	lockFile := path + ".lock"
	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fn() // fallback: run without lock
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fn() // fallback: run without lock
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
