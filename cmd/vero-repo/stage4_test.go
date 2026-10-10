package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/edwards25519"
)

// TestAppleIcon: an .icns of every size Finder uses, each a PNG.
func TestAppleIcon(t *testing.T) {
	a, _ := linuxApp(t)
	icns, err := appleIcon(a.Path(a.Icon))
	if err != nil {
		t.Fatal(err)
	}
	if string(icns[:4]) != "icns" || int(binary.BigEndian.Uint32(icns[4:8])) != len(icns) {
		t.Fatal("not an icns")
	}
	kinds := map[string]bool{}
	for pos := 8; pos < len(icns); {
		kind, size := string(icns[pos:pos+4]), int(binary.BigEndian.Uint32(icns[pos+4:pos+8]))
		if !bytes.HasPrefix(icns[pos+8:pos+size], []byte("\x89PNG")) {
			t.Errorf("%s isn't a PNG", kind)
		}
		kinds[kind] = true
		pos += size
	}
	for _, k := range []string{"icp4", "ic08", "ic10", "ic13"} {
		if !kinds[k] {
			t.Errorf("no %s", k)
		}
	}
}

// TestInfoPlist: the Info.plist of an app vero makes names it, its
// version, and, for Sparkle, where updates are and what key checks them.
func TestInfoPlist(t *testing.T) {
	a, _ := linuxApp(t)
	a.MacOS = &MacOS{Product: "Example", Menubar: true, Feed: "https://example.com/macos/appcast.xml", SparklePublicKey: "abc="}
	a.Build = "42"
	plist := string(infoPlist(a))
	for _, want := range []string{"<string>dev.vero.example</string>", "<key>CFBundleVersion</key>\n\t<string>42</string>",
		"<key>CFBundleShortVersionString</key>\n\t<string>1.2.3-beta</string>", "<key>LSUIElement</key>",
		"<string>https://example.com/macos/appcast.xml</string>", "<key>SUPublicEDKey</key>\n\t<string>abc=</string>"} {
		if !strings.Contains(plist, want) {
			t.Errorf("no %q in\n%s", want, plist)
		}
	}
}

// TestSparkleKeys: a key vero makes, a seed Sparkle's tools make, and an
// expanded key older Sparkle kept all sign the same, as Sparkle checks.
func TestSparkleKeys(t *testing.T) {
	s := testKey(t)
	k, err := s.sparkle()
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("a disk image")
	if !ed25519.Verify(k.public, data, k.sign(data)) {
		t.Fatal("a made key's signature doesn't check")
	}
	again, _ := s.sparkle()
	if !bytes.Equal(again.public, k.public) {
		t.Fatal("the key changed when read again")
	}

	// An expanded key: scalar and prefix, as SHA-512 of a seed gives them.
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	h := sha512Of(priv.Seed())
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64
	expanded := base64.StdEncoding.EncodeToString(h[:])
	dir := t.TempDir()
	file := filepath.Join(dir, "old.key")
	os.WriteFile(file, []byte(expanded+"\n"), 0o600)
	if err := importSparkle(dir, file); err != nil {
		t.Fatal(err)
	}
	imported, err := (&signer{dir: dir}).sparkle()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(imported.public, priv.Public().(ed25519.PublicKey)) {
		t.Fatal("the expanded key's public half differs from the seed's")
	}
	if !ed25519.Verify(imported.public, data, imported.sign(data)) {
		t.Fatal("an expanded key's signature doesn't check")
	}
	if !bytes.Equal(imported.sign(data), ed25519.Sign(priv, data)) {
		t.Fatal("an expanded key signs differently from its seed")
	}
	if err := importSparkle(dir, file); err == nil {
		t.Fatal("a Sparkle key was replaced")
	}

	// The same expanded key followed by its public key, as older Sparkle
	// kept them in the keychain; and one whose halves don't match.
	pub := priv.Public().(ed25519.PublicKey)
	both := base64.StdEncoding.EncodeToString(append(append([]byte{}, h[:]...), pub...))
	dir96 := t.TempDir()
	os.WriteFile(filepath.Join(dir96, "old.key"), []byte(both), 0o600)
	if err := importSparkle(dir96, filepath.Join(dir96, "old.key")); err != nil {
		t.Fatal(err)
	}
	if k96, err := (&signer{dir: dir96}).sparkle(); err != nil || !bytes.Equal(k96.sign(data), ed25519.Sign(priv, data)) {
		t.Fatalf("a 96-byte key signs differently from its seed: %v", err)
	}
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	bad := base64.StdEncoding.EncodeToString(append(append([]byte{}, h[:]...), other.Public().(ed25519.PublicKey)...))
	if _, err := parseSparkleKey(bad); err == nil {
		t.Fatal("a 96-byte key with someone else's public half was taken")
	}
	// The scalar made from a seed is what the expanded key holds.
	if _, err := edwards25519.NewScalar().SetBytesWithClamping(h[:32]); err != nil {
		t.Fatal(err)
	}
}

