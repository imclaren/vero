package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/clearsign"
	"github.com/ulikunitz/xz"
)

func TestMain(m *testing.M) {
	keyBits = 2048 // as good for a test, and much quicker to make
	now = func() time.Time { return time.Date(2026, 10, 6, 1, 2, 3, 0, time.UTC) }
	os.Exit(m.Run())
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.10", "1.0.9", 1},
		{"1.0~beta1", "1.0", -1},
		{"1.0", "1.0a", -1},
		{"1:0.1", "2.0", 1},
		{"1.0-2", "1.0-10", -1},
		{"1.0.0", "1.0", 1},
		{"0.9.0", "0.10.0", -1},
	} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLoadApp(t *testing.T) {
	a, err := LoadApp("../../example/vero-app.toml")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "vero-example" || a.GTK == nil || a.WPF == nil || a.PublisherName() != "Example Publisher" {
		t.Errorf("the example: %+v", a)
	}
	if root, err := a.ModuleRoot(); err != nil || filepath.Base(root) == "example" {
		t.Errorf("its module root: %s, %v", root, err)
	}
	bad := filepath.Join(t.TempDir(), "vero-app.toml")
	os.WriteFile(bad, []byte("name = \"x\"\nnmae = \"typo\"\n"), 0o644)
	if _, err := LoadApp(bad); err == nil || !strings.Contains(err.Error(), "nmae") {
		t.Errorf("a misspelt setting: %v", err)
	}
}

func TestKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "key")
	if err := makeKey(dir, "Example Publisher", "you@example.com"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(dir, privateFile)); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the private key: %v, %v", info.Mode(), err)
	}
	pub, _ := os.ReadFile(filepath.Join(dir, publicFile))
	if !strings.Contains(string(pub), "BEGIN PGP PUBLIC KEY BLOCK") || strings.Contains(string(pub), "PRIVATE") {
		t.Errorf("the public key: %.60s", pub)
	}
	if err := makeKey(dir, "Someone Else", "else@example.com"); err == nil {
		t.Error("a key was replaced")
	}
	if _, err := loadKey(dir); err != nil {
		t.Error(err)
	}
}

