package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeAUR writes site/aur: a PKGBUILD and its .SRCINFO for the Arch User
// Repository, which install the app from its newest .deb in the site's
// apt repository. Publishing it is a step for the publisher, with their
// own AUR account: the README says how. debs are the newest package of
// each Debian architecture.
func writeAUR(site, url string, a *App, debs map[string]*debFile) error {
	some := false
	for _, arch := range a.linuxArches("arch") {
		some = some || debs[arch.deb] != nil
	}
	if !some {
		return nil
	}
	url = strings.TrimRight(url, "/")
	type source struct{ arch, url, sum string }
	var sources []source
	version := ""
	for _, arch := range a.linuxArches("arch") {
		d := debs[arch.deb]
		if d == nil {
			continue
		}
		data, err := os.ReadFile(d.path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(site, d.path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		sources = append(sources, source{arch.name, url + "/" + filepath.ToSlash(rel), hex.EncodeToString(sum[:])})
		version = d.control.Get("Version")
	}
	pkgver := pacmanVersion(version)
	pkgname := a.Name + "-bin"
	var arches, depends []string
	for _, s := range sources {
		arches = append(arches, "'"+s.arch+"'")
	}
	var deps, optdeps, optdepends []string
	if a.GTK != nil {
		deps = splitList(a.GTK.Arch.Depends)
		optdeps = splitList(a.GTK.Arch.OptDepends)
	}
	for _, d := range deps {
		depends = append(depends, shellQuote(d))
	}
	for _, d := range optdeps {
		optdepends = append(optdepends, shellQuote(d))
	}
	licence := a.Licence
	if licence == "" {
		licence = "custom"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Maintainer: %s\n", a.Publisher)
	fmt.Fprintf(&b, "# Made by vero-repo from %s/aur: installs the newest .deb from its apt repository.\n", url)
	fmt.Fprintf(&b, "pkgname=%s\npkgver=%s\npkgrel=1\n", pkgname, pkgver)
	// Quoted as sh quotes, since makepkg reads the PKGBUILD as a script.
	fmt.Fprintf(&b, "pkgdesc=%s\narch=(%s)\n", shellQuote(a.Summary), strings.Join(arches, " "))
	if a.Homepage != "" {
		fmt.Fprintf(&b, "url=%s\n", shellQuote(a.Homepage))
	}
	fmt.Fprintf(&b, "license=('%s')\ndepends=(%s)\n", licence, strings.Join(depends, " "))
	if len(optdepends) > 0 {
		fmt.Fprintf(&b, "optdepends=(%s)\n", strings.Join(optdepends, " "))
	}
	fmt.Fprintf(&b, "provides=('%s')\nconflicts=('%s')\n", a.Name, a.Name)
	// Built already: nothing to strip, and no debug package to make.
	b.WriteString("options=('!debug' '!strip')\n")
	for _, s := range sources {
		fmt.Fprintf(&b, "source_%s=(%s)\nsha256sums_%s=('%s')\n", s.arch, shellQuote(s.url), s.arch, s.sum)
	}
	b.WriteString(`
package() {
  # makepkg has unpacked the .deb; its files are in data.tar.
  bsdtar -xf data.tar.* -C "$pkgdir"
}
`)
	var info strings.Builder
	fmt.Fprintf(&info, "pkgbase = %s\n\tpkgdesc = %s\n\tpkgver = %s\n\tpkgrel = 1\n", pkgname, a.Summary, pkgver)
	if a.Homepage != "" {
		fmt.Fprintf(&info, "\turl = %s\n", a.Homepage)
	}
	for _, s := range sources {
		fmt.Fprintf(&info, "\tarch = %s\n", s.arch)
	}
	fmt.Fprintf(&info, "\tlicense = %s\n", licence)
	for _, d := range deps {
		fmt.Fprintf(&info, "\tdepends = %s\n", d)
	}
	for _, d := range optdeps {
		fmt.Fprintf(&info, "\toptdepends = %s\n", d)
	}
	fmt.Fprintf(&info, "\tprovides = %s\n\tconflicts = %s\n\toptions = !debug\n\toptions = !strip\n", a.Name, a.Name)
	for _, s := range sources {
		fmt.Fprintf(&info, "\tsource_%s = %s\n\tsha256sums_%s = %s\n", s.arch, s.url, s.arch, s.sum)
	}
	fmt.Fprintf(&info, "\npkgname = %s\n", pkgname)
	dir := filepath.Join(site, "aur")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "PKGBUILD"), []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".SRCINFO"), []byte(info.String()), 0o644)
}
