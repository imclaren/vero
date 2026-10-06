package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

// Flatpaks come from vero as a branch called stable, and their runtime
// from Flathub.
const (
	flatpakBranch = "stable"
	flathubRepo   = "https://dl.flathub.org/repo/flathub.flatpakrepo"
)

// packageFlatpak builds the app as a Flatpak bundle for each architecture
// - one file, NAME-VERSION-ARCH.flatpak - from the same files as the
// .deb, under /app. The bundles are made in vero's tools container from
// a build folder written here, since the app is built already: no SDK is
// needed, only flatpak itself. vero-repo build puts them in the site's
// Flatpak repository.
func packageFlatpak(a *App, workers map[string]string, out string, t *tools) error {
	home, _ := os.UserHomeDir()
	if err := os.MkdirAll(filepath.Join(home, ".cache"), 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Join(home, ".cache"), "vero-flatpak.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	outDir, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	script := ""
	var built []string
	for _, arch := range linuxArches {
		build := filepath.Join(stage, "build-"+arch.name)
		if err := flatpakBuildFolder(a, workers[arch.goarch], arch.name, build); err != nil {
			return err
		}
		bundle := filepath.Join(outDir, fmt.Sprintf("%s-%s-%s.flatpak", a.Name, a.Version, arch.name))
		script += fmt.Sprintf("flatpak build-export --arch=%s %s %s %s >/dev/null\n",
			arch.name, shellQuote(filepath.Join(stage, "repo")), shellQuote(build), flatpakBranch)
		script += fmt.Sprintf("flatpak build-bundle --arch=%s --runtime-repo=%s %s %s %s %s\n",
			arch.name, flathubRepo, shellQuote(filepath.Join(stage, "repo")), shellQuote(bundle), a.ID, flatpakBranch)
		built = append(built, bundle)
	}
	if err := t.runFlatpak(script, stage, outDir); err != nil {
		return fmt.Errorf("making the Flatpaks: %w", err)
	}
	for _, b := range built {
		fmt.Println("built", b)
	}
	return nil
}

// flatpakBuildFolder writes the folder flatpak build-export takes: the
// app in files/, what the desktop sees of it in export/, and metadata,
// which says its runtime, its command, and what it may reach.
func flatpakBuildFolder(a *App, worker, arch, dir string) error {
	command := fmt.Sprintf("/usr/bin/flatpak run --branch=%s --arch=%s --command=%s %s", flatpakBranch, arch, a.Name, a.ID)
	files, err := linuxTree(a, "/app", worker, a.Name)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := writeTree(filepath.Join(dir, "files", strings.TrimPrefix(f.path, "/app/")), f.data, f.mode); err != nil {
			return err
		}
		// The desktop sees the menu entry, running it with flatpak, and
		// the icons.
		rel := strings.TrimPrefix(f.path, "/app/")
		switch {
		case strings.HasPrefix(rel, "share/applications/"):
			entry := strings.Replace(string(desktopEntry(a, command)), "StartupNotify=true\n", "StartupNotify=true\nX-Flatpak="+a.ID+"\n", 1)
			if err := writeTree(filepath.Join(dir, "export", rel), []byte(entry), 0o644); err != nil {
				return err
			}
		case strings.HasPrefix(rel, "share/icons/"):
			if err := writeTree(filepath.Join(dir, "export", rel), f.data, 0o644); err != nil {
				return err
			}
		}
	}
	// What software centres show of the app before it's installed: the
	// repository's appstream branch is made from this, in the app's files.
	icon, err := iconSizes(a.Path(a.Icon), 128)
	if err != nil {
		return err
	}
	if err := writeTree(filepath.Join(dir, "files", "share", "app-info", "icons", "flatpak", "128x128", a.ID+".png"), icon[128], 0o644); err != nil {
		return err
	}
	collection, err := appStreamCollection(a, arch)
	if err != nil {
		return err
	}
	if err := writeTree(filepath.Join(dir, "files", "share", "app-info", "xmls", a.ID+".xml.gz"), collection, 0o644); err != nil {
		return err
	}
	return writeTree(filepath.Join(dir, "metadata"), flatpakMetadata(a, arch), 0o644)
}

// flatpakMetadata is the Flatpak's metadata: its runtime, its command,
// and the permissions its needs ask for. A file chosen in a GTK file
// dialog comes through a portal and needs no permission; files gives the
// home folder, for an app that keeps what it works on in a folder of the
// person's choosing. Notifications, and opening at sign-in, go through
// portals too, which need nothing here.
func flatpakMetadata(a *App, arch string) []byte {
	runtime, version := a.GTK.runtime()
	sdk := strings.Replace(runtime, ".Platform", ".Sdk", 1)
	shared := []string{"ipc"}
	if a.Needs.Network {
		shared = append(shared, "network")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[Application]\nname=%s\nruntime=%s/%s/%s\nsdk=%s/%s/%s\ncommand=%s\n\n",
		a.ID, runtime, arch, version, sdk, arch, version, a.Name)
	fmt.Fprintf(&b, "[Context]\nshared=%s;\nsockets=wayland;fallback-x11;\ndevices=dri;\n", strings.Join(shared, ";"))
	if a.Needs.Files {
		b.WriteString("filesystems=home;\n")
	}
	if a.Needs.Keyring {
		b.WriteString("\n[Session Bus Policy]\norg.freedesktop.secrets=talk\n")
	}
	return []byte(b.String())
}

