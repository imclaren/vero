package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// The front ends that aren't installed from a package manager: a page in a
// browser, Android and iOS apps, a terminal app whose worker is WASI, and
// Plan 9. Each is made only when vero-app.toml has its section.
//
// What signs the Android app comes from the environment, never from
// vero-app.toml, as for the Mac:
//
//	VERO_ANDROID_KEYSTORE    the keystore that signs every release
//	VERO_ANDROID_KEY_ALIAS   the key in it
//	VERO_ANDROID_PASSWORD    its password
//
// Without them, a keystore is made in the signing key's folder, the first
// time, and kept: Android installs an update only if the same key signed it.
//
// An iOS app reaches phones only through the App Store or TestFlight, with
// the publisher's Apple account:
//
//	VERO_IOS_IDENTITY        "Apple Distribution: Name (TEAMID)"
//	VERO_IOS_PROFILE         the app's App Store provisioning profile

// Web is a front end that runs in a browser: the files its build makes,
// put in the site's web folder.
type Web struct {
	Folder string   `toml:"folder"`
	Build  string   `toml:"build"`
	Files  []string `toml:"files"`
}

// Android is the Android app: its build, and the unsigned, aligned .apk
// it makes, which vero signs. AAB is an app bundle its build makes for
// Google Play, which vero signs too.
type Android struct {
	Folder string `toml:"folder"`
	Build  string `toml:"build"`
	APK    string `toml:"apk"`
	AAB    string `toml:"aab"`
}

// IOS is the iOS app: its build, the .app it makes for the Simulator,
// and, built for phones by the publisher's own build, the .app that
// becomes an .ipa for the App Store. AppStore is the app's App Store page.
type IOS struct {
	Folder    string `toml:"folder"`
	Build     string `toml:"build"`
	Simulator string `toml:"simulator_app"`
	Device    string `toml:"device_app"`
	AppStore  string `toml:"app_store_url"`
}

// WASI is a terminal front end, built for each desktop, whose worker is
// one worker.wasm that a WASI runtime runs. Args are what the front end is
// started with, beside the worker: the run script passes them.
type WASI struct {
	Frontend string   `toml:"frontend"`
	Worker   string   `toml:"worker"`
	Args     []string `toml:"args"`
	Runtime  string   `toml:"runtime"`
}

// Plan9 is the Plan 9 front end, a Go program like the worker, which
// finds the worker beside it.
type Plan9 struct {
	Frontend string   `toml:"frontend"`
	Arches   []string `toml:"arches"`
}

// wasiHosts are what a WASI bundle's front end is built for.
var wasiHosts = [][2]string{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "arm64"}, {"darwin", "amd64"}, {"windows", "amd64"}, {"windows", "arm64"}}

// shell runs a front end's build command in its folder.
func shell(dir, what, command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

// packageWeb builds the web front end into NAME-VERSION-web.tar.gz.
func packageWeb(a *App, out string) error {
	w := a.Web
	dir := a.Path(w.Folder)
	if w.Build != "" {
		if err := shell(dir, "[web] build", w.Build); err != nil {
			return err
		}
	}
	var files []treeFile
	for _, f := range w.Files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return fmt.Errorf("[web] files: %w", err)
		}
		files = append(files, treeFile{path: f, data: data, mode: 0o644})
	}
	return writeTarGz(filepath.Join(out, fmt.Sprintf("%s-%s-web.tar.gz", a.Name, a.Version)), "", files)
}

