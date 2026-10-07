package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// workerFile is one of the worker's source files, parsed.
type workerFile struct {
	name string
	ast  *ast.File
}

// servePackage is where the worker's code goes so that a front end can
// compile it in: internal, so that only this module imports it.
const servePackage = "internal/worker"

// lift moves the worker's code out of its command into servePackage, a
// package the iOS and browser front ends can import, since neither may
// start a program and so must run the worker inside themselves. The
// command is left as a few lines that parse the flags and call Run; the
// package gains Serve(in, out), which those front ends call. It returns
// the package's import path.
//
// It takes a worker of the shape vero's example has: a main that makes a
// vero.WorkerOptions, calls vero.NewWorker with it, sets the worker up,
// and ends by calling its Serve. A main of another shape is left alone,
// with an error saying why, and the front ends get a placeholder.
func lift(app *App) (string, error) {
	dir := filepath.Join(app.Dir, app.Worker)
	dest := filepath.Join(app.Dir, filepath.FromSlash(servePackage))
	path := app.Module + "/" + servePackage
	if _, err := os.Stat(filepath.Join(dest, "serve.go")); err == nil {
		return path, nil // lifted on an earlier run
	}

	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var files []workerFile
	var mainFile *ast.File
	var mainFunc *ast.FuncDecl
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ParseComments)
		if err != nil {
			return "", err
		}
		if f.Name.Name != "main" {
			return "", fmt.Errorf("%s is package %s, not main", e.Name(), f.Name.Name)
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "main" {
				mainFile, mainFunc = f, fn
			}
		}
		files = append(files, workerFile{e.Name(), f})
	}
	if mainFunc == nil {
		return "", errors.New("no func main in the worker")
	}

	version, err := toRun(fset, mainFile, mainFunc)
	if err != nil {
		return "", err
	}
	if version == "" {
		version = versionVar(files)
	}
	if version == "" {
		version = "0.0.0"
	}

	// The command's version is its own; the package keeps the variable only
	// if something else still reads it.
	if !mentions(files, "version", mainFile) {
		dropVersion(fset, mainFile)
	}

	// Write the package, then the command, then take the old files away:
	// a failure part way leaves the old files where they were.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	var written []string
	for _, f := range files {
		f.ast.Name.Name = "worker"
		pruneImports(f.ast)
		if f.ast.Doc != nil {
			dropComments(f.ast, []ast.Node{f.ast.Doc})
		}
		var b bytes.Buffer
		if err := format.Node(&b, fset, f.ast); err != nil {
			return "", fmt.Errorf("%s: %v", f.name, err)
		}
		src, err := format.Source(b.Bytes())
		if err != nil {
			return "", fmt.Errorf("%s, after moving: %v", f.name, err)
		}
		name := f.name
		if name == "main.go" {
			name = "worker.go"
		}
		if f.ast == mainFile {
			src = bytes.Replace(src, []byte("func Run("), []byte(runDoc+"func Run("), 1)
			src = bytes.Replace(src, []byte("error {\n\n"), []byte("error {\n"), 1)
		}
		if f.ast.Doc != nil {
			// The comment at the top described the command; this is the
			// package now.
			src = bytes.Replace(src, []byte("package worker"), []byte(strings.NewReplacer("{{.Display}}", app.Display).Replace(packageDoc)+"package worker"), 1)
		}
		if err := os.WriteFile(filepath.Join(dest, name), src, 0o644); err != nil {
			return "", err
		}
		written = append(written, name)
	}
	serve := strings.NewReplacer("{{.Version}}", version).Replace(serveFile)
	if err := os.WriteFile(filepath.Join(dest, "serve.go"), []byte(serve), 0o644); err != nil {
		return "", err
	}
	stub := strings.NewReplacer("{{.Name}}", app.WorkerName, "{{.Display}}", app.Display, "{{.Import}}", path,
		"{{.Version}}", version, "{{.Package}}", servePackage).Replace(stubFile)
	for _, f := range files {
		os.Remove(filepath.Join(dir, f.name))
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(stub), 0o644); err != nil {
		return "", err
	}
	fmt.Printf("moved the worker's code to %s/ (%s), with a Serve for the front ends that run it inside themselves; %s/main.go now just starts it\n",
		servePackage, strings.Join(written, ", "), app.Worker)
	return path, nil
}

