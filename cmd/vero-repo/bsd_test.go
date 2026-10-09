package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/cavaliergopher/rpm"
	"github.com/ulikunitz/xz"
)

// testKey makes a signing key in a temporary folder.
func testKey(t *testing.T) *signer {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "key")
	if err := makeKey(dir, "Example Publisher", "you@example.com"); err != nil {
		t.Fatal(err)
	}
	s, err := loadKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// arMembers are the members of an ar archive, by name.
func arMembers(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	if !bytes.HasPrefix(data, []byte("!<arch>\n")) {
		t.Fatal("not an ar archive")
	}
	out := map[string][]byte{}
	var order []string
	for pos := 8; pos+60 <= len(data); {
		h := data[pos : pos+60]
		name := strings.TrimSpace(string(h[0:16]))
		var size int
		for _, c := range strings.TrimSpace(string(h[48:58])) {
			size = size*10 + int(c-'0')
		}
		if string(h[58:60]) != "`\n" {
			t.Fatalf("a damaged ar header for %s", name)
		}
		out[name] = data[pos+60 : pos+60+size]
		order = append(order, name)
		pos += 60 + size + size%2
	}
	if strings.Join(order, " ") != "debian-binary control.tar.gz data.tar.xz" {
		t.Errorf("the .deb holds %v", order)
	}
	return out
}

func TestPackageDeb(t *testing.T) {
	a, workers := linuxApp(t)
	a.Description = "First paragraph, which is long enough that it has to be wrapped onto a second line of the control file.\n\nSecond."
	out := t.TempDir()
	if err := packageDeb(a, workers, out); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(out, "vero-example_1.2.3-beta_arm64.deb")
	c, err := readControl(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Get("Package") != "vero-example" || c.Get("Version") != "1.2.3-beta" || c.Get("Architecture") != "arm64" || c.Get("Installed-Size") == "" {
		t.Errorf("the control file: %+v", c.Fields)
	}
	desc := c.Get("Description")
	if !strings.HasPrefix(desc, "Shows jobs running in a Go worker\n First paragraph") || !strings.Contains(desc, "\n .\n Second.") {
		t.Errorf("the description: %q", desc)
	}
	for _, line := range strings.Split(desc, "\n") {
		if len(line) > 73 {
			t.Errorf("a description line longer than 72: %q", line)
		}
	}
	data, _ := os.ReadFile(path)
	m := arMembers(t, data)
	if string(m["debian-binary"]) != "2.0\n" {
		t.Errorf("debian-binary is %q", m["debian-binary"])
	}
	z, err := xz.NewReader(bytes.NewReader(m["data.tar.xz"]))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(z)
	files := map[string]*tar.Header{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Uname != "root" || h.Gname != "root" || h.Uid != 0 || h.Gid != 0 {
			t.Errorf("%s is owned by %s:%s", h.Name, h.Uname, h.Gname)
		}
		files[h.Name] = h
	}
	for name, mode := range map[string]int64{
		"./usr/":                                            0o755,
		"./usr/lib/vero-example/":                           0o755,
		"./usr/bin/vero-example":                            0o755,
		"./usr/lib/vero-example/worker":                     0o755,
		"./usr/lib/vero-example/vero.py":                    0o644,
		"./usr/share/applications/dev.vero.example.desktop": 0o644,
	} {
		if h := files[name]; h == nil || h.Mode != mode {
			t.Errorf("%s: %+v", name, h)
		}
	}
	if files["./usr/lib/vero-example/__pycache__/main.cpython-312.pyc"] != nil {
		t.Error("Python's cache is in the package")
	}
}

func TestDebFromFolder(t *testing.T) {
	dir := t.TempDir()
	writeTree(filepath.Join(dir, "DEBIAN", "control"), []byte("Package: x\nVersion: 1\nArchitecture: amd64\nMaintainer: A <a@example.com>\nDescription: y\n"), 0o644)
	writeTree(filepath.Join(dir, "DEBIAN", "postinst"), []byte("#!/bin/sh\n"), 0o755)
	writeTree(filepath.Join(dir, "usr", "bin", "x"), []byte("#!/bin/sh\n"), 0o755)
	out := filepath.Join(t.TempDir(), "x.deb")
	if err := debFromFolder(dir, out); err != nil {
		t.Fatal(err)
	}
	c, err := readControl(out)
	if err != nil {
		t.Fatal(err)
	}
	// One for the file, and one for each of its folders, /usr/bin and /usr.
	if c.Get("Installed-Size") != "3" {
		t.Errorf("Installed-Size is %q, not 3", c.Get("Installed-Size"))
	}
	data, _ := os.ReadFile(out)
	z, _ := gzip.NewReader(bytes.NewReader(arMembers(t, data)["control.tar.gz"]))
	tr := tar.NewReader(z)
	var names []string
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, h.Name)
		if h.Name == "./postinst" && h.Mode != 0o755 {
			t.Errorf("postinst's mode is %o", h.Mode)
		}
	}
	if strings.Join(names, " ") != "./ ./control ./md5sums ./postinst" {
		t.Errorf("control.tar.gz holds %v", names)
	}
}

