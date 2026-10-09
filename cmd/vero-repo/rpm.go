package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/rpmpack"
)

// packageRPM builds the app's rpm for each architecture, for Fedora and
// openSUSE, in Go: the same files as the .deb, under /usr. workers are the
// worker built for each of Go's architectures.
func packageRPM(a *App, workers map[string]string, out string) error {
	for _, arch := range a.linuxArches("rpm") {
		files, err := linuxTree(a, "/usr", workers[arch.goarch], a.Name)
		if err != nil {
			return err
		}
		md := rpmpack.RPMMetaData{
			Name: a.Name, Version: rpmVersion(a.Version), Release: "1", Arch: arch.name, OS: "linux",
			Summary: a.Summary, Description: strings.TrimSpace(a.Description), Licence: a.Licence,
			URL: a.Homepage, Vendor: a.PublisherName(), Packager: a.Publisher,
			BuildTime: time.Now(), Compressor: "gzip",
		}
		if md.Description == "" {
			md.Description = a.Summary
		}
		for list, deps := range map[*rpmpack.Relations]string{&md.Requires: a.GTK.RPM.Requires, &md.Recommends: a.GTK.RPM.Recommends} {
			for _, d := range splitList(deps) {
				if err := list.Set(d); err != nil {
					return fmt.Errorf("[gtk.rpm] %q: %w", d, err)
				}
			}
		}
		r, err := rpmpack.NewRPM(md)
		if err != nil {
			return err
		}
		// The app's own folders, so that removing it leaves none behind.
		dirs := map[string]bool{}
		lib := "/usr/lib/" + a.Name
		mtime := uint32(time.Now().Unix())
		for _, f := range files {
			for d := path.Dir(f.path); strings.HasPrefix(d, lib); d = path.Dir(d) {
				dirs[d] = true
			}
		}
		for d := range dirs {
			r.AddFile(rpmpack.RPMFile{Name: d, Mode: 040755, Owner: "root", Group: "root", MTime: mtime})
		}
		for _, f := range files {
			r.AddFile(rpmpack.RPMFile{Name: f.path, Body: f.data, Mode: uint(f.mode), Owner: "root", Group: "root", MTime: mtime})
		}
		if err := os.MkdirAll(out, 0o755); err != nil {
			return err
		}
		name := filepath.Join(out, fmt.Sprintf("%s-%s-1.%s.rpm", a.Name, md.Version, arch.name))
		f, err := os.Create(name)
		if err != nil {
			return err
		}
		if err := r.Write(f); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Println("built", name)
	}
	return nil
}

// rpmVersion is a version as rpm allows it: no hyphens, which divide the
// version from the release. "1.2.3-beta" becomes "1.2.3~beta", which rpm,
// like Debian, sorts before 1.2.3.
func rpmVersion(v string) string { return strings.ReplaceAll(v, "-", "~") }

// splitList is a comma-separated list, each item trimmed.
func splitList(s string) []string {
	var out []string
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// rpmFile is NAME-VERSION-RELEASE.ARCH.rpm, as packageRPM names them.
func rpmFile(name string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `-([^-]+)-([^-]+)\.(` + linuxArchNames("rpm") + `|noarch)\.rpm$`)
}

// buildRPM makes site/rpm a repository that dnf and zypper install and
// update from: the new rpms beside those there already, the newest keep
// of each architecture, each signed by s, with repodata listing them and
// repomd.xml signed by s - all in Go. It returns the newest rpm of each
// architecture.
func buildRPM(site string, newRPMs []string, keep int, url string, a *App, s *signer) (map[string]Download, error) {
	root := filepath.Join(site, "rpm")
	dir := filepath.Join(root, "packages")
	match := rpmFile(a.Name)
	for _, rpm := range newRPMs {
		if !match.MatchString(filepath.Base(rpm)) {
			continue
		}
		dest := filepath.Join(dir, filepath.Base(rpm))
		if err := copyFile(rpm, dest); err != nil {
			return nil, err
		}
		if err := signRPM(dest, s); err != nil {
			return nil, err
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	byArch := map[string][]string{}
	version := map[string]string{}
	for _, e := range entries {
		if m := match.FindStringSubmatch(e.Name()); m != nil {
			byArch[m[3]] = append(byArch[m[3]], e.Name())
			version[e.Name()] = m[1]
		}
	}
	if len(byArch) == 0 {
		return nil, nil
	}
	newest := map[string]Download{}
	for arch, files := range byArch {
		sort.Slice(files, func(i, j int) bool { return compareVersions(version[files[i]], version[files[j]]) > 0 })
		for i, f := range files {
			if keep > 0 && i >= keep {
				if err := os.Remove(filepath.Join(dir, f)); err != nil {
					return nil, err
				}
			}
		}
		newest[arch] = Download{Version: strings.ReplaceAll(version[files[0]], "~", "-"), URL: "rpm/packages/" + files[0]}
	}
	if err := writeRepodata(root); err != nil {
		return nil, fmt.Errorf("making the rpm repository: %w", err)
	}
	repomd, err := os.ReadFile(filepath.Join(root, "repodata", "repomd.xml"))
	if err != nil {
		return nil, err
	}
	sig, err := s.detach(repomd)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(root, "repodata", "repomd.xml.asc"), sig, 0o644); err != nil {
		return nil, err
	}
	return newest, os.WriteFile(filepath.Join(root, a.Name+".repo"), repoFile(a, url), 0o644)
}

// repoFile is the .repo file that adds the repository to dnf or zypper,
// checking both the packages' signatures and the index's. dnf looks for a
// new index every six hours, not its usual 48, so that a release reaches
// people the same day.
func repoFile(a *App, url string) []byte {
	return []byte(fmt.Sprintf(`[%s]
name=%s
baseurl=%s/rpm
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=%s/%s
metadata_expire=6h
`, a.Name, a.DisplayName, strings.TrimRight(url, "/"), strings.TrimRight(url, "/"), publicFile))
}
