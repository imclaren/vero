package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/ulikunitz/xz"
)

// unixSystem is a Unix other than Linux that vero packages for, with the
// GTK front end: how Go, the system and its packages name each
// architecture, and where its packages install.
type unixSystem struct {
	name   string // vero's name for it, and the site's folder
	label  string // what people call it
	goos   string
	prefix string
	arches []unixArch
	// format is how its packages are made: "pkg" (FreeBSD's), "pkgsrc"
	// or "openbsd".
	format string
	// opsys and osVersion are what a pkgsrc package says it was built
	// for.
	opsys, osVersion string
}

type unixArch struct {
	goarch string
	name   string // as the system names it: amd64, aarch64, x86_64
	// abi is a pkg package's ABI, which may match many versions, and
	// legacy the same as older pkg wrote it, which pkg still reads.
	abi, legacy string
	// extra is packaged only where the system's section names it.
	extra bool
}

var unixSystems = []unixSystem{
	{name: "freebsd", label: "FreeBSD", goos: "freebsd", prefix: "/usr/local", format: "pkg", arches: []unixArch{
		{"amd64", "amd64", "FreeBSD:*:amd64", "freebsd:*:x86:64", false}, {"arm64", "aarch64", "FreeBSD:*:aarch64", "freebsd:*:aarch64:64", false},
		{"386", "i386", "FreeBSD:*:i386", "freebsd:*:x86:32", true}, {"arm", "armv7", "FreeBSD:*:armv7", "freebsd:*:armv7:32:el:eabi:hardfp", true}}},
	{name: "dragonfly", label: "DragonFly", goos: "dragonfly", prefix: "/usr/local", format: "pkg", arches: []unixArch{
		{"amd64", "x86:64", "dragonfly:*:x86:64", "dragonfly:*:x86:64", false}}},
	{name: "netbsd", label: "NetBSD", goos: "netbsd", prefix: "/usr/pkg", format: "pkgsrc", opsys: "NetBSD", osVersion: "10.0",
		arches: []unixArch{{"amd64", "x86_64", "", "", false}, {"arm64", "aarch64", "", "", false}, {"386", "i386", "", "", true}}},
	{name: "illumos", label: "illumos", goos: "illumos", prefix: "/opt/local", format: "pkgsrc", opsys: "SunOS", osVersion: "5.11",
		arches: []unixArch{{"amd64", "x86_64", "", "", false}}},
	{name: "openbsd", label: "OpenBSD", goos: "openbsd", prefix: "/usr/local", format: "openbsd",
		arches: []unixArch{{"amd64", "amd64", "", "", false}, {"arm64", "aarch64", "", "", false},
			{"386", "i386", "", "", true}, {"arm", "arm", "", "", true}, {"riscv64", "riscv64", "", "", true}}},
}

// system is one of unixSystems, by name.
func system(name string) *unixSystem {
	for i := range unixSystems {
		if unixSystems[i].name == name {
			return &unixSystems[i]
		}
	}
	return nil
}

// enabled says whether the app asks to be packaged for sys: whether its
// [gtk.NAME] section is there.
func (g *GTK) enabled(sys string) bool {
	switch sys {
	case "freebsd":
		return g.FreeBSD != nil
	case "dragonfly":
		return g.DragonFly != nil
	case "netbsd":
		return g.NetBSD != nil
	case "illumos":
		return g.Illumos != nil
	case "openbsd":
		return g.OpenBSD != nil
	}
	return false
}

// bsdVersion is a version as pkg and pkgsrc allow it: they divide a
// package's name from its version at the last hyphen, so none may be in
// the version. "1.2.3-beta" becomes "1.2.3.beta".
func bsdVersion(v string) string { return strings.ReplaceAll(v, "-", ".") }

// unixPackageName is the file vero-repo package makes for sys on arch.
func unixPackageName(a *App, sys *unixSystem, arch unixArch) string {
	ext := map[string]string{"pkg": ".pkg", "pkgsrc": ".tgz", "openbsd": ".tgz"}[sys.format]
	return fmt.Sprintf("%s-%s-%s-%s%s", a.Name, bsdVersion(a.Version), sys.name, fileArch(arch), ext)
}

// unixPackage is NAME-VERSION-SYSTEM-ARCH.EXT, as packageUnix names them.
func unixPackage(name string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `-(.+)-(freebsd|dragonfly|netbsd|illumos|openbsd)-([a-z0-9_]+)\.(pkg|tgz)$`)
}

