package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUpgradeFiles checks that vero add records the files of vero's it
// copies, that vero upgrade updates one the app has not changed and
// refuses one it has, unless forced, and names it.
func TestUpgradeFiles(t *testing.T) {
	app := jobsApp(t)
	if _, err := add(app, []string{"gtk", "wpf"}, false); err != nil {
		t.Fatal(err)
	}
	rec := readRecord(app.Dir)
	for _, f := range []string{"gtk/vero.py", "gtk/vero_gtk.py", "windows/Vero.cs", "windows/VeroWpf.cs"} {
		if rec[f] == "" {
			t.Errorf("vero add did not record %s: %v", f, rec)
		}
	}
	app, _ = analyseWith(app.Dir, true)
	// An older copy, as vero wrote it, is updated; an edited one is not.
	os.WriteFile(filepath.Join(app.Dir, "gtk", "vero.py"), []byte("# vero 0.13's\n"), 0o644)
	rec["gtk/vero.py"] = hashOf([]byte("# vero 0.13's\n"))
	writeRecord(app.Dir, rec)
	os.WriteFile(filepath.Join(app.Dir, "gtk", "vero_gtk.py"), []byte("# changed by the app\n"), 0o644)
	u := &upgrader{dir: app.Dir, app: app}
	if err := u.copyFiles(); err != nil {
		t.Fatal(err)
	}
	if len(u.edited) != 1 || u.edited[0] != filepath.Join("gtk", "vero_gtk.py") {
		t.Errorf("edited %v, want gtk/vero_gtk.py", u.edited)
	}
	if !strings.Contains(strings.Join(u.changed, "\n"), "gtk/vero.py: updated") {
		t.Errorf("changed %v", u.changed)
	}
	if data, _ := os.ReadFile(filepath.Join(app.Dir, "gtk", "vero_gtk.py")); string(data) != "# changed by the app\n" {
		t.Error("the edited file was replaced")
	}
	u = &upgrader{dir: app.Dir, app: app, force: true}
	if err := u.copyFiles(); err != nil || len(u.edited) != 0 {
		t.Fatalf("forced: %v, edited %v", err, u.edited)
	}
	if data, _ := os.ReadFile(filepath.Join(app.Dir, "gtk", "vero_gtk.py")); !strings.Contains(string(data), "vero's GTK library") {
		t.Error("--force did not replace the edited file")
	}
}

// TestAddSwiftFile checks the edit of an Xcode project, on audiobooks':
// the file in the app's group and its target's sources, once.
func TestAddSwiftFile(t *testing.T) {
	data, err := os.ReadFile("testdata/upgrade/audiobooks.pbxproj")
	if err != nil {
		t.Fatal(err)
	}
	text, added, err := addSwiftFile(string(data), "audiobooks", "WorkerAPI.swift")
	if err != nil || !added {
		t.Fatalf("added %v: %v", added, err)
	}
	for _, want := range []string{
		"/* WorkerAPI.swift in Sources */ = {isa = PBXBuildFile; fileRef = ",
		"/* WorkerAPI.swift */ = {isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = WorkerAPI.swift; sourceTree = \"<group>\"; };",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q", want)
		}
	}
	// In the group whose path is audiobooks, and in the Sources phase.
	group := text[strings.Index(text, "/* audiobooks */ = {\n\t\t\tisa = PBXGroup;"):]
	group = group[:strings.Index(group, "};")]
	if !strings.Contains(group, "/* WorkerAPI.swift */,") {
		t.Errorf("not in the group:\n%s", group)
	}
	sources := text[strings.Index(text, "/* Sources */ = {\n\t\t\tisa = PBXSourcesBuildPhase;"):]
	sources = sources[:strings.Index(sources, "};")]
	if !strings.Contains(sources, "/* WorkerAPI.swift in Sources */,") {
		t.Errorf("not in the Sources phase:\n%s", sources)
	}
	if _, again, _ := addSwiftFile(text, "audiobooks", "WorkerAPI.swift"); again {
		t.Error("added twice")
	}
	// A project whose folders follow the disk needs nothing.
	if _, added, _ := addSwiftFile("objects = { PBXFileSystemSynchronizedRootGroup }", "x", "WorkerAPI.swift"); added {
		t.Error("added to a synchronised project")
	}
}

// TestWindowsProject checks the move to .NET 10, with Windows Forms.
func TestWindowsProject(t *testing.T) {
	app := jobsApp(t)
	os.MkdirAll(filepath.Join(app.Dir, "windows"), 0o755)
	os.WriteFile(filepath.Join(app.Dir, "vero-app.toml"), []byte("name = \"jobs\"\n\n[wpf]\nfolder = \"windows\"\n"), 0o644)
	proj := filepath.Join(app.Dir, "windows", "Jobs.csproj")
	os.WriteFile(proj, []byte("<Project><PropertyGroup><TargetFramework>net8.0-windows10.0.19041.0</TargetFramework><UseWPF>true</UseWPF></PropertyGroup></Project>"), 0o644)
	app, _ = analyseWith(app.Dir, true)
	u := &upgrader{dir: app.Dir, app: app}
	if err := u.windowsProject(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(proj)
	if !strings.Contains(string(data), "<TargetFramework>net10.0-windows10.0.19041.0</TargetFramework>") || !strings.Contains(string(data), "<UseWindowsForms>true</UseWindowsForms>") {
		t.Errorf("project:\n%s", data)
	}
}
