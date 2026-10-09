package main

import (
	"archive/tar"
	"bytes"
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// voidArches are the architectures Void's repositories are found by:
// each of glibc and musl, which the same package serves, since the
// worker needs neither. Void has i686 only with glibc.
func voidArches(arch string) []string {
	if arch == "i686" {
		return []string{arch}
	}
	return []string{arch, arch + "-musl"}
}

// voidPackage is NAME-VERSION_1.ARCH.xbps, as packageVoid names them.
func voidPackage(name string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `-([^-_]+)_1\.((?:` + linuxArchNames("void") + `)(?:-musl)?)\.xbps$`)
}

// voidDepends is what a Void package depends on, as xbps patterns:
// [gtk.void]'s, or GTK 4 for Python.
func voidDepends(a *App) []string {
	deps := splitList(a.GTK.Void.Depends)
	if len(deps) == 0 {
		deps = []string{"python3", "python3-gobject", "gtk4"}
	}
	for i, d := range deps {
		if !strings.ContainsAny(d, "<>=*?[") {
			deps[i] = d + ">=0"
		}
	}
	return deps
}

// packageVoid builds the app's Void package for each architecture, and
// each of its C libraries, in Go, as xbps-create would: a tar compressed
// with zstd, of props.plist, saying what it is, files.plist, the hash of
// each file, then the files. vero-repo build signs it, and puts it in the
// site's repository.
func packageVoid(a *App, workers map[string]string, out string) error {
	version := bsdVersion(a.Version)
	for _, arch := range a.linuxArches("void") {
		files, err := linuxTree(a, "/usr", workers[arch.goarch], a.Name)
		if err != nil {
			return err
		}
		for _, name := range voidArches(arch.name) {
			file := filepath.Join(out, fmt.Sprintf("%s-%s_1.%s.xbps", a.Name, version, name))
			if err := writeVoid(file, voidProps(a, version, name, files), files); err != nil {
				return err
			}
			fmt.Println("built", file)
		}
	}
	return nil
}

// voidProps is a package's props.plist.
func voidProps(a *App, version, arch string, files []treeFile) plistDict {
	var size int64
	for _, f := range files {
		size += int64(len(f.data))
	}
	licence := a.Licence
	if licence == "" {
		licence = "custom"
	}
	p := plistDict{
		"pkgname":        a.Name,
		"version":        version + "_1",
		"pkgver":         a.Name + "-" + version + "_1",
		"architecture":   arch,
		"build-date":     buildTime().UTC().Format("2006-01-02 15:04 MST"),
		"installed_size": size,
		"license":        licence,
		"maintainer":     a.Publisher,
		"short_desc":     a.Summary,
		"run_depends":    voidDepends(a),
	}
	if a.Homepage != "" {
		p["homepage"] = a.Homepage
	}
	if d := strings.TrimSpace(a.Description); d != "" {
		p["long_desc"] = d
	}
	return p
}

// writeVoid writes a Void package: ./props.plist, ./files.plist, then
// the files.
func writeVoid(name string, props plistDict, files []treeFile) error {
	mtime := buildTime()
	entries := treeEntries(files)
	var fileList, dirList []any
	for _, e := range entries {
		if e.dir {
			dirList = append(dirList, plistDict{"file": e.path})
			continue
		}
		sum := sha256.Sum256(e.data)
		fileList = append(fileList, plistDict{"file": e.path, "sha256": hex.EncodeToString(sum[:]), "size": int64(len(e.data)), "mtime": mtime.Unix()})
	}
	var b bytes.Buffer
	z, err := zstd.NewWriter(&b)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(z)
	if err := tarFile(tw, "./props.plist", plist(props), 0o644, mtime); err != nil {
		return err
	}
	if err := tarFile(tw, "./files.plist", plist(plistDict{"files": fileList, "dirs": dirList}), 0o644, mtime); err != nil {
		return err
	}
	for _, e := range entries {
		if e.dir {
			err = tarDir(tw, "."+e.path+"/", mtime)
		} else {
			err = tarFile(tw, "."+e.path, e.data, e.mode, mtime)
		}
		if err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := z.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	return os.WriteFile(name, b.Bytes(), 0o644)
}

// voidContents are a package's props.plist and the files in it.
func voidContents(pkg []byte) (plistDict, []string, error) {
	z, err := zstd.NewReader(bytes.NewReader(pkg))
	if err != nil {
		return nil, nil, err
	}
	defer z.Close()
	tr := tar.NewReader(z)
	var props plistDict
	var files []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		switch h.Name {
		case "./props.plist":
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, nil, err
			}
			if props, err = parsePlist(data); err != nil {
				return nil, nil, err
			}
		case "./files.plist":
		default:
			files = append(files, h.Name)
		}
	}
	if props == nil {
		return nil, nil, errors.New("no props.plist")
	}
	return props, files, nil
}

