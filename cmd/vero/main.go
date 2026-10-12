// Command vero adds front ends to a vero app, so that an app written for
// one system runs on the others.
//
//	vero add                       the desktop: macOS, GTK and WPF
//	vero add mobile                Android and iOS
//	vero add all --except plan9    everything vero has a front end for
//	vero add windows android       any mix of groups and systems
//
// Run it in your app's folder, the one with its go.mod and its worker. It
// reads the worker - the requests it handles and the state it pushes - and
// writes a starter front end for each system you ask for, wired to them:
// a window that shows the state and offers each request. Front ends you
// already have are left alone. It adds each to vero-app.toml, so that
// vero's packaging builds them, and writes PORTING.md: what in your worker
// needs attention on the new systems.
//
//	vero gen                       each front end's requests, from the worker
//
// vero gen reads the worker with Go's type checker and writes, beside each
// front end's copy of vero's bindings, a type for every request, reply and
// the state, and a call for each request: WorkerAPI.swift, WorkerApi.cs and
// worker_api.py. A request changed in Go then fails to build in Swift and
// C#, rather than failing when the app runs. [gen] in vero-app.toml can
// name other paths, as swift, csharp and python.
//
//	vero upgrade                   an app made with an older vero, brought up to this one
//
// vero upgrade moves go.mod and the kit to this vero, the Mac app's pin on
// vero's Swift package to Up to Next Major, and the Windows app to .NET 10.
// It copies vero's files into each front end afresh, unless one has been
// changed since vero wrote it, writes vero-app.toml's dependencies in the
// form that works them out, and runs vero gen. It then says what it
// changed, and what to change in the app's own code, such as a type that
// vero now defines.
package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
)

// The front ends, in the order they are offered.
var platforms = []string{"macos", "gtk", "wpf", "android", "ios", "web", "wasi", "plan9"}

var groups = map[string][]string{
	"desktop": {"macos", "gtk", "wpf"},
	"mobile":  {"android", "ios"},
	"all":     platforms,
}