// toRun turns the worker's main into Run(opts vero.WorkerOptions) error:
// the flag parsing goes, since the caller did that or has no flags; the
// options become the parameter; and the Serve at the end returns. It
// returns the version main gave its options, for the command to keep.
func toRun(fset *token.FileSet, f *ast.File, fn *ast.FuncDecl) (version string, err error) {
	body := fn.Body.List
	at := -1
	var optsName, workerName string
	var before []ast.Stmt // assignments that stand in for a literal's fields
	for i, s := range body {
		as, ok := s.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 || len(as.Lhs) != 1 {
			continue
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok || typeName(call.Fun) != "vero.NewWorker" || len(call.Args) != 1 {
			continue
		}
		lhs, ok := as.Lhs[0].(*ast.Ident)
		if !ok {
			return "", errors.New("vero.NewWorker's result is not a plain variable")
		}
		workerName = lhs.Name
		switch arg := call.Args[0].(type) {
		case *ast.Ident:
			optsName = arg.Name
		case *ast.CompositeLit:
			optsName = "opts"
			for _, el := range arg.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					return "", errors.New("vero.WorkerOptions{...} without field names")
				}
				key := typeName(kv.Key)
				if key == "In" || key == "Out" {
					continue
				}
				if key == "Version" {
					if v, ok := stringLit(kv.Value); ok {
						version = v
					}
				}
				before = append(before, &ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent("opts"), Sel: ast.NewIdent(key)}},
					Tok: token.ASSIGN, Rhs: []ast.Expr{kv.Value}})
			}
			call.Args[0] = ast.NewIdent("opts")
		default:
			return "", errors.New("vero.NewWorker's argument is neither a variable nor a vero.WorkerOptions{...}")
		}
		at = i
		break
	}
	if at < 0 {
		return "", errors.New("main does not call vero.NewWorker itself")
	}

	// What came before NewWorker and touched the options, the flags or the
	// version was setting the command up; the caller does that now. The
	// rest stays.
	var kept []ast.Stmt
	var removed []ast.Node
	for _, s := range body[:at] {
		if touches(s, optsName) || touches(s, "version") || usesPackage(s, "flag") {
			removed = append(removed, s)
			if v := versionIn(s); v != "" {
				version = v
			}
			continue
		}
		kept = append(kept, s)
	}
	kept = append(kept, before...)
	after := body[at:]

	// The Serve call at the end becomes the return.
	served := false
	for i, s := range after {
		call := serveCall(s, workerName)
		if call == nil {
			continue
		}
		if i == len(after)-1 {
			after[i] = &ast.ReturnStmt{Results: []ast.Expr{call}}
		} else {
			after[i] = &ast.IfStmt{
				Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("err")}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
				Cond: &ast.BinaryExpr{X: ast.NewIdent("err"), Op: token.NEQ, Y: ast.NewIdent("nil")},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("err")}}}},
			}
			if _, isReturn := after[len(after)-1].(*ast.ReturnStmt); !isReturn {
				after = append(after, &ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("nil")}})
			}
		}
		served = true
		break
	}
	if !served {
		return "", fmt.Errorf("main does not end by calling %s.Serve()", workerName)
	}
	fn.Body.List = append(kept, after...)

	// A bare return in main is a return of nothing; here it returns nil.
	// Function literals inside keep their own returns.
	var fix func(n ast.Node) bool
	fix = func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			if len(n.Results) == 0 {
				n.Results = []ast.Expr{ast.NewIdent("nil")}
			}
		}
		return true
	}
	ast.Inspect(fn.Body, fix)

	fn.Name = ast.NewIdent("Run")
	fn.Type.Params = &ast.FieldList{List: []*ast.Field{{
		Names: []*ast.Ident{ast.NewIdent(optsName)},
		Type:  &ast.SelectorExpr{X: ast.NewIdent("vero"), Sel: ast.NewIdent("WorkerOptions")}}}}
	fn.Type.Results = &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("error")}}}
	if fn.Doc != nil {
		removed = append(removed, fn.Doc)
		fn.Doc = nil
	}
	dropComments(f, removed)
	return version, nil
}

