//go:build js || wasip1

package vero

import "os"

// Nothing to lock against.  The lock stops two processes supervising one
// worker, and wasm has no second process: a browser has none at all, and a
// WASI guest cannot start one.  So this succeeds, which is the truthful
// answer rather than a convenient one.
func lockFile(*os.File) error { return nil }

func unlockFile(*os.File) {}
