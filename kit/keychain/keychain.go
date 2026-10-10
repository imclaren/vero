// Package keychain keeps small secrets - sign-ins, tokens - in the system's
// store for them: the login keychain on macOS, the Credential Manager on
// Windows, and the Secret Service (GNOME Keyring, KWallet) on Linux and the
// BSDs. It falls back to a file only this user can read where there is none
// to use: a system without one, a Linux without a desktop, or a build
// without cgo where the store needs it (macOS, FreeBSD, DragonFly).
//
// On macOS an item belongs to the program that wrote it: macOS lets that
// program read it back without asking, by its code signature, and asks the
// person before it lets anything else. So sign your worker with the same
// identity in development and in a release, and it reads its own secrets
// silently from one version to the next. A binary built by `go run`, signed
// by nothing but its own hash, is asked about after every build.
package keychain

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/imclaren/vero/kit/profile"
)

// ErrNotFound is a secret that is not there.
var ErrNotFound = errors.New("not in the keychain")

// Available says whether this build can use the keychain.
func Available() bool { return available }

// Item is one secret: in the keychain under Service and Account, or in the
// file at Path where there is no keychain. Path is also where a secret
// kept before the keychain was used is looked for, and moved in from.
type Item struct {
	Service string // your app's id, such as "com.example.myapp"
	Account string // which secret, such as "tokens/me@example.com"
	Path    string
	// File keeps the secret in Path even where there is a keychain: for
	// tests, which must not write to the keychain of whoever runs them.
	File bool
}

// service is the keychain service, for VERO_PROFILE's copy of the app
// when it is set (kit/profile), so that a test never sees the app's own.
func (it Item) service() string { return profile.Service(it.Service) }

// Load reads the secret. Nothing kept is not an error: it returns nil.
func (it Item) Load() ([]byte, error) {
	if it.File || !available {
		return readFile(it.Path)
	}
	data, err := get(it.service(), it.Account)
	if err == nil {
		return data, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	// Kept in a file by an earlier version: move it in.
	data, err = readFile(it.Path)
	if err != nil || data == nil {
		return data, err
	}
	if err := set(it.service(), it.Account, data); err != nil {
		return data, nil // still readable; moved next time
	}
	os.Remove(it.Path)
	return data, nil
}

// Save keeps the secret, replacing what was there.
func (it Item) Save(data []byte) error {
	if it.File || !available {
		return writeFile(it.Path, data)
	}
	if err := set(it.service(), it.Account, data); err != nil {
		return err
	}
	if it.Path != "" {
		os.Remove(it.Path) // an older copy, outside the keychain
	}
	return nil
}

// Remove forgets the secret, wherever it was kept.
func (it Item) Remove() error {
	var err error
	if !it.File && available {
		if err = del(it.service(), it.Account); errors.Is(err, ErrNotFound) {
			err = nil
		}
	}
	if it.Path != "" {
		if rerr := os.Remove(it.Path); rerr != nil && !errors.Is(rerr, os.ErrNotExist) && err == nil {
			err = rerr
		}
	}
	return err
}

func readFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// writeFile writes a file only this user can read, whole or not at all.
func writeFile(path string, data []byte) error {
	if path == "" {
		return errors.New("nowhere to keep it")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
