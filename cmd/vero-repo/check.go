package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/clearsign"
	"github.com/cavaliergopher/rpm"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// check verifies a site before it's uploaded, with nothing but the site's
// files: that each repository's signatures check out with the keys the
// site publishes, that every index's sizes and hashes match the files it
// lists, that latest.json, the appcast, the winget manifests and the page
// point at files that are there, that each package holds the worker and
// the front end, and that no system's newest version is older than the
// one the site had before, which would leave nobody offered the update.
type checker struct {
	site, url string
	a         *App
	// sparkle is the public key Mac updates are checked with, if known.
	sparkle  ed25519.PublicKey
	keyring  openpgp.EntityList
	problems []string
	checked  []string
}

func (c *checker) fail(f string, args ...any) {
	c.problems = append(c.problems, fmt.Sprintf(f, args...))
}
func (c *checker) ok(what string) { c.checked = append(c.checked, what) }

// file is a file of the site, by its path in it.
func (c *checker) file(rel string) ([]byte, bool) {
	data, err := os.ReadFile(filepath.Join(c.site, filepath.FromSlash(rel)))
	if err != nil {
		c.fail("%s is missing", rel)
		return nil, false
	}
	return data, true
}

// local is the path in the site that a URL on it points at, or "".
func (c *checker) local(u string) string {
	if c.url == "" || !strings.HasPrefix(u, c.url+"/") {
		return ""
	}
	return strings.TrimPrefix(u, c.url+"/")
}

func checkCommand(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	appPath := fs.String("app", "vero-app.toml", "the app's vero-app.toml")
	url := fs.String("url", "", "where the site will be, as given to build")
	keyDir := fs.String("key", "", "the signing key's folder: with it, the Mac appcast's signatures are checked too")
	fs.Parse(args)
	site := "dist/site"
	if fs.NArg() > 0 {
		site = fs.Arg(0)
	}
	if *url == "" {
		return errors.New("check needs --url, where the site will be")
	}
	a, err := LoadApp(*appPath)
	if err != nil {
		return err
	}
	var sparkle ed25519.PublicKey
	if *keyDir != "" {
		s, err := loadKey(*keyDir)
		if err != nil {
			return err
		}
		if k, err := s.sparkle(); err == nil {
			sparkle = k.public
		}
	}
	return checkSite(site, *url, a, sparkle, nil)
}

// checkSite checks the site in site, as it will be at url. previous is
// latest.json as it was before this build, if there was one.
func checkSite(site, url string, a *App, sparkle ed25519.PublicKey, previous *Latest) error {
	c := &checker{site: site, url: strings.TrimRight(url, "/"), a: a, sparkle: sparkle}
	if sparkle == nil && a.MacOS != nil && a.MacOS.SparklePublicKey != "" {
		if k, err := base64.StdEncoding.DecodeString(a.MacOS.SparklePublicKey); err == nil && len(k) == ed25519.PublicKeySize {
			c.sparkle = k
		}
	}
	if data, ok := c.file(publicFile); ok {
		kr, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(data))
		if err != nil {
			c.fail("%s isn't a public key: %v", publicFile, err)
		}
		c.keyring = kr
	}
	var latest Latest
	if data, ok := c.file("latest.json"); ok {
		if err := json.Unmarshal(data, &latest); err != nil {
			c.fail("latest.json: %v", err)
		}
	}
	for key, d := range latest.Downloads {
		rel := c.local(d.URL)
		if rel == "" {
			c.fail("latest.json: %s points at %s, which isn't on the site at %s", key, d.URL, c.url)
			continue
		}
		if _, err := os.Stat(filepath.Join(site, filepath.FromSlash(rel))); err != nil {
			c.fail("latest.json: %s points at %s, which is missing", key, rel)
		}
		if previous != nil {
			if old, ok := previous.Downloads[key]; ok && compareVersions(d.Version, old.Version) < 0 {
				c.fail("%s goes back from %s to %s: copies already installed wouldn't be offered it", key, old.Version, d.Version)
			}
		}
	}
	if len(latest.Downloads) > 0 {
		c.ok("latest.json")
	}
	exists := func(rel string) bool { _, err := os.Stat(filepath.Join(site, rel)); return err == nil }
	if exists("apt") {
		c.checkApt()
	}
	if exists("rpm") {
		c.checkRPM()
	}
	if exists("arch") {
		c.checkPacman()
	}
	if exists("alpine") {
		c.checkAlpine()
	}
	if exists("void") {
		c.checkVoid()
	}
	for _, sys := range unixSystems {
		if !exists(sys.name) {
			continue
		}
		switch sys.format {
		case "pkg":
			c.checkPkg(sys)
		case "pkgsrc":
			c.checkPkgsrc(sys)
		case "openbsd":
			c.checkOpenBSD(sys)
		}
	}
	if exists("flatpak") {
		c.checkFlatpak()
	}
	if exists("macos") {
		c.checkMac(latest)
	}
	if exists("winget") {
		c.checkWinget()
	}
	c.checkBundles(latest)
	if exists("index.html") {
		c.checkPage()
	}
	if len(c.problems) > 0 {
		return fmt.Errorf("the site isn't ready to upload:\n  %s", strings.Join(c.problems, "\n  "))
	}
	fmt.Printf("checked %s: %s\n", site, strings.Join(c.checked, ", "))
	return nil
}