// packageUnix builds the app's package for sys, for each architecture:
// workers are the worker built for each of Go's architectures there.
func packageUnix(a *App, sys *unixSystem, workers map[string]string, out string) error {
	for _, arch := range a.archesFor(sys) {
		// Both by their whole path, under the package's prefix: a login's
		// PATH may leave it out, as OpenIndiana's does /opt/local/bin, and
		// a desktop session's even more often.
		python := a.GTK.python(systemPython(a.GTK, sys.name, arch.name))
		if !strings.Contains(python, "/") {
			python = path.Join(sys.prefix, "bin", python)
		}
		files, err := unixTree(a, sys.prefix, workers[arch.goarch], path.Join(sys.prefix, "bin", a.Name), python)
		if err != nil {
			return err
		}
		name := filepath.Join(out, unixPackageName(a, sys, arch))
		switch sys.format {
		case "pkg":
			err = writePkg(name, a, sys, arch, files)
		case "pkgsrc":
			err = writePkgsrc(name, a, sys, arch, files)
		case "openbsd":
			err = writeOpenBSD(name, a, sys, arch, files)
		}
		if err != nil {
			return err
		}
		fmt.Println("built", name)
	}
	return nil
}

// systemPython is the python a system's own section asks for on arch, by
// the system's name for it, or "".
func systemPython(g *GTK, sys, arch string) string {
	switch sys {
	case "freebsd":
		return g.FreeBSD.pythonFor(arch)
	case "dragonfly":
		return g.DragonFly.pythonFor(arch)
	case "netbsd":
		return g.NetBSD.Python
	case "illumos":
		return g.Illumos.Python
	case "openbsd":
		return g.OpenBSD.Python
	}
	return ""
}

// pkgManifest is a FreeBSD or DragonFly package's manifest: compact, as a
// repository lists it, or with files, as the package holds it.
func pkgManifest(a *App, sys *unixSystem, arch unixArch, files []treeFile, full bool) map[string]any {
	deps := map[string]any{}
	section := a.GTK.FreeBSD
	if sys.name == "dragonfly" {
		section = a.GTK.DragonFly
	}
	for name, origin := range section.depsFor(arch.name) {
		deps[name] = map[string]string{"origin": origin}
	}
	var flatsize int
	for _, f := range files {
		flatsize += len(f.data)
	}
	maintainer := a.Publisher
	if m := regexp.MustCompile(`<([^>]+)>`).FindStringSubmatch(a.Publisher); m != nil {
		maintainer = m[1]
	}
	desc := strings.TrimSpace(a.Description)
	if desc == "" {
		desc = a.Summary
	}
	m := map[string]any{
		"name": a.Name, "origin": "misc/" + a.Name, "version": bsdVersion(a.Version),
		"comment": a.Summary, "maintainer": maintainer, "www": a.Homepage,
		"abi": arch.abi, "arch": arch.legacy, "prefix": sys.prefix, "flatsize": flatsize,
		"licenselogic": "single", "desc": desc, "categories": []string{"misc"},
		"deps": deps,
	}
	if a.Licence != "" {
		m["licenses"] = []string{a.Licence}
	}
	if full {
		fileSums := map[string]string{}
		dirs := map[string]string{}
		lib := path.Join(sys.prefix, "lib", a.Name)
		for _, f := range files {
			sum := sha256.Sum256(f.data)
			fileSums[f.path] = "1$" + hex.EncodeToString(sum[:])
			for d := path.Dir(f.path); strings.HasPrefix(d, lib); d = path.Dir(d) {
				dirs[d] = "y"
			}
		}
		m["files"] = fileSums
		m["directories"] = dirs
	}
	return m
}

