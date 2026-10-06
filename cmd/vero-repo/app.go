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
	Licence   string `toml:"licence"`
	Icon      string `toml:"icon"`

	Worker Worker `toml:"worker"`
	GTK    *GTK   `toml:"gtk"`
	WPF    *WPF   `toml:"wpf"`
	Needs  Needs  `toml:"needs"`

	// dir is the folder the file is in, which its paths are relative to.
	dir string
}

// Worker is the app's Go worker.
type Worker struct {
	// Package is its main package's folder; Name the program's name,
	// which gains .exe on Windows.
	Package string `toml:"package"`
	Name    string `toml:"name"`
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
}

// BSDPkg is what a FreeBSD or DragonFly package depends on: each package
// by name, with the port it comes from ("x11-toolkits/gtk40"), which pkg
// needs as well.
type BSDPkg struct {
	Deps   map[string]string `toml:"deps"`
	Python string            `toml:"python"`
}

// Pkgsrc is what a pkgsrc package, for NetBSD or illumos, depends on:
// patterns pkg_add matches, such as "gtk4-[0-9]*".
type Pkgsrc struct {
	Depends []string `toml:"depends"`
	Python  string   `toml:"python"`
}

// OpenBSD is what an OpenBSD package depends on: each as pkg_add names
// it, with the port it comes from, as "x11/gtk+4:gtk+4-*".
type OpenBSD struct {
	Depends []string `toml:"depends"`
	Python  string   `toml:"python"`
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
}

// Arch is what an Arch Linux package depends on, comma separated.
type Arch struct {
	Depends string `toml:"depends"`
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
}

// WPF is the front end for Windows.
type WPF struct {
	Folder string `toml:"folder"`
	Exe    string `toml:"exe"`
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
	if extra := md.Undecoded(); len(extra) > 0 {
		return nil, fmt.Errorf("%s: unknown setting %s", path, extra[0])
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
	if _, err := mail.ParseAddress(a.Publisher); err != nil {
		return problem("publisher %q should be \"Name <email>\"", a.Publisher)
	}
	if a.GTK != nil && (a.GTK.Folder == "" || a.GTK.Entry == "") {
		return problem("[gtk] needs folder and entry")
	}
	if a.WPF != nil && (a.WPF.Folder == "" || a.WPF.Exe == "") {
		return problem("[wpf] needs folder and exe")
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
