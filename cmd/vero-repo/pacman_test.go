package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// TestPacman: an Arch package holds .PKGINFO, .MTREE and the app; the
// repository's indexes list it with its hashes and signature, signed,
// as repo-add would; and check finds a changed package.
func TestPacman(t *testing.T) {
	a, workers := linuxApp(t)
	a.GTK.Arch.Depends = "python, python-gobject, gtk4"
	pkgs, site := t.TempDir(), t.TempDir()
	if err := packagePacman(a, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	built, _ := filepath.Glob(filepath.Join(pkgs, "*.pkg.tar.zst"))
	if len(built) != 2 {
		t.Fatalf("built %v", built)
	}
	file := "vero-example-1.2.3_beta-1-x86_64.pkg.tar.zst"
	info, files, err := readPacman(mustRead(t, filepath.Join(pkgs, file)))
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"pkgname": "vero-example", "pkgver": "1.2.3_beta-1", "arch": "x86_64", "license": "MIT"} {
		if got := pkgInfo(info, k); got != want {
			t.Errorf(".PKGINFO's %s is %q, want %q", k, got, want)
		}
	}
	if got := strings.Join(pkgInfoAll(info, "depend"), ","); got != "python,python-gobject,gtk4" {
		t.Errorf("depends on %s", got)
	}
	list := strings.Join(files, " ")
	for _, want := range []string{"usr/", "usr/lib/vero-example/", "usr/lib/vero-example/worker", "usr/bin/vero-example"} {
		if !strings.Contains(" "+list+" ", " "+want+" ") {
			t.Errorf("the package lacks %s: %s", want, list)
		}
	}

	s := testKey(t)
	newest, err := buildPacman(site, built, 3, a, s)
	if err != nil {
		t.Fatal(err)
	}
	if newest["x86_64"].URL != "arch/x86_64/"+file || newest["aarch64"].Version != "1.2.3_beta" {
		t.Errorf("newest: %v", newest)
	}
	dir := filepath.Join(site, "arch", "x86_64")
	db := mustRead(t, filepath.Join(dir, "vero-example.db"))
	kr, _ := openpgp.ReadArmoredKeyRing(bytes.NewReader(s.public))
	if _, err := openpgp.CheckDetachedSignature(kr, bytes.NewReader(db), bytes.NewReader(mustRead(t, filepath.Join(dir, "vero-example.db.sig"))), nil); err != nil {
		t.Errorf("the index's signature: %v", err)
	}
	members, err := tarMembers(db)
	if err != nil {
		t.Fatal(err)
	}
	desc := pacmanFields(members["vero-example-1.2.3_beta-1/desc"])
	sig := mustRead(t, filepath.Join(dir, file+".sig"))
	if desc["FILENAME"] != file || desc["VERSION"] != "1.2.3_beta-1" || desc["PGPSIG"] != base64.StdEncoding.EncodeToString(sig) || desc["DEPENDS"] != "python\npython-gobject\ngtk4" {
		t.Errorf("desc: %v", desc)
	}
	filesDB, err := tarMembers(mustRead(t, filepath.Join(dir, "vero-example.files")))
	if err != nil || !strings.Contains(string(filesDB["vero-example-1.2.3_beta-1/files"]), "usr/lib/vero-example/worker\n") {
		t.Errorf("the files index: %v %q", err, filesDB["vero-example-1.2.3_beta-1/files"])
	}

	os.WriteFile(filepath.Join(site, publicFile), s.public, 0o644)
	c := &checker{site: site, url: "https://example.com", a: a, keyring: kr}
	c.checkPacman()
	if len(c.problems) > 0 {
		t.Fatal(c.problems)
	}

	// A newer version replaces it in the indexes, as pacman needs: one
	// version of each package.
	a.Version = "1.2.4"
	pkgs = t.TempDir()
	if err := packagePacman(a, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	built, _ = filepath.Glob(filepath.Join(pkgs, "*.pkg.tar.zst"))
	if newest, err = buildPacman(site, built, 3, a, s); err != nil {
		t.Fatal(err)
	}
	members, _ = tarMembers(mustRead(t, filepath.Join(dir, "vero-example.db")))
	if newest["x86_64"].Version != "1.2.4" || len(members) != 1 || members["vero-example-1.2.4-1/desc"] == nil {
		t.Errorf("after 1.2.4: newest %v, index %d entries", newest, len(members))
	}
	os.WriteFile(filepath.Join(dir, "vero-example-1.2.4-1-x86_64.pkg.tar.zst"), []byte("changed"), 0o644)
	c.checkPacman()
	if len(c.problems) == 0 {
		t.Error("a changed package passed")
	}
}
