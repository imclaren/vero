//go:build solaris || aix

package vero

import (
	"os"
	"syscall"
)

// Solaris and AIX have no flock.  They have fcntl locking, which gives the
// same guarantee for this purpose: the lock is held by the open file and the
// kernel drops it when the process exits, so a crash cannot leave one behind.
//
// The two differ elsewhere - fcntl locks are per process rather than per
// descriptor, and closing any descriptor on the file releases them - but a
// supervisor opens the lock once and holds it, so neither applies here.
//
// Through syscall rather than golang.org/x/sys, so that this package keeps
// having no dependencies, which is also why Windows calls kernel32 directly.
func lockFile(f *os.File) error {
	lk := syscall.Flock_t{
		Type:   syscall.F_WRLCK,
		Whence: 0, // from the start of the file
		Start:  0,
		Len:    0, // to the end of it, however long it becomes
	}
	return syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk)
}

func unlockFile(f *os.File) {
	lk := syscall.Flock_t{Type: syscall.F_UNLCK, Whence: 0, Start: 0, Len: 0}
	_ = syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk)
}
