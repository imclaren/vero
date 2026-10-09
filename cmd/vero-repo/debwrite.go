package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

// packageDeb builds the app's .deb for each architecture, for Debian and
// Ubuntu, in Go: the same files as the rpm, under /usr. workers are the
// worker built for each of Go's architectures.
func packageDeb(a *App, workers map[string]string, out string) error {
	for _, arch := range a.linuxArches("deb") {
		files, err := linuxTree(a, "/usr", workers[arch.goarch], a.Name)
		if err != nil {
			return err
		}
		control := debControl(a, arch.name, files)
		name := filepath.Join(out, fmt.Sprintf("%s_%s_%s.deb", a.Name, a.Version, arch.name))
		if err := writeDeb(name, control, nil, files); err != nil {
			return err
		}
		fmt.Println("built", name)
	}
	return nil
}

// debControl is the control file of the app's .deb for arch.
func debControl(a *App, arch string, files []treeFile) []byte {
	depends := a.GTK.Deb.Depends
	if depends == "" {
		depends = "python3 (>= 3.10), python3-gi, gir1.2-gtk-4.0"
	}
	section := a.GTK.Deb.Section
	if section == "" {
		section = "utils"
	}
	fields := []Field{
		{"Package", a.Name},
		{"Version", a.Version},
		{"Architecture", arch},
		{"Maintainer", a.Publisher},
		{"Installed-Size", fmt.Sprint(installedSize(files))},
		{"Depends", depends},
	}
	if a.GTK.Deb.Recommends != "" {
		fields = append(fields, Field{"Recommends", a.GTK.Deb.Recommends})
	}
	fields = append(fields, Field{"Section", section}, Field{"Priority", "optional"})
	if a.Homepage != "" {
		fields = append(fields, Field{"Homepage", a.Homepage})
	}
	fields = append(fields, Field{"Description", a.Summary + debDescription(a.Description)})
	var b bytes.Buffer
	writeControl(&b, fields)
	return b.Bytes()
}

// debDescription is a long description as a control file has it after
// the summary: each line indented, wrapped at 72, a blank line a dot.
func debDescription(text string) string {
	var out strings.Builder
	paras := strings.Split(strings.TrimSpace(text), "\n\n")
	for i, p := range paras {
		words := strings.Fields(p)
		if len(words) == 0 {
			continue
		}
		if i > 0 && out.Len() > 0 {
			out.WriteString("\n .")
		}
		line := ""
		for _, w := range words {
			if line != "" && len(line)+1+len(w) > 72 {
				out.WriteString("\n " + line)
				line = ""
			}
			if line != "" {
				line += " "
			}
			line += w
		}
		out.WriteString("\n " + line)
	}
	return out.String()
}

// installedSize is what dpkg says a package takes once installed, in KiB:
// each file rounded up to a KiB, and one for each folder.
func installedSize(files []treeFile) int64 {
	var size int64
	dirs := map[string]bool{}
	for _, f := range files {
		size += (int64(len(f.data)) + 1023) / 1024
		for d := path.Dir(f.path); d != "/" && !dirs[d]; d = path.Dir(d) {
			dirs[d] = true
			size++
		}
	}
	return size
}

// controlFile is a file beside control in a .deb's control.tar, such as
// postinst.
type controlFile struct {
	name string
	data []byte
	mode fs.FileMode
}

