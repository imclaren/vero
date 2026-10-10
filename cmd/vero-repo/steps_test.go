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
