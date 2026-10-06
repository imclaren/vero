// Command vero-repo makes the installers of a vero app, and the static
// site its users install and update them from: signed repositories for
// Debian and Ubuntu (apt), Fedora and openSUSE (rpm), FreeBSD and
// DragonFly (pkg) and OpenBSD, pkgsrc repositories for NetBSD and illumos,
// optionally Flatpak, a recipe for Arch's AUR, the Windows installers, a
// page saying how to install on each system, and latest.json for the
// app's own update check. The site is plain files, to put on any web host.
// Everything is made in Go, except a Flatpak, which needs Docker.
//
//	vero-repo key --name "Example Publisher" --email you@example.com [--dir DIR]
//	vero-repo package --app vero-app.toml --version 1.2.3 [--targets deb,rpm,freebsd,windows,...]
//	vero-repo build --app vero-app.toml --key DIR --url https://example.com/myapp
//
// key makes the key that signs every release, once; keep it safe, and out
// of your repository. package builds the installers into dist/packages.
// build adds them to the site in dist/site, keeping the newest few
// versions of each, and signs it. To release an update, package and build
// again into the same site, and upload it.
package main

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "key":
		err = keyCommand(os.Args[2:])
	case "package":
		err = packageCommand(os.Args[2:])
	case "build":
		err = buildCommand(os.Args[2:])
	case "check":
		err = checkCommand(os.Args[2:])
	case "deb":
		// What scripts/package-linux.sh builds its .deb with: a folder laid
		// out for dpkg-deb, made into a .deb in Go.
		if len(os.Args) != 4 {
			usage()
		}
		err = debFromFolder(os.Args[2], os.Args[3])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "vero-repo:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  vero-repo key --name "Your Name or Company" --email you@example.com [--dir DIR]
  vero-repo key --dir DIR --import-sparkle FILE
  vero-repo check --app vero-app.toml --url https://example.com/myapp [--key DIR] [dist/site]
  vero-repo package --app vero-app.toml --version 1.2.3 [--targets deb,rpm,flatpak,macos,windows,freebsd,dragonfly,netbsd,illumos,openbsd] [--out dist/packages]
  vero-repo build --app vero-app.toml --key DIR --url https://example.com/myapp [--packages dist/packages] [--out dist/site] [--keep 3] [--no-page]
