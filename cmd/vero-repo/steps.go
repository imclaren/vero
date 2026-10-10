package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Test is vero-app.toml's [test]: the steps vero's tests play against the
// installed app, through vero's binding in the app, while its window is
// open and recorded.
//
//	[test]
//	files = ["testdata/sample.mp3"]
//
//	[[test.step]]
//	call = "setLibrary"
//	with = { path = "{tmp}/library" }
//
//	[[test.step]]
//	copy = { from = "sample.mp3", to = "{tmp}/library/imports/" }
//
//	[[test.step]]
//	wait = { path = "stats.books", at_least = 1 }
//	timeout = 300
type Test struct {
	// Files go with the steps, into the folder {files} names.
	Files []string `toml:"files"`
	// Env is set for the app as the test starts it: a setting that keeps a
	// test copy's settings apart from those of a copy in use, say.
	Env   map[string]string `toml:"env"`
	Steps []map[string]any  `toml:"step"`
}

var stepKinds = []string{"call", "send", "wait", "pause", "copy"}

var validEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (t *Test) check(a *App) error {
	for i, step := range t.Steps {
		n := i + 1
		var kinds []string
		for _, k := range stepKinds {
			if _, ok := step[k]; ok {
				kinds = append(kinds, k)
			}
		}
		if len(kinds) != 1 {
			return fmt.Errorf("[[test.step]] %d should be one of %s", n, strings.Join(stepKinds, ", "))
		}
		for k := range step {
			switch {
			case k == kinds[0]:
			case k == "with" && kinds[0] == "call":
			case k == "timeout" && kinds[0] == "wait":
			default:
				return fmt.Errorf("[[test.step]] %d: %s doesn't go with %s", n, k, kinds[0])
			}
		}
		switch kinds[0] {
		case "call":
			if s, ok := step["call"].(string); !ok || s == "" {
				return fmt.Errorf("[[test.step]] %d: call is a request's name", n)
			}
		case "wait":
			w, ok := step["wait"].(map[string]any)
			if !ok {
				return fmt.Errorf("[[test.step]] %d: wait is { path = ..., is = ... }", n)
			}
			ops := 0
			for _, op := range []string{"is", "not", "at_least", "contains"} {
				if _, ok := w[op]; ok {
					ops++
				}
			}
			if _, ok := w["path"].(string); !ok || ops != 1 || len(w) != 2 {
				return fmt.Errorf("[[test.step]] %d: wait needs a path and one of is, not, at_least or contains", n)
			}
		case "copy":
			c, ok := step["copy"].(map[string]any)
			from, _ := c["from"].(string)
			to, _ := c["to"].(string)
			if !ok || from == "" || to == "" || len(c) != 2 {
				return fmt.Errorf("[[test.step]] %d: copy is { from = ..., to = ... }", n)
			}
		}
	}
	for k := range t.Env {
		if !validEnvName.MatchString(k) {
			return fmt.Errorf("[test] env: %q isn't a variable's name", k)
		}
	}
	for _, f := range t.Files {
		if _, err := os.Stat(a.Path(f)); err != nil {
			return fmt.Errorf("[test] files: %v", err)
		}
	}
	return nil
}

// stepsCommand writes what a test copies onto the system it tests: the
// steps as the binding reads them, steps.json, and the test's files in
// files/.
func stepsCommand(args []string) error {
	fset := flag.NewFlagSet("steps", flag.ExitOnError)
	app := fset.String("app", "vero-app.toml", "the app's vero-app.toml")
	out := fset.String("out", "", "the folder to write (required)")
	fset.Parse(args)
	if *out == "" {
		return fmt.Errorf("steps needs --out")
	}
	a, err := LoadApp(*app)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(*out, "files"), 0o755); err != nil {
		return err
	}
	t := a.Test
	if t == nil {
		t = &Test{}
	}
	steps := t.Steps
	if steps == nil {
		steps = []map[string]any{}
	}
	data, err := json.MarshalIndent(map[string]any{"steps": steps}, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, "steps.json"), data, 0o644); err != nil {
		return err
	}
	var env strings.Builder
	for k, v := range t.Env {
		fmt.Fprintf(&env, "export %s='%s'\n", k, strings.ReplaceAll(v, "'", `'\''`))
	}
	if err := os.WriteFile(filepath.Join(*out, "env.sh"), []byte(env.String()), 0o644); err != nil {
		return err
	}
	for _, f := range t.Files {
		data, err := os.ReadFile(a.Path(f))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*out, "files", filepath.Base(f)), data, 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("wrote %d steps and %d files to %s\n", len(steps), len(t.Files), *out)
	return nil
}

// showCommand prints what a script needs to know about an app, as shell
// assignments: test-repo.sh reads them to test an app other than vero's
// example.
func showCommand(args []string) error {
	fset := flag.NewFlagSet("show", flag.ExitOnError)
	app := fset.String("app", "vero-app.toml", "the app's vero-app.toml")
	fset.Parse(args)
	a, err := LoadApp(*app)
	if err != nil {
		return err
	}
	exe, msix := "", ""
	if a.WPF != nil {
		exe, msix = a.WPF.Exe, a.WPF.MSIX.IdentityName
	}
	file, err := filepath.Abs(*app)
	if err != nil {
		return err
	}
	for _, kv := range [][2]string{
		{"APP_NAME", a.Name}, {"APP_DISPLAY", a.DisplayName}, {"APP_ID", a.ID},
		{"APP_WORKER", a.Worker.Name}, {"APP_EXE", exe}, {"APP_MSIX", msix}, {"APP_TOML", file}, {"APP_DIR", a.dir},
	} {
		fmt.Printf("%s='%s'\n", kv[0], strings.ReplaceAll(kv[1], "'", `'\''`))
	}
	return nil
}