// verify checks an armored detached signature of data with the site's key.
func (c *checker) verify(what string, data, sig []byte) {
	if c.keyring == nil {
		return
	}
	if _, err := openpgp.CheckArmoredDetachedSignature(c.keyring, bytes.NewReader(data), bytes.NewReader(sig), nil); err != nil {
		c.fail("%s: the signature doesn't check out with key.asc: %v", what, err)
	}
}

// sum checks that data is size bytes long with this SHA-256.
func (c *checker) sum(what string, data []byte, size int64, sha string) {
	got := sha256.Sum256(data)
	if size >= 0 && int64(len(data)) != size {
		c.fail("%s is %d bytes, but its index says %d", what, len(data), size)
	}
	if !strings.EqualFold(hex.EncodeToString(got[:]), sha) {
		c.fail("%s doesn't match the hash its index has for it", what)
	}
}

func (c *checker) checkApt() {
	dist := "apt/dists/stable"
	release, ok := c.file(dist + "/Release")
	if !ok {
		return
	}
	if sig, ok := c.file(dist + "/Release.gpg"); ok {
		c.verify(dist+"/Release.gpg", release, sig)
	}
	if in, ok := c.file(dist + "/InRelease"); ok && c.keyring != nil {
		block, _ := clearsign.Decode(in)
		if block == nil {
			c.fail("%s/InRelease isn't signed", dist)
		} else if _, err := openpgp.CheckDetachedSignature(c.keyring, bytes.NewReader(block.Bytes), block.ArmoredSignature.Body, nil); err != nil {
			c.fail("%s/InRelease: the signature doesn't check out with key.asc: %v", dist, err)
		}
	}
	// Release's SHA256 section: each index, its size, its hash.
	inSHA := false
	var packages []string
	for _, line := range strings.Split(string(release), "\n") {
		if !strings.HasPrefix(line, " ") {
			inSHA = strings.HasPrefix(line, "SHA256:")
			continue
		}
		f := strings.Fields(line)
		if !inSHA || len(f) != 3 {
			continue
		}
		size, _ := strconv.ParseInt(f[1], 10, 64)
		data, ok := c.file(dist + "/" + f[2])
		if !ok {
			continue
		}
		c.sum(dist+"/"+f[2], data, size, f[0])
		if path.Base(f[2]) == "Packages" {
			packages = append(packages, dist+"/"+f[2])
		}
	}
	for _, p := range packages {
		data, _ := c.file(p)
		var newest, newestFile string
		for _, stanza := range strings.Split(string(data), "\n\n") {
			fields := map[string]string{}
			for _, line := range strings.Split(stanza, "\n") {
				if k, v, ok := strings.Cut(line, ": "); ok && !strings.HasPrefix(line, " ") {
					fields[k] = v
				}
			}
			if fields["Filename"] == "" {
				continue
			}
			size, _ := strconv.ParseInt(fields["Size"], 10, 64)
			deb, ok := c.file("apt/" + fields["Filename"])
			if !ok {
				continue
			}
			c.sum("apt/"+fields["Filename"], deb, size, fields["SHA256"])
			if newest == "" || compareVersions(fields["Version"], newest) > 0 {
				newest, newestFile = fields["Version"], fields["Filename"]
			}
		}
		if newestFile != "" {
			deb, _ := c.file("apt/" + newestFile)
			c.contents("apt/"+newestFile, debFiles(deb), "/usr/lib/"+c.a.Name+"/")
		}
	}
	c.ok("the apt repository")
}

// contents checks that a package holds the worker and the front end's
// entry, in lib, under prefix.
func (c *checker) contents(what string, files []string, lib string) {
	if files == nil {
		c.fail("%s can't be read", what)
		return
	}
	want := []string{lib + c.a.Worker.Name}
	if c.a.GTK != nil {
		want = append(want, lib+c.a.GTK.Entry)
	}
	have := map[string]bool{}
	for _, f := range files {
		have["/"+strings.TrimPrefix(strings.TrimPrefix(f, "."), "/")] = true
	}
	for _, w := range want {
		if !have[w] {
			c.fail("%s doesn't hold %s", what, w)
		}
	}
}

