//go:build !windows

package blobwriter

import (
	"os"
	"syscall"
)

// sharedLock acquires a blocking shared advisory lock on f. Multiple writers
// may hold shared locks concurrently.
func sharedLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_SH)
}

// exclusiveLock acquires a blocking exclusive advisory lock on f. It is
// granted only when no shared locks are held, so CleanupStaging can not run
// while writers are active.
func exclusiveLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// unlock releases an advisory lock held on f.
func unlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// syncDirectory persists directory entry changes (e.g. newly created links).
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