// versionVar is the string `var version = "..."` holds, in any of the
// files, or "".
func versionVar(files []workerFile) string {
	for _, f := range files {
		for _, d := range f.ast.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.VAR {
				continue
			}
			for _, sp := range g.Specs {
				vs := sp.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if n.Name == "version" && i < len(vs.Values) {
						if v, ok := stringLit(vs.Values[i]); ok {
							return v
						}
					}
				}
			}
		}
	}
	return ""
}

// serveCall is the worker's Serve call in a statement of the kinds a main
// ends with - `w.Serve()`, `if err := w.Serve(); err != nil {...}`,
// `log.Fatal(w.Serve())` - or nil.
func serveCall(s ast.Stmt, worker string) *ast.CallExpr {
	isServe := func(e ast.Expr) *ast.CallExpr {
		if call, ok := e.(*ast.CallExpr); ok && typeName(call.Fun) == worker+".Serve" && len(call.Args) == 0 {
			return call
		}
		return nil
	}
	switch s := s.(type) {
	case *ast.ExprStmt:
		if c := isServe(s.X); c != nil {
			return c
		}
		if outer, ok := s.X.(*ast.CallExpr); ok && len(outer.Args) == 1 {
			return isServe(outer.Args[0])
		}
	case *ast.IfStmt:
		if as, ok := s.Init.(*ast.AssignStmt); ok && len(as.Rhs) == 1 {
			return isServe(as.Rhs[0])
		}
	case *ast.AssignStmt:
		if len(s.Rhs) == 1 {
			return isServe(s.Rhs[0])
		}
	}
	return nil
}

// touches says whether a statement mentions a name.
func touches(s ast.Stmt, name string) bool {
	found := false
	ast.Inspect(s, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// usesPackage says whether a statement calls into a package by name.
func usesPackage(s ast.Stmt, pkg string) bool {
	found := false
	ast.Inspect(s, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == pkg {
				found = true
			}
		}
		return !found
	})
	return found
}

// versionIn is the string a statement like `opts.Version = "1.2.3"`
// assigns, or "".
func versionIn(s ast.Stmt) string {
	as, ok := s.(*ast.AssignStmt)
	if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return ""
	}
	if sel, ok := as.Lhs[0].(*ast.SelectorExpr); ok && sel.Sel.Name == "Version" {
		if v, ok := stringLit(as.Rhs[0]); ok {
			return v
		}
	}
	return ""
}

// mentions says whether any file uses a name other than to declare it at
// the top of the one file named.
func mentions(files []workerFile, name string, declaredIn *ast.File) bool {
	found := false
	for _, f := range files {
		for _, d := range f.ast.Decls {
			if g, ok := d.(*ast.GenDecl); ok && f.ast == declaredIn && g.Tok == token.VAR && declares(g, name) {
				// The declaration's own value may not mention it.
				continue
			}
			ast.Inspect(d, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && id.Name == name {
					found = true
				}
				return !found
			})
		}
	}
	return found
}

func declares(g *ast.GenDecl, name string) bool {
	for _, s := range g.Specs {
		if vs, ok := s.(*ast.ValueSpec); ok {
			for _, n := range vs.Names {
				if n.Name == name {
					return true
				}
			}
		}
	}
	return false
}