// debFiles are the files a .deb installs.
func debFiles(deb []byte) []string {
	data, err := arMember(deb, "data.tar")
	if err != nil {
		return nil
	}
	return tarNames(data)
}

// arMember is the member of an ar archive whose name starts with prefix,
// uncompressed.
func arMember(b []byte, prefix string) ([]byte, error) {
	if !bytes.HasPrefix(b, []byte("!<arch>\n")) {
		return nil, errors.New("not an ar archive")
	}
	pos := 8
	for pos+60 <= len(b) {
		name := strings.TrimRight(strings.TrimSpace(string(b[pos:pos+16])), "/")
		size, err := strconv.Atoi(strings.TrimSpace(string(b[pos+48 : pos+58])))
		if err != nil || pos+60+size > len(b) {
			return nil, errors.New("a damaged ar archive")
		}
		body := b[pos+60 : pos+60+size]
		if strings.HasPrefix(name, prefix) {
			return decompress(name, body)
		}
		pos += 60 + size + size%2
	}
	return nil, fmt.Errorf("no %s", prefix)
}

func decompress(name string, body []byte) ([]byte, error) {
	switch {
	case strings.HasSuffix(name, ".gz"):
		r, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(r)
	case strings.HasSuffix(name, ".xz"):
		r, err := xz.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(r)
	}
	return body, nil
}

// tarNames are the names in a tar.
func tarNames(data []byte) []string {
	var names []string
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		h, err := tr.Next()
		if err != nil {
			return names
		}
		names = append(names, h.Name)
	}
}

func (c *checker) checkRPM() {
	md, ok := c.file("rpm/repodata/repomd.xml")
	if !ok {
		return
	}
	if sig, ok := c.file("rpm/repodata/repomd.xml.asc"); ok {
		c.verify("rpm/repodata/repomd.xml.asc", md, sig)
	}
	var repomd struct {
		Data []struct {
			Type     string `xml:"type,attr"`
			Checksum string `xml:"checksum"`
			Size     int64  `xml:"size"`
			Location struct {
				Href string `xml:"href,attr"`
			} `xml:"location"`
		} `xml:"data"`
	}
	if err := xml.Unmarshal(md, &repomd); err != nil {
		c.fail("rpm/repodata/repomd.xml: %v", err)
		return
	}
	for _, d := range repomd.Data {
		data, ok := c.file("rpm/" + d.Location.Href)
		if !ok {
			continue
		}
		c.sum("rpm/"+d.Location.Href, data, d.Size, d.Checksum)
		if d.Type != "primary" {
			continue
		}
		raw, err := decompress(d.Location.Href, data)
		if err != nil {
			c.fail("rpm/%s: %v", d.Location.Href, err)
			continue
		}
		var primary struct {
			Packages []struct {
				Version struct {
					Ver string `xml:"ver,attr"`
					Rel string `xml:"rel,attr"`
				} `xml:"version"`
				Checksum string `xml:"checksum"`
				Size     struct {
					Package int64 `xml:"package,attr"`
				} `xml:"size"`
				Location struct {
					Href string `xml:"href,attr"`
				} `xml:"location"`
			} `xml:"package"`
		}
		if err := xml.Unmarshal(raw, &primary); err != nil {
			c.fail("rpm/%s: %v", d.Location.Href, err)
			continue
		}
		for _, p := range primary.Packages {
			data, ok := c.file("rpm/" + p.Location.Href)
			if !ok {
				continue
			}
			c.sum("rpm/"+p.Location.Href, data, p.Size.Package, p.Checksum)
			pkg, err := rpm.Read(bytes.NewReader(data))
			if err != nil {
				c.fail("rpm/%s: %v", p.Location.Href, err)
				continue
			}
			var names []string
			for _, f := range pkg.Files() {
				names = append(names, f.Name())
			}
			c.contents("rpm/"+p.Location.Href, names, "/usr/lib/"+c.a.Name+"/")
		}
	}
	c.ok("the rpm repository")
}

