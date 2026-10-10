package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	textTemplate "text/template"
)

// Download is the newest of one kind of installer.
type Download struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// Latest is latest.json: the newest version, and where each installer is,
// for an app's own check for updates.
type Latest struct {
	Name      string              `json:"name"`
	Version   string              `json:"version"`
	Downloads map[string]Download `json:"downloads"`
}

// windowsInstaller is NAME-VERSION-ARCH-setup.exe, as package-windows.sh
// names them.
func windowsInstaller(name string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `-(.+)-(x64|arm64|x86)-setup\.exe$`)
}

// buildWindows puts the new Windows installers in site/windows, keeps the
// newest keep of each architecture, and returns the newest of each.
func buildWindows(site string, newExes []string, keep int, a *App) (map[string]Download, error) {
	dir := filepath.Join(site, "windows")
	match := windowsInstaller(a.Name)
	for _, exe := range newExes {
		if match.MatchString(filepath.Base(exe)) {
			if err := copyFile(exe, filepath.Join(dir, filepath.Base(exe))); err != nil {
				return nil, err
			}
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	byArch := map[string][]string{} // file names, by architecture
	version := map[string]string{}
	for _, e := range entries {
		if m := match.FindStringSubmatch(e.Name()); m != nil {
			byArch[m[2]] = append(byArch[m[2]], e.Name())
			version[e.Name()] = m[1]
		}
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
		newest[arch] = Download{Version: version[files[0]], URL: "windows/" + files[0]}
	}
	return newest, nil
}

// page is what index.html and install.sh are made from.
type page struct {
	App     *App
	URL     string
	Latest  Latest
	Apt     bool
	RPM     bool
	Flatpak bool
	AUR     bool
	// Pacman is whether the site has a pacman repository, and
	// Fingerprint the signing key's, which pacman-key trusts.
	Pacman      bool
	Fingerprint string
	// VoidKey is the key's fingerprint as xbps names it.
	VoidKey string
	Windows []windowsDownload
	// Mac is the newest disk image, if the site has one.
	Mac *windowsDownload
	// Others are the bundles for the web, Android, iOS, WASI and Plan 9.
	Others []otherSection
	// Unix are the BSDs and illumos the site has a repository for.
	Unix []unixSection
	// Linux are Alpine, Void and Chimera, if the site has their repositories,
	// which are added as root.
	Linux []unixSection
	// Recordings are GIFs of the app at work, from vero's tests.
	Recordings []recording
}

// unixSection is how to install on one of the BSDs or illumos.
type unixSection struct {
	System   string // vero's name, as in latest.json
	Label    string
	Commands []string
	Update   string
	Note     string
}

type windowsDownload struct {
	Label, URL, Version string
}

// writeSite writes the site's pages: latest.json, index.html with how to
// install on each system, and install.sh, which does it on every Unix the
// site has a repository for. urls in latest are relative to the site
// until here.
func writeSite(site, url string, a *App, latest Latest, s *signer, noPage bool, recordings []recording) error {
	url = strings.TrimRight(url, "/")
	for k, d := range latest.Downloads {
		d.URL = url + "/" + d.URL
		latest.Downloads[k] = d
	}
	data, err := json.MarshalIndent(latest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(site, "latest.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(site, publicFile), s.public, 0o644); err != nil {
		return err
	}
	if noPage {
		os.Remove(filepath.Join(site, "index.html"))
		os.Remove(filepath.Join(site, "install.sh"))
		return nil
	}
	p := page{App: a, URL: url, Latest: latest, Fingerprint: s.keyFingerprint(), Recordings: recordings}
	if k, err := s.rsaKey(); err == nil {
		p.VoidKey = voidFingerprint(&k.PublicKey)
	}
	for k := range latest.Downloads {
		switch {
		case strings.HasPrefix(k, "linux-"):
			p.Apt = true
		case strings.HasPrefix(k, "rpm-"):
			p.RPM = true
		case strings.HasPrefix(k, "flatpak-"):
			p.Flatpak = true
		case strings.HasPrefix(k, "arch-"):
			p.Pacman = true
		}
	}
	if _, err := os.Stat(filepath.Join(site, "aur", "PKGBUILD")); err == nil {
		p.AUR = true
	}
	for _, sys := range unixSystems {
		for k := range latest.Downloads {
			if strings.HasPrefix(k, sys.name+"-") {
				p.Unix = append(p.Unix, unixInstructions(p, sys, ""))
				break
			}
		}
	}
	for _, sys := range []string{"alpine", "void", "chimera"} {
		for k := range latest.Downloads {
			if strings.HasPrefix(k, sys+"-") {
				p.Linux = append(p.Linux, linuxInstructions(p, sys, ""))
				break
			}
		}
	}
	if d, ok := latest.Downloads["macos-universal"]; ok {
		p.Mac = &windowsDownload{"Mac (Apple silicon and Intel)", d.URL, d.Version}
	}
	p.Others = otherSections(p)
	for _, arch := range windowsArches {
		if d, ok := latest.Downloads["windows-"+arch]; ok {
			label := map[string]string{"x64": "Most PCs (x64)", "arm64": "ARM PCs (ARM64)", "x86": "Older PCs with 32-bit Windows (x86)"}[arch]
			p.Windows = append(p.Windows, windowsDownload{label, d.URL, d.Version})
		}
	}
	var html bytes.Buffer
	if err := indexPage.Execute(&html, p); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(site, "index.html"), html.Bytes(), 0o644); err != nil {
		return err
	}
	if !p.Apt && !p.RPM && !p.Pacman && !p.Flatpak && len(p.Unix) == 0 && len(p.Linux) == 0 {
		return nil
	}
	var sh bytes.Buffer
	if err := installScript.Execute(&sh, p); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(site, "install.sh"), sh.Bytes(), 0o755)
}

// aptCommands are the commands that add the repository and install the
// app, as index.html shows them. install.sh runs them with -y, since
// piped into sh it has nobody to ask.
func aptCommands(p page) []string { return aptSteps(p, "") }

func scriptCommands(p page) []string { return aptSteps(p, " -y") }

func aptSteps(p page, yes string) []string {
	key := "/etc/apt/keyrings/" + p.App.Name + ".asc"
	return []string{
		"sudo install -d -m 0755 /etc/apt/keyrings",
		fmt.Sprintf("curl -fsSL %s/%s | sudo tee %s > /dev/null", p.URL, publicFile, key),
		fmt.Sprintf(`echo "deb [signed-by=%s] %s/apt stable main" | sudo tee /etc/apt/sources.list.d/%s.list > /dev/null`, key, p.URL, p.App.Name),
		"sudo apt update",
		"sudo apt install" + yes + " " + p.App.Name,
	}
}

// dnfCommands, zypperCommands, flatpakCommands and aurCommands are the
// commands for Fedora, openSUSE, any Linux with Flatpak, and Arch.
func dnfCommands(p page) []string { return dnfSteps(p, "") }

func dnfSteps(p page, yes string) []string {
	return []string{
		fmt.Sprintf("sudo curl -fsSL %s/rpm/%s.repo -o /etc/yum.repos.d/%s.repo", p.URL, p.App.Name, p.App.Name),
		"sudo dnf install" + yes + " " + p.App.Name,
	}
}

func zypperCommands(p page) []string { return zypperSteps(p, "") }

func zypperSteps(p page, yes string) []string {
	if yes != "" {
		return []string{
			fmt.Sprintf("sudo zypper --non-interactive addrepo %s/rpm/%s.repo", p.URL, p.App.Name),
			"sudo zypper --non-interactive --gpg-auto-import-keys install " + p.App.Name,
		}
	}
	return []string{
		fmt.Sprintf("sudo zypper addrepo %s/rpm/%s.repo", p.URL, p.App.Name),
		"sudo zypper install " + p.App.Name,
	}
}

func flatpakCommands(p page) []string { return flatpakSteps(p, "") }

func flatpakSteps(p page, yes string) []string {
	return []string{fmt.Sprintf("flatpak install%s %s/flatpak/%s.flatpakref", yes, p.URL, p.App.Name)}
}

// pacmanCommands trust the signing key, add the repository to
// pacman.conf, once, and install the app. pacman fills in $arch.
func pacmanCommands(p page) []string { return pacmanSteps(p, "") }

func pacmanSteps(p page, yes string) []string {
	n := p.App.Name
	return []string{
		fmt.Sprintf("curl -fsSL %s/%s | sudo pacman-key --add -", p.URL, publicFile),
		"sudo pacman-key --lsign-key " + p.Fingerprint,
		fmt.Sprintf(`grep -qs '^\[%s\]' /etc/pacman.conf || printf '\n[%s]\nServer = %s/arch/$arch\n' | sudo tee -a /etc/pacman.conf > /dev/null`, n, n, p.URL),
		"sudo pacman -Syu" + yes + " " + n,
	}
}

func aurCommands(p page) []string {
	return []string{
		fmt.Sprintf("curl -fsSLO %s/aur/PKGBUILD", p.URL),
		"makepkg -si",
	}
}

// unixInstructions are the commands, run as root, that add the app's
// repository on sys and install it, and the one that updates it. yes
// makes them ask nothing, for install.sh.
func unixInstructions(p page, sys unixSystem, yes string) unixSection {
	n, u := p.App.Name, p.URL
	s := unixSection{System: sys.name, Label: sys.label}
	switch sys.format {
	case "pkg":
		y, update := "", "pkg update"
		if yes != "" {
			// A new system has only pkg's bootstrap, which asks first.
			y, update = " -y", "ASSUME_ALWAYS_YES=yes pkg update"
		}
		s.Commands = []string{
			"mkdir -p /usr/local/etc/pkg/repos /usr/local/etc/pkg/keys",
			fmt.Sprintf("fetch -o /usr/local/etc/pkg/keys/%s.pem %s/%s/key.pem", n, u, sys.name),
			fmt.Sprintf("fetch -o /usr/local/etc/pkg/repos/%s.conf %s/%s/%s.conf", n, u, sys.name, n),
			update,
			"pkg install" + y + " " + n,
		}
		s.Update = "pkg upgrade"
	case "pkgsrc":
		prefix := sys.prefix
		y := ""
		if yes != "" {
			y = " -y"
		}
		repo := fmt.Sprintf("%s/%s/$arch/All", u, sys.name)
		conf := prefix + "/etc/pkgin/repositories.conf"
		s.Commands = []string{
			fmt.Sprintf("grep -qs '%s' %s || echo '%s' >> %s", repo, conf, repo, conf),
			// -f: pkgin otherwise skips a repository it knew before.
			"pkgin" + y + " -f update",
			"pkgin" + y + " install " + n,
		}
		// -f again: pkgin can't add a newer summary from one repository
		// while another is unchanged; it numbers their packages alike.
		s.Update = "pkgin -f upgrade"
		if sys.name == "illumos" {
			// pkgsrc there installs only packages signed by a key in its
			// keyring: this one is added to it, once.
			keyring := prefix + "/etc/gnupg/pkgsrc.gpg"
			s.Commands = append([]string{
				fmt.Sprintf("grep -qs '%s' %s.added || { curl -fsSL %s/%s/key.gpg >> %s && echo '%s' >> %s.added; }", u, keyring, u, sys.name, keyring, u, keyring),
			}, s.Commands...)
		}
		if sys.name == "netbsd" {
			s.Note = "These need pkgin, which NetBSD's installer offers; pkg_add pkgin adds it if it's missing."
		} else {
			s.Note = "These need pkgsrc, in " + prefix + ". SmartOS has it; for OmniOS and OpenIndiana, pkgsrc.smartos.org says how to add it."
		}
	case "openbsd":
		path := fmt.Sprintf("PKG_PATH=%s/%s/%%a/:installpath", u, sys.name)
		y := ""
		if yes != "" {
			y = " -I"
		}
		s.Commands = []string{
			fmt.Sprintf("ftp -o /etc/signify/%s-pkg.pub %s/%s/%s-pkg.pub", n, u, sys.name, n),
			fmt.Sprintf("%s pkg_add%s %s", path, y, n),
		}
		s.Update = fmt.Sprintf("%s pkg_add -u %s", path, n)
	}
	return s
}

// linuxInstructions are the commands, run as root, that add the app's
// repository on Alpine, Void or Chimera and install it, and the one that updates
// it. yes makes them ask nothing, for install.sh.
func linuxInstructions(p page, sys, yes string) unixSection {
	n, u := p.App.Name, p.URL
	switch sys {
	case "alpine":
		repo := u + "/alpine"
		return unixSection{System: "alpine", Label: "Alpine Linux", Commands: []string{
			fmt.Sprintf("wget -qO /etc/apk/keys/%s %s/alpine/%s", alpineKeyName(p.App), u, alpineKeyName(p.App)),
			fmt.Sprintf("grep -qx '%s' /etc/apk/repositories || echo '%s' >> /etc/apk/repositories", repo, repo),
			"apk add -U " + n,
		}, Update: "apk upgrade -U", Note: "GTK 4 is in Alpine's community repository, which needs to be on too."}
	case "chimera":
		// Chimera has FreeBSD's fetch, rather than curl or wget, on all but
		// a minimal install. Its apk asks before installing much, and from
		// install.sh would read the answer from the rest of the script.
		return unixSection{System: "chimera", Label: "Chimera Linux", Commands: []string{
			fmt.Sprintf("fetch -qo /etc/apk/keys/%s %s/chimera/%s", alpineKeyName(p.App), u, alpineKeyName(p.App)),
			fmt.Sprintf("mkdir -p /etc/apk/repositories.d && echo '%s/chimera' > /etc/apk/repositories.d/%s.list", u, n),
			"apk add -U" + map[string]string{"": "", "yes": " --interactive=no"}[yes] + " " + n,
		}, Update: "apk upgrade -U", Note: "On a minimal install, apk add chimerautils-extra adds fetch first."}
	case "void":
		y := ""
		if yes != "" {
			y = "y"
		}
		s := unixSection{System: "void", Label: "Void Linux", Commands: []string{
			fmt.Sprintf("xbps-fetch -o /var/db/xbps/keys/%s.plist %s/void/%s.plist", p.VoidKey, u, p.VoidKey),
			fmt.Sprintf("echo 'repository=%s/void' > /etc/xbps.d/%s.conf", u, n),
			"xbps-install -S" + y + " " + n,
		}, Update: "xbps-install -Su"}
		return s
	}
	return unixSection{}
}

var funcs = template.FuncMap{"apt": aptCommands, "dnf": dnfCommands, "zypper": zypperCommands, "flatpak": flatpakCommands, "aur": aurCommands, "pacman": pacmanCommands}

var indexPage = template.Must(template.New("index").Funcs(funcs).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Install {{.App.DisplayName}}</title>
<style>
  :root { color-scheme: light dark; --ink: #1d1d1f; --soft: #6e6e73; --line: #d2d2d7; --bg: #fff; --code: #f5f5f7; }
  @media (prefers-color-scheme: dark) { :root { --ink: #f5f5f7; --soft: #a1a1a6; --line: #424245; --bg: #1d1d1f; --code: #2c2c2e; } }
  body { margin: 0; background: var(--bg); color: var(--ink); font: 16px/1.5 system-ui, -apple-system, "Segoe UI", sans-serif; }
  main { max-width: 720px; margin: 0 auto; padding: 32px 16px 64px; }
  h1 { margin: 0 0 4px; font-size: 28px; }
  h2 { margin: 32px 0 8px; font-size: 20px; }
  p { margin: 8px 0; }
  .soft { color: var(--soft); }
  .yours { font-size: 13px; font-weight: 600; color: var(--soft); margin-left: 8px; }
  pre { background: var(--code); padding: 12px 14px; border-radius: 8px; overflow-x: auto; font-size: 14px; }
  section { border-top: 1px solid var(--line); }
  a { color: inherit; }
  code { font-size: 0.9em; }
  .recordings { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 12px; }
  .recordings figure { margin: 0; }
  .recordings img { display: block; width: 100%; height: auto; border: 1px solid var(--line); border-radius: 6px; }
  .recordings figcaption { font-size: 13px; color: var(--soft); margin-top: 4px; }
</style>
</head>
<body>
<main>
<h1>{{.App.DisplayName}}</h1>
<p>{{.App.Summary}}</p>
<p class="soft">Version {{.Latest.Version}}{{if .App.Homepage}} · <a href="{{.App.Homepage}}">{{.App.Homepage}}</a>{{end}}</p>
{{- if .Recordings}}
<h2>See it at work</h2>
<p class="soft">Recorded installing from this site and running, on each of these systems.</p>
<div class="recordings">
{{- range .Recordings}}
<figure><a href="{{.File}}"><img src="{{.File}}" alt="{{$.App.DisplayName}} at work on {{.System}}" loading="lazy"></a><figcaption>{{.System}}</figcaption></figure>
{{- end}}
</div>
{{- end}}
<div id="systems">
{{- if .Apt}}
<section data-system="linux">
<h2>Debian, Ubuntu and Chromebooks</h2>
<p>Add {{.App.DisplayName}}'s repository and install it. After that, your usual updates keep it up to date.</p>
<p class="soft">On a Chromebook, turn on Linux first, in Settings → Developers, then run these in its Terminal.</p>
<pre>{{range apt .}}{{.}}
{{end}}</pre>
<p>Or run one command, which does the same:</p>
<pre>curl -fsSL {{.URL}}/install.sh | sh</pre>
<p class="soft">These need curl, which <code>sudo apt install curl</code> adds if it's missing.</p>
</section>
{{- end}}
{{- if .RPM}}
<section data-system="linux">
<h2>Fedora and openSUSE</h2>
<p>Add {{.App.DisplayName}}'s repository and install it. After that, your usual updates keep it up to date. The first time, you're asked to accept the key that signs it, which is at <a href="{{.URL}}/key.asc">{{.URL}}/key.asc</a>.</p>
<p>On Fedora:</p>
<pre>{{range dnf .}}{{.}}
{{end}}</pre>
<p>On openSUSE:</p>
<pre>{{range zypper .}}{{.}}
{{end}}</pre>
</section>
{{- end}}
{{- if .Flatpak}}
<section data-system="linux">
<h2>Any Linux, with Flatpak</h2>
<p><a href="{{.URL}}/flatpak/{{.App.Name}}.flatpakref">Install {{.App.DisplayName}} with Flatpak</a>, which opens it in your software centre, or run:</p>
<pre>{{range flatpak .}}{{.}}
{{end}}</pre>
<p class="soft">Flatpak keeps it up to date, with your other Flatpak apps. If your system doesn't have Flatpak yet, <a href="https://flatpak.org/setup/">flatpak.org/setup</a> says how to add it.</p>
</section>
{{- end}}
{{- if .Pacman}}
<section data-system="linux">
<h2>Arch Linux</h2>
<p>Trust the key that signs {{.App.DisplayName}}, add its repository and install it. After that, <code>sudo pacman -Syu</code> keeps it up to date.</p>
<pre>{{range pacman .}}{{.}}
{{end}}</pre>
{{- if .AUR}}
<p class="soft">Or build it yourself from <a href="{{.URL}}/aur/PKGBUILD">its recipe</a>, with <code>makepkg -si</code>.</p>
{{- end}}
</section>
{{- else if .AUR}}
<section data-system="linux">
<h2>Arch Linux</h2>
<p>Download the package's recipe and build it with makepkg:</p>
<pre>{{range aur .}}{{.}}
{{end}}</pre>
<p class="soft">To update it, do the same again.</p>
</section>
{{- end}}
{{- range .Linux}}
<section data-system="linux">
<h2>{{.Label}}</h2>
<p>As root, add {{$.App.DisplayName}}'s repository and install it:</p>
<pre>{{range .Commands}}{{.}}
{{end}}</pre>
<p>To update it later, with your other packages:</p>
<pre>{{.Update}}</pre>
{{- if .Note}}
<p class="soft">{{.Note}}</p>
{{- end}}
</section>
{{- end}}
{{- range .Unix}}
<section data-system="{{.System}}">
<h2>{{.Label}}</h2>
<p>As root, add {{$.App.DisplayName}}'s repository and install it:</p>
<pre>{{range .Commands}}{{.}}
{{end}}</pre>
<p>To update it later, with your other packages:</p>
<pre>{{.Update}}</pre>
{{- if .Note}}
<p class="soft">{{.Note}}</p>
{{- end}}
</section>
{{- end}}
{{- if .Mac}}
<section data-system="macos">
<h2>Mac</h2>
<p><a href="{{.Mac.URL}}">Download {{.App.DisplayName}}</a> <span class="soft">version {{.Mac.Version}}</span>, open it, and drag {{.App.DisplayName}} to Applications. It checks for updates itself.</p>
<p class="soft">If your Mac says it can't check the app, open System Settings, choose Privacy &amp; Security, and choose <b>Open Anyway</b>.</p>
</section>
{{- end}}
{{- range .Others}}
<section data-system="{{.System}}">
<h2>{{.Label}}</h2>
{{- range .Paragraphs}}
<p>{{.}}</p>
{{- end}}
{{- if .Commands}}
<pre>{{range .Commands}}{{.}}
{{end}}</pre>
{{- end}}
</section>
{{- end}}
{{- if .Windows}}
<section data-system="windows">
<h2>Windows</h2>
<p>Download the installer and open it. Windows may warn that it doesn't recognise the app; choose <b>More info</b>, then <b>Run anyway</b>.</p>
<ul>
{{- range .Windows}}
<li><a href="{{.URL}}">{{.Label}}</a> <span class="soft">version {{.Version}}</span></li>
{{- end}}
</ul>
</section>
{{- end}}
</div>
</main>
<script>
  // The section for this computer first.
  var ua = navigator.userAgent, system = /Windows/.test(ua) ? "windows" : /Android/.test(ua) ? "android" :
    /iPhone|iPad/.test(ua) ? "ios" : /Macintosh/.test(ua) ? "macos" : /FreeBSD/.test(ua) ? "freebsd" : /OpenBSD/.test(ua) ? "openbsd" :
    /NetBSD/.test(ua) ? "netbsd" : /DragonFly/.test(ua) ? "dragonfly" : /SunOS|illumos/.test(ua) ? "illumos" :
    /Linux|X11/.test(ua) && !/Android/.test(ua) ? "linux" : "";
  var mine = system ? document.querySelectorAll('[data-system="' + system + '"]') : [];
  var list = document.getElementById("systems");
  for (var i = mine.length - 1; i >= 0; i--) {
    list.insertBefore(mine[i], list.firstChild);
    mine[i].querySelector("h2").insertAdjacentHTML("beforeend", '<span class="yours">For this computer</span>');
  }
</script>
</body>
</html>
`))

var installScript = textTemplate.Must(textTemplate.New("install").Funcs(textTemplate.FuncMap{
	"apt":     scriptCommands,
	"dnf":     func(p page) []string { return dnfSteps(p, " -y") },
	"zypper":  func(p page) []string { return zypperSteps(p, " -y") },
	"flatpak": func(p page) []string { return flatpakSteps(p, " -y") },
	"pacman":  func(p page) []string { return pacmanSteps(p, " --noconfirm") },
	"linux": func(p page) []unixSection {
		var out []unixSection
		for _, s := range p.Linux {
			out = append(out, linuxInstructions(p, s.System, "yes"))
		}
		return out
	},
	"unix": func(p page) []unixSection {
		var out []unixSection
		for _, s := range p.Unix {
			out = append(out, unixInstructions(p, *system(s.System), "yes"))
		}
		return out
	},
}).Parse(`#!/bin/sh
# Installs {{.App.DisplayName}} from {{.URL}}, from its repository for this system,
# so that your usual updates keep it up to date. {{.URL}} says which
# systems it has, and how to do the same by hand.
set -e
{{- range unix .}}
if [ "$(uname -s)" = {{if eq .System "freebsd"}}FreeBSD{{else if eq .System "dragonfly"}}DragonFly{{else if eq .System "netbsd"}}NetBSD{{else if eq .System "openbsd"}}OpenBSD{{else}}SunOS{{end}} ]; then
    [ "$(id -u)" = 0 ] || { echo "Run this as root." >&2; exit 1; }
{{- range .Commands}}
    {{.}}
{{- end}}
    echo "{{$.App.DisplayName}} is installed. To update it later: {{.Update}}"
    exit 0
fi
{{- end}}
{{- range linux .}}
if grep -qs '^ID="\{0,1\}{{.System}}' /etc/os-release; then
    [ "$(id -u)" = 0 ] || { echo "Run this as root." >&2; exit 1; }
{{- range .Commands}}
    {{.}}
{{- end}}
    echo "{{$.App.DisplayName}} is installed. To update it later: {{.Update}}"
    exit 0
fi
{{- end}}
command -v curl > /dev/null || { echo "This needs curl. Install it, then run this again." >&2; exit 1; }
# Run as root, as in a container, there's no sudo, and no need for it.
[ "$(id -u)" = 0 ] && sudo() { "$@"; }
os() { grep -qs -e "^ID=$1" -e "^ID_LIKE=.*$1" /etc/os-release; }
{{- if .Apt}}
if os debian || os ubuntu; then
{{- range apt .}}
    {{.}}
{{- end}}
    echo "{{.App.DisplayName}} is installed."
    exit 0
fi
{{- end}}
{{- if .RPM}}
if command -v dnf > /dev/null; then
{{- range dnf .}}
    {{.}}
{{- end}}
    echo "{{.App.DisplayName}} is installed."
    exit 0
fi
if command -v zypper > /dev/null; then
{{- range zypper .}}
    {{.}}
{{- end}}
    echo "{{.App.DisplayName}} is installed."
    exit 0
fi
{{- end}}
{{- if .Pacman}}
if command -v pacman > /dev/null; then
{{- range pacman .}}
    {{.}}
{{- end}}
    echo "{{.App.DisplayName}} is installed."
    exit 0
fi
{{- end}}
{{- if .Flatpak}}
if command -v flatpak > /dev/null; then
{{- range flatpak .}}
    {{.}}
{{- end}}
    echo "{{.App.DisplayName}} is installed."
    exit 0
fi
{{- end}}
echo "This script doesn't know how to install {{.App.DisplayName}} here. See {{.URL}}" >&2
exit 1
`))
