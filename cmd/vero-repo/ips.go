package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// illumos packages are IPS packages, from OpenIndiana's own repository's
// point of view: they depend on its GTK 4, libadwaita, PyGObject and
// Python, and install with its pkg.
//
// An IPS repository is usually served by pkg.depotd. vero's sites are plain
// files, so this writes the files at the very paths pkg asks a depot for,
// which any web server then serves:
//
//	versions/0/index.html          what the "depot" offers
//	publisher/0/index.html         its publisher
//	PUB/catalog/1/catalog.attrs    the catalog, and its parts
//	PUB/manifest/0/STEM@VERSION    each package's manifest
//	PUB/file/1/HASH                each file, gzipped, by the SHA-1 of its contents
//
// as pkg.depotd and pkgsend write them, signatures and all, checked against
// OpenIndiana's own on 12 October 2026.
//
// A package vero-repo package makes is a tar of its manifest and its
// files, NAME-VERSION-illumos-x86_64.ips, from which release makes the
// repository.

// ipsStem is the app's package name in IPS: application/NAME.
func ipsStem(a *App) string { return "application/" + a.Name }

// ipsPublisher is the publisher the app's packages come from: its id.
func ipsPublisher(a *App) string { return a.ID }

var ipsVersionRe = regexp.MustCompile(`^\d+(\.\d+)*$`)

// ipsCatalogTime is a time as pkg writes one in a catalog, which pkg
// reads into Python's datetime and writes again with isoformat before it
// checks the catalog's signature: with microseconds, unless they are
// none, when isoformat leaves them out.
func ipsCatalogTime(t time.Time) string {
	t = t.UTC()
	if t.Nanosecond()/1000 == 0 {
		return t.Format("20060102T150405Z")
	}
	return t.Format("20060102T150405") + fmt.Sprintf(".%06dZ", t.Nanosecond()/1000)
}

// ipsTime is a time as IPS writes one in an FMRI: 20261012T090553Z.
func ipsTime(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// ipsQuote is an action's attribute value, quoted where it must be. A
// value is on one line, as an action is, so line breaks become spaces.
func ipsQuote(v string) string {
	v = strings.Join(strings.Fields(v), " ")
	if v != "" && !strings.ContainsAny(v, " \t\"'=\\") {
		return v
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
}

// ipsDirs are directories that OpenIndiana's own packages deliver, with
// their own attributes, which the app's package leaves to them.
func ipsSystemDir(dir string) bool {
	return dir == "usr" || dir == "usr/bin" || dir == "usr/lib" || dir == "usr/share" ||
		strings.HasPrefix(dir, "usr/share/applications") || strings.HasPrefix(dir, "usr/share/icons") ||
		strings.HasPrefix(dir, "usr/share/metainfo")
}

// writeIPS writes the app's IPS package for arch: its manifest and its
// files, in a tar, at name.
func writeIPS(name string, a *App, sys *unixSystem, arch unixArch, files []treeFile) error {
	if !ipsVersionRe.MatchString(a.Version) {
		return fmt.Errorf("illumos's packages need a version of numbers and dots, such as 1.2.3, not %s", a.Version)
	}
	when := buildTime()
	fmri := fmt.Sprintf("pkg://%s/%s@%s,5.11-0:%s", ipsPublisher(a), ipsStem(a), a.Version, ipsTime(when))
	var b strings.Builder
	set := func(name, value string) { fmt.Fprintf(&b, "set name=%s value=%s\n", name, ipsQuote(value)) }
	set("pkg.fmri", fmri)
	set("pkg.summary", a.Summary)
	if a.Description != "" {
		set("pkg.description", a.Description)
	}
	set("variant.arch", "i386")
	// The app's own directories; the system's are its packages'.
	dirs := map[string]bool{}
	for _, f := range files {
		for d := path.Dir(strings.TrimPrefix(f.path, "/")); d != "." && !ipsSystemDir(d); d = path.Dir(d) {
			dirs[d] = true
		}
	}
	var dirList []string
	for d := range dirs {
		dirList = append(dirList, d)
	}
	sort.Strings(dirList)
	for _, d := range dirList {
		fmt.Fprintf(&b, "dir group=bin mode=0755 owner=root path=%s\n", d)
	}
	blobs := map[string][]byte{}
	for _, f := range files {
		var gz bytes.Buffer
		z, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
		z.Write(f.data)
		z.Close()
		hash := sha1Hex(f.data)
		blobs[hash] = gz.Bytes()
		mode := "0644"
		if f.mode&0o111 != 0 {
			mode = "0755"
		}
		// The menu entry and the icons, so that the desktop's caches of
		// them are rebuilt when the package is installed.
		restart := ""
		switch {
		case strings.Contains(f.path, "/share/applications/"):
			restart = " restart_fmri=svc:/application/desktop-cache/desktop-mime-cache:default"
		case strings.Contains(f.path, "/share/icons/"):
			restart = " restart_fmri=svc:/application/desktop-cache/icon-cache:default"
		}
		fmt.Fprintf(&b, "file %s chash=%s group=bin mode=%s owner=root path=%s pkg.content-hash=file:sha512t_256:%s pkg.content-hash=gzip:sha512t_256:%s pkg.csize=%d pkg.size=%d%s\n",
			hash, sha1Hex(gz.Bytes()), mode, strings.TrimPrefix(f.path, "/"), sha512t256Hex(f.data), sha512t256Hex(gz.Bytes()), gz.Len(), len(f.data), restart)
	}
	for _, d := range a.GTK.Illumos.Depends {
		fmt.Fprintf(&b, "depend fmri=%s type=require\n", d)
	}
	for _, d := range a.GTK.Illumos.Recommends {
		fmt.Fprintf(&b, "depend fmri=%s type=group\n", d)
	}

	out, err := os.Create(name)
	if err != nil {
		return err
	}
	defer out.Close()
	tw := tar.NewWriter(out)
	if err := bsdTarFile(tw, "manifest", []byte(b.String()), 0o644, when); err != nil {
		return err
	}
	hashes := make([]string, 0, len(blobs))
	for h := range blobs {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)
	for _, h := range hashes {
		if err := bsdTarFile(tw, "file/"+h, blobs[h], 0o644, when); err != nil {
			return err
		}
	}
	return tw.Close()
}

func sha1Hex(data []byte) string {
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:])
}

