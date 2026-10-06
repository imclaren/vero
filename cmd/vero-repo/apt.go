package main

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// now is the time a release is dated, which a test fixes.
var now = time.Now

// debFile is a package in the repository's pool.
type debFile struct {
	path    string // in the pool
	control *Control
}

// buildApt makes site/apt a Debian repository holding the packages in the
// pool already there and the new ones, the newest keep of each package
// and architecture, with its index signed by s. It returns the newest
// package for each architecture.
func buildApt(site string, newDebs []string, keep int, a *App, s *signer) (map[string]*debFile, error) {
	root := filepath.Join(site, "apt")
	for _, deb := range newDebs {
		c, err := readControl(deb)
		if err != nil {
			return nil, err
		}
		dest := filepath.Join(root, poolPath(c.Get("Package"), filepath.Base(deb)))
		if err := copyFile(deb, dest); err != nil {
			return nil, err
		}
	}
	var all []*debFile
	err := filepath.WalkDir(filepath.Join(root, "pool"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".deb") {
			return nil
		}
		c, err := readControl(path)
		if err != nil {
			return err
		}
		all = append(all, &debFile{path, c})
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The newest first, in each package and architecture; the rest gone.
	groups := map[string][]*debFile{}
	for _, d := range all {
		k := d.control.Get("Package") + "/" + d.control.Get("Architecture")
		groups[k] = append(groups[k], d)
	}
	newest := map[string]*debFile{}
	var kept []*debFile
	for _, g := range groups {
		sort.Slice(g, func(i, j int) bool {
			return compareVersions(g[i].control.Get("Version"), g[j].control.Get("Version")) > 0
		})
		for i, d := range g {
			if keep > 0 && i >= keep {
				if err := os.Remove(d.path); err != nil {
					return nil, err
				}
				continue
			}
			kept = append(kept, d)
		}
		if g[0].control.Get("Package") == a.Name {
			newest[g[0].control.Get("Architecture")] = g[0]
		}
	}

	// One Packages index for each architecture; "all" goes in each.
	arches := map[string]bool{"amd64": true, "arm64": true}
	for _, d := range kept {
		if arch := d.control.Get("Architecture"); arch != "all" {
			arches[arch] = true
		}
	}
	var archList []string
	for arch := range arches {
		archList = append(archList, arch)
	}
	sort.Strings(archList)
	dist := filepath.Join(root, "dists", "stable")
	var indexes []string // relative to dist
	for _, arch := range archList {
		var entries []*debFile
		for _, d := range kept {
			if a := d.control.Get("Architecture"); a == arch || a == "all" {
				entries = append(entries, d)
			}
		}
		sort.Slice(entries, func(i, j int) bool {
			pi, pj := entries[i].control.Get("Package"), entries[j].control.Get("Package")
			if pi != pj {
				return pi < pj
			}
			return compareVersions(entries[i].control.Get("Version"), entries[j].control.Get("Version")) < 0
		})
		var index bytes.Buffer
		for i, d := range entries {
			if i > 0 {
				index.WriteByte('\n')
			}
			entry, err := packagesEntry(root, d)
			if err != nil {
				return nil, err
			}
			index.Write(entry)
		}
		dir := filepath.Join("main", "binary-"+arch)
		if err := os.MkdirAll(filepath.Join(dist, dir), 0o755); err != nil {
			return nil, err
		}
		var gz bytes.Buffer
		z := gzip.NewWriter(&gz)
		z.Write(index.Bytes())
		z.Close()
		for name, data := range map[string][]byte{"Packages": index.Bytes(), "Packages.gz": gz.Bytes()} {
			if err := os.WriteFile(filepath.Join(dist, dir, name), data, 0o644); err != nil {
				return nil, err
			}
			indexes = append(indexes, filepath.ToSlash(filepath.Join(dir, name)))
		}
	}
	sort.Strings(indexes)

	release, err := releaseFile(dist, indexes, archList, a)
	if err != nil {
		return nil, err
	}
	signed, err := s.clearsign(release)
	if err != nil {
		return nil, err
	}
	detached, err := s.detach(release)
	if err != nil {
		return nil, err
	}
	for name, data := range map[string][]byte{"Release": release, "InRelease": signed, "Release.gpg": detached} {
		if err := os.WriteFile(filepath.Join(dist, name), data, 0o644); err != nil {
			return nil, err
		}
	}
	return newest, nil
}

// poolPath is where a package's file goes, as Debian's own archive lays
// them out: pool/main/v/vero-example/, or pool/main/libf/libfoo/.
func poolPath(pkg, file string) string {
	prefix := pkg[:1]
	if strings.HasPrefix(pkg, "lib") && len(pkg) > 3 {
		prefix = pkg[:4]
	}
	return filepath.Join("pool", "main", prefix, pkg, file)
}

// packagesEntry is a package's paragraph in the Packages index: its
// control file, then where it is, its size and its hashes.
func packagesEntry(root string, d *debFile) ([]byte, error) {
	data, err := os.ReadFile(d.path)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, d.path)
	if err != nil {
		return nil, err
	}
	m, s1, s256 := md5.Sum(data), sha1.Sum(data), sha256.Sum256(data)
	fields := append([]Field{}, d.control.Fields...)
	fields = append(fields,
		Field{"Filename", filepath.ToSlash(rel)},
		Field{"Size", fmt.Sprint(len(data))},
		Field{"MD5sum", hex.EncodeToString(m[:])},
		Field{"SHA1", hex.EncodeToString(s1[:])},
		Field{"SHA256", hex.EncodeToString(s256[:])},
	)
	var b bytes.Buffer
	writeControl(&b, fields)
	return b.Bytes(), nil
}

// releaseFile is the suite's Release: what it is, and the size and hashes
// of each index, which apt checks against what it downloads.
func releaseFile(dist string, indexes, arches []string, a *App) ([]byte, error) {
	var b bytes.Buffer
	writeControl(&b, []Field{
		{"Origin", a.DisplayName},
		{"Label", a.DisplayName},
		{"Suite", "stable"},
		{"Codename", "stable"},
		{"Date", now().UTC().Format("Mon, 02 Jan 2006 15:04:05 UTC")},
		{"Architectures", strings.Join(arches, " ")},
		{"Components", "main"},
		{"Description", a.Summary},
	})
	type sums struct{ md5, sha256 string }
	sizes, hashes := map[string]int{}, map[string]sums{}
	for _, name := range indexes {
		data, err := os.ReadFile(filepath.Join(dist, name))
		if err != nil {
			return nil, err
		}
		m, s := md5.Sum(data), sha256.Sum256(data)
		sizes[name], hashes[name] = len(data), sums{hex.EncodeToString(m[:]), hex.EncodeToString(s[:])}
	}
	b.WriteString("MD5Sum:\n")
	for _, name := range indexes {
		fmt.Fprintf(&b, " %s %d %s\n", hashes[name].md5, sizes[name], name)
	}
	b.WriteString("SHA256:\n")
	for _, name := range indexes {
		fmt.Fprintf(&b, " %s %d %s\n", hashes[name].sha256, sizes[name], name)
	}
	return b.Bytes(), nil
}

func copyFile(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := to + ".partial"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, to)
}
