package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// App is what vero add learned of the app.
type App struct {
	Dir    string
	Module string // the Go module path
	// Name is the package and command name, from vero-app.toml or the
	// module's last part; Display what people see; ID the reverse-domain id.
	Name, Display, ID string
	// Worker is the worker's folder, relative to Dir, and its program's name.
	Worker, WorkerName string
	// State is the type the worker pushes, and Types every struct it refers
	// to, by name.
	State *Struct
	Types map[string]*Struct
	// Requests are the handlers the worker has, in the order found.
	Requests []Request
	// Notes are what PORTING.md says, by concern.
	Notes []Note
	// Existing are the front ends found already: "gtk", "wpf", ...
	Existing map[string]bool
	toml     *tomlFile
}

// Struct is a Go struct the front ends need the shape of.
type Struct struct {
	Name   string
	Fields []Field
}

// Field is one field of one.
type Field struct {
	Name string // Go
	JSON string // its JSON key
	// Kind is string, int, float, bool, list, struct or any; Elem the
	// struct's name for struct and list-of-struct, "" for a list of
	// scalars.
	Kind, Elem string
	Scalar     bool
}

// Request is one handler on the worker.
type Request struct {
	Name string
	// Fields are what it takes: none for a request with no data.
	Fields []Field
	// Replies says it answers with the state, or something else.
	RepliesState bool
}

// Note is one thing PORTING.md says.
type Note struct {
	Title, Text string
	Where       []string // file:line
}

// analyse reads the app's folder: its module, its worker, and the front
// ends it has.
func analyse(dir string) (*App, error) {
	dir, _ = filepath.Abs(dir)
	mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return nil, fmt.Errorf("run this in your app's folder, the one with its go.mod: %v", err)
	}
	app := &App{Dir: dir, Types: map[string]*Struct{}, Existing: map[string]bool{}}
	if m := regexp.MustCompile(`(?m)^module\s+(\S+)`).FindSubmatch(mod); m != nil {
		app.Module = string(m[1])
	}
	app.toml = readToml(filepath.Join(dir, "vero-app.toml"))
	app.Name = app.toml.get("", "name")
	if app.Name == "" {
		app.Name = strings.ToLower(filepath.Base(app.Module))
	}
	app.Display = app.toml.get("", "display_name")
	if app.Display == "" {
		app.Display = app.Name
	}
	app.ID = app.toml.get("", "id")
	if app.ID == "" {
		app.ID = "com.example." + strings.ReplaceAll(app.Name, "-", "")
	}
	for _, p := range platforms {
		app.Existing[p] = app.toml.has(sectionOf[p])
	}
	if err := app.findWorker(); err != nil {
		return nil, err
	}
	app.scan()
	return app, nil
}

// sectionOf is each front end's section in vero-app.toml.
var sectionOf = map[string]string{"macos": "macos", "gtk": "gtk", "wpf": "wpf", "android": "android", "ios": "ios", "web": "web", "wasi": "wasi", "plan9": "plan9"}