func sha512t256Hex(data []byte) string {
	sum := sha512.Sum512_256(data)
	return hex.EncodeToString(sum[:])
}

// ipsPackageFile is one package: its manifest, its files, and what its
// manifest's first line says of it.
type ipsPackageFile struct {
	manifest []byte
	files    map[string][]byte
	version  string // 1.2.3,5.11-0:20261012T090553Z
}

func readIPS(data []byte) (*ipsPackageFile, error) {
	p := &ipsPackageFile{files: map[string][]byte{}}
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		switch {
		case h.Name == "manifest":
			p.manifest = body
		case strings.HasPrefix(h.Name, "file/"):
			p.files[strings.TrimPrefix(h.Name, "file/")] = body
		}
	}
	m := regexp.MustCompile(`(?m)^set name=pkg\.fmri value=pkg://[^/]+/[^@]+@(\S+)$`).FindSubmatch(p.manifest)
	if m == nil {
		return nil, errors.New("an IPS package without a pkg.fmri")
	}
	p.version = string(m[1])
	return p, nil
}

// ipsJSON is v as pkg writes it: compact, keys sorted, only ASCII, and
// signed, as {..., "_SIGNATURE": {"sha-1": SHA-1 of the rest}}, with a
// newline after it.
func ipsJSON(v any) ([]byte, string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, "", err
	}
	text := asciiJSON(bytes.TrimRight(b.Bytes(), "\n"))
	sig := sha1Hex(append(append([]byte{}, text...), '\n'))
	signed := append(append([]byte{}, text[:len(text)-1]...), []byte(`,"_SIGNATURE":{"sha-1":"`+sig+`"}}`+"\n")...)
	return signed, sig, nil
}

