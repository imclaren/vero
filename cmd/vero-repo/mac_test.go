package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestNestedCode: what's inside an app is signed deepest first, programs
// before the bundles around them, and frameworks after what they hold.
func TestNestedCode(t *testing.T) {
	app := filepath.Join(t.TempDir(), "Example.app")
	macho := []byte{0xcf, 0xfa, 0xed, 0xfe, 0, 0, 0, 0}
	files := []string{
		"Contents/MacOS/Example",
		"Contents/Resources/example-worker",
		"Contents/Frameworks/Kit.framework/Versions/B/Kit",
		"Contents/Frameworks/Kit.framework/Versions/B/Autoupdate",
		"Contents/Frameworks/Kit.framework/Versions/B/Updater.app/Contents/MacOS/Updater",
		"Contents/Frameworks/Kit.framework/Versions/B/XPCServices/Installer.xpc/Contents/MacOS/Installer",
	}
	for _, f := range files {
		os.MkdirAll(filepath.Join(app, filepath.Dir(f)), 0o755)
		os.WriteFile(filepath.Join(app, f), macho, 0o755)
	}
	os.WriteFile(filepath.Join(app, "Contents/Resources/licence.txt"), []byte("text"), 0o644)
	got, err := nestedCode(app)
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]int{}
	for i, p := range got {
		pos[strings.TrimPrefix(p, app+"/")] = i
	}
	if _, ok := pos["Contents/Resources/licence.txt"]; ok {
		t.Error("a text file is signed")
	}
	before := [][2]string{
		{"Contents/Frameworks/Kit.framework/Versions/B/Updater.app/Contents/MacOS/Updater", "Contents/Frameworks/Kit.framework/Versions/B/Updater.app"},
		{"Contents/Frameworks/Kit.framework/Versions/B/XPCServices/Installer.xpc", "Contents/Frameworks/Kit.framework"},
		{"Contents/Frameworks/Kit.framework/Versions/B/Updater.app", "Contents/Frameworks/Kit.framework"},
		{"Contents/Frameworks/Kit.framework/Versions/B/Autoupdate", "Contents/Frameworks/Kit.framework"},
	}
	for _, b := range before {
		i, ok1 := pos[b[0]]
		j, ok2 := pos[b[1]]
		if !ok1 || !ok2 || i > j {
			t.Errorf("%s isn't signed before %s: %v", b[0], b[1], got)
		}
	}
	if _, ok := pos["Contents/Resources/example-worker"]; !ok {
		t.Error("the worker isn't signed")
	}
}

// TestSignWithSparkle signs an app that embeds Sparkle.framework, ad hoc,
// and checks it as Gatekeeper would. It needs a Sparkle.framework, given
// by VERO_SPARKLE_FRAMEWORK; scripts/test-repo.sh --mac fetches one.
func TestSignWithSparkle(t *testing.T) {
	framework := os.Getenv("VERO_SPARKLE_FRAMEWORK")
	if runtime.GOOS != "darwin" || framework == "" {
		t.Skip("set VERO_SPARKLE_FRAMEWORK to a Sparkle.framework, on a Mac")
	}
	app := filepath.Join(t.TempDir(), "Example.app")
	for _, d := range []string{"MacOS", "Resources", "Frameworks"} {
		os.MkdirAll(filepath.Join(app, "Contents", d), 0o755)
	}
	if out, err := exec.Command("ditto", framework, filepath.Join(app, "Contents", "Frameworks", "Sparkle.framework")).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	// Any program will do for the app and the worker: the Mac's own true.
	for _, p := range []string{"Contents/MacOS/Example", "Contents/Resources/example-worker"} {
		if out, err := exec.Command("cp", "/usr/bin/true", filepath.Join(app, p)).CombinedOutput(); err != nil {
			t.Fatal(string(out))
		}
	}
	a := &App{Name: "example", DisplayName: "Example", ID: "com.example.app", Version: "1.0.0", MacOS: &MacOS{Product: "Example"}}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), infoPlist(a), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setVersions(app, "2.1.0", "57"); err != nil {
		t.Fatal(err)
	}
	if err := signApp(app, "", ""); err != nil {
		t.Fatal(err)
	}
	info, err := readInfo(app)
	if err != nil || info.ShortVersion != "2.1.0" || info.BundleVersion != "57" {
		t.Errorf("versions %+v %v", info, err)
	}
	// Each nested bundle has its own signature.
	for _, b := range []string{"Sparkle.framework/Versions/B/Updater.app", "Sparkle.framework/Versions/B/XPCServices/Installer.xpc", "Sparkle.framework"} {
		if out, err := exec.Command("codesign", "--verify", "--strict", filepath.Join(app, "Contents", "Frameworks", b)).CombinedOutput(); err != nil {
			t.Errorf("%s: %s", b, out)
		}
	}
}