func sha512Of(seed []byte) [64]byte {
	return sha512Sum(seed)
}

// TestAppcastAndCask: a site with two disk images gets an appcast with
// both, newest first, each signed, and a cask for the newest.
func TestAppcastAndCask(t *testing.T) {
	a, _ := linuxApp(t)
	a.MacOS = &MacOS{Product: "Example"}
	s := testKey(t)
	site, pkgs := t.TempDir(), t.TempDir()
	var dmgs []string
	for _, v := range []string{"1.0.0", "1.0.1"} {
		dmg := filepath.Join(pkgs, "vero-example-"+v+"-macos.dmg")
		os.WriteFile(dmg, []byte("dmg "+v), 0o644)
		info, _ := json.Marshal(macInfo{BundleVersion: v, ShortVersion: v, Minimum: "13.0", App: "vero example.app"})
		os.WriteFile(dmg+".json", info, 0o644)
		dmgs = append(dmgs, dmg)
	}
	newest, err := buildMac(site, dmgs, nil, "", 3, "https://example.com/vero/", a, s)
	if err != nil {
		t.Fatal(err)
	}
	if newest["universal"].Version != "1.0.1" {
		t.Errorf("newest %+v", newest)
	}
	cast, _ := os.ReadFile(filepath.Join(site, "macos", "appcast.xml"))
	var rss struct {
		Items []struct {
			Version   string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle version"`
			Enclosure struct {
				URL       string `xml:"url,attr"`
				Length    int    `xml:"length,attr"`
				Signature string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle edSignature,attr"`
			} `xml:"enclosure"`
		} `xml:"channel>item"`
	}
	if err := xml.Unmarshal(cast, &rss); err != nil {
		t.Fatal(err)
	}
	if len(rss.Items) != 2 || rss.Items[0].Version != "1.0.1" || rss.Items[0].Enclosure.URL != "https://example.com/vero/macos/vero-example-1.0.1-macos.dmg" {
		t.Fatalf("appcast: %+v", rss.Items)
	}
	key, _ := s.sparkle()
	sig, _ := base64.StdEncoding.DecodeString(rss.Items[0].Enclosure.Signature)
	if !ed25519.Verify(key.public, []byte("dmg 1.0.1"), sig) {
		t.Error("the appcast's signature doesn't check")
	}
	cask, _ := os.ReadFile(filepath.Join(site, "homebrew", "vero-example.rb"))
	if !strings.Contains(string(cask), `version "1.0.1"`) || !strings.Contains(string(cask), `app "vero example.app"`) {
		t.Errorf("cask:\n%s", cask)
	}
	// The check sees it all, and catches a damaged image.
	a.Version = "1.0.1"
	latest := Latest{Name: a.Name, Version: "1.0.1", Downloads: map[string]Download{"macos-universal": newest["universal"]}}
	if err := writeSite(site, "https://example.com/vero", a, latest, s, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := checkSite(site, "https://example.com/vero", a, key.public, nil); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(site, "macos", "vero-example-1.0.1-macos.dmg"), []byte("dmg 1.0.1 changed"), 0o644)
	err = checkSite(site, "https://example.com/vero", a, key.public, nil)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Errorf("a changed image passed: %v", err)
	}
	// A release that goes backwards is caught too.
	os.WriteFile(filepath.Join(site, "macos", "vero-example-1.0.1-macos.dmg"), []byte("dmg 1.0.1"), 0o644)
	previous := &Latest{Downloads: map[string]Download{"macos-universal": {Version: "1.0.2", URL: "x"}}}
	if err := checkSite(site, "https://example.com/vero", a, key.public, previous); err == nil || !strings.Contains(err.Error(), "goes back") {
		t.Errorf("going back from 1.0.2 passed: %v", err)
	}
}

// TestNoPage: --no-page leaves the page and install.sh out.
func TestNoPage(t *testing.T) {
	a, _ := linuxApp(t)
	s := testKey(t)
	site := t.TempDir()
	os.WriteFile(filepath.Join(site, "index.html"), []byte("old"), 0o644)
	latest := Latest{Name: a.Name, Version: "1.2.3", Downloads: map[string]Download{"windows-x64": {"1.2.3", "windows/x.exe"}}}
	if err := writeSite(site, "https://example.com", a, latest, s, true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(site, "index.html")); err == nil {
		t.Error("index.html was written")
	}
	if _, err := os.Stat(filepath.Join(site, "latest.json")); err != nil {
		t.Error("latest.json wasn't")
	}
}

