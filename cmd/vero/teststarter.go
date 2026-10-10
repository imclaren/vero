package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// testStarter is the [test] section vero add writes into vero-app.toml
// when there is none: one step that always passes, so that every system's
// test records the window, and commented examples of the rest, made from
// the worker's own requests and state.
func testStarter(app *App) string {
	var b strings.Builder
	b.WriteString(`
# What vero's tests do with the app once it's installed and its window is
# open, recorded as a GIF for each system: vero's scripts/test-repo.sh
# --app vero-app.toml, and scripts/record-mac.sh on the Mac. Put files the
# steps need in testdata/ and list them here; {files} is where they are on
# the system tested, and {tmp} a new empty folder. vero-repo release puts
# each system's GIF on the install page.
[test]
files = []
# A copy of its own, so that a test on your own computer cannot see or
# change the settings and sign-ins of the copy you use: kit/keychain and
# kit/profile follow VERO_PROFILE, and vero's Swift package does too.
env = { VERO_PROFILE = "test" }

[[test.step]]
pause = 3
`)
	example := func(lines ...string) {
		b.WriteString("\n")
		for _, l := range lines {
			b.WriteString("# " + l + "\n")
		}
	}
	for _, r := range app.Requests {
		if len(r.Fields) == 0 {
			example("[[test.step]]", fmt.Sprintf("call = %q", r.Name))
			break
		}
		var with []string
		for _, f := range r.Fields {
			with = append(with, f.JSON+" = "+sampleValue(f))
		}
		example("[[test.step]]", fmt.Sprintf("call = %q", r.Name), "with = { "+strings.Join(with, ", ")+" }")
		break
	}
	if app.State != nil {
		for _, f := range app.State.Fields {
			switch f.Kind {
			case "list":
				example("[[test.step]]", fmt.Sprintf("wait = { path = %q, at_least = 1 }", f.JSON+".#"), "timeout = 60")
			case "int", "float":
				example("[[test.step]]", fmt.Sprintf("wait = { path = %q, at_least = 1 }", f.JSON), "timeout = 60")
			case "bool":
				example("[[test.step]]", fmt.Sprintf("wait = { path = %q, is = true }", f.JSON), "timeout = 60")
			case "string":
				example("[[test.step]]", fmt.Sprintf("wait = { path = %q, not = \"\" }", f.JSON), "timeout = 60")
			default:
				continue
			}
			break
		}
	}
	example("[[test.step]]", `copy = { from = "sample.txt", to = "{tmp}/" }`)
	return b.String()
}

// sampleValue is something of f's kind, for an example.
func sampleValue(f Field) string {
	switch f.Kind {
	case "int", "float":
		return "1"
	case "bool":
		return "true"
	case "string":
		return `"{tmp}"`
	}
	return "{}"
}

// addTest writes the starter [test] and a testdata/ folder for its files,
// unless the app has them.
func addTest(app *App) error {
	if app.toml.has("test") {
		return nil
	}
	if err := app.toml.write(app, testStarter(app)); err != nil {
		return err
	}
	dir := filepath.Join(app.Dir, "testdata")
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "README.md"), []byte(`# testdata

Files vero's tests give the app: list them under [test] files in
vero-app.toml, and a step's copy finds them by name. A demo needs
something to show, so put in what your app works on: a few sample
documents, songs or photos, small and yours to share, since the
recordings of it go on your install page.
`), 0o644)
}

// addIcon gives the app vero's example icon when the one vero-app.toml
// names is not there, so that it can be packaged straight away; the
// checklist in PORTING.md says to replace it.
func addIcon(app *App) error {
	name := app.toml.get("", "icon")
	if name == "" {
		name = "icon.png"
	}
	path := filepath.Join(app.Dir, filepath.FromSlash(name))
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := copyBinding(app, "example/icon.png", filepath.Dir(path)); err != nil {
		return err
	}
	if filepath.Base(path) != "icon.png" {
		return os.Rename(filepath.Join(filepath.Dir(path), "icon.png"), path)
	}
	return nil
}