// TestMacPkg: the installer package installs into /Applications, not over
// a copy elsewhere, and gives the app to the user who installed it.
func TestMacPkg(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("pkgbuild is the Mac's")
	}
	dir := t.TempDir()
	app := filepath.Join(dir, "Example.app")
	os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755)
	exec.Command("cp", "/usr/bin/true", filepath.Join(app, "Contents", "MacOS", "Example")).Run()
	a := &App{Name: "example", DisplayName: "Example", ID: "com.example.app", Version: "1.2.3", MacOS: &MacOS{Product: "Example"}}
	os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), infoPlist(a), 0o644)
	if err := signApp(app, "", ""); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(dir, "example-1.2.3-macos.pkg")
	if err := buildMacPkg(a, app, pkg, filepath.Join(dir, "work")); err != nil {
		t.Fatal(err)
	}
	expanded := filepath.Join(dir, "expanded")
	if out, err := exec.Command("pkgutil", "--expand", pkg, expanded).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	dist, _ := os.ReadFile(filepath.Join(expanded, "Distribution"))
	if !strings.Contains(string(dist), `CFBundleShortVersionString="1.2.3"`) || !strings.Contains(string(dist), "hostArchitectures") {
		t.Errorf("distribution:\n%s", dist)
	}
	info, _ := os.ReadFile(filepath.Join(expanded, "component.pkg", "PackageInfo"))
	if strings.Contains(string(info), "<relocate>") && !strings.Contains(string(info), "<relocate/>") {
		t.Errorf("relocatable:\n%s", info)
	}
	script := filepath.Join(expanded, "component.pkg", "Scripts", "postinstall")
	if out, err := exec.Command("sh", "-n", script).CombinedOutput(); err != nil {
		t.Errorf("postinstall: %s", out)
	}
	if body, _ := os.ReadFile(script); !strings.Contains(string(body), `chown -R "$WHO:$GROUP" "$APP"`) || !strings.Contains(string(body), "Example.app") {
		t.Errorf("postinstall:\n%s", body)
	}
}

