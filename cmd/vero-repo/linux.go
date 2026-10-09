package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/image/draw"
)

// linuxArches are the architectures the Linux packages may be built for:
// Go's name, and each kind of package's own, by its section of
// vero-app.toml; a kind without one has no packages for it. The extra ones
// are packaged only where a section's arches names them.
var linuxArches = []struct {
	goarch string
	names  map[string]string
	extra  bool
}{
	{"amd64", map[string]string{"deb": "amd64", "rpm": "x86_64", "arch": "x86_64", "flatpak": "x86_64", "alpine": "x86_64", "void": "x86_64"}, false},
	{"arm64", map[string]string{"deb": "arm64", "rpm": "aarch64", "arch": "aarch64", "flatpak": "aarch64", "alpine": "aarch64", "void": "aarch64"}, false},
	{"riscv64", map[string]string{"deb": "riscv64", "rpm": "riscv64", "arch": "riscv64", "alpine": "riscv64"}, true},
	{"ppc64le", map[string]string{"deb": "ppc64el", "rpm": "ppc64le", "alpine": "ppc64le"}, true},
	// 32-bit ARM as Raspberry Pi OS and its like have it: ARMv7, with
	// hardware floating point.
	{"arm", map[string]string{"deb": "armhf", "arch": "armv7h", "alpine": "armv7", "void": "armv7l"}, true},
	// 32-bit Intel and AMD, from the Pentium 4 on, since Go's code needs
	// SSE2: i686 to rpm, pacman and xbps, which have it as the oldest they
	// run on.
	{"386", map[string]string{"deb": "i386", "rpm": "i686", "arch": "i686", "alpine": "x86", "void": "i686"}, true},
	{"loong64", map[string]string{"deb": "loong64", "arch": "loong64", "alpine": "loongarch64"}, true},
	// POWER, big-endian, as Debian's ports have it.
	{"ppc64", map[string]string{"deb": "ppc64"}, true},
	{"s390x", map[string]string{"deb": "s390x", "rpm": "s390x", "alpine": "s390x"}, true},
	{"mips64le", map[string]string{"deb": "mips64el"}, true},
	{"mipsle", map[string]string{"deb": "mipsel"}, true},
}

// linuxArchNames are every name a kind of Linux package has for an
// architecture, as a regular expression's alternatives.
func linuxArchNames(section string) string {
	var names []string
	for _, a := range linuxArches {
		if n := a.names[section]; n != "" {
			names = append(names, regexp.QuoteMeta(n))
		}
	}
	return strings.Join(names, "|")
}

// linuxArch is an architecture a kind of Linux package is made for: Go's
// name, the kind's own, and Debian's.
type linuxArch struct{ goarch, name, deb string }

// treeFile is one file of an installed app: its path once installed,
// what is in it, and its mode.
type treeFile struct {
	path string
	data []byte
	mode fs.FileMode
}

// buildWorker cross-compiles the app's worker for goos on goarch, as
// build-all.sh does, with main.version set, into dir.
func buildWorker(a *App, root, worker, goos, goarch, ldflags, dir string) (string, error) {
	out := filepath.Join(dir, a.Worker.Name+"-"+goos+"-"+goarch)
	flags := strings.TrimSpace("-s -w -X main.version=" + a.Version + " " + ldflags)
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", flags, "-o", out, worker)
	cmd.Dir, cmd.Stdout, cmd.Stderr = root, os.Stdout, os.Stderr
	cmd.Env = append(append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0"), goarchEnv(goarch)...)
	if goos == "darwin" && a.Worker.CGO {
		arch := map[string]string{"amd64": "x86_64"}[goarch]
		if arch == "" {
			arch = goarch
		}
		minimum := "13.0"
		if a.MacOS != nil {
			minimum = macMinimum(a.MacOS)
		}
		cmd.Env = append(cmd.Env, "CGO_ENABLED=1", "CC=clang -arch "+arch, "MACOSX_DEPLOYMENT_TARGET="+minimum)
	}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("building the worker for %s/%s: %w", goos, goarch, err)
	}
	return out, nil
}

// goarchEnv is what Go is told, besides GOARCH, to build for goarch as
// the systems packaged for have it: ARMv7 with hardware floating point,
// and the Pentium 4's SSE2.
func goarchEnv(goarch string) []string {
	switch goarch {
	case "arm":
		return []string{"GOARM=7"}
	case "386":
		return []string{"GO386=sse2"}
	}
	return nil
}

