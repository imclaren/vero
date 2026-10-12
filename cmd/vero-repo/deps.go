package main

import (
	"fmt"
	"slices"
	"strings"
)

// What an app's GTK front end needs from each system, worked out from
// [gtk] uses and extra, so that each app need not know every system's
// names for them:
//
//	[gtk]
//	uses = ["adwaita", "webview", "keyring"]
//	extra = ["ffmpeg"]
//
// Every app gets Python, PyGObject and GTK 4, and a font where the system
// brings none with GTK. uses are what vero's GTK library can use: libadwaita,
// WebKitGTK's GTK 4 web view, and a keyring for kit/keychain. extra are
// programs vero knows that only some apps need. What a system's own section
// lists, such as [gtk.deb] depends, is added after them, for anything vero
// does not know.
//
// Where a package manager can say that something is recommended rather
// than needed (deb, rpm and Arch), the web view and the keyring are
// recommended, since the library works without them. libadwaita is needed
// on each system and processor whose packages have 1.5 or later, which the
// library uses, and left out where they do not, as on OpenBSD's arm and
// powerpc64 and in SmartOS's pkgsrc for illumos; the library uses plain
// GTK 4 there. Debian 12's libadwaita is too old for the library, which
// then uses plain GTK 4, but recommending it does no harm.
//
// The names were checked against each system's package index on 12 October
// 2026.

var knownUses = []string{"adwaita", "webview", "keyring"}

var knownExtra = []string{"ffmpeg"}

// sysDeps is one system's packages for the base and for each use and extra:
// what is needed, and what is only recommended.
type sysDeps struct {
	base      []string
	need      map[string][]string
	recommend map[string][]string
}

var linuxDeps = map[string]sysDeps{
	"deb": {
		base:      []string{"python3 (>= 3.11)", "python3-gi", "gir1.2-gtk-4.0"},
		need:      map[string][]string{"ffmpeg": {"ffmpeg"}},
		recommend: map[string][]string{"adwaita": {"gir1.2-adw-1"}, "webview": {"gir1.2-webkit-6.0"}, "keyring": {"gnome-keyring"}},
	},
	// Fedora's and openSUSE's names differ; rpm's rich dependencies take
	// whichever the system has.
	"rpm": {
		base: []string{"python3 >= 3.11", "python3-gobject", "(typelib(Gtk) = 4.0 if openSUSE-release else (gtk4 and gobject-introspection))"},
		need: map[string][]string{"ffmpeg": {"(ffmpeg or ffmpeg-free)"}},
		recommend: map[string][]string{
			"adwaita": {"(typelib(Adw) = 1 if openSUSE-release else libadwaita)"},
			"webview": {"(typelib(WebKit) = 6.0 if openSUSE-release else webkitgtk6.0)"},
			"keyring": {"gnome-keyring"},
		},
	},
	// Arch's optdepends are "name: what for".
	"arch": {
		base:      []string{"python", "python-gobject", "gtk4"},
		need:      map[string][]string{"adwaita": {"libadwaita"}, "ffmpeg": {"ffmpeg"}},
		recommend: map[string][]string{"webview": {"webkitgtk-6.0: web pages inside the app"}, "keyring": {"gnome-keyring: keeping sign-ins in the keyring"}},
	},
	"alpine": {
		base: []string{"python3", "py3-gobject3", "gtk4.0", "font-dejavu"},
		need: map[string][]string{"adwaita": {"libadwaita"}, "ffmpeg": {"ffmpeg"}},
	},
	"void": {
		base: []string{"python3", "python3-gobject", "gtk4", "dejavu-fonts-ttf"},
		need: map[string][]string{"adwaita": {"libadwaita"}, "ffmpeg": {"ffmpeg6"}},
	},
	"chimera": {
		base: []string{"python", "python-gobject", "gtk4"},
		need: map[string][]string{"adwaita": {"libadwaita"}, "ffmpeg": {"ffmpeg"}},
	},
}

// bsdDeps are FreeBSD's and DragonFly's packages, each with its port, and
// the Python they run on; byArch replaces them on a processor whose
// packages differ, as FreeBSD's armv7 ones do, frozen at Python 3.11.
type bsdDeps struct {
	base   map[string]string
	need   map[string]map[string]string
	python string
	byArch map[string]bsdDeps
}