func (c *checker) checkPkg(sys unixSystem) {
	pemData, ok := c.file(sys.name + "/key.pem")
	if !ok {
		return
	}
	block, _ := pem.Decode(pemData)
	var pub *rsa.PublicKey
	if block != nil {
		if k, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
			pub, _ = k.(*rsa.PublicKey)
		}
	}
	if pub == nil {
		c.fail("%s/key.pem isn't an RSA public key", sys.name)
		return
	}
	for _, arch := range sys.arches {
		dir := filepath.ToSlash(filepathRel(c.site, pkgRepoDir(c.site, &sys, arch)))
		archive, ok := c.file(dir + "/packagesite.pkg")
		if !ok {
			continue
		}
		members := map[string][]byte{}
		z, err := xz.NewReader(bytes.NewReader(archive))
		if err != nil {
			c.fail("%s/packagesite.pkg: %v", dir, err)
			continue
		}
		tr := tar.NewReader(z)
		for {
			h, err := tr.Next()
			if err != nil {
				break
			}
			members[h.Name], _ = io.ReadAll(tr)
		}
		manifests, sig := members["packagesite.yaml"], bytes.TrimRight(members["signature"], "\x00")
		sum := sha256.Sum256(manifests)
		hashed := sha256.Sum256([]byte(hex.EncodeToString(sum[:])))
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hashed[:], sig); err != nil {
			c.fail("%s/packagesite.pkg: the signature doesn't check out with key.pem", dir)
		}
		sc := bufio.NewScanner(bytes.NewReader(manifests))
		sc.Buffer(nil, 1<<24)
		for sc.Scan() {
			var m struct {
				Path    string `json:"repopath"`
				Sum     string `json:"sum"`
				PkgSize int64  `json:"pkgsize"`
			}
			if json.Unmarshal(sc.Bytes(), &m) != nil || m.Path == "" {
				continue
			}
			if data, ok := c.file(dir + "/" + m.Path); ok {
				c.sum(dir+"/"+m.Path, data, m.PkgSize, m.Sum)
			}
		}
	}
	c.ok("the " + sys.label + " repository")
}

func (c *checker) checkPkgsrc(sys unixSystem) {
	summaries, _ := filepath.Glob(filepath.Join(c.site, sys.name, "*", "All", "pkg_summary.gz"))
	more, _ := filepath.Glob(filepath.Join(c.site, sys.name, "All", "pkg_summary.gz"))
	for _, s := range append(summaries, more...) {
		rel := filepathRel(c.site, s)
		gz, _ := os.ReadFile(s)
		data, err := decompress(".gz", gz)
		if err != nil {
			c.fail("%s: %v", rel, err)
			continue
		}
		for _, entry := range strings.Split(string(data), "\n\n") {
			fields := map[string]string{}
			for _, line := range strings.Split(entry, "\n") {
				if k, v, ok := strings.Cut(line, "="); ok {
					fields[k] = v
				}
			}
			if fields["FILE_NAME"] == "" {
				continue
			}
			size, _ := strconv.ParseInt(fields["FILE_SIZE"], 10, 64)
			if pkg, ok := c.file(path.Join(path.Dir(filepath.ToSlash(rel)), fields["FILE_NAME"])); ok {
				c.sum(path.Join(path.Dir(filepath.ToSlash(rel)), fields["FILE_NAME"]), pkg, size, strings.TrimPrefix(fields["FILE_CKSUM"], "sha256 "))
			}
		}
	}
	c.ok("the " + sys.label + " repository")
}

func (c *checker) checkOpenBSD(sys unixSystem) {
	pubFile, ok := c.file(path.Join(sys.name, c.a.Name+"-pkg.pub"))
	if !ok {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(pubFile)), "\n")
	raw, err := base64.StdEncoding.DecodeString(lines[len(lines)-1])
	if err != nil || len(raw) != 42 {
		c.fail("%s/%s-pkg.pub isn't a signify key", sys.name, c.a.Name)
		return
	}
	pub := ed25519.PublicKey(raw[10:])
	pkgs, _ := filepath.Glob(filepath.Join(c.site, sys.name, "*", "*.tgz"))
	for _, p := range pkgs {
		data, _ := os.ReadFile(p)
		if err := verifySignifyGzip(data, raw[2:10], pub); err != nil {
			c.fail("%s: %v", filepathRel(c.site, p), err)
		}
	}
	c.ok("the " + sys.label + " repository")
}