// asciiJSON escapes every character beyond ASCII as \uXXXX, as Python's
// json does, so that a signature over the text is the same as pkg's.
func asciiJSON(text []byte) []byte {
	var b bytes.Buffer
	for len(text) > 0 {
		r, n := utf8.DecodeRune(text)
		if r < 0x80 {
			b.WriteByte(text[0])
		} else if r > 0xffff {
			r -= 0x10000
			fmt.Fprintf(&b, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
		} else {
			fmt.Fprintf(&b, `\u%04x`, r)
		}
		text = text[n:]
	}
	return b.Bytes()
}

// ipsCatalogActions are a manifest's actions of the kinds a catalog part
// lists: depend and variant for its dependency part, summary and
// description for its summary part.
func ipsCatalogActions(manifest []byte, part string) []string {
	var out []string
	for _, line := range strings.Split(string(manifest), "\n") {
		switch {
		case part == "dependency" && (strings.HasPrefix(line, "depend ") || strings.HasPrefix(line, "set name=variant.")):
			out = append(out, line)
		case part == "summary" && (strings.HasPrefix(line, "set name=pkg.summary ") || strings.HasPrefix(line, "set name=pkg.description ")):
			out = append(out, line)
		}
	}
	return out
}

// buildIPSRepo puts the new packages in site/illumos/packages, keeping the
// newest few, and writes the repository from them.
func buildIPSRepo(site string, sys *unixSystem, newPkgs []string, keep int, a *App) (map[string]Download, error) {
	root := filepath.Join(site, sys.name)
	store := filepath.Join(root, "packages")
	if err := os.MkdirAll(store, 0o755); err != nil {
		return nil, err
	}
	for _, p := range newPkgs {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if err := writeTree(filepath.Join(store, filepath.Base(p)), data, 0o644); err != nil {
			return nil, err
		}
	}
	files, versions, err := keepNewest(store, regexp.MustCompile(`^`+regexp.QuoteMeta(a.Name)+`-(.+)-illumos-x86_64\.ips$`), keep)
	if err != nil || len(files) == 0 {
		return nil, err
	}
	// The repository is written afresh each time, from what is kept.
	for _, d := range []string{"versions", "publisher", "catalog", ipsPublisher(a)} {
		os.RemoveAll(filepath.Join(root, d))
	}
	pub, stem := ipsPublisher(a), ipsStem(a)
	var base, deps, summary []map[string]any
	var newestTime string
	// Oldest first, as pkg lists them.
	for i := len(files) - 1; i >= 0; i-- {
		data, err := os.ReadFile(filepath.Join(store, files[i]))
		if err != nil {
			return nil, err
		}
		p, err := readIPS(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", files[i], err)
		}
		for hash, gz := range p.files {
			if err := writeTree(filepath.Join(root, pub, "file", "1", hash), gz, 0o644); err != nil {
				return nil, err
			}
		}
		// The manifest, by the path pkg asks for: the stem's / as %2F,
		// which most servers decode to a /, so it is written both ways.
		name := stem + "@" + p.version
		for _, rel := range []string{name, strings.ReplaceAll(stem, "/", "%2F") + "@" + p.version} {
			if err := writeTree(filepath.Join(root, pub, "manifest", "0", filepath.FromSlash(rel)), p.manifest, 0o644); err != nil {
				return nil, err
			}
		}
		base = append(base, map[string]any{"signature-sha-1": sha1Hex(p.manifest), "version": p.version})
		deps = append(deps, map[string]any{"actions": ipsCatalogActions(p.manifest, "dependency"), "version": p.version})
		summary = append(summary, map[string]any{"actions": ipsCatalogActions(p.manifest, "summary"), "version": p.version})
		newestTime = p.version[strings.LastIndex(p.version, ":")+1:]
	}
	// The catalog's times, from the newest package's: the same files for
	// the same packages.
	t, err := time.Parse("20060102T150405Z", newestTime)
	if err != nil {
		return nil, err
	}
	stamp := ipsCatalogTime(t)
	parts := map[string]any{}
	catalog := map[string][]byte{}
	for name, entries := range map[string][]map[string]any{"catalog.base.C": base, "catalog.dependency.C": deps, "catalog.summary.C": summary} {
		data, sig, err := ipsJSON(map[string]any{pub: map[string]any{stem: entries}})
		if err != nil {
			return nil, err
		}
		catalog[name] = data
		parts[name] = map[string]any{"last-modified": stamp, "signature-sha-1": sig}
	}
	attrs, _, err := ipsJSON(map[string]any{"created": stamp, "last-modified": stamp, "package-count": 1,
		"package-version-count": len(files), "parts": parts, "updates": map[string]any{}, "version": 1})
	if err != nil {
		return nil, err
	}
	catalog["catalog.attrs"] = attrs
	// pkg asks for the catalog with the publisher's name before it, and,
	// once, without.
	for name, data := range catalog {
		for _, dir := range []string{filepath.Join(root, pub, "catalog", "1"), filepath.Join(root, "catalog", "1")} {
			if err := writeTree(filepath.Join(dir, name), data, 0o644); err != nil {
				return nil, err
			}
		}
	}
	// What the depot offers, and its publisher, as pkg.depotd answers.
	versionsText := "pkg-server vero\ncatalog 0 1\nfile 0 1 2\ninfo 0\nmanifest 0 1\np5i 0\npublisher 0 1\nsearch 0 1\nstatus 0\nversions 0\n"
	if err := writeTree(filepath.Join(root, "versions", "0", "index.html"), []byte(versionsText), 0o644); err != nil {
		return nil, err
	}
	p5i, _ := json.MarshalIndent(map[string]any{"packages": []string{}, "publishers": []map[string]any{{"alias": nil, "name": pub, "packages": []string{}, "repositories": []string{}}}, "version": 1}, "", "  ")
	for _, v := range []string{"0", "1"} {
		if err := writeTree(filepath.Join(root, "publisher", v, "index.html"), append(p5i, '\n'), 0o644); err != nil {
			return nil, err
		}
	}
	return map[string]Download{"x86_64": {Version: versions[files[0]], URL: filepathRel(site, filepath.Join(store, files[0]))}}, nil
}

// checkIPS checks an illumos repository as pkg will read it: each catalog
// part signed as catalog.attrs says, each package's manifest there with
// the SHA-1 the catalog gives, and each of its files there, by the SHA-1
// of its contents.
func (c *checker) checkIPS(sys unixSystem) {
	root := sys.name
	attrsData, ok := c.file(root + "/catalog/1/catalog.attrs")
	if !ok {
		return
	}
	signed := func(rel string, data []byte) string {
		var v map[string]any
		if err := json.Unmarshal(data, &v); err != nil {
			c.fail("%s: %v", rel, err)
			return ""
		}
		sig, _ := v["_SIGNATURE"].(map[string]any)["sha-1"].(string)
		text := regexp.MustCompile(`,"_SIGNATURE":\{[^}]*\}`).ReplaceAll(data, nil)
		if sha1Hex(text) != sig {
			c.fail("%s: its signature is not its contents'", rel)
		}
		return sig
	}
	signed(root+"/catalog/1/catalog.attrs", attrsData)
	var attrs struct {
		Parts map[string]struct {
			Signature string `json:"signature-sha-1"`
		} `json:"parts"`
	}
	json.Unmarshal(attrsData, &attrs)
	pubs, _ := filepath.Glob(filepath.Join(c.site, root, "*", "catalog", "1"))
	for _, dir := range pubs {
		pub := filepath.Base(filepath.Dir(filepath.Dir(dir)))
		for name, part := range attrs.Parts {
			rel := path.Join(root, pub, "catalog", "1", name)
			if data, ok := c.file(rel); ok && signed(rel, data) != part.Signature {
				c.fail("%s: not the part catalog.attrs signs", rel)
			}
		}
		baseData, ok := c.file(path.Join(root, pub, "catalog", "1", "catalog.base.C"))
		if !ok {
			continue
		}
		var base map[string]any
		json.Unmarshal(baseData, &base)
		stems, _ := base[pub].(map[string]any)
		for stem, list := range stems {
			entries, _ := list.([]any)
			for _, e := range entries {
				entry, _ := e.(map[string]any)
				version, _ := entry["version"].(string)
				want, _ := entry["signature-sha-1"].(string)
				manifest, ok := c.file(path.Join(root, pub, "manifest", "0", stem+"@"+version))
				if !ok {
					continue
				}
				if sha1Hex(manifest) != want {
					c.fail("%s@%s: its manifest is not the one the catalog lists", stem, version)
				}
				for _, m := range regexp.MustCompile(`(?m)^file ([0-9a-f]{40}) `).FindAllSubmatch(manifest, -1) {
					hash := string(m[1])
					gz, ok := c.file(path.Join(root, pub, "file", "1", hash))
					if !ok {
						continue
					}
					plain, err := decompress(".gz", gz)
					if err != nil || sha1Hex(plain) != hash {
						c.fail("%s@%s: the file %s is damaged", stem, version, hash)
					}
				}
			}
		}
	}
	c.ok(sys.label + "'s IPS repository")
}
