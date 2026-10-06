package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// packageCommand builds an app's installers with vero's scripts:
// package-linux.sh for a GTK front end, package-windows.sh for a WPF one.
func packageCommand(args []string) error {
	fset := flag.NewFlagSet("package", flag.ExitOnError)
	appPath := fset.String("app", "vero-app.toml", "the app's vero-app.toml")
	version := fset.String("version", "", "the version to build; the file's when not given")
	targets := fset.String("targets", "", "linux, windows, or both, comma separated (default: every front end the app has)")
	out := fset.String("out", "dist/packages", "where the installers go")
	ldflags := fset.String("ldflags", "", "more of the worker's build flags: what your app builds into it")
	vero := fset.String("vero", "", "vero's folder (default: found from this program's source)")
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
	if *vero == "" {
		return errors.New("--vero is needed; scripts/package.sh gives it")
	}
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
	want := map[string]bool{}
	for _, t := range strings.Split(*targets, ",") {
		if t = strings.TrimSpace(t); t != "" {
			want[t] = true
		}
	}
	all := len(want) == 0
	did := false
	if a.GTK != nil && (all || want["linux"]) {
		if err := packageLinux(a, *vero, root, worker, outDir, *ldflags); err != nil {
			return err
		}
		did = true
	}
	if a.WPF != nil && (all || want["windows"]) {
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

func packageLinux(a *App, vero, root, worker, out, ldflags string) error {
	// The app's folder as it's installed: its own files, what it includes
	// beside them, and nothing a run left behind.
	home, _ := os.UserHomeDir()
	if err := os.MkdirAll(filepath.Join(home, ".cache"), 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Join(home, ".cache"), "vero-app.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	app := filepath.Join(stage, "app")
	src := a.Path(a.GTK.Folder)
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() {
			if d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == a.Worker.Name || strings.HasSuffix(d.Name(), ".pyc") {
			return nil
		}
		return copyFile(path, filepath.Join(app, rel))
	})
	if err != nil {
		return err
	}
	for _, inc := range a.GTK.Include {
		if err := copyFile(a.Path(inc), filepath.Join(app, filepath.Base(inc))); err != nil {
			return err
		}
	}
	metainfo := filepath.Join(stage, a.ID+".metainfo.xml")
	if err := os.WriteFile(metainfo, appStream(a), 0o644); err != nil {
		return err
	}
	args := []string{
		"--name", a.Name, "--display-name", a.DisplayName, "--version", a.Version,
		"--app", app, "--entry", a.GTK.Entry, "--worker", worker, "--worker-name", a.Worker.Name,
		"--icon", a.Path(a.Icon), "--summary", a.Summary, "--description", strings.TrimSpace(a.Description),
		"--maintainer", a.Publisher, "--id", a.ID, "--metainfo", metainfo, "--out", out,
	}
	if a.Homepage != "" {
		args = append(args, "--homepage", a.Homepage)
	}
	if a.GTK.Deb.Depends != "" {
		args = append(args, "--depends", a.GTK.Deb.Depends)
	}
	if a.GTK.Deb.Recommends != "" {
		args = append(args, "--recommends", a.GTK.Deb.Recommends)
	}
	if a.GTK.Categories != "" {
		args = append(args, "--categories", a.GTK.Categories)
	}
	if a.GTK.Deb.Section != "" {
		args = append(args, "--section", a.GTK.Deb.Section)
	}
	if ldflags != "" {
		args = append(args, "--ldflags", ldflags)
	}
	return run(root, filepath.Join(vero, "scripts", "package-linux.sh"), args)
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
		Developer: a.PublisherName(), Releases: []release{{a.Version}}, Rating: tagged{Type: "oars-1.1"},
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