// dropVersion takes `var version = ...` out of a file, with its comment.
func dropVersion(fset *token.FileSet, f *ast.File) {
	for i, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR && len(g.Specs) == 1 && declares(g, "version") {
			var gone []ast.Node
			gone = append(gone, g)
			if g.Doc != nil {
				gone = append(gone, g.Doc)
			}
			f.Decls = append(f.Decls[:i], f.Decls[i+1:]...)
			dropComments(f, gone)
			return
		}
	}
}

// dropComments takes out of the file's comments any that sat inside the
// nodes removed, which the printer would otherwise leave floating.
func dropComments(f *ast.File, removed []ast.Node) {
	var kept []*ast.CommentGroup
	for _, c := range f.Comments {
		inside := false
		for _, r := range removed {
			if c.Pos() >= r.Pos() && c.End() <= r.End() {
				inside = true
				break
			}
		}
		if !inside {
			kept = append(kept, c)
		}
	}
	f.Comments = kept
}

// pruneImports takes out imports nothing uses any more, such as "flag"
// after the flag parsing went.
func pruneImports(f *ast.File) {
	used := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				used[id.Name] = true
			}
		}
		return true
	})
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.IMPORT {
			continue
		}
		var specs []ast.Spec
		for _, s := range g.Specs {
			im := s.(*ast.ImportSpec)
			path := strings.Trim(im.Path.Value, `"`)
			name := path[strings.LastIndex(path, "/")+1:]
			if im.Name != nil {
				name = im.Name.Name
			}
			if name == "_" || name == "." || name == "C" || used[name] || used[strings.TrimSuffix(name, "-go")] {
				specs = append(specs, s)
				continue
			}
			// Paths like gopkg.in/yaml.v3 are used as "yaml".
			if i := strings.Index(name, "."); i > 0 && used[name[:i]] {
				specs = append(specs, s)
			}
		}
		g.Specs = specs
	}
	var decls []ast.Decl
	for _, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT && len(g.Specs) == 0 {
			continue
		}
		decls = append(decls, d)
	}
	f.Decls = decls
	f.Imports = nil
}

const packageDoc = `// Package worker is {{.Display}}'s worker: its state, its requests and its
// work, as a package so that every front end can have it. The command in
// cmd starts it as a program; iOS and the browser, which may not, run it
// inside themselves through Serve.
`

const runDoc = `// Run is the worker: its state, its requests and its work. It answers on
// the pipes opts names until they close. The command calls it with the
// flags it parsed, and Serve calls it for a front end that runs the worker
// inside itself.
`

const serveFile = `package worker

import (
	"io"

	"github.com/imclaren/vero"
)

// Version is what the worker reports when a front end runs it inside
// itself, where no flags reach it. The command has a version of its own,
// which its build sets.
var Version = "{{.Version}}"

// Serve runs the worker on the pipes given, inside the program that calls
// it: vero.ServeInProcess on iOS, and SupervisorOptions.Serve in a browser,
// where an app may not start a program.
func Serve(in io.Reader, out io.Writer) error {
	return Run(vero.WorkerOptions{In: in, Out: out, Version: Version})
}
`

const stubFile = `// Command {{.Name}} is {{.Display}}'s worker: the Go half of the app, which
// each front end starts and speaks JSON with over a pipe. The work is in
// {{.Package}}, so that the iOS and browser front ends, which may not
// start a program, can run it inside themselves; this is what the others
// start.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/imclaren/vero"

	worker "{{.Import}}"
)

// version has to increase on every release, so the frontend can tell which
// of two copies is newer. The build sets it.
var version = "{{.Version}}"

func main() {
	var opts vero.WorkerOptions
	opts.Version = version
	opts.RegisterFlags(flag.CommandLine)
	flag.Parse()
	opts.PrintVersionAndExit()
	if err := worker.Run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "{{.Name}}:", err)
		os.Exit(1)
	}
}
`
