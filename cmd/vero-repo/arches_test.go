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

// TestLinuxArches: RISC-V, POWER and 32-bit ARM are packaged only when a
// section names them, under each kind of package's own name for them,
// and only where that kind has packages for them; apt indexes them.
func TestLinuxArches(t *testing.T) {
	a, workers := linuxApp(t)
	names := func(section string) string {
		var out []string
		for _, arch := range a.linuxArches(section) {
			out = append(out, arch.name)
		}
		return strings.Join(out, " ")
	}
	if got := names("deb"); got != "amd64 arm64" {
		t.Errorf("deb's arches are %s, without any named", got)
	}
	a.GTK.Deb.Arches = []string{"amd64", "riscv64", "ppc64el", "armhf"}
	a.GTK.RPM.Arches = []string{"x86_64", "riscv64", "ppc64le"}
	if err := a.GTK.checkArches(); err != nil {
		t.Fatal(err)
	}
	if got := names("deb"); got != "amd64 riscv64 ppc64el armhf" {
		t.Errorf("deb's arches are %s", got)
	}
	if got := names("rpm"); got != "x86_64 riscv64 ppc64le" {
		t.Errorf("rpm's arches are %s", got)
	}
	if got := strings.Join(a.linuxGoarches("deb", "rpm"), " "); got != "amd64 riscv64 ppc64le arm" {
		t.Errorf("the workers needed are %s", got)
	}
	for section, arch := range map[string]string{"rpm": "armhf", "flatpak": "riscv64"} {
		b := *a.GTK
		b.RPM.Arches, b.Flatpak.Arches = nil, nil
		if section == "rpm" {
			b.RPM.Arches = []string{arch}
		} else {
			b.Flatpak.Arches = []string{arch}
		}
		if err := b.checkArches(); err == nil {
			t.Errorf("[gtk.%s] took %s", section, arch)
		}
	}

	pkgs, site := t.TempDir(), t.TempDir()
	if err := packageDeb(a, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	if err := packageRPM(a, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	debs, _ := filepath.Glob(filepath.Join(pkgs, "*.deb"))
	rpms, _ := filepath.Glob(filepath.Join(pkgs, "*.rpm"))
	if len(debs) != 4 || len(rpms) != 3 {
		t.Fatalf("packaged %v %v", debs, rpms)
	}
	if c, err := readControl(filepath.Join(pkgs, "vero-example_1.2.3-beta_armhf.deb")); err != nil || c.Get("Architecture") != "armhf" {
		t.Errorf("the armhf .deb says %v (%v)", c, err)
	}
	s := testKey(t)
	if _, err := buildApt(site, debs, 3, a, s); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64", "riscv64", "ppc64el", "armhf"} {
		if _, err := os.Stat(filepath.Join(site, "apt", "dists", "stable", "main", "binary-"+arch, "Packages")); err != nil {
			t.Errorf("no index for %s: %v", arch, err)
		}
	}
	if _, err := buildRPM(site, rpms, 3, "https://example.com", a, s); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(site, publicFile), s.public, 0o644)
	c := &checker{site: site, url: "https://example.com", a: a}
	c.keyring, _ = loadKeyring(s.public)
	c.checkApt()
	c.checkRPM()
	if len(c.problems) > 0 {
		t.Fatal(c.problems)
	}
}
