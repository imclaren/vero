package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// Control is a .deb's control file: its fields in order, as Debian writes
// them, each value as it is - a long description keeps its lines.
type Control struct {
	Fields []Field
}

// Field is one field of a control file.
type Field struct{ Name, Value string }

// Get is a field's value, "" without it.
func (c *Control) Get(name string) string {
	for _, f := range c.Fields {
		if strings.EqualFold(f.Name, name) {
			return f.Value
		}
	}
	return ""
}

// readControl reads the control file of the .deb at path. A .deb is an ar
// archive: debian-binary, then control.tar - gzip, xz, zstd or none - then
// the files.
func readControl(path string) (*Control, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	magic := make([]byte, 8)
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != "!<arch>\n" {
		return nil, fmt.Errorf("%s isn't a .deb", path)
	}
	for {
		header := make([]byte, 60)
		if _, err := io.ReadFull(r, header); err != nil {
			return nil, fmt.Errorf("%s has no control.tar", path)
		}
		name := strings.TrimRight(strings.TrimSpace(string(header[0:16])), "/")
		size, err := strconv.ParseInt(strings.TrimSpace(string(header[48:58])), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: a damaged archive", path)
		}
		body := io.LimitReader(r, size)
		if strings.HasPrefix(name, "control.tar") {
			return controlFromTar(name, body)
		}
		if _, err := io.Copy(io.Discard, body); err != nil {
			return nil, err
		}
		if size%2 == 1 { // members start on even offsets
			if _, err := r.ReadByte(); err != nil {
				return nil, err
			}
		}
	}
}

func controlFromTar(name string, body io.Reader) (*Control, error) {
	var in io.Reader
	switch {
	case strings.HasSuffix(name, ".gz"):
		z, err := gzip.NewReader(body)
		if err != nil {
			return nil, err
		}
		in = z
	case strings.HasSuffix(name, ".xz"):
		z, err := xz.NewReader(body)
		if err != nil {
			return nil, err
		}
		in = z
	case strings.HasSuffix(name, ".zst"):
		z, err := zstd.NewReader(body)
		if err != nil {
			return nil, err
		}
		defer z.Close()
		in = z
	case name == "control.tar":
		in = body
	default:
		return nil, fmt.Errorf("%s: a compression vero-repo can't read", name)
	}
	t := tar.NewReader(in)
	for {
		h, err := t.Next()
		if err != nil {
			return nil, errors.New("the package has no control file")
		}
		if strings.TrimPrefix(h.Name, "./") == "control" {
			data, err := io.ReadAll(t)
			if err != nil {
				return nil, err
			}
			return parseControl(data)
		}
	}
}

// parseControl reads a control file: "Name: value" lines, and lines that
// start with a space continuing the field before.
func parseControl(data []byte) (*Control, error) {
	var c Control
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if len(c.Fields) == 0 {
				return nil, errors.New("a control file that starts with a continuation")
			}
			c.Fields[len(c.Fields)-1].Value += "\n" + line
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("a control line with no colon: %q", line)
		}
		c.Fields = append(c.Fields, Field{name, strings.TrimSpace(value)})
	}
	for _, need := range []string{"Package", "Version", "Architecture"} {
		if c.Get(need) == "" {
			return nil, fmt.Errorf("the control file has no %s", need)
		}
	}
	return &c, nil
}

// writeControl writes fields as a control file's paragraph.
func writeControl(w *bytes.Buffer, fields []Field) {
	for _, f := range fields {
		fmt.Fprintf(w, "%s: %s\n", f.Name, f.Value)
	}
}