// voidSign signs a package as xbps-rindex --sign-pkg does: .sig2, with
// RSA and SHA-256, which xbps checks now, and .sig, with SHA-1, which
// older xbps checked.
func voidSign(data []byte, s *signer) (sig2, sig []byte, err error) {
	k, err := s.rsaKey()
	if err != nil {
		return nil, nil, err
	}
	sum := sha256.Sum256(data)
	if sig2, err = rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, sum[:]); err != nil {
		return nil, nil, err
	}
	old := sha1.Sum(data)
	sig, err = rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA1, old[:])
	return sig2, sig, err
}

// buildVoid makes site/void a repository xbps installs and updates from:
// the newest keep packages of each architecture, each signed, and
// ARCH-repodata for each, which lists them and carries the public key
// that checks them, as xbps-rindex makes it; and FINGERPRINT.plist, the
// key as xbps keeps it once it trusts it. It returns the newest package
// of each architecture.
func buildVoid(site string, newPkgs []string, keep int, a *App, s *signer) (map[string]Download, error) {
	dir := filepath.Join(site, "void")
	match := voidPackage(a.Name)
	for _, p := range newPkgs {
		if match.MatchString(filepath.Base(p)) {
			if err := copyFile(p, filepath.Join(dir, filepath.Base(p))); err != nil {
				return nil, err
			}
			for _, ext := range []string{".sig2", ".sig"} {
				os.Remove(filepath.Join(dir, filepath.Base(p)+ext))
			}
		}
	}
	key, pub, err := voidKey(a, s)
	if err != nil {
		return nil, err
	}
	newest := map[string]Download{}
	for _, arch := range a.linuxArches("void") {
		for _, name := range voidArches(arch.name) {
			files, versions, err := keepNewest(dir, regexp.MustCompile(`^`+regexp.QuoteMeta(a.Name)+`-([^-_]+)_1\.`+regexp.QuoteMeta(name)+`\.xbps$`), keep)
			if err != nil {
				return nil, err
			}
			if len(files) == 0 {
				continue
			}
			index := plistDict{}
			for i, f := range files {
				data, err := os.ReadFile(filepath.Join(dir, f))
				if err != nil {
					return nil, err
				}
				if _, err := os.Stat(filepath.Join(dir, f+".sig2")); err != nil {
					sig2, sig, err := voidSign(data, s)
					if err != nil {
						return nil, err
					}
					os.WriteFile(filepath.Join(dir, f+".sig2"), sig2, 0o644)
					os.WriteFile(filepath.Join(dir, f+".sig"), sig, 0o644)
				}
				// The index lists one version of each package: the newest.
				if i > 0 {
					continue
				}
				props, _, err := voidContents(data)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", f, err)
				}
				sum := sha256.Sum256(data)
				props["filename-sha256"] = hex.EncodeToString(sum[:])
				props["filename-size"] = int64(len(data))
				index[a.Name] = props
			}
			meta := plistDict{"signature-type": "rsa"}
			for k, v := range key {
				meta[k] = v
			}
			repodata, err := voidRepodata(index, meta)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(dir, name+"-repodata"), repodata, 0o644); err != nil {
				return nil, err
			}
			newest[name] = Download{Version: versions[files[0]], URL: filepathRel(site, filepath.Join(dir, files[0]))}
		}
	}
	if len(newest) == 0 {
		return nil, nil
	}
	// The key, as xbps keeps one it trusts, for the page's commands to put
	// there, so that xbps needn't ask.
	if err := os.WriteFile(filepath.Join(dir, voidFingerprint(pub)+".plist"), plist(key), 0o644); err != nil {
		return nil, err
	}
	// Signatures of packages no longer kept go with them.
	sigs, _ := filepath.Glob(filepath.Join(dir, "*.xbps.sig*"))
	for _, sig := range sigs {
		if _, err := os.Stat(strings.TrimSuffix(strings.TrimSuffix(sig, "2"), ".sig")); err != nil {
			os.Remove(sig)
		}
	}
	return newest, nil
}

