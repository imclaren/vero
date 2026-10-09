package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// TestOlderARM: ARMv6 and ARMv5 are packaged only when named, by each
// system's own name for them, beside ARMv7, and their workers are built
// for arm with GOARM=6 and 5. Debian's armhf is built for ARMv6, which
// Raspberry Pi OS's armhf is.
func TestOlderARM(t *testing.T) {
	a, workers := linuxApp(t)
	a.GTK.Deb.Arches = []string{"armhf", "armel"}
	a.GTK.Alpine = &LinuxPkg{Arches: []string{"armv7", "armhf"}}
	a.GTK.Void = &LinuxPkg{Arches: []string{"armv7l", "armv6l"}}
	if err := a.GTK.checkArches(); err != nil {
		t.Fatal(err)
	}
	for section, want := range map[string]string{"deb": "armv6:armhf armv5:armel", "alpine": "arm:armv7 armv6:armhf", "void": "arm:armv7l armv6:armv6l"} {
		var got []string
		for _, arch := range a.linuxArches(section) {
			got = append(got, arch.goarch+":"+arch.name)
		}
		if strings.Join(got, " ") != want {
			t.Errorf("%s's arches are %v, want %s", section, got, want)
		}
	}
	for arch, want := range map[string]string{"arm": "arm GOARM=7", "armv6": "arm GOARM=6", "armv5": "arm GOARM=5", "386": "386 GO386=sse2"} {
		if got := goBuildArch(arch) + " " + strings.Join(goarchEnv(arch), " "); got != want {
			t.Errorf("%s is built as %s, want %s", arch, got, want)
		}
	}
	b := *a.GTK
	b.RPM.Arches = []string{"armel"}
	if err := b.checkArches(); err == nil {
		t.Error("[gtk.rpm] took armel")
	}

	pkgs, site := t.TempDir(), t.TempDir()
	for _, pack := range []func(*App, map[string]string, string) error{packageDeb, packageAlpine, packageVoid} {
		if err := pack(a, workers, pkgs); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	entries, _ := os.ReadDir(pkgs)
	for _, e := range entries {
		got = append(got, e.Name())
	}
	sort.Strings(got)
	want := "vero-example-1.2.3.beta_1.armv6l-musl.xbps vero-example-1.2.3.beta_1.armv6l.xbps vero-example-1.2.3.beta_1.armv7l-musl.xbps vero-example-1.2.3.beta_1.armv7l.xbps vero-example-1.2.3_beta-r0-armhf.apk vero-example-1.2.3_beta-r0-armv7.apk " +
		"vero-example_1.2.3-beta_armel.deb vero-example_1.2.3-beta_armhf.deb"
	if strings.Join(got, " ") != want {
		t.Fatalf("packaged %v, want %s", got, want)
	}
	s := testKey(t)
	glob := func(pattern string) []string {
		files, _ := filepath.Glob(filepath.Join(pkgs, pattern))
		return files
	}
	newest, err := buildAlpine(site, glob("*.apk"), 3, a, s)
	if err != nil || newest["armhf"].URL != "alpine/armhf/vero-example-1.2.3_beta-r0.apk" {
		t.Fatalf("alpine: %v %v", newest, err)
	}
	if newest, err = buildVoid(site, glob("*.xbps"), 3, a, s); err != nil || len(newest) != 4 {
		t.Fatalf("void: %v %v", newest, err)
	}
	if _, err := os.Stat(filepath.Join(site, "void", "armv6l-musl-repodata")); err != nil {
		t.Error("no armv6l-musl repository")
	}
}

// TestChimera: Chimera's packages are Alpine's kind, named apart from
// Alpine's, with Chimera's names for its dependencies and architectures,
// in a signed repository of their own that the check passes.
func TestChimera(t *testing.T) {
	a, workers := linuxApp(t)
	a.GTK.Alpine = &LinuxPkg{}
	a.GTK.Chimera = &LinuxPkg{Arches: []string{"x86_64", "aarch64", "ppc64le", "ppc64", "riscv64", "loongarch64"}}
	if err := a.GTK.checkArches(); err != nil {
		t.Fatal(err)
	}
	b := *a.GTK
	b.Chimera = &LinuxPkg{Arches: []string{"armv7"}}
	if err := b.checkArches(); err == nil {
		t.Error("[gtk.chimera] took armv7")
	}
	pkgs, site := t.TempDir(), t.TempDir()
	if err := packageAlpine(a, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	if err := packageChimera(a, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	chimera, _ := filepath.Glob(filepath.Join(pkgs, "*-r0-chimera-*.apk"))
	all, _ := filepath.Glob(filepath.Join(pkgs, "*-r0-*.apk"))
	if len(chimera) != 6 || len(all) != 8 {
		t.Fatalf("packaged %v", all)
	}
	info, _, err := apkInfo(mustRead(t, filepath.Join(pkgs, "vero-example-1.2.3_beta-r0-chimera-loongarch64.apk")))
	if err != nil || pkgInfo(info, "arch") != "loongarch64" || strings.Join(pkgInfoAll(info, "depend"), " ") != "python python-gobject gtk4" {
		t.Fatalf("Chimera's package says %v (%v)", info, err)
	}
	s := testKey(t)
	// Each repository takes only its own packages, from the same folder.
	alpine, err := buildAlpine(site, all, 3, a, s)
	if err != nil || len(alpine) != 2 {
		t.Fatalf("alpine: %v %v", alpine, err)
	}
	newest, err := buildChimera(site, all, 3, a, s)
	if err != nil || len(newest) != 6 || newest["ppc64"].URL != "chimera/ppc64/vero-example-1.2.3_beta-r0.apk" {
		t.Fatalf("chimera: %v %v", newest, err)
	}
	if _, err := os.Stat(filepath.Join(site, "chimera", alpineKeyName(a))); err != nil {
		t.Error("no key for apk on Chimera")
	}
	c := &checker{site: site, url: "https://example.com", a: a}
	c.checkApk(alpineSystem)
	c.checkApk(chimeraSystem)
	if len(c.problems) > 0 {
		t.Fatal(c.problems)
	}

	// The page and install.sh: Chimera's key and repository, fetched
	// with the fetch it has.
	latest := Latest{Name: a.Name, Version: "1.2.3_beta", Downloads: map[string]Download{"chimera-x86_64": newest["x86_64"]}}
	if err := writeSite(site, "https://example.com", a, latest, s, false); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{
		"index.html": "fetch -qo /etc/apk/keys/vero-example.rsa.pub https://example.com/chimera/vero-example.rsa.pub",
		"install.sh": "echo 'https://example.com/chimera' > /etc/apk/repositories.d/vero-example.list",
	} {
		if !strings.Contains(string(mustRead(t, filepath.Join(site, file))), want) {
			t.Errorf("%s has no %q", file, want)
		}
	}
	if !strings.Contains(string(mustRead(t, filepath.Join(site, "install.sh"))), `grep -qs '^ID="\{0,1\}chimera' /etc/os-release`) {
		t.Error("install.sh doesn't know Chimera")
	}
}

// TestArchOptDepends: what the Arch package can use if it's there is in
// its .PKGINFO, the repository's index and the AUR's recipe.
func TestArchOptDepends(t *testing.T) {
	a, workers := linuxApp(t)
	a.GTK.Arch = Arch{Depends: "python, gtk4", OptDepends: "webkitgtk-6.0: signing in inside the app", Arches: []string{"i686"}}
	pkgs, site := t.TempDir(), t.TempDir()
	if err := packagePacman(a, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(pkgs, "vero-example-1.2.3_beta-1-pentium4.pkg.tar.zst")
	z, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := z.DecodeAll(mustRead(t, file), nil)
	if err != nil || !strings.Contains(string(data), "\noptdepend = webkitgtk-6.0: signing in inside the app\n") {
		t.Fatalf("the package has no optdepend (%v)", err)
	}
	if _, err := buildPacman(site, []string{file}, 3, a, testKey(t)); err != nil {
		t.Fatal(err)
	}
	if desc := pacmanDesc(file, mustRead(t, file), nil, pacmanInfoOf(t, data)); !strings.Contains(string(desc), "%OPTDEPENDS%\nwebkitgtk-6.0: signing in inside the app\n") {
		t.Errorf("the index says %s", desc)
	}
}

// pacmanInfoOf is .PKGINFO's fields, from a package's tar.
func pacmanInfoOf(t *testing.T, data []byte) [][2]string {
	t.Helper()
	i := strings.Index(string(data), "# Generated by vero-repo")
	end := strings.Index(string(data[i:]), "\x00")
	var info [][2]string
	for _, line := range strings.Split(string(data[i:i+end]), "\n") {
		if k, v, ok := strings.Cut(line, " = "); ok {
			info = append(info, [2]string{k, v})
		}
	}
	return info
}

// TestFreeBSDByArch: an architecture's own dependencies and python
// replace the section's there, and only there; a name FreeBSD doesn't have
// is refused.
func TestFreeBSDByArch(t *testing.T) {
	a, workers := bsdApp(t)
	a.GTK.FreeBSD.Deps = map[string]string{"py312-pygobject": "devel/py-pygobject@py312"}
	a.GTK.FreeBSD.Arches = []string{"amd64", "armv7"}
	a.GTK.FreeBSD.ByArch = map[string]BSDArchPkg{"armv7": {Deps: map[string]string{"py311-pygobject": "devel/py-pygobject@py311"}, Python: "python3.11"}}
	if err := a.GTK.checkArches(); err != nil {
		t.Fatal(err)
	}
	pkgs := t.TempDir()
	if err := packageUnix(a, system("freebsd"), workers, pkgs); err != nil {
		t.Fatal(err)
	}
	for arch, want := range map[string]string{"amd64": "py312-pygobject", "armv7": "py311-pygobject"} {
		file := filepath.Join(pkgs, "vero-example-1.2.3.beta-freebsd-"+arch+".pkg")
		m, err := pkgCompactManifest(mustRead(t, file))
		if err != nil {
			t.Fatal(err)
		}
		deps, _ := m["deps"].(map[string]any)
		if _, ok := deps[want]; !ok || len(deps) != 1 {
			t.Errorf("the %s package depends on %v, want %s", arch, deps, want)
		}
	}
	files, err := unixTree(a, "/usr/local", workers["arm"], a.Name, a.GTK.python(systemPython(a.GTK, "freebsd", "armv7")))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.path == "/usr/local/bin/vero-example" && !strings.Contains(string(f.data), "exec python3.11 ") {
			t.Errorf("armv7's launcher is %s", f.data)
		}
	}
	a.GTK.FreeBSD.ByArch = map[string]BSDArchPkg{"arm64": {Python: "python3.11"}}
	if err := a.GTK.checkArches(); err == nil {
		t.Error("by_arch took arm64, which FreeBSD calls aarch64")
	}
}