// verifySignifyGzip checks a gzip signify -zS signed: the signature in the
// header's comment, of the block hashes after it, which have to match the
// stream that follows.
func verifySignifyGzip(gz, number []byte, pub ed25519.PublicKey) error {
	if len(gz) < 10 || gz[3]&16 == 0 {
		return errors.New("not signed")
	}
	end := bytes.IndexByte(gz[10:], 0)
	if end < 0 {
		return errors.New("a damaged signature")
	}
	comment, body := string(gz[10:10+end]), gz[10+end+1:]
	parts := strings.SplitN(comment, "\n", 3)
	if len(parts) != 3 {
		return errors.New("a damaged signature")
	}
	sig, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil || len(sig) != 74 || !bytes.Equal(sig[2:10], number) {
		return errors.New("signed with another key")
	}
	msg := []byte(parts[2])
	if !ed25519.Verify(pub, msg, sig[10:]) {
		return errors.New("the signature doesn't check out")
	}
	head, hashes, _ := strings.Cut(parts[2], "\n\n")
	size := 65536
	for _, line := range strings.Split(head, "\n") {
		if v, ok := strings.CutPrefix(line, "blocksize="); ok {
			size, _ = strconv.Atoi(v)
		}
	}
	want := strings.Fields(hashes)
	for i, n := 0, 0; i < len(body); i, n = i+size, n+1 {
		sum := sha512.Sum512_256(body[i:min(i+size, len(body))])
		if n >= len(want) || want[n] != hex.EncodeToString(sum[:]) {
			return errors.New("doesn't match its signed hashes")
		}
	}
	return nil
}

func (c *checker) checkFlatpak() {
	if _, ok := c.file("flatpak/repo/summary"); !ok {
		return
	}
	refs, _ := filepath.Glob(filepath.Join(c.site, "flatpak", "*.flatpakref"))
	if len(refs) == 0 {
		c.fail("flatpak has no .flatpakref")
	}
	// The repository's own signatures are made, and checked, by ostree in
	// vero's tools container when the site is built.
	c.ok("the Flatpak repository's files")
}

// macDisk matches a disk image's name, NAME-VERSION-macos.dmg, by the
// app's name when the check knows it, and by the pattern otherwise.
func (c *checker) macDisk(name string) []string {
	if c.a != nil {
		return macDisk(c.a.Name).FindStringSubmatch(name)
	}
	return regexp.MustCompile(`^.+-(.+)-macos\.dmg$`).FindStringSubmatch(name)
}

func (c *checker) checkMac(latest Latest) {
	data, ok := c.file("macos/appcast.xml")
	if !ok {
		return
	}
	// The appcast apps already installed read, when it is somewhere else
	// too, is the same.
	if c.a != nil && c.a.MacOS != nil && c.a.MacOS.Appcast != "" {
		if other, ok := c.file(c.a.MacOS.Appcast); ok && !bytes.Equal(other, data) {
			c.fail("%s isn't the same as macos/appcast.xml", c.a.MacOS.Appcast)
		}
	}
	var cast struct {
		Items []struct {
			Version      string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle version"`
			ShortVersion string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString"`
			Notes        string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle releaseNotesLink"`
			Enclosure    struct {
				URL       string `xml:"url,attr"`
				Length    int64  `xml:"length,attr"`
				Signature string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle edSignature,attr"`
			} `xml:"enclosure"`
		} `xml:"channel>item"`
	}
	if err := xml.Unmarshal(data, &cast); err != nil {
		c.fail("macos/appcast.xml: %v", err)
		return
	}
	for i, item := range cast.Items {
		if i > 0 && compareVersions(item.Version, cast.Items[i-1].Version) >= 0 {
			c.fail("macos/appcast.xml: build %s isn't older than %s above it, so Sparkle wouldn't offer the newest", item.Version, cast.Items[i-1].Version)
		}
		if item.Notes != "" {
			if rel := c.local(item.Notes); rel != "" {
				c.file(rel)
			}
		}
		// The versions are the app's own: what package read from its
		// Info.plist, beside the disk image, and in its name.
		name := path.Base(item.Enclosure.URL)
		if m := c.macDisk(name); m != nil {
			if item.ShortVersion != m[1] {
				c.fail("macos/appcast.xml: %s says version %s, but the disk image is %s", name, item.ShortVersion, m[1])
			}
			if note, err := os.ReadFile(filepath.Join(c.site, "macos", name+".json")); err == nil {
				var info macInfo
				if json.Unmarshal(note, &info) == nil && (info.BundleVersion != item.Version || info.ShortVersion != item.ShortVersion) {
					c.fail("%s: the app inside is %s (build %s), but the appcast says %s (build %s)", name, info.ShortVersion, info.BundleVersion, item.ShortVersion, item.Version)
				}
			}
			if i == 0 {
				if d, ok := latest.Downloads["macos-universal"]; ok && d.Version != item.ShortVersion {
					c.fail("latest.json says the Mac's newest is %s, but the appcast's is %s", d.Version, item.ShortVersion)
				}
			}
		}
		rel := c.local(item.Enclosure.URL)
		if rel == "" {
			// Served from somewhere else, as download_url says.
			continue
		}
		dmg, ok := c.file(rel)
		if !ok {
			continue
		}
		if int64(len(dmg)) != item.Enclosure.Length {
			c.fail("%s is %d bytes, but the appcast says %d", rel, len(dmg), item.Enclosure.Length)
		}
		if c.sparkle != nil {
			sig, err := base64.StdEncoding.DecodeString(item.Enclosure.Signature)
			if err != nil || !ed25519.Verify(c.sparkle, dmg, sig) {
				c.fail("%s: the appcast's signature doesn't check out with the Sparkle key", rel)
			}
		}
	}
	if c.sparkle == nil {
		c.checked = append(c.checked, "the appcast (not its signatures: give --key, or sparkle_public_key in vero-app.toml)")
	} else {
		c.ok("the appcast")
	}
}

