package keychain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestPassword checks the files kept where there is no keychain: nothing
// new saved before a password is chosen, an older plain file still read
// and then encrypted, locking and unlocking, a wrong password refused, and
// a new password that every file is encrypted with again.
func TestPassword(t *testing.T) {
	dir := t.TempDir()
	passwords.Lock()
	passwords.noKeychain, passwords.key = true, nil
	passwords.Unlock()
	t.Cleanup(func() {
		passwords.Lock()
		passwords.noKeychain, passwords.key, passwords.dir = false, nil, ""
		passwords.Unlock()
	})
	UsePassword(dir)
	it := Item{Service: "dev.vero.test", Account: "a", Path: filepath.Join(dir, "token")}
	old := Item{Service: "dev.vero.test", Account: "b", Path: filepath.Join(dir, "older")}
	os.WriteFile(old.Path, []byte("from before"), 0o600)

	if s := CurrentStatus(); !s.NeedsPassword || s.Keychain || s.Locked {
		t.Fatalf("status before a password: %+v", s)
	}
	if err := it.Save([]byte("secret")); !errors.Is(err, ErrNeedsPassword) {
		t.Errorf("saved before a password: %v", err)
	}
	if data, err := old.Load(); err != nil || string(data) != "from before" {
		t.Errorf("an older plain file: %q, %v", data, err)
	}
	if err := SetPassword("", "short"); err == nil {
		t.Error("a short password was taken")
	}
	if err := SetPassword("", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if err := it.Save([]byte("secret")); err != nil {
		t.Fatal(err)
	}
	disk, _ := os.ReadFile(it.Path)
	if !bytes.HasPrefix(disk, magic) || bytes.Contains(disk, []byte("secret")) {
		t.Errorf("not encrypted on disk: %q", disk)
	}
	if data, _ := old.Load(); string(data) != "from before" {
		t.Errorf("older file: %q", data)
	}
	if disk, _ := os.ReadFile(old.Path); !bytes.HasPrefix(disk, magic) {
		t.Error("the older file was not encrypted once the password was known")
	}

	Lock()
	if s := CurrentStatus(); !s.Locked {
		t.Errorf("status when locked: %+v", s)
	}
	if _, err := it.Load(); !errors.Is(err, ErrLocked) {
		t.Errorf("read while locked: %v", err)
	}
	if err := Unlock("wrong horse"); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("wrong password: %v", err)
	}
	if err := Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	if data, err := it.Load(); err != nil || string(data) != "secret" {
		t.Errorf("after unlocking: %q, %v", data, err)
	}

	// A new password, with which both files are encrypted again.
	if err := SetPassword("wrong horse", "battery staple"); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("changed with the wrong old password: %v", err)
	}
	if err := SetPassword("correct horse", "battery staple"); err != nil {
		t.Fatal(err)
	}
	Lock()
	if err := Unlock("correct horse"); !errors.Is(err, ErrWrongPassword) {
		t.Error("the old password still works")
	}
	if err := Unlock("battery staple"); err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct {
		item Item
		want string
	}{{it, "secret"}, {old, "from before"}} {
		if data, err := x.item.Load(); err != nil || string(data) != x.want {
			t.Errorf("%s after the change: %q, %v", x.item.Path, data, err)
		}
	}

	// The standard requests, as the front end reads them.
	s, _ := StatusHandler(context.Background(), struct{}{})
	data, _ := json.Marshal(s)
	if string(data) != `{"keychain":false,"needsPassword":false,"locked":false}` {
		t.Errorf("status reply %s", data)
	}
	Lock()
	if _, err := UnlockHandler(context.Background(), struct {
		Password string `json:"password"`
	}{"battery staple"}); err != nil {
		t.Error(err)
	}
}