// makeDeb writes a minimal .deb: debian-binary, control.tar.gz (or .xz),
// and an empty data.tar.gz.
func makeDeb(t *testing.T, dir, name, version, arch string, useXZ bool) string {
	t.Helper()
	control := fmt.Sprintf("Package: %s\nVersion: %s\nArchitecture: %s\nMaintainer: Example Publisher <you@example.com>\nDepends: python3\nDescription: An example\n A second line.\n .\n A new paragraph.\n", name, version, arch)
	var ctl bytes.Buffer
	tw := tar.NewWriter(&ctl)
	tw.WriteHeader(&tar.Header{Name: "./control", Mode: 0o644, Size: int64(len(control))})
	tw.Write([]byte(control))
	tw.Close()
	var packed bytes.Buffer
	member := "control.tar.gz"
	if useXZ {
		member = "control.tar.xz"
		w, _ := xz.NewWriter(&packed)
		w.Write(ctl.Bytes())
		w.Close()
	} else {
		w := gzip.NewWriter(&packed)
		w.Write(ctl.Bytes())
		w.Close()
	}
	var data bytes.Buffer
	z := gzip.NewWriter(&data)
	tar.NewWriter(z).Close()
	z.Close()
	var ar bytes.Buffer
	ar.WriteString("!<arch>\n")
	add := func(n string, body []byte) {
		fmt.Fprintf(&ar, "%-16s%-12d%-6d%-6d%-8s%-10d`\n", n, 0, 0, 0, "100644", len(body))
		ar.Write(body)
		if len(body)%2 == 1 {
			ar.WriteByte('\n')
		}
	}
	add("debian-binary", []byte("2.0\n"))
	add(member, packed.Bytes())
	add("data.tar.gz", data.Bytes())
	path := filepath.Join(dir, fmt.Sprintf("%s_%s_%s.deb", name, version, arch))
	if err := os.WriteFile(path, ar.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadControl(t *testing.T) {
	dir := t.TempDir()
	for _, useXZ := range []bool{false, true} {
		c, err := readControl(makeDeb(t, dir, "vero-example", "1.2.3", "arm64", useXZ))
		if err != nil {
			t.Fatal(err)
		}
		if c.Get("Version") != "1.2.3" || c.Get("Architecture") != "arm64" ||
			c.Get("Description") != "An example\n A second line.\n .\n A new paragraph." {
			t.Errorf("xz %v: %+v", useXZ, c.Fields)
		}
	}
}

func testApp() *App {
	return &App{Name: "vero-example", DisplayName: "vero example", ID: "dev.vero.example",
		Summary: "Shows jobs running in a Go worker", Publisher: "Example Publisher <you@example.com>"}
}

func TestBuild(t *testing.T) {
	tmp := t.TempDir()
	keyDir := filepath.Join(tmp, "key")
	if err := makeKey(keyDir, "Example Publisher", "you@example.com"); err != nil {
		t.Fatal(err)
	}
	s, err := loadKey(keyDir)
	if err != nil {
		t.Fatal(err)
	}
	ring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(s.public))
	if err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(tmp, "site")
	release := func(version string, keep int) Latest {
		t.Helper()
		pkgs := filepath.Join(tmp, "packages-"+version)
		os.MkdirAll(pkgs, 0o755)
		debs := []string{makeDeb(t, pkgs, "vero-example", version, "amd64", false), makeDeb(t, pkgs, "vero-example", version, "arm64", true)}
		var exes []string
		for _, arch := range []string{"x64", "arm64"} {
			exe := filepath.Join(pkgs, fmt.Sprintf("vero-example-%s-%s-setup.exe", version, arch))
			os.WriteFile(exe, []byte("MZ "+version), 0o644)
			exes = append(exes, exe)
		}
		latest, err := build(site, debs, exes, keep, "https://example.com/vero/", testApp(), s)
		if err != nil {
			t.Fatal(err)
		}
		return latest
	}
	release("1.0.0", 2)
	release("1.0.10", 2)
	latest := release("1.0.9", 2)

	// Two versions kept, by version not by when they came: 1.0.0 is gone.
	debs, _ := filepath.Glob(filepath.Join(site, "apt", "pool", "main", "v", "vero-example", "*.deb"))
	exes, _ := filepath.Glob(filepath.Join(site, "windows", "*.exe"))
	if len(debs) != 4 || len(exes) != 4 {
		t.Errorf("kept %d .debs and %d installers, not 4 of each", len(debs), len(exes))
	}
	for _, f := range append(debs, exes...) {
		if strings.Contains(filepath.Base(f), "1.0.0") {
			t.Errorf("%s wasn't removed", filepath.Base(f))
		}
	}

	// latest.json: the newest, with where to download it.
	if latest.Version != "1.0.10" {
		t.Errorf("latest is %s", latest.Version)
	}
	var onDisk Latest
	data, _ := os.ReadFile(filepath.Join(site, "latest.json"))
	json.Unmarshal(data, &onDisk)
	want := map[string]string{
		"linux-amd64":   "https://example.com/vero/apt/pool/main/v/vero-example/vero-example_1.0.10_amd64.deb",
		"linux-arm64":   "https://example.com/vero/apt/pool/main/v/vero-example/vero-example_1.0.10_arm64.deb",
		"windows-x64":   "https://example.com/vero/windows/vero-example-1.0.10-x64-setup.exe",
		"windows-arm64": "https://example.com/vero/windows/vero-example-1.0.10-arm64-setup.exe",
	}
	for k, url := range want {
		if d := onDisk.Downloads[k]; d.URL != url || d.Version != "1.0.10" {
			t.Errorf("latest.json %s: %+v", k, d)
		}
	}

	// The Packages index: each version, with hashes that match the files.
	dist := filepath.Join(site, "apt", "dists", "stable")
	packages, _ := os.ReadFile(filepath.Join(dist, "main", "binary-amd64", "Packages"))
	for _, para := range strings.Split(strings.TrimSpace(string(packages)), "\n\n") {
		c, err := parseControl([]byte(para))
		if err != nil {
			t.Fatal(err)
		}
		file, _ := os.ReadFile(filepath.Join(site, "apt", c.Get("Filename")))
		sum := sha256.Sum256(file)
		if c.Get("SHA256") != hex.EncodeToString(sum[:]) || c.Get("Size") != fmt.Sprint(len(file)) {
			t.Errorf("%s: the index's hash or size doesn't match", c.Get("Filename"))
		}
		if !strings.Contains(c.Get("Description"), "\n .\n A new paragraph.") {
			t.Errorf("the long description wasn't kept: %q", c.Get("Description"))
		}
	}
	if n := strings.Count(string(packages), "Package: vero-example"); n != 2 {
		t.Errorf("binary-amd64 lists %d packages", n)
	}

	// Release: each index's size and hashes, signed both ways.
	rel, _ := os.ReadFile(filepath.Join(dist, "Release"))
	for _, idx := range []string{"main/binary-amd64/Packages", "main/binary-arm64/Packages.gz"} {
		data, _ := os.ReadFile(filepath.Join(dist, idx))
		m, s2 := md5.Sum(data), sha256.Sum256(data)
		for _, line := range []string{
			fmt.Sprintf(" %s %d %s\n", hex.EncodeToString(m[:]), len(data), idx),
			fmt.Sprintf(" %s %d %s\n", hex.EncodeToString(s2[:]), len(data), idx),
		} {
			if !strings.Contains(string(rel), line) {
				t.Errorf("Release lacks %q", line)
			}
		}
	}
	if !strings.Contains(string(rel), "Date: Tue, 06 Oct 2026 01:02:03 UTC\n") || !strings.Contains(string(rel), "Architectures: amd64 arm64\n") {
		t.Errorf("Release:\n%s", rel)
	}
	sig, _ := os.Open(filepath.Join(dist, "Release.gpg"))
	defer sig.Close()
	if _, err := openpgp.CheckArmoredDetachedSignature(ring, bytes.NewReader(rel), sig, nil); err != nil {
		t.Errorf("Release.gpg: %v", err)
	}
	in, _ := os.ReadFile(filepath.Join(dist, "InRelease"))
	block, _ := clearsign.Decode(in)
	if block == nil {
		t.Fatal("InRelease isn't clearsigned")
	}
	if _, err := block.VerifySignature(ring, nil); err != nil {
		t.Errorf("InRelease: %v", err)
	}
	if !bytes.Equal(bytes.TrimRight(block.Plaintext, "\n"), bytes.TrimRight(rel, "\n")) {
		t.Error("InRelease doesn't hold Release")
	}

	// The pages: the public key, and how to install, on each system.
	key, _ := os.ReadFile(filepath.Join(site, "key.asc"))
	if !bytes.Equal(key, s.public) {
		t.Error("key.asc isn't the public key")
	}
	page, _ := os.ReadFile(filepath.Join(site, "index.html"))
	for _, want := range []string{
		"deb [signed-by=/etc/apt/keyrings/vero-example.asc] https://example.com/vero/apt stable main",
		"sudo apt install vero-example",
		`href="https://example.com/vero/windows/vero-example-1.0.10-arm64-setup.exe"`,
		"curl -fsSL https://example.com/vero/install.sh | sh",
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("index.html lacks %q", want)
		}
	}
	script, _ := os.ReadFile(filepath.Join(site, "install.sh"))
	if !strings.Contains(string(script), "sudo apt install -y vero-example") || !strings.HasPrefix(string(script), "#!/bin/sh") {
		t.Errorf("install.sh:\n%s", script)
	}
}

func TestAppStream(t *testing.T) {
	a := testApp()
	a.Version, a.Licence, a.Homepage = "1.0.0", "MIT", "https://example.com"
	a.Description = "One paragraph,\nwrapped.\n\nAnother."
	x := string(appStream(a))
	for _, want := range []string{"<id>dev.vero.example</id>", "<p>One paragraph, wrapped.</p>", "<p>Another.</p>",
		`<launchable type="desktop-id">dev.vero.example.desktop</launchable>`, `<release version="1.0.0"></release>`} {
		if !strings.Contains(x, want) {
			t.Errorf("metainfo lacks %s:\n%s", want, x)
		}
	}
}