// Other names people use for a front end.
var aliases = map[string]string{
	"mac": "macos", "darwin": "macos", "linux": "gtk", "bsd": "gtk", "freebsd": "gtk", "windows": "wpf",
	"openbsd": "gtk", "netbsd": "gtk", "dragonfly": "gtk", "illumos": "gtk",
	"win": "wpf", "browser": "web", "wasm": "web", "plan-9": "plan9",
}

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "ship", "setup", "gen", "upgrade", "install", "local-version":
			do := map[string]func([]string) error{"ship": ship, "upgrade": upgrade, "setup": setup, "gen": gen, "install": install, "local-version": localVersionCommand}[os.Args[1]]
			if err := do(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "vero %s: %v\n", os.Args[1], err)
				os.Exit(1)
			}
			return
		case "release", "credentials", "publish", "key", "package", "build", "check", "steps", "show", "plan":
			// The packaging is vero-repo's; this hands over to it, so that
			// one tool is all anyone installs.
			os.Exit(veroRepo(os.Args[1:]))
		}
	}
	if len(os.Args) >= 3 && os.Args[1] == "new" && !strings.HasPrefix(os.Args[2], "-") {
		// vero new NAME [front ends] [--module PATH] [--vero DIR] [add's flags]:
		// the app, then vero add in its folder.
		name, module, local := os.Args[2], "", ""
		var rest []string
		for i := 3; i < len(os.Args); i++ {
			switch a := os.Args[i]; {
			case (a == "--module" || a == "-module") && i+1 < len(os.Args):
				module = os.Args[i+1]
				i++
			case (a == "--vero" || a == "-vero") && i+1 < len(os.Args):
				local = os.Args[i+1]
				i++
			default:
				rest = append(rest, a)
			}
		}
		if err := newApp(name, module, local); err != nil {
			fmt.Fprintln(os.Stderr, "vero new:", err)
			os.Exit(1)
		}
		fmt.Printf("made %s/, a Go module with vero's example worker in cmd/worker\n", name)
		os.Args = append([]string{os.Args[0], "add"}, append(rest, "--dir", name)...)
	}
	if len(os.Args) < 2 || os.Args[1] != "add" {
		fmt.Fprintln(os.Stderr, "usage: vero setup                 installs what vero needs on this Mac, then says what is missing\n"+
			"       vero ship [FILE.go] [--name NAME] [--plan] [--no-release] [--no-vms] [--no-mac] [--only LIST] [--skip LIST]\n"+
			"                 [--version 1.2.3] [--notes TEXT] [--upload user@host:/path] [--url URL]\n"+
			"                 makes an app from a worker file, or takes the one here, tests and records it everywhere, and releases it\n"+
			"       vero new NAME [front ends, as for add] [--module PATH] [add's flags]\n"+
			"       vero add [desktop|mobile|all|macos|gtk|wpf|android|ios|web|wasi|plan9 ...] [--except LIST] [--dir DIR]\n"+
			"                [--menubar | --tray] [--summary TEXT] [--description TEXT] [--publisher \"Name <email>\"] [--keep-worker] [--force]\n"+
			"       vero install [--published 1.2.3] [--dir DIR]\n"+
			"                builds the Mac app from here and installs it, numbered just above the release that is out\n"+
			"       vero local-version [--published 1.2.3] [--dir DIR]\n"+
			"                prints that number, for a prebuild script that numbers Xcode's own builds the same way\n"+
			"       vero upgrade [--version v0.18.0] [--force] [--dir DIR]\n"+
			"                brings an app made with vero 0.13 or later up to this vero, and says what to change in its own code\n"+
			"       vero gen [--check] [--out DIR] [--dir DIR]\n"+
			"                writes each front end's requests from the worker's types, in Swift, C# and Python\n"+
			"       vero release [--version 1.2.3] [--targets ...] [--notes \"...\"] [--upload user@host:/path]\n"+
			"       vero credentials\n"+
			"       vero publish --homebrew | --winget | --aur | --all\n"+
			"       vero key | package | build | check | steps | show | plan ...   vero-repo's own commands, which vero runs for you")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	except := fs.String("except", "", "front ends to leave out, comma-separated")
	dir := fs.String("dir", ".", "the app's folder")
	force := fs.Bool("force", false, "write a starter even where a front end's folder exists")
	summary := fs.String("summary", "", "one line about the app, for vero-app.toml")
	description := fs.String("description", "", "a sentence or two about the app, for vero-app.toml")
	publisher := fs.String("publisher", "", "who publishes it, as \"Name <email>\", for vero-app.toml")
	menubar := fs.Bool("menubar", false, "an app that lives in the menu bar on the Mac and in the tray elsewhere, with a small window from its icon, rather than in a window")
	tray := fs.Bool("tray", false, "a window app that also has an icon in the menu bar and the tray, and keeps running when its window is closed")
	keepWorker := fs.Bool("keep-worker", false, "for iOS and the browser, leave the worker's code where it is and give them a placeholder, rather than moving it into a package they can run")
	// Flags may come after the names.
	var names, rest []string
	for _, a := range os.Args[2:] {
		if strings.HasPrefix(a, "-") || len(rest) > 0 {
			rest = append(rest, a)
		} else {
			names = append(names, a)
		}
	}
	fs.Parse(rest)

	want, err := choose(names, *except)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	app, err := analyse(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var guessed []string
	app.Description = Description{Summary: *summary, Text: *description, Publisher: *publisher}
	if app.toml.text == "" {
		app.Description, guessed = describe(app.Description, app.Display, gitConfig)
	}
	app.KeepWorker = *keepWorker
	if *menubar && *tray {
		fmt.Fprintln(os.Stderr, "vero add: --menubar or --tray, not both: a menu bar app already has its icon, and --tray is for a window app")
		os.Exit(2)
	}
	for _, s := range []struct {
		on  bool
		key string
		set *bool
	}{{*menubar, "menubar", &app.MenuBar}, {*tray, "tray", &app.Tray}} {
		if !s.on {
			continue
		}
		*s.set = true
		if err := app.toml.setTrue(s.key); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	added, err := add(app, want, *force)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(added) == 0 {
		fmt.Println("nothing to add: every front end asked for is there already")
		return
	}
	fmt.Printf("added %s for %s\n", strings.Join(added, ", "), app.Name)
	if len(guessed) > 0 {
		list := strings.Join(guessed, ", ")
		if n := len(guessed); n > 1 {
			list = strings.Join(guessed[:n-1], ", ") + " and " + guessed[n-1]
		}
		fmt.Printf("vero-app.toml has a %s made up for you, which you can change there\n", list)
	}
	fmt.Println("next: read PORTING.md in " + app.Dir + ", then build each front end as its README says")
}

// choose is the front ends named, groups opened out, less the exceptions.
func choose(names []string, except string) ([]string, error) {
	if len(names) == 0 {
		names = []string{"desktop"}
	}
	var out []string
	for _, n := range names {
		n = strings.ToLower(n)
		if g, ok := groups[n]; ok {
			out = append(out, g...)
			continue
		}
		if a, ok := aliases[n]; ok {
			n = a
		}
		if !slices.Contains(platforms, n) {
			return nil, fmt.Errorf("no front end called %q: desktop, mobile, all, or %s", n, strings.Join(platforms, ", "))
		}
		out = append(out, n)
	}
	for _, x := range strings.Split(except, ",") {
		x = strings.ToLower(strings.TrimSpace(x))
		if a, ok := aliases[x]; ok {
			x = a
		}
		out = slices.DeleteFunc(out, func(p string) bool { return p == x })
	}
	slices.SortFunc(out, func(a, b string) int { return slices.Index(platforms, a) - slices.Index(platforms, b) })
	return slices.Compact(out), nil
}
