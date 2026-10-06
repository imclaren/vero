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
		if strings.HasPrefix(k, "linux-") {
			p.Apt = true
		}
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
	if !p.Apt {
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

var funcs = template.FuncMap{"apt": aptCommands}

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
  var mine = system && document.querySelector('[data-system="' + system + '"]');
  if (mine) {
    var list = document.getElementById("systems");
    list.insertBefore(mine, list.firstChild);
    mine.querySelector("h2").insertAdjacentHTML("beforeend", '<span class="yours">For this computer</span>');
  }
</script>
</body>
</html>
`))

var installScript = textTemplate.Must(textTemplate.New("install").Funcs(textTemplate.FuncMap{"apt": scriptCommands}).Parse(`#!/bin/sh
# Installs {{.App.DisplayName}} on Debian or Ubuntu from {{.URL}}: adds its
# repository, signed by the key at {{.URL}}/key.asc, then installs it, so
# that your usual updates keep it up to date.
set -e
if ! grep -qs -e '^ID=debian' -e '^ID=ubuntu' -e '^ID_LIKE=.*debian' /etc/os-release; then
    echo "This script installs {{.App.DisplayName}} on Debian and Ubuntu. For other systems, see {{.URL}}" >&2
    exit 1
fi
command -v curl > /dev/null || { echo "This needs curl: sudo apt install curl" >&2; exit 1; }
# Run as root, as in a container, there's no sudo, and no need for it.
[ "$(id -u)" = 0 ] && sudo() { "$@"; }
{{range apt .}}{{.}}
{{end}}echo "{{.App.DisplayName}} is installed."
`))
