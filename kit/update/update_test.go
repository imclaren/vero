package update

import (
	"context"
	"net/http"
	"net/http/httptest"
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
