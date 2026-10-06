package keychain

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFile keeps a secret in its file, and moves nothing anywhere else.
func TestFile(t *testing.T) {
	it := Item{Path: filepath.Join(t.TempDir(), "s.json"), File: true}
	if data, err := it.Load(); data != nil || err != nil {
		t.Fatalf("nothing kept: %q, %v", data, err)
	}
	if err := it.Save([]byte("secret")); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(it.Path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file: %v, %v", info, err)
	}
	if data, _ := it.Load(); string(data) != "secret" {
		t.Fatalf("read back %q", data)
	}
	if err := it.Remove(); err != nil {
		t.Fatal(err)
	}
	if data, _ := it.Load(); data != nil {
		t.Fatalf("removed, still %q", data)
	}
}

// TestKeychain uses the real login keychain, so it runs only when asked:
// VERO_KEYCHAIN_TEST=1 go test ./keychain. It writes one item
// and removes it.
func TestKeychain(t *testing.T) {
	if os.Getenv("VERO_KEYCHAIN_TEST") == "" || !Available() {
		t.Skip("set VERO_KEYCHAIN_TEST=1 to use the login keychain")
	}
	old := filepath.Join(t.TempDir(), "old.json")
	os.WriteFile(old, []byte("from a file"), 0o600)
	it := Item{Service: "dev.vero.keychain.test", Account: "keychain_test", Path: old}
	defer it.Remove()

	// A secret kept in a file before is moved in.
	if data, err := it.Load(); err != nil || string(data) != "from a file" {
		t.Fatalf("first load: %q, %v", data, err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("the file was not removed once the secret was in the keychain")
	}
	if data, err := it.Load(); err != nil || string(data) != "from a file" {
		t.Fatalf("from the keychain: %q, %v", data, err)
	}
	if err := it.Save([]byte("replaced")); err != nil {
		t.Fatal(err)
	}
	if data, _ := it.Load(); string(data) != "replaced" {
		t.Fatalf("after save: %q", data)
	}
	if err := it.Remove(); err != nil {
		t.Fatal(err)
	}
	if data, err := it.Load(); data != nil || err != nil {
		t.Fatalf("after remove: %q, %v", data, err)
	}
}