// linuxTree is the app as a Linux package installs it under prefix (/usr,
// or /app in a Flatpak): its folder in lib with the worker beside it, the
// command in bin, the menu entry, the icon at the sizes a desktop uses,
// and its AppStream metadata. exec is the menu entry's command.
func linuxTree(a *App, prefix, workerPath, exec string) ([]treeFile, error) {
	return unixTree(a, prefix, workerPath, exec, a.GTK.python(""))
}

// unixTree is linuxTree on any Unix: the app under prefix, started with
// python.
func unixTree(a *App, prefix, workerPath, exec, python string) ([]treeFile, error) {
	lib := path.Join(prefix, "lib", a.Name)
	var files []treeFile
	err := appFiles(a, func(rel string, data []byte, mode fs.FileMode) {
		if rel == a.GTK.Entry {
			mode = 0o755
		}
		files = append(files, treeFile{path.Join(lib, filepath.ToSlash(rel)), data, mode})
	})
	if err != nil {
		return nil, err
	}
	worker, err := os.ReadFile(workerPath)
	if err != nil {
		return nil, err
	}
	launcher := fmt.Sprintf("#!/bin/sh\nexec %s %s/%s \"$@\"\n", python, lib, a.GTK.Entry)
	files = append(files,
		treeFile{path.Join(lib, a.Worker.Name), worker, 0o755},
		treeFile{path.Join(prefix, "bin", a.Name), []byte(launcher), 0o755},
		treeFile{path.Join(prefix, "share", "applications", a.ID+".desktop"), desktopEntry(a, exec), 0o644},
		treeFile{path.Join(prefix, "share", "metainfo", a.ID+".metainfo.xml"), appStream(a), 0o644},
	)
	icons, err := iconSizes(a.Path(a.Icon), 64, 128, 256, 512)
	if err != nil {
		return nil, err
	}
	for size, data := range icons {
		files = append(files, treeFile{path.Join(prefix, "share", "icons", "hicolor", fmt.Sprintf("%dx%d", size, size), "apps", a.ID+".png"), data, 0o644})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, nil
}

// appFiles calls add with each of the front end's files, relative to its
// folder: its own, and those it includes, but not what a run left behind
// (a worker, Python's caches).
func appFiles(a *App, add func(rel string, data []byte, mode fs.FileMode)) error {
	src := a.Path(a.GTK.Folder)
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == a.Worker.Name || strings.HasSuffix(d.Name(), ".pyc") {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		add(rel, data, mode)
		return nil
	})
	if err != nil {
		return err
	}
	for _, inc := range a.GTK.Include {
		data, err := os.ReadFile(a.Path(inc))
		if err != nil {
			return err
		}
		add(filepath.Base(inc), data, 0o644)
	}
	return nil
}