// voidRepodata is ARCH-repodata: a tar, compressed with zstd, of
// index.plist and index-meta.plist.
func voidRepodata(index, meta plistDict) ([]byte, error) {
	var b bytes.Buffer
	z, err := zstd.NewWriter(&b)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(z)
	for _, f := range []struct {
		name string
		dict plistDict
	}{{"index.plist", index}, {"index-meta.plist", meta}} {
		if err := tarFile(tw, f.name, plist(f.dict), 0o644, buildTime()); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// voidFingerprint is the key's fingerprint as xbps names it: the MD5 of
// the key as SSH writes an RSA key, in hex, colon separated. xbps keeps a
// key it trusts as FINGERPRINT.plist in /var/db/xbps/keys.
func voidFingerprint(k *rsa.PublicKey) string {
	field := func(b []byte) []byte {
		return append(binary.BigEndian.AppendUint32(nil, uint32(len(b))), b...)
	}
	// An SSH mpint: big-endian, with a zero first if the top bit is set.
	mpint := func(n *big.Int) []byte {
		b := n.Bytes()
		if len(b) > 0 && b[0]&0x80 != 0 {
			b = append([]byte{0}, b...)
		}
		return field(b)
	}
	blob := append(field([]byte("ssh-rsa")), mpint(big.NewInt(int64(k.E)))...)
	blob = append(blob, mpint(k.N)...)
	sum := md5.Sum(blob)
	var parts []string
	for _, b := range sum {
		parts = append(parts, fmt.Sprintf("%02x", b))
	}
	return strings.Join(parts, ":")
}

// voidKey is the key as xbps keeps one it trusts: what the repodata's
// index-meta.plist says of it.
func voidKey(a *App, s *signer) (plistDict, *rsa.PublicKey, error) {
	pub, err := s.rsaPublicPEM()
	if err != nil {
		return nil, nil, err
	}
	k, err := s.rsaKey()
	if err != nil {
		return nil, nil, err
	}
	return plistDict{
		"public-key":      pub,
		"public-key-size": int64(k.N.BitLen()),
		"signature-by":    a.PublisherName(),
	}, &k.PublicKey, nil
}

// plistDict is a property list's dictionary, as xbps reads them with
// proplib: values are strings, int64s, []byte (data), []string, []any
// or plistDicts.
type plistDict map[string]any

// plist is d as an XML property list.
func plist(d plistDict) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n")
	plistValue(&b, d, 0)
	b.WriteString("</plist>\n")
	return []byte(b.String())
}

func plistValue(b *strings.Builder, v any, depth int) {
	indent := strings.Repeat("\t", depth)
	// proplib reads &amp;, &lt; and &gt;, but not the numbered
	// references xml.EscapeText makes of a newline or a tab.
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace
	switch v := v.(type) {
	case plistDict:
		b.WriteString(indent + "<dict>\n")
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(b, "%s\t<key>%s</key>\n", indent, esc(k))
			plistValue(b, v[k], depth+1)
		}
		b.WriteString(indent + "</dict>\n")
	case []any:
		b.WriteString(indent + "<array>\n")
		for _, x := range v {
			plistValue(b, x, depth+1)
		}
		b.WriteString(indent + "</array>\n")
	case []string:
		b.WriteString(indent + "<array>\n")
		for _, x := range v {
			fmt.Fprintf(b, "%s\t<string>%s</string>\n", indent, esc(x))
		}
		b.WriteString(indent + "</array>\n")
	case string:
		fmt.Fprintf(b, "%s<string>%s</string>\n", indent, esc(v))
	case int64:
		fmt.Fprintf(b, "%s<integer>%d</integer>\n", indent, v)
	case []byte:
		fmt.Fprintf(b, "%s<data>%s</data>\n", indent, base64.StdEncoding.EncodeToString(v))
	case bool:
		if v {
			b.WriteString(indent + "<true/>\n")
		} else {
			b.WriteString(indent + "<false/>\n")
		}
	}
}

// parsePlist reads a property list plist wrote.
func parsePlist(data []byte) (plistDict, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		t, err := d.Token()
		if err != nil {
			return nil, err
		}
		if se, ok := t.(xml.StartElement); ok && se.Name.Local == "dict" {
			v, err := plistParse(d, se)
			if err != nil {
				return nil, err
			}
			return v.(plistDict), nil
		}
	}
}

func plistParse(d *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "dict":
		out := plistDict{}
		key := ""
		for {
			t, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := t.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					if err := d.DecodeElement(&key, &t); err != nil {
						return nil, err
					}
					continue
				}
				v, err := plistParse(d, t)
				if err != nil {
					return nil, err
				}
				out[key] = v
			case xml.EndElement:
				return out, nil
			}
		}
	case "array":
		var out []any
		for {
			t, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := t.(type) {
			case xml.StartElement:
				v, err := plistParse(d, t)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			case xml.EndElement:
				return out, nil
			}
		}
	case "true", "false":
		d.Skip()
		return start.Name.Local == "true", nil
	}
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return nil, err
	}
	switch start.Name.Local {
	case "integer":
		var n int64
		fmt.Sscan(s, &n)
		return n, nil
	case "data":
		return base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
	}
	return s, nil
}