// TestMacSite: build puts the .pkg and release notes on the site, links
// the notes from the appcast, writes the appcast where [macos] appcast
// says as well, and check finds a version that doesn't match.
func TestMacSite(t *testing.T) {
	a, _ := linuxApp(t)
	a.MacOS = &MacOS{Product: "Example", Appcast: "appcast.xml"}
	s := testKey(t)
	site, pkgs := t.TempDir(), t.TempDir()
	dmg := filepath.Join(pkgs, a.Name+"-1.0.1-macos.dmg")
	os.WriteFile(dmg, []byte("dmg"), 0o644)
	info, _ := json.Marshal(macInfo{BundleVersion: "41", ShortVersion: "1.0.1", Minimum: "13.0", App: "vero example.app"})
	os.WriteFile(dmg+".json", info, 0o644)
	pkg := filepath.Join(pkgs, a.Name+"-1.0.1-macos.pkg")
	os.WriteFile(pkg, []byte("pkg"), 0o644)
	notes := "Faster syncing.\n\n- One thing\n- Another <thing>\n"
	mac, err := buildMac(site, []string{dmg}, []string{pkg}, notes, 3, "https://example.com/app", a, s)
	if err != nil {
		t.Fatal(err)
	}
	if mac["pkg"].URL != "macos/"+a.Name+"-1.0.1-macos.pkg" || mac["universal"].Version != "1.0.1" {
		t.Errorf("downloads %+v", mac)
	}
	page, err := os.ReadFile(filepath.Join(site, "macos", a.Name+"-1.0.1-notes.html"))
	if err != nil || !strings.Contains(string(page), "<p>Faster syncing.</p>") || !strings.Contains(string(page), "<li>Another &lt;thing&gt;</li>") {
		t.Errorf("notes page %v:\n%s", err, page)
	}
	cast, _ := os.ReadFile(filepath.Join(site, "macos", "appcast.xml"))
	if !strings.Contains(string(cast), "<sparkle:releaseNotesLink>https://example.com/app/macos/"+a.Name+"-1.0.1-notes.html</sparkle:releaseNotesLink>") ||
		!strings.Contains(string(cast), "<sparkle:version>41</sparkle:version>") {
		t.Errorf("appcast:\n%s", cast)
	}
	if top, _ := os.ReadFile(filepath.Join(site, "appcast.xml")); string(top) != string(cast) {
		t.Error("the appcast at the top isn't the same")
	}

	latest := Latest{Name: a.Name, Version: "1.0.1", Downloads: map[string]Download{}}
	for k, d := range mac {
		latest.Downloads["macos-"+k] = d
	}
	c := &checker{site: site, url: "https://example.com/app", a: a}
	c.checkMac(latest)
	if len(c.problems) > 0 {
		t.Errorf("a good site: %v", c.problems)
	}
	// The app inside says another version than the appcast.
	os.WriteFile(filepath.Join(site, "macos", a.Name+"-1.0.1-macos.dmg.json"), []byte(`{"bundleVersion":"40","shortVersion":"1.0.0"}`), 0o644)
	c = &checker{site: site, url: "https://example.com/app", a: a}
	c.checkMac(latest)
	if !strings.Contains(strings.Join(c.problems, "\n"), "the app inside is 1.0.0") {
		t.Errorf("a mismatched app wasn't caught: %v", c.problems)
	}
	// latest.json saying another version than the appcast.
	latest.Downloads["macos-universal"] = Download{Version: "1.0.2", URL: "macos/x.dmg"}
	c = &checker{site: site, url: "https://example.com/app", a: a}
	c.checkMac(latest)
	if !strings.Contains(strings.Join(c.problems, "\n"), "latest.json says the Mac's newest is 1.0.2") {
		t.Errorf("a mismatched latest.json wasn't caught: %v", c.problems)
	}
}

// TestNotarisedTogether: the disk image and the package go to Apple at
// once, and each is stapled; a failure is the error.
func TestNotarisedTogether(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "log")
	fake := "#!/bin/sh\n" +
		"case $1 in\n" +
		"notarytool) echo \"start $3\" >>" + log + "; sleep 2; echo \"end $3\" >>" + log + "; case $3 in *bad*) exit 1 ;; esac ;;\n" +
		"stapler) echo \"staple $3\" >>" + log + " ;;\n" +
		"esac\n"
	os.WriteFile(filepath.Join(bin, "xcrun"), []byte(fake), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	began := time.Now()
	if err := notariseAll([]string{"/x/app.dmg", "/x/app.pkg"}, "vero"); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(began); took > 3500*time.Millisecond {
		t.Errorf("took %v: one after the other", took)
	}
	lines := strings.Split(strings.TrimSpace(string(mustRead(t, log))), "\n")
	if len(lines) != 6 || !strings.HasPrefix(lines[0], "start") || !strings.HasPrefix(lines[1], "start") {
		t.Errorf("not at once:\n%s", strings.Join(lines, "\n"))
	}
	for _, f := range []string{"staple /x/app.dmg", "staple /x/app.pkg"} {
		if !slices.Contains(lines, f) {
			t.Errorf("no %q", f)
		}
	}
	if err := notariseAll([]string{"/x/app.dmg", "/x/bad.pkg"}, "vero"); err == nil || !strings.Contains(err.Error(), "bad.pkg") {
		t.Errorf("a failure: %v", err)
	}
}
