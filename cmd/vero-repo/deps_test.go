package main

import (
	"slices"
	"strings"
	"testing"

	tomlpkg "github.com/BurntSushi/toml"
)

// TestResolveDeps checks that [gtk] uses and extra become each system's
// packages, with what a section lists added after them, and that a
// processor whose packages lack something is not made to need it.
func TestResolveDeps(t *testing.T) {
	toml := `name = "example"
id = "dev.vero.example"
[worker]
package = "cmd/worker"
name = "example-worker"
[gtk]
folder = "gtk"
entry = "main.py"
uses = ["adwaita", "webview", "keyring"]
extra = ["ffmpeg"]
[gtk.deb]
depends = "python3-gi, libfoo1"
[gtk.alpine]
[gtk.void]
[gtk.freebsd]
[gtk.netbsd]
[gtk.illumos]
[gtk.openbsd]
`
	var a App
	if _, err := tomlpkg.Decode(toml, &a); err != nil {
		t.Fatal(err)
	}
	if err := a.GTK.resolveDeps(); err != nil {
		t.Fatal(err)
	}
	g := a.GTK
	for _, c := range []struct{ what, got, want string }{
		{"deb depends", g.Deb.Depends, "python3 (>= 3.11), python3-gi, gir1.2-gtk-4.0, ffmpeg, libfoo1"},
		{"deb recommends", g.Deb.Recommends, "gir1.2-adw-1, gir1.2-webkit-6.0, gnome-keyring"},
		{"arch depends", g.Arch.Depends, "python, python-gobject, gtk4, libadwaita, ffmpeg"},
		{"alpine", g.Alpine.Depends, "python3, py3-gobject3, gtk4.0, font-dejavu, libadwaita, ffmpeg"},
		{"void", g.Void.Depends, "python3, python3-gobject, gtk4, dejavu-fonts-ttf, libadwaita, ffmpeg6"},
	} {
		if c.got != c.want {
			t.Errorf("%s: %q, want %q", c.what, c.got, c.want)
		}
	}
	if !strings.Contains(g.RPM.Recommends, "typelib(Adw) = 1") || !strings.Contains(g.RPM.Requires, "ffmpeg-free") {
		t.Errorf("rpm: requires %q, recommends %q", g.RPM.Requires, g.RPM.Recommends)
	}
	if !strings.Contains(g.Arch.OptDepends, "webkitgtk-6.0: ") {
		t.Errorf("arch optdepends %q", g.Arch.OptDepends)
	}
	if g.Chimera != nil {
		t.Error("chimera was packaged without its section")
	}
	// FreeBSD's armv7 packages are on Python 3.11.
	if g.FreeBSD.Deps["libadwaita"] != "x11-toolkits/libadwaita" || g.FreeBSD.Deps["py312-pygobject"] == "" {
		t.Errorf("freebsd deps %v", g.FreeBSD.Deps)
	}
	if arm := g.FreeBSD.ByArch["armv7"]; arm.Python != "python3.11" || arm.Deps["py311-pygobject"] == "" || arm.Deps["libadwaita"] == "" {
		t.Errorf("freebsd armv7 %+v", arm)
	}
	if !slices.Contains(g.NetBSD.Depends, "libadwaita-[0-9]*") || g.NetBSD.Python != "python3.12" {
		t.Errorf("netbsd %+v", g.NetBSD)
	}
	// OpenIndiana's ffmpeg is in hipster-encumbered, which the site's
	// install commands add first.
	if !slices.Contains(g.Illumos.Depends, "library/desktop/libadwaita") || !slices.Contains(g.Illumos.Depends, "video/ffmpeg") ||
		!slices.Equal(g.Illumos.Publishers, []string{encumbered}) {
		t.Errorf("illumos depends %v, publishers %v", g.Illumos.Depends, g.Illumos.Publishers)
	}
	cmds := unixInstructions(page{App: &a, URL: "https://example.com"}, unixSystem{name: "illumos", label: "illumos", format: "ips"}, "").Commands
	if len(cmds) != 3 || !strings.Contains(cmds[0], "pkg set-publisher -g https://pkg.openindiana.org/hipster-encumbered/ hipster-encumbered") ||
		!strings.Contains(cmds[1], "dev.vero.example") {
		t.Errorf("illumos commands %q", cmds)
	}
	has := func(arch, pkg string) bool {
		return slices.ContainsFunc(g.OpenBSD.dependsOn(arch), func(d string) bool { return strings.HasPrefix(d, pkg) })
	}
	if !has("amd64", "x11/libadwaita") || has("arm", "x11/libadwaita") || has("powerpc64", "x11/libadwaita") {
		t.Errorf("openbsd libadwaita: amd64 %v, arm %v, powerpc64 %v", has("amd64", "x11/libadwaita"), has("arm", "x11/libadwaita"), has("powerpc64", "x11/libadwaita"))
	}
	if has("powerpc64", "graphics/ffmpeg") || !has("arm", "graphics/ffmpeg") {
		t.Error("openbsd ffmpeg on the wrong processors")
	}
}

// TestResolveDepsUnknown checks that a use or extra vero does not know is
// refused with what to do instead, and that an app naming neither keeps
// its sections as they are.
func TestResolveDepsUnknown(t *testing.T) {
	g := &GTK{Uses: []string{"adwaita", "maps"}}
	if err := g.resolveDeps(); err == nil || !strings.Contains(err.Error(), `"maps"`) {
		t.Errorf("uses maps: %v", err)
	}
	g = &GTK{Extra: []string{"imagemagick"}}
	if err := g.resolveDeps(); err == nil || !strings.Contains(err.Error(), "own section") {
		t.Errorf("extra imagemagick: %v", err)
	}
	g = &GTK{Uses: []string{}}
	if err := g.resolveDeps(); err != nil || g.Deb.Depends != "python3 (>= 3.11), python3-gi, gir1.2-gtk-4.0" || g.Deb.Recommends != "" {
		t.Errorf("uses = [] did not give the base alone: %+v, %v", g.Deb, err)
	}
	g = &GTK{Deb: Deb{Depends: "python3, gir1.2-gtk-4.0"}}
	if err := g.resolveDeps(); err != nil || g.Deb.Depends != "python3, gir1.2-gtk-4.0" || g.Deb.Recommends != "" {
		t.Errorf("an app without uses changed: %+v, %v", g.Deb, err)
	}
}

// TestEveryLinuxHasAFont: GTK draws no text without a font, and Alpine,
// Void, Chimera and openSUSE bring none with it.
func TestEveryLinuxHasAFont(t *testing.T) {
	for _, sys := range []string{"alpine", "void", "chimera", "rpm"} {
		found := false
		for _, p := range linuxDeps[sys].base {
			if strings.Contains(p, "dejavu") {
				found = true
			}
		}
		if !found {
			t.Errorf("%s's packages have no font: %v", sys, linuxDeps[sys].base)
		}
	}
}
