package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/cavaliergopher/rpm"
)

// rpm's signature header tags, and the region tag that heads it.
const (
	sigRegion = 62   // RPMTAG_HEADERSIGNATURES
	sigRSA    = 268  // a signature of the header
	sigPGP    = 1002 // a signature of the header and the payload
)

// rpm's types of header value, by how long they are.
const (
	rpmNull = iota
	rpmChar
	rpmInt8
	rpmInt16
	rpmInt32
	rpmInt64
	rpmString
	rpmBin
	rpmStringArray
	rpmI18NString
)

// headerEntry is one tag of an rpm header, with its value as it's stored.
type headerEntry struct {
	tag, typ, count int
	data            []byte
}

// signRPM signs the rpm at path in place, as rpmsign --addsign does: an
// OpenPGP signature of its header, which rpm checks, and one of its header
// and payload, which older rpm checks, are added to its signature header.
func signRPM(path string, s *signer) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	const lead = 96
	sigEntries, sigLen, err := readHeader(b, lead)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	start := lead + sigLen + (8-sigLen%8)%8
	_, mainLen, err := readHeader(b, start)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	header := b[start : start+mainLen]
	payload := b[start+mainLen:]
	sign := func(data []byte) ([]byte, error) {
		var out bytes.Buffer
		if err := openpgp.DetachSign(&out, s.entity, bytes.NewReader(data), signConfig); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}
	headerSig, err := sign(header)
	if err != nil {
		return err
	}
	bothSig, err := sign(append(append([]byte(nil), header...), payload...))
	if err != nil {
		return err
	}
	var kept []headerEntry
	for _, e := range sigEntries {
		if e.tag != sigRSA && e.tag != sigPGP {
			kept = append(kept, e)
		}
	}
	kept = append(kept,
		headerEntry{sigRSA, rpmBin, len(headerSig), headerSig},
		headerEntry{sigPGP, rpmBin, len(bothSig), bothSig})
	sig := writeHeader(sigRegion, kept)
	var out bytes.Buffer
	out.Write(b[:lead])
	out.Write(sig)
	out.Write(make([]byte, (8-len(sig)%8)%8))
	out.Write(header)
	out.Write(payload)
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// readHeader reads the rpm header at offset in b: its entries, without
// the region tag that heads it, and how long it is.
func readHeader(b []byte, offset int) ([]headerEntry, int, error) {
	if len(b) < offset+16 || !bytes.Equal(b[offset:offset+4], []byte{0x8e, 0xad, 0xe8, 0x01}) {
		return nil, 0, errors.New("not an rpm header")
	}
	n := int(binary.BigEndian.Uint32(b[offset+8:]))
	size := int(binary.BigEndian.Uint32(b[offset+12:]))
	length := 16 + n*16 + size
	if len(b) < offset+length {
		return nil, 0, errors.New("a damaged rpm header")
	}
	store := b[offset+16+n*16 : offset+length]
	var entries []headerEntry
	for i := 0; i < n; i++ {
		at := offset + 16 + i*16
		tag := int(int32(binary.BigEndian.Uint32(b[at:])))
		typ := int(binary.BigEndian.Uint32(b[at+4:]))
		off := int(int32(binary.BigEndian.Uint32(b[at+8:])))
		count := int(binary.BigEndian.Uint32(b[at+12:]))
		if tag >= 61 && tag <= 63 { // a region: rebuilt when written
			continue
		}
		if off < 0 || off > len(store) {
			return nil, 0, errors.New("a damaged rpm header")
		}
		l, err := valueLength(store[off:], typ, count)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, headerEntry{tag, typ, count, store[off : off+l]})
	}
	return entries, length, nil
}