// packageAndroid builds the Android app and signs it, into
// NAME-VERSION-android.apk, and an .aab beside it if the build makes one.
func packageAndroid(a *App, out string, keyDir string) error {
	an := a.Android
	dir := a.Path(an.Folder)
	if an.Build != "" {
		if err := shell(dir, "[android] build", an.Build); err != nil {
			return err
		}
	}
	tools, err := androidBuildTools()
	if err != nil {
		return err
	}
	ks, alias, pass, err := androidKeystore(keyDir)
	if err != nil {
		return err
	}
	apk := filepath.Join(out, fmt.Sprintf("%s-%s-android.apk", a.Name, a.Version))
	aligned := apk + ".aligned"
	if err := runIn("", filepath.Join(tools, "zipalign"), "-f", "4", filepath.Join(dir, an.APK), aligned); err != nil {
		return err
	}
	defer os.Remove(aligned)
	if err := runJava(filepath.Join(tools, "apksigner"), "sign", "--ks", ks, "--ks-key-alias", alias,
		"--ks-pass", "pass:"+pass, "--key-pass", "pass:"+pass, "--out", apk, aligned); err != nil {
		return err
	}
	os.Remove(apk + ".idsig")
	fmt.Println("built", apk)
	if an.AAB != "" {
		aab := filepath.Join(out, fmt.Sprintf("%s-%s-android.aab", a.Name, a.Version))
		if err := copyFile(filepath.Join(dir, an.AAB), aab); err != nil {
			return err
		}
		if err := runIn("", javaTool("jarsigner"), "-keystore", ks, "-storepass", pass, "-keypass", pass, aab, alias); err != nil {
			return err
		}
		fmt.Println("built", aab)
	}
	return nil
}

// javaTool is a program of the JDK: from JAVA_HOME, or Homebrew's
// openjdk, which Homebrew keeps off the PATH, or the PATH.
func javaTool(name string) string {
	home := os.Getenv("JAVA_HOME")
	if home == "" {
		home = "/opt/homebrew/opt/openjdk"
	}
	if p := filepath.Join(home, "bin", name); fileExists(p) {
		return p
	}
	return name
}

// androidBuildTools is the newest build-tools folder of the Android SDK.
func androidBuildTools() (string, error) {
	sdk := os.Getenv("ANDROID_HOME")
	if sdk == "" {
		home, _ := os.UserHomeDir()
		sdk = filepath.Join(home, "Library", "Android", "sdk")
	}
	all, _ := filepath.Glob(filepath.Join(sdk, "build-tools", "*"))
	newest := ""
	for _, d := range all {
		if newest == "" || compareVersions(filepath.Base(d), filepath.Base(newest)) > 0 {
			newest = d
		}
	}
	if newest == "" {
		return "", fmt.Errorf("no Android build-tools in %s: scripts/setup-android.sh installs them", sdk)
	}
	return newest, nil
}

// androidKeystore is the keystore that signs the Android app: the
// publisher's, from the environment, or one made in the key's folder.
func androidKeystore(keyDir string) (ks, alias, pass string, err error) {
	if ks = os.Getenv("VERO_ANDROID_KEYSTORE"); ks != "" {
		alias, pass = os.Getenv("VERO_ANDROID_KEY_ALIAS"), os.Getenv("VERO_ANDROID_PASSWORD")
		if alias == "" || pass == "" {
			return "", "", "", fmt.Errorf("VERO_ANDROID_KEYSTORE needs VERO_ANDROID_KEY_ALIAS and VERO_ANDROID_PASSWORD")
		}
		return ks, alias, pass, nil
	}
	if keyDir == "" {
		return "", "", "", fmt.Errorf("signing the Android app needs a key: give --key, or set VERO_ANDROID_KEYSTORE")
	}
	ks, alias = filepath.Join(keyDir, "android.keystore"), "release"
	passFile := ks + ".password"
	if data, err := os.ReadFile(passFile); err == nil {
		return ks, alias, strings.TrimSpace(string(data)), nil
	}
	pass = randomPassword()
	err = runIn("", javaTool("keytool"), "-genkeypair", "-keystore", ks, "-storetype", "PKCS12", "-alias", alias,
		"-storepass", pass, "-keypass", pass, "-keyalg", "RSA", "-keysize", "4096", "-validity", "10000",
		"-dname", "CN=Android release key")
	if err != nil {
		return "", "", "", err
	}
	if err := os.WriteFile(passFile, []byte(pass+"\n"), 0o600); err != nil {
		return "", "", "", err
	}
	fmt.Printf("made %s, which signs the Android app: keep it with private.asc, since Android takes updates only from the same key\n", ks)
	return ks, alias, pass, nil
}

