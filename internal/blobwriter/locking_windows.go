//go:build windows

package blobwriter

import "os"

// Windows does not provide flock(2); the staging coordination is best-effort
// there (it primarily protects same-process writers, which are always
// isolated by their own temporary files).

func sharedLock(f *os.File) error    { return nil }
func exclusiveLock(f *os.File) error { return nil }
func unlock(f *os.File) error        { return nil }

func syncDirectory(path string) error { return nil }
