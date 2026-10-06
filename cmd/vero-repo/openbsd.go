package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// An OpenBSD package is a tar, gzipped, of +CONTENTS - its packing list,
// which names it, what it depends on, and each file with its SHA-256,
// size and time - then +DESC, then the files in the list's order. pkg_add
// installs only signed packages unless told otherwise: signed by signify
// -z, which puts the signature, and a SHA-512/256 of each 64 KiB of the
// gzipped stream, in the gzip header's comment.

// signifyFile is the signify key that signs OpenBSD packages, kept in the
// signing key's folder beside private.asc: a key number, and Ed25519's
// seed.
const signifyFile = "signify.sec"

// writeOpenBSD writes an OpenBSD package, unsigned: vero-repo build signs
// it when it adds it to the site, as with the rpms.
func writeOpenBSD(name string, a *App, sys *unixSystem, arch unixArch, files []treeFile) error {
	pkgname := a.Name + "-" + bsdVersion(a.Version)
	desc := strings.TrimSpace(a.Summary)
	if d := strings.TrimSpace(a.Description); d != "" {
		desc += "\n\n" + d
	}
	if a.Homepage != "" {
		desc += "\n\nWWW: " + a.Homepage
	}
	descData := []byte(desc + "\n")
	mtime := buildTime()
	var contents bytes.Buffer
	fmt.Fprintf(&contents, "@name %s\n", pkgname)
	fmt.Fprintf(&contents, "@comment pkgpath=misc/%s ftp=yes\n", a.Name)
	fmt.Fprintf(&contents, "@arch %s\n", arch.name)
	fmt.Fprintf(&contents, "+DESC\n@sha %s\n@size %d\n", sha256Base64(descData), len(descData))
	for _, d := range a.GTK.OpenBSD.Depends {
		fmt.Fprintf(&contents, "@depend %s\n", d)
	}
	fmt.Fprintf(&contents, "@cwd %s\n", sys.prefix)
	for _, f := range files {
		rel := strings.TrimPrefix(f.path, sys.prefix+"/")
		keyword := ""
		if f.mode&0o111 != 0 && path.Base(path.Dir(f.path)) == "bin" {
			keyword = "@bin "
		}
		fmt.Fprintf(&contents, "%s%s\n@sha %s\n@size %d\n@ts %d\n", keyword, rel, sha256Base64(f.data), len(f.data), mtime.Unix())
	}
	var b bytes.Buffer
	z, err := gzip.NewWriterLevel(&b, gzip.BestCompression)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(z)
	for _, f := range []treeFile{{"+CONTENTS", contents.Bytes(), 0o644}, {"+DESC", descData, 0o644}} {
		if err := ustarFile(tw, f.path, f.data, f.mode, mtime.Unix()); err != nil {
			return err
		}
	}
	for _, f := range files {
		if err := ustarFile(tw, strings.TrimPrefix(f.path, sys.prefix+"/"), f.data, f.mode, mtime.Unix()); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := z.Close(); err != nil {
		return err
	}
	return writeTree(name, b.Bytes(), 0o644)
}

func sha256Base64(data []byte) string {
	sum := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// ustarFile writes a file as OpenBSD's ustar archives hold it: owned by
// root and bin, as pkg_create makes them.
func ustarFile(tw *tar.Writer, name string, data []byte, mode os.FileMode, ts int64) error {
	h := &tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: int64(mode.Perm()), Size: int64(len(data)),
		ModTime: time.Unix(ts, 0), Uname: "root", Gname: "bin", Format: tar.FormatUSTAR}
	if err := tw.WriteHeader(h); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// signifyKey is a signify key: its number, which a signature names, and
// its Ed25519 private key.
type signifyKey struct {
	number [8]byte
	key    ed25519.PrivateKey
}

// signify is the key that signs OpenBSD packages: signify.sec in the key's
// folder, made the first time it's needed.
func (s *signer) signify() (*signifyKey, error) {
	if s.dir == "" {
		return nil, errors.New("the signing key has no folder")
	}
	file := filepath.Join(s.dir, signifyFile)
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		var k signifyKey
		rand.Read(k.number[:])
		_, k.key, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		out := hex.EncodeToString(k.number[:]) + "\n" + base64.StdEncoding.EncodeToString(k.key.Seed()) + "\n"
		if err := os.WriteFile(file, []byte(out), 0o600); err != nil {
			return nil, err
		}
		fmt.Printf("made %s, which signs OpenBSD packages: keep it with private.asc\n", file)
		return &k, nil
	}
	if err != nil {
		return nil, err
	}
	lines := strings.Fields(string(data))
	if len(lines) != 2 {
		return nil, fmt.Errorf("%s isn't a signify key", file)
	}
	num, err := hex.DecodeString(lines[0])
	if err != nil || len(num) != 8 {
		return nil, fmt.Errorf("%s isn't a signify key", file)
	}
	seed, err := base64.StdEncoding.DecodeString(lines[1])
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s isn't a signify key", file)
	}
	var k signifyKey
	copy(k.number[:], num)
	k.key = ed25519.NewKeyFromSeed(seed)
	return &k, nil
}

