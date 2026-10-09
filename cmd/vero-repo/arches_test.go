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

// TestWindowsArches: x64 and arm64 unless [wpf] says, x86 when it's named,
// by Windows' or Go's name, and nothing else.
func TestWindowsArches(t *testing.T) {
	w := &WPF{}
	if got, _ := w.arches(); strings.Join(got, " ") != "x64 arm64" {
		t.Errorf("without arches: %v", got)
	}
	w.Arches = []string{"amd64", "arm64", "386"}
	if got, err := w.arches(); err != nil || strings.Join(got, " ") != "x64 arm64 x86" {
		t.Errorf("arches %v: %v %v", w.Arches, got, err)
	}
	w.Arches = []string{"ia64"}
	if _, err := w.arches(); err == nil {
		t.Error("ia64 was taken")
	}
	if !windowsInstaller("vero-example").MatchString("vero-example-1.2.3-x86-setup.exe") {
		t.Error("an x86 installer isn't one the site takes")
	}
}

// TestMoreLinuxArches: 32-bit Intel, LoongArch, big-endian POWER, IBM Z
// and MIPS, each under its system's own name, packaged only when named;
// Void's i686 only for glibc, as Void has it; and every repository indexes
// them and passes the check.
func TestMoreLinuxArches(t *testing.T) {
	a, workers := linuxApp(t)
	a.GTK.Alpine, a.GTK.Void = &LinuxPkg{}, &LinuxPkg{}
	for _, section := range []string{"deb", "rpm", "arch", "alpine", "void"} {
		for _, arch := range a.linuxArches(section) {
			if arch.goarch != "amd64" && arch.goarch != "arm64" {
				t.Errorf("[gtk.%s] has %s without naming it", section, arch.name)
			}
		}
	}
	a.GTK.Deb.Arches = []string{"i386", "loong64", "ppc64", "s390x", "mips64el", "mipsel"}
	a.GTK.RPM.Arches = []string{"i686", "s390x"}
	a.GTK.Arch.Arches = []string{"i686", "loong64"}
	a.GTK.Alpine.Arches = []string{"x86", "loongarch64", "s390x"}
	a.GTK.Void.Arches = []string{"i686"}
	if err := a.GTK.checkArches(); err != nil {
		t.Fatal(err)
	}
	names := func(section string) string {
		var out []string
		for _, arch := range a.linuxArches(section) {
			out = append(out, arch.name)
		}
		return strings.Join(out, " ")
	}
	for section, want := range map[string]string{
		"deb": "i386 loong64 ppc64 s390x mips64el mipsel", "rpm": "i686 s390x", "arch": "i686 loong64",
		"alpine": "x86 loongarch64 s390x", "void": "i686",
	} {
		if got := names(section); got != want {
			t.Errorf("[gtk.%s]'s arches are %s, want %s", section, got, want)
		}
	}
	if got := strings.Join(a.linuxGoarches("deb", "rpm", "arch", "alpine", "void"), " "); got != "386 loong64 ppc64 s390x mips64le mipsle" {
		t.Errorf("the workers needed are %s", got)
	}
	// What each system has no packages for.
	for section, arch := range map[string]string{"rpm": "loong64", "arch": "s390x", "alpine": "ppc64", "void": "s390x", "flatpak": "i386"} {
		b := *a.GTK
		b.Deb, b.RPM, b.Arch, b.Alpine, b.Void = Deb{}, RPM{}, Arch{}, &LinuxPkg{}, &LinuxPkg{}
		switch section {
		case "rpm":
			b.RPM.Arches = []string{arch}
		case "arch":
			b.Arch.Arches = []string{arch}
		case "alpine":
			b.Alpine.Arches = []string{arch}
		case "void":
			b.Void.Arches = []string{arch}
		case "flatpak":
			b.Flatpak.Arches = []string{arch}
		}
		if err := b.checkArches(); err == nil {
			t.Errorf("[gtk.%s] took %s", section, arch)
		}
	}
	if env := strings.Join(goarchEnv("386"), " "); env != "GO386=sse2" {
		t.Errorf("386 is built with %q", env)
	}
	if env := strings.Join(goarchEnv("arm"), " "); env != "GOARM=7" {
		t.Errorf("arm is built with %q", env)
	}

	pkgs, site := t.TempDir(), t.TempDir()
	for _, pack := range []func(*App, map[string]string, string) error{packageDeb, packageRPM, packagePacman, packageAlpine, packageVoid} {
		if err := pack(a, workers, pkgs); err != nil {
			t.Fatal(err)
		}
	}
	glob := func(pattern string) []string {
		files, _ := filepath.Glob(filepath.Join(pkgs, pattern))
		return files
	}
	debs, rpms, pacs, apks, xbps := glob("*.deb"), glob("*.rpm"), glob("*.pkg.tar.zst"), glob("*.apk"), glob("*.xbps")
	if len(debs) != 6 || len(rpms) != 2 || len(pacs) != 2 || len(apks) != 3 || len(xbps) != 1 {
		t.Fatalf("packaged %v %v %v %v %v", debs, rpms, pacs, apks, xbps)
	}
	if filepath.Base(xbps[0]) != "vero-example-1.2.3.beta_1.i686.xbps" {
		t.Errorf("Void's package is %s", xbps[0])
	}
	if c, err := readControl(filepath.Join(pkgs, "vero-example_1.2.3-beta_mips64el.deb")); err != nil || c.Get("Architecture") != "mips64el" {
		t.Errorf("the mips64el .deb says %v (%v)", c, err)
	}

	s := testKey(t)
	if _, err := buildApt(site, debs, 3, a, s); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"i386", "loong64", "ppc64", "s390x", "mips64el", "mipsel"} {
		if _, err := os.Stat(filepath.Join(site, "apt", "dists", "stable", "main", "binary-"+arch, "Packages")); err != nil {
			t.Errorf("no index for %s: %v", arch, err)
		}
	}
	newest, err := buildRPM(site, rpms, 3, "https://example.com", a, s)
	if err != nil || newest["i686"].URL == "" || newest["s390x"].URL == "" {
		t.Fatalf("rpm: %v %v", newest, err)
	}
	if newest, err = buildPacman(site, pacs, 3, a, s); err != nil || newest["i686"].URL == "" || newest["loong64"].URL == "" {
		t.Fatalf("pacman: %v %v", newest, err)
	}
	if newest, err = buildAlpine(site, apks, 3, a, s); err != nil || newest["x86"].URL == "" || newest["loongarch64"].URL == "" || newest["s390x"].URL == "" {
		t.Fatalf("alpine: %v %v", newest, err)
	}
	if newest, err = buildVoid(site, xbps, 3, a, s); err != nil || newest["i686"].URL == "" || len(newest) != 1 {
		t.Fatalf("void: %v %v", newest, err)
	}
	for _, f := range []string{"arch/i686/vero-example.db", "arch/loong64/vero-example.db", "alpine/x86/APKINDEX.tar.gz", "alpine/loongarch64/APKINDEX.tar.gz", "void/i686-repodata"} {
		if _, err := os.Stat(filepath.Join(site, f)); err != nil {
			t.Errorf("no %s: %v", f, err)
		}
	}
	os.WriteFile(filepath.Join(site, publicFile), s.public, 0o644)
	c := &checker{site: site, url: "https://example.com", a: a}
	c.keyring, _ = loadKeyring(s.public)
	c.checkApt()
	c.checkRPM()
	c.checkPacman()
	c.checkAlpine()
	c.checkVoid()
	if len(c.problems) > 0 {
		t.Fatal(c.problems)
	}
}

