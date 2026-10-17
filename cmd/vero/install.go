package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// install builds the Mac app from this checkout, signs it, and puts it in
// /Applications in place of the copy there, numbered as localVersion
// says, so that neither Sparkle nor vero's worker replaces it with the
// release that is out.
func install(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	dir := fs.String("dir", ".", "the app's folder")
	published := fs.String("published", "", "the newest published version, where vero cannot find it")
	ldflags := fs.String("ldflags", "", "more of Go's -ldflags for the worker, such as -X values a release builds in")
	fs.Parse(args)
	version, from, err := localVersion(*dir, *published)
	if err != nil {
		return err
	}
	fmt.Printf("%s, above %s\n", version, from)
	repo, err := veroRepoPath(os.Getenv("VERO_DIR"))
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "vero-install")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	cmd := exec.Command(repo, installArgs(*dir, version, tmp, *ldflags)...)
	cmd.Env = append(os.Environ(), "VERO_NOTARISE=no")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// installArgs are vero-repo's arguments for vero install.
func installArgs(dir, version, out, ldflags string) []string {
	args := []string{"package", "--app", filepath.Join(dir, "vero-app.toml"), "--targets", "macos",
		"--version", version, "--build", version, "--install", "--out", out}
	if ldflags != "" {
		args = append(args, "--ldflags", ldflags)
	}
	return args
}

// localVersionCommand prints the number a local build of the app takes,
// for a prebuild script that numbers Xcode's own builds the same way.
func localVersionCommand(args []string) error {
	fs := flag.NewFlagSet("local-version", flag.ExitOnError)
	dir := fs.String("dir", ".", "the app's folder")
	published := fs.String("published", "", "the newest published version, where vero cannot find it")
	fs.Parse(args)
	version, _, err := localVersion(*dir, *published)
	if err != nil {
		return err
	}
	fmt.Println(version)
	return nil
}

// localVersion is the number for a build of the app on this computer:
// the newest published version, then the date and time, as
// 0.5.0.202610121530. That is above the release that is out, so that
// neither Sparkle nor vero's worker replaces the build with it, and below
// the next release, which replaces the build as usual. from says where
// the published version was found.
func localVersion(dir, published string) (version, from string, err error) {
	if published == "" {
		published, from = publishedVersion(dir)
	} else {
		from = "the version given"
	}
	if published == "" {
		return "", "", fmt.Errorf("vero cannot find the newest published version: no latest.json in the app's folder, on a releases branch or at its site. " +
			"Give it with --published 1.2.3, since a build numbered below the release would be replaced by it")
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(published) {
		return "", "", fmt.Errorf("the newest published version is %q, which is not three numbers such as 1.2.3", published)
	}
	return published + "." + time.Now().Format("200601021504"), from + " (" + published + ")", nil
}

// publishedVersion is the newest version among every latest.json vero can
// find for the app: in its folder, as vero ship and a release script leave
// them; on a releases branch, where an app may keep it; and at its site.
func publishedVersion(dir string) (string, string) {
	var best, where string
	consider := func(data []byte, source string) {
		var l struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &l) != nil || l.Version == "" {
			return
		}
		if best == "" || newer(l.Version, best) {
			best, where = l.Version, source
		}
	}
	root, _ := filepath.Abs(dir)
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			if strings.Count(rel, string(filepath.Separator)) > 3 || d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == ".build" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "latest.json" {
			data, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(root, p)
			consider(data, rel)
		}
		return nil
	})
	exec.Command("git", "-C", root, "fetch", "--quiet", "origin", "+refs/heads/releases:refs/remotes/origin/releases").Run()
	if out, err := exec.Command("git", "-C", root, "show", "origin/releases:latest.json").Output(); err == nil {
		consider(out, "the releases branch")
	}
	if site := readToml(filepath.Join(root, "vero-app.toml")).get("", "site"); site != "" {
		client := http.Client{Timeout: 5 * time.Second}
		if resp, err := client.Get(strings.TrimRight(site, "/") + "/latest.json"); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				consider(data, site)
			}
		}
	}
	return best, where
}

// newer says whether version a is after b, number by number.
func newer(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int64
		if i < len(pa) {
			x, _ = strconv.ParseInt(pa[i], 10, 64)
		}
		if i < len(pb) {
			y, _ = strconv.ParseInt(pb[i], 10, 64)
		}
		if x != y {
			return x > y
		}
	}
	return false
}
