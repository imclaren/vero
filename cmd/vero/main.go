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
		case "release", "credentials", "publish", "key", "package", "build", "check", "steps", "show":
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
		fmt.Fprintln(os.Stderr, "usage: vero new NAME [front ends, as for add] [--module PATH] [add's flags]\n"+
			"       vero add [desktop|mobile|all|macos|gtk|wpf|android|ios|web|wasi|plan9 ...] [--except LIST] [--dir DIR]\n"+
			"                [--summary TEXT] [--description TEXT] [--publisher \"Name <email>\"] [--keep-worker] [--force]\n"+
			"       vero release [--version 1.2.3] [--targets ...] [--notes \"...\"] [--upload user@host:/path]\n"+
			"       vero credentials\n"+
			"       vero publish --homebrew | --winget | --aur | --all\n"+
			"       vero key | package | build | check | steps | show ...   vero-repo's own commands, which vero runs for you")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	except := fs.String("except", "", "front ends to leave out, comma-separated")
	dir := fs.String("dir", ".", "the app's folder")
	force := fs.Bool("force", false, "write a starter even where a front end's folder exists")
	summary := fs.String("summary", "", "one line about the app, for vero-app.toml")
	description := fs.String("description", "", "a sentence or two about the app, for vero-app.toml")
	publisher := fs.String("publisher", "", "who publishes it, as \"Name <email>\", for vero-app.toml")
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
	app.Description = describe(Description{Summary: *summary, Text: *description, Publisher: *publisher},
		app.toml.text == "", os.Stdin, os.Stdout, terminal())
	app.KeepWorker = *keepWorker
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
