package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCopyFrameworks checks that a framework beside a SwiftPM executable
// ends up in Contents/Frameworks with its symlinks kept, and that the
// executable is given the rpath that finds it there. Sparkle's framework,
// from the VeroUpdates product, is what this is for: without it the app
// does not start.
func TestCopyFrameworks(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("needs install_name_tool and otool")
	}
	tmp := t.TempDir()
	// One build folder per architecture, each with the same universal
	// framework, as an xcframework gives.
	var bins []string
	for _, arch := range []string{"arm64", "x86_64"} {
		bin := filepath.Join(tmp, arch, "release")
		versions := filepath.Join(bin, "Demo.framework", "Versions")
		if err := os.MkdirAll(filepath.Join(versions, "B", "Resources"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(versions, "B", "Demo"), []byte("the same binary"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("B", filepath.Join(versions, "Current")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("Versions/Current/Demo", filepath.Join(bin, "Demo.framework", "Demo")); err != nil {
			t.Fatal(err)
		}
		bins = append(bins, filepath.Join(bin, "App"))
	}
	contents := filepath.Join(tmp, "App.app", "Contents")
	if err := os.MkdirAll(filepath.Join(contents, "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A real Mach-O executable, for install_name_tool.
	exe := filepath.Join(contents, "MacOS", "App")
	if err := exec.Command("cp", "/bin/ls", exe).Run(); err != nil {
		t.Fatal(err)
	}
	if err := copyFrameworks(bins, contents, "App"); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(contents, "Frameworks", "Demo.framework")
	if data, err := os.ReadFile(filepath.Join(copied, "Demo")); err != nil || string(data) != "the same binary" {
		t.Fatalf("the framework's binary through its symlink: %q, %v", data, err)
	}
	if st, err := os.Lstat(filepath.Join(copied, "Versions", "Current")); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Error("Versions/Current is no longer a symlink")
	}
	out, err := exec.Command("otool", "-l", exe).Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "@executable_path/../Frameworks") {
		t.Error("the executable was not given the rpath to Contents/Frameworks")
	}
}

// TestCopyFrameworksNone checks that an app without frameworks is left
// alone, with no empty Frameworks folder.
func TestCopyFrameworksNone(t *testing.T) {
	tmp := t.TempDir()
	contents := filepath.Join(tmp, "App.app", "Contents")
	if err := copyFrameworks([]string{filepath.Join(tmp, "release", "App")}, contents, "App"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(contents, "Frameworks")); err == nil {
		t.Error("an empty Frameworks folder was made")
	}
}