// valueLength is how many bytes a header value of type typ and count takes.
func valueLength(b []byte, typ, count int) (int, error) {
	switch typ {
	case rpmChar, rpmInt8, rpmBin:
		return count, nil
	case rpmInt16:
		return 2 * count, nil
	case rpmInt32:
		return 4 * count, nil
	case rpmInt64:
		return 8 * count, nil
	case rpmString, rpmStringArray, rpmI18NString:
		n := 0
		for i := 0; i < count; i++ {
			end := bytes.IndexByte(b[n:], 0)
			if end < 0 {
				return 0, errors.New("a damaged rpm header")
			}
			n += end + 1
		}
		return n, nil
	}
	return 0, fmt.Errorf("an rpm header value of type %d", typ)
}

// writeHeader writes an rpm header headed by region: its entries in tag
// order, integers aligned as rpm needs, and the region's trailer last.
func writeHeader(region int, entries []headerEntry) []byte {
	sort.Slice(entries, func(i, j int) bool { return entries[i].tag < entries[j].tag })
	var store bytes.Buffer
	offsets := make([]int, len(entries))
	for i, e := range entries {
		align := map[int]int{rpmInt16: 2, rpmInt32: 4, rpmInt64: 8}[e.typ]
		if align > 0 && store.Len()%align != 0 {
			store.Write(make([]byte, align-store.Len()%align))
		}
		offsets[i] = store.Len()
		store.Write(e.data)
	}
	n := len(entries) + 1
	trailer := store.Len()
	binary.Write(&store, binary.BigEndian, []int32{int32(region), rpmBin, int32(-16 * n), 16})
	var out bytes.Buffer
	out.Write([]byte{0x8e, 0xad, 0xe8, 0x01, 0, 0, 0, 0})
	binary.Write(&out, binary.BigEndian, []int32{int32(n), int32(store.Len())})
	binary.Write(&out, binary.BigEndian, []int32{int32(region), rpmBin, int32(trailer), 16})
	for i, e := range entries {
		binary.Write(&out, binary.BigEndian, []int32{int32(e.tag), int32(e.typ), int32(offsets[i]), int32(e.count)})
	}
	out.Write(store.Bytes())
	return out.Bytes()
}

// The repodata files, as createrepo_c writes them.
type repoPackage struct {
	XMLName  xml.Name    `xml:"package"`
	Type     string      `xml:"type,attr"`
	Name     string      `xml:"name"`
	Arch     string      `xml:"arch"`
	Version  repoVersion `xml:"version"`
	Checksum struct {
		Type  string `xml:"type,attr"`
		PkgID string `xml:"pkgid,attr"`
		Value string `xml:",chardata"`
	} `xml:"checksum"`
	Summary     string `xml:"summary"`
	Description string `xml:"description"`
	Packager    string `xml:"packager"`
	URL         string `xml:"url"`
	Time        struct {
		File  int64 `xml:"file,attr"`
		Build int64 `xml:"build,attr"`
	} `xml:"time"`
	Size struct {
		Package   int64  `xml:"package,attr"`
		Installed uint64 `xml:"installed,attr"`
		Archive   uint64 `xml:"archive,attr"`
	} `xml:"size"`
	Location struct {
		Href string `xml:"href,attr"`
	} `xml:"location"`
	Format repoFormat `xml:"format"`
}

type repoVersion struct {
	Epoch int    `xml:"epoch,attr"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr"`
}

type repoFormat struct {
	License     string `xml:"rpm:license"`
	Vendor      string `xml:"rpm:vendor"`
	Group       string `xml:"rpm:group"`
	BuildHost   string `xml:"rpm:buildhost"`
	SourceRPM   string `xml:"rpm:sourcerpm"`
	HeaderRange struct {
		Start int `xml:"start,attr"`
		End   int `xml:"end,attr"`
	} `xml:"rpm:header-range"`
	Provides   *repoDeps      `xml:"rpm:provides,omitempty"`
	Requires   *repoDeps      `xml:"rpm:requires,omitempty"`
	Recommends *repoDeps      `xml:"rpm:recommends,omitempty"`
	Files      []repodataFile `xml:"file"`
}

