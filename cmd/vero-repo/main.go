// Command vero-repo makes the installers of a vero app, and the static
// site its users install and update them from: Debian and Ubuntu's apt
// repository, signed, the Windows installers, a page saying how to
// install on each system, and latest.json for the app's own update check.
// The site is plain files, to put on any web host.
//
//	vero-repo key --name "Example Publisher" --email you@example.com [--dir DIR]
//	vero-repo package --app vero-app.toml --version 1.2.3 [--targets linux,windows]
//	vero-repo build --app vero-app.toml --key DIR --url https://example.com/myapp
//
// key makes the key that signs every release, once; keep it safe, and out
// of your repository. package builds the installers into dist/packages,
// with vero's scripts. build adds them to the site in dist/site, keeping
// the newest few versions of each, and signs it. To release an update,
// package and build again into the same site, and upload it.
package main

import (
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
  vero-repo package --app vero-app.toml --version 1.2.3 [--targets linux,windows] [--out dist/packages]
  vero-repo build --app vero-app.toml --key DIR --url https://example.com/myapp [--packages dist/packages] [--out dist/site] [--keep 3]
`)
	os.Exit(2)
}

func keyCommand(args []string) error {
	fs := flag.NewFlagSet("key", flag.ExitOnError)
	name := fs.String("name", "", "whose key it is: your name, or your company's")
	email := fs.String("email", "", "an email address for the key")
	dir := fs.String("dir", "", "where to keep it (default ~/.config/vero-repo/NAME)")
	fs.Parse(args)
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
	fs.Parse(args)
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
	debs, _ := filepath.Glob(filepath.Join(*packages, "*.deb"))
	exes, _ := filepath.Glob(filepath.Join(*packages, "*.exe"))
	latest, err := build(*out, debs, exes, *keep, *url, a, s)
	if err != nil {
		return err
	}
	fmt.Printf("built %s: %s %s\n", *out, a.Name, latest.Version)
	fmt.Println("upload the folder to", *url)
	return nil
}

// build adds new installers to the site in out, and writes its indexes
// and pages.
func build(out string, debs, exes []string, keep int, url string, a *App, s *signer) (Latest, error) {
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
	}
	windows, err := buildWindows(out, exes, keep, a)
	if err != nil {
		return latest, err
	}
	for arch, d := range windows {
		note("windows-"+arch, d)
	}
	if len(latest.Downloads) == 0 {
		return latest, errors.New("no installers to publish: run vero-repo package first")
	}
	return latest, writeSite(out, url, a, latest, s)
}
