package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"filippo.io/edwards25519"
)

// The Mac app: an .app built with SwiftPM or Xcode, the worker inside it,
// signed and put on a disk image. With a Developer ID it is signed with
// that, and notarised when there is a notarytool profile; without one it is
// signed ad hoc, which runs once whoever opens it allows it.
//
// What signs it comes from the environment, never from vero-app.toml:
//
//	VERO_MAC_IDENTITY        "Developer ID Application: Name (TEAMID)"
//	VERO_MAC_INSTALLER       "Developer ID Installer: Name (TEAMID)", for a .pkg
//	VERO_NOTARY_PROFILE      a profile made with xcrun notarytool store-credentials

// MacOS is the front end for macOS.
type MacOS struct {
	// Folder is the front end's folder. A SwiftPM package names its
	// executable Product; an Xcode project names Project and Scheme.
	Folder  string `toml:"folder"`
	Product string `toml:"product"`
	Project string `toml:"project"`
	Scheme  string `toml:"scheme"`
	// Prebuild is a command run once in Folder before anything is built:
	// what downloads or builds the files Resources lists, say. Prepare is
	// run in Folder before each architecture is built, with GOARCH and CC
	// set for it: what builds a C archive of Go, say.
	Prebuild string `toml:"prebuild"`
	Prepare  string `toml:"prepare"`
	// Resources are more files for the app's Contents/Resources: helper
	// programs and their licences, say. Each program in it is signed
	// before the app around it.
	Resources []string `toml:"resources"`
	// Entitlements is a .plist of entitlements the app is signed with,
	// under the hardened runtime. The programs and bundles inside it are
	// signed without, as Apple asks.
	Entitlements string `toml:"entitlements"`
	// DownloadURL is where the disk images are downloaded from, when that
	// isn't the site's macos folder: a server of your own, say. The appcast
	// and the Homebrew cask point there.
	DownloadURL string `toml:"download_url"`
	// Menubar is an app that lives in the menu bar, with no Dock icon.
	Menubar bool `toml:"menubar"`
	// Minimum is the oldest macOS it runs on: "13.0" unless it says.
	Minimum string `toml:"minimum"`
	// Category is its App Store category, which Finder shows.
	Category string `toml:"category"`
	// Pkg also makes an installer package, beside the disk image.
	Pkg bool `toml:"pkg"`
	// Appcast is a second place in the site for the appcast, beside
	// macos/appcast.xml: where apps already installed look for it, such as
	// "appcast.xml" at the site's top.
	Appcast string `toml:"appcast"`
	// Feed and SparklePublicKey are what an app that updates itself with
	// Sparkle reads from its Info.plist: where the site's appcast is, and
	// the public half of the key that signs updates, which vero-repo key
	// prints. vero writes them into an Info.plist it makes; an Xcode
	// project's own Info.plist has to say them itself.
	Feed             string `toml:"feed"`
	SparklePublicKey string `toml:"sparkle_public_key"`
}

// macArches are what a Mac app is built for: one binary, for both.
var macArches = []struct{ goarch, swift string }{{"arm64", "arm64"}, {"amd64", "x86_64"}}

// macInfo is what package says about a disk image, beside it, for build:
// the versions in its Info.plist, which Sparkle compares, and the oldest
// macOS it runs on.
type macInfo struct {
	BundleVersion string `json:"bundleVersion"`
	ShortVersion  string `json:"shortVersion"`
	Minimum       string `json:"minimum"`
	App           string `json:"app"`
}

// macDisk is NAME-VERSION-macos.dmg, and macPkg NAME-VERSION-macos.pkg.
func macDisk(name string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `-(.+)-macos\.dmg$`)
}

func macPkg(name string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `-(.+)-macos\.pkg$`)
}

// macNotes is the page of what's new in a version, which the appcast
// links for Sparkle's update prompt.
func macNotes(name, version string) string { return fmt.Sprintf("%s-%s-notes.html", name, version) }

// notesPage is release notes as a small page: paragraphs, split at blank
// lines, and lines starting "- " as a list.
func notesPage(title, notes string) []byte {
	esc := func(s string) string { return html.EscapeString(strings.TrimSpace(s)) }
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>" + esc(title) + "</title>\n")
	b.WriteString("<style>body{font:13px -apple-system,system-ui,sans-serif;margin:12px;color-scheme:light dark}ul{padding-left:1.2em}</style></head><body>\n")
	for _, para := range regexp.MustCompile(`\n\s*\n`).Split(strings.TrimSpace(notes), -1) {
		inList := false
		var text []string
		flush := func() {
			if len(text) > 0 {
				b.WriteString("<p>" + esc(strings.Join(text, " ")) + "</p>\n")
				text = nil
			}
		}
		for _, line := range strings.Split(para, "\n") {
			if item, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok {
				flush()
				if !inList {
					b.WriteString("<ul>\n")
					inList = true
				}
				b.WriteString("<li>" + esc(item) + "</li>\n")
				continue
			}
			if inList {
				b.WriteString("</ul>\n")
				inList = false
			}
			text = append(text, line)
		}
		flush()
		if inList {
			b.WriteString("</ul>\n")
		}
	}
	b.WriteString("</body></html>\n")
	return []byte(b.String())
}