// findWorker finds the package that calls vero.NewWorker, and reads its
// state and requests.
func (a *App) findWorker() error {
	if w := a.toml.get("worker", "package"); w != "" {
		a.Worker = w
	}
	fset := token.NewFileSet()
	var found string
	var files []*ast.File
	filepath.WalkDir(a.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); n != "." && (strings.HasPrefix(n, ".") || n == "vendor" || n == "node_modules" || n == "dist") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if a.Worker != "" && filepath.Dir(path) != filepath.Join(a.Dir, a.Worker) {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(src), "vero.NewWorker(") {
			return nil
		}
		found = filepath.Dir(path)
		return nil
	})
	if found == "" {
		return fmt.Errorf("no worker found: a package that calls vero.NewWorker")
	}
	rel, _ := filepath.Rel(a.Dir, found)
	a.Worker = filepath.ToSlash(rel)
	a.WorkerName = a.toml.get("worker", "name")
	if a.WorkerName == "" {
		a.WorkerName = filepath.Base(found)
		if a.WorkerName == "worker" || a.WorkerName == "." {
			a.WorkerName = a.Name + "-worker"
		}
	}
	entries, _ := os.ReadDir(found)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			f, err := parser.ParseFile(fset, filepath.Join(found, e.Name()), nil, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("%s: %v", e.Name(), err)
			}
			files = append(files, f)
		}
	}
	structs := map[string]*ast.StructType{}
	for _, f := range files {
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok {
				for _, s := range g.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok {
						if st, ok := ts.Type.(*ast.StructType); ok {
							structs[ts.Name.Name] = st
						}
					}
				}
			}
		}
	}
	// The state's type: written in vero.NewState(w, Status{...}), or, for
	// a worker that passes a variable, wherever it says vero.State[Status].
	var stateName, declared string
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if ix, ok := n.(*ast.IndexExpr); ok && typeName(ix.X) == "vero.State" {
				declared = typeName(ix.Index)
				return true
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "vero" {
				return true
			}
			switch sel.Sel.Name {
			case "NewState":
				if len(call.Args) > 1 {
					if cl, ok := call.Args[1].(*ast.CompositeLit); ok {
						stateName = typeName(cl.Type)
					} else if id, ok := call.Args[1].(*ast.Ident); ok {
						stateName = id.Name
					}
				}
			case "Update", "UpdateWith", "UpdateItem", "Handle":
				if len(call.Args) < 3 {
					return true
				}
				name, ok := stringLit(call.Args[1])
				if !ok {
					return true
				}
				r := Request{Name: name, RepliesState: sel.Sel.Name != "Handle"}
				switch sel.Sel.Name {
				case "UpdateWith":
					if fn, ok := call.Args[2].(*ast.FuncLit); ok && len(fn.Type.Params.List) >= 2 {
						r.Fields = a.fieldsOf(fn.Type.Params.List[len(fn.Type.Params.List)-1].Type, structs)
					}
				case "Handle":
					if fn, ok := call.Args[2].(*ast.FuncLit); ok && len(fn.Type.Params.List) >= 2 {
						r.Fields = a.fieldsOf(fn.Type.Params.List[1].Type, structs)
					}
				case "UpdateItem":
					r.Fields = []Field{{Name: "ID", JSON: "id", Kind: "int", Scalar: true}}
				}
				a.Requests = append(a.Requests, r)
			}
			return true
		})
	}
	if structs[stateName] == nil {
		stateName = declared
	}
	if stateName == "" || structs[stateName] == nil {
		return fmt.Errorf("no state found: a vero.NewState(w, YourStatus{...}), or a vero.State[YourStatus], in %s", a.Worker)
	}
	a.State = a.structOf(stateName, structs)
	return nil
}

// fieldsOf is what a request type carries: vero.ID[K] is an id, a struct of
// the package is its fields, anything else nothing a starter can fill in.
func (a *App) fieldsOf(t ast.Expr, structs map[string]*ast.StructType) []Field {
	switch x := t.(type) {
	case *ast.IndexExpr:
		if typeName(x.X) == "vero.ID" {
			return []Field{{Name: "ID", JSON: "id", Kind: kindOf(typeName(x.Index)), Scalar: true}}
		}
	case *ast.Ident:
		if structs[x.Name] != nil {
			return a.structOf(x.Name, structs).Fields
		}
	}
	return nil
}

// structOf reads a struct's fields, and those of the structs it refers
// to, into Types.
func (a *App) structOf(name string, structs map[string]*ast.StructType) *Struct {
	if s, ok := a.Types[name]; ok {
		return s
	}
	s := &Struct{Name: name}
	a.Types[name] = s
	st := structs[name]
	if st == nil {
		return s
	}
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 || !ast.IsExported(f.Names[0].Name) {
			continue
		}
		fld := Field{Name: f.Names[0].Name, JSON: f.Names[0].Name}
		if f.Tag != nil {
			tag, _ := strconv.Unquote(f.Tag.Value)
			if j := regexp.MustCompile(`json:"([^",]*)`).FindStringSubmatch(tag); j != nil {
				if j[1] == "-" {
					continue
				}
				if j[1] != "" {
					fld.JSON = j[1]
				}
			}
		}
		fld.Kind, fld.Elem = a.kindOfExpr(f.Type, structs)
		fld.Scalar = fld.Kind != "list" && fld.Kind != "struct" && fld.Kind != "any"
		s.Fields = append(s.Fields, fld)
	}
	return s
}

func (a *App) kindOfExpr(t ast.Expr, structs map[string]*ast.StructType) (kind, elem string) {
	switch x := t.(type) {
	case *ast.Ident:
		if structs[x.Name] != nil {
			a.structOf(x.Name, structs)
			return "struct", x.Name
		}
		return kindOf(x.Name), ""
	case *ast.StarExpr:
		return a.kindOfExpr(x.X, structs)
	case *ast.ArrayType:
		k, e := a.kindOfExpr(x.Elt, structs)
		if k == "struct" {
			return "list", e
		}
		return "list", ""
	case *ast.SelectorExpr:
		if typeName(x) == "time.Time" {
			return "string", ""
		}
	}
	return "any", ""
}

