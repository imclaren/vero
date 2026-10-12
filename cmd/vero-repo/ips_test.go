package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestIPSJSON checks that the catalog files are written as OpenIndiana's
// pkgsend wrote them, signatures and all: testdata/ips is its catalog of a
// package made with it, which ipsJSON must write again byte for byte.
func TestIPSJSON(t *testing.T) {
	for _, name := range []string{"catalog.attrs", "catalog.base.C", "catalog.dependency.C", "catalog.summary.C"} {
		want, err := os.ReadFile(filepath.Join("testdata", "ips", name))
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		if err := json.Unmarshal(want, &v); err != nil {
			t.Fatal(err)
		}
		delete(v, "_SIGNATURE")
		got, _, err := ipsJSON(v)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s:\n got %s\nwant %s", name, got, want)
		}
	}
	// As Python's isoformat writes them, which pkg checks the catalog by.
	for when, want := range map[time.Time]string{
		time.Date(2026, 10, 12, 9, 41, 4, 0, time.UTC):         "20261012T094104Z",
		time.Date(2026, 10, 12, 9, 5, 49, 991484000, time.UTC): "20261012T090549.991484Z",
		time.Date(2026, 10, 12, 9, 5, 49, 991484999, time.UTC): "20261012T090549.991484Z",
	} {
		if got := ipsCatalogTime(when); got != want {
			t.Errorf("%v: %s, want %s", when, got, want)
		}
	}
	if got := string(asciiJSON([]byte(`"café …"`))); got != `"caf\u00e9 \u2026"` {
		t.Errorf("ASCII: %s", got)
	}
}

// TestIPSRepo builds two versions of a package and the repository from
// them, and checks it as pkg will read it: the catalog's signatures, each
// manifest by its SHA-1, and each file by its contents'.
func TestIPSRepo(t *testing.T) {
	dir := t.TempDir()
	a := &App{Name: "example", ID: "dev.vero.example", Summary: "An example, \"quoted\".", Description: "Café,\nover two lines.",
		GTK: &GTK{Uses: []string{"adwaita", "webview"}, Illumos: &Pkgsrc{}}}
	if err := a.GTK.resolveDeps(); err != nil {
		t.Fatal(err)
	}
	sys := system("illumos")
	files := []treeFile{
		{"/usr/bin/example", []byte("#!/bin/sh\nexec /usr/bin/python3.9 /usr/lib/example/main.py \"$@\"\n"), 0o755},
		{"/usr/lib/example/main.py", []byte("print('hello')\n"), 0o644},
		{"/usr/share/applications/dev.vero.example.desktop", []byte("[Desktop Entry]\n"), 0o644},
	}
	var pkgs []string
	for _, v := range []string{"1.0.0", "1.0.1"} {
		a.Version = v
		name := filepath.Join(dir, unixPackageName(a, sys, sys.arches[0]))
		if err := writeIPS(name, a, sys, sys.arches[0], files); err != nil {
			t.Fatal(err)
		}
		pkgs = append(pkgs, name)
	}
	// The manifest in the newest package.
	data, _ := os.ReadFile(pkgs[1])
	tr := tar.NewReader(bytes.NewReader(data))
	tr.Next()
	manifest, _ := io.ReadAll(tr)
	for _, want := range []string{
		"set name=pkg.fmri value=pkg://dev.vero.example/application/example@1.0.1,5.11-0:",
		`set name=pkg.summary value="An example, \"quoted\"."`,
		"dir group=bin mode=0755 owner=root path=usr/lib/example\n",
		`set name=pkg.description value="Café, over two lines."`,
		"group=bin mode=0755 owner=root path=usr/bin/example pkg.content-hash=file:sha512t_256:",
		"restart_fmri=svc:/application/desktop-cache/desktop-mime-cache:default",
		"depend fmri=library/desktop/gtk4 type=require\n",
		"depend fmri=library/desktop/libadwaita type=require\n",
		"depend fmri=library/desktop/webkitgtk2-60 type=group\n",
	} {
		if !strings.Contains(string(manifest), want) {
			t.Errorf("manifest lacks %q:\n%s", want, manifest)
		}
	}
	if strings.Contains(string(manifest), "path=usr/bin\n") {
		t.Error("the package delivers /usr/bin, which is OpenIndiana's")
	}
	site := filepath.Join(dir, "site")
	newest, err := buildIPSRepo(site, sys, pkgs, 3, a)
	if err != nil {
		t.Fatal(err)
	}
	if newest["x86_64"].Version != "1.0.1" {
		t.Errorf("newest %+v", newest)
	}
	c := &checker{site: site}
	c.checkIPS(*sys)
	if len(c.problems) > 0 {
		t.Errorf("the repository:\n%s", strings.Join(c.problems, "\n"))
	}
	attrs, _ := os.ReadFile(filepath.Join(site, "illumos", "catalog", "1", "catalog.attrs"))
	if !strings.Contains(string(attrs), `"package-version-count":2`) {
		t.Errorf("attrs %s", attrs)
	}
	for _, rel := range []string{"versions/0/index.html", "publisher/0/index.html", "dev.vero.example/catalog/1/catalog.base.C",
		"dev.vero.example/manifest/0/application%2Fexample@" + strings.TrimPrefix(string(regexpFind(manifest)), "")} {
		if _, err := os.Stat(filepath.Join(site, "illumos", filepath.FromSlash(rel))); err != nil {
			t.Error(err)
		}
	}
	// A version that is not numbers is refused.
	a.Version = "1.0.2-beta"
	if err := writeIPS(filepath.Join(dir, "x.ips"), a, sys, sys.arches[0], files); err == nil {
		t.Error("1.0.2-beta was taken")
	}
}

// regexpFind is the version part of a manifest's FMRI.
func regexpFind(manifest []byte) []byte {
	line := manifest[:bytes.IndexByte(manifest, '\n')]
	return line[bytes.IndexByte(line, '@')+1:]
}