// desktopEntry is the app's menu entry, which runs exec.
func desktopEntry(a *App, exec string) []byte {
	categories := a.GTK.Categories
	if categories == "" {
		categories = "Utility;"
	}
	return []byte(fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Comment=%s
Exec=%s
Icon=%s
Categories=%s
StartupNotify=true
`, a.DisplayName, a.Summary, exec, a.ID, categories))
}

// iconSizes is the icon as a PNG at each size.
func iconSizes(file string, sizes ...int) (map[int][]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	out := map[int][]byte{}
	for _, size := range sizes {
		dst := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		var b bytes.Buffer
		if err := png.Encode(&b, dst); err != nil {
			return nil, err
		}
		out[size] = b.Bytes()
	}
	return out, nil
}

// goarchOf is an architecture as Go names it, from any name vero-app.toml
// may give it: Go's own, or a system's.
func goarchOf(name string) string {
	switch strings.ToLower(name) {
	case "amd64", "x86_64", "x86-64", "x64", "x86:64":
		return "amd64"
	case "arm64", "aarch64":
		return "arm64"
	case "riscv64":
		return "riscv64"
	case "ppc64le", "ppc64el", "powerpc64le":
		return "ppc64le"
	case "arm", "armhf", "armv7", "armv7h", "armv7l", "armv7hl", "earmv7hf":
		return "arm"
	case "386", "i386", "i486", "i586", "i686", "x86", "pentium4":
		return "386"
	case "loong64", "loongarch64":
		return "loong64"
	case "ppc64", "powerpc64":
		return "ppc64"
	case "s390x":
		return "s390x"
	case "mips64le", "mips64el":
		return "mips64le"
	case "mipsle", "mipsel":
		return "mipsle"
	}
	return ""
}

// allows says whether arches, from a section of vero-app.toml, takes in
// goarch: all of them, when it's empty.
func allows(arches []string, goarch string) bool {
	if len(arches) == 0 {
		return true
	}
	for _, a := range arches {
		if goarchOf(a) == goarch {
			return true
		}
	}
	return false
}

// linuxArchesOf are the architectures section's packages are made for,
// given the arches it names: amd64 and arm64 when it names none, and only
// those it names when it does.
func linuxArchesOf(section string, arches []string) []linuxArch {
	var out []linuxArch
	for _, a := range linuxArches {
		name := a.names[section]
		if name == "" || (len(arches) == 0 && a.extra) || (len(arches) > 0 && !allows(arches, a.goarch)) {
			continue
		}
		out = append(out, linuxArch{a.goarch, name, a.names["deb"]})
	}
	return out
}

// linuxArches are the architectures the app's Linux packages of a kind -
// deb, rpm, arch, flatpak, alpine or void - are made for.
func (a *App) linuxArches(section string) []linuxArch {
	if a.GTK == nil {
		return linuxArchesOf(section, nil)
	}
	return linuxArchesOf(section, a.GTK.sectionArches()[section])
}

// linuxGoarches are the architectures, as Go names them, of every kind of
// Linux package in sections.
func (a *App) linuxGoarches(sections ...string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range sections {
		for _, arch := range a.linuxArches(s) {
			if !seen[arch.goarch] {
				seen[arch.goarch] = true
				out = append(out, arch.goarch)
			}
		}
	}
	return out
}

// sectionArches are the arches each of the GTK front end's sections names,
// by section.
func (g *GTK) sectionArches() map[string][]string {
	m := map[string][]string{"deb": g.Deb.Arches, "rpm": g.RPM.Arches, "arch": g.Arch.Arches, "flatpak": g.Flatpak.Arches}
	if g.FreeBSD != nil {
		m["freebsd"] = g.FreeBSD.Arches
	}
	if g.DragonFly != nil {
		m["dragonfly"] = g.DragonFly.Arches
	}
	if g.NetBSD != nil {
		m["netbsd"] = g.NetBSD.Arches
	}
	if g.Illumos != nil {
		m["illumos"] = g.Illumos.Arches
	}
	if g.OpenBSD != nil {
		m["openbsd"] = g.OpenBSD.Arches
	}
	if g.Alpine != nil {
		m["alpine"] = g.Alpine.Arches
	}
	if g.Void != nil {
		m["void"] = g.Void.Arches
	}
	return m
}

// checkArches says whether every section's arches are ones its system has.
func (g *GTK) checkArches() error {
	for section, arches := range g.sectionArches() {
		has := map[string]bool{}
		for _, a := range linuxArches {
			if a.names[section] != "" {
				has[a.goarch] = true
			}
		}
		if sys := system(section); sys != nil {
			has = map[string]bool{}
			for _, a := range sys.arches {
				has[a.goarch] = true
			}
		}
		for _, a := range arches {
			if g := goarchOf(a); g == "" || !has[g] {
				return fmt.Errorf("[gtk.%s] arches: %s isn't one vero packages for there", section, a)
			}
		}
	}
	return nil
}

// archesFor are the architectures the app is packaged for on sys: the
// system's, less any its [gtk.NAME] section leaves out, and less its extra
// ones unless the section names them. Where sys's
// repositories go still follows sys.arches, so that narrowing them doesn't
// move a repository.
func (a *App) archesFor(sys *unixSystem) []unixArch {
	var arches []string
	if a.GTK != nil {
		arches = a.GTK.sectionArches()[sys.name]
	}
	var out []unixArch
	for _, arch := range sys.arches {
		if (len(arches) == 0 && !arch.extra) || (len(arches) > 0 && allows(arches, arch.goarch)) {
			out = append(out, arch)
		}
	}
	return out
}