// writePkg writes a FreeBSD or DragonFly package: a tar, xz-compressed,
// of +COMPACT_MANIFEST, +MANIFEST, then the files at their paths.
func writePkg(name string, a *App, sys *unixSystem, arch unixArch, files []treeFile) error {
	compact, err := json.Marshal(pkgManifest(a, sys, arch, files, false))
	if err != nil {
		return err
	}
	manifest, err := json.Marshal(pkgManifest(a, sys, arch, files, true))
	if err != nil {
		return err
	}
	var b bytes.Buffer
	z, err := xz.NewWriter(&b)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(z)
	mtime := buildTime()
	for _, f := range []treeFile{{"+COMPACT_MANIFEST", compact, 0o644}, {"+MANIFEST", manifest, 0o644}} {
		if err := bsdTarFile(tw, f.path, f.data, f.mode, mtime); err != nil {
			return err
		}
	}
	for _, f := range files {
		if err := bsdTarFile(tw, f.path, f.data, f.mode, mtime); err != nil {
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

func bsdTarFile(tw *tar.Writer, name string, data []byte, mode fs.FileMode, mtime time.Time) error {
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg, Name: name, Mode: int64(mode.Perm()), Size: int64(len(data)), ModTime: mtime,
		Uname: "root", Gname: "wheel", Format: tar.FormatPAX,
	}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// buildPkgRepo makes site/SYSTEM/ARCH a repository pkg installs and
// updates from, for each architecture: the packages in All, the newest
// keep of each, and the indexes, signed with the key's RSA half as pkg's
// "pubkey" signatures are, as pkg on FreeBSD signs them; DragonFly's pkg
// checks that kind first too. It returns the newest package of each
// architecture.
func buildPkgRepo(site string, sys *unixSystem, newPkgs []string, keep int, url string, a *App, s *signer) (map[string]Download, error) {
	newest := map[string]Download{}
	for _, arch := range a.archesFor(sys) {
		dir := pkgRepoDir(site, sys, arch)
		all := filepath.Join(dir, "All")
		suffix := "-" + sys.name + "-" + fileArch(arch) + ".pkg"
		for _, p := range newPkgs {
			base := filepath.Base(p)
			if !strings.HasSuffix(base, suffix) {
				continue
			}
			version := strings.TrimSuffix(strings.TrimPrefix(base, a.Name+"-"), suffix)
			if err := copyFile(p, filepath.Join(all, a.Name+"-"+version+".pkg")); err != nil {
				return nil, err
			}
		}
		files, versions, err := keepNewest(all, regexp.MustCompile(`^`+regexp.QuoteMeta(a.Name)+`-(.+)\.pkg$`), keep)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			continue
		}
		var lines []json.RawMessage
		for _, f := range files {
			data, err := os.ReadFile(filepath.Join(all, f))
			if err != nil {
				return nil, err
			}
			compact, err := pkgCompactManifest(data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			sum := sha256.Sum256(data)
			compact["sum"] = hex.EncodeToString(sum[:])
			compact["pkgsize"] = len(data)
			compact["path"] = "All/" + f
			compact["repopath"] = "All/" + f
			line, err := json.Marshal(compact)
			if err != nil {
				return nil, err
			}
			lines = append(lines, line)
		}
		var manifests bytes.Buffer
		for _, l := range lines {
			manifests.Write(l)
			manifests.WriteString("\n")
		}
		data, err := json.Marshal(map[string]any{"groups": []any{}, "expired_packages": []any{}, "packages": lines})
		if err != nil {
			return nil, err
		}
		data = append(data, '\n')
		for _, archive := range []struct {
			name, member string
			data         []byte
		}{{"packagesite", "packagesite.yaml", manifests.Bytes()}, {"data", "data", data}} {
			packed, err := pkgArchive(archive.member, archive.data, s, false)
			if err != nil {
				return nil, err
			}
			// .pkg for pkg as it is now; .txz for older pkg, as on DragonFly.
			for _, ext := range []string{".pkg", ".txz"} {
				if err := os.WriteFile(filepath.Join(dir, archive.name+ext), packed, 0o644); err != nil {
					return nil, err
				}
			}
		}
		meta := "version = 2;\npacking_format = \"txz\";\nmanifests = \"packagesite.yaml\";\ndata = \"data\";\n" +
			"filesite = \"filesite.yaml\";\nmanifests_archive = \"packagesite\";\nfilesite_archive = \"filesite\";\n"
		if err := os.WriteFile(filepath.Join(dir, "meta.conf"), []byte(meta), 0o644); err != nil {
			return nil, err
		}
		newest[fileArch(arch)] = Download{Version: versions[files[0]], URL: filepathRel(site, filepath.Join(all, files[0]))}
	}
	if len(newest) == 0 {
		return nil, nil
	}
	key, err := s.rsaPublicPEM()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(site, sys.name, "key.pem"), key, 0o644); err != nil {
		return nil, err
	}
	return newest, os.WriteFile(filepath.Join(site, sys.name, a.Name+".conf"), pkgRepoConf(a, sys, url), 0o644)
}

// pkgRepoDir is the folder of sys's repository for arch. FreeBSD's
// repositories are found by ${ARCH}, which pkg fills in; DragonFly has
// only one.
func pkgRepoDir(site string, sys *unixSystem, arch unixArch) string {
	if len(sys.arches) == 1 {
		return filepath.Join(site, sys.name)
	}
	return filepath.Join(site, sys.name, arch.name)
}

// pkgRepoConf is the file in /usr/local/etc/pkg/repos that adds the
// repository, checking its signature with the key in key.pem.
func pkgRepoConf(a *App, sys *unixSystem, url string) []byte {
	repo := strings.TrimRight(url, "/") + "/" + sys.name
	if len(sys.arches) > 1 {
		repo += "/${ARCH}"
	}
	return []byte(fmt.Sprintf(`%s: {
  url: "%s",
  signature_type: "pubkey",
  pubkey: "/usr/local/etc/pkg/keys/%s.pem",
  enabled: yes
}
`, a.Name, repo, a.Name))
}

// pkgCompactManifest reads the +COMPACT_MANIFEST at the start of a
// package.
func pkgCompactManifest(data []byte) (map[string]any, error) {
	z, err := xz.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(z)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, errors.New("no +COMPACT_MANIFEST")
		}
		if h.Name == "+COMPACT_MANIFEST" {
			var m map[string]any
			if err := json.NewDecoder(tr).Decode(&m); err != nil {
				return nil, err
			}
			return m, nil
		}
	}
}

