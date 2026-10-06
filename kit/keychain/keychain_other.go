//go:build !(darwin && cgo) && !(windows || linux || netbsd || openbsd || (freebsd && cgo) || (dragonfly && cgo))

package keychain

// No keychain here: every Item is its file.
const available = false

func get(string, string) ([]byte, error) { return nil, ErrNotFound }
func set(string, string, []byte) error   { return ErrNotFound }
func del(string, string) error           { return ErrNotFound }
