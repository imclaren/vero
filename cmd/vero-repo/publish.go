package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// publishCommand puts a release's listings where people look for them,
// beyond the site: a Homebrew tap, winget, the AUR. Each is a git push or
// a pull request made from files the site already holds, so each is done
// with gh and git, with nothing more to set up than an account.
//
//	vero-repo publish --homebrew    a tap of your own, USER/homebrew-tap, made if missing
//	vero-repo publish --winget      a pull request to microsoft/winget-pkgs
//	vero-repo publish --aur         NAME-bin on the Arch User Repository
//	vero-repo publish --all         whichever of those the site has files for
func publishCommand(args []string) error {
	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	appPath := fs.String("app", "vero-app.toml", "the app's vero-app.toml")
	site := fs.String("site", "dist/site", "the site vero-repo build made")
	homebrew := fs.Bool("homebrew", false, "push the cask to your Homebrew tap on GitHub")
	winget := fs.Bool("winget", false, "open a pull request to microsoft/winget-pkgs with the manifests")
	aur := fs.Bool("aur", false, "push the PKGBUILD to the Arch User Repository")
	all := fs.Bool("all", false, "each of those the site has files for")
	tap := fs.String("tap", "homebrew-tap", "the tap's repository name on GitHub")
	fs.Parse(args)
	a, err := LoadApp(*appPath)
	if err != nil {
		return err
	}
	l, err := readLatest(*site)
	if err != nil {
		return err
	}
	if *all {
		*homebrew = fileExists(filepath.Join(*site, "homebrew", a.Name+".rb"))
		*winget = fileExists(filepath.Join(*site, "winget"))
		*aur = fileExists(filepath.Join(*site, "aur", "PKGBUILD"))
	}
	if !*homebrew && !*winget && !*aur {
		return errors.New("publish where? --homebrew, --winget, --aur, or --all")
	}
	if *homebrew || *winget {
		if err := exec.Command("gh", "auth", "status").Run(); err != nil {
			return errors.New("gh isn't signed in: brew install gh && gh auth login")
		}
	}
	if *homebrew {
		if err := publishHomebrew(a, *site, l.Version, *tap); err != nil {
			return fmt.Errorf("homebrew: %w", err)
		}
	}
	if *winget {
		if err := publishWinget(a, *site, l.Version); err != nil {
			return fmt.Errorf("winget: %w", err)
		}
	}
	if *aur {
		if err := publishAUR(a, *site, l.Version); err != nil {
			return fmt.Errorf("aur: %w", err)
		}
	}
	return nil
}

func readLatest(site string) (Latest, error) {
	var l Latest
	data, err := os.ReadFile(filepath.Join(site, "latest.json"))
	if err != nil {
		return l, fmt.Errorf("%s has no latest.json: vero-repo build or release first", site)
	}
	return l, json.Unmarshal(data, &l)
}

// ghUser is who gh is signed in as.
func ghUser() (string, error) {
	out, err := exec.Command("gh", "api", "user", "--jq", ".login").Output()
	if err != nil {
		return "", errors.New("gh can't say who you are: gh auth login")
	}
	return strings.TrimSpace(string(out)), nil
}

// git runs git in a folder, its output shown only when it fails.
func git(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %v\n%s", args[0], err, out)
	}
	return nil
}

// publishHomebrew puts the cask in USER/homebrew-tap on GitHub, making the
// repository the first time. People then install with
// `brew install --cask USER/tap/NAME`.
func publishHomebrew(a *App, site, version, tap string) error {
	user, err := ghUser()
	if err != nil {
		return err
	}
	repo := user + "/" + tap
	if exec.Command("gh", "repo", "view", repo).Run() != nil {
		fmt.Printf("making %s on GitHub\n", repo)
		if err := runIn("", "gh", "repo", "create", repo, "--public", "--description", "Homebrew casks"); err != nil {
			return err
		}
	}
	dir, err := os.MkdirTemp("", "vero-tap.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := runIn("", "gh", "repo", "clone", repo, dir, "--", "--quiet"); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "Casks"), 0o755); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(site, "homebrew", a.Name+".rb"), filepath.Join(dir, "Casks", a.Name+".rb")); err != nil {
		return err
	}
	if err := git(dir, "add", "Casks"); err != nil {
		return err
	}
	if exec.Command("git", "-C", dir, "diff", "--cached", "--quiet").Run() == nil {
		fmt.Printf("the tap already has %s %s\n", a.Name, version)
		return nil
	}
	if err := git(dir, "commit", "-q", "-m", a.Name+" "+version); err != nil {
		return err
	}
	if err := git(dir, "push", "-q"); err != nil {
		return err
	}
	short := strings.TrimPrefix(tap, "homebrew-")
	fmt.Printf("published %s %s to the tap: brew install --cask %s/%s/%s\n", a.Name, version, user, short, a.Name)
	return nil
}

