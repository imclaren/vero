// Package site serves the folder vero-repo build makes - the install page,
// the repositories each system updates from, latest.json and the installers
// - from an app's own Go server, for an app that has one. Nothing here is
// needed to publish: the folder is plain files, and any web host serves it.
// This is for a server that wants it alongside its own pages, or wants
// downloads only for people who have signed in.
//
//	http.Handle("/", site.Handler(site.Options{Dir: "dist/site"}))
//
// serves the site, plus /download?for=windows&arch=x64, which sends the
// browser to that installer: a link a download page can use without
// knowing version numbers.
package site

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Options say what to serve.
type Options struct {
	// Dir is the folder vero-repo build wrote.
	Dir string
	// Prefix is where the site is mounted, "/downloads" say, so that the
	// redirects know their own address. "" is the root.
	Prefix string
}

// Handler serves the site. Every file in Dir is served at its path, and
// /download?for=SYSTEM[&arch=ARCH] redirects to that system's newest
// installer from latest.json: for=windows&arch=x64, for=linux (the .deb),
// for=rpm, for=macos (the disk image), for=pkg (the Mac's installer
// package). Common names work too: mac and osx, win, deb, debian and
// ubuntu, fedora. The architecture is guessed from the browser when it
// is left out (arm64 for Apple silicon and Windows on ARM, where the
// browser says so).
func Handler(o Options) http.Handler {
	s := &server{o: o, files: http.FileServer(http.Dir(o.Dir))}
	mux := http.NewServeMux()
	mux.HandleFunc("/download", s.download)
	mux.Handle("/", s)
	if o.Prefix != "" {
		return http.StripPrefix(strings.TrimSuffix(o.Prefix, "/"), mux)
	}
	return mux
}

// Private wraps a handler so that only requests check allows reach it;
// the rest get 401. For an app whose downloads are for people who have
// signed in.
func Private(h http.Handler, check func(*http.Request) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !check(r) {
			http.Error(w, "sign in to download", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}

type server struct {
	o     Options
	files http.Handler

	mu     sync.Mutex
	latest latest
	read   time.Time
}

type latest struct {
	Name      string              `json:"name"`
	Version   string              `json:"version"`
	Downloads map[string]download `json:"downloads"`
}

type download struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The repositories' index files are fetched often and change each
	// release: no caching beyond a few minutes, so an update is seen soon.
	w.Header().Set("Cache-Control", "max-age=300")
	s.files.ServeHTTP(w, r)
}

// current is latest.json, re-read when it is more than a minute old, so
// that an upload of a new release is served without a restart.
func (s *server) current() (latest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.read) < time.Minute && s.latest.Version != "" {
		return s.latest, nil
	}
	data, err := os.ReadFile(filepath.Join(s.o.Dir, "latest.json"))
	if err != nil {
		return latest{}, err
	}
	var l latest
	if err := json.Unmarshal(data, &l); err != nil {
		return latest{}, err
	}
	s.latest, s.read = l, time.Now()
	return l, nil
}

func (s *server) download(w http.ResponseWriter, r *http.Request) {
	l, err := s.current()
	if err != nil {
		http.Error(w, "no releases yet", http.StatusNotFound)
		return
	}
	system := systemName(r.URL.Query().Get("for"))
	arch := r.URL.Query().Get("arch")
	if arch == "" {
		arch = guessArch(r.UserAgent())
	}
	for _, key := range keys(system, arch) {
		if d, ok := l.Downloads[key]; ok {
			target := d.URL
			if !strings.Contains(target, "://") {
				target = path.Join(s.o.Prefix, "/", target)
			}
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
	}
	http.Error(w, "no download for "+system+" "+arch, http.StatusNotFound)
}

// systemName is the name vero-repo uses for a system, from the names
// people link with: mac and osx for macos, win for windows, deb, debian
// and ubuntu for linux's .deb, fedora for rpm, pkg for the Mac's installer
// package.
func systemName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if n, ok := map[string]string{
		"mac": "macos", "osx": "macos", "darwin": "macos", "macosx": "macos",
		"pkg": "macos-pkg", "mac-pkg": "macos-pkg",
		"win": "windows", "win32": "windows", "win64": "windows",
		"deb": "linux", "debian": "linux", "ubuntu": "linux",
		"fedora": "rpm", "opensuse": "rpm", "suse": "rpm",
	}[s]; ok {
		return n
	}
	return s
}

// keys are the latest.json keys to try for a system and architecture, the
// best fit first, and then the other architecture, so that a wrong guess
// still gets an installer.
func keys(system, arch string) []string {
	arches := map[string][]string{
		"windows": {"x64", "arm64"}, "linux": {"amd64", "arm64"}, "rpm": {"x86_64", "aarch64"},
		"flatpak": {"x86_64", "aarch64"}, "freebsd": {"amd64", "aarch64"}, "macos": {"universal", ""},
		"macos-pkg": {""},
	}[system]
	if arches == nil {
		arches = []string{"amd64", "x86_64", "aarch64", "arm64"}
	}
	// Spellings of the two architectures.
	want := map[string]bool{}
	switch arch {
	case "arm64", "aarch64":
		want["arm64"], want["aarch64"] = true, true
	case "x64", "amd64", "x86_64":
		want["x64"], want["amd64"], want["x86_64"] = true, true, true
	}
	var first, rest []string
	for _, a := range arches {
		k := system
		if a != "" {
			k += "-" + a
		}
		if a == "" || want[a] {
			first = append(first, k)
		} else {
			rest = append(rest, k)
		}
	}
	return append(first, rest...)
}

// guessArch reads the architecture from a browser's User-Agent where it
// says: "ARM64" for Windows on ARM, "ARM" or "aarch64" for Linux. Apple
// silicon Macs say Intel, and get x64 - a Mac downloads a universal
// installer, so it does not matter there.
func guessArch(ua string) string {
	switch ua = strings.ToLower(ua); {
	case strings.Contains(ua, "arm64"), strings.Contains(ua, "aarch64"), strings.Contains(ua, "; arm"):
		return "arm64"
	}
	return "x64"
}