// appStreamCollection is the app's AppStream metadata as a repository
// carries it: in a collection, with its cached icon and its Flatpak ref.
func appStreamCollection(a *App, arch string) ([]byte, error) {
	runtime, version := a.GTK.runtime()
	component := string(appStream(a))
	component = strings.TrimPrefix(component, strings.TrimSpace(xmlHeader))
	extra := fmt.Sprintf("  <icon type=\"cached\" width=\"128\" height=\"128\">%s.png</icon>\n  <bundle type=\"flatpak\" runtime=\"%s/%s/%s\">app/%s/%s/%s</bundle>\n</component>",
		a.ID, runtime, arch, version, a.ID, arch, flatpakBranch)
	component = strings.Replace(component, "</component>", extra, 1)
	doc := xmlHeader + "<components version=\"0.8\" origin=\"flatpak\">" + strings.TrimSpace(component) + "\n</components>\n"
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	z.Write([]byte(doc))
	if err := z.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

const xmlHeader = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"

func writeTree(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}

// flatpakBundle is NAME-VERSION-ARCH.flatpak, as packageFlatpak names them.
func flatpakBundle(name string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `-(.+)-(x86_64|aarch64)\.flatpak$`)
}

// buildFlatpak puts new Flatpak bundles in site/flatpak/repo, an OSTree
// repository signed by s, keeping keep versions of each architecture, and
// writes the .flatpakref that installs the app from it in one click and
// the .flatpakrepo that adds the repository. The newest bundles stay
// beside it too, as single files to download. It returns the newest
// version of each architecture.
func buildFlatpak(site string, newBundles []string, keep int, url string, a *App, s *signer, t *tools) (map[string]Download, error) {
	root := filepath.Join(site, "flatpak")
	repo := filepath.Join(root, "repo")
	match := flatpakBundle(a.Name)
	type bundle struct{ path, version, arch string }
	var bundles []bundle
	for _, b := range newBundles {
		if m := match.FindStringSubmatch(filepath.Base(b)); m != nil {
			bundles = append(bundles, bundle{b, m[1], m[2]})
		}
	}
	_, err := os.Stat(repo)
	if len(bundles) == 0 && err != nil {
		return nil, nil
	}
	// Oldest first, so that each architecture's newest is its last commit.
	sort.Slice(bundles, func(i, j int) bool { return compareVersions(bundles[i].version, bundles[j].version) < 0 })
	in := filepath.Join(root, "incoming")
	script := t.importKey
	if err != nil {
		script += fmt.Sprintf("ostree init --mode=archive-z2 --repo=%s\n", shellQuote(repo))
	}
	for _, b := range bundles {
		dest := filepath.Join(in, filepath.Base(b.path))
		if err := copyFile(b.path, dest); err != nil {
			return nil, err
		}
		script += fmt.Sprintf("flatpak build-import-bundle --gpg-sign=%s %s %s >/dev/null\n", s.fingerprint(), shellQuote(repo), shellQuote(dest))
	}
	prune := ""
	if keep > 0 {
		prune = fmt.Sprintf("--prune --prune-depth=%d ", keep-1)
	}
	script += fmt.Sprintf("flatpak build-update-repo --gpg-sign=%s %s--title=%s --default-branch=%s %s >/dev/null\n",
		s.fingerprint(), prune, shellQuote(a.DisplayName), flatpakBranch, shellQuote(repo))
	if err := t.runFlatpak(script, root); err != nil {
		return nil, fmt.Errorf("making the Flatpak repository: %w", err)
	}
	// The newest bundle of each architecture, to download; the rest gone.
	newest := map[string]Download{}
	for _, b := range bundles {
		newest[b.arch] = Download{Version: b.version, URL: "flatpak/" + filepath.Base(b.path)}
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		m := match.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		if d, ok := newest[m[2]]; ok && d.URL != "flatpak/"+e.Name() {
			os.Remove(filepath.Join(root, e.Name()))
		} else if !ok {
			newest[m[2]] = Download{Version: m[1], URL: "flatpak/" + e.Name()}
		}
	}
	for _, b := range bundles {
		if newest[b.arch].URL == "flatpak/"+filepath.Base(b.path) {
			if err := os.Rename(filepath.Join(in, filepath.Base(b.path)), filepath.Join(root, filepath.Base(b.path))); err != nil {
				return nil, err
			}
		}
	}
	os.RemoveAll(in)
	key, err := gpgKey(s.public)
	if err != nil {
		return nil, err
	}
	url = strings.TrimRight(url, "/")
	ref := fmt.Sprintf(`[Flatpak Ref]
Name=%s
Branch=%s
Title=%s
Url=%s/flatpak/repo/
RuntimeRepo=%s
IsRuntime=false
GPGKey=%s
`, a.ID, flatpakBranch, a.DisplayName, url, flathubRepo, key)
	repoFile := fmt.Sprintf(`[Flatpak Repo]
Title=%s
Url=%s/flatpak/repo/
Comment=%s
GPGKey=%s
`, a.DisplayName, url, a.Summary, key)
	if a.Homepage != "" {
		ref += "Homepage=" + a.Homepage + "\n"
		repoFile += "Homepage=" + a.Homepage + "\n"
	}
	if err := os.WriteFile(filepath.Join(root, a.Name+".flatpakref"), []byte(ref), 0o644); err != nil {
		return nil, err
	}
	return newest, os.WriteFile(filepath.Join(root, a.Name+".flatpakrepo"), []byte(repoFile), 0o644)
}

// gpgKey is an armored public key as Flatpak's files carry it: the key
// itself, in base64 on one line.
func gpgKey(armored []byte) (string, error) {
	block, err := armor.Decode(bytes.NewReader(armored))
	if err != nil {
		return "", err
	}
	var raw bytes.Buffer
	if _, err := raw.ReadFrom(block.Body); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw.Bytes()), nil
}