// TestMoreBSDArches: FreeBSD on 32-bit Intel and ARM, NetBSD on 32-bit
// Intel, and OpenBSD on 32-bit Intel and ARM and on RISC-V, only when
// named, each in the folder its package manager looks in, and the check
// passes without the architectures left out.
func TestMoreBSDArches(t *testing.T) {
	a, workers := bsdApp(t)
	for _, name := range []string{"freebsd", "netbsd", "openbsd"} {
		for _, arch := range a.archesFor(system(name)) {
			if arch.goarch != "amd64" && arch.goarch != "arm64" {
				t.Errorf("%s has %s without naming it", name, arch.name)
			}
		}
	}
	a.GTK.FreeBSD.Arches = []string{"amd64", "i386", "armv7"}
	a.GTK.NetBSD.Arches = []string{"i386"}
	a.GTK.OpenBSD.Arches = []string{"i386", "armv7", "riscv64"}
	if err := a.GTK.checkArches(); err != nil {
		t.Fatal(err)
	}
	pkgs := t.TempDir()
	for _, name := range []string{"freebsd", "netbsd", "openbsd"} {
		if err := packageUnix(a, system(name), workers, pkgs); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	entries, _ := os.ReadDir(pkgs)
	for _, e := range entries {
		got = append(got, e.Name())
	}
	sort.Strings(got)
	want := "vero-example-1.2.3.beta-freebsd-amd64.pkg vero-example-1.2.3.beta-freebsd-armv7.pkg vero-example-1.2.3.beta-freebsd-i386.pkg " +
		"vero-example-1.2.3.beta-netbsd-i386.tgz " +
		"vero-example-1.2.3.beta-openbsd-armv7.tgz vero-example-1.2.3.beta-openbsd-i386.tgz vero-example-1.2.3.beta-openbsd-riscv64.tgz"
	if strings.Join(got, " ") != want {
		t.Fatalf("packaged %v, want %s", got, want)
	}
	m, err := pkgCompactManifest(mustRead(t, filepath.Join(pkgs, "vero-example-1.2.3.beta-freebsd-armv7.pkg")))
	if err != nil || m["abi"] != "FreeBSD:*:armv7" || m["arch"] != "freebsd:*:armv7:32:el:eabi:hardfp" {
		t.Errorf("the armv7 package says %v (%v)", m, err)
	}
	if m, _ = pkgCompactManifest(mustRead(t, filepath.Join(pkgs, "vero-example-1.2.3.beta-freebsd-i386.pkg"))); m["abi"] != "FreeBSD:*:i386" {
		t.Errorf("the i386 package says %v", m)
	}

	site := t.TempDir()
	s := testKey(t)
	of := func(name string) []string {
		var out []string
		for _, p := range got {
			if strings.Contains(p, "-"+name+"-") {
				out = append(out, filepath.Join(pkgs, p))
			}
		}
		return out
	}
	if _, err := buildPkgRepo(site, system("freebsd"), of("freebsd"), 3, "https://example.com", a, s); err != nil {
		t.Fatal(err)
	}
	if _, err := buildPkgsrcRepo(site, system("netbsd"), of("netbsd"), 3, a, s); err != nil {
		t.Fatal(err)
	}
	if _, err := buildOpenBSDRepo(site, system("openbsd"), of("openbsd"), 3, a, s); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"freebsd/i386/packagesite.pkg", "freebsd/armv7/packagesite.pkg", "netbsd/i386/All/pkg_summary.gz"} {
		if _, err := os.Stat(filepath.Join(site, f)); err != nil {
			t.Errorf("no %s: %v", f, err)
		}
	}
	for _, arch := range []string{"i386", "armv7", "riscv64"} {
		if files, _ := filepath.Glob(filepath.Join(site, "openbsd", arch, "*.tgz")); len(files) != 1 {
			t.Errorf("openbsd/%s has %v", arch, files)
		}
	}
	gz := mustRead(t, filepath.Join(site, "netbsd", "i386", "All", "pkg_summary.gz"))
	if summary, err := decompress(".gz", gz); err != nil || !strings.Contains(string(summary), "MACHINE_ARCH=i386\n") {
		t.Errorf("NetBSD's summary: %s (%v)", summary, err)
	}
	c := &checker{site: site, url: "https://example.com", a: a}
	for _, name := range []string{"freebsd", "netbsd", "openbsd"} {
		switch sys := *system(name); sys.format {
		case "pkg":
			c.checkPkg(sys)
		case "pkgsrc":
			c.checkPkgsrc(sys)
		case "openbsd":
			c.checkOpenBSD(sys)
		}
	}
	if len(c.problems) > 0 {
		t.Fatal(c.problems)
	}
}