// publicFile is the key's public half as signify keeps it, for
// /etc/signify/NAME.pub.
func (k *signifyKey) publicFile(name string) []byte {
	raw := append([]byte("Ed"), k.number[:]...)
	raw = append(raw, k.key.Public().(ed25519.PublicKey)...)
	return []byte(fmt.Sprintf("untrusted comment: %s public key\n%s\n", name, base64.StdEncoding.EncodeToString(raw)))
}

// signGzip signs a gzipped file as signify -zS does, for the key named
// name ("myapp-pkg"): the stream after the gzip header, hashed in blocks,
// with the signature and the hashes in a new header's comment.
func (k *signifyKey) signGzip(gz []byte, name string) ([]byte, error) {
	if len(gz) < 10 || gz[0] != 0x1f || gz[1] != 0x8b || gz[2] != 8 {
		return nil, errors.New("not a gzip file")
	}
	flags := gz[3]
	pos := 10
	for _, flag := range []byte{8, 16} { // a name, then a comment
		if flags&flag != 0 {
			end := bytes.IndexByte(gz[pos:], 0)
			if end < 0 {
				return nil, errors.New("a damaged gzip header")
			}
			pos += end + 1
		}
	}
	if flags&^(8|16) != 0 {
		return nil, errors.New("a gzip header signify can't sign")
	}
	body := gz[pos:]
	const blocksize = 65536
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "date=%s\nkey=%s.sec\nalgorithm=SHA512/256\nblocksize=%d\n\n",
		buildTime().UTC().Format("2006-01-02T15:04:05Z"), name, blocksize)
	for i := 0; i < len(body); i += blocksize {
		end := min(i+blocksize, len(body))
		sum := sha512.Sum512_256(body[i:end])
		msg.WriteString(hex.EncodeToString(sum[:]) + "\n")
	}
	sig := ed25519.Sign(k.key, msg.Bytes())
	raw := append([]byte("Ed"), k.number[:]...)
	raw = append(raw, sig...)
	var out bytes.Buffer
	out.Write([]byte{0x1f, 0x8b, 8, 16, 0, 0, 0, 0, gz[8], 3})
	fmt.Fprintf(&out, "untrusted comment: verify with %s.pub\n%s\n", name, base64.StdEncoding.EncodeToString(raw))
	out.Write(msg.Bytes())
	out.WriteByte(0)
	out.Write(body)
	return out.Bytes(), nil
}

// buildOpenBSDRepo makes site/openbsd/ARCH a folder pkg_add installs and
// updates from: the packages, each signed, the newest keep of each, and an
// index.html listing them, which is how pkg_add looks in a folder over
// HTTP. The public key goes beside, as NAME-pkg.pub.
func buildOpenBSDRepo(site string, sys *unixSystem, newPkgs []string, keep int, a *App, s *signer) (map[string]Download, error) {
	var k *signifyKey
	newest := map[string]Download{}
	keyName := a.Name + "-pkg"
	for _, arch := range sys.arches {
		dir := filepath.Join(site, sys.name, arch.name)
		suffix := "-" + sys.name + "-" + arch.name + ".tgz"
		for _, p := range newPkgs {
			base := filepath.Base(p)
			if !strings.HasSuffix(base, suffix) {
				continue
			}
			if k == nil {
				var err error
				if k, err = s.signify(); err != nil {
					return nil, err
				}
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			signed, err := k.signGzip(data, keyName)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", base, err)
			}
			version := strings.TrimSuffix(strings.TrimPrefix(base, a.Name+"-"), suffix)
			if err := writeTree(filepath.Join(dir, a.Name+"-"+version+".tgz"), signed, 0o644); err != nil {
				return nil, err
			}
		}
		files, versions, err := keepNewest(dir, regexp.MustCompile(`^`+regexp.QuoteMeta(a.Name)+`-(.+)\.tgz$`), keep)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			continue
		}
		var list bytes.Buffer
		list.WriteString("<!doctype html>\n<html><body>\n")
		for _, f := range files {
			fmt.Fprintf(&list, "<a href=\"%s\">%s</a><br>\n", html.EscapeString(f), html.EscapeString(f))
		}
		list.WriteString("</body></html>\n")
		if err := os.WriteFile(filepath.Join(dir, "index.html"), list.Bytes(), 0o644); err != nil {
			return nil, err
		}
		newest[arch.name] = Download{Version: versions[files[0]], URL: filepathRel(site, filepath.Join(dir, files[0]))}
	}
	if len(newest) == 0 {
		return nil, nil
	}
	if k == nil {
		var err error
		if k, err = s.signify(); err != nil {
			return nil, err
		}
	}
	return newest, os.WriteFile(filepath.Join(site, sys.name, keyName+".pub"), k.publicFile(keyName), 0o644)
}
