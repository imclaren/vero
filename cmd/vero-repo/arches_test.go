package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestArches: a section's arches narrows what's packaged and put in the
// repository, without moving where the repository goes; an architecture
// the system doesn't have is refused.
func TestArches(t *testing.T) {
	a, workers := bsdApp(t)
	a.GTK.NetBSD.Arches = []string{"x86_64"}
	a.GTK.FreeBSD.Arches = []string{"amd64"}
	a.GTK.Deb.Arches = []string{"x64"}
	if err := a.GTK.checkArches(); err != nil {
		t.Fatal(err)
	}
	pkgs := t.TempDir()
	for _, name := range []string{"netbsd", "freebsd"} {
		if err := packageUnix(a, system(name), workers, pkgs); err != nil {
			t.Fatal(err)
		}
	}
	if err := packageDeb(a, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	var got []string
	entries, _ := os.ReadDir(pkgs)
	for _, e := range entries {
		got = append(got, e.Name())
	}
	sort.Strings(got)
	want := "vero-example-1.2.3.beta-freebsd-amd64.pkg vero-example-1.2.3.beta-netbsd-x86_64.tgz vero-example_1.2.3-beta_amd64.deb"
	if strings.Join(got, " ") != want {
		t.Fatalf("packaged %v, want %s", got, want)
	}

	// The repositories: only x86_64's, in the folders they'd have anyway.
	site := t.TempDir()
	var netbsd []string
	for _, p := range got {
		if strings.Contains(p, "netbsd") {
			netbsd = append(netbsd, filepath.Join(pkgs, p))
		}
	}
	if _, err := buildPkgsrcRepo(site, system("netbsd"), netbsd, 3, a, testKey(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(site, "netbsd", "x86_64", "All", "pkg_summary.gz")); err != nil {
		t.Errorf("no x86_64 repository: %v", err)
	}
	if _, err := os.Stat(filepath.Join(site, "netbsd", "aarch64")); err == nil {
		t.Error("an aarch64 repository, which arches leaves out")
	}

	// What a system doesn't have, and what isn't an architecture.
	a.GTK.DragonFly = &BSDPkg{Arches: []string{"arm64"}}
	if err := a.GTK.checkArches(); err == nil || !strings.Contains(err.Error(), "[gtk.dragonfly]") {
		t.Errorf("DragonFly on arm64 was taken: %v", err)
	}
	a.GTK.DragonFly = nil
	a.GTK.RPM.Arches = []string{"sparc"}
	if err := a.GTK.checkArches(); err == nil {
		t.Error("sparc was taken")
	}
}
