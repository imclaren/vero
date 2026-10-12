package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWorkerChecks checks that a worker which does not build for one of
// the app's systems is named, with the error that stops it, and that the
// others pass.
func TestWorkerChecks(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a worker for several systems")
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "cmd", "worker"), 0o755)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/w\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "cmd", "worker", "main.go"), []byte("package main\n\nfunc main() { notOnWindows() }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "cmd", "worker", "unix.go"), []byte("//go:build !windows\n\npackage main\n\nfunc notOnWindows() {}\n"), 0o644)
	a := &App{dir: dir, Worker: Worker{Package: "cmd/worker"}, WPF: &WPF{Arches: []string{"x64"}}, MacOS: &MacOS{},
		GTK: &GTK{Deb: Deb{Arches: []string{"amd64", "armel"}}}}
	steps := workerChecks(a)
	byName := map[string]PlanStep{}
	for _, s := range steps {
		byName[s.Name] = s
	}
	if s := byName["windows/amd64"]; s.OK || !strings.Contains(s.Why, "notOnWindows") {
		t.Errorf("windows/amd64: %+v", s)
	}
	for _, ok := range []string{"darwin/arm64", "darwin/amd64", "linux/amd64", "linux/armv5"} {
		if !byName[ok].OK {
			t.Errorf("%s did not build: %+v (all: %+v)", ok, byName[ok], steps)
		}
	}
	if text := workerText(steps); !strings.Contains(text, "worker fails on    windows/amd64") {
		t.Errorf("plan text:\n%s", text)
	}
}