// TestWinget: three manifests, naming the installers and their hashes.
func TestWinget(t *testing.T) {
	a, _ := linuxApp(t)
	a.WPF = &WPF{Folder: "wpf", Exe: "Example.exe"}
	site := t.TempDir()
	os.MkdirAll(filepath.Join(site, "windows"), 0o755)
	os.WriteFile(filepath.Join(site, "windows", "vero-example-1.2.3-x64-setup.exe"), []byte("exe"), 0o644)
	os.WriteFile(filepath.Join(site, "windows", "vero-example-1.2.3-x86-setup.exe"), []byte("exe"), 0o644)
	windows := map[string]Download{"x64": {"1.2.3", "windows/vero-example-1.2.3-x64-setup.exe"}, "x86": {"1.2.3", "windows/vero-example-1.2.3-x86-setup.exe"}}
	if err := writeWinget(site, "https://example.com/vero", a, windows); err != nil {
		t.Fatal(err)
	}
	if id := wingetID(a); id != "ExamplePublisher.VeroExample" {
		t.Errorf("id %s", id)
	}
	dir := filepath.Join(site, "winget", "manifests", "e", "ExamplePublisher", "VeroExample", "1.2.3")
	installer, err := os.ReadFile(filepath.Join(dir, "ExamplePublisher.VeroExample.installer.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	sum, _ := fileSHA256(filepath.Join(site, "windows", "vero-example-1.2.3-x64-setup.exe"))
	for _, want := range []string{"InstallerType: nullsoft", "Silent: /S", "- Architecture: x64", "- Architecture: x86",
		"InstallerUrl: \"https://example.com/vero/windows/vero-example-1.2.3-x64-setup.exe\"", strings.ToUpper(sum), "ManifestVersion: " + wingetSchema} {
		if !strings.Contains(string(installer), want) {
			t.Errorf("no %q in\n%s", want, installer)
		}
	}
	for _, f := range []string{"ExamplePublisher.VeroExample.locale.en-US.yaml", "ExamplePublisher.VeroExample.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Error(err)
		}
	}
	// And the check reads them.
	c := &checker{site: site, url: "https://example.com/vero", a: a}
	c.checkWinget()
	if len(c.problems) > 0 {
		t.Error(c.problems)
	}
	os.WriteFile(filepath.Join(site, "windows", "vero-example-1.2.3-x64-setup.exe"), []byte("changed"), 0o644)
	c.checkWinget()
	if len(c.problems) == 0 {
		t.Error("a changed installer passed")
	}
}

// TestMSIX: a package with its manifest, a block map hashing every file
// in 64 KB blocks, and content types; and a version the Store takes.
func TestMSIX(t *testing.T) {
	a, _ := linuxApp(t)
	a.Needs.Autostart, a.Needs.Network = true, true
	a.WPF = &WPF{Folder: "wpf", Exe: "Example.exe", MSIX: MSIXConfig{IdentityName: "ExamplePublisher.Example", Publisher: "CN=0000-1111"}}
	pub := t.TempDir()
	big := bytes.Repeat([]byte("x"), msixBlock+10)
	os.WriteFile(filepath.Join(pub, "Example.exe"), big, 0o755)
	os.MkdirAll(filepath.Join(pub, "runtimes"), 0o755)
	os.WriteFile(filepath.Join(pub, "runtimes", "a.dll"), []byte("dll"), 0o644)
	worker := filepath.Join(t.TempDir(), "worker.exe")
	os.WriteFile(worker, []byte("worker"), 0o755)
	out := t.TempDir()
	path, err := packageMSIX(a, pub, worker, "x64", out)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		for _, f := range z.File {
			if f.Name == name {
				if f.Method != zip.Store {
					t.Errorf("%s is compressed", name)
				}
				r, _ := f.Open()
				var b bytes.Buffer
				b.ReadFrom(r)
				return b.String()
			}
		}
		t.Errorf("no %s", name)
		return ""
	}
	manifest := read("AppxManifest.xml")
	for _, want := range []string{`Name="ExamplePublisher.Example"`, `Publisher="CN=0000-1111"`, `Version="1.2.3.0"`,
		`Executable="Example.exe"`, "windows.startupTask", `Name="internetClient"`, "runFullTrust"} {
		if !strings.Contains(manifest, want) {
			t.Errorf("no %q in the manifest", want)
		}
	}
	blocks := read("AppxBlockMap.xml")
	if strings.Count(blocks, "<Block ") != 2+1+1+3+1 { // the exe in 2, the dll, the worker, 3 logos, the manifest
		t.Errorf("block map:\n%s", blocks)
	}
	if !strings.Contains(blocks, `<File Name="runtimes\a.dll" Size="3" LfhSize="44">`) {
		t.Errorf("block map:\n%s", blocks)
	}
	types := read("[Content_Types].xml")
	if !strings.Contains(types, `Extension="exe"`) || !strings.Contains(types, "vnd.ms-appx.manifest+xml") {
		t.Errorf("content types:\n%s", types)
	}
	read("worker.exe")
	read("Assets/StoreLogo.png")
	for v, want := range map[string]string{"1.2.3": "1.2.3.0", "2.0": "2.0.0.0", "1.2.3-beta.4": "1.2.3.0", "0.3.21": "0.3.21.0"} {
		if got := msixVersion(v); got != want {
			t.Errorf("msixVersion(%s) = %s", v, got)
		}
	}
}