// packageMac builds the Mac app and its disk image, and a .pkg if asked,
// into out.
func packageMac(a *App, root, worker, out, ldflags, tmp string) error {
	m := a.MacOS
	// The worker, for both architectures, in one file.
	var workers []string
	for _, arch := range macArches {
		w, err := buildWorker(a, root, worker, "darwin", arch.goarch, ldflags, tmp)
		if err != nil {
			return err
		}
		workers = append(workers, w)
	}
	universal := filepath.Join(tmp, a.Worker.Name+"-darwin")
	if err := runIn("", "lipo", append([]string{"-create", "-output", universal}, workers...)...); err != nil {
		return err
	}

	if m.Prebuild != "" {
		cmd := exec.Command("sh", "-c", m.Prebuild)
		cmd.Dir, cmd.Stdout, cmd.Stderr = a.Path(m.Folder), os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("[macos] prebuild: %w", err)
		}
	}
	var app string
	var err error
	if m.Project != "" {
		app, err = buildXcodeApp(a, tmp)
	} else {
		app, err = buildSwiftPMApp(a, tmp)
	}
	if err != nil {
		return err
	}
	// The versions are this release's, whatever the project says: Sparkle
	// compares CFBundleVersion, and shows CFBundleShortVersionString.
	if err := setVersions(app, a.Version, a.buildNumber()); err != nil {
		return err
	}
	resources := filepath.Join(app, "Contents", "Resources")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		return err
	}
	if err := copyFile(universal, filepath.Join(resources, a.Worker.Name)); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(resources, a.Worker.Name), 0o755); err != nil {
		return err
	}
	for _, r := range m.Resources {
		src := a.Path(filepath.Join(m.Folder, r))
		if filepath.IsAbs(r) {
			src = r
		}
		st, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("[macos] resources: %w", err)
		}
		if err := copyFile(src, filepath.Join(resources, filepath.Base(r))); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Join(resources, filepath.Base(r)), st.Mode().Perm()); err != nil {
			return err
		}
	}
	info, err := readInfo(app)
	if err != nil {
		return err
	}

	identity := os.Getenv("VERO_MAC_IDENTITY")
	entitlements := ""
	if m.Entitlements != "" {
		entitlements = a.Path(filepath.Join(m.Folder, m.Entitlements))
	}
	if err := signApp(app, identity, entitlements); err != nil {
		return err
	}
	if identity == "" {
		fmt.Println("signed the Mac app ad hoc: set VERO_MAC_IDENTITY to sign it with your Developer ID")
	}

	// The disk image: the app, and a link to Applications to drag it to.
	stage := filepath.Join(tmp, "dmg")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return err
	}
	if err := runIn("", "ditto", app, filepath.Join(stage, filepath.Base(app))); err != nil {
		return err
	}
	if err := os.Symlink("/Applications", filepath.Join(stage, "Applications")); err != nil {
		return err
	}
	dmg := filepath.Join(out, fmt.Sprintf("%s-%s-macos.dmg", a.Name, a.Version))
	os.Remove(dmg)
	if err := runIn("", "hdiutil", "create", "-quiet", "-volname", a.DisplayName, "-srcfolder", stage, "-ov", "-format", "UDZO", dmg); err != nil {
		return err
	}
	if identity != "" {
		if err := runIn("", "codesign", "--force", "--timestamp", "--sign", identity, dmg); err != nil {
			return err
		}
	}
	if profile := os.Getenv("VERO_NOTARY_PROFILE"); profile != "" && identity != "" {
		fmt.Println("notarising the disk image, which takes a few minutes")
		if err := runIn("", "xcrun", "notarytool", "submit", dmg, "--keychain-profile", profile, "--wait"); err != nil {
			return err
		}
		if err := runIn("", "xcrun", "stapler", "staple", dmg); err != nil {
			return err
		}
	}
	info.App = filepath.Base(app)
	data, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(dmg+".json", append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Println("built", dmg)

	if m.Pkg {
		pkg := filepath.Join(out, fmt.Sprintf("%s-%s-macos.pkg", a.Name, a.Version))
		if err := buildMacPkg(a, app, pkg, filepath.Join(tmp, "pkg")); err != nil {
			return err
		}
		installer := os.Getenv("VERO_MAC_INSTALLER")
		if profile := os.Getenv("VERO_NOTARY_PROFILE"); profile != "" && installer != "" {
			fmt.Println("notarising the installer package")
			if err := runIn("", "xcrun", "notarytool", "submit", pkg, "--keychain-profile", profile, "--wait"); err != nil {
				return err
			}
			if err := runIn("", "xcrun", "stapler", "staple", pkg); err != nil {
				return err
			}
		}
		fmt.Println("built", pkg)
	}
	return nil
}

