package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// shipPlan is what vero-repo plan --json says: what can be built and
// tested on this Mac.
type shipPlan struct {
	Name string `json:"name"`
	Test []struct {
		Name string `json:"name"`
		OK   bool   `json:"ok"`
	} `json:"test"`
}

// ship makes an app from a worker file if it is given one, then tests and
// records it on every system this Mac can, and releases it. It stops at
// the first failure.
func ship(args []string) error {
	fset := flag.NewFlagSet("ship", flag.ExitOnError)
	name := fset.String("name", "", "the app's name, for a new app (default: the file's name)")
	module := fset.String("module", "", "a new app's module path (default: its name)")
	local := fset.String("vero", os.Getenv("VERO_DIR"), "a vero checkout to use in place of the published one")
	planOnly := fset.Bool("plan", false, "say what it would do, and stop")
	noRelease := fset.Bool("no-release", false, "test and record, but do not release")
	noVMs := fset.Bool("no-vms", false, "test in containers only, which is quicker")
	noMac := fset.Bool("no-mac", false, "do not record the Mac app")
	only := fset.String("only", "", "test only these systems, space separated, such as \"macos debian\"")
	skip := fset.String("skip", "", "do not test these systems, space separated")
	version := fset.String("version", "", "the version to release (default: the patch after the newest released)")
	notes := fset.String("notes", "", "what is new, for the Mac's update prompt")
	upload := fset.String("upload", "", "where to rsync the site, such as user@host:/srv/myapp")
	url := fset.String("url", "", "where the site will be")
	var file string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		file, args = args[0], args[1:]
	}
	fset.Parse(args)
	if fset.NArg() > 0 && file == "" {
		file = fset.Arg(0)
	}

	dir := "."
	if file != "" {
		n := *name
		if n == "" {
			n = strings.TrimSuffix(filepath.Base(file), ".go")
		}
		if err := shipNew(file, n, *module, *local); err != nil {
			return err
		}
		dir = n
	}
	toml, err := filepath.Abs(filepath.Join(dir, "vero-app.toml"))
	if err != nil {
		return err
	}
	if _, err := os.Stat(toml); err != nil {
		return fmt.Errorf("no vero-app.toml here: run vero ship in an app's folder, or give it a worker, as in vero ship counter.go")
	}

	src, err := veroSource(*local)
	if err != nil {
		return err
	}
	repo, err := veroRepoPath(src)
	if err != nil {
		return err
	}
	if err := runAt(dir, nil, repo, "plan", "--app", toml); err != nil {
		return err
	}
	out, err := exec.Command(repo, "plan", "--app", toml, "--json").Output()
	if err != nil {
		return fmt.Errorf("vero-repo plan: %v", err)
	}
	var plan shipPlan
	if err := json.Unmarshal(out, &plan); err != nil {
		return err
	}
	if *planOnly {
		return nil
	}

	var systems []string
	mac := false
	for _, t := range plan.Test {
		switch {
		case !t.OK, *only != "" && !hasWord(*only, t.Name), hasWord(*skip, t.Name):
		case t.Name == "macos":
			mac = !*noMac
		default:
			systems = append(systems, t.Name)
		}
	}
	env := []string{"VERO_REPO=" + repo}
	if len(systems) > 0 {
		fmt.Printf("\ntesting on %s\n", strings.Join(systems, ", "))
		testArgs := []string{filepath.Join(src, "scripts", "test-all.sh"), "--app", toml, "--only", strings.Join(systems, " ")}
		if *noVMs {
			testArgs = append(testArgs, "--no-vms")
		}
		if err := runAt(dir, env, "sh", testArgs...); err != nil {
			return fmt.Errorf("a test failed: each system's log is in ~/.cache/vero/test-runs/")
		}
	}
	if mac {
		fmt.Println("\nrecording the Mac app")
		if err := recordMac(src, repo, toml, env); err != nil {
			return err
		}
	}
	if *noRelease {
		fmt.Println("\ntested; vero ship without --no-release releases it")
		return nil
	}
	fmt.Println("\nreleasing")
	rel := []string{"release", "--app", toml}
	for flag, v := range map[string]string{"--version": *version, "--notes": *notes, "--upload": *upload, "--url": *url} {
		if v != "" {
			rel = append(rel, flag, v)
		}
	}
	return runAt(dir, env, repo, rel...)
}

// shipNew makes an app called name from a worker file: vero new's module,
// with the file as cmd/worker/main.go, and the desktop front ends.
func shipNew(file, name, module, local string) error {
	worker, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(name, "vero-app.toml")); err == nil {
		return fmt.Errorf("%s is made already: run vero ship in %s/, which builds from %s/cmd/worker", name, name, name)
	}
	if err := newApp(name, module, local); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(name, "cmd", "worker", "main.go"), worker, 0o644); err != nil {
		return err
	}
	if err := run(name, "go", "mod", "tidy"); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	// No terminal, so that vero add asks nothing and takes its defaults.
	cmd := exec.Command(self, "add", "desktop", "--dir", name)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// recordMac builds the Mac app, at a version of its own since it is
