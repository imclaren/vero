//go:build !plan9

package vero

import "os"

// openLockFile opens the file whose lock the supervisor then takes.
//
// Everywhere but Plan 9 the open is ordinary and the locking is a separate
// call: flock, fcntl, or LockFileEx.  Plan 9 has none of those, so there the
// open is the lock, and this is where that difference lives.
func openLockFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
}