// packageIOS builds the iOS app: for the Simulator, zipped for testing
// with Xcode; and, when the publisher's build has made one for phones and
// their Apple account is set up, the .ipa for the App Store.
func packageIOS(a *App, out string) error {
	i := a.IOS
	dir := a.Path(i.Folder)
	if i.Build != "" {
		if err := shell(dir, "[ios] build", i.Build); err != nil {
			return err
		}
	}
	if i.Simulator != "" {
		zip := filepath.Join(out, fmt.Sprintf("%s-%s-ios-simulator.zip", a.Name, a.Version))
		os.Remove(zip)
		if err := runIn("", "ditto", "-c", "-k", "--keepParent", filepath.Join(dir, i.Simulator), zip); err != nil {
			return err
		}
		fmt.Println("built", zip, "(for the Simulator: xcrun simctl install booted on what it holds)")
	}
	if i.Device == "" {
		return nil
	}
	identity, profile := os.Getenv("VERO_IOS_IDENTITY"), os.Getenv("VERO_IOS_PROFILE")
	if identity == "" || profile == "" {
		fmt.Println("no .ipa: set VERO_IOS_IDENTITY and VERO_IOS_PROFILE to sign the app for the App Store")
		return nil
	}
	tmp, err := os.MkdirTemp("", "vero-ipa.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	app := filepath.Join(tmp, "Payload", filepath.Base(i.Device))
	if err := os.MkdirAll(filepath.Dir(app), 0o755); err != nil {
		return err
	}
	if err := runIn("", "ditto", filepath.Join(dir, i.Device), app); err != nil {
		return err
	}
	if err := copyFile(profile, filepath.Join(app, "embedded.mobileprovision")); err != nil {
		return err
	}
	// The entitlements the profile grants, which the app is signed with.
	decoded, err := exec.Command("security", "cms", "-D", "-i", profile).Output()
	if err != nil {
		return fmt.Errorf("reading %s: %w", profile, err)
	}
	plist := filepath.Join(tmp, "profile.plist")
	if err := os.WriteFile(plist, decoded, 0o644); err != nil {
		return err
	}
	ents, err := exec.Command("plutil", "-extract", "Entitlements", "xml1", "-o", "-", plist).Output()
	if err != nil {
		return fmt.Errorf("the profile's entitlements: %w", err)
	}
	entFile := filepath.Join(tmp, "entitlements.plist")
	if err := os.WriteFile(entFile, ents, 0o644); err != nil {
		return err
	}
	if err := runIn("", "codesign", "--force", "--sign", identity, "--entitlements", entFile, app); err != nil {
		return err
	}
	ipa := filepath.Join(out, fmt.Sprintf("%s-%s.ipa", a.Name, a.Version))
	os.Remove(ipa)
	if err := runIn(tmp, "zip", "-qry", ipa, "Payload"); err != nil {
		return err
	}
	fmt.Println("built", ipa, "- upload it to App Store Connect with Transporter, or xcrun altool --upload-app")
	return nil
}

// packageWASI builds the worker once, as worker.wasm, and the front end
// for each desktop, into NAME-VERSION-wasi-OS-ARCH.tar.gz.
func packageWASI(a *App, root, out, ldflags, tmp string) error {
	w := a.WASI
	rel := func(p string) (string, error) {
		r, err := filepath.Rel(root, a.Path(p))
		return "./" + filepath.ToSlash(r), err
	}
	worker, err := rel(w.Worker)
	if err != nil {
		return err
	}
	frontend, err := rel(w.Frontend)
	if err != nil {
		return err
	}
	wasm, err := goBuild(root, worker, "wasip1", "wasm", a.Version, ldflags, filepath.Join(tmp, "worker.wasm"))
	if err != nil {
		return err
	}
	runtime := w.Runtime
	if runtime == "" {
		runtime = "wasmtime"
	}
	args := strings.Join(append([]string{"-worker", "worker.wasm"}, w.Args...), " ")
	if len(w.Args) > 0 {
		args = strings.Join(w.Args, " ")
	}
	for _, h := range wasiHosts {
		exe := a.Name
		if h[0] == "windows" {
			exe += ".exe"
		}
		bin, err := goBuild(root, frontend, h[0], h[1], a.Version, ldflags, filepath.Join(tmp, "wasi-"+h[0]+"-"+h[1], exe))
		if err != nil {
			return err
		}
		files := []treeFile{{path: exe, data: bin, mode: 0o755}, {path: "worker.wasm", data: wasm, mode: 0o644}}
		readme := fmt.Sprintf("%s %s\n\n%s\n\nIt needs %s, a WASI runtime, which runs its worker.\nStart it from this folder with ", a.DisplayName, a.Version, a.Summary, runtime)
		if h[0] == "windows" {
			files = append(files, treeFile{path: "run.cmd", data: []byte("@echo off\r\ncd /d \"%~dp0\"\r\n" + exe + " " + args + "\r\n"), mode: 0o755})
			readme += "run.cmd.\n"
		} else {
			files = append(files, treeFile{path: "run", data: []byte("#!/bin/sh\ncd \"$(dirname \"$0\")\" && exec ./" + exe + " " + args + "\n"), mode: 0o755})
			readme += "./run.\n"
		}
		files = append(files, treeFile{path: "README.txt", data: []byte(readme), mode: 0o644})
		name := fmt.Sprintf("%s-%s-wasi-%s-%s.tar.gz", a.Name, a.Version, h[0], h[1])
		if err := writeTarGz(filepath.Join(out, name), a.Name+"/", files); err != nil {
			return err
		}
	}
	fmt.Println("built", len(wasiHosts), "WASI bundles in", out)
	return nil
}

// packagePlan9 builds the worker and the front end for Plan 9, into
// NAME-VERSION-plan9-ARCH.tgz, with an rc script that installs them.
func packagePlan9(a *App, root, worker, out, ldflags, tmp string) error {
	p := a.Plan9
	arches := p.Arches
	if len(arches) == 0 {
		arches = []string{"amd64", "386"}
	}
	front := a.Path(p.Frontend)
	for _, arch := range arches {
		w, err := goBuild(root, worker, "plan9", arch, a.Version, ldflags, filepath.Join(tmp, "plan9-"+arch, "worker"))
		if err != nil {
			return err
		}
		// The front end may be a module of its own, as vero's example is.
		f, err := goBuild(front, ".", "plan9", arch, a.Version, ldflags, filepath.Join(tmp, "plan9-"+arch, a.Name))
		if err != nil {
			return err
		}
		install := fmt.Sprintf(`#!/bin/rc
# Installs %[1]s for this user: the program and its worker in
# $home/lib/%[1]s, and %[1]s in $home/bin/rc to start it.
rfork e
here=`+"`"+`{cd `+"`"+`{basename -d $0} && pwd}
mkdir -p $home/lib/%[1]s $home/bin/rc
cp $here/%[1]s $here/%[2]s $home/lib/%[1]s/
echo '#!/bin/rc' > $home/bin/rc/%[1]s
echo 'cd $home/lib/%[1]s && exec ./%[1]s' >> $home/bin/rc/%[1]s
chmod +x $home/bin/rc/%[1]s
echo installed %[1]s: run it with %[1]s
`, a.Name, a.Worker.Name)
		files := []treeFile{
			{path: a.Name, data: f, mode: 0o755},
			{path: a.Worker.Name, data: w, mode: 0o755},
			{path: "install", data: []byte(install), mode: 0o755},
		}
		name := fmt.Sprintf("%s-%s-plan9-%s.tgz", a.Name, a.Version, arch)
		if err := writeTarGz(filepath.Join(out, name), a.Name+"/", files); err != nil {
			return err
		}
		fmt.Println("built", filepath.Join(out, name))
	}
	return nil
}

// goBuild builds a Go main package for goos on goarch into out, and
// returns what it built.
func goBuild(dir, pkg, goos, goarch, version, ldflags, out string) ([]byte, error) {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return nil, err
	}
	flags := strings.TrimSpace("-s -w -X main.version=" + version + " " + ldflags)
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", flags, "-o", out, pkg)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("building %s for %s/%s: %w", pkg, goos, goarch, err)
	}
	return os.ReadFile(out)
}

