//go:build !(darwin || windows || linux || freebsd || openbsd || netbsd)

package awake

// Nothing here can hold the computer awake.
func hold(string) (func(), error) { return nil, ErrUnsupported }
