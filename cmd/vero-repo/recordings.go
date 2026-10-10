package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// recording is a GIF of the app at work on one system, as vero's tests
// recorded it (scripts/test-repo.sh, scripts/record-mac.sh), for the
// install page.
type recording struct {
	System string // as people know it: "Fedora"
	File   string // in the site: "demo/fedora-latest.gif"
}

// recordingNames are the systems' names for test-repo.sh's folders, in the
// order the page shows them; a folder not here shows under its own name,
// after them.
var recordingNames = []struct{ prefix, name string }{
	{"macos", "Mac"}, {"windows", "Windows"},
	{"debian", "Debian"}, {"ubuntu", "Ubuntu"}, {"fedora", "Fedora"}, {"opensuse", "openSUSE"},
	{"archlinux", "Arch Linux"}, {"alpine", "Alpine"}, {"chimeralinux", "Chimera"},
	{"ghcr.io-void-linux", "Void"}, {"flatpak", "Flatpak"},
	{"vm-freebsd", "FreeBSD"}, {"vm-dragonfly", "DragonFly"}, {"vm-netbsd", "NetBSD"},
	{"vm-openbsd", "OpenBSD"}, {"vm-illumos", "illumos"},
}

// copyRecordings puts the passing recordings in dir (one folder a system,
// each with app.gif and a status file starting PASS) into site/demo, in
// place of the last release's, and says what they are. No dir, or one
// that isn't there, is none.
func copyRecordings(dir, site string) ([]recording, error) {
	demo := filepath.Join(site, "demo")
	if err := os.RemoveAll(demo); err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	type found struct {
		order int
		recording
	}
	var all []found
	for _, e := range entries {
		status, err := os.ReadFile(filepath.Join(dir, e.Name(), "status"))
		if !e.IsDir() || err != nil || !strings.HasPrefix(string(status), "PASS") {
			continue
		}
		gif, err := os.ReadFile(filepath.Join(dir, e.Name(), "app.gif"))
		if err != nil {
			continue
		}
		if err := os.MkdirAll(demo, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(demo, e.Name()+".gif"), gif, 0o644); err != nil {
			return nil, err
		}
		f := found{len(recordingNames), recording{e.Name(), "demo/" + e.Name() + ".gif"}}
		for i, n := range recordingNames {
			if strings.HasPrefix(e.Name(), n.prefix) {
				f.order, f.System = i, n.name
				break
			}
		}
		all = append(all, f)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].order < all[j].order })
	var out []recording
	seen := map[string]bool{}
	for _, f := range all {
		// One a system: ubuntu:24.04 and ubuntu:22.04 would both be Ubuntu.
		if seen[f.System] {
			continue
		}
		seen[f.System] = true
		out = append(out, f.recording)
	}
	return out, nil
}
