package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFind(t *testing.T) {
	dir := t.TempDir()
	name := "frobnicate"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755)
	if _, err := Find("frobnicate"); err == nil {
		t.Fatal("found a tool that is nowhere")
	}
	t.Setenv("VERO_TOOLS", dir)
	if p, err := Find("frobnicate"); err != nil || p != filepath.Join(dir, name) {
		t.Fatalf("VERO_TOOLS: %q, %v", p, err)
	}
	t.Setenv("VERO_TOOLS", "")
	Dirs = []string{dir}
	defer func() { Dirs = nil }()
	if p, err := Find("frobnicate"); err != nil || p != filepath.Join(dir, name) {
		t.Fatalf("Dirs: %q, %v", p, err)
	}
}