type repoDeps struct {
	Entries []repoDep `xml:"rpm:entry"`
}

type repoDep struct {
	Name  string `xml:"name,attr"`
	Flags string `xml:"flags,attr,omitempty"`
	Epoch string `xml:"epoch,attr,omitempty"`
	Ver   string `xml:"ver,attr,omitempty"`
	Rel   string `xml:"rel,attr,omitempty"`
	Pre   string `xml:"pre,attr,omitempty"`
}

type repodataFile struct {
	Type string `xml:"type,attr,omitempty"`
	Path string `xml:",chardata"`
}

// writeRepodata writes root/repodata for the rpms in root/packages, as
// createrepo_c does: primary, filelists and other, gzipped, named by their
// checksums, and repomd.xml, which lists them.
func writeRepodata(root string) error {
	dir := filepath.Join(root, "packages")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var primary, filelists, other bytes.Buffer
	count := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".rpm") {
			continue
		}
		file := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		p, err := rpm.Read(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		id := hex.EncodeToString(sum[:])
		var r repoPackage
		r.Type, r.Name, r.Arch = "rpm", p.Name(), p.Architecture()
		r.Version = repoVersion{p.Epoch(), p.Version(), p.Release()}
		r.Checksum.Type, r.Checksum.PkgID, r.Checksum.Value = "sha256", "YES", id
		r.Summary, r.Description, r.Packager, r.URL = p.Summary(), p.Description(), p.Packager(), p.URL()
		r.Time.File, r.Time.Build = info.ModTime().Unix(), p.BuildTime().Unix()
		r.Size.Package, r.Size.Installed, r.Size.Archive = int64(len(data)), p.Size(), p.ArchiveSize()
		r.Location.Href = "packages/" + e.Name()
		f := &r.Format
		f.License, f.Vendor, f.BuildHost, f.SourceRPM = p.License(), p.Vendor(), p.BuildHost(), p.SourceRPM()
		if g := p.Groups(); len(g) > 0 {
			f.Group = g[0]
		}
		f.HeaderRange.Start, f.HeaderRange.End = p.HeaderRange()
		f.Provides = repoDependencies(p.Provides())
		f.Requires = repoDependencies(p.Requires())
		f.Recommends = repoDependencies(p.Recommends())
		var all []repodataFile
		for _, fi := range p.Files() {
			rf := repodataFile{Path: fi.Name()}
			if fi.IsDir() {
				rf.Type = "dir"
			}
			all = append(all, rf)
			// primary lists only what dnf looks for most: commands, and
			// files in /etc.
			if strings.HasPrefix(fi.Name(), "/etc/") || strings.Contains(fi.Name(), "bin/") {
				f.Files = append(f.Files, rf)
			}
		}
		x, err := xml.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		primary.Write(x)
		primary.WriteString("\n")
		ver := fmt.Sprintf(`<version epoch="%d" ver="%s" rel="%s"/>`, p.Epoch(), xmlText(p.Version()), xmlText(p.Release()))
		fmt.Fprintf(&filelists, "<package pkgid=\"%s\" name=\"%s\" arch=\"%s\">\n  %s\n", id, xmlText(p.Name()), xmlText(p.Architecture()), ver)
		for _, rf := range all {
			if rf.Type != "" {
				fmt.Fprintf(&filelists, "  <file type=\"%s\">%s</file>\n", rf.Type, xmlText(rf.Path))
			} else {
				fmt.Fprintf(&filelists, "  <file>%s</file>\n", xmlText(rf.Path))
			}
		}
		filelists.WriteString("</package>\n")
		fmt.Fprintf(&other, "<package pkgid=\"%s\" name=\"%s\" arch=\"%s\">\n  %s\n</package>\n", id, xmlText(p.Name()), xmlText(p.Architecture()), ver)
		count++
	}
	files := map[string][]byte{
		"primary": []byte(fmt.Sprintf("%s<metadata xmlns=\"http://linux.duke.edu/metadata/common\" xmlns:rpm=\"http://linux.duke.edu/metadata/rpm\" packages=\"%d\">\n%s</metadata>\n",
			xml.Header, count, primary.String())),
		"filelists": []byte(fmt.Sprintf("%s<filelists xmlns=\"http://linux.duke.edu/metadata/filelists\" packages=\"%d\">\n%s</filelists>\n",
			xml.Header, count, filelists.String())),
		"other": []byte(fmt.Sprintf("%s<otherdata xmlns=\"http://linux.duke.edu/metadata/other\" packages=\"%d\">\n%s</otherdata>\n",
			xml.Header, count, other.String())),
	}
	repodata := filepath.Join(root, "repodata")
	if err := os.RemoveAll(repodata); err != nil {
		return err
	}
	if err := os.MkdirAll(repodata, 0o755); err != nil {
		return err
	}
	stamp := now().Unix()
	var md bytes.Buffer
	fmt.Fprintf(&md, "%s<repomd xmlns=\"http://linux.duke.edu/metadata/repo\" xmlns:rpm=\"http://linux.duke.edu/metadata/rpm\">\n  <revision>%d</revision>\n", xml.Header, stamp)
	for _, kind := range []string{"primary", "filelists", "other"} {
		open := files[kind]
		var gz bytes.Buffer
		w := gzip.NewWriter(&gz)
		w.Write(open)
		w.Close()
		sum := sha256.Sum256(gz.Bytes())
		openSum := sha256.Sum256(open)
		name := fmt.Sprintf("%x-%s.xml.gz", sum, kind)
		if err := os.WriteFile(filepath.Join(repodata, name), gz.Bytes(), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(&md, `  <data type="%s">
    <checksum type="sha256">%x</checksum>
    <open-checksum type="sha256">%x</open-checksum>
    <location href="repodata/%s"/>
    <timestamp>%d</timestamp>
    <size>%d</size>
    <open-size>%d</open-size>
  </data>
`, kind, sum, openSum, name, stamp, gz.Len(), len(open))
	}
	md.WriteString("</repomd>\n")
	return os.WriteFile(filepath.Join(repodata, "repomd.xml"), md.Bytes(), 0o644)
}

