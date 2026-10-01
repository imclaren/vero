//go:build plan9

package vero

import (
	"os"
	"strings"
)

// Plan 9 has neither flock nor fcntl locking.  What it has is the
// exclusive-use bit: a file carrying DMEXCL can be held open by one process
// at a time, and the kernel closes it when that process exits - the same
// guarantee flock gives, arrived at from the other end.
//
// So here the open is the lock, and lockFile has nothing left to do.
func openLockFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600|os.ModeExclusive)
	if err != nil {
		// Plan 9 refuses the open rather than failing a later lock call, so
		// this is what "somebody else has it" looks like.  9front says
		//
		//	open /tmp/vero-1b18d2d8425c14c6.lock: file is locked
		//
		// and older kernels say "exclusive use file already open", so match
		// either rather than the whole string.
		if message := err.Error(); strings.Contains(message, "file is locked") ||
			strings.Contains(message, "exclusive use") {
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}

	// A lock file left by an older version has no exclusive-use bit, and
	// without it the open above excludes nobody.  Setting it now costs one
	// call and makes every open after this one behave.
	if fi, statErr := f.Stat(); statErr == nil && fi.Mode()&os.ModeExclusive == 0 {
		_ = f.Chmod(fi.Mode() | os.ModeExclusive)
	}
	return f, nil
}

func lockFile(*os.File) error { return nil }

func unlockFile(*os.File) {}
