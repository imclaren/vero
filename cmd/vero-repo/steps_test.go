package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStepsFromTheExample(t *testing.T) {
	out := t.TempDir()
	if err := stepsCommand([]string{"--app", "../../example/vero-app.toml", "--out", out}); err != nil {
		t.Fatal(err)
	}
	var got struct{ Steps []map[string]any }
	if err := json.Unmarshal(mustRead(t, filepath.Join(out, "steps.json")), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) < 3 || got.Steps[3]["call"] != "restartJob" {
		t.Errorf("steps.json has %v", got.Steps)
	}
}

func TestStepsAreChecked(t *testing.T) {
	for step, want := range map[string]string{
		`call = "x"` + "\n" + `pause = 1`:        "should be one of",
		`wait = { path = "a" }`:                  "one of is, not",
		`wait = { path = "a", is = 1, not = 2 }`: "one of is, not",
		`call = "x"` + "\n" + `timeout = 3`:      "timeout doesn't go with call",
		`copy = { from = "a" }`:                  "copy is",
		`frobnicate = 1`:                         "should be one of",
	} {
		file := filepath.Join(t.TempDir(), "vero-app.toml")
		toml := strings.Join([]string{
			`name = "x"`, `id = "org.example.x"`, `summary = "s"`, `publisher = "P <p@example.com>"`,
			"[worker]", `package = "w"`, `name = "w"`, "[[test.step]]", step}, "\n")
		if err := os.WriteFile(file, []byte(toml), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadApp(file); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", step, err, want)
		}
	}
}

// TestRecordingsOnThePage: the systems whose recordings passed, on the
// install page, in the page's order, and none of those that failed.
func TestRecordingsOnThePage(t *testing.T) {
	results := t.TempDir()
	for name, status := range map[string]string{
		"macos": "PASS\n40\n\n", "fedora-latest": "PASS\n30\n\n", "ubuntu-24.04": "PASS\n30\n\n",
		"ubuntu-22.04": "PASS\n30\n\n", "vm-netbsd": "FAIL\n10\nthe app exited (1)\n", "something-new": "PASS\n5\n\n",
	} {
		os.MkdirAll(filepath.Join(results, name), 0o755)
		os.WriteFile(filepath.Join(results, name, "status"), []byte(status), 0o644)
		os.WriteFile(filepath.Join(results, name, "app.gif"), []byte("GIF89a "+name), 0o644)
	}
	site := t.TempDir()
	os.MkdirAll(filepath.Join(site, "demo"), 0o755)
	os.WriteFile(filepath.Join(site, "demo", "old.gif"), []byte("last release's"), 0o644)
	recs, err := copyRecordings(results, site)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range recs {
		got = append(got, r.System)
	}
	if strings.Join(got, ",") != "Mac,Ubuntu,Fedora,something-new" {
		t.Errorf("recordings: %v", got)
	}
	if _, err := os.Stat(filepath.Join(site, "demo", "old.gif")); err == nil {
		t.Error("the last release's recording is still there")
	}
	if _, err := os.Stat(filepath.Join(site, "demo", "vm-netbsd.gif")); err == nil {
		t.Error("a failed recording was copied")
	}

	a, _ := linuxApp(t)
	var html strings.Builder
	if err := indexPage.Execute(&html, page{App: a, Latest: Latest{Version: "1.0.0"}, Recordings: recs}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<h2>See it at work</h2>", `<img src="demo/macos.gif" alt="vero example at work on Mac"`, "<figcaption>Fedora</figcaption>"} {
		if !strings.Contains(html.String(), want) {
			t.Errorf("the page has no %q", want)
		}
	}
	if recs, _ := copyRecordings(filepath.Join(results, "none"), site); recs != nil {
		t.Error("recordings from a folder that isn't there")
	}
}
