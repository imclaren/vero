// Package update tells an app whether a newer version of it is on its
// site: the latest.json that vero-repo build writes, or a Sparkle appcast.
// The worker can answer a front end's "check for updates" with it, and
// the front end opens the download. (Sparkle itself, on macOS, checks and
// installs updates by itself; this is for the other systems, and for an
// app's own check.)
package update

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// Latest is latest.json: the newest version, and where each installer is,
// by the keys vero-repo build gives them, such as "linux-amd64",
// "rpm-x86_64", "windows-x64" and "freebsd-aarch64".
type Latest struct {
	Name      string              `json:"name"`
	Version   string              `json:"version"`
	Downloads map[string]Download `json:"downloads"`
}

// Download is one installer: its version, and its URL relative to the
// site, or absolute.
type Download struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// Result is what a check found.
type Result struct {
	// Current is the version this is, and Latest the newest on the site.
	Current, Latest string
	// Newer says Latest is newer than Current.
	Newer bool
	// URL is where to get it: the installer for this system when the
	// site has one, otherwise the site's page.
	URL string
}

// Check reads the site's latest.json at url and says whether it has a
// newer version than current. keys are the downloads to look for, in
// order, as the app was installed: "linux-amd64" for a .deb, say; none
// means every key that fits the system this is running on.
func Check(ctx context.Context, url, current string, keys ...string) (Result, error) {
	var l Latest
	if err := get(ctx, url, func(r io.Reader) error { return json.NewDecoder(r).Decode(&l) }); err != nil {
		return Result{}, err
	}
	return l.Check(current, keys...)
}

// Check is Check for latest.json already read.
func (l Latest) Check(current string, keys ...string) (Result, error) {
	if l.Version == "" {
		return Result{}, errors.New("update: latest.json has no version")
	}
	if len(keys) == 0 {
		keys = Keys(runtime.GOOS, runtime.GOARCH)
	}
	r := Result{Current: current, Latest: l.Version, Newer: Compare(l.Version, current) > 0}
	for _, k := range keys {
		if d, ok := l.Downloads[k]; ok {
			r.URL = d.URL
			if d.Version != "" {
				r.Latest = d.Version
				r.Newer = Compare(d.Version, current) > 0
			}
			break
		}
	}
	return r, nil
}

// Keys are the latest.json keys an app on goos/goarch may have been
// installed from, most usual first.
func Keys(goos, goarch string) []string {
	switch goos {
	case "windows":
		return []string{"windows-" + map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}[goarch]}
	case "darwin":
		return []string{"macos-universal", "macos"}
	case "linux":
		// Each kind of package names the architecture its own way; a
		// kind without a name has no package for it.
		names := map[string]map[string]string{
			"amd64":   {"linux": "amd64", "rpm": "x86_64", "flatpak": "x86_64", "arch": "x86_64", "alpine": "x86_64", "void": "x86_64"},
			"arm64":   {"linux": "arm64", "rpm": "aarch64", "flatpak": "aarch64", "arch": "aarch64", "alpine": "aarch64", "void": "aarch64"},
			"riscv64": {"linux": "riscv64", "rpm": "riscv64", "arch": "riscv64", "alpine": "riscv64"},
			"ppc64le": {"linux": "ppc64el", "rpm": "ppc64le", "alpine": "ppc64le"},
			"arm":     {"linux": "armhf", "arch": "armv7h", "alpine": "armv7", "void": "armv7l"},
		}[goarch]
		var keys []string
		for _, kind := range []string{"linux", "rpm", "flatpak", "arch", "alpine", "void"} {
			if names[kind] != "" {
				keys = append(keys, kind+"-"+names[kind])
			}
		}
		return keys
	default:
		return []string{goos + "-" + goarch, goos + "-" + map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]}
	}
}

// Appcast is a Sparkle appcast: each item's version, and its download.
type Appcast struct {
	Items []AppcastItem `xml:"channel>item"`
}

// AppcastItem is one release in an appcast.
type AppcastItem struct {
	Title     string `xml:"title"`
	Version   string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle version"`
	ShortVer  string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString"`
	Enclosure struct {
		URL      string `xml:"url,attr"`
		Version  string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle version,attr"`
		ShortVer string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString,attr"`
	} `xml:"enclosure"`
}

// CheckAppcast reads a Sparkle appcast at url and says whether its newest
// item is newer than current (a short version, "1.2.3").
func CheckAppcast(ctx context.Context, url, current string) (Result, error) {
	var a Appcast
	if err := get(ctx, url, func(r io.Reader) error { return xml.NewDecoder(r).Decode(&a) }); err != nil {
		return Result{}, err
	}
	return a.Check(current)
}

// Check is CheckAppcast for an appcast already read.
func (a Appcast) Check(current string) (Result, error) {
	r := Result{Current: current}
	for _, it := range a.Items {
		v := it.Enclosure.ShortVer
		if v == "" {
			v = it.ShortVer
		}
		if v == "" {
			v = it.Enclosure.Version
		}
		if v == "" {
			v = it.Version
		}
		if v == "" {
			continue
		}
		if r.Latest == "" || Compare(v, r.Latest) > 0 {
			r.Latest, r.URL = v, it.Enclosure.URL
		}
	}
	if r.Latest == "" {
		return r, errors.New("update: the appcast has no versions")
	}
	r.Newer = Compare(r.Latest, current) > 0
	return r, nil
}

func get(ctx context.Context, url string, read func(io.Reader) error) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("update: %s: %s", url, resp.Status)
	}
	return read(io.LimitReader(resp.Body, 4<<20))
}

// Compare compares two versions as people number them - "1.2.10" is
// newer than "1.2.9", "1.0" equals "1.0.0", and a suffix such as
// "-beta" or "~rc1" comes before the release - and returns -1, 0 or 1.
func Compare(a, b string) int {
	ap, as := split(a)
	bp, bs := split(b)
	for i := 0; i < len(ap) || i < len(bp); i++ {
		var x, y int
		if i < len(ap) {
			x = ap[i]
		}
		if i < len(bp) {
			y = bp[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case as == bs:
		return 0
	case as == "":
		return 1 // a release, after its betas
	case bs == "":
		return -1
	case as < bs:
		return -1
	}
	return 1
}

// split is a version's numbers, and whatever follows them.
func split(v string) ([]int, string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var nums []int
	for {
		n, rest, ok := leadingNumber(v)
		if !ok {
			return nums, v
		}
		nums = append(nums, n)
		if !strings.HasPrefix(rest, ".") {
			return nums, rest
		}
		v = rest[1:]
	}
}

func leadingNumber(s string) (n int, rest string, ok bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		i++
	}
	return n, s[i:], i > 0
}