func (c *checker) checkWinget() {
	installers, _ := filepath.Glob(filepath.Join(c.site, "winget", "manifests", "*", "*", "*", "*", "*.installer.yaml"))
	urlLine := regexp.MustCompile(`InstallerUrl: "?([^"\n]+)"?\n\s*InstallerSha256: ([0-9A-Fa-f]{64})`)
	for _, f := range installers {
		data, _ := os.ReadFile(f)
		for _, m := range urlLine.FindAllStringSubmatch(string(data), -1) {
			rel := c.local(m[1])
			if rel == "" {
				c.fail("%s: %s isn't on the site", filepathRel(c.site, f), m[1])
				continue
			}
			if exe, ok := c.file(rel); ok {
				c.sum(rel, exe, -1, m[2])
			}
		}
	}
	c.ok("the winget manifests")
}

// checkBundles checks that the Android, WASI and Plan 9 bundles, and the
// web front end, hold what they should.
func (c *checker) checkBundles(latest Latest) {
	seen := map[string]bool{}
	for key, d := range latest.Downloads {
		switch kind, _, _ := strings.Cut(key, "-"); kind {
		case "android", "wasi", "plan9", "web":
			if !seen[kind] {
				seen[kind] = true
				c.ok(map[string]string{"android": "the Android app", "wasi": "the WASI bundles", "plan9": "the Plan 9 bundles", "web": "the web front end"}[kind])
			}
		}
		rel := c.local(d.URL)
		switch {
		case key == "android":
			data, ok := c.file(rel)
			if !ok {
				continue
			}
			z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil || !zipHas(z, "AndroidManifest.xml") {
				c.fail("%s isn't an Android app", rel)
			}
		case strings.HasPrefix(key, "wasi-"), strings.HasPrefix(key, "plan9-"):
			data, ok := c.file(rel)
			if !ok {
				continue
			}
			raw, err := decompress(".gz", data)
			if err != nil {
				c.fail("%s: %v", rel, err)
				continue
			}
			worker := c.a.Worker.Name
			if strings.HasPrefix(key, "wasi-") {
				worker = "worker.wasm"
			}
			names := map[string]bool{}
			for _, n := range tarNames(raw) {
				names[path.Base(n)] = true
			}
			front := c.a.Name
			if strings.HasPrefix(key, "wasi-windows") {
				front += ".exe"
			}
			for _, want := range []string{worker, front} {
				if !names[want] {
					c.fail("%s doesn't hold %s", rel, want)
				}
			}
		case key == "web" && c.a.Web != nil:
			for _, f := range c.a.Web.Files {
				c.file("web/" + f)
			}
		}
	}
}

func zipHas(z *zip.Reader, name string) bool {
	for _, f := range z.File {
		if f.Name == name {
			return true
		}
	}
	return false
}

// checkPage checks that every link on the page to the site is to a file
// that's there.
func (c *checker) checkPage() {
	data, _ := c.file("index.html")
	for _, m := range regexp.MustCompile(`(?:href|src)="([^"]+)"`).FindAllStringSubmatch(string(data), -1) {
		rel := c.local(m[1])
		if rel == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c.site, filepath.FromSlash(rel))); err != nil {
			c.fail("index.html links to %s, which is missing", rel)
		}
	}
	for _, m := range regexp.MustCompile(`(?m)(?:curl -fsSL|hget) (\S+)`).FindAllStringSubmatch(string(data), -1) {
		if rel := c.local(m[1]); rel != "" {
			if _, err := os.Stat(filepath.Join(c.site, filepath.FromSlash(rel))); err != nil {
				c.fail("index.html's commands fetch %s, which is missing", rel)
			}
		}
	}
	c.ok("the page's links")
}

// loadKeyring reads an armored public key, as key.asc is.
func loadKeyring(armored []byte) (openpgp.EntityList, error) {
	return openpgp.ReadArmoredKeyRing(bytes.NewReader(armored))
}