var freebsdDeps = bsdDeps{
	base: map[string]string{"python3": "lang/python3", "gtk4": "x11-toolkits/gtk40", "py312-pygobject": "devel/py-pygobject@py312", "dejavu": "x11-fonts/dejavu"},
	need: map[string]map[string]string{"adwaita": {"libadwaita": "x11-toolkits/libadwaita"}, "ffmpeg": {"ffmpeg": "multimedia/ffmpeg"}},
	byArch: map[string]bsdDeps{"armv7": {
		base:   map[string]string{"python3": "lang/python3", "gtk4": "x11-toolkits/gtk40", "py311-pygobject": "devel/py-pygobject@py311", "dejavu": "x11-fonts/dejavu"},
		need:   map[string]map[string]string{"adwaita": {"libadwaita": "x11-toolkits/libadwaita"}, "ffmpeg": {"ffmpeg": "multimedia/ffmpeg"}},
		python: "python3.11",
	}},
}

var dragonflyDeps = bsdDeps{
	base:   map[string]string{"gtk4": "x11-toolkits/gtk40", "py311-pygobject": "devel/py-pygobject@py311", "dejavu": "x11-fonts/dejavu"},
	need:   map[string]map[string]string{"adwaita": {"libadwaita": "x11-toolkits/libadwaita"}, "ffmpeg": {"ffmpeg": "multimedia/ffmpeg"}},
	python: "python3.11",
}

// listDeps are pkgsrc's and OpenBSD's, as patterns; only, when set, names
// the processors on which a use's packages exist.
type listDeps struct {
	base   []string
	need   map[string][]string
	only   map[string][]string
	python string
}

var netbsdDeps = listDeps{
	base:   []string{"gtk4-[0-9]*", "py312-gobject3-[0-9]*", "dejavu-ttf-[0-9]*"},
	need:   map[string][]string{"adwaita": {"libadwaita-[0-9]*"}, "ffmpeg": {"ffmpeg8-[0-9]*"}},
	python: "python3.12",
}

// SmartOS's pkgsrc, from which illumos gets GTK, has no libadwaita.
var illumosDeps = listDeps{
	base:   []string{"gtk4-[0-9]*", "py312-gobject3-[0-9]*", "dejavu-ttf-[0-9]*"},
	need:   map[string][]string{"ffmpeg": {"ffmpeg8-[0-9]*"}},
	python: "python3.12",
}

// OpenBSD's are port:pattern:package. Its packages for arm and powerpc64
// have no libadwaita, and those for powerpc64 no ffmpeg.
var openbsdDeps = listDeps{
	base: []string{"x11/gtk+4:gtk+4-*:gtk+4-4.0", "devel/py-gobject3:py3-gobject3-*:py3-gobject3-3.0"},
	need: map[string][]string{"adwaita": {"x11/libadwaita:libadwaita-*:libadwaita-1.9"}, "ffmpeg": {"graphics/ffmpeg:ffmpeg-*:ffmpeg-6.1"}},
	only: map[string][]string{"adwaita": {"amd64", "aarch64", "i386", "riscv64"}, "ffmpeg": {"amd64", "aarch64", "i386", "arm", "riscv64"}},
}

