// Package tools finds the helper programs an app ships beside its worker -
// ffmpeg, say - wherever the app is installed: inside the app bundle on
// macOS, beside the worker on Windows, under the app's lib folder on
// Linux, the BSDs and illumos, or anywhere the app names.
//
// Ship each as the plain program for each system and architecture, in the
// place vero-app.toml's [tools] section packages it to, and ask for it by
// name here; the path back is what os/exec takes.
package tools

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ErrNotFound is a tool that is not there.
var ErrNotFound = errors.New("tools: not found")

// Dirs are extra folders to look in, first: an app passes what its front
// end told it (macOS passes the bundle's Resources folder, for example), or
// a folder of its own.
var Dirs []string

// Find is the path of the tool called name, or ErrNotFound. It looks, in
// order, in Dirs, in the VERO_TOOLS environment variable's folders, in the
// places an installed app keeps them, and last on the PATH.
func Find(name string) (string, error) {
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		name += ".exe"
	}
	for _, dir := range candidates() {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", ErrNotFound
}

// candidates are the folders Find looks in, in order.
func candidates() []string {
	var dirs []string
	dirs = append(dirs, Dirs...)
	dirs = append(dirs, filepath.SplitList(os.Getenv("VERO_TOOLS"))...)
	exe, err := os.Executable()
	if err != nil {
		return dirs
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	here := filepath.Dir(exe)
	dirs = append(dirs, here, filepath.Join(here, "tools"))
	switch runtime.GOOS {
	case "darwin":
		// The worker in Contents/MacOS or Contents/Resources of the bundle,
		// or in the app's own bin folder vero copies it to: the bundle's
		// Resources, or that bin folder's tools.
		dirs = append(dirs, filepath.Join(here, "..", "Resources"), filepath.Join(here, "..", "..", "Contents", "Resources"))
	case "windows":
	default:
		// /usr/lib/<app> for a worker there, and the same under /usr/local,
		// /usr/pkg and /opt/local for the BSDs and illumos.
		dirs = append(dirs, filepath.Join(here, "..", "lib", filepath.Base(here)), filepath.Join(here, "..", "libexec", filepath.Base(here)))
	}
	return dirs
}