// pkgArchive is a repository index as pkg fetches it: a tar, xz-compressed,
// of its signature, then the index named member. legacy signs it as
// older pkg did.
func pkgArchive(member string, data []byte, s *signer, legacy bool) ([]byte, error) {
	sig, err := s.pkgSign(data, legacy)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	z, err := xz.NewWriter(&b)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(z)
	mtime := buildTime()
	if err := bsdTarFile(tw, "signature", sig, 0o644, mtime); err != nil {
		return nil, err
	}
	if err := bsdTarFile(tw, member, data, 0o644, mtime); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// rsaKey is the signing key's RSA private key, which pkg's signatures are
// made with.
func (s *signer) rsaKey() (*rsa.PrivateKey, error) {
	k, ok := s.entity.PrivateKey.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("the signing key isn't an RSA key, which pkg needs: make a new one with vero-repo key")
	}
	return k, nil
}

// rsaPublicPEM is the key's RSA public half as pkg reads it: PEM, "BEGIN
// PUBLIC KEY".
func (s *signer) rsaPublicPEM() ([]byte, error) {
	k, err := s.rsaKey()
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

// pkgSign signs data as pkg's "pubkey" repositories are signed: pkg
// hashes the file with SHA-256, and signs the hash's hex. pkg as FreeBSD
// has it now, on OpenSSL 3, signs that hex's own SHA-256, with RSA PKCS #1
// and SHA-256, and checks that first; older pkg, as DragonFly has, signs
// the hex itself, in a DigestInfo that says SHA-1, which pkg on OpenSSL 3
// can no longer check. legacy makes the older kind. pkg writes a NUL
// after the signature, which it then ignores; so does this.
func (s *signer) pkgSign(data []byte, legacy bool) ([]byte, error) {
	k, err := s.rsaKey()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	hexSum := []byte(hex.EncodeToString(sum[:]))
	var sig []byte
	if legacy {
		// DER: SEQUENCE { SEQUENCE { OID sha1, NULL }, OCTET STRING (64) }.
		info := append([]byte{0x30, 0x4d, 0x30, 0x09, 0x06, 0x05, 0x2b, 0x0e, 0x03, 0x02, 0x1a, 0x05, 0x00, 0x04, 0x40}, hexSum...)
		sig, err = rsa.SignPKCS1v15(rand.Reader, k, crypto.Hash(0), info)
	} else {
		hashed := sha256.Sum256(hexSum)
		sig, err = rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, hashed[:])
	}
	if err != nil {
		return nil, err
	}
	return append(sig, 0), nil
}

