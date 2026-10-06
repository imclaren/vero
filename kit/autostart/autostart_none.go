//go:build plan9 || wasip1 || js || ios || android

package autostart

import "errors"

// ErrUnsupported is a system with no way to open an app at sign-in that a
// program can ask for.
var ErrUnsupported = errors.New("autostart: not available on this system")

func enable(App) error          { return ErrUnsupported }
func disable(App) error         { return ErrUnsupported }
func enabled(App) (bool, error) { return false, ErrUnsupported }
