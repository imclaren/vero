package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// releaseCommand is the whole of a release in one command: it works out
// the version, makes the signing key if there is none, builds every
// installer this machine has the tools for, builds the site, and says how
// to publish it - or does, with --upload. Nothing in it needs an account
// with anyone: the Mac app is signed ad hoc and the Windows installer
// not at all, which the site's page explains to people; the Linux and
// BSD repositories are signed with the key, as they must be. Credentials
// in the environment (VERO_MAC_IDENTITY and the rest, which `vero-repo
// credentials` lists) are used when they are there.
func releaseCommand(args []string) error {
	fs := flag.NewFlagSet("release", flag.ExitOnError)
	appPath := fs.String("app", "vero-app.toml", "the app's vero-app.toml")
	version := fs.String("version", "", "the version to release (default: the patch after the newest released)")
	bump := fs.String("bump", "patch", "which part of the version to raise when none is given: patch, minor or major")
	keyDir := fs.String("key", "", "the signing key's folder (default: one named after the publisher, made if missing)")
	url := fs.String("url", "", "where the site will be (default: site in vero-app.toml, or a local address for a preview)")
	targets := fs.String("targets", "", "comma separated, as for package (default: everything this machine has the tools for)")
	notes := fs.String("notes", "", "what's new, for the Mac's update prompt")
	notesFile := fs.String("notes-file", "", "the same, from a file")
	upload := fs.String("upload", "", "where to rsync the site to when it's built, such as user@host:/srv/myapp")
	packages := fs.String("packages", "dist/packages", "where the installers go")
	out := fs.String("out", "dist/site", "the site's folder; an update builds into the same one")
	ldflags := fs.String("ldflags", "", "more of the worker's build flags")
	fs.Parse(args)

	a, err := LoadApp(*appPath)
	if err != nil {
		return err
	}

	// The version: given, or the next after the newest the site has or
	// the file says.
	if *version == "" {
		newest := a.Version
		if data, err := os.ReadFile(filepath.Join(*out, "latest.json")); err == nil {
			var l Latest
			if json.Unmarshal(data, &l) == nil && compareVersions(l.Version, newest) > 0 {
				newest = l.Version
			}
		}
		if newest == "" {
			*version = "1.0.0"
		} else if *version, err = nextVersion(newest, *bump); err != nil {
			return err
		}
	}

	// The key: the folder named, or one for the publisher, made on the
	// first release and kept.
	if *keyDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		*keyDir = filepath.Join(dir, "vero-repo", strings.ToLower(strings.ReplaceAll(a.PublisherName(), " ", "-")))
	}
	if !fileExists(filepath.Join(*keyDir, privateFile)) {
		addr, err := mail.ParseAddress(a.Publisher)
		if err != nil {
			return fmt.Errorf("publisher %q should be \"Name <email>\"", a.Publisher)
		}
		fmt.Printf("making a signing key in %s, which signs this and every later release: back it up\n", *keyDir)
		if err := makeKey(*keyDir, addr.Name, addr.Address); err != nil {
			return err
		}
	}

	// The site's address: given, in the file, or a local one to look at.
	preview := false
	if *url == "" {
		*url = a.Site
	}
	if *url == "" {
		*url, preview = "http://localhost:8080", true
	}

	// The targets: asked for, or everything the app has that this machine
	// can build, saying what it can't and why.
	if *targets == "" {
		var can, cannot []string
		for _, t := range possibleTargets(a) {
			if why := missingFor(t.name); why != "" {
				cannot = append(cannot, t.name+" ("+why+")")
			} else {
				can = append(can, t.name)
			}
		}
		if len(can) == 0 {
			return errors.New("nothing can be built here: " + strings.Join(cannot, ", "))
		}
		*targets = strings.Join(can, ",")
		if len(cannot) > 0 {
			fmt.Printf("not built here: %s\n", strings.Join(cannot, ", "))
		}
	}

	vero, err := veroDir(a.dir)
	if err != nil {
		return err
	}
	fmt.Printf("==> %s %s: building %s\n", a.DisplayName, *version, *targets)
	pkgArgs := []string{"--app", *appPath, "--version", *version, "--targets", *targets, "--out", *packages, "--key", *keyDir, "--vero", vero}
	if *ldflags != "" {
		pkgArgs = append(pkgArgs, "--ldflags", *ldflags)
	}
	if err := packageCommand(pkgArgs); err != nil {
		return err
	}

	fmt.Printf("==> the site, for %s\n", *url)
	buildArgs := []string{"--app", *appPath, "--key", *keyDir, "--url", *url, "--packages", *packages, "--out", *out}
	if *notes != "" {
		buildArgs = append(buildArgs, "--notes", *notes)
	}
	if *notesFile != "" {
		buildArgs = append(buildArgs, "--notes-file", *notesFile)
	}
	if err := buildCommand(buildArgs); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(signingSummary(a, *targets))
	if *upload != "" {
		fmt.Printf("==> uploading to %s\n", *upload)
		if err := runIn("", "rsync", "-a", "--delete", filepath.Clean(*out)+"/", *upload); err != nil {
			return err
		}
		fmt.Printf("released %s %s at %s\n", a.DisplayName, *version, *url)
		return nil
	}
	fmt.Printf("released %s %s into %s\n", a.DisplayName, *version, *out)
	if preview {
		fmt.Printf("  look at it:   vero-site -dir %s\n", *out)
		fmt.Printf("  publish it:   put site = \"https://...\" in %s (where the folder will be), release again, and upload %s there\n", filepath.Base(*appPath), *out)
	} else {
		fmt.Printf("  look at it:   vero-site -dir %s\n", *out)
		fmt.Printf("  publish it:   upload %s to %s, or next time give --upload user@host:/path\n", *out, *url)
	}
	return nil
}

