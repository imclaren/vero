//go:build windows || linux || netbsd || openbsd || (freebsd && cgo) || (dragonfly && cgo)

package keychain

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// Windows keeps secrets in its Credential Manager, and Linux and the BSDs
// in the Secret Service their desktop runs. A system with no desktop has no
// Secret Service, and every Item is its file there.
var available = probe()

// probe asks for something that is not there: a Secret Service, or the
// Credential Manager, answers that it is not.
func probe() bool {
	_, err := keyring.Get("dev.vero.keychain", "probe")
	return err == nil || errors.Is(err, keyring.ErrNotFound)
}

func get(service, account string) ([]byte, error) {
	s, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrNotFound
	}
	return []byte(s), err
}

func set(service, account string, data []byte) error {
	return keyring.Set(service, account, string(data))
}

func del(service, account string) error {
	err := keyring.Delete(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
