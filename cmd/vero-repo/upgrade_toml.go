package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
)

// upgradeTomlCommand rewrites an app's [gtk] in the form that works out
// each system's packages, for vero upgrade: it adds uses and extra, from
// what the sections already list, and takes out of each section what vero
// now works out, leaving the rest and every comment. It prints a line for
// each change.
func upgradeTomlCommand(args []string) error {
	fs := flag.NewFlagSet("upgrade-toml", flag.ExitOnError)
	app := fs.String("app", "vero-app.toml", "the app's vero-app.toml")
	fs.Parse(args)
	data, err := os.ReadFile(*app)
	if err != nil {
		return err
	}
	text, changes := upgradeToml(string(data))
	for _, c := range changes {
		fmt.Println(c)
	}
	if text == string(data) {
		return nil
	}
	return os.WriteFile(*app, []byte(text), 0o644)
}

// upgradeToml is the work of upgradeTomlCommand, on the file's text.
func upgradeToml(text string) (string, []string) {
	lines := strings.Split(text, "\n")
	section := func(i int) string {
		for ; i >= 0; i-- {
			if m := regexp.MustCompile(`^\s*\[([^\]]+)\]`).FindStringSubmatch(lines[i]); m != nil {
				return m[1]
			}
		}
		return ""
	}
	gtkAt := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "[gtk]" {
			gtkAt = i
		}
	}
	if gtkAt < 0 {
		return text, nil
	}
	for i := gtkAt + 1; i < len(lines) && section(i) == "gtk"; i++ {
		if regexp.MustCompile(`^\s*(uses|extra)\s*=`).MatchString(lines[i]) {
			return text, nil // already in the new form
		}
	}
	// What the sections ask for that vero knows: the web view, a keyring
	// and ffmpeg. libadwaita, which vero's GTK library uses where it is
	// installed, is added for every app.
	all := strings.ToLower(text[strings.Index(text, "[gtk]"):])
	uses := []string{"adwaita"}
	if strings.Contains(all, "webkit") {
		uses = append(uses, "webview")
	}
	if strings.Contains(all, "keyring") || strings.Contains(all, "libsecret") {
		uses = append(uses, "keyring")
	}
	var extra []string
	if strings.Contains(all, "ffmpeg") {
		extra = append(extra, "ffmpeg")
	}
	// What vero now works out, for each system.
	g := &GTK{Uses: uses, Extra: extra, Alpine: &LinuxPkg{}, Void: &LinuxPkg{}, Chimera: &LinuxPkg{},
		FreeBSD: &BSDPkg{}, DragonFly: &BSDPkg{}, NetBSD: &Pkgsrc{}, Illumos: &Pkgsrc{}, OpenBSD: &OpenBSD{}}
	g.resolveDeps()
	strs := map[string][]string{
		"gtk.deb/depends": splitList(g.Deb.Depends), "gtk.deb/recommends": splitList(g.Deb.Recommends),
		"gtk.rpm/requires": splitList(g.RPM.Requires), "gtk.rpm/recommends": splitList(g.RPM.Recommends),
		"gtk.arch/depends": splitList(g.Arch.Depends), "gtk.arch/optdepends": splitList(g.Arch.OptDepends),
		"gtk.alpine/depends": splitList(g.Alpine.Depends), "gtk.void/depends": splitList(g.Void.Depends),
		"gtk.chimera/depends": splitList(g.Chimera.Depends),
	}
	arrays := map[string][]string{
		"gtk.netbsd/depends": g.NetBSD.Depends, "gtk.illumos/depends": g.Illumos.Depends, "gtk.openbsd/depends": g.OpenBSD.Depends,
	}
	tables := map[string]map[string]string{
		"gtk.freebsd/deps": g.FreeBSD.Deps, "gtk.dragonfly/deps": g.DragonFly.Deps,
		"gtk.freebsd.by_arch.armv7/deps": g.FreeBSD.ByArch["armv7"].Deps,
	}
	pythons := map[string]string{
		"gtk.freebsd/python": g.FreeBSD.Python, "gtk.dragonfly/python": g.DragonFly.Python, "gtk.netbsd/python": g.NetBSD.Python,
		"gtk.illumos/python": g.Illumos.Python, "gtk.freebsd.by_arch.armv7/python": g.FreeBSD.ByArch["armv7"].Python,
	}
	var changes []string
	keyRe := regexp.MustCompile(`^(\s*)([a-z_]+)(\s*=\s*)(.*)$`)
	var out []string
	for i, l := range lines {
		m := keyRe.FindStringSubmatch(l)
		if m == nil || i < gtkAt {
			out = append(out, l)
			continue
		}
		key := section(i) + "/" + m[2]
		value := strings.TrimSpace(m[4])
		var kept string
		var removed []string
		switch {
		case strs[key] != nil && strings.HasPrefix(value, `"`):
			own := splitList(strings.Trim(value, `"`))
			var keep []string
			for _, o := range own {
				if covered(o, strs[key]) {
					removed = append(removed, o)
				} else {
					keep = append(keep, o)
				}
			}
			kept = `"` + strings.Join(keep, ", ") + `"`
			if len(keep) == 0 {
				kept = ""
			}
		case arrays[key] != nil && strings.HasPrefix(value, "["):
			var keep []string
			for _, o := range regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(value, -1) {
				// illumos now installs from OpenIndiana's own packages, so
				// pkgsrc's patterns there go.
				if covered(o[1], arrays[key]) || (key == "gtk.illumos/depends" && strings.Contains(o[1], "-[0-9]*")) {
					removed = append(removed, o[1])
				} else {
					keep = append(keep, `"`+o[1]+`"`)
				}
			}
			kept = "[" + strings.Join(keep, ", ") + "]"
			if len(keep) == 0 {
				kept = ""
			}
		case tables[key] != nil && strings.HasPrefix(value, "{"):
			var keep []string
			for _, o := range regexp.MustCompile(`([\w.+-]+)\s*=\s*"([^"]*)"`).FindAllStringSubmatch(value, -1) {
				if tables[key][o[1]] == o[2] {
					removed = append(removed, o[1])
				} else {
					keep = append(keep, o[1]+` = "`+o[2]+`"`)
				}
			}
			kept = "{ " + strings.Join(keep, ", ") + " }"
			if len(keep) == 0 {
				kept = ""
			}
		case pythons[key] != "" && value == `"`+pythons[key]+`"`:
			removed, kept = []string{pythons[key]}, ""
		case key == "gtk.illumos/python" && !strings.HasPrefix(strings.Trim(value, `"`), "/"):
			// pkgsrc's Python, by its name in /opt/local/bin.
			removed, kept = []string{strings.Trim(value, `"`)}, ""
		default:
			out = append(out, l)
			continue
		}
		if len(removed) == 0 {
			out = append(out, l)
			continue
		}
		changes = append(changes, fmt.Sprintf("[%s] %s: %s, which vero now works out", section(i), m[2], strings.Join(removed, ", ")))
		if kept != "" {
			out = append(out, m[1]+m[2]+m[3]+kept)
		}
	}
	// uses and extra, at the end of [gtk]'s own settings.
	at := gtkAt + 1
	for at < len(out) && !regexp.MustCompile(`^\s*\[`).MatchString(out[at]) {
		at++
	}
	for at > gtkAt+1 && strings.TrimSpace(out[at-1]) == "" {
		at--
	}
	quote := func(list []string) string {
		q := make([]string, len(list))
		for i, s := range list {
			q[i] = `"` + s + `"`
		}
		return "[" + strings.Join(q, ", ") + "]"
	}
	add := []string{"# What the app uses and needs, from which vero works out each system's", "# packages, as vero's example/vero-app.toml explains.", "uses = " + quote(uses)}
	if len(extra) > 0 {
		add = append(add, "extra = "+quote(extra))
	}
	out = slices.Insert(out, at, add...)
	changes = append([]string{"[gtk] uses " + strings.Join(uses, ", ") + func() string {
		if len(extra) == 0 {
			return ""
		}
		return ", and extra " + strings.Join(extra, ", ")
	}()}, changes...)
	return strings.Join(out, "\n"), changes
}

// covered says whether vero's own list has the package that own names,
// perhaps with another version or reason: "python3 (>= 3.10)" is covered
// by "python3 (>= 3.11)", and "webkitgtk-6.0: signing in" by vero's
// "webkitgtk-6.0: web pages inside the app".
func covered(own string, computed []string) bool {
	name := func(s string) string {
		s = strings.TrimSpace(s)
		if i := strings.IndexAny(s, " :("); i > 0 && !strings.HasPrefix(s, "(") {
			s = s[:i]
		}
		return s
	}
	for _, c := range computed {
		if own == c || name(own) == name(c) {
			return true
		}
		// OpenBSD's port:pattern:package, by its port.
		if strings.Count(own, ":") == 2 && strings.Count(c, ":") == 2 && strings.SplitN(own, ":", 2)[0] == strings.SplitN(c, ":", 2)[0] {
			return true
		}
	}
	return false
}
