package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// rsaPublic is the RSA public key a site publishes as PEM.
func rsaPublic(t *testing.T, data []byte) *rsa.PublicKey {
	t.Helper()
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("not PEM")
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return k.(*rsa.PublicKey)
}

func TestAlpineVersion(t *testing.T) {
	for v, want := range map[string]string{"1.2.3": "1.2.3", "1.2.3-beta": "1.2.3_beta", "2.0-rc.1": "2.0_rc1", "1.0a": "1.0a"} {
		if got, err := alpineVersion(v); err != nil || got != want {
			t.Errorf("alpineVersion(%s) = %s, %v; want %s", v, got, err, want)
		}
	}
	if _, err := alpineVersion("1.2-nightly"); err == nil {
		t.Error("1.2-nightly was taken")
	}
}

// TestAlpine: an Alpine package is its control segment and its files,
// each file with its SHA-1, the control's datahash theirs; the repository
// signs each package and APKINDEX with the key it publishes, and the
// index knows each package by its control segment's hash.
func TestAlpine(t *testing.T) {
	a, workers := linuxApp(t)
	a.GTK.Alpine = &LinuxPkg{Arches: []string{"x86_64", "aarch64", "armv7"}}
	if err := a.GTK.checkArches(); err != nil {
		t.Fatal(err)
	}
	pkgs, site := t.TempDir(), t.TempDir()
	if err := packageAlpine(a, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	built, _ := filepath.Glob(filepath.Join(pkgs, "*-r0-*.apk"))
	if len(built) != 3 {
		t.Fatalf("built %v", built)
	}
	unsigned := mustRead(t, filepath.Join(pkgs, "vero-example-1.2.3_beta-r0-armv7.apk"))
	streams, err := gzipStreams(unsigned)
	if err != nil || len(streams) != 2 {
		t.Fatalf("%d streams: %v", len(streams), err)
	}
	info, _, err := apkInfo(unsigned)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(streams[1])
	if pkgInfo(info, "datahash") != hex.EncodeToString(sum[:]) || pkgInfo(info, "arch") != "armv7" || pkgInfo(info, "pkgver") != "1.2.3_beta-r0" {
		t.Errorf(".PKGINFO: %v", info)
	}
	if got := strings.Join(pkgInfoAll(info, "depend"), " "); got != "python3 py3-gobject3 gtk4.0" {
		t.Errorf("depends on %s", got)
	}
	z, _ := gzip.NewReader(bytes.NewReader(streams[1]))
	tr := tar.NewReader(z)
	found := false
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		if h.Name == "usr/lib/vero-example/worker" {
			body, _ := io.ReadAll(tr)
			s1 := sha1.Sum(body)
			found = h.PAXRecords["APK-TOOLS.checksum.SHA1"] == hex.EncodeToString(s1[:])
		}
	}
	if !found {
		t.Error("the worker isn't there with its SHA-1")
	}

	s := testKey(t)
	newest, err := buildAlpine(site, built, 3, a, s)
	if err != nil {
		t.Fatal(err)
	}
	if newest["x86_64"].URL != "alpine/x86_64/vero-example-1.2.3_beta-r0.apk" {
		t.Errorf("newest: %v", newest)
	}
	pub := rsaPublic(t, mustRead(t, filepath.Join(site, "alpine", "vero-example.rsa.pub")))
	signed := func(what string, data []byte) [][]byte {
		streams, err := gzipStreams(data)
		if err != nil || len(streams) < 2 {
			t.Fatalf("%s: %v", what, err)
		}
		files, _, err := gunzipTar(streams[0])
		if err != nil {
			t.Fatal(err)
		}
		sig, ok := files[".SIGN.RSA256.vero-example.rsa.pub"]
		sum := sha256.Sum256(streams[1])
		if !ok || rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) != nil {
			t.Errorf("%s: the signature doesn't check out", what)
		}
		return streams
	}
	pkg := mustRead(t, filepath.Join(site, "alpine", "aarch64", "vero-example-1.2.3_beta-r0.apk"))
	control := signed("the package", pkg)[1]
	index := signed("APKINDEX", mustRead(t, filepath.Join(site, "alpine", "aarch64", "APKINDEX.tar.gz")))
	files, _, err := gunzipTar(index[1])
	if err != nil {
		t.Fatal(err)
	}
	c := sha1.Sum(control)
	entry := string(files["APKINDEX"])
	for _, want := range []string{"C:Q1" + base64.StdEncoding.EncodeToString(c[:]) + "\n", "P:vero-example\n", "V:1.2.3_beta-r0\n", "A:aarch64\n", "S:" + strconv.Itoa(len(pkg)) + "\n", "D:python3 py3-gobject3 gtk4.0\n"} {
		if !strings.Contains(entry, want) {
			t.Errorf("APKINDEX lacks %q:\n%s", want, entry)
		}
	}

	ch := &checker{site: site, url: "https://example.com", a: a}
	ch.checkApk(alpineSystem)
	if len(ch.problems) > 0 {
		t.Fatal(ch.problems)
	}
	os.WriteFile(filepath.Join(site, "alpine", "aarch64", "vero-example-1.2.3_beta-r0.apk"), append(pkg, 0), 0o644)
	ch.checkApk(alpineSystem)
	if len(ch.problems) == 0 {
		t.Error("a changed package passed")
	}
}