// resolveDeps fills in each system's section from [gtk] uses and extra,
// before what the section lists itself. An app that names neither keeps
// its sections as they are.
func (g *GTK) resolveDeps() error {
	// uses = [] asks for the base alone; no uses and no extra at all keep
	// the sections as they are.
	if g == nil || (g.Uses == nil && g.Extra == nil) {
		return nil
	}
	for _, u := range g.Uses {
		if !slices.Contains(knownUses, u) {
			return fmt.Errorf("[gtk] uses: vero does not know %q; it knows %s", u, strings.Join(knownUses, ", "))
		}
	}
	for _, e := range g.Extra {
		if !slices.Contains(knownExtra, e) {
			return fmt.Errorf("[gtk] extra: vero does not know %q (it knows %s); list it in each system's own section instead, such as depends in [gtk.deb]",
				e, strings.Join(knownExtra, ", "))
		}
	}
	wanted := append(slices.Clone(g.Uses), g.Extra...)
	lists := func(d sysDeps) (need, recommend []string) {
		need = slices.Clone(d.base)
		for _, w := range wanted {
			need = append(need, d.need[w]...)
			recommend = append(recommend, d.recommend[w]...)
		}
		return need, recommend
	}
	join := func(computed []string, own string) string {
		return strings.Join(mergeList(computed, splitList(own)), ", ")
	}
	need, rec := lists(linuxDeps["deb"])
	g.Deb.Depends, g.Deb.Recommends = join(need, g.Deb.Depends), join(rec, g.Deb.Recommends)
	need, rec = lists(linuxDeps["rpm"])
	g.RPM.Requires, g.RPM.Recommends = join(need, g.RPM.Requires), join(rec, g.RPM.Recommends)
	need, rec = lists(linuxDeps["arch"])
	g.Arch.Depends, g.Arch.OptDepends = join(need, g.Arch.Depends), join(rec, g.Arch.OptDepends)
	for name, p := range map[string]*LinuxPkg{"alpine": g.Alpine, "void": g.Void, "chimera": g.Chimera} {
		if p != nil {
			need, _ := lists(linuxDeps[name])
			p.Depends = join(need, p.Depends)
		}
	}
	for _, s := range []struct {
		pkg  *BSDPkg
		deps bsdDeps
	}{{g.FreeBSD, freebsdDeps}, {g.DragonFly, dragonflyDeps}} {
		if s.pkg == nil {
			continue
		}
		s.pkg.Deps = bsdMerge(s.deps, wanted, s.pkg.Deps)
		if s.pkg.Python == "" {
			s.pkg.Python = s.deps.python
		}
		for arch, d := range s.deps.byArch {
			own := s.pkg.ByArch[arch]
			own.Deps = bsdMerge(d, wanted, own.Deps)
			if own.Python == "" {
				own.Python = d.python
			}
			if s.pkg.ByArch == nil {
				s.pkg.ByArch = map[string]BSDArchPkg{}
			}
			s.pkg.ByArch[arch] = own
		}
	}
	for _, s := range []struct {
		pkg  *Pkgsrc
		deps listDeps
	}{{g.NetBSD, netbsdDeps}, {g.Illumos, illumosDeps}} {
		if s.pkg == nil {
			continue
		}
		s.pkg.Depends = mergeList(listMerge(s.deps, wanted), s.pkg.Depends)
		if s.pkg.Python == "" {
			s.pkg.Python = s.deps.python
		}
	}
	if g.OpenBSD != nil {
		g.OpenBSD.Depends = mergeList(listMerge(openbsdDeps, wanted), g.OpenBSD.Depends)
		g.OpenBSD.only = map[string][]string{}
		for _, w := range wanted {
			for _, d := range openbsdDeps.need[w] {
				if arches := openbsdDeps.only[w]; arches != nil {
					g.OpenBSD.only[d] = arches
				}
			}
		}
	}
	return nil
}

func bsdMerge(d bsdDeps, wanted []string, own map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range d.base {
		out[k] = v
	}
	for _, w := range wanted {
		for k, v := range d.need[w] {
			out[k] = v
		}
	}
	for k, v := range own {
		out[k] = v
	}
	return out
}

func listMerge(d listDeps, wanted []string) []string {
	out := slices.Clone(d.base)
	for _, w := range wanted {
		out = append(out, d.need[w]...)
	}
	return out
}

// mergeList is computed, then each of own not already there.
func mergeList(computed, own []string) []string {
	out := slices.Clone(computed)
	for _, o := range own {
		if !slices.Contains(out, o) {
			out = append(out, o)
		}
	}
	return out
}

// dependsOn is what an OpenBSD package for one processor depends on:
// each dependency, less those whose packages that processor lacks.
func (o *OpenBSD) dependsOn(arch string) []string {
	var out []string
	for _, d := range o.Depends {
		if only, ok := o.only[d]; ok && !slices.Contains(only, arch) {
			continue
		}
		out = append(out, d)
	}
	return out
}