// nextVersion raises one part of a version: 1.2.3 to 1.2.4, 1.3.0 or 2.0.0.
func nextVersion(v, part string) (string, error) {
	fields := strings.Split(v, ".")
	for len(fields) < 3 {
		fields = append(fields, "0")
	}
	n := make([]int, 3)
	for i := range n {
		var err error
		if n[i], err = strconv.Atoi(fields[i]); err != nil {
			return "", fmt.Errorf("the version %q is not MAJOR.MINOR.PATCH; give --version", v)
		}
	}
	switch part {
	case "patch":
		n[2]++
	case "minor":
		n[1], n[2] = n[1]+1, 0
	case "major":
		n[0], n[1], n[2] = n[0]+1, 0, 0
	default:
		return "", fmt.Errorf("--bump %q: patch, minor or major", part)
	}
	return fmt.Sprintf("%d.%d.%d", n[0], n[1], n[2]), nil
}

// target is one thing package can build, and what the app needs for it.
type target struct{ name string }

// possibleTargets is every target the app's vero-app.toml has a front
// end for, in the order package builds them.
func possibleTargets(a *App) []target {
	var ts []target
	add := func(on bool, name string) {
		if on {
			ts = append(ts, target{name})
		}
	}
	add(a.GTK != nil, "deb")
	add(a.GTK != nil, "rpm")
	add(a.GTK != nil, "pacman")
	add(a.GTK != nil && a.GTK.Alpine != nil, "alpine")
	add(a.GTK != nil && a.GTK.Void != nil, "void")
	add(a.GTK != nil && a.GTK.Flatpak.Build, "flatpak")
	for _, sys := range unixSystems {
		add(a.GTK != nil && a.GTK.enabled(sys.name), sys.name)
	}
	add(a.Web != nil, "web")
	add(a.Android != nil, "android")
	add(a.IOS != nil, "ios")
	add(a.WASI != nil, "wasi")
	add(a.Plan9 != nil, "plan9")
	add(a.MacOS != nil, "macos")
	add(a.WPF != nil && a.WPF.MSIX.Build, "msix")
	add(a.WPF != nil, "windows")
	return ts
}