func kindOf(goType string) string {
	switch goType {
	case "string":
		return "string"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "int"
	case "float32", "float64":
		return "float"
	case "bool":
		return "bool"
	}
	return "any"
}

func typeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return typeName(x.X) + "." + x.Sel.Name
	case *ast.StarExpr:
		return typeName(x.X)
	case *ast.IndexExpr:
		return typeName(x.X)
	}
	return ""
}

func stringLit(e ast.Expr) (string, bool) {
	if l, ok := e.(*ast.BasicLit); ok && l.Kind == token.STRING {
		s, err := strconv.Unquote(l.Value)
		return s, err == nil
	}
	return "", false
}

// scan reads every Go file of the module for what needs attention on the
// new systems, into Notes.
func (a *App) scan() {
	type hit struct {
		re          *regexp.Regexp
		title, text string
	}
	hits := []hit{
		{regexp.MustCompile(`^import "C"|^\s*#cgo\b`), "cgo",
			"These files use cgo, which needs a C compiler for every system you build for, and stops the pure-Go cross-compile that builds the worker for every system from one Mac. Move what they do onto vero's kit packages (keychain for secrets) or a pure-Go library, or put them behind a build tag with a pure-Go fallback."},
		{regexp.MustCompile(`\bexec\.(Command|CommandContext|LookPath)\(`), "programs the worker runs",
			"The worker starts other programs. Ship each for every system and architecture, and find them with kit/tools, which looks wherever the app is installed. On iOS and in a browser an app may not start a program at all, so those front ends need another way, or leave this feature out."},
		{regexp.MustCompile(`\b(net\.Listen|http\.ListenAndServe|http\.Serve|net\.ListenTCP|httptest\.NewServer)\(`), "listening on a port",
			"The worker listens on a network port. On iOS and in a browser the worker runs inside the app, where it cannot listen; serve whatever this serves another way there (a file, or the front end asking the worker for it)."},
		{regexp.MustCompile(`"github\.com/keybase/go-keychain"|"github\.com/zalando/go-keyring"|internal/keychain"|/keychain"`), "secrets",
			"Sign-ins and other secrets are kept with a keychain library of the app's own. vero's kit/keychain does this on every system - the login keychain, the Credential Manager, the Secret Service, or a private file - with one API."},
		{regexp.MustCompile(`LaunchAgents|SMAppService|launchctl|/autostart/|CurrentVersion\\\\Run`), "opening at sign-in",
			"The app opens at sign-in in a way that is one system's. kit/autostart does it on each."},
		{regexp.MustCompile(`UNUserNotificationCenter|org\.freedesktop\.Notifications|ToastNotification`), "notifications",
			"Notifications are shown one system's way. Either keep what is new in the worker's state and let each front end notify (works everywhere), or use kit/notify from the worker where the system allows it."},
		{regexp.MustCompile(`appcast|Sparkle|latest\.json`), "updates",
			"The app checks for updates its own way. kit/update reads the latest.json and the Sparkle appcast that vero-repo build publishes, for every system; Sparkle still handles macOS itself."},
		{regexp.MustCompile(`\bsyscall\.|golang\.org/x/sys/(unix|windows)`), "system calls",
			"These files use a system's own calls. Check each builds for the systems you add, or put it behind a build tag with a fallback."},
		{regexp.MustCompile(`//go:build .*(darwin|windows|linux)`), "build tags",
			"These files are for one system. Make sure each has a counterpart, or a fallback, for the systems you add."},
	}
	found := map[string]*Note{}
	filepath.WalkDir(a.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); n != "." && (strings.HasPrefix(n, ".") || n == "vendor" || n == "node_modules" || n == "dist") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(a.Dir, path)
		for i, line := range strings.Split(string(src), "\n") {
			for _, h := range hits {
				if h.re.MatchString(line) {
					n := found[h.title]
					if n == nil {
						n = &Note{Title: h.title, Text: h.text}
						found[h.title] = n
					}
					if len(n.Where) < 8 {
						n.Where = append(n.Where, fmt.Sprintf("%s:%d", filepath.ToSlash(rel), i+1))
					}
				}
			}
		}
		return nil
	})
	var titles []string
	for t := range found {
		titles = append(titles, t)
	}
	sort.Strings(titles)
	for _, t := range titles {
		a.Notes = append(a.Notes, *found[t])
	}
}
