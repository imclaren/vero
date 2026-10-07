package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// starter is one front end's files, each a template over the App.
type starter struct {
	folder string
	files  map[string]string
	// bindings are files of vero's to copy in: "bindings/csharp/Vero.cs".
	bindings []string
	toml     string
	// note is what PORTING.md says about this front end in particular.
	note string
	// limited says the worker runs inside the app here, so it cannot start
	// programs or listen.
	limited bool
}

// add writes the front ends asked for that the app has not got, and
// returns their names.
func add(app *App, want []string, force bool) ([]string, error) {
	// A front end that runs the worker inside itself needs it as a package
	// with a Serve. Make one from the command, unless told to leave it.
	for _, p := range want {
		if s := starters[p]; s != nil && s.limited && !app.Existing[p] && !app.KeepWorker && app.ServeImport == "" {
			path, err := lift(app)
			if err != nil {
				fmt.Printf("%s: the worker stays as it is, and gets a placeholder, because its main is not the shape vero add can move: %v\n", p, err)
				app.KeepWorker = true
				break
			}
			app.ServeImport = path
			break
		}
	}
	var added []string
	for _, p := range want {
		s := starters[p]
		if s == nil {
			continue
		}
		dir := filepath.Join(app.Dir, s.folder)
		if app.Existing[p] && !force {
			continue
		}
		if _, err := os.Stat(dir); err == nil && !force {
			fmt.Printf("%s: %s/ is there already, so it is left alone (--force writes a starter beside it)\n", p, s.folder)
			continue
		}
		for name, text := range s.files {
			out, err := render(text, app)
			if err != nil {
				return added, fmt.Errorf("%s/%s: %v", s.folder, name, err)
			}
			// File names are templates too: "{{pascal .Name}}.csproj".
			named, err := render(name, app)
			if err != nil {
				return added, fmt.Errorf("%s/%s: %v", s.folder, name, err)
			}
			path := filepath.Join(dir, filepath.FromSlash(string(named)))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return added, err
			}
			perm := os.FileMode(0o644)
			if strings.HasSuffix(name, ".sh") || strings.HasSuffix(name, ".py") {
				perm = 0o755
			}
			if err := os.WriteFile(path, out, perm); err != nil {
				return added, err
			}
		}
		// The bindings a front end imports, copied from the vero module the
		// app uses, so that the starter builds as it is.
		for _, b := range s.bindings {
			if err := copyBinding(app, b, dir); err != nil {
				fmt.Printf("%s: %v\n", p, err)
			}
		}
		if s.toml != "" && !app.Existing[p] {
			section, err := render(s.toml, app)
			if err != nil {
				return added, err
			}
			if err := app.toml.write(app, string(section)); err != nil {
				return added, err
			}
		}
		added = append(added, p)
	}
	if len(added) == 0 {
		return nil, nil
	}
	return added, writePorting(app, added)
}

func render(text string, app *App) ([]byte, error) {
	t, err := template.New("").Funcs(funcs).Parse(text)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, app); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// writePorting writes PORTING.md: the front ends added, what each needs,
// and what the scan of the worker found.
func writePorting(app *App, added []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Taking %s to more systems\n\n", app.Display)
	fmt.Fprintf(&b, "`vero add` wrote a starter front end for each system below, wired to the worker in `%s/`: its state (`%s`) and its requests (%s). Each is a first draft to make your own, not a finished app. This file says what to look at; delete it when you are done.\n\n",
		app.Worker, app.State.Name, requestList(app))
	b.WriteString("## The front ends added\n\n")
	for _, p := range added {
		s := starters[p]
		fmt.Fprintf(&b, "- **%s** in `%s/`: %s\n", p, s.folder, s.note)
	}
	b.WriteString("\nEach folder has a README saying how to build and run it. `vero-app.toml` now lists them, so `scripts/package.sh` and `vero-repo build` package them (see vero's PACKAGING.md, \"Creating app installers\").\n\n")
	limited := false
	for _, p := range added {
		limited = limited || starters[p].limited
	}
	if len(app.Notes) > 0 {
		b.WriteString("## What the worker does that needs attention\n\n")
		for _, n := range app.Notes {
			if !limited && (n.Title == "listening on a port") {
				continue
			}
			fmt.Fprintf(&b, "### %s\n\n%s\n\n", strings.ToUpper(n.Title[:1])+n.Title[1:], n.Text)
			for _, w := range n.Where {
				fmt.Fprintf(&b, "- `%s`\n", w)
			}
			b.WriteString("\n")
		}
	} else {
		b.WriteString("## The worker\n\nNothing in the worker looks tied to one system: it should build and run as it is on each.\n\n")
	}
	b.WriteString("## Checklist\n\n")
	b.WriteString("- [ ] Build each front end as its README says, and run it against the worker.\n")
	b.WriteString("- [ ] Make the starter windows your own: they show every field of the state and a control for every request, which is a start, not a design.\n")
	b.WriteString("- [ ] Fill in `vero-app.toml`: the summary, description, publisher, homepage, licence and icon, and what each system's package needs.\n")
	if limited && app.ServeImport != "" {
		fmt.Fprintf(&b, "- [x] For iOS and the browser, the worker runs inside the app. `vero add` moved its code to `%s/`, which has a `Serve` those front ends call, and left `%s/main.go` to start it for the rest. Nothing else changed; check it builds and keep working in `%s/`.\n", servePackage, app.Worker, servePackage)
	} else if limited {
		b.WriteString("- [ ] For iOS and the browser, the worker runs inside the app: give it a `Serve(in io.Reader, out io.Writer) error` that sets up the same worker as `main` does, in a package the front end can import, and point the starters at it where they say. (`vero add` does this itself for a worker whose main makes a vero.WorkerOptions, calls vero.NewWorker with it, and ends with Serve.)\n")
	}
	b.WriteString("- [ ] Release: `scripts/package.sh --app vero-app.toml --version X`, `vero-repo build`, upload.\n")
	return os.WriteFile(filepath.Join(app.Dir, "PORTING.md"), []byte(b.String()), 0o644)
}

func requestList(app *App) string {
	var names []string
	for _, r := range app.Requests {
		names = append(names, "`"+r.Name+"`")
	}
	if len(names) == 0 {
		return "none yet"
	}
	return strings.Join(names, ", ")
}

// copyBinding copies one of vero's binding files into a front end's
// folder, from the vero module the app's go.mod names.
func copyBinding(app *App, rel, dir string) error {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/imclaren/vero")
	cmd.Dir = app.Dir
	out, err := cmd.Output()
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return fmt.Errorf("couldn't find the vero module to copy %s from: add github.com/imclaren/vero to go.mod, then copy it by hand", filepath.Base(rel))
	}
	data, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(out)), filepath.FromSlash(rel)))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, filepath.Base(rel)), data, 0o644)
}