// setVersions writes this release's versions into an app's Info.plist.
func setVersions(app, short, build string) error {
	plist := filepath.Join(app, "Contents", "Info.plist")
	for _, kv := range [][2]string{{"CFBundleShortVersionString", short}, {"CFBundleVersion", build}} {
		if err := runIn("", "plutil", "-replace", kv[0], "-string", kv[1], plist); err != nil {
			return err
		}
	}
	return nil
}

// signApp signs an app inside out. A bundle's signature covers what is in
// it, so everything nested is signed first, deepest first: the programs
// and libraries - the worker, helpers in Resources, Sparkle's Autoupdate -
// then the bundles around them - .xpc services, helper .apps such as
// Sparkle's Updater.app, frameworks - and the app last, the only one with
// the entitlements. With an identity each has the hardened runtime and a
// timestamp, which notarisation needs; without, each is signed ad hoc.
// codesign --deep would seem to do this and doesn't: it signs nested code
// with the outer bundle's settings, and Apple has deprecated it.
func signApp(app, identity, entitlements string) error {
	args := []string{"--force", "--sign", "-"}
	if identity != "" {
		args = []string{"--force", "--options", "runtime", "--timestamp", "--sign", identity}
	}
	nested, err := nestedCode(app)
	if err != nil {
		return err
	}
	for _, path := range nested {
		if err := quietSign(append(append([]string{}, args...), path)); err != nil {
			return err
		}
	}
	last := append([]string{}, args...)
	if entitlements != "" {
		last = append(last, "--entitlements", entitlements)
	}
	if err := quietSign(append(last, app)); err != nil {
		return err
	}
	// Checked, as notarisation and Gatekeeper will.
	out, err := exec.Command("codesign", "--verify", "--deep", "--strict", app).CombinedOutput()
	if err != nil {
		return fmt.Errorf("the signed app doesn't verify: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// quietSign runs codesign, showing only what goes wrong.
func quietSign(args []string) error {
	out, err := exec.Command("codesign", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("codesign %s: %s", args[len(args)-1], strings.TrimSpace(string(out)))
	}
	return nil
}

// codeBundles are the bundles inside an app that are signed as bundles.
var codeBundles = []string{".app", ".xpc", ".framework", ".appex", ".bundle", ".plugin"}

// nestedCode is what is signed before the app itself: every Mach-O file
// and every code bundle under its Contents, deepest first, and at the same
// depth files before bundles.
func nestedCode(app string) ([]string, error) {
	type item struct {
		path   string
		depth  int
		bundle bool
	}
	var items []item
	err := filepath.Walk(filepath.Join(app, "Contents"), func(path string, st os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		depth := strings.Count(path, string(filepath.Separator))
		if st.IsDir() {
			for _, ext := range codeBundles {
				if strings.HasSuffix(path, ext) {
					items = append(items, item{path, depth, true})
				}
			}
			return nil
		}
		if !st.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		var magic [4]byte
		if n, _ := f.Read(magic[:]); n == 4 && isMachO(magic) {
			items = append(items, item{path, depth, false})
		}
		return nil
	})
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].depth != items[j].depth {
			return items[i].depth > items[j].depth
		}
		return !items[i].bundle && items[j].bundle
	})
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.path
	}
	return out, err
}