`)
	os.Exit(2)
}

func keyCommand(args []string) error {
	fs := flag.NewFlagSet("key", flag.ExitOnError)
	name := fs.String("name", "", "whose key it is: your name, or your company's")
	email := fs.String("email", "", "an email address for the key")
	dir := fs.String("dir", "", "where to keep it (default ~/.config/vero-repo/NAME)")
	sparkle := fs.String("import-sparkle", "", "a Sparkle private key to keep, in a file, so that copies of a Mac app already installed keep updating")
	fs.Parse(args)
	if *sparkle != "" {
		if *dir == "" {
			return errors.New("--import-sparkle needs --dir, the key's folder")
		}
		return importSparkle(*dir, *sparkle)
	}
	if *name == "" || *email == "" {
		return errors.New("key needs --name and --email")
	}
	if *dir == "" {
		home, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		*dir = filepath.Join(home, "vero-repo", strings.ToLower(strings.ReplaceAll(*name, " ", "-")))
	}
	if err := makeKey(*dir, *name, *email); err != nil {
		return err
	}
	fmt.Printf("made a signing key in %s\n", *dir)
	fmt.Printf("  %s is the private key: keep it safe, back it up, and never publish it\n", filepath.Join(*dir, privateFile))
	fmt.Printf("  %s is the public key, which the site publishes\n", filepath.Join(*dir, publicFile))
	return nil
}

func buildCommand(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	appPath := fs.String("app", "vero-app.toml", "the app's vero-app.toml")
	packages := fs.String("packages", "dist/packages", "the installers vero-repo package made")
	keyDir := fs.String("key", "", "the signing key's folder, from vero-repo key")
	url := fs.String("url", "", "where the site will be, such as https://example.com/myapp")
	out := fs.String("out", "dist/site", "the site's folder; an update builds into the same one")
	keep := fs.Int("keep", 3, "how many versions of each installer to keep")
	noPage := fs.Bool("no-page", false, "no index.html or install.sh: for downloads you serve yourself, privately")
	notes := fs.String("notes", "", "what's new in this release, for the Mac's update prompt: paragraphs, and lines starting \"- \" as a list")
	notesFile := fs.String("notes-file", "", "the same as --notes, from a file")
	fs.Parse(args)
	if *notesFile != "" {
		data, err := os.ReadFile(*notesFile)
		if err != nil {
			return err
		}
		*notes = string(data)
	}
	if *keyDir == "" || *url == "" {
		return errors.New("build needs --key and --url")
	}
	a, err := LoadApp(*appPath)
	if err != nil {
		return err
	}
	s, err := loadKey(*keyDir)
	if err != nil {
		return err
	}
	var in packageFiles
	in.debs, _ = filepath.Glob(filepath.Join(*packages, "*.deb"))
	in.exes, _ = filepath.Glob(filepath.Join(*packages, "*.exe"))
	in.dmgs, _ = filepath.Glob(filepath.Join(*packages, "*.dmg"))
	in.macPkgs, _ = filepath.Glob(filepath.Join(*packages, "*-macos.pkg"))
	in.notes = *notes
	for _, pattern := range []string{"*-android.apk", "*-wasi-*.tar.gz", "*-plan9-*.tgz", "*-web.tar.gz"} {
		more, _ := filepath.Glob(filepath.Join(*packages, pattern))
		in.bundles = append(in.bundles, more...)
	}
	in.rpms, _ = filepath.Glob(filepath.Join(*packages, "*.rpm"))
	in.flatpaks, _ = filepath.Glob(filepath.Join(*packages, "*.flatpak"))
	unix, _ := filepath.Glob(filepath.Join(*packages, "*.pkg"))
	tgz, _ := filepath.Glob(filepath.Join(*packages, "*.tgz"))
	in.unix = append(unix, tgz...)
	in.noPage = *noPage
	// What the site offered before, which this release mustn't go back from.
	var previous *Latest
	if data, err := os.ReadFile(filepath.Join(*out, "latest.json")); err == nil {
		previous = &Latest{}
		if json.Unmarshal(data, previous) != nil {
			previous = nil
		}
	}
	latest, err := build(*out, in, *keep, *url, a, s, *keyDir)
	if err != nil {
		return err
	}
	var sparkle ed25519.PublicKey
	if _, err := os.Stat(filepath.Join(*out, "macos")); err == nil {
		if k, err := s.sparkle(); err == nil {
			sparkle = k.public
		}
	}
	if err := checkSite(*out, *url, a, sparkle, previous); err != nil {
		return err
	}
	fmt.Printf("built %s: %s %s\n", *out, a.Name, latest.Version)
	fmt.Println("upload the folder to", *url)
	return nil
}

// packageFiles are the installers vero-repo package made, by kind.
type packageFiles struct {
	debs, exes, rpms, flatpaks, dmgs []string
	// macPkgs are the Mac's installer packages, beside its disk images;
	// notes are what's new in this release, for the Mac's update prompt.
	macPkgs []string
	notes   string
	// bundles are the web, Android, WASI and Plan 9 bundles.
	bundles []string
	// unix are the packages for the BSDs and illumos.
	unix []string
	// noPage leaves out the public page and install.sh.
	noPage bool
}

// build adds new installers to the site in out, and writes its indexes
// and pages, in Go; only the Flatpak repository is made in vero's tools
// container, with the key in keyDir.
func build(out string, in packageFiles, keep int, url string, a *App, s *signer, keyDir string) (Latest, error) {
	debs, exes := in.debs, in.exes
	latest := Latest{Name: a.Name, Downloads: map[string]Download{}}
	note := func(key string, d Download) {
		latest.Downloads[key] = d
		if latest.Version == "" || compareVersions(d.Version, latest.Version) > 0 {
			latest.Version = d.Version
		}
	}
	if _, err := os.Stat(filepath.Join(out, "apt")); len(debs) > 0 || err == nil {
		newest, err := buildApt(out, debs, keep, a, s)
		if err != nil {
			return latest, err
		}
		for arch, d := range newest {
			rel, err := filepath.Rel(out, d.path)
			if err != nil {
				return latest, err
			}
			note("linux-"+arch, Download{d.control.Get("Version"), filepath.ToSlash(rel)})
		}
		if err := writeAUR(out, url, a, newest); err != nil {
			return latest, err
		}
	}
	if _, err := os.Stat(filepath.Join(out, "rpm")); len(in.rpms) > 0 || err == nil {
		rpms, err := buildRPM(out, in.rpms, keep, url, a, s)
		if err != nil {
			return latest, err
		}
		for arch, d := range rpms {
			note("rpm-"+arch, d)
		}
	}
	// Only Flatpak needs Docker: its repository is made by flatpak and
	// ostree, in vero's tools container.
	if _, err := os.Stat(filepath.Join(out, "flatpak")); len(in.flatpaks) > 0 || err == nil {
		t, err := newTools(keyDir)
		if err != nil {
			return latest, err
		}
		flatpaks, err := buildFlatpak(out, in.flatpaks, keep, url, a, s, t)
		if err != nil {
			return latest, err
		}
		for arch, d := range flatpaks {
			note("flatpak-"+arch, d)
		}
	}
	for i := range unixSystems {
		sys := &unixSystems[i]
		var mine []string
		match := unixPackage(a.Name)
		for _, p := range in.unix {
			if m := match.FindStringSubmatch(filepath.Base(p)); m != nil && m[2] == sys.name {
				mine = append(mine, p)
			}
		}
		if _, err := os.Stat(filepath.Join(out, sys.name)); len(mine) == 0 && err != nil {
			continue
		}
		var newest map[string]Download
		var err error
		switch sys.format {
		case "pkg":
			newest, err = buildPkgRepo(out, sys, mine, keep, url, a, s)
		case "pkgsrc":
			newest, err = buildPkgsrcRepo(out, sys, mine, keep, a, s)
		case "openbsd":
			newest, err = buildOpenBSDRepo(out, sys, mine, keep, a, s)
		}
		if err != nil {
			return latest, fmt.Errorf("the %s repository: %w", sys.label, err)
		}
		for arch, d := range newest {
			note(sys.name+"-"+arch, d)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "macos")); len(in.dmgs) > 0 || err == nil {
		mac, err := buildMac(out, in.dmgs, in.macPkgs, in.notes, keep, url, a, s)
		if err != nil {
			return latest, err
		}
		for arch, d := range mac {
			note("macos-"+arch, d)
		}
	}
	others, err := buildBundles(out, in.bundles, keep, a)
	if err != nil {
		return latest, err
	}
	for k, d := range others {
		note(k, d)
	}
	windows, err := buildWindows(out, exes, keep, a)
	if err != nil {
		return latest, err
	}
	for arch, d := range windows {
		note("windows-"+arch, d)
	}
	if err := writeWinget(out, url, a, windows); err != nil {
		return latest, err
	}
	if len(latest.Downloads) == 0 {
		return latest, errors.New("no installers to publish: run vero-repo package first")
	}
	return latest, writeSite(out, url, a, latest, s, in.noPage)
}
