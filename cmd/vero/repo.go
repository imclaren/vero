package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// veroRepo runs vero-repo with the arguments given, installing it first
// if it isn't on the PATH - at this tool's own version, so that the two
// match. It returns the exit status.
func veroRepo(args []string) int {
	path, err := exec.LookPath("vero-repo")
	if err != nil {
		path, err = installVeroRepo()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// installVeroRepo installs vero-repo with go install, at the version of
// vero this tool was built from, into Go's bin folder.
func installVeroRepo() (string, error) {
	version := "latest"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	module := "github.com/imclaren/vero/cmd/vero-repo@" + version
	fmt.Fprintf(os.Stderr, "installing %s\n", module)
	cmd := exec.Command("go", "install", module)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go install %s: %v", module, err)
	}
	out, err := exec.Command("go", "env", "GOPATH").Output()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(strings.TrimSpace(string(out)), "bin", "vero-repo")
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("vero-repo was installed, but not at %s: put Go's bin folder on your PATH", bin)
	}
	return bin, nil
}