func TestSignRPMAndRepodata(t *testing.T) {
	a, workers := linuxApp(t)
	a.GTK.RPM.Requires = "python3 >= 3.10, (gtk4 if foo else gtk3)"
	s := testKey(t)
	site := t.TempDir()
	pkgs := t.TempDir()
	if err := packageRPM(a, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	rpms, _ := filepath.Glob(filepath.Join(pkgs, "*.rpm"))
	newest, err := buildRPM(site, rpms, 3, "https://example.com/x", a, s)
	if err != nil {
		t.Fatal(err)
	}
	if newest["aarch64"].URL != "rpm/packages/vero-example-1.2.3~beta-1.aarch64.rpm" || newest["aarch64"].Version != "1.2.3-beta" {
		t.Errorf("the newest: %+v", newest)
	}
	ring, _ := openpgp.ReadArmoredKeyRing(bytes.NewReader(s.public))

	// Each rpm signed: its header alone, and its header and payload.
	file := filepath.Join(site, "rpm", "packages", "vero-example-1.2.3~beta-1.x86_64.rpm")
	data, _ := os.ReadFile(file)
	sig, sigLen, err := readHeader(data, 96)
	if err != nil {
		t.Fatal(err)
	}
	start := 96 + sigLen + (8-sigLen%8)%8
	_, mainLen, _ := readHeader(data, start)
	header := data[start : start+mainLen]
	found := map[int]bool{}
	for _, e := range sig {
		switch e.tag {
		case sigRSA:
			_, err = openpgp.CheckDetachedSignature(ring, bytes.NewReader(header), bytes.NewReader(e.data), nil)
		case sigPGP:
			_, err = openpgp.CheckDetachedSignature(ring, bytes.NewReader(data[start:]), bytes.NewReader(e.data), nil)
		default:
			continue
		}
		if err != nil {
			t.Errorf("signature %d: %v", e.tag, err)
		}
		found[e.tag] = true
	}
	if !found[sigRSA] || !found[sigPGP] {
		t.Errorf("the rpm's signatures: %v", found)
	}
	// Still an rpm, with its own digests intact.
	p, err := rpm.Read(bytes.NewReader(data))
	if err != nil || p.Name() != "vero-example" {
		t.Fatalf("the signed rpm doesn't read: %v", err)
	}

	// repomd.xml lists the three indexes, with their checksums, and is
	// signed.
	repomd, _ := os.ReadFile(filepath.Join(site, "rpm", "repodata", "repomd.xml"))
	asc, _ := os.ReadFile(filepath.Join(site, "rpm", "repodata", "repomd.xml.asc"))
	if _, err := openpgp.CheckArmoredDetachedSignature(ring, bytes.NewReader(repomd), bytes.NewReader(asc), nil); err != nil {
		t.Errorf("repomd.xml's signature: %v", err)
	}
	var md struct {
		Data []struct {
			Type     string `xml:"type,attr"`
			Checksum string `xml:"checksum"`
			Location struct {
				Href string `xml:"href,attr"`
			} `xml:"location"`
		} `xml:"data"`
	}
	if err := xml.Unmarshal(repomd, &md); err != nil || len(md.Data) != 3 {
		t.Fatalf("repomd.xml: %v %+v", err, md)
	}
	var primary []byte
	for _, d := range md.Data {
		gz, err := os.ReadFile(filepath.Join(site, "rpm", d.Location.Href))
		if err != nil {
			t.Fatal(err)
		}
		if sum := sha256.Sum256(gz); hex.EncodeToString(sum[:]) != d.Checksum {
			t.Errorf("%s's checksum doesn't match", d.Type)
		}
		if d.Type == "primary" {
			z, _ := gzip.NewReader(bytes.NewReader(gz))
			primary, _ = io.ReadAll(z)
		}
	}
	text := string(primary)
	for _, want := range []string{
		`packages="2"`, `<name>vero-example</name>`, `<version epoch="0" ver="1.2.3~beta" rel="1">`,
		`<location href="packages/vero-example-1.2.3~beta-1.x86_64.rpm">`,
		`<rpm:entry name="python3" flags="GE" epoch="0" ver="3.10"`,
		`<rpm:entry name="(gtk4 if foo else gtk3)">`,
		`<file>/usr/bin/vero-example</file>`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("primary.xml lacks %s", want)
		}
	}
	if strings.Contains(text, "rpmlib(") {
		t.Error("primary.xml lists rpm's own requirements")
	}
	var parsed struct {
		Packages []struct {
			Checksum string `xml:"checksum"`
		} `xml:"package"`
	}
	if err := xml.Unmarshal(primary, &parsed); err != nil || len(parsed.Packages) != 2 {
		t.Fatalf("primary.xml: %v", err)
	}
}

