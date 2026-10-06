package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/image/draw"
)

// linuxArches are the architectures the Linux packages are built for: Go's
// name, and the name rpm and Flatpak use.
var linuxArches = []struct{ goarch, name string }{
	{"amd64", "x86_64"},
	{"arm64", "aarch64"},
}

// treeFile is one file of an installed app: its path once installed,
// what is in it, and its mode.
type treeFile struct {
	path string
	data []byte
	mode fs.FileMode
}

// buildWorker cross-compiles the app's worker for goos on goarch, as
// build-all.sh does, with main.version set, into dir.
func buildWorker(a *App, root, worker, goos, goarch, ldflags, dir string) (string, error) {
	out := filepath.Join(dir, a.Worker.Name+"-"+goos+"-"+goarch)
	flags := strings.TrimSpace("-s -w -X main.version=" + a.Version + " " + ldflags)
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", flags, "-o", out, worker)
	cmd.Dir, cmd.Stdout, cmd.Stderr = root, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	if goos == "darwin" && a.Worker.CGO {
		arch := map[string]string{"amd64": "x86_64"}[goarch]
		if arch == "" {
			arch = goarch
		}
		minimum := "13.0"
		if a.MacOS != nil {
			minimum = macMinimum(a.MacOS)
		}
		cmd.Env = append(cmd.Env, "CGO_ENABLED=1", "CC=clang -arch "+arch, "MACOSX_DEPLOYMENT_TARGET="+minimum)
	}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("building the worker for %s/%s: %w", goos, goarch, err)
	}
	return out, nil
}

// linuxTree is the app as a Linux package installs it under prefix (/usr,
// or /app in a Flatpak): its folder in lib with the worker beside it, the
// command in bin, the menu entry, the icon at the sizes a desktop uses,
// and its AppStream metadata. exec is the menu entry's command.
func linuxTree(a *App, prefix, workerPath, exec string) ([]treeFile, error) {
	return unixTree(a, prefix, workerPath, exec, a.GTK.python(""))
}

// unixTree is linuxTree on any Unix: the app under prefix, started with
// python.
func unixTree(a *App, prefix, workerPath, exec, python string) ([]treeFile, error) {
	lib := path.Join(prefix, "lib", a.Name)
	var files []treeFile
	err := appFiles(a, func(rel string, data []byte, mode fs.FileMode) {
		if rel == a.GTK.Entry {
			mode = 0o755
		}
		files = append(files, treeFile{path.Join(lib, filepath.ToSlash(rel)), data, mode})
	})
	if err != nil {
		return nil, err
	}
	worker, err := os.ReadFile(workerPath)
	if err != nil {
		return nil, err
	}
	launcher := fmt.Sprintf("#!/bin/sh\nexec %s %s/%s \"$@\"\n", python, lib, a.GTK.Entry)
	files = append(files,
		treeFile{path.Join(lib, a.Worker.Name), worker, 0o755},
		treeFile{path.Join(prefix, "bin", a.Name), []byte(launcher), 0o755},
		treeFile{path.Join(prefix, "share", "applications", a.ID+".desktop"), desktopEntry(a, exec), 0o644},
		treeFile{path.Join(prefix, "share", "metainfo", a.ID+".metainfo.xml"), appStream(a), 0o644},
	)
	icons, err := iconSizes(a.Path(a.Icon), 64, 128, 256, 512)
	if err != nil {
		return nil, err
	}
	for size, data := range icons {
		files = append(files, treeFile{path.Join(prefix, "share", "icons", "hicolor", fmt.Sprintf("%dx%d", size, size), "apps", a.ID+".png"), data, 0o644})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, nil
}

// appFiles calls add with each of the front end's files, relative to its
// folder: its own, and those it includes, but not what a run left behind
// (a worker, Python's caches).
func appFiles(a *App, add func(rel string, data []byte, mode fs.FileMode)) error {
	src := a.Path(a.GTK.Folder)
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == a.Worker.Name || strings.HasSuffix(d.Name(), ".pyc") {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		add(rel, data, mode)
		return nil
	})
	if err != nil {
		return err
	}
	for _, inc := range a.GTK.Include {
		data, err := os.ReadFile(a.Path(inc))
		if err != nil {
			return err
		}
		add(filepath.Base(inc), data, 0o644)
	}
	return nil
}

// desktopEntry is the app's menu entry, which runs exec.
func desktopEntry(a *App, exec string) []byte {
	categories := a.GTK.Categories
	if categories == "" {
		categories = "Utility;"
	}
	return []byte(fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Comment=%s
Exec=%s
Icon=%s
Categories=%s
StartupNotify=true
`, a.DisplayName, a.Summary, exec, a.ID, categories))
}

// iconSizes is the icon as a PNG at each size.
func iconSizes(file string, sizes ...int) (map[int][]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	out := map[int][]byte{}
	for _, size := range sizes {
		dst := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		var b bytes.Buffer
		if err := png.Encode(&b, dst); err != nil {
			return nil, err
		}
		out[size] = b.Bytes()
	}
	return out, nil
}