// never released, makes a copy of it with a bundle ID of
// its own, so that it keeps its settings apart from the copy you use, and
// records that copy playing the app's steps.
func recordMac(src, repo, toml string, env []string) error {
	tmp, err := os.MkdirTemp("", "vero-mac.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := runAt(filepath.Dir(toml), env, repo, "package", "--app", toml, "--version", "0.0.0", "--targets", "macos", "--out", tmp); err != nil {
		return err
	}
	dmgs, _ := filepath.Glob(filepath.Join(tmp, "*.dmg"))
	if len(dmgs) != 1 {
		return fmt.Errorf("vero-repo package made %d disk images, not one", len(dmgs))
	}
	mnt := filepath.Join(tmp, "mnt")
	if err := runAt("", nil, "hdiutil", "attach", "-quiet", "-nobrowse", "-readonly", "-mountpoint", mnt, dmgs[0]); err != nil {
		return err
	}
	apps, _ := filepath.Glob(filepath.Join(mnt, "*.app"))
	if len(apps) == 1 {
		err = runAt("", nil, "ditto", apps[0], filepath.Join(tmp, filepath.Base(apps[0])))
	} else {
		err = fmt.Errorf("the disk image holds %d apps, not one", len(apps))
	}
	exec.Command("hdiutil", "detach", "-quiet", mnt).Run()
	if err != nil {
		return err
	}
	app := filepath.Join(tmp, filepath.Base(apps[0]))
	plist := filepath.Join(app, "Contents", "Info.plist")
	id, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier", plist).Output()
	if err != nil {
		return err
	}
	testID := strings.TrimSpace(string(id)) + ".test"
	if err := runAt("", nil, "/usr/libexec/PlistBuddy", "-c", "Set :CFBundleIdentifier "+testID, plist); err != nil {
		return err
	}
	if err := runAt("", nil, "codesign", "--force", "--deep", "--sign", "-", app); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	return runAt(filepath.Dir(toml), env, "sh", filepath.Join(src, "scripts", "record-mac.sh"),
		"--app", toml, "--bundle", app,
		"--clean", filepath.Join(home, "Library", "Preferences", testID+".plist"))
}

// setup installs what vero needs on this Mac, then says what is still
// missing.
func setup(args []string) error {
	fset := flag.NewFlagSet("setup", flag.ExitOnError)
	local := fset.String("vero", os.Getenv("VERO_DIR"), "a vero checkout to use in place of the published one")
	fset.Parse(args)
	src, err := veroSource(*local)
	if err != nil {
		return err
	}
	if err := runAt("", nil, "sh", filepath.Join(src, "scripts", "setup.sh"), "--no-build"); err != nil {
		return err
	}
	if _, err := veroRepoPath(src); err != nil {
		return err
	}
	return runAt("", nil, "sh", filepath.Join(src, "scripts", "doctor.sh"))
}

// toolVersion is the version of vero this tool was installed at, or ""
// when it was built from a checkout.
func toolVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return ""
}

// veroSource is a folder of vero's source whose scripts this tool runs:
// the checkout given, or vero at this tool's version, copied out of Go's
// module cache, whose files cannot be run or written.
func veroSource(local string) (string, error) {
	if local != "" {
		return filepath.Abs(local)
	}
	version := toolVersion()
	if version == "" {
		return "", fmt.Errorf("this vero was built from a checkout: give it with --vero DIR, or set VERO_DIR")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(home, ".cache", "vero", "src", version)
	if _, err := os.Stat(filepath.Join(dst, "scripts", "test-all.sh")); err == nil {
		return dst, nil
	}
	out, err := exec.Command("go", "mod", "download", "-json", "github.com/imclaren/vero@"+version).Output()
	if err != nil {
		return "", fmt.Errorf("go mod download github.com/imclaren/vero@%s: %v", version, err)
	}
	var mod struct{ Dir string }
	if err := json.Unmarshal(out, &mod); err != nil || mod.Dir == "" {
		return "", fmt.Errorf("go mod download github.com/imclaren/vero@%s said nothing of where it is", version)
	}
	tmp := dst + ".partial"
	os.RemoveAll(tmp)
	err = filepath.WalkDir(mod.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(mod.Dir, path)
		to := filepath.Join(tmp, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") || strings.HasSuffix(path, ".ps1") {
			mode = 0o755
		}
		return os.WriteFile(to, data, mode)
	})
	if err != nil {
		return "", err
	}
	return dst, os.Rename(tmp, dst)
}

// veroRepoPath is a vero-repo that matches this tool: built from the
// checkout when there is one, or installed at this tool's version into
// vero's cache.
func veroRepoPath(src string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if src != "" && fileExists(filepath.Join(src, "cmd", "vero-repo", "go.mod")) {
		bin := filepath.Join(home, ".cache", "vero", "bin", "vero-repo-checkout")
		if err := runAt("", nil, "go", "build", "-C", filepath.Join(src, "cmd", "vero-repo"), "-o", bin, "."); err != nil {
			return "", err
		}
		return bin, nil
	}
	version := toolVersion()
	if version == "" {
		if path, err := exec.LookPath("vero-repo"); err == nil {
			return path, nil
		}
		version = "latest"
	}
	gobin := filepath.Join(home, ".cache", "vero", "bin", version)
	bin := filepath.Join(gobin, "vero-repo")
	if version != "latest" && fileExists(bin) {
		return bin, nil
	}
	module := "github.com/imclaren/vero/cmd/vero-repo@" + version
	fmt.Fprintf(os.Stderr, "installing %s\n", module)
	cmd := exec.Command("go", "install", module)
	cmd.Env = append(os.Environ(), "GOBIN="+gobin)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go install %s: %v", module, err)
	}
	return bin, nil
}

// runAt runs a command in dir, with env added to this one's, showing its
// output.
func runAt(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, append(os.Environ(), env...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func hasWord(list, word string) bool {
	for _, w := range strings.Fields(list) {
		if w == word {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
