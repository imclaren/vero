package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// packageCommand builds an app's installers: in Go, from a GTK front end,
// a .deb and an rpm for Linux and a package for each Unix the app names;
// a Flatpak, in vero's tools container, when asked for; and from a WPF
// front end, the Windows installers, with package-windows.sh.
func packageCommand(args []string) error {
	fset := flag.NewFlagSet("package", flag.ExitOnError)
	appPath := fset.String("app", "vero-app.toml", "the app's vero-app.toml")
	version := fset.String("version", "", "the version to build; the file's when not given")
	targets := fset.String("targets", "", "comma separated: deb, rpm, flatpak, macos, windows, web, android, ios, wasi, plan9, freebsd, dragonfly, netbsd, illumos, openbsd; linux for deb and rpm, bsd for the BSDs and illumos (default: all the app has)")
	out := fset.String("out", "dist/packages", "where the installers go")
	ldflags := fset.String("ldflags", "", "more of the worker's build flags: what your app builds into it")
	vero := fset.String("vero", "", "vero's folder (default: found from this program's source)")
	buildNum := fset.String("build", "", "a Mac app's CFBundleVersion, when it numbers its builds apart from its versions")
	keyDir := fset.String("key", "", "the signing key's folder, which keeps the Android app's keystore")
	fset.Parse(args)
	a, err := LoadApp(*appPath)
	if err != nil {
		return err
	}
	if *version != "" {
		a.Version = *version
	}
	if a.Version == "" {
		return errors.New("which version? give --version, or version in vero-app.toml")
	}
	a.Build = *buildNum
	root, err := a.ModuleRoot()
	if err != nil {
		return err
	}
	worker, err := filepath.Rel(root, a.Path(a.Worker.Package))
	if err != nil {
		return err
	}
	worker = "./" + filepath.ToSlash(worker)
	outDir, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	want := map[string]bool{}
	for _, t := range strings.Split(*targets, ",") {
		if t = strings.TrimSpace(t); t != "" {
			want[t] = true
		}
	}
	all := len(want) == 0
	gtk := func(kind string) bool {
		if a.GTK == nil {
			return false
		}
		switch kind {
		case "deb", "rpm":
			return all || want["linux"] || want[kind]
		case "flatpak":
			return want["flatpak"] || a.GTK.Flatpak.Build && (all || want["linux"])
		}
		return a.GTK.enabled(kind) && (all || want["bsd"] || want[kind]) || want[kind]
	}
	for t := range want {
		if s := system(t); s != nil && a.GTK != nil && !a.GTK.enabled(t) {
			return fmt.Errorf("--targets %s: vero-app.toml has no [gtk.%s], which says what the app needs there", t, t)
		}
	}

	// Each worker once, for every package that needs it.
	home, _ := os.UserHomeDir()
	if err := os.MkdirAll(filepath.Join(home, ".cache"), 0o755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(filepath.Join(home, ".cache"), "vero-workers.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	workers := map[string]map[string]string{} // by GOOS, then GOARCH
	needWorkers := func(goos string, goarches ...string) (map[string]string, error) {
		if workers[goos] == nil {
			workers[goos] = map[string]string{}
		}
		for _, goarch := range goarches {
			if workers[goos][goarch] != "" {
				continue
			}
			w, err := buildWorker(a, root, worker, goos, goarch, *ldflags, dir)
			if err != nil {
				return nil, err
			}
			workers[goos][goarch] = w
		}
		return workers[goos], nil
	}
	linuxGoarches := []string{"amd64", "arm64"}
	did := false
	if gtk("deb") {
		w, err := needWorkers("linux", linuxGoarches...)
		if err != nil {
			return err
		}
		if err := packageDeb(a, w, outDir); err != nil {
			return err
		}
		did = true
	}
	if gtk("rpm") {
		w, err := needWorkers("linux", linuxGoarches...)
		if err != nil {
			return err
		}
		if err := packageRPM(a, w, outDir); err != nil {
			return err
		}
		did = true
	}
	if gtk("flatpak") {
		w, err := needWorkers("linux", linuxGoarches...)
		if err != nil {
			return err
		}
		t, err := newTools("")
		if err != nil {
			return err
		}
		if err := packageFlatpak(a, w, outDir, t); err != nil {
			return err
		}
		did = true
	}
	for i := range unixSystems {
		sys := &unixSystems[i]
		if !gtk(sys.name) {
			continue
		}
		var goarches []string
		for _, arch := range sys.arches {
			goarches = append(goarches, arch.goarch)
		}
		w, err := needWorkers(sys.goos, goarches...)
		if err != nil {
			return err
		}
		if err := packageUnix(a, sys, w, outDir); err != nil {
			return err
		}
		did = true
	}
	others := []struct {
		target string
		on     bool
		make   func() error
	}{
		{"web", a.Web != nil, func() error { return packageWeb(a, outDir) }},
		{"android", a.Android != nil, func() error { return packageAndroid(a, outDir, *keyDir) }},
		{"ios", a.IOS != nil, func() error { return packageIOS(a, outDir) }},
		{"wasi", a.WASI != nil, func() error { return packageWASI(a, root, outDir, *ldflags, dir) }},
		{"plan9", a.Plan9 != nil, func() error { return packagePlan9(a, root, worker, outDir, *ldflags, dir) }},
	}
	for _, o := range others {
		if o.on && (all || want[o.target]) {
			if err := o.make(); err != nil {
				return err
			}
			did = true
		}
	}
	if a.MacOS != nil && (all || want["macos"]) {
		if err := packageMac(a, root, worker, outDir, *ldflags, dir); err != nil {
			return err
		}
		did = true
	}
	if a.WPF != nil && (all || want["windows"]) {
		if *vero == "" {
			return errors.New("--vero is needed for Windows; scripts/package.sh gives it")
		}
		if err := packageWindows(a, *vero, root, worker, outDir, *ldflags); err != nil {
			return err
		}
		did = true
	}
	if !did {
		return errors.New("nothing to package: the app has no front end for those targets")
	}
	return nil
}

func packageWindows(a *App, vero, root, worker, out, ldflags string) error {
	args := []string{
		"--name", a.Name, "--version", a.Version, "--app", a.Path(a.WPF.Folder), "--exe", a.WPF.Exe,
		"--worker", worker, "--worker-name", a.Worker.Name + ".exe", "--icon", a.Path(a.Icon),
		"--publisher", a.PublisherName(), "--id", a.Name, "--out", out,
	}
	if a.Homepage != "" {
		args = append(args, "--url", a.Homepage)
	}
	if a.Needs.Autostart {
		args = append(args, "--startup", "Open "+a.DisplayName+" when I sign in")
	}
	if a.Needs.Webview {
		args = append(args, "--webview2")
	}
	if ldflags != "" {
		args = append(args, "--ldflags", ldflags)
	}
	return run(root, filepath.Join(vero, "scripts", "package-windows.sh"), args)
}

func run(dir, script string, args []string) error {
	cmd := exec.Command(script, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(script), err)
	}
	return nil
}

// appStream is the app's AppStream metadata, which software centres such
// as GNOME Software show: its name, summary, description and licence.
func appStream(a *App) []byte {
	type tagged struct {
		Type  string `xml:"type,attr"`
		Value string `xml:",chardata"`
	}
	type release struct {
		Version string `xml:"version,attr"`
		Date    string `xml:"date,attr"`
	}
	type component struct {
		XMLName         xml.Name `xml:"component"`
		Type            string   `xml:"type,attr"`
		ID              string   `xml:"id"`
		MetadataLicense string   `xml:"metadata_license"`
		ProjectLicense  string   `xml:"project_license,omitempty"`
		Name            string   `xml:"name"`
		Summary         string   `xml:"summary"`
		Description     struct {
			P []string `xml:"p"`
		} `xml:"description"`
		Launchable tagged    `xml:"launchable"`
		URL        *tagged   `xml:"url,omitempty"`
		Developer  string    `xml:"developer_name"`
		Releases   []release `xml:"releases>release"`
		Rating     tagged    `xml:"content_rating"`
	}
	c := component{
		Type: "desktop-application", ID: a.ID, MetadataLicense: "CC0-1.0", ProjectLicense: a.Licence,
		Name: a.DisplayName, Summary: a.Summary, Launchable: tagged{"desktop-id", a.ID + ".desktop"},
		Developer: a.PublisherName(), Releases: []release{{a.Version, now().UTC().Format("2006-01-02")}}, Rating: tagged{Type: "oars-1.1"},
	}
	for _, p := range strings.Split(strings.TrimSpace(a.Description), "\n\n") {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			c.Description.P = append(c.Description.P, p)
		}
	}
	if len(c.Description.P) == 0 {
		c.Description.P = []string{a.Summary}
	}
	if a.Homepage != "" {
		c.URL = &tagged{"homepage", a.Homepage}
	}
	data, _ := xml.MarshalIndent(c, "", "  ")
	return append([]byte(xml.Header), append(data, '\n')...)
}