func sha512Sum(b []byte) [64]byte { return sha512.Sum512(b) }

// verifyBinary checks an unarmored detached signature of data, as pacman
// reads them, with the site's key.
func (c *checker) verifyBinary(what string, data, sig []byte) {
	if c.keyring == nil {
		return
	}
	if _, err := openpgp.CheckDetachedSignature(c.keyring, bytes.NewReader(data), bytes.NewReader(sig), nil); err != nil {
		c.fail("%s: the signature doesn't check out with key.asc: %v", what, err)
	}
}

// checkPacman checks each architecture's pacman repository: that its
// indexes and packages are signed, and that each package matches what
// the index says of it.
func (c *checker) checkPacman() {
	dirs, _ := filepath.Glob(filepath.Join(c.site, "arch", "*", c.a.Name+".db"))
	for _, db := range dirs {
		dir := filepathRel(c.site, filepath.Dir(db))
		for _, index := range []string{".db", ".files"} {
			data, ok := c.file(dir + "/" + c.a.Name + index)
			if !ok {
				continue
			}
			if sig, ok := c.file(dir + "/" + c.a.Name + index + ".sig"); ok {
				c.verifyBinary(dir+"/"+c.a.Name+index, data, sig)
			}
		}
		data, _ := c.file(dir + "/" + c.a.Name + ".db")
		descs, err := tarMembers(data)
		if err != nil {
			c.fail("%s/%s.db: %v", dir, c.a.Name, err)
			continue
		}
		for name, desc := range descs {
			if path.Base(name) != "desc" {
				continue
			}
			fields := pacmanFields(desc)
			file := dir + "/" + fields["FILENAME"]
			pkg, ok := c.file(file)
			if !ok {
				continue
			}
			size, _ := strconv.ParseInt(fields["CSIZE"], 10, 64)
			c.sum(file, pkg, size, fields["SHA256SUM"])
			sig, ok := c.file(file + ".sig")
			if !ok {
				continue
			}
			c.verifyBinary(file, pkg, sig)
			if fields["PGPSIG"] != base64.StdEncoding.EncodeToString(sig) {
				c.fail("%s.sig isn't the signature the index has for it", file)
			}
			_, files, err := readPacman(pkg)
			if err != nil {
				c.fail("%s: %v", file, err)
				continue
			}
			c.contents(file, files, "/usr/lib/"+c.a.Name+"/")
		}
	}
	c.ok("the Arch Linux repository")
}

// pacmanFields are the fields of a desc in a pacman index, each value's
// lines joined by newlines.
func pacmanFields(desc []byte) map[string]string {
	fields := map[string]string{}
	for _, block := range strings.Split(strings.TrimSpace(string(desc)), "\n\n") {
		name, value, _ := strings.Cut(block, "\n")
		fields[strings.Trim(name, "%")] = value
	}
	return fields
}

// tarMembers are the files in a tar, gzipped or not, by name.
func tarMembers(data []byte) (map[string][]byte, error) {
	var r io.Reader = bytes.NewReader(data)
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		z, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		r = z
	}
	tr := tar.NewReader(r)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg {
			out[h.Name], _ = io.ReadAll(tr)
		}
	}
}

// rsaPEM is the RSA public key in a site's PEM file, or nil.
func (c *checker) rsaPEM(rel string) *rsa.PublicKey {
	data, ok := c.file(rel)
	if !ok {
		return nil
	}
	if block, _ := pem.Decode(data); block != nil {
		if k, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
			if pub, ok := k.(*rsa.PublicKey); ok {
				return pub
			}
		}
	}
	c.fail("%s isn't an RSA public key", rel)
	return nil
}

// alpineSigned checks the signature segment at the start of data, an
// Alpine package or index, and returns the streams after it.
func (c *checker) alpineSigned(what string, data []byte, pub *rsa.PublicKey) [][]byte {
	streams, err := gzipStreams(data)
	if err != nil || len(streams) < 2 {
		c.fail("%s can't be read: %v", what, err)
		return nil
	}
	files, _, err := gunzipTar(streams[0])
	if err != nil {
		c.fail("%s can't be read: %v", what, err)
		return nil
	}
	sig, ok := files[".SIGN.RSA256."+alpineKeyName(c.a)]
	sum := sha256.Sum256(streams[1])
	if !ok || rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) != nil {
		c.fail("%s: the signature doesn't check out with alpine/%s", what, alpineKeyName(c.a))
	}
	return streams[1:]
}