// missingFor is what this machine lacks to build a target, or "". Most
// need only Go; the rest need the system's own tools.
func missingFor(name string) string {
	have := func(tool string) bool { _, err := exec.LookPath(tool); return err == nil }
	switch name {
	case "flatpak":
		if !have("docker") || exec.Command("docker", "info").Run() != nil {
			return "needs Docker running"
		}
	case "macos":
		if !have("xcodebuild") || !have("swift") {
			return "needs Xcode"
		}
	case "windows":
		if !have("makensis") {
			return "needs makensis: brew install makensis"
		}
		fallthrough
	case "msix":
		if home, _ := os.UserHomeDir(); !have("dotnet") && !fileExists(filepath.Join(home, ".dotnet", "dotnet")) {
			return "needs dotnet: brew install dotnet"
		}
	case "android":
		if _, err := androidBuildTools(); err != nil {
			return "needs the Android SDK: scripts/setup-android.sh"
		}
		if !fileExists(javaTool("keytool")) && !have("keytool") {
			return "needs a JDK: brew install openjdk"
		}
	case "ios":
		if exec.Command("xcrun", "--sdk", "iphonesimulator", "--show-sdk-path").Run() != nil {
			return "needs Xcode and a Simulator runtime"
		}
	}
	return ""
}

// veroDir is vero's own folder: the module the app uses, wherever Go
// keeps it, for the scripts package needs.
func veroDir(appDir string) (string, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/imclaren/vero")
	cmd.Dir = appDir
	out, err := cmd.Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return "", errors.New("couldn't find vero's module from the app's go.mod: go get github.com/imclaren/vero")
	}
	return strings.TrimSpace(string(out)), nil
}

// signingSummary says how each installer built was signed, and what
// would sign it better: the one thing a release without credentials
// leaves to the publisher.
func signingSummary(a *App, targets string) string {
	has := func(t string) bool {
		for _, x := range strings.Split(targets, ",") {
			if x == t {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	b.WriteString("signed:\n")
	if has("deb") || has("rpm") || has("pacman") || has("alpine") || has("void") || has("flatpak") {
		b.WriteString("  Linux             with your key; nothing more to do\n")
	}
	for _, sys := range unixSystems {
		if has(sys.name) {
			b.WriteString("  the BSDs, illumos with your key; nothing more to do\n")
			break
		}
	}
	if has("macos") {
		if os.Getenv("VERO_MAC_IDENTITY") == "" {
			b.WriteString("  macOS             ad hoc: people allow it once in Privacy & Security, as the page says (Developer ID: vero-repo credentials)\n")
		} else if os.Getenv("VERO_NOTARY_PROFILE") == "" {
			b.WriteString("  macOS             with your Developer ID, not notarised (vero-repo credentials)\n")
		} else {
			b.WriteString("  macOS             with your Developer ID, notarised\n")
		}
	}
	if has("windows") {
		b.WriteString("  Windows           not signed: SmartScreen asks once, as the page says (a certificate: vero-repo credentials)\n")
	}
	if has("android") {
		if os.Getenv("VERO_ANDROID_KEYSTORE") == "" {
			b.WriteString("  Android           with the keystore beside your key, which installs and updates anywhere but Google Play\n")
		} else {
			b.WriteString("  Android           with your keystore\n")
		}
	}
	if has("ios") {
		if os.Getenv("VERO_IOS_IDENTITY") == "" {
			b.WriteString("  iOS               for the Simulator only; phones need TestFlight or the App Store (vero-repo credentials)\n")
		} else {
			b.WriteString("  iOS               an .ipa with your Apple identity, for TestFlight or the App Store\n")
		}
	}
	return b.String()
}