func bsdApp(t *testing.T) (*App, map[string]string) {
	a, workers := linuxApp(t)
	a.GTK.FreeBSD = &BSDPkg{Deps: map[string]string{"gtk4": "x11-toolkits/gtk40"}}
	a.GTK.NetBSD = &Pkgsrc{Depends: []string{"gtk4-[0-9]*"}, Python: "python3.12"}
	a.GTK.OpenBSD = &OpenBSD{Depends: []string{"x11/gtk+4:gtk+4-*:gtk+4-4.0"}}
	return a, workers
}

func TestFreeBSD(t *testing.T) {
	a, workers := bsdApp(t)
	sys := system("freebsd")
	pkgs := t.TempDir()
	if err := packageUnix(a, sys, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(pkgs, "vero-example-1.2.3.beta-freebsd-aarch64.pkg")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// +COMPACT_MANIFEST first, then +MANIFEST, which lists each file's
	// SHA-256, then the files at their paths.
	z, _ := xz.NewReader(bytes.NewReader(data))
	tr := tar.NewReader(z)
	var names []string
	var manifest map[string]any
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, h.Name)
		if h.Name == "+MANIFEST" {
			json.NewDecoder(tr).Decode(&manifest)
		}
	}
	if len(names) < 3 || names[0] != "+COMPACT_MANIFEST" || names[1] != "+MANIFEST" {
		t.Fatalf("the package holds %v", names)
	}
	files, _ := manifest["files"].(map[string]any)
	if !strings.HasPrefix(files["/usr/local/bin/vero-example"].(string), "1$") || manifest["abi"] != "FreeBSD:*:aarch64" || manifest["prefix"] != "/usr/local" {
		t.Errorf("the manifest: %v", manifest)
	}
	if deps := manifest["deps"].(map[string]any); deps["gtk4"].(map[string]any)["origin"] != "x11-toolkits/gtk40" {
		t.Errorf("the dependencies: %v", deps)
	}

	s := testKey(t)
	site := t.TempDir()
	newest, err := buildPkgRepo(site, sys, []string{file, filepath.Join(pkgs, "vero-example-1.2.3.beta-freebsd-amd64.pkg")}, 3, "https://example.com/x", a, s)
	if err != nil {
		t.Fatal(err)
	}
	if newest["aarch64"].URL != "freebsd/aarch64/All/vero-example-1.2.3.beta.pkg" {
		t.Errorf("the newest: %+v", newest)
	}
	conf, _ := os.ReadFile(filepath.Join(site, "freebsd", "vero-example.conf"))
	if !strings.Contains(string(conf), `url: "https://example.com/x/freebsd/${ARCH}"`) || !strings.Contains(string(conf), `signature_type: "pubkey"`) {
		t.Errorf("the repository's conf:\n%s", conf)
	}
	block, _ := pem.Decode(mustRead(t, filepath.Join(site, "freebsd", "key.pem")))
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	// packagesite.pkg: the signature, then the index, which the signature
	// checks as pkg checks it.
	z, _ = xz.NewReader(bytes.NewReader(mustRead(t, filepath.Join(site, "freebsd", "aarch64", "packagesite.pkg"))))
	tr = tar.NewReader(z)
	members := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		members[h.Name], _ = io.ReadAll(tr)
	}
	index, sig := members["packagesite.yaml"], members["signature"]
	if len(index) == 0 || len(sig) == 0 || sig[len(sig)-1] != 0 {
		t.Fatalf("packagesite.pkg holds %d bytes of index and %d of signature", len(index), len(sig))
	}
	// As FreeBSD's pkg checks it: the SHA-256 of the index's SHA-256 in hex.
	sum := sha256.Sum256(index)
	hashed := sha256.Sum256([]byte(hex.EncodeToString(sum[:])))
	if err := rsa.VerifyPKCS1v15(key.(*rsa.PublicKey), crypto.SHA256, hashed[:], sig[:len(sig)-1]); err != nil {
		t.Errorf("packagesite's signature: %v", err)
	}
	// And DragonFly's, as older pkg checks it.
	old, err := s.pkgSign(index, true)
	if err != nil {
		t.Fatal(err)
	}
	info := append([]byte{0x30, 0x4d, 0x30, 0x09, 0x06, 0x05, 0x2b, 0x0e, 0x03, 0x02, 0x1a, 0x05, 0x00, 0x04, 0x40}, []byte(hex.EncodeToString(sum[:]))...)
	if err := rsa.VerifyPKCS1v15(key.(*rsa.PublicKey), crypto.Hash(0), info, old[:len(old)-1]); err != nil {
		t.Errorf("the older kind of signature: %v", err)
	}
	var entry map[string]any
	json.Unmarshal(bytes.TrimSpace(index), &entry)
	if entry["path"] != "All/vero-example-1.2.3.beta.pkg" || entry["sum"] == nil || entry["files"] != nil {
		t.Errorf("packagesite's entry: %v", entry)
	}
	pkgSum := sha256.Sum256(mustRead(t, filepath.Join(site, "freebsd", "aarch64", "All", "vero-example-1.2.3.beta.pkg")))
	if entry["sum"] != hex.EncodeToString(pkgSum[:]) {
		t.Error("packagesite's sum isn't the package's")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPkgsrc(t *testing.T) {
	a, workers := bsdApp(t)
	sys := system("netbsd")
	pkgs := t.TempDir()
	if err := packageUnix(a, sys, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(pkgs, "vero-example-1.2.3.beta-netbsd-aarch64.tgz")
	z, err := gzip.NewReader(bytes.NewReader(mustRead(t, file)))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(z)
	var names []string
	var contents string
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, h.Name)
		if h.Name == "+CONTENTS" {
			b, _ := io.ReadAll(tr)
			contents = string(b)
		}
	}
	if strings.Join(names[:5], " ") != "+CONTENTS +COMMENT +DESC +BUILD_INFO +SIZE_PKG" {
		t.Fatalf("the package starts %v", names[:5])
	}
	// The files in the packing list's order, each with its MD5.
	var listed []string
	sc := bufio.NewScanner(strings.NewReader(contents))
	for sc.Scan() {
		if line := sc.Text(); line != "" && line[0] != '@' {
			listed = append(listed, line)
		}
	}
	if strings.Join(listed, "|") != strings.Join(names[5:], "|") {
		t.Errorf("the packing list and the archive disagree:\n%v\n%v", listed, names[5:])
	}
	for _, want := range []string{"@name vero-example-1.2.3.beta\n", "@pkgdep gtk4-[0-9]*\n", "@cwd /usr/pkg\n", "bin/vero-example\n@comment MD5:", "@pkgdir lib/vero-example\n"} {
		if !strings.Contains(contents, want) {
			t.Errorf("+CONTENTS lacks %q", want)
		}
	}
	site := t.TempDir()
	if _, err := buildPkgsrcRepo(site, sys, []string{file}, 3, a, testKey(t)); err != nil {
		t.Fatal(err)
	}
	z, _ = gzip.NewReader(bytes.NewReader(mustRead(t, filepath.Join(site, "netbsd", "aarch64", "All", "pkg_summary.gz"))))
	summary, _ := io.ReadAll(z)
	for _, want := range []string{"PKGNAME=vero-example-1.2.3.beta\n", "DEPENDS=gtk4-[0-9]*\n", "MACHINE_ARCH=aarch64\n", "OPSYS=NetBSD\n", "FILE_NAME=vero-example-1.2.3.beta.tgz\n"} {
		if !strings.Contains(string(summary), want) {
			t.Errorf("pkg_summary lacks %q:\n%s", want, summary)
		}
	}
	launcher := mustReadFromTgz(t, file, "bin/vero-example")
	if !strings.Contains(launcher, "exec /usr/pkg/bin/python3.12 /usr/pkg/lib/vero-example/main.py") {
		t.Errorf("the launcher: %q", launcher)
	}
	// The desktop entry too, by its whole path: a session's PATH may leave
	// out pkgsrc's bin.
	if entry := mustReadFromTgz(t, file, "share/applications/dev.vero.example.desktop"); !strings.Contains(entry, "Exec=/usr/pkg/bin/vero-example") {
		t.Errorf("the desktop entry: %q", entry)
	}
}