// writeDeb writes a .deb, as dpkg-deb --root-owner-group --build does: an
// ar archive of debian-binary, control.tar.gz (control, md5sums and any
// scripts) and data.tar.xz (the files, every one owned by root).
func writeDeb(name string, control []byte, scripts []controlFile, files []treeFile) error {
	mtime := buildTime()
	var data bytes.Buffer
	xw, err := xz.NewWriter(&data)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(xw)
	dirs := map[string]bool{}
	var md5sums bytes.Buffer
	sorted := append([]treeFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })
	if err := tarDir(tw, "./", mtime); err != nil {
		return err
	}
	for _, f := range sorted {
		// Each folder once, before what's in it.
		var parents []string
		for d := path.Dir(f.path); d != "/" && !dirs[d]; d = path.Dir(d) {
			parents = append(parents, d)
			dirs[d] = true
		}
		for i := len(parents) - 1; i >= 0; i-- {
			if err := tarDir(tw, "."+parents[i]+"/", mtime); err != nil {
				return err
			}
		}
		if err := tarFile(tw, "."+f.path, f.data, f.mode, mtime); err != nil {
			return err
		}
		fmt.Fprintf(&md5sums, "%x  %s\n", md5.Sum(f.data), strings.TrimPrefix(f.path, "/"))
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := xw.Close(); err != nil {
		return err
	}

	var ctl bytes.Buffer
	gw := gzip.NewWriter(&ctl)
	tw = tar.NewWriter(gw)
	if err := tarDir(tw, "./", mtime); err != nil {
		return err
	}
	if err := tarFile(tw, "./control", control, 0o644, mtime); err != nil {
		return err
	}
	if err := tarFile(tw, "./md5sums", md5sums.Bytes(), 0o644, mtime); err != nil {
		return err
	}
	for _, s := range scripts {
		if err := tarFile(tw, "./"+s.name, s.data, s.mode, mtime); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gw.Close(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	out, err := os.Create(name)
	if err != nil {
		return err
	}
	ar := arWriter{w: out, mtime: mtime}
	ar.start()
	ar.add("debian-binary", []byte("2.0\n"))
	ar.add("control.tar.gz", ctl.Bytes())
	ar.add("data.tar.xz", data.Bytes())
	if ar.err != nil {
		out.Close()
		return ar.err
	}
	return out.Close()
}

// buildTime is the time packages give their files: SOURCE_DATE_EPOCH, for
// a build that's the same each time, or now.
func buildTime() time.Time {
	if s := os.Getenv("SOURCE_DATE_EPOCH"); s != "" {
		var n int64
		if _, err := fmt.Sscan(s, &n); err == nil {
			return time.Unix(n, 0)
		}
	}
	return now().Truncate(time.Second)
}

func tarDir(tw *tar.Writer, name string, mtime time.Time) error {
	return tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir, Name: name, Mode: 0o755, ModTime: mtime,
		Uname: "root", Gname: "root", Format: tar.FormatGNU,
	})
}

func tarFile(tw *tar.Writer, name string, data []byte, mode fs.FileMode, mtime time.Time) error {
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg, Name: name, Mode: int64(mode.Perm()), Size: int64(len(data)), ModTime: mtime,
		Uname: "root", Gname: "root", Format: tar.FormatGNU,
	}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// arWriter writes an ar archive, the kind a .deb is.
type arWriter struct {
	w     io.Writer
	mtime time.Time
	err   error
}

func (a *arWriter) start() { a.write([]byte("!<arch>\n")) }

func (a *arWriter) add(name string, data []byte) {
	a.write([]byte(fmt.Sprintf("%-16s%-12d%-6d%-6d%-8s%-10d`\n", name, a.mtime.Unix(), 0, 0, "100644", len(data))))
	a.write(data)
	if len(data)%2 == 1 {
		a.write([]byte{'\n'})
	}
}

func (a *arWriter) write(b []byte) {
	if a.err == nil {
		_, a.err = a.w.Write(b)
	}
}

// debFromFolder builds the .deb of a folder laid out for dpkg-deb: DEBIAN/
// with control and any scripts, and the files as they're installed. It's
// what package-linux.sh calls, in place of dpkg-deb.
func debFromFolder(dir, name string) error {
	control, err := os.ReadFile(filepath.Join(dir, "DEBIAN", "control"))
	if err != nil {
		return fmt.Errorf("%s has no DEBIAN/control", dir)
	}
	var scripts []controlFile
	var files []treeFile
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, "DEBIAN/") {
			if rel != "DEBIAN/control" {
				scripts = append(scripts, controlFile{strings.TrimPrefix(rel, "DEBIAN/"), data, mode})
			}
			return nil
		}
		files = append(files, treeFile{"/" + rel, data, mode})
		return nil
	})
	if err != nil {
		return err
	}
	// dpkg-deb adds Installed-Size when the control file doesn't say it.
	c, err := parseControl(control)
	if err != nil {
		return err
	}
	if c.Get("Installed-Size") == "" {
		var b bytes.Buffer
		for _, f := range c.Fields {
			writeControl(&b, []Field{f})
			if f.Name == "Maintainer" {
				writeControl(&b, []Field{{"Installed-Size", fmt.Sprint(installedSize(files))}})
			}
		}
		control = b.Bytes()
	}
	return writeDeb(name, control, scripts, files)
}