// checkAlpine checks each architecture's Alpine repository: that the
// index and the packages are signed, and that each package is what the
// index says.
func (c *checker) checkAlpine() {
	pub := c.rsaPEM("alpine/" + alpineKeyName(c.a))
	if pub == nil {
		return
	}
	indexes, _ := filepath.Glob(filepath.Join(c.site, "alpine", "*", "APKINDEX.tar.gz"))
	for _, index := range indexes {
		dir := filepathRel(c.site, filepath.Dir(index))
		data, _ := c.file(dir + "/APKINDEX.tar.gz")
		streams := c.alpineSigned(dir+"/APKINDEX.tar.gz", data, pub)
		if streams == nil {
			continue
		}
		files, _, err := gunzipTar(streams[0])
		if err != nil {
			c.fail("%s/APKINDEX.tar.gz: %v", dir, err)
			continue
		}
		for _, entry := range strings.Split(strings.TrimSpace(string(files["APKINDEX"])), "\n\n") {
			fields := map[string]string{}
			for _, line := range strings.Split(entry, "\n") {
				if k, v, ok := strings.Cut(line, ":"); ok {
					fields[k] = v
				}
			}
			file := dir + "/" + fields["P"] + "-" + fields["V"] + ".apk"
			pkg, ok := c.file(file)
			if !ok {
				continue
			}
			if fields["S"] != strconv.Itoa(len(pkg)) {
				c.fail("%s is %d bytes, but its index says %s", file, len(pkg), fields["S"])
			}
			rest := c.alpineSigned(file, pkg, pub)
			if len(rest) != 2 {
				continue
			}
			control := sha1.Sum(rest[0])
			if fields["C"] != "Q1"+base64.StdEncoding.EncodeToString(control[:]) {
				c.fail("%s doesn't match the hash its index has for it", file)
			}
			info, _, err := apkInfo(pkg)
			data := sha256.Sum256(rest[1])
			if err != nil || pkgInfo(info, "datahash") != hex.EncodeToString(data[:]) {
				c.fail("%s: its files don't match its .PKGINFO", file)
			}
			names, _, err := gunzipTar(rest[1])
			if err != nil {
				c.fail("%s: %v", file, err)
				continue
			}
			var list []string
			for n := range names {
				list = append(list, n)
			}
			c.contents(file, list, "/usr/lib/"+c.a.Name+"/")
		}
	}
	c.ok("the Alpine repository")
}

// checkVoid checks the Void repository: each architecture's repodata
// carries the site's key, and each package it lists is there, matches
// its hash, and is signed with that key.
func (c *checker) checkVoid() {
	repodata, _ := filepath.Glob(filepath.Join(c.site, "void", "*-repodata"))
	for _, r := range repodata {
		rel := filepathRel(c.site, r)
		data, _ := c.file(rel)
		index, meta, err := readRepodata(data)
		if err != nil {
			c.fail("%s: %v", rel, err)
			continue
		}
		key, _ := meta["public-key"].([]byte)
		var pub *rsa.PublicKey
		if block, _ := pem.Decode(key); block != nil {
			if k, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
				pub, _ = k.(*rsa.PublicKey)
			}
		}
		if pub == nil {
			c.fail("%s carries no RSA public key", rel)
			continue
		}
		for _, v := range index {
			props, _ := v.(plistDict)
			pkgver, _ := props["pkgver"].(string)
			arch, _ := props["architecture"].(string)
			file := "void/" + pkgver + "." + arch + ".xbps"
			pkg, ok := c.file(file)
			if !ok {
				continue
			}
			size, _ := props["filename-size"].(int64)
			sha, _ := props["filename-sha256"].(string)
			c.sum(file, pkg, size, sha)
			if sig, ok := c.file(file + ".sig2"); ok {
				sum := sha256.Sum256(pkg)
				if rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) != nil {
					c.fail("%s.sig2: the signature doesn't check out with the repository's key", file)
				}
			}
			_, files, err := voidContents(pkg)
			if err != nil {
				c.fail("%s: %v", file, err)
				continue
			}
			c.contents(file, files, "/usr/lib/"+c.a.Name+"/")
		}
	}
	c.ok("the Void repository")
}

// readRepodata is a Void repository's index.plist and index-meta.plist.
func readRepodata(data []byte) (index, meta plistDict, err error) {
	z, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	defer z.Close()
	tr := tar.NewReader(z)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return nil, nil, err
		}
		switch h.Name {
		case "index.plist":
			index, err = parsePlist(body)
		case "index-meta.plist":
			meta, err = parsePlist(body)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", h.Name, err)
		}
	}
	if index == nil || meta == nil {
		return nil, nil, errors.New("no index.plist and index-meta.plist")
	}
	return index, meta, nil
}
