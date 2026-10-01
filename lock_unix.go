//go:build !windows && !solaris && !aix && !plan9

package vero

import (
	"os"
	"syscall"
)

// flock is released when the descriptor closes, including when the process
// dies, which is what makes a stale lock impossible.
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func unlockFile(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