// writeTarGz writes files into a gzipped tar, each under prefix.
func writeTarGz(path, prefix string, files []treeFile) error {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	mtime := buildTime()
	if prefix != "" {
		if err := tw.WriteHeader(&tar.Header{Name: prefix, Mode: 0o755, Typeflag: tar.TypeDir, ModTime: mtime, Format: tar.FormatUSTAR}); err != nil {
			return err
		}
	}
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: prefix + f.path, Mode: int64(f.mode.Perm()), Size: int64(len(f.data)),
			ModTime: mtime, Format: tar.FormatUSTAR}); err != nil {
			return err
		}
		if _, err := tw.Write(f.data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// bundle is one kind of bundle: how its files are named, where in the
// site they go, and what latest.json calls the newest of each.
type bundle struct {
	kind  string // the site's folder
	match *regexp.Regexp
	// key is latest.json's name for one, from its match.
	key func(m []string) string
}

func bundles(name string) []bundle {
	q := regexp.QuoteMeta(name)
	return []bundle{
		{"android", regexp.MustCompile(`^` + q + `-(.+)-android\.apk$`), func([]string) string { return "android" }},
		{"wasi", regexp.MustCompile(`^` + q + `-(.+)-wasi-([a-z]+)-([a-z0-9]+)\.tar\.gz$`), func(m []string) string { return "wasi-" + m[2] + "-" + m[3] }},
		{"plan9", regexp.MustCompile(`^` + q + `-(.+)-plan9-([a-z0-9]+)\.tgz$`), func(m []string) string { return "plan9-" + m[2] }},
	}
}

// buildBundles puts the new Android, WASI and Plan 9 bundles in the site,
// keeping the newest keep of each, and unpacks the newest web bundle into
// site/web, where the page runs.
func buildBundles(site string, newFiles []string, keep int, a *App) (map[string]Download, error) {
	newest := map[string]Download{}
	for _, b := range bundles(a.Name) {
		dir := filepath.Join(site, b.kind)
		groups := map[string][]string{} // by latest.json's key
		for _, f := range newFiles {
			if m := b.match.FindStringSubmatch(filepath.Base(f)); m != nil {
				if err := copyFile(f, filepath.Join(dir, filepath.Base(f))); err != nil {
					return nil, err
				}
			}
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if m := b.match.FindStringSubmatch(e.Name()); m != nil {
				groups[b.key(m)] = append(groups[b.key(m)], e.Name())
			}
		}
		for key := range groups {
			one := regexp.MustCompile(`^` + regexp.QuoteMeta(a.Name) + `-(.+)-` + regexp.QuoteMeta(strings.TrimPrefix(suffixOf(groups[key][0], a.Name), "-")) + `$`)
			files, version, err := keepNewest(dir, one, keep)
			if err != nil {
				return nil, err
			}
			if len(files) > 0 {
				newest[key] = Download{Version: version[files[0]], URL: b.kind + "/" + files[0]}
			}
		}
	}
	// The web front end: the newest, unpacked where it runs.
	web := regexp.MustCompile(`^` + regexp.QuoteMeta(a.Name) + `-(.+)-web\.tar\.gz$`)
	var best, bestVersion string
	for _, f := range newFiles {
		if m := web.FindStringSubmatch(filepath.Base(f)); m != nil && (best == "" || compareVersions(m[1], bestVersion) > 0) {
			best, bestVersion = f, m[1]
		}
	}
	if best != "" {
		dir := filepath.Join(site, "web")
		os.RemoveAll(dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if err := untarGz(best, dir); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, ".version"), []byte(bestVersion+"\n"), 0o644); err != nil {
			return nil, err
		}
	}
	if data, err := os.ReadFile(filepath.Join(site, "web", ".version")); err == nil {
		newest["web"] = Download{Version: strings.TrimSpace(string(data)), URL: "web/"}
	}
	return newest, nil
}