// publishWinget opens a pull request to microsoft/winget-pkgs with the
// release's manifests, from a fork of yours, which it makes the first
// time. The repository is huge, so only the manifests' folder is checked
// out.
func publishWinget(a *App, site, version string) error {
	user, err := ghUser()
	if err != nil {
		return err
	}
	manifests, err := filepath.Glob(filepath.Join(site, "winget", "manifests", "*", "*", "*", version))
	if err != nil || len(manifests) != 1 {
		return fmt.Errorf("no manifests for %s in %s/winget", version, site)
	}
	rel, _ := filepath.Rel(filepath.Join(site, "winget"), manifests[0])
	rel = filepath.ToSlash(rel)
	fork := user + "/winget-pkgs"
	if exec.Command("gh", "repo", "view", fork).Run() != nil {
		fmt.Println("forking microsoft/winget-pkgs")
		if err := runIn("", "gh", "repo", "fork", "microsoft/winget-pkgs", "--clone=false"); err != nil {
			return err
		}
	}
	dir, err := os.MkdirTemp("", "vero-winget.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	// A sparse, shallow clone of the upstream, so the branch starts from
	// today's master rather than a stale fork.
	if err := git("", "clone", "--quiet", "--depth", "1", "--filter=blob:none", "--sparse", "https://github.com/microsoft/winget-pkgs.git", dir); err != nil {
		return err
	}
	if err := git(dir, "sparse-checkout", "set", filepath.Dir(rel)); err != nil {
		return err
	}
	branch := strings.ReplaceAll(a.Name, "/", "-") + "-" + version
	if err := git(dir, "checkout", "-q", "-b", branch); err != nil {
		return err
	}
	dest := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	files, _ := filepath.Glob(filepath.Join(manifests[0], "*.yaml"))
	for _, f := range files {
		if err := copyFile(f, filepath.Join(dest, filepath.Base(f))); err != nil {
			return err
		}
	}
	if err := git(dir, "add", filepath.FromSlash(rel)); err != nil {
		return err
	}
	title := fmt.Sprintf("New version: %s version %s", wingetID(a), version)
	if err := git(dir, "commit", "-q", "-m", title); err != nil {
		return err
	}
	if err := git(dir, "remote", "add", "fork", "https://github.com/"+fork+".git"); err != nil {
		return err
	}
	if err := git(dir, "push", "-q", "-f", "fork", branch); err != nil {
		return err
	}
	body := fmt.Sprintf("Manifests for %s %s, built by vero-repo from %s.", a.DisplayName, version, a.Site)
	if err := runIn(dir, "gh", "pr", "create", "--repo", "microsoft/winget-pkgs", "--head", user+":"+branch, "--title", title, "--body", body); err != nil {
		return err
	}
	return nil
}

// publishAUR pushes the PKGBUILD and .SRCINFO to NAME-bin on the AUR,
// which makes the package the first time. It needs your SSH key on your
// AUR account.
func publishAUR(a *App, site, version string) error {
	if exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "aur@aur.archlinux.org").Run() != nil {
		return errors.New("aur.archlinux.org doesn't know your SSH key: make an account there and add it")
	}
	pkg := a.Name + "-bin"
	dir, err := os.MkdirTemp("", "vero-aur.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := git("", "clone", "--quiet", "ssh://aur@aur.archlinux.org/"+pkg+".git", dir); err != nil {
		return err
	}
	for _, f := range []string{"PKGBUILD", ".SRCINFO"} {
		if err := copyFile(filepath.Join(site, "aur", f), filepath.Join(dir, f)); err != nil {
			return err
		}
	}
	if err := git(dir, "add", "PKGBUILD", ".SRCINFO"); err != nil {
		return err
	}
	if exec.Command("git", "-C", dir, "diff", "--cached", "--quiet").Run() == nil {
		fmt.Printf("the AUR already has %s %s\n", pkg, version)
		return nil
	}
	if err := git(dir, "commit", "-q", "-m", "Release "+version); err != nil {
		return err
	}
	if err := git(dir, "push", "-q", "origin", "HEAD:master"); err != nil {
		return err
	}
	fmt.Printf("published %s %s: https://aur.archlinux.org/packages/%s\n", pkg, version, pkg)
	return nil
}
