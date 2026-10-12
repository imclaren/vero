package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestLocalVersion checks that a local build is numbered above the newest
// latest.json in the app's folder, by date and time, and that without one
// vero says so rather than numbering below a release.
func TestLocalVersion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "vero-app.toml"), []byte("name = \"a\"\n"), 0o644)
	if _, _, err := localVersion(dir, ""); err == nil || !strings.Contains(err.Error(), "--published") {
		t.Errorf("no published version: %v", err)
	}
	for path, v := range map[string]string{"dist/site/latest.json": "0.4.9", ".releases/desktop/latest.json": "0.5.0"} {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(path)), 0o755)
		os.WriteFile(filepath.Join(dir, path), []byte(`{"name": "a", "version": "`+v+`"}`), 0o644)
	}
	v, from, err := localVersion(dir, "")
	if err != nil || !regexp.MustCompile(`^0\.5\.0\.\d{12}$`).MatchString(v) || !strings.Contains(from, ".releases") {
		t.Errorf("version %q from %q: %v", v, from, err)
	}
	if !newer(v, "0.5.0") || newer(v, "0.5.1") || !newer("0.10.0", "0.9.9") {
		t.Errorf("%s is not between 0.5.0 and 0.5.1", v)
	}
	if v, _, _ := localVersion(dir, "1.2.3"); !strings.HasPrefix(v, "1.2.3.") {
		t.Errorf("--published 1.2.3 gave %s", v)
	}
	if _, _, err := localVersion(dir, "1.2"); err == nil {
		t.Error("1.2 was taken as a version")
	}
}