// keepNewest is the files in dir that match - the first submatch their
// version - newest first, removing all but the newest keep.
func keepNewest(dir string, match *regexp.Regexp, keep int) ([]string, map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	var files []string
	version := map[string]string{}
	for _, e := range entries {
		if m := match.FindStringSubmatch(e.Name()); m != nil {
			files = append(files, e.Name())
			version[e.Name()] = m[1]
		}
	}
	sort.Slice(files, func(i, j int) bool { return compareVersions(version[files[i]], version[files[j]]) > 0 })
	if keep > 0 && len(files) > keep {
		for _, f := range files[keep:] {
			if err := os.Remove(filepath.Join(dir, f)); err != nil {
				return nil, nil, err
			}
		}
		files = files[:keep]
	}
	return files, version, nil
}

func filepathRel(base, p string) string {
	rel, err := filepath.Rel(base, p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(rel)
}

// pkgsrcPackage is the pkgsrc package for a pkgsrc system: what it
// depends on.
func pkgsrcDepends(a *App, sys *unixSystem) []string {
	if sys.name == "illumos" {
		return a.GTK.Illumos.Depends
	}
	return a.GTK.NetBSD.Depends
}

// writePkgsrc writes a pkgsrc binary package, as pkg_create does: a tar,
// gzipped, of +CONTENTS (the packing list, which names each file
// relative to the prefix, with its MD5), +COMMENT, +DESC, +BUILD_INFO
// and +SIZE_PKG, then the files in the packing list's order.
func writePkgsrc(name string, a *App, sys *unixSystem, arch unixArch, files []treeFile) error {
	pkgname := a.Name + "-" + bsdVersion(a.Version)
	var contents bytes.Buffer
	fmt.Fprintf(&contents, "@name %s\n", pkgname)
	for _, d := range pkgsrcDepends(a, sys) {
		fmt.Fprintf(&contents, "@pkgdep %s\n", d)
	}
	fmt.Fprintf(&contents, "@cwd %s\n", sys.prefix)
	size := 0
	dirs := map[string]bool{}
	lib := path.Join(sys.prefix, "lib", a.Name)
	for _, f := range files {
		fmt.Fprintf(&contents, "%s\n@comment MD5:%x\n", strings.TrimPrefix(f.path, sys.prefix+"/"), md5.Sum(f.data))
		size += len(f.data)
		for d := path.Dir(f.path); strings.HasPrefix(d, lib); d = path.Dir(d) {
			dirs[d] = true
		}
	}
	var sortedDirs []string
	for d := range dirs {
		sortedDirs = append(sortedDirs, d)
	}
	// Deepest first, as they're removed.
	sort.Sort(sort.Reverse(sort.StringSlice(sortedDirs)))
	for _, d := range sortedDirs {
		fmt.Fprintf(&contents, "@pkgdir %s\n", strings.TrimPrefix(d, sys.prefix+"/"))
	}
	desc := strings.TrimSpace(a.Description)
	if desc == "" {
		desc = a.Summary
	}
	if a.Homepage != "" {
		desc += "\n\nHomepage:\n" + a.Homepage
	}
	meta := []treeFile{
		{"+CONTENTS", contents.Bytes(), 0o644},
		{"+COMMENT", []byte(a.Summary + "\n"), 0o644},
		{"+DESC", []byte(desc + "\n"), 0o644},
		{"+BUILD_INFO", pkgsrcBuildInfo(a, sys, arch), 0o644},
		{"+SIZE_PKG", []byte(fmt.Sprintf("%d\n", size)), 0o644},
	}
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	tw := tar.NewWriter(z)
	mtime := buildTime()
	for _, f := range meta {
		if err := bsdTarFile(tw, f.path, f.data, f.mode, mtime); err != nil {
			return err
		}
	}
	for _, f := range files {
		if err := bsdTarFile(tw, strings.TrimPrefix(f.path, sys.prefix+"/"), f.data, f.mode, mtime); err != nil {
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

// pkgsrcBuildInfo is a pkgsrc package's +BUILD_INFO: what pkg_add checks
// it was built for.
func pkgsrcBuildInfo(a *App, sys *unixSystem, arch unixArch) []byte {
	info := fmt.Sprintf("OPSYS=%s\nOS_VERSION=%s\nMACHINE_ARCH=%s\nPKGTOOLS_VERSION=20091115\nPKGPATH=misc/%s\nCATEGORIES=misc\n",
		sys.opsys, sys.osVersion, arch.name, a.Name)
	if a.Homepage != "" {
		info += "HOMEPAGE=" + a.Homepage + "\n"
	}
	return []byte(info)
}

// buildPkgsrcRepo makes site/SYSTEM/ARCH/All a repository pkgin installs
// and updates from: the packages, the newest keep of each architecture,
// and pkg_summary.gz, which lists the newest. pkgsrc packages aren't signed:
// HTTPS is what vouches for them. It returns the newest of each
// architecture.
func buildPkgsrcRepo(site string, sys *unixSystem, newPkgs []string, keep int, a *App, s *signer) (map[string]Download, error) {
	newest := map[string]Download{}
	for _, arch := range a.archesFor(sys) {
		all := filepath.Join(site, sys.name, arch.name, "All")
		suffix := "-" + sys.name + "-" + arch.name + ".tgz"
		for _, p := range newPkgs {
			base := filepath.Base(p)
			if !strings.HasSuffix(base, suffix) {
				continue
			}
			version := strings.TrimSuffix(strings.TrimPrefix(base, a.Name+"-"), suffix)
			file := a.Name + "-" + version + ".tgz"
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			// illumos's pkgsrc, as SmartOS sets it up, installs only
			// signed packages; NetBSD's, as it comes, takes either.
			if sys.name == "illumos" {
				if data, err = signPkgsrc(data, a.Name+"-"+version, file, s); err != nil {
					return nil, err
				}
			}
			if err := writeTree(filepath.Join(all, file), data, 0o644); err != nil {
				return nil, err
			}
		}
		files, versions, err := keepNewest(all, regexp.MustCompile(`^`+regexp.QuoteMeta(a.Name)+`-(.+)\.tgz$`), keep)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			continue
		}
		// Only the newest, as pkgsrc's own summaries list one version of
		// each package: pkgin, given two, can keep the older. The others
		// stay in the folder, for anyone who wants one by name.
		data, err := os.ReadFile(filepath.Join(all, files[0]))
		if err != nil {
			return nil, err
		}
		summary, err := pkgsrcSummary(files[0], data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", files[0], err)
		}
		var gz bytes.Buffer
		z := gzip.NewWriter(&gz)
		z.Write(summary)
		z.Close()
		if err := os.WriteFile(filepath.Join(all, "pkg_summary.gz"), gz.Bytes(), 0o644); err != nil {
			return nil, err
		}
		newest[arch.name] = Download{Version: versions[files[0]], URL: filepathRel(site, filepath.Join(all, files[0]))}
	}
	if sys.name == "illumos" && len(newest) > 0 {
		key, err := s.gpgKeyring()
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(site, sys.name, "key.gpg"), key, 0o644); err != nil {
			return nil, err
		}
	}
	return newest, nil
}

// pkgsrcSummary is a package's entry in pkg_summary: what pkgin knows of
// it, from the package's own files.
func pkgsrcSummary(file string, data []byte) ([]byte, error) {
	inner := data
	if bytes.HasPrefix(data, []byte("!<arch>\n")) {
		var err error
		if inner, err = arLast(data); err != nil {
			return nil, err
		}
	}
	z, err := gzip.NewReader(bytes.NewReader(inner))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(z)
	meta := map[string]string{}
	for {
		h, err := tr.Next()
		if err != nil || !strings.HasPrefix(h.Name, "+") {
			break
		}
		var b bytes.Buffer
		b.ReadFrom(tr)
		meta[h.Name] = b.String()
	}
	if meta["+CONTENTS"] == "" {
		return nil, errors.New("no +CONTENTS")
	}
	var out bytes.Buffer
	for _, line := range strings.Split(meta["+CONTENTS"], "\n") {
		switch {
		case strings.HasPrefix(line, "@name "):
			fmt.Fprintf(&out, "PKGNAME=%s\n", strings.TrimPrefix(line, "@name "))
		case strings.HasPrefix(line, "@pkgdep "):
			fmt.Fprintf(&out, "DEPENDS=%s\n", strings.TrimPrefix(line, "@pkgdep "))
		}
	}
	fmt.Fprintf(&out, "COMMENT=%s\n", strings.TrimSpace(meta["+COMMENT"]))
	fmt.Fprintf(&out, "SIZE_PKG=%s\n", strings.TrimSpace(meta["+SIZE_PKG"]))
	fmt.Fprintf(&out, "BUILD_DATE=%s\n", buildTime().UTC().Format("2006-01-02 15:04:05 -0700"))
	for _, line := range strings.Split(strings.TrimSpace(meta["+BUILD_INFO"]), "\n") {
		if line != "" {
			out.WriteString(line + "\n")
		}
	}
	for _, line := range strings.Split(strings.TrimRight(meta["+DESC"], "\n"), "\n") {
		fmt.Fprintf(&out, "DESCRIPTION=%s\n", line)
	}
	sum := sha256.Sum256(data)
	fmt.Fprintf(&out, "FILE_NAME=%s\nFILE_SIZE=%d\nFILE_CKSUM=sha256 %x\n\n", file, len(data), sum)
	return out.Bytes(), nil
}

// fileArch is arch as a file name has it: DragonFly's "x86:64" as
// "x86_64".
func fileArch(arch unixArch) string { return strings.ReplaceAll(arch.name, ":", "_") }

// signPkgsrc signs a pkgsrc package as pkg_admin gpg-sign-package does,
// for a pkgsrc that installs only signed packages, as SmartOS's does: a
// BSD ar archive of +PKG_HASH - the SHA-512 of each 64 KiB of the
// package - its detached OpenPGP signature, and the package, named file.
func signPkgsrc(pkg []byte, pkgname, file string, s *signer) ([]byte, error) {
	var hash bytes.Buffer
	fmt.Fprintf(&hash, "pkgsrc signature\n\nversion: 1\npkgname: %s\nalgorithm: SHA512\nblock size: 65536\nfile size: %d\n\n", pkgname, len(pkg))
	for i := 0; i < len(pkg); i += 65536 {
		sum := sha512.Sum512(pkg[i:min(i+65536, len(pkg))])
		hash.WriteString(hex.EncodeToString(sum[:]) + "\n")
	}
	hash.WriteString("end pkgsrc signature\n")
	// SHA-512: pkg_add checks the signature as a clearsigned message that
	// says its hash is.
	// As gpg makes it: no salt notation, which netpgp, which pkg_add
	// checks with, doesn't expect; and a newline at the end.
	var sig bytes.Buffer
	plain := false
	cfg := &packet.Config{DefaultHash: crypto.SHA512, NonDeterministicSignaturesViaNotation: &plain}
	if err := openpgp.ArmoredDetachSign(&sig, s.entity, bytes.NewReader(hash.Bytes()), cfg); err != nil {
		return nil, err
	}
	sig.WriteString("\n")
	var out bytes.Buffer
	out.WriteString("!<arch>\n")
	mtime := buildTime().Unix()
	for _, m := range []struct {
		name string
		data []byte
	}{{"+PKG_HASH", hash.Bytes()}, {"+PKG_GPG_SIGNATURE", sig.Bytes()}, {file, pkg}} {
		// BSD ar: a name longer than 16 goes before the data, as #1/LEN.
		name, data := m.name, m.data
		if len(name) > 16 || strings.Contains(name, " ") {
			data = append([]byte(name), data...)
			name = fmt.Sprintf("#1/%d", len(m.name))
		}
		fmt.Fprintf(&out, "%-16s%-12d%-6d%-6d%-8s%-10d`\n", name, mtime, 0, 0, "100644", len(data))
		out.Write(data)
		if len(data)%2 == 1 {
			out.WriteByte('\n')
		}
	}
	return out.Bytes(), nil
}

// gpgKeyring is the public key as pkgsrc's keyring holds it: binary
// OpenPGP, which can be added to the end of one.
func (s *signer) gpgKeyring() ([]byte, error) {
	var b bytes.Buffer
	if err := s.entity.Serialize(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// arLast is the last member of a BSD ar archive: a signed pkgsrc
// package's package.
func arLast(data []byte) ([]byte, error) {
	var last []byte
	for pos := 8; pos+60 <= len(data); {
		h := data[pos : pos+60]
		var size int
		if _, err := fmt.Sscan(strings.TrimSpace(string(h[48:58])), &size); err != nil || pos+60+size > len(data) {
			return nil, errors.New("a damaged ar archive")
		}
		body := data[pos+60 : pos+60+size]
		if name := strings.TrimSpace(string(h[0:16])); strings.HasPrefix(name, "#1/") {
			var n int
			fmt.Sscan(name[3:], &n)
			body = body[n:]
		}
		last = body
		pos += 60 + size + size%2
	}
	if last == nil {
		return nil, errors.New("an empty ar archive")
	}
	return last, nil
}
