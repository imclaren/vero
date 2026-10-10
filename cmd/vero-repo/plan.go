package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Plan is what vero ship will do with an app on this Mac: each installer
// it can build, each system it can test, and why it cannot do the rest.
type Plan struct {
	Name  string     `json:"name"`
	Build []PlanStep `json:"build"`
	Test  []PlanStep `json:"test"`
}

// PlanStep is one installer or one system's test. Why is what is missing
// when it cannot be done here; Note is said when it can, such as that a
// VM is made the first time.
type PlanStep struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	Why  string `json:"why,omitempty"`
	Note string `json:"note,omitempty"`
}

// testSystems are the systems test-all.sh tests, with the package each
// installs and what it runs in.
var testSystems = []struct{ name, target, vm string }{
	{"debian", "deb", ""}, {"ubuntu", "deb", ""}, {"fedora", "rpm", ""}, {"opensuse", "rpm", ""},
	{"arch", "pacman", ""}, {"alpine", "alpine", ""}, {"chimera", "chimera", ""}, {"void", "void", ""},
	{"flatpak", "flatpak", ""},
	{"freebsd", "freebsd", "freebsd"}, {"netbsd", "netbsd", "netbsd"}, {"openbsd", "openbsd", "openbsd"},
	{"dragonfly", "dragonfly", "dragonfly"}, {"illumos", "illumos", "illumos"}, {"windows", "windows", "windows"},
}

// makeVM is how long each VM takes to make, the first time it is used.
var makeVM = map[string]string{
	"freebsd": "a few minutes", "netbsd": "a few minutes", "openbsd": "about 15 minutes",
	"dragonfly": "the best part of an hour", "illumos": "about an hour and a half", "windows": "about 20 minutes",
}

// planFor works out an app's plan on this machine.
func planFor(a *App) Plan {
	p := Plan{Name: a.Name}
	built := map[string]bool{}
	for _, t := range possibleTargets(a) {
		why := missingFor(t.name)
		built[t.name] = why == ""
		p.Build = append(p.Build, PlanStep{Name: t.name, OK: why == "", Why: why})
	}
	docker := exec.Command("docker", "info").Run() == nil
	_, qemuErr := exec.LookPath("qemu-system-aarch64")
	home, _ := os.UserHomeDir()
	for _, s := range testSystems {
		ok, present := built[s.target]
		if !present {
			continue
		}
		step := PlanStep{Name: s.name, OK: ok}
		switch {
		case !ok:
			step.Why = "its installer cannot be built here"
		case s.vm == "" && !docker:
			step.OK, step.Why = false, "needs Docker running: colima start"
		case s.vm != "" && qemuErr != nil:
			step.OK, step.Why = false, "needs qemu: brew install qemu"
		case s.vm == "windows":
			vm := filepath.Join(home, "vm", "vero-windows")
			if !fileExists(filepath.Join(vm, "disk.qcow2")) {
				if isos, _ := filepath.Glob(filepath.Join(vm, "*.iso")); len(isos) == 0 {
					step.OK, step.Why = false, "needs Microsoft's Windows 11 ARM64 ISO in ~/vm/vero-windows/"
				} else {
					step.Note = "its VM is made first, " + makeVM["windows"]
				}
			}
		case s.vm != "" && !fileExists(filepath.Join(home, "vm", "vero-"+s.vm)):
			step.Note = "its VM is made first, " + makeVM[s.vm]
		}
		p.Test = append(p.Test, step)
	}
	if built["macos"] {
		step := PlanStep{Name: "macos", OK: true}
		if a.Test == nil || a.Test.Env["VERO_PROFILE"] == "" {
			step.OK, step.Why = false, "set VERO_PROFILE in [test] env, so that a test cannot touch the copy you use"
		}
		p.Test = append([]PlanStep{step}, p.Test...)
	}
	return p
}

// planCommand prints an app's plan, as text or, with --json, for vero
// ship to read.
func planCommand(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	app := fs.String("app", "vero-app.toml", "the app's vero-app.toml")
	asJSON := fs.Bool("json", false, "print it as JSON")
	fs.Parse(args)
	a, err := LoadApp(*app)
	if err != nil {
		return err
	}
	p := planFor(a)
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(p)
	}
	fmt.Print(p.Text())
	return nil
}

// Text is the plan as vero ship says it, before it starts.
func (p Plan) Text() string {
	var b strings.Builder
	list := func(heading string, steps []PlanStep, ok bool) {
		var names []string
		for _, s := range steps {
			if s.OK == ok {
				names = append(names, s.Name)
			}
		}
		if len(names) > 0 {
			fmt.Fprintf(&b, "  %-18s %s\n", heading, strings.Join(names, ", "))
		}
	}
	fmt.Fprintf(&b, "%s on this Mac:\n", p.Name)
	list("builds", p.Build, true)
	list("tests and records", p.Test, true)
	for _, s := range p.Build {
		if !s.OK {
			fmt.Fprintf(&b, "  %-18s %s: %s\n", "skips building", s.Name, s.Why)
		}
	}
	for _, s := range p.Test {
		if !s.OK {
			fmt.Fprintf(&b, "  %-18s %s: %s\n", "skips testing", s.Name, s.Why)
		}
	}
	for _, s := range p.Test {
		if s.OK && s.Note != "" {
			fmt.Fprintf(&b, "  %-18s %s: %s\n", "first", s.Name, s.Note)
		}
	}
	return b.String()
}
