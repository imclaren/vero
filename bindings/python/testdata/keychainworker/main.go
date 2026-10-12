//go:build ignore

// A worker that keeps a sign-in with kit/keychain, for the GTK library's
// test of unlock_sign_ins. It needs vero and vero's kit, so
// scripts/test-gtk-libs.sh builds it in a module of its own;
// VERO_TEST_KEYCHAIN_DIR is the folder for its files.
package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/imclaren/vero"
	"github.com/imclaren/vero/kit/keychain"
)

func main() {
	dir := os.Getenv("VERO_TEST_KEYCHAIN_DIR")
	w := vero.NewWorker(vero.WorkerOptions{})
	keychain.UsePassword(dir)
	vero.Handle(w, keychain.StatusRequest, keychain.StatusHandler)
	vero.Handle(w, keychain.UnlockRequest, keychain.UnlockHandler)
	vero.Handle(w, keychain.PasswordRequest, keychain.PasswordHandler)
	item := keychain.Item{Service: "dev.vero.test", Account: "token", Path: filepath.Join(dir, "token")}
	vero.Handle(w, "save", func(_ context.Context, req struct {
		Text string `json:"text"`
	}) (struct{}, error) {
		return struct{}{}, item.Save([]byte(req.Text))
	})
	vero.Handle(w, "load", func(context.Context, struct{}) (string, error) {
		data, err := item.Load()
		return string(data), err
	})
	w.Serve()
}