// suffixOf is what follows NAME-VERSION in a bundle's name: "-plan9-amd64.tgz".
func suffixOf(file, name string) string {
	rest := strings.TrimPrefix(file, name+"-")
	for _, marker := range []string{"-android", "-wasi-", "-plan9-"} {
		if i := strings.Index(rest, marker); i >= 0 {
			return rest[i:]
		}
	}
	return rest
}

// untarGz unpacks a gzipped tar into dir.
func untarGz(path, dir string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		name := filepath.Clean(h.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("%s: %s is outside the bundle", path, h.Name)
		}
		target := filepath.Join(dir, name)
		if h.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		var b bytes.Buffer
		if _, err := b.ReadFrom(tr); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := fs.FileMode(h.Mode).Perm()
		if mode&0o400 == 0 {
			mode |= 0o644
		}
		if err := os.WriteFile(target, b.Bytes(), mode); err != nil {
			return err
		}
	}
}

// otherSection is the page's section for a bundle.
type otherSection struct {
	System     string
	Label      string
	Paragraphs []template.HTML
	Commands   []string
}

func randomPassword() string {
	b := make([]byte, 18)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// otherSections are the page's sections for the web, Android, iOS, WASI
// and Plan 9, for what the site has.
func otherSections(p page) []otherSection {
	a, d := p.App, p.Latest.Downloads
	esc := template.HTMLEscapeString
	link := func(url, text string) string { return `<a href="` + esc(url) + `">` + esc(text) + `</a>` }
	var out []otherSection
	if w, ok := d["web"]; ok {
		out = append(out, otherSection{System: "web", Label: "In your browser", Paragraphs: []template.HTML{
			template.HTML(link(w.URL, "Open "+a.DisplayName) + " in your browser. It needs nothing installed."),
		}})
	}
	if an, ok := d["android"]; ok {
		out = append(out, otherSection{System: "android", Label: "Android", Paragraphs: []template.HTML{
			template.HTML(link(an.URL, "Download "+a.DisplayName+" for Android") + ` <span class="soft">version ` + esc(an.Version) + `</span>, and open it. Android asks you to allow your browser to install apps the first time.`),
			"To update it, download and open the new version the same way.",
		}})
	}
	if a.IOS != nil && a.IOS.AppStore != "" {
		out = append(out, otherSection{System: "ios", Label: "iPhone and iPad", Paragraphs: []template.HTML{
			template.HTML(link(a.IOS.AppStore, a.DisplayName+" on the App Store") + ", which keeps it up to date."),
		}})
	}
	var wasi []template.HTML
	for _, h := range wasiHosts {
		if w, ok := d["wasi-"+h[0]+"-"+h[1]]; ok {
			label := map[string]string{"linux": "Linux", "darwin": "Mac", "windows": "Windows"}[h[0]] + " (" + map[string]string{"amd64": "x86_64", "arm64": "ARM64"}[h[1]] + ")"
			wasi = append(wasi, template.HTML(link(w.URL, label)))
		}
	}
	if len(wasi) > 0 {
		runtime := "wasmtime"
		if a.WASI != nil && a.WASI.Runtime != "" {
			runtime = a.WASI.Runtime
		}
		paras := []template.HTML{template.HTML("A version for the terminal, whose worker runs in " + esc(runtime) + `, which <a href="https://wasmtime.dev">wasmtime.dev</a> says how to install. Download it for your computer, unpack it, and start it with <code>run</code> in its folder:`)}
		out = append(out, otherSection{System: "wasi", Label: "In a terminal, with WASI", Paragraphs: append(paras, template.HTML(joinHTML(wasi)))})
	}
	for _, arch := range []string{"amd64", "386", "arm"} {
		pl, ok := d["plan9-"+arch]
		if !ok {
			continue
		}
		file := pl.URL[strings.LastIndex(pl.URL, "/")+1:]
		out = append(out, otherSection{System: "plan9", Label: "Plan 9 (" + arch + ")",
			Paragraphs: []template.HTML{"Download it and install it for yourself:"},
			Commands: []string{
				"hget " + pl.URL + " > /tmp/" + file,
				"cd /tmp && gunzip < " + file + " | tar x",
				a.Name + "/install",
			}})
	}
	return out
}

func joinHTML(items []template.HTML) string {
	var s []string
	for _, i := range items {
		s = append(s, string(i))
	}
	return strings.Join(s, " · ")
}

// runJava runs one of the Android SDK's tools, which are Java programs
// that find Java by JAVA_HOME.
func runJava(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	// Shown only when it fails: a new JDK warns about every native call
	// apksigner makes, which is nothing to the person releasing.
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	home := os.Getenv("JAVA_HOME")
	if home == "" && fileExists("/opt/homebrew/opt/openjdk/bin/java") {
		home = "/opt/homebrew/opt/openjdk"
	}
	if home != "" {
		cmd.Env = append(os.Environ(), "JAVA_HOME="+home, "PATH="+filepath.Join(home, "bin")+":"+os.Getenv("PATH"))
	}
	if err := cmd.Run(); err != nil {
		os.Stderr.Write(out.Bytes())
		return fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	return nil
}