// repoDependencies are an rpm's dependencies as repodata lists them,
// without rpm's own (rpmlib(...)), which createrepo leaves out.
func repoDependencies(deps []rpm.Dependency) *repoDeps {
	var out repoDeps
	seen := map[string]bool{}
	for _, d := range deps {
		if strings.HasPrefix(d.Name(), "rpmlib(") {
			continue
		}
		e := repoDep{Name: d.Name()}
		switch d.Flags() & (rpm.DepFlagLesser | rpm.DepFlagGreater | rpm.DepFlagEqual) {
		case rpm.DepFlagEqual:
			e.Flags = "EQ"
		case rpm.DepFlagLesser:
			e.Flags = "LT"
		case rpm.DepFlagGreater:
			e.Flags = "GT"
		case rpm.DepFlagLesser | rpm.DepFlagEqual:
			e.Flags = "LE"
		case rpm.DepFlagGreater | rpm.DepFlagEqual:
			e.Flags = "GE"
		}
		if e.Flags != "" {
			e.Epoch = fmt.Sprint(d.Epoch())
			e.Ver, e.Rel = d.Version(), d.Release()
		}
		if d.Flags()&(rpm.DepFlagPrereq|rpm.DepFlagScriptPre|rpm.DepFlagScriptPost) != 0 {
			e.Pre = "1"
		}
		key := e.Name + "|" + e.Flags + "|" + e.Ver
		if seen[key] {
			continue
		}
		seen[key] = true
		out.Entries = append(out.Entries, e)
	}
	if len(out.Entries) == 0 {
		return nil
	}
	return &out
}

func xmlText(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}