// buildMacPkg makes an installer package for the app that installs it in
// /Applications, as the user's own, so that Sparkle can update it.
//
// Not relocatable: left as pkgbuild makes it, Installer looks for a copy
// of the app anywhere on the disk - the build folder, Downloads - and
// installs over that one, reporting success, with /Applications empty.
// The switch for it is only in a component list, from pkgbuild --analyze.
//
// Given to whoever installs it: an installer runs as root, and Sparkle
// can't replace an app root owns without an administrator's password, so
// a postinstall script hands it to the user, as an app dragged from the
// disk image is. The owner is no part of the signature.
func buildMacPkg(a *App, app, pkg, work string) error {
	os.RemoveAll(work)
	root, scripts := filepath.Join(work, "root"), filepath.Join(work, "scripts")
	for _, d := range []string{root, scripts} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	name := filepath.Base(app)
	if err := runIn("", "ditto", app, filepath.Join(root, name)); err != nil {
		return err
	}
	components := filepath.Join(work, "components.plist")
	if err := exec.Command("pkgbuild", "--analyze", "--root", root, components).Run(); err != nil {
		return fmt.Errorf("pkgbuild --analyze: %w", err)
	}
	if err := runIn("", "/usr/libexec/PlistBuddy", "-c", "Set :0:BundleIsRelocatable false", components); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(scripts, "postinstall"), []byte(postinstall(name)), 0o755); err != nil {
		return err
	}
	component := filepath.Join(work, "component.pkg")
	if err := exec.Command("pkgbuild", "--root", root, "--component-plist", components, "--install-location", "/Applications",
		"--scripts", scripts, "--identifier", a.ID, "--version", a.Version, component).Run(); err != nil {
		return fmt.Errorf("pkgbuild: %w", err)
	}
	if err := checkPkg(component); err != nil {
		return err
	}
	dist := filepath.Join(work, "Distribution")
	if err := os.WriteFile(dist, []byte(distribution(a, name)), 0o644); err != nil {
		return err
	}
	args := []string{"--distribution", dist, "--package-path", work}
	if installer := os.Getenv("VERO_MAC_INSTALLER"); installer != "" {
		args = append(args, "--sign", installer, "--timestamp")
	}
	os.Remove(pkg)
	if out, err := exec.Command("productbuild", append(args, pkg)...).CombinedOutput(); err != nil {
		return fmt.Errorf("productbuild: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// postinstall gives the installed app to whoever installed it: the user
// Installer runs for or, from sudo or a management tool, whoever is at the
// screen. With nobody there, it stays root's.
func postinstall(app string) string {
	return `#!/bin/sh
# $2 is where the package installs to: /Applications.
APP="$2/` + app + `"
[ -d "$APP" ] || APP="/Applications/` + app + `"
[ -d "$APP" ] || exit 0
WHO="$USER"
case "$WHO" in ""|root) WHO=$(stat -f %Su /dev/console 2>/dev/null) ;; esac
case "$WHO" in ""|root|loginwindow|_mbsetupuser) exit 0 ;; esac
GROUP=$(id -gn "$WHO" 2>/dev/null) || exit 0
# Never a reason for the installation to fail.
chown -R "$WHO:$GROUP" "$APP" || true
exit 0
`
}

// distribution is the productbuild distribution: the one component, for
// both architectures, from the app's oldest macOS on.
func distribution(a *App, app string) string {
	id, v := xmlText(a.ID), xmlText(a.Version)
	return `<?xml version="1.0" encoding="utf-8"?>
<installer-gui-script minSpecVersion="2">
    <pkg-ref id="` + id + `">
        <bundle-version>
            <bundle CFBundleShortVersionString="` + v + `" CFBundleVersion="` + xmlText(a.buildNumber()) + `" id="` + id + `" path="` + xmlText(app) + `"/>
        </bundle-version>
    </pkg-ref>
    <product id="` + id + `" version="` + v + `"/>
    <title>` + xmlText(a.DisplayName) + `</title>
    <options customize="never" require-scripts="false" hostArchitectures="arm64,x86_64"/>
    <volume-check>
        <allowed-os-versions>
            <os-version min="` + xmlText(macMinimum(a.MacOS)) + `"/>
        </allowed-os-versions>
    </volume-check>
    <choices-outline><line choice="default"><line choice="` + id + `"/></line></choices-outline>
    <choice id="default"/>
    <choice id="` + id + `" visible="false"><pkg-ref id="` + id + `"/></choice>
    <pkg-ref id="` + id + `" version="` + v + `" onConclusion="none">component.pkg</pkg-ref>
</installer-gui-script>
`
}

// checkPkg makes sure a component package installs in place, and has its
// postinstall: a package that installs somewhere else says nothing, and
// neither does one that lost its script.
func checkPkg(component string) error {
	expanded := component + ".expanded"
	os.RemoveAll(expanded)
	defer os.RemoveAll(expanded)
	if err := exec.Command("pkgutil", "--expand", component, expanded).Run(); err != nil {
		return fmt.Errorf("pkgutil --expand: %w", err)
	}
	info, err := os.ReadFile(filepath.Join(expanded, "PackageInfo"))
	if err != nil {
		return err
	}
	if bytes.Contains(info, []byte("<relocate>")) && !bytes.Contains(info, []byte("<relocate/>")) {
		return errors.New("the installer package would relocate the app instead of installing it in /Applications")
	}
	if st, err := os.Stat(filepath.Join(expanded, "Scripts", "postinstall")); err != nil || st.Mode()&0o100 == 0 || !bytes.Contains(info, []byte("postinstall")) {
		return errors.New("the installer package has no postinstall script, so the app would be left owned by root")
	}
	return nil
}

// buildSwiftPMApp builds a SwiftPM executable for each architecture, joins
// them, and puts the result in an .app vero makes, with an Info.plist and
// an icon from vero-app.toml.
func buildSwiftPMApp(a *App, tmp string) (string, error) {
	m := a.MacOS
	folder := a.Path(m.Folder)
	var bins []string
	for _, arch := range macArches {
		if m.Prepare != "" {
			cmd := exec.Command("sh", "-c", m.Prepare)
			cmd.Dir, cmd.Stdout, cmd.Stderr = folder, os.Stdout, os.Stderr
			cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH="+arch.goarch, "CGO_ENABLED=1",
				"CC=clang -arch "+arch.swift, "MACOSX_DEPLOYMENT_TARGET="+macMinimum(m))
			if err := cmd.Run(); err != nil {
				return "", fmt.Errorf("[macos] prepare, for %s: %w", arch.goarch, err)
			}
		}
		scratch := filepath.Join(tmp, "swift-"+arch.swift)
		args := []string{"build", "-c", "release", "--arch", arch.swift, "--scratch-path", scratch, "--product", m.Product}
		if err := runIn(folder, "swift", args...); err != nil {
			return "", err
		}
		path, err := exec.Command("swift", append(args, "--show-bin-path")...).Output()
		if err != nil {
			return "", err
		}
		bins = append(bins, filepath.Join(strings.TrimSpace(string(path)), m.Product))
	}
	app := filepath.Join(tmp, a.DisplayName+".app")
	contents := filepath.Join(app, "Contents")
	for _, d := range []string{"MacOS", "Resources"} {
		if err := os.MkdirAll(filepath.Join(contents, d), 0o755); err != nil {
			return "", err
		}
	}
	if err := runIn("", "lipo", append([]string{"-create", "-output", filepath.Join(contents, "MacOS", m.Product)}, bins...)...); err != nil {
		return "", err
	}
	icns, err := appleIcon(a.Path(a.Icon))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(contents, "Resources", "AppIcon.icns"), icns, 0o644); err != nil {
		return "", err
	}
	return app, os.WriteFile(filepath.Join(contents, "Info.plist"), infoPlist(a), 0o644)
}

