package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// wingetSchema is the version of winget's manifest format vero writes:
// one that winget, and microsoft/winget-pkgs, take.
const wingetSchema = "1.10.0"

// wingetID is the app's PackageIdentifier in winget: Publisher.App, from
// vero-app.toml's [wpf] winget_id, or made from the publisher's and the
// app's names.
func wingetID(a *App) string {
	if a.WPF != nil && a.WPF.WingetID != "" {
		return a.WPF.WingetID
	}
	clean := regexp.MustCompile(`[^A-Za-z0-9]+`)
	part := func(s string) string {
		var out strings.Builder
		for _, w := range clean.Split(s, -1) {
			if w != "" {
				out.WriteString(strings.ToUpper(w[:1]) + w[1:])
			}
		}
		return out.String()
	}
	return part(a.PublisherName()) + "." + part(a.DisplayName)
}

// writeWinget writes the three manifests winget installs the newest
// Windows installers from, in the folder layout microsoft/winget-pkgs
// keeps them in: manifests/p/Publisher/App/VERSION.
func writeWinget(site, url string, a *App, windows map[string]Download) error {
	if len(windows) == 0 {
		return nil
	}
	version := ""
	for _, d := range windows {
		if version == "" || compareVersions(d.Version, version) > 0 {
			version = d.Version
		}
	}
	id := wingetID(a)
	parts := strings.SplitN(id, ".", 2)
	root := filepath.Join(site, "winget")
	os.RemoveAll(root)
	dir := filepath.Join(root, "manifests", strings.ToLower(id[:1]), parts[0], parts[1], version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	url = strings.TrimRight(url, "/")
	header := func(kind string) string {
		return fmt.Sprintf("# yaml-language-server: $schema=https://aka.ms/winget-manifest.%s.%s.schema.json\n\n", kind, wingetSchema)
	}
	q := func(s string) string { return fmt.Sprintf("%q", s) }

	var installers strings.Builder
	for _, arch := range []string{"x64", "arm64"} {
		d, ok := windows[arch]
		if !ok || d.Version != version {
			continue
		}
		sum, err := fileSHA256(filepath.Join(site, filepath.FromSlash(d.URL)))
		if err != nil {
			return err
		}
		fmt.Fprintf(&installers, "- Architecture: %s\n  InstallerUrl: %s\n  InstallerSha256: %s\n", arch, q(url+"/"+d.URL), strings.ToUpper(sum))
	}
	installer := header("installer") + fmt.Sprintf(`PackageIdentifier: %s
PackageVersion: %s
InstallerType: nullsoft
Scope: user
InstallModes:
- interactive
- silent
InstallerSwitches:
  Silent: /S
  SilentWithProgress: /S
UpgradeBehavior: install
Installers:
%sManifestType: installer
ManifestVersion: %s
`, id, q(version), installers.String(), wingetSchema)

	licence := a.Licence
	if licence == "" {
		licence = "Proprietary"
	}
	locale := header("defaultLocale") + fmt.Sprintf(`PackageIdentifier: %s
PackageVersion: %s
PackageLocale: en-US
Publisher: %s
PackageName: %s
License: %s
ShortDescription: %s
`, id, q(version), q(a.PublisherName()), q(a.DisplayName), q(licence), q(a.Summary))
	if a.Homepage != "" {
		locale += "PackageUrl: " + q(a.Homepage) + "\n"
	}
	if desc := strings.Join(strings.Fields(a.Description), " "); desc != "" {
		locale += "Description: " + q(desc) + "\n"
	}
	locale += "ManifestType: defaultLocale\nManifestVersion: " + wingetSchema + "\n"

	ver := header("version") + fmt.Sprintf("PackageIdentifier: %s\nPackageVersion: %s\nDefaultLocale: en-US\nManifestType: version\nManifestVersion: %s\n",
		id, q(version), wingetSchema)

	for name, data := range map[string]string{
		id + ".installer.yaml":    installer,
		id + ".locale.en-US.yaml": locale,
		id + ".yaml":              ver,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			return err
		}
	}
	return nil
}
