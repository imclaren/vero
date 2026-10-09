package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.2.10", "1.2.9", 1}, {"1.0", "1.0.0", 0}, {"v2.0", "1.9.9", 1},
		{"1.0-beta", "1.0", -1}, {"1.0~rc1", "1.0", -1}, {"1.0-beta1", "1.0-beta2", -1}, {"0.3.21", "0.3.21", 0},
	} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest.json":
			w.Write([]byte(`{"name":"myapp","version":"1.2.0","downloads":{"windows-x64":{"version":"1.2.0","url":"windows/myapp-1.2.0-x64-setup.exe"},"linux-amd64":{"version":"1.1.0","url":"apt/pool/main/m/myapp/myapp_1.1.0_amd64.deb"}}}`))
		case "/appcast.xml":
			w.Write([]byte(`<?xml version="1.0"?><rss xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle"><channel>
<item><title>1.0.0</title><enclosure url="https://example.com/myapp-1.0.0.dmg" sparkle:shortVersionString="1.0.0" sparkle:version="10"/></item>
<item><title>1.2.0</title><enclosure url="https://example.com/myapp-1.2.0.dmg" sparkle:shortVersionString="1.2.0" sparkle:version="12"/></item>
</channel></rss>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	r, err := Check(ctx, srv.URL+"/latest.json", "1.1.0", "windows-x64")
	if err != nil || !r.Newer || r.Latest != "1.2.0" || r.URL != "windows/myapp-1.2.0-x64-setup.exe" {
		t.Errorf("windows: %+v, %v", r, err)
	}
	// The .deb is still 1.1.0, so there is nothing newer for a .deb install.
	r, err = Check(ctx, srv.URL+"/latest.json", "1.1.0", "linux-amd64")
	if err != nil || r.Newer || r.Latest != "1.1.0" {
		t.Errorf("linux: %+v, %v", r, err)
	}
	r, err = CheckAppcast(ctx, srv.URL+"/appcast.xml", "1.0.0")
	if err != nil || !r.Newer || r.Latest != "1.2.0" || r.URL != "https://example.com/myapp-1.2.0.dmg" {
		t.Errorf("appcast: %+v, %v", r, err)
	}
	if _, err := Check(ctx, srv.URL+"/missing", "1.0.0"); err == nil {
		t.Error("a missing file was no error")
	}
	if k := Keys("linux", "amd64"); k[0] != "linux-amd64" || k[1] != "rpm-x86_64" {
		t.Errorf("keys: %v", k)
	}
}

// TestKeys: an app finds its own download in latest.json, as vero-repo
// build names them.
func TestKeys(t *testing.T) {
	if k := Keys("darwin", "arm64"); k[0] != "macos-universal" {
		t.Errorf("darwin: %v", k)
	}
	if k := Keys("windows", "arm64"); k[0] != "windows-arm64" {
		t.Errorf("windows: %v", k)
	}
	if k := Keys("linux", "amd64"); k[0] != "linux-amd64" {
		t.Errorf("linux: %v", k)
	}
	if k := Keys("windows", "386"); k[0] != "windows-x86" {
		t.Errorf("32-bit windows: %v", k)
	}
	if k := strings.Join(Keys("linux", "arm"), " "); k != "linux-armhf arch-armv7h alpine-armv7 void-armv7l" {
		t.Errorf("32-bit ARM: %v", k)
	}
	if k := strings.Join(Keys("linux", "ppc64le"), " "); k != "linux-ppc64el rpm-ppc64le alpine-ppc64le" {
		t.Errorf("POWER: %v", k)
	}
}
