package main

import (
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// App is what vero-app.toml says about an app: everything its installers
// and its site need that is not secret. Paths in it are relative to the
// file's folder.
type App struct {
	// Name is the package and command name: lower case letters, digits
	// and hyphens. DisplayName is what people see; Name when empty.
	Name        string `toml:"name"`
	DisplayName string `toml:"display_name"`
	// ID is the app's reverse-domain name, such as org.example.myapp.
	ID          string `toml:"id"`
	Version     string `toml:"version"`
	Summary     string `toml:"summary"`
	Description string `toml:"description"`
	// Publisher is who publishes it, as "Name <email>".
	Publisher string `toml:"publisher"`
	Homepage  string `toml:"homepage"`
	// Site is where the install site will be, such as
	// https://example.com/myapp: what release builds it for.
	Site    string `toml:"site"`
	Licence string `toml:"licence"`
	Icon    string `toml:"icon"`

	Worker Worker `toml:"worker"`
	GTK    *GTK   `toml:"gtk"`
	WPF    *WPF   `toml:"wpf"`
	MacOS  *MacOS `toml:"macos"`
	// The front ends that aren't installed from a package manager.
	Web     *Web     `toml:"web"`
	Android *Android `toml:"android"`
	IOS     *IOS     `toml:"ios"`
	WASI    *WASI    `toml:"wasi"`
	Plan9   *Plan9   `toml:"plan9"`
	Needs   Needs    `toml:"needs"`
	// Test is what vero's tests do with the app once it's installed and
	// its window is open.
	Test *Test `toml:"test"`

	// dir is the folder the file is in, which its paths are relative to.
	dir string
	// Build is a Mac app's CFBundleVersion, which Sparkle compares: the
	// version unless package is given --build.
	Build string `toml:"-"`
}

// Worker is the app's Go worker.
type Worker struct {
	// Package is its main package's folder; Name the program's name,
	// which gains .exe on Windows.
	Package string `toml:"package"`
	Name    string `toml:"name"`
	// CGO builds the Mac worker with cgo - for the keychain, say - for
	// both architectures, with Xcode's clang. Elsewhere it's built without.
	CGO bool `toml:"cgo"`
}

// GTK is the front end for Linux and the other Unixes.
type GTK struct {
	Folder string `toml:"folder"`
	Entry  string `toml:"entry"`
	// Include are files copied beside the entry: vero.py, for one.
	Include    []string `toml:"include"`
	Categories string   `toml:"categories"`
	// Deb is what the Debian package needs from Debian; RPM what the rpm
	// needs from Fedora or openSUSE; Arch what the AUR package needs from
	// Arch Linux; and Flatpak, the runtime the Flatpak runs on.
	Deb     Deb     `toml:"deb"`
	RPM     RPM     `toml:"rpm"`
	Arch    Arch    `toml:"arch"`
	Flatpak Flatpak `toml:"flatpak"`
	// Python is the command the app is started with: python3 unless it
	// says otherwise, which a system's own section can say for it.
	Python string `toml:"python"`
	// FreeBSD and DragonFly are what the app needs from their packages;
	// NetBSD and Illumos from pkgsrc's; OpenBSD from its own. A system is
	// packaged for when its section is here.
	FreeBSD   *BSDPkg  `toml:"freebsd"`
	DragonFly *BSDPkg  `toml:"dragonfly"`
	NetBSD    *Pkgsrc  `toml:"netbsd"`
	Illumos   *Pkgsrc  `toml:"illumos"`
	OpenBSD   *OpenBSD `toml:"openbsd"`
	// Alpine and Void are what the app needs from Alpine Linux's and Void
	// Linux's packages; each is packaged for when its section is here.
	Alpine *LinuxPkg `toml:"alpine"`
	Void   *LinuxPkg `toml:"void"`
	// Chimera is what the app needs from Chimera Linux's packages, whose
	// apk installs the same kind of package as Alpine's.
	Chimera *LinuxPkg `toml:"chimera"`
	// Data are more files that every Linux, BSD and illumos package and
	// the Flatpak install, such as a GNOME search provider or a D-Bus
	// service.
	Data []DataFile `toml:"data"`
}

// DataFile is one of [gtk]'s extra files: From in the repository, relative
// to vero-app.toml, installed at To, relative to the package's prefix
// (/usr on Linux, /usr/local on FreeBSD, /app in a Flatpak). With Expand,
// {prefix}, {id} and {name} in the file become the package's prefix, the
// app's ID and its name, so that one file can name the app's command on
// every system.
type DataFile struct {
	From   string `toml:"from"`
	To     string `toml:"to"`
	Expand bool   `toml:"expand"`
}

// BSDPkg is what a FreeBSD or DragonFly package depends on: each package
// by name, with the port it comes from ("x11-toolkits/gtk40"), which pkg
// needs as well.
type BSDPkg struct {
	Deps   map[string]string `toml:"deps"`
	Python string            `toml:"python"`
	// ByArch replaces deps, python or both on an architecture, by the
	// system's name for it, where its packages differ there, as in
	// [gtk.freebsd.by_arch.armv7].
	ByArch map[string]BSDArchPkg `toml:"by_arch"`
	// Arches are the architectures packaged: x86-64 and ARM64, where the
	// system has them, when it's empty. Naming some narrows them, such as
	// ["x86_64"] where the system's own packages lack something on ARM, or
	// adds others, such as 32-bit Intel, which are packaged only when named.
	Arches []string `toml:"arches"`
}

// BSDArchPkg is what a FreeBSD or DragonFly package needs on one
// architecture, where it isn't what its section says for the others.
type BSDArchPkg struct {
	Deps   map[string]string `toml:"deps"`
	Python string            `toml:"python"`
}

// depsFor are the packages a FreeBSD or DragonFly package depends on, on
// arch, by the system's name for it.
func (b *BSDPkg) depsFor(arch string) map[string]string {
	if o, ok := b.ByArch[arch]; ok && len(o.Deps) > 0 {
		return o.Deps
	}
	return b.Deps
}

// pythonFor is the python a FreeBSD or DragonFly package starts the app
// with on arch, or "" for the usual.
func (b *BSDPkg) pythonFor(arch string) string {
	if o, ok := b.ByArch[arch]; ok && o.Python != "" {
		return o.Python
	}
	return b.Python
}

// Pkgsrc is what a pkgsrc package, for NetBSD or illumos, depends on:
// patterns pkg_add matches, such as "gtk4-[0-9]*".
type Pkgsrc struct {
	Depends []string `toml:"depends"`
	Python  string   `toml:"python"`
	// Arches are the architectures packaged: x86-64 and ARM64, where the
	// system has them, when it's empty. Naming some narrows them, such as
	// ["x86_64"] where the system's own packages lack something on ARM, or
	// adds others, such as 32-bit Intel, which are packaged only when named.
	Arches []string `toml:"arches"`
}

// OpenBSD is what an OpenBSD package depends on: each as pkg_add names
// it, with the port it comes from, as "x11/gtk+4:gtk+4-*".
type OpenBSD struct {
	Depends []string `toml:"depends"`
	Python  string   `toml:"python"`
	// Arches are the architectures packaged: x86-64 and ARM64, where the
	// system has them, when it's empty. Naming some narrows them, such as
	// ["x86_64"] where the system's own packages lack something on ARM, or
	// adds others, such as 32-bit Intel, which are packaged only when named.
	Arches []string `toml:"arches"`
}

// python is the command the app is started with on a system whose own
// section says system, or "".
func (g *GTK) python(system string) string {
	if system != "" {
		return system
	}
	if g.Python != "" {
		return g.Python
	}
	return "python3"
}

// RPM is what an rpm depends on: packages, comma separated, each with a
// version if it needs one ("python3 >= 3.10").
type RPM struct {
	Requires   string `toml:"requires"`
	Recommends string `toml:"recommends"`
	// Arches are the architectures packaged: x86-64 and ARM64, where the
	// system has them, when it's empty. Naming some narrows them, such as
	// ["x86_64"] where the system's own packages lack something on ARM, or
	// adds others, such as 32-bit Intel, which are packaged only when named.
	Arches []string `toml:"arches"`
}

// Arch is what an Arch Linux package depends on, comma separated, and
// what it can use if it's there, each as pacman's optdepends has it,
// "NAME: what for", with no comma in what for.
type Arch struct {
	Depends    string `toml:"depends"`
	OptDepends string `toml:"optdepends"`
	// Arches are the architectures packaged: x86-64 and ARM64, where the
	// system has them, when it's empty. Naming some narrows them, such as
	// ["x86_64"] where the system's own packages lack something on ARM, or
	// adds others, such as 32-bit Intel, which are packaged only when named.
	Arches []string `toml:"arches"`
}

// Flatpak is the runtime a Flatpak runs on, from Flathub: GNOME's, which
// has GTK 4, WebKitGTK, Python and PyGObject, unless it says otherwise.
type Flatpak struct {
	// Build makes a Flatpak whenever the Linux packages are made. It's
	// off unless asked for, since Flatpak needs Docker; --targets flatpak
	// makes one either way.
	Build          bool   `toml:"build"`
	Runtime        string `toml:"runtime"`
	RuntimeVersion string `toml:"runtime_version"`
	// Resources are more files for the Flatpak, put beside the worker in
	// /app/lib/NAME: a helper program the runtime lacks, such as the
	// app's own ffmpeg, and its licence, say. {arch} in a path is the
	// architecture, as Flatpak names it: x86_64 or aarch64. Prepare is a
	// command run in vero-app.toml's folder before each architecture's
	// Flatpak is made, with ARCH and GOARCH set: what downloads or builds
	// those files, say.
	Resources []string `toml:"resources"`
	Prepare   string   `toml:"prepare"`
	// Arches are the architectures packaged: x86-64 and ARM64, where the
	// system has them, when it's empty. Naming some narrows them, such as
	// ["x86_64"] where the system's own packages lack something on ARM, or
	// adds others, such as 32-bit Intel, which are packaged only when named.
	Arches []string `toml:"arches"`
}

// The runtime a Flatpak runs on when vero-app.toml doesn't say.
const (
	defaultRuntime        = "org.gnome.Platform"
	defaultRuntimeVersion = "51"
)

// runtime is the Flatpak's runtime and its version.
func (g *GTK) runtime() (string, string) {
	r, v := g.Flatpak.Runtime, g.Flatpak.RuntimeVersion
	if r == "" {
		r = defaultRuntime
	}
	if v == "" {
		v = defaultRuntimeVersion
	}
	return r, v
}

// Deb is what a Debian package depends on, and where it is filed.
type Deb struct {
	Depends    string `toml:"depends"`
	Recommends string `toml:"recommends"`
	Section    string `toml:"section"`
	// Arches are the architectures packaged: x86-64 and ARM64, where the
	// system has them, when it's empty. Naming some narrows them, such as
	// ["x86_64"] where the system's own packages lack something on ARM, or
	// adds others, such as 32-bit Intel, which are packaged only when named.
	Arches []string `toml:"arches"`
}

// WPF is the front end for Windows.
type WPF struct {
	Folder string `toml:"folder"`
	Exe    string `toml:"exe"`
	// Arches are the installers made: x64 and arm64 unless it says, and
	// x86, for 32-bit Windows, only when it's named.
	Arches []string `toml:"arches"`
	// WingetID is the app's identifier in winget, as Publisher.App; made
	// from the publisher's and the app's names unless it says.
	WingetID string `toml:"winget_id"`
	// MSIX is the package for the Microsoft Store, made only when asked.
	MSIX MSIXConfig `toml:"msix"`
}

// Needs are what the app asks of the system it runs on.
type Needs struct {
	Network       bool `toml:"network"`
	Files         bool `toml:"files"`
	Keyring       bool `toml:"keyring"`
	Notifications bool `toml:"notifications"`
	Autostart     bool `toml:"autostart"`
	Webview       bool `toml:"webview"`
}

var (
	validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	validID   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*(\.[A-Za-z][A-Za-z0-9_-]*)+$`)
)

// LoadApp reads and checks a vero-app.toml.
func LoadApp(path string) (*App, error) {
	var a App
	md, err := toml.DecodeFile(path, &a)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, key := range md.Undecoded() {
		// A step's own tables, such as wait's, are the step's to check.
		if len(key) > 3 && key[0] == "test" && key[1] == "step" {
			continue
		}
		return nil, fmt.Errorf("%s: unknown setting %s", path, key)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	a.dir = filepath.Dir(abs)
	if a.DisplayName == "" {
		a.DisplayName = a.Name
	}
	return &a, a.check(path)
}

func (a *App) check(path string) error {
	problem := func(f string, args ...any) error { return fmt.Errorf("%s: "+f, append([]any{path}, args...)...) }
	switch {
	case !validName.MatchString(a.Name):
		return problem("name %q should be lower case letters, digits and hyphens", a.Name)
	case !validID.MatchString(a.ID):
		return problem("id %q should be a reverse-domain name, such as org.example.myapp", a.ID)
	case a.Summary == "":
		return problem("summary is needed")
	case a.Publisher == "":
		return problem("publisher is needed, as \"Name <email>\"")
	case a.Worker.Package == "" || a.Worker.Name == "":
		return problem("[worker] needs package and name")
	}
	if a.Icon != "" {
		if _, err := os.Stat(a.Path(a.Icon)); err != nil {
			return problem("icon %q is not there: a square PNG, 512 or 1024 pixels", a.Icon)
		}
	}
	if _, err := mail.ParseAddress(a.Publisher); err != nil {
		return problem("publisher %q should be \"Name <email>\"", a.Publisher)
	}
	if a.GTK != nil && (a.GTK.Folder == "" || a.GTK.Entry == "") {
		return problem("[gtk] needs folder and entry")
	}
	if a.GTK != nil {
		if err := a.GTK.checkArches(); err != nil {
			return problem("%v", err)
		}
		for _, d := range a.GTK.Data {
			to := filepath.ToSlash(filepath.Clean(d.To))
			if d.From == "" || d.To == "" || strings.HasPrefix(d.To, "/") || to == "." || strings.HasPrefix(to, "../") || to == ".." {
				return problem("each [[gtk.data]] needs from, a file in the repository, and to, a path under the package's prefix such as share/gnome-shell/search-providers/%s.search-provider.ini", a.ID)
			}
			if fi, err := os.Stat(a.Path(d.From)); err != nil || fi.IsDir() {
				return problem("[[gtk.data]] from %q is not a file", d.From)
			}
		}
	}
	if a.WPF != nil {
		if _, err := a.WPF.arches(); err != nil {
			return problem("%v", err)
		}
	}
	if m := a.MacOS; m != nil && (m.Folder == "" || (m.Product == "") == (m.Project == "" || m.Scheme == "")) {
		return problem("[macos] needs folder, and either product (SwiftPM) or project and scheme (Xcode)")
	}
	switch {
	case a.Web != nil && (a.Web.Folder == "" || len(a.Web.Files) == 0):
		return problem("[web] needs folder and files")
	case a.Android != nil && (a.Android.Folder == "" || a.Android.APK == ""):
		return problem("[android] needs folder and apk")
	case a.IOS != nil && a.IOS.Folder == "":
		return problem("[ios] needs folder")
	case a.WASI != nil && (a.WASI.Frontend == "" || a.WASI.Worker == ""):
		return problem("[wasi] needs frontend and worker")
	case a.Plan9 != nil && a.Plan9.Frontend == "":
		return problem("[plan9] needs frontend")
	}
	if a.WPF != nil && (a.WPF.Folder == "" || a.WPF.Exe == "") {
		return problem("[wpf] needs folder and exe")
	}
	if a.Test != nil {
		if err := a.Test.check(a); err != nil {
			return problem("%v", err)
		}
	}
	return nil
}

// Path is one of the file's paths, made absolute.
func (a *App) Path(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(a.dir, p)
}

// PublisherName is the publisher without the email address.
func (a *App) PublisherName() string {
	if addr, err := mail.ParseAddress(a.Publisher); err == nil && addr.Name != "" {
		return addr.Name
	}
	return strings.TrimSpace(a.Publisher)
}

// ModuleRoot is the folder with the go.mod the worker is built in: the
// nearest one above the worker's package.
func (a *App) ModuleRoot() (string, error) {
	dir := a.Path(a.Worker.Package)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		up := filepath.Dir(dir)
		if up == dir {
			return "", fmt.Errorf("no go.mod above %s", a.Path(a.Worker.Package))
		}
		dir = up
	}
}

// buildNumber is the Mac app's CFBundleVersion.
func (a *App) buildNumber() string {
	if a.Build != "" {
		return a.Build
	}
	return a.Version
}

// MSIXConfig is the MSIX package, for the Microsoft Store.
type MSIXConfig struct {
	// Build makes one whenever the Windows installers are made; --targets
	// msix makes one either way.
	Build bool `toml:"build"`
	// The identity Partner Center gives the app when you reserve its name:
	// Package/Identity/Name, and Publisher, as "CN=...", and the publisher
	// name it shows.
	IdentityName  string `toml:"identity_name"`
	Publisher     string `toml:"publisher"`
	PublisherName string `toml:"publisher_name"`
}
