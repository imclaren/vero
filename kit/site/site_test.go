package site

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSite(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "windows"), 0o755)
	os.WriteFile(filepath.Join(dir, "windows", "myapp-1.2.0-x64-setup.exe"), []byte("exe"), 0o644)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>install</p>"), 0o644)
	os.WriteFile(filepath.Join(dir, "latest.json"), []byte(`{"name":"myapp","version":"1.2.0","downloads":{
		"windows-x64":{"version":"1.2.0","url":"windows/myapp-1.2.0-x64-setup.exe"},
		"linux-amd64":{"version":"1.2.0","url":"apt/pool/main/m/myapp/myapp_1.2.0_amd64.deb"}}}`), 0o644)

	srv := httptest.NewServer(Handler(Options{Dir: dir, Prefix: "/dl"}))
	defer srv.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(p string) *http.Response {
		r, err := client.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := get("/dl/"); r.StatusCode != 200 || r.Header.Get("Cache-Control") != "max-age=300" {
		t.Errorf("index: %d %v", r.StatusCode, r.Header)
	}
	if r := get("/dl/download?for=windows&arch=x64"); r.StatusCode != 302 || r.Header.Get("Location") != "/dl/windows/myapp-1.2.0-x64-setup.exe" {
		t.Errorf("windows: %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	// No ARM installer: the other architecture's, rather than nothing.
	if r := get("/dl/download?for=windows&arch=arm64"); r.StatusCode != 302 || r.Header.Get("Location") != "/dl/windows/myapp-1.2.0-x64-setup.exe" {
		t.Errorf("windows arm: %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	if r := get("/dl/download?for=linux"); r.StatusCode != 302 || r.Header.Get("Location") != "/dl/apt/pool/main/m/myapp/myapp_1.2.0_amd64.deb" {
		t.Errorf("linux: %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	if r := get("/dl/download?for=plan9"); r.StatusCode != 404 {
		t.Errorf("plan9: %d", r.StatusCode)
	}

	private := httptest.NewServer(Private(Handler(Options{Dir: dir}), func(r *http.Request) bool { return r.Header.Get("X-Ok") == "1" }))
	defer private.Close()
	if r, _ := client.Get(private.URL + "/latest.json"); r.StatusCode != 401 {
		t.Errorf("private without sign-in: %d", r.StatusCode)
	}
	req, _ := http.NewRequest("GET", private.URL+"/latest.json", nil)
	req.Header.Set("X-Ok", "1")
	if r, _ := client.Do(req); r.StatusCode != 200 {
		t.Errorf("private with sign-in: %d", r.StatusCode)
	}
	if guessArch("Mozilla/5.0 (Windows NT 10.0; ARM64) Chrome") != "arm64" || guessArch("Mozilla/5.0 (Windows NT 10.0; Win64; x64)") != "x64" {
		t.Error("guessArch")
	}
}
