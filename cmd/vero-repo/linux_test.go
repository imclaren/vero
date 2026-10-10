package main

import (
	"bytes"
	"compress/gzip"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cavaliergopher/rpm"
)

// linuxApp is an app with a GTK front end, an icon and a worker, in a
// folder of its own.
func linuxApp(t *testing.T) (*App, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "gtk", "__pycache__"), 0o755)
	os.WriteFile(filepath.Join(dir, "gtk", "main.py"), []byte("print('hi')\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "gtk", "__pycache__", "main.cpython-312.pyc"), []byte("cache"), 0o644)
	os.WriteFile(filepath.Join(dir, "gtk", "worker"), []byte("left by a run"), 0o755)
	os.WriteFile(filepath.Join(dir, "vero.py"), []byte("# binding\n"), 0o644)
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	img.Set(1, 1, color.NRGBA{255, 0, 0, 255})
	var b bytes.Buffer
	png.Encode(&b, img)
	os.WriteFile(filepath.Join(dir, "icon.png"), b.Bytes(), 0o644)
	workers := map[string]string{}
	for _, arch := range linuxArches {
		w := filepath.Join(dir, "worker-"+arch.goarch)
		os.WriteFile(w, []byte("ELF "+arch.goarch), 0o755)
		workers[arch.goarch] = w
	}
	a := testApp()
	a.dir, a.Version, a.Licence, a.Icon, a.Homepage = dir, "1.2.3-beta", "MIT", "icon.png", "https://example.com"
	a.Worker = Worker{Package: "worker", Name: "worker"}
	a.GTK = &GTK{Folder: "gtk", Entry: "main.py", Include: []string{"vero.py"}, Categories: "Utility;",
		RPM: RPM{Requires: "python3 >= 3.10, python3-gobject, gtk4"}}
	return a, workers
}

func TestPackageRPM(t *testing.T) {
	a, workers := linuxApp(t)
	out := t.TempDir()
	if err := packageRPM(a, workers, out); err != nil {
		t.Fatal(err)
	}
	p, err := rpm.Open(filepath.Join(out, "vero-example-1.2.3~beta-1.aarch64.rpm"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "vero-example" || p.Version() != "1.2.3~beta" || p.Release() != "1" || p.Architecture() != "aarch64" {
		t.Errorf("the rpm is %s %s-%s %s", p.Name(), p.Version(), p.Release(), p.Architecture())
	}
	var requires []string
	for _, r := range p.Requires() {
		requires = append(requires, strings.TrimSpace(r.Name()+" "+r.Version()))
	}
	if got := strings.Join(requires, ", "); !strings.Contains(got, "python3 3.10") || !strings.Contains(got, "gtk4") {
		t.Errorf("requires %s", got)
	}
	files := map[string]rpm.FileInfo{}
	for _, f := range p.Files() {
		files[f.Name()] = f
	}
	for name, mode := range map[string]os.FileMode{
		"/usr/bin/vero-example":                                      0o755,
		"/usr/lib/vero-example/main.py":                              0o755,
		"/usr/lib/vero-example/vero.py":                              0o644,
		"/usr/lib/vero-example/worker":                               0o755,
		"/usr/share/applications/dev.vero.example.desktop":           0o644,
		"/usr/share/metainfo/dev.vero.example.metainfo.xml":          0o644,
		"/usr/share/icons/hicolor/128x128/apps/dev.vero.example.png": 0o644,
	} {
		f, ok := files[name]
		if !ok {
			t.Errorf("the rpm lacks %s", name)
		} else if f.Mode().Perm() != mode {
			t.Errorf("%s is %v, not %v", name, f.Mode().Perm(), mode)
		}
	}
	if f, ok := files["/usr/lib/vero-example"]; !ok || !f.IsDir() {
		t.Error("the rpm doesn't own its folder")
	}
	for name := range files {
		if strings.Contains(name, "__pycache__") {
			t.Errorf("the rpm holds a cache: %s", name)
		}
	}
}

func TestFlatpakBuildFolder(t *testing.T) {
	a, workers := linuxApp(t)
	a.Needs = Needs{Network: true, Keyring: true}
	// The app's own ffmpeg for each architecture, and its licence.
	os.MkdirAll(filepath.Join(a.dir, "ffmpeg", "aarch64"), 0o755)
	os.WriteFile(filepath.Join(a.dir, "ffmpeg", "aarch64", "ffmpeg"), []byte("ELF ffmpeg"), 0o755)
	os.WriteFile(filepath.Join(a.dir, "ffmpeg", "LICENSE.txt"), []byte("LGPL"), 0o644)
	a.GTK.Flatpak.Resources = []string{"ffmpeg/{arch}/ffmpeg", "ffmpeg/LICENSE.txt"}
	dir := t.TempDir()
	if err := flatpakBuildFolder(a, workers["arm64"], "aarch64", dir); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(dir, "files/lib/vero-example/ffmpeg")); err != nil || info.Mode()&0o111 == 0 {
		t.Errorf("the app's ffmpeg isn't beside the worker, runnable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "files/lib/vero-example/LICENSE.txt")); err != nil {
		t.Error(err)
	}
	read := func(rel string) string {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Error(err)
		}
		return string(data)
	}
	if got := read("files/lib/vero-example/worker"); got != "ELF arm64" {
		t.Errorf("the worker is %q", got)
	}
	if got := read("files/bin/vero-example"); !strings.Contains(got, "exec python3 /app/lib/vero-example/main.py") {
		t.Errorf("the launcher:\n%s", got)
	}
	desktop := read("export/share/applications/dev.vero.example.desktop")
	if !strings.Contains(desktop, "Exec=/usr/bin/flatpak run --branch=stable --arch=aarch64 --command=vero-example dev.vero.example\n") ||
		!strings.Contains(desktop, "X-Flatpak=dev.vero.example\n") {
		t.Errorf("the exported menu entry:\n%s", desktop)
	}
	meta := read("metadata")
	for _, want := range []string{
		"runtime=org.gnome.Platform/aarch64/51\n", "sdk=org.gnome.Sdk/aarch64/51\n", "command=vero-example\n",
		"shared=ipc;network;\n", "org.freedesktop.secrets=talk\n",
	} {
		if !strings.Contains(meta, want) {
			t.Errorf("metadata lacks %q:\n%s", want, meta)
		}
	}
	if strings.Contains(meta, "filesystems=") {
		t.Error("an app that doesn't need files was given the home folder")
	}
	f, err := os.Open(filepath.Join(dir, "files/share/app-info/xmls/dev.vero.example.xml.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	xml, _ := io.ReadAll(z)
	for _, want := range []string{"<components version=\"0.8\" origin=\"flatpak\">", "<bundle type=\"flatpak\" runtime=\"org.gnome.Platform/aarch64/51\">app/dev.vero.example/aarch64/stable</bundle>", "<id>dev.vero.example</id>"} {
		if !strings.Contains(string(xml), want) {
			t.Errorf("the AppStream collection lacks %q:\n%s", want, xml)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "files/share/app-info/icons/flatpak/128x128/dev.vero.example.png")); err != nil {
		t.Error(err)
	}
}

func TestFlatpakFiles(t *testing.T) {
	a, _ := linuxApp(t)
	a.Needs.Files = true
	if !strings.Contains(string(flatpakMetadata(a, "x86_64")), "filesystems=home;\n") {
		t.Error("an app that keeps files in a folder wasn't given the home folder")
	}
	a.GTK.Flatpak = Flatpak{RuntimeVersion: "50"}
	if !strings.Contains(string(flatpakMetadata(a, "x86_64")), "runtime=org.gnome.Platform/x86_64/50\n") {
		t.Error("the runtime's version wasn't the app's")
	}
}

// TestDataFiles checks that [[gtk.data]] lands under each package's
// prefix, filled in where it asks to be, and that LoadApp refuses one
// that would land outside it.
func TestDataFiles(t *testing.T) {
	a, workers := linuxApp(t)
	os.MkdirAll(filepath.Join(a.dir, "linux"), 0o755)
	os.WriteFile(filepath.Join(a.dir, "linux", "search.ini"), []byte("DesktopId={id}.desktop\n"), 0o644)
	os.WriteFile(filepath.Join(a.dir, "linux", "dbus.service"), []byte("Exec={prefix}/bin/{name}\n"), 0o644)
	a.GTK.Data = []DataFile{
		{From: "linux/search.ini", To: "share/gnome-shell/search-providers/dev.vero.example.search-provider.ini"},
		{From: "linux/dbus.service", To: "share/dbus-1/services/dev.vero.example.service", Expand: true},
	}
	for _, prefix := range []string{"/usr", "/usr/local", "/app"} {
		files, err := unixTree(a, prefix, workers["amd64"], a.Name, "python3")
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, f := range files {
			got[f.path] = string(f.data)
		}
		if s := got[prefix+"/share/gnome-shell/search-providers/dev.vero.example.search-provider.ini"]; s != "DesktopId={id}.desktop\n" {
			t.Errorf("%s: the search provider is %q", prefix, s)
		}
		if s := got[prefix+"/share/dbus-1/services/dev.vero.example.service"]; s != "Exec="+prefix+"/bin/vero-example\n" {
			t.Errorf("%s: the D-Bus service is %q", prefix, s)
		}
	}

	// The Flatpak shows both to the desktop, the service started through
	// flatpak.
	dir := t.TempDir()
	if err := flatpakBuildFolder(a, workers["amd64"], "x86_64", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "export", "share", "gnome-shell", "search-providers", "dev.vero.example.search-provider.ini")); err != nil {
		t.Error("the Flatpak does not export the search provider")
	}
	service, _ := os.ReadFile(filepath.Join(dir, "export", "share", "dbus-1", "services", "dev.vero.example.service"))
	if !strings.Contains(string(service), "Exec=/usr/bin/flatpak run ") {
		t.Errorf("the Flatpak's exported D-Bus service is %q", service)
	}

	toml := `name = "vero-example"
display_name = "vero example"
id = "dev.vero.example"
summary = "An example"
publisher = "Example <you@example.com>"
[worker]
package = "worker"
name = "worker"
[gtk]
folder = "gtk"
entry = "main.py"
[[gtk.data]]
from = "linux/search.ini"
to = "%s"
`
	for to, ok := range map[string]bool{"share/x.ini": true, "/usr/share/x.ini": false, "../etc/x": false, "share/../../x": false} {
		p := filepath.Join(a.dir, "vero-app.toml")
		os.WriteFile(p, []byte(strings.Replace(toml, "%s", to, 1)), 0o644)
		_, err := LoadApp(p)
		if (err == nil) != ok {
			t.Errorf("to = %q: %v", to, err)
		}
	}
}
