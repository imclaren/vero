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
	return regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `-(.+)-(x64|arm64)-setup\.exe$`)
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
	Windows []windowsDownload
}

type windowsDownload struct {
	Label, URL, Version string
}

// writeSite writes the site's pages: latest.json, index.html with how to
// install on each system, and install.sh, which does it on Debian and
// Ubuntu. urls in latest are relative to the site until here.
func writeSite(site, url string, a *App, latest Latest, s *signer) error {
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
	p := page{App: a, URL: url, Latest: latest}
	for k := range latest.Downloads {
		switch {
		case strings.HasPrefix(k, "linux-"):
			p.Apt = true
		case strings.HasPrefix(k, "rpm-"):
			p.RPM = true
		case strings.HasPrefix(k, "flatpak-"):
			p.Flatpak = true
		}
	}
	if _, err := os.Stat(filepath.Join(site, "aur", "PKGBUILD")); err == nil {
		p.AUR = true
	}
	for _, arch := range []string{"x64", "arm64"} {
		if d, ok := latest.Downloads["windows-"+arch]; ok {
			label := map[string]string{"x64": "Most PCs (x64)", "arm64": "ARM PCs (ARM64)"}[arch]
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
	if !p.Apt && !p.RPM && !p.Flatpak {
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

func aurCommands(p page) []string {
	return []string{
		fmt.Sprintf("curl -fsSLO %s/aur/PKGBUILD", p.URL),
		"makepkg -si",
	}
}

var funcs = template.FuncMap{"apt": aptCommands, "dnf": dnfCommands, "zypper": zypperCommands, "flatpak": flatpakCommands, "aur": aurCommands}

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
</style>
</head>
<body>
<main>
<h1>{{.App.DisplayName}}</h1>
<p>{{.App.Summary}}</p>
<p class="soft">Version {{.Latest.Version}}{{if .App.Homepage}} · <a href="{{.App.Homepage}}">{{.App.Homepage}}</a>{{end}}</p>
<div id="systems">
{{- if .Apt}}
<section data-system="linux">
<h2>Debian and Ubuntu</h2>
<p>Add {{.App.DisplayName}}'s repository and install it. After that, your usual updates keep it up to date.</p>
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
{{- if .AUR}}
<section data-system="linux">
<h2>Arch Linux</h2>
<p>Download the package's recipe and build it with makepkg:</p>
<pre>{{range aur .}}{{.}}
{{end}}</pre>
<p class="soft">To update it, do the same again.</p>
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
  var ua = navigator.userAgent, system = /Windows/.test(ua) ? "windows" : /Linux|X11/.test(ua) && !/Android/.test(ua) ? "linux" : "";
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
}).Parse(`#!/bin/sh
# Installs {{.App.DisplayName}} from {{.URL}}: from its repository on Debian,
# Ubuntu{{if .RPM}}, Fedora and openSUSE{{end}}, signed by the key at {{.URL}}/key.asc,
# so that your usual updates keep it up to date{{if .Flatpak}}; elsewhere, with Flatpak{{end}}.
set -e
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