// TestBundles: the web, WASI and Plan 9 bundles find their places in the
// site, the newest kept, and the check knows what each should hold.
func TestBundles(t *testing.T) {
	a, _ := linuxApp(t)
	a.Web = &Web{Folder: "web", Files: []string{"index.html", "main.wasm"}}
	pkgs, site := t.TempDir(), t.TempDir()
	tgz := func(name, prefix string, files []treeFile) string {
		p := filepath.Join(pkgs, name)
		if err := writeTarGz(p, prefix, files); err != nil {
			t.Fatal(err)
		}
		return p
	}
	var in []string
	for _, v := range []string{"1.0.0", "1.0.1"} {
		in = append(in,
			tgz("vero-example-"+v+"-web.tar.gz", "", []treeFile{{path: "index.html", data: []byte("<p>" + v)}, {path: "main.wasm", data: []byte("wasm")}}),
			tgz("vero-example-"+v+"-wasi-linux-amd64.tar.gz", "vero-example/", []treeFile{{path: "vero-example", data: []byte("bin"), mode: 0o755}, {path: "worker.wasm", data: []byte("w")}}),
			tgz("vero-example-"+v+"-plan9-amd64.tgz", "vero-example/", []treeFile{{path: "vero-example", data: []byte("bin"), mode: 0o755}, {path: "worker", data: []byte("w")}}))
	}
	newest, err := buildBundles(site, in, 1, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"web", "wasi-linux-amd64", "plan9-amd64"} {
		if newest[k].Version != "1.0.1" {
			t.Errorf("%s: %+v", k, newest[k])
		}
	}
	if left, _ := filepath.Glob(filepath.Join(site, "wasi", "*")); len(left) != 1 {
		t.Errorf("kept %v", left)
	}
	if page, _ := os.ReadFile(filepath.Join(site, "web", "index.html")); string(page) != "<p>1.0.1" {
		t.Errorf("web/index.html: %s", page)
	}
	latest := Latest{Name: a.Name, Version: "1.0.1", Downloads: map[string]Download{}}
	for k, d := range newest {
		d.URL = "https://example.com/" + d.URL
		latest.Downloads[k] = d
	}
	c := &checker{site: site, url: "https://example.com", a: a}
	c.checkBundles(latest)
	if len(c.problems) > 0 {
		t.Error(c.problems)
	}
	// A bundle missing its worker is caught.
	os.WriteFile(filepath.Join(site, "plan9", "vero-example-1.0.1-plan9-amd64.tgz"), mustTarGz(t, []treeFile{{path: "vero-example", data: []byte("bin")}}), 0o644)
	c.checkBundles(latest)
	if len(c.problems) != 1 || !strings.Contains(c.problems[0], "doesn't hold worker") {
		t.Error(c.problems)
	}
}

func mustTarGz(t *testing.T, files []treeFile) []byte {
	p := filepath.Join(t.TempDir(), "x.tgz")
	if err := writeTarGz(p, "vero-example/", files); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	return data
}

// TestCheckApt: the check reads the apt repository, and catches a package
// that doesn't match its index.
func TestCheckApt(t *testing.T) {
	a, _ := linuxApp(t)
	s := testKey(t)
	site, pkgs := t.TempDir(), t.TempDir()
	workers := map[string]string{}
	for _, arch := range linuxArches {
		w := filepath.Join(pkgs, "w-"+arch.goarch)
		os.WriteFile(w, []byte("ELF"), 0o755)
		workers[arch.goarch] = w
	}
	if err := packageDeb(a, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	debs, _ := filepath.Glob(filepath.Join(pkgs, "*.deb"))
	if _, err := buildApt(site, debs, 3, a, s); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(site, publicFile), s.public, 0o644)
	c := &checker{site: site, url: "https://example.com", a: a}
	kr, _ := loadKeyring(s.public)
	c.keyring = kr
	c.checkApt()
	if len(c.problems) > 0 {
		t.Fatal(c.problems)
	}
	pool, _ := filepath.Glob(filepath.Join(site, "apt", "pool", "main", "v", "vero-example", "*_arm64.deb"))
	os.WriteFile(pool[0], []byte("changed"), 0o644)
	c.checkApt()
	if len(c.problems) == 0 {
		t.Error("a changed .deb passed")
	}
}