// buildXcodeApp builds an Xcode project's scheme for both architectures,
// unsigned, since vero signs it once the worker is inside.
func buildXcodeApp(a *App, tmp string) (string, error) {
	m := a.MacOS
	data := filepath.Join(tmp, "xcode")
	err := runIn(a.Path(m.Folder), "xcodebuild", "-quiet", "-project", m.Project, "-scheme", m.Scheme,
		"-configuration", "Release", "-derivedDataPath", data, "ARCHS=arm64 x86_64", "ONLY_ACTIVE_ARCH=NO",
		"MARKETING_VERSION="+a.Version, "CURRENT_PROJECT_VERSION="+a.buildNumber(),
		"CODE_SIGNING_ALLOWED=NO", "build")
	if err != nil {
		return "", err
	}
	apps, _ := filepath.Glob(filepath.Join(data, "Build", "Products", "Release", "*.app"))
	if len(apps) != 1 {
		return "", fmt.Errorf("xcodebuild made %d apps, not one", len(apps))
	}
	return apps[0], nil
}

func macMinimum(m *MacOS) string {
	if m.Minimum != "" {
		return m.Minimum
	}
	return "13.0"
}

// infoPlist is the Info.plist of an app vero makes.
func infoPlist(a *App) []byte {
	m := a.MacOS
	category := m.Category
	if category == "" {
		category = "public.app-category.utilities"
	}
	keys := [][2]string{
		{"CFBundleDevelopmentRegion", "en"},
		{"CFBundleExecutable", m.Product},
		{"CFBundleIconFile", "AppIcon"},
		{"CFBundleIdentifier", a.ID},
		{"CFBundleInfoDictionaryVersion", "6.0"},
		{"CFBundleName", a.DisplayName},
		{"CFBundleDisplayName", a.DisplayName},
		{"CFBundlePackageType", "APPL"},
		{"CFBundleShortVersionString", a.Version},
		{"CFBundleVersion", a.buildNumber()},
		{"LSApplicationCategoryType", category},
		{"LSMinimumSystemVersion", macMinimum(m)},
		{"NSHumanReadableCopyright", a.PublisherName()},
	}
	if m.Feed != "" {
		keys = append(keys, [2]string{"SUFeedURL", m.Feed})
	}
	if m.SparklePublicKey != "" {
		keys = append(keys, [2]string{"SUPublicEDKey", m.SparklePublicKey})
	}
	var b bytes.Buffer
	b.WriteString(xml.Header + `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n<plist version=\"1.0\">\n<dict>\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "\t<key>%s</key>\n\t<string>%s</string>\n", k[0], xmlText(k[1]))
	}
	b.WriteString("\t<key>NSHighResolutionCapable</key>\n\t<true/>\n")
	if m.Menubar {
		b.WriteString("\t<key>LSUIElement</key>\n\t<true/>\n")
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}

// readInfo is what an app's Info.plist says of its versions, read with
// plutil, which every Mac has.
func readInfo(app string) (macInfo, error) {
	var info macInfo
	plist := filepath.Join(app, "Contents", "Info.plist")
	out, err := exec.Command("plutil", "-convert", "json", "-o", "-", plist).Output()
	if err != nil {
		return info, fmt.Errorf("reading %s: %w", plist, err)
	}
	var keys map[string]any
	if err := json.Unmarshal(out, &keys); err != nil {
		return info, err
	}
	str := func(k string) string { s, _ := keys[k].(string); return s }
	info.BundleVersion, info.ShortVersion, info.Minimum = str("CFBundleVersion"), str("CFBundleShortVersionString"), str("LSMinimumSystemVersion")
	if info.BundleVersion == "" {
		return info, fmt.Errorf("%s has no CFBundleVersion, which updates are compared by", plist)
	}
	return info, nil
}

// appleIcon is an .icns of the icon, made of PNGs at the sizes Finder and
// the Dock use, as macOS has read them since 10.7.
func appleIcon(file string) ([]byte, error) {
	types := []struct {
		kind string
		size int
	}{{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024}, {"ic11", 32}, {"ic12", 64}, {"ic13", 256}, {"ic14", 512}}
	var sizes []int
	for _, t := range types {
		sizes = append(sizes, t.size)
	}
	pngs, err := iconSizes(file, sizes...)
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	for _, t := range types {
		body.WriteString(t.kind)
		binary.Write(&body, binary.BigEndian, uint32(8+len(pngs[t.size])))
		body.Write(pngs[t.size])
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

// runIn runs a program in dir, its output shown.
func runIn(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// sparkleFile is the key that signs Mac updates for Sparkle, beside the
// OpenPGP key: an Ed25519 seed, which Sparkle's own keys are too.
const sparkleFile = "sparkle.sec"

// sparkleKey signs Mac updates as Sparkle checks them: with Ed25519, from
// a seed, or - for a key Sparkle's own older tools made, which kept only
// the expanded key - from that.
type sparkleKey struct {
	seed     ed25519.PrivateKey // nil for an expanded key
	expanded []byte             // the scalar, then the prefix: 64 bytes
	public   []byte
}

// sign is the signature Sparkle's sign_update would give data.
func (k *sparkleKey) sign(data []byte) []byte {
	if k.seed != nil {
		return ed25519.Sign(k.seed, data)
	}
	// Ed25519 with the expanded key, as RFC 8032 signs once it has it.
	s, _ := edwards25519.NewScalar().SetBytesWithClamping(k.expanded[:32])
	h := sha512.New()
	h.Write(k.expanded[32:])
	h.Write(data)
	r, _ := edwards25519.NewScalar().SetUniformBytes(h.Sum(nil))
	R := (&edwards25519.Point{}).ScalarBaseMult(r).Bytes()
	h.Reset()
	h.Write(R)
	h.Write(k.public)
	h.Write(data)
	c, _ := edwards25519.NewScalar().SetUniformBytes(h.Sum(nil))
	S := edwards25519.NewScalar().MultiplyAdd(c, s, r)
	return append(R, S.Bytes()...)
}

// parseSparkleKey reads a Sparkle private key as base64: a 32-byte seed,
// as vero and Sparkle's generate_keys -x keep one; a seed and its public
// key; or an expanded key, alone or followed by its public key, as older
// Sparkle kept them.
func parseSparkleKey(text string) (*sparkleKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil {
		return nil, errors.New("a Sparkle private key is base64")
	}
	switch len(raw) {
	case ed25519.SeedSize:
		key := ed25519.NewKeyFromSeed(raw)
		return &sparkleKey{seed: key, public: key.Public().(ed25519.PublicKey)}, nil
	case 64:
		if key := ed25519.NewKeyFromSeed(raw[:32]); bytes.Equal(key[32:], raw[32:]) {
			return &sparkleKey{seed: key, public: key[32:]}, nil
		}
		s, err := edwards25519.NewScalar().SetBytesWithClamping(raw[:32])
		if err != nil {
			return nil, err
		}
		pub := (&edwards25519.Point{}).ScalarBaseMult(s).Bytes()
		return &sparkleKey{expanded: raw, public: pub}, nil
	case 96:
		// An expanded key followed by its public key, as older Sparkle kept
		// them in the keychain: the public key has to be the private key's.
		s, err := edwards25519.NewScalar().SetBytesWithClamping(raw[:32])
		if err != nil {
			return nil, err
		}
		pub := (&edwards25519.Point{}).ScalarBaseMult(s).Bytes()
		if !bytes.Equal(pub, raw[64:]) {
			return nil, errors.New("this Sparkle key's public half doesn't match its private half")
		}
		return &sparkleKey{expanded: raw[:64], public: pub}, nil
	}
	return nil, fmt.Errorf("a Sparkle private key is 32, 64 or 96 bytes, not %d", len(raw))
}

// PublicBase64 is SUPublicEDKey: what an app's Info.plist says.
func (k *sparkleKey) PublicBase64() string { return base64.StdEncoding.EncodeToString(k.public) }

// sparkle is the key that signs Mac updates: sparkle.sec in the key's
// folder - imported with vero-repo key --import-sparkle, or made the first
// time it's needed.
func (s *signer) sparkle() (*sparkleKey, error) {
	if s.dir == "" {
		return nil, errors.New("the signing key has no folder")
	}
	file := filepath.Join(s.dir, sparkleFile)
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(file, []byte(base64.StdEncoding.EncodeToString(key.Seed())+"\n"), 0o600); err != nil {
			return nil, err
		}
		k := &sparkleKey{seed: key, public: key.Public().(ed25519.PublicKey)}
		fmt.Printf("made %s, which signs Mac updates: keep it with private.asc\n", file)
		fmt.Printf("its public key, for sparkle_public_key in vero-app.toml or SUPublicEDKey in Info.plist: %s\n", k.PublicBase64())
		return k, nil
	}
	if err != nil {
		return nil, err
	}
	k, err := parseSparkleKey(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return k, nil
}

// importSparkle keeps an existing Sparkle private key in dir, so that apps
// already installed - which check updates against its public half - keep
// updating.
func importSparkle(dir, from string) error {
	file := filepath.Join(dir, sparkleFile)
	if _, err := os.Stat(file); err == nil {
		return fmt.Errorf("%s already has a Sparkle key, which isn't replaced", dir)
	}
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	k, err := parseSparkleKey(string(data))
	if err != nil {
		return fmt.Errorf("%s: %w", from, err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(file, []byte(strings.TrimSpace(string(data))+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Printf("kept the Sparkle key in %s; its public key is %s\n", file, k.PublicBase64())
	return nil
}

// buildMac puts new disk images in site/macos, keeps the newest keep, and
// writes the Sparkle appcast and the Homebrew cask from them.
func buildMac(site string, newDmgs, newPkgs []string, notes string, keep int, url string, a *App, s *signer) (map[string]Download, error) {
	dir := filepath.Join(site, "macos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	for _, dmg := range newDmgs {
		if !macDisk(a.Name).MatchString(filepath.Base(dmg)) {
			continue
		}
		info, err := os.ReadFile(dmg + ".json")
		if err != nil {
			return nil, fmt.Errorf("%s has no %s beside it, which vero-repo package writes", dmg, filepath.Base(dmg)+".json")
		}
		if err := copyFile(dmg, filepath.Join(dir, filepath.Base(dmg))); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(dmg)+".json"), info, 0o644); err != nil {
			return nil, err
		}
		if strings.TrimSpace(notes) != "" {
			v := macDisk(a.Name).FindStringSubmatch(filepath.Base(dmg))[1]
			page := notesPage(a.DisplayName+" "+v, notes)
			if err := os.WriteFile(filepath.Join(dir, macNotes(a.Name, v)), page, 0o644); err != nil {
				return nil, err
			}
		}
	}
	for _, pkg := range newPkgs {
		if macPkg(a.Name).MatchString(filepath.Base(pkg)) {
			if err := copyFile(pkg, filepath.Join(dir, filepath.Base(pkg))); err != nil {
				return nil, err
			}
		}
	}
	pkgs, pkgVersion, err := keepNewest(dir, macPkg(a.Name), keep)
	if err != nil {
		return nil, err
	}
	files, version, err := keepNewest(dir, macDisk(a.Name), keep)
	if err != nil {
		return nil, err
	}
	// What keepNewest removed takes its note, and its release notes, with it.
	jsons, _ := filepath.Glob(filepath.Join(dir, "*.dmg.json"))
	for _, n := range jsons {
		if _, err := os.Stat(strings.TrimSuffix(n, ".json")); os.IsNotExist(err) {
			os.Remove(n)
		}
	}
	kept := map[string]bool{}
	for _, f := range files {
		kept[macNotes(a.Name, version[f])] = true
	}
	pages, _ := filepath.Glob(filepath.Join(dir, a.Name+"-*-notes.html"))
	for _, p := range pages {
		if !kept[filepath.Base(p)] {
			os.Remove(p)
		}
	}
	if len(files) == 0 {
		return nil, nil
	}
	key, err := s.sparkle()
	if err != nil {
		return nil, err
	}
	url = strings.TrimRight(url, "/")
	downloads := url + "/macos"
	if a.MacOS != nil && a.MacOS.DownloadURL != "" {
		downloads = strings.TrimRight(a.MacOS.DownloadURL, "/")
	}
	var items []appcastItem
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return nil, err
		}
		var info macInfo
		note, err := os.ReadFile(filepath.Join(dir, f+".json"))
		if err != nil || json.Unmarshal(note, &info) != nil {
			return nil, fmt.Errorf("%s has no readable %s", f, f+".json")
		}
		st, _ := os.Stat(filepath.Join(dir, f))
		link := ""
		if _, err := os.Stat(filepath.Join(dir, macNotes(a.Name, version[f]))); err == nil {
			link = url + "/macos/" + macNotes(a.Name, version[f])
		}
		items = append(items, appcastItem{Notes: link,
			Title: a.DisplayName + " " + version[f], Date: st.ModTime().UTC().Format("Mon, 02 Jan 2006 15:04:05 +0000"),
			Version: info.BundleVersion, ShortVersion: info.ShortVersion, Minimum: info.Minimum,
			Enclosure: appcastEnclosure{URL: downloads + "/" + f, Length: len(data), Type: "application/octet-stream",
				Signature: base64.StdEncoding.EncodeToString(key.sign(data))},
		})
	}
	cast := appcast{Version: "2.0", Sparkle: "http://www.andymatuschak.org/xml-namespaces/sparkle", Channel: appcastChannel{
		Title: a.DisplayName, Link: url, Description: a.Summary, Language: "en", Items: items}}
	data, err := xml.MarshalIndent(cast, "", "  ")
	if err != nil {
		return nil, err
	}
	feed := append([]byte(xml.Header), append(data, '\n')...)
	if err := os.WriteFile(filepath.Join(dir, "appcast.xml"), feed, 0o644); err != nil {
		return nil, err
	}
	if a.MacOS != nil && a.MacOS.Appcast != "" {
		extra := a.MacOS.Appcast
		if filepath.IsAbs(extra) || strings.Contains(extra, "..") {
			return nil, fmt.Errorf("[macos] appcast %q is a path inside the site", extra)
		}
		if err := os.MkdirAll(filepath.Join(site, filepath.Dir(extra)), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(site, extra), feed, 0o644); err != nil {
			return nil, err
		}
	}
	newest := files[0]
	sum, err := fileSHA256(filepath.Join(dir, newest))
	if err != nil {
		return nil, err
	}
	var info macInfo
	note, _ := os.ReadFile(filepath.Join(dir, newest+".json"))
	json.Unmarshal(note, &info)
	if err := writeCask(site, downloads, url, a, version[newest], sum, info); err != nil {
		return nil, err
	}
	mac := map[string]Download{"universal": {Version: version[newest], URL: "macos/" + newest}}
	if len(pkgs) > 0 {
		mac["pkg"] = Download{Version: pkgVersion[pkgs[0]], URL: "macos/" + pkgs[0]}
	}
	return mac, nil
}

type appcast struct {
	XMLName xml.Name       `xml:"rss"`
	Version string         `xml:"version,attr"`
	Sparkle string         `xml:"xmlns:sparkle,attr"`
	Channel appcastChannel `xml:"channel"`
}

type appcastChannel struct {
	Title       string        `xml:"title"`
	Link        string        `xml:"link"`
	Description string        `xml:"description"`
	Language    string        `xml:"language"`
	Items       []appcastItem `xml:"item"`
}

type appcastItem struct {
	Title        string           `xml:"title"`
	Date         string           `xml:"pubDate"`
	Version      string           `xml:"sparkle:version"`
	ShortVersion string           `xml:"sparkle:shortVersionString"`
	Minimum      string           `xml:"sparkle:minimumSystemVersion,omitempty"`
	Notes        string           `xml:"sparkle:releaseNotesLink,omitempty"`
	Enclosure    appcastEnclosure `xml:"enclosure"`
}

type appcastEnclosure struct {
	URL       string `xml:"url,attr"`
	Length    int    `xml:"length,attr"`
	Type      string `xml:"type,attr"`
	Signature string `xml:"sparkle:edSignature,attr"`
}

// writeCask writes a Homebrew cask for the newest disk image, which a tap
// of the publisher's can carry, or homebrew-cask itself.
func writeCask(site, downloads, url string, a *App, version, sum string, info macInfo) error {
	dir := filepath.Join(site, "homebrew")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	app := info.App
	if app == "" {
		app = a.DisplayName + ".app"
	}
	homepage := a.Homepage
	if homepage == "" {
		homepage = url
	}
	cask := fmt.Sprintf(`cask %q do
  version %q
  sha256 %q

  url "%s/%s-#{version}-macos.dmg"
  name %q
  desc %q
  homepage %q

  app %q
end
`, a.Name, version, sum, downloads, a.Name, a.DisplayName, strings.TrimSuffix(a.Summary, "."), homepage, app)
	return os.WriteFile(filepath.Join(dir, a.Name+".rb"), []byte(cask), 0o644)
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func isMachO(m [4]byte) bool {
	switch binary.BigEndian.Uint32(m[:]) {
	case 0xfeedface, 0xfeedfacf, 0xcefaedfe, 0xcffaedfe, 0xcafebabe, 0xbebafeca:
		return true
	}
	return false
}
