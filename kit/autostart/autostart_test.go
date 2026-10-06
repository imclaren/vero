package autostart

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestQuote(t *testing.T) {
	got := quote([]string{"/opt/my app/run", "--flag", `say "hi"`})
	want := `"/opt/my app/run" --flag "say \"hi\""`
	if got != want {
		t.Errorf("quote: %s, want %s", got, want)
	}
}

// TestDesktop writes, finds and removes the autostart entry, in a config
// folder of its own, on systems that keep one.
func TestDesktop(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("a .desktop file is for Linux and the BSDs")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := App{ID: "dev.vero.test", Name: "vero test", Exec: []string{"/usr/bin/vero-test", "--quiet"}}
	if on, err := Enabled(a); on || err != nil {
		t.Fatalf("before: %v, %v", on, err)
	}
	if err := Enable(a); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "autostart", "dev.vero.test.desktop"))
	if err != nil || !strings.Contains(string(data), "Exec=/usr/bin/vero-test --quiet\n") {
		t.Fatalf("the entry: %s, %v", data, err)
	}
	if on, _ := Enabled(a); !on {
		t.Error("not enabled after Enable")
	}
	if err := Disable(a); err != nil {
		t.Fatal(err)
	}
	if on, _ := Enabled(a); on {
		t.Error("enabled after Disable")
	}
}