func mustReadFromTgz(t *testing.T, file, name string) string {
	t.Helper()
	z, _ := gzip.NewReader(bytes.NewReader(mustRead(t, file)))
	tr := tar.NewReader(z)
	for {
		h, err := tr.Next()
		if err != nil {
			t.Fatalf("%s has no %s", file, name)
		}
		if h.Name == name {
			b, _ := io.ReadAll(tr)
			return string(b)
		}
	}
}

func TestOpenBSD(t *testing.T) {
	a, workers := bsdApp(t)
	sys := system("openbsd")
	pkgs := t.TempDir()
	if err := packageUnix(a, sys, workers, pkgs); err != nil {
		t.Fatal(err)
	}
	s := testKey(t)
	site := t.TempDir()
	if _, err := buildOpenBSDRepo(site, sys, []string{filepath.Join(pkgs, "vero-example-1.2.3.beta-openbsd-amd64.tgz")}, 3, a, s); err != nil {
		t.Fatal(err)
	}
	signed := mustRead(t, filepath.Join(site, "openbsd", "amd64", "vero-example-1.2.3.beta.tgz"))
	if signed[3] != 16 {
		t.Fatalf("the gzip header's flags are %d, not a comment's", signed[3])
	}
	end := bytes.IndexByte(signed[10:], 0) + 10
	comment := string(signed[10:end])
	lines := strings.SplitN(comment, "\n", 3)
	if lines[0] != "untrusted comment: verify with vero-example-pkg.pub" {
		t.Errorf("the comment starts %q", lines[0])
	}
	raw, _ := base64.StdEncoding.DecodeString(lines[1])
	pub := strings.Split(string(mustRead(t, filepath.Join(site, "openbsd", "vero-example-pkg.pub"))), "\n")
	pubRaw, _ := base64.StdEncoding.DecodeString(pub[1])
	if string(raw[:2]) != "Ed" || !bytes.Equal(raw[2:10], pubRaw[2:10]) {
		t.Fatal("the signature doesn't name the public key's number")
	}
	msg := lines[2]
	if !ed25519.Verify(pubRaw[10:], []byte(msg), raw[10:]) {
		t.Error("the signature doesn't verify")
	}
	if !strings.Contains(msg, "key=vero-example-pkg.sec\nalgorithm=SHA512/256\nblocksize=65536\n\n") {
		t.Errorf("the signed message: %q", msg[:120])
	}
	// The hashes are of the stream after the header, block by block.
	body := signed[end+1:]
	hashes := strings.Fields(msg[strings.Index(msg, "\n\n")+2:])
	for i, h := range hashes {
		from, to := i*65536, min((i+1)*65536, len(body))
		if sum := sha512.Sum512_256(body[from:to]); hex.EncodeToString(sum[:]) != h {
			t.Errorf("block %d's hash doesn't match", i)
		}
	}
	// Still a gzip file, of +CONTENTS, +DESC and the files.
	z, err := gzip.NewReader(bytes.NewReader(signed))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(z)
	h, _ := tr.Next()
	contents, _ := io.ReadAll(tr)
	if h.Name != "+CONTENTS" {
		t.Fatalf("the package starts with %s", h.Name)
	}
	for _, want := range []string{"@name vero-example-1.2.3.beta\n", "@comment pkgpath=misc/vero-example ftp=yes\n", "@arch amd64\n", "+DESC\n@sha ",
		"@depend x11/gtk+4:gtk+4-*:gtk+4-4.0\n", "@cwd /usr/local\n", "@bin bin/vero-example\n@sha "} {
		if !strings.Contains(string(contents), want) {
			t.Errorf("+CONTENTS lacks %q", want)
		}
	}
	listing := string(mustRead(t, filepath.Join(site, "openbsd", "amd64", "index.html")))
	if !strings.Contains(listing, `<a href="vero-example-1.2.3.beta.tgz">`) {
		t.Errorf("the listing: %s", listing)
	}
}
