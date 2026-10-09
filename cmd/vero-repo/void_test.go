package main

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVoid: a Void package holds props.plist, files.plist and the app,
// for glibc and musl alike; the repository signs each package, and its
// repodata lists the newest, with its hash, and carries the key.
func TestVoid(t *testing.T) {
	a, workers := linuxApp(t)
	a.GTK.Void = &LinuxPkg{}
	pkgs, site := t.TempDir(), t.TempDir()
	if err := packageVoid(a, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	built, _ := filepath.Glob(filepath.Join(pkgs, "*.xbps"))
	if len(built) != 4 {
		t.Fatalf("built %v", built)
	}
	file := "vero-example-1.2.3.beta_1.x86_64-musl.xbps"
	props, files, err := voidContents(mustRead(t, filepath.Join(pkgs, file)))
	if err != nil {
		t.Fatal(err)
	}
	if props["pkgver"] != "vero-example-1.2.3.beta_1" || props["architecture"] != "x86_64-musl" {
		t.Errorf("props: %v", props)
	}
	if deps, _ := props["run_depends"].([]any); len(deps) != 3 || deps[0] != "python3>=0" {
		t.Errorf("run_depends: %v", props["run_depends"])
	}
	if !strings.Contains(" "+strings.Join(files, " ")+" ", " ./usr/lib/vero-example/worker ") {
		t.Errorf("files: %v", files)
	}

	s := testKey(t)
	newest, err := buildVoid(site, built, 3, a, s)
	if err != nil {
		t.Fatal(err)
	}
	if newest["x86_64-musl"].URL != "void/"+file || newest["aarch64"].Version != "1.2.3.beta" {
		t.Errorf("newest: %v", newest)
	}
	index, meta, err := readRepodata(mustRead(t, filepath.Join(site, "void", "x86_64-musl-repodata")))
	if err != nil {
		t.Fatal(err)
	}
	pem, _ := s.rsaPublicPEM()
	if !bytes.Equal(meta["public-key"].([]byte), pem) || meta["public-key-size"] != int64(rsaPublic(t, pem).N.BitLen()) || meta["signature-type"] != "rsa" {
		t.Errorf("index-meta: %v", meta)
	}
	pkg := mustRead(t, filepath.Join(site, "void", file))
	entry, _ := index["vero-example"].(plistDict)
	sum := sha256.Sum256(pkg)
	if entry["filename-sha256"] != hex.EncodeToString(sum[:]) || entry["filename-size"] != int64(len(pkg)) {
		t.Errorf("index: %v", entry)
	}
	if rsa.VerifyPKCS1v15(rsaPublic(t, pem), crypto.SHA256, sum[:], mustRead(t, filepath.Join(site, "void", file+".sig2"))) != nil {
		t.Error("the package's signature doesn't check out")
	}

	// The key as xbps keeps one it trusts, named by its fingerprint.
	trusted, err := parsePlist(mustRead(t, filepath.Join(site, "void", voidFingerprint(rsaPublic(t, pem))+".plist")))
	if err != nil || !bytes.Equal(trusted["public-key"].([]byte), pem) || trusted["signature-by"] != "Example Publisher" {
		t.Errorf("the trusted key: %v %v", trusted, err)
	}

	c := &checker{site: site, url: "https://example.com", a: a}
	c.checkVoid()
	if len(c.problems) > 0 {
		t.Fatal(c.problems)
	}
	os.WriteFile(filepath.Join(site, "void", file), append(pkg, 0), 0o644)
	c.checkVoid()
	if len(c.problems) == 0 {
		t.Error("a changed package passed")
	}
}
