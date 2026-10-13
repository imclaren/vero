package main

// vero gen writes each front end's requests from the worker's own types:
// a type for every request, reply and the state, and a call for each
// request, in Swift, C# and Python. It reads the worker with Go's type
// checker, so that it follows types into any package of the module, and
// keeps their comments.

import (
	"flag"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"
)

const veroPath = "github.com/imclaren/vero"

// API is what the worker offers its front ends.
type API struct {
	// State is the type the worker pushes, or nil.
	State *TypeRef
	// Requests are in the order of their names.
	Requests []*APIRequest
	// Types are every named type the requests and state use, by name.
	Types []*APIType
	// Notes are what vero gen tells you: the types it had to rename.
	Notes []string
	// SwiftNonisolated marks the Swift types nonisolated, for an app whose
	// code is on the main actor unless it says, as Xcode 26's new projects
	// are: without it, the types could be used on the main actor alone.
	SwiftNonisolated bool
}

// APIRequest is one handler.
type APIRequest struct {
	Name string
	Doc  string
	// Fields are what the request takes, when it is a struct; Value is the
	// whole request when it is not, and both are empty for none.
	Fields []*APIField
	Value  *TypeRef
	Reply  *TypeRef
	// Where is the file and line of the vero.Handle call.
	Where string
}

// APIType is a named type: a struct, or another name for a scalar.
type APIType struct {
	Name   string
	Doc    string
	Fields []*APIField // a struct
	Alias  *TypeRef    // a scalar, such as type Speed string
}

// APIField is one field as JSON has it.
type APIField struct {
	JSON string
	Doc  string
	Type *TypeRef
}

// TypeRef is a type as JSON carries it.
type TypeRef struct {
	// Kind is bool, int, float, string, json (any value), list, map,
	// optional, named or empty.
	Kind string
	Elem *TypeRef // list, map and optional
	Name string   // named
	// Time marks a string that is Go's time.Time, in RFC 3339, which Swift
	// reads as a Date; the other languages keep the string, which starts
	// as goZeroTime, since Go cannot read an empty one.
	Time bool
}

// goZeroTime is Go's zero time.Time as JSON carries it.
const goZeroTime = "0001-01-01T00:00:00Z"

func gen(args []string) error {
	fs := flag.NewFlagSet("gen", flag.ExitOnError)
	dir := fs.String("dir", ".", "the app's folder")
	check := fs.Bool("check", false, "write nothing, and fail if a file is out of date")
	outDir := fs.String("out", "", "write the files into this folder instead, to look at them")
	fs.Parse(args)
	app, err := analyseWith(*dir, true)
	if err != nil {
		return err
	}
	api, err := readAPI(app.Dir, app.Worker)
	if err != nil {
		return err
	}
	for _, note := range api.Notes {
		fmt.Println(note)
	}
	outs := genOutputs(app)
	if *outDir != "" {
		// Every language, to look at, whatever front ends the app has.
		outs = []genOutput{{lang: "swift", path: "WorkerAPI.swift"}, {lang: "python", path: "worker_api.py"}, {lang: "csharp", path: "WorkerApi.cs", namespace: pascal(app.Name)}}
	}
	stale := 0
	for _, out := range outs {
		var text string
		switch out.lang {
		case "swift":
			api.SwiftNonisolated = mainActorByDefault(app)
			text = swiftAPI(api)
		case "csharp":
			text = csharpAPI(api, out.namespace)
		case "python":
			text = pythonAPI(api)
		}
		path := filepath.Join(app.Dir, filepath.FromSlash(out.path))
		if *outDir != "" {
			path = filepath.Join(*outDir, filepath.Base(out.path))
		}
		old, _ := os.ReadFile(path)
		if string(old) == text {
			continue
		}
		if *check {
			fmt.Printf("%s is out of date: run vero gen\n", out.path)
			stale++
			continue
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return err
		}
		if *outDir != "" {
			fmt.Printf("wrote %s\n", path)
		} else {
			fmt.Printf("wrote %s\n", out.path)
		}
	}
	if stale > 0 {
		return fmt.Errorf("%d generated files are out of date", stale)
	}
	return nil
}

// genOutput is one file vero gen writes.
type genOutput struct {
	lang, path, namespace string
}

// genOutputs are the files for the front ends the app has: beside vero's
// bindings in each, or where [gen] in vero-app.toml says.
// mainActorByDefault says whether the Mac app's Swift is on the main actor
// unless it says otherwise: SWIFT_DEFAULT_ACTOR_ISOLATION in its Xcode
// project, or defaultIsolation in its Package.swift.
func mainActorByDefault(app *App) bool {
	folder := filepath.Join(app.Dir, app.toml.get("macos", "folder"))
	projects, _ := filepath.Glob(filepath.Join(folder, "*.xcodeproj", "project.pbxproj"))
	for _, p := range projects {
		if data, err := os.ReadFile(p); err == nil && strings.Contains(string(data), "SWIFT_DEFAULT_ACTOR_ISOLATION = MainActor") {
			return true
		}
	}
	data, _ := os.ReadFile(filepath.Join(folder, "Package.swift"))
	return strings.Contains(string(data), "defaultIsolation(MainActor")
}

func genOutputs(app *App) []genOutput {
	var outs []genOutput
	at := func(key, def string) string {
		if p := app.toml.get("gen", key); p != "" {
			return p
		}
		return def
	}
	if app.Existing["macos"] {
		folder := app.toml.get("macos", "folder")
		def := filepath.Join(folder, "WorkerAPI.swift")
		if product := app.toml.get("macos", "product"); product != "" {
			def = filepath.Join(folder, "Sources", product, "WorkerAPI.swift")
		} else if scheme := app.toml.get("macos", "scheme"); scheme != "" {
			if st, err := os.Stat(filepath.Join(app.Dir, folder, scheme)); err == nil && st.IsDir() {
				def = filepath.Join(folder, scheme, "WorkerAPI.swift")
			}
		}
		outs = append(outs, genOutput{lang: "swift", path: filepath.ToSlash(at("swift", def))})
	}
	if app.Existing["gtk"] {
		outs = append(outs, genOutput{lang: "python", path: filepath.ToSlash(at("python", filepath.Join(app.toml.get("gtk", "folder"), "worker_api.py")))})
	}
	if app.Existing["wpf"] {
		outs = append(outs, genOutput{lang: "csharp", path: filepath.ToSlash(at("csharp", filepath.Join(app.toml.get("wpf", "folder"), "WorkerApi.cs"))), namespace: pascal(app.Name)})
	}
	return outs
}

// readAPI type-checks the module and finds the worker's requests and
// state: every call to vero.Handle, vero.Update, vero.UpdateWith and
// vero.NewState in the module's packages.
func readAPI(dir, worker string) (*API, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports | packages.NeedModule,
		Dir: dir,
	}
	pkgs, err := packages.Load(cfg, "./"+filepath.ToSlash(worker))
	if err != nil {
		return nil, err
	}
	var problems []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			problems = append(problems, e.Error())
		}
	})
	if len(problems) > 0 {
		return nil, fmt.Errorf("the worker does not build:\n  %s", strings.Join(problems, "\n  "))
	}
	module := ""
	if len(pkgs) > 0 && pkgs[0].Module != nil {
		module = pkgs[0].Module.Path
	}

	r := &reader{docs: map[token.Pos]string{}, named: map[*types.TypeName]*APIType{}, names: map[string]*types.TypeName{}}
	// The module's own packages, which are the only ones whose calls count
	// and whose comments are kept.
	var own []*packages.Package
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.Module != nil && p.Module.Path == module {
			own = append(own, p)
			r.fset = p.Fset
		}
	})
	for _, p := range own {
		for _, f := range p.Syntax {
			r.collectDocs(f)
		}
	}
	// First every call, so that each request's own types can have their
	// names before any other type takes one.
	type found struct {
		p     *packages.Package
		call  *ast.CallExpr
		fn    string
		targs []types.Type
		doc   string
	}
	var calls []found
	for _, p := range own {
		for _, f := range p.Syntax {
			stmtDocs := statementDocs(r.fset, f)
			ast.Inspect(f, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if fn, targs := veroCall(p.TypesInfo, call); fn != "" {
						calls = append(calls, found{p, call, fn, targs, stmtDocs[call.Pos()]})
					}
				}
				return true
			})
		}
	}
	for _, c := range calls {
		if c.fn != "NewState" {
			if name, ok := constantString(c.p.TypesInfo, c.call.Args[1]); ok {
				r.reserve(pascal(name) + "Request")
			}
		}
	}
	api := &API{}
	byName := map[string]*APIRequest{}
	for _, c := range calls {
		where := r.fset.Position(c.call.Pos())
		at := fmt.Sprintf("%s:%d", relTo(dir, where.Filename), where.Line)
		if c.fn == "NewState" {
			state := r.ref(c.targs[0], "State")
			if api.State != nil && !reflect.DeepEqual(api.State, state) {
				problems = append(problems, at+": a second vero.NewState, of another type")
			}
			api.State = state
			continue
		}
		name, ok := constantString(c.p.TypesInfo, c.call.Args[1])
		if !ok {
			problems = append(problems, at+": the request's name is not a constant, so vero gen cannot name it")
			continue
		}
		var req, rep types.Type
		switch c.fn {
		case "Handle":
			req, rep = c.targs[0], c.targs[1]
		case "UpdateWith":
			req, rep = c.targs[1], c.targs[0]
		case "Update":
			req, rep = types.NewStruct(nil, nil), c.targs[0]
		case "UpdateItem":
			// vero.ID[K]: the item's key, as {"id": key}.
			req = types.NewStruct([]*types.Var{types.NewField(token.NoPos, nil, "ID", c.targs[2], false)}, []string{`json:"id"`})
			rep = c.targs[0]
		}
		ar := &APIRequest{Name: name, Doc: c.doc, Where: at}
		if s, ok := under(req).(*types.Struct); ok {
			ar.Fields = r.fields(s, pascal(name)+"Request")
		} else {
			ar.Value = r.ref(req, pascal(name)+"Request")
		}
		ar.Reply = r.ref(rep, pascal(name)+"Reply")
		if old := byName[name]; old != nil {
			// The same handler registered twice, as calmtasks does for its
			// two ways of running the worker, is one request.
			if !sameRequest(old, ar) {
				problems = append(problems, fmt.Sprintf("%s: %q is handled again, with other types, at %s", at, name, old.Where))
			}
			continue
		}
		byName[name] = ar
		api.Requests = append(api.Requests, ar)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	sort.Slice(api.Requests, func(i, j int) bool { return api.Requests[i].Name < api.Requests[j].Name })
	for _, t := range r.order {
		api.Types = append(api.Types, t)
	}
	sort.Slice(api.Types, func(i, j int) bool { return api.Types[i].Name < api.Types[j].Name })
	api.Notes = r.renamed
	return api, nil
}

func sameRequest(a, b *APIRequest) bool {
	return reflect.DeepEqual(a.Fields, b.Fields) && reflect.DeepEqual(a.Value, b.Value) && reflect.DeepEqual(a.Reply, b.Reply)
}

func relTo(dir, path string) string {
	if rel, err := filepath.Rel(dir, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return path
}

// veroCall says which of vero's functions a call is, with its type
// arguments, or "".
func veroCall(info *types.Info, call *ast.CallExpr) (string, []types.Type) {
	fun := ast.Unparen(call.Fun)
	switch x := fun.(type) {
	case *ast.IndexExpr:
		fun = x.X
	case *ast.IndexListExpr:
		fun = x.X
	}
	var id *ast.Ident
	switch x := fun.(type) {
	case *ast.SelectorExpr:
		id = x.Sel
	case *ast.Ident:
		id = x
	default:
		return "", nil
	}
	obj, ok := info.Uses[id].(*types.Func)
	if !ok || obj.Pkg() == nil || obj.Pkg().Path() != veroPath {
		return "", nil
	}
	// w.EmitOnChange(ctx, interval, func() any { return snapshot }) pushes
	// whatever the function returns, which is the state.
	if obj.Name() == "EmitOnChange" && len(call.Args) == 3 {
		if lit, ok := call.Args[2].(*ast.FuncLit); ok {
			var state types.Type
			ast.Inspect(lit.Body, func(n ast.Node) bool {
				if _, ok := n.(*ast.FuncLit); ok {
					return false
				}
				if ret, ok := n.(*ast.ReturnStmt); ok && len(ret.Results) == 1 && state == nil {
					state = info.TypeOf(ret.Results[0])
				}
				return true
			})
			if state != nil {
				return "NewState", []types.Type{state}
			}
		}
		return "", nil
	}
	inst, ok := info.Instances[id]
	if !ok {
		return "", nil
	}
	var targs []types.Type
	for i := 0; i < inst.TypeArgs.Len(); i++ {
		targs = append(targs, inst.TypeArgs.At(i))
	}
	switch obj.Name() {
	case "Handle", "UpdateWith":
		if len(targs) == 2 && len(call.Args) == 3 {
			return obj.Name(), targs
		}
	case "Update":
		if len(targs) == 1 && len(call.Args) == 3 {
			return obj.Name(), targs
		}
	case "UpdateItem":
		if len(targs) == 3 && len(call.Args) == 3 {
			return obj.Name(), targs
		}
	case "NewState":
		if len(targs) == 1 {
			return obj.Name(), targs
		}
	}
	return "", nil
}

func constantString(info *types.Info, e ast.Expr) (string, bool) {
	tv, ok := info.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

// statementDocs are the comments just above each statement, by the
// position of the call that the statement starts with.
func statementDocs(fset *token.FileSet, f *ast.File) map[token.Pos]string {
	cmap := ast.NewCommentMap(fset, f, f.Comments)
	docs := map[token.Pos]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		stmt, ok := n.(*ast.ExprStmt)
		if !ok {
			return true
		}
		call, ok := stmt.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, g := range cmap[stmt] {
			if g.End() < stmt.Pos() {
				docs[call.Pos()] = strings.TrimSpace(g.Text())
			}
		}
		return true
	})
	return docs
}

// reader turns Go types into the API's, naming each type once.
type reader struct {
	fset  *token.FileSet
	docs  map[token.Pos]string // a field's or type's comment, by its name's position
	named map[*types.TypeName]*APIType
	names map[string]*types.TypeName // the Go type each name is given to
	order []*APIType
	// renamed says which types have another name in the front ends.
	renamed  []string
	lastName string
}

func (r *reader) collectDocs(f *ast.File) {
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GenDecl:
			if x.Tok == token.TYPE && len(x.Specs) == 1 && x.Doc != nil {
				r.docs[x.Specs[0].(*ast.TypeSpec).Name.Pos()] = strings.TrimSpace(x.Doc.Text())
			}
		case *ast.TypeSpec:
			if x.Doc != nil {
				r.docs[x.Name.Pos()] = strings.TrimSpace(x.Doc.Text())
			}
		case *ast.Field:
			g := x.Doc
			if g == nil {
				g = x.Comment
			}
			if g != nil {
				for _, name := range x.Names {
					r.docs[name.Pos()] = strings.TrimSpace(g.Text())
				}
			}
		}
		return true
	})
}

func under(t types.Type) types.Type { return types.Unalias(t).Underlying() }

// ref is a type as JSON carries it. hint names an anonymous struct.
func (r *reader) ref(t types.Type, hint string) *TypeRef {
	t = types.Unalias(t)
	if special := r.special(t); special != nil {
		return special
	}
	switch x := t.(type) {
	case *types.Pointer:
		return &TypeRef{Kind: "optional", Elem: r.ref(x.Elem(), hint)}
	case *types.Named:
		if s, ok := x.Underlying().(*types.Struct); ok {
			if s.NumFields() == 0 {
				return &TypeRef{Kind: "empty"}
			}
			return &TypeRef{Kind: "named", Name: r.namedStruct(x, s)}
		}
		if b, ok := x.Underlying().(*types.Basic); ok && x.Obj().Pkg() != nil {
			return &TypeRef{Kind: "named", Name: r.namedScalar(x, b)}
		}
		return r.ref(x.Underlying(), hint)
	case *types.Basic:
		return basicRef(x)
	case *types.Slice:
		if b, ok := under(x.Elem()).(*types.Basic); ok && b.Kind() == types.Byte {
			return &TypeRef{Kind: "string"} // base64
		}
		return &TypeRef{Kind: "list", Elem: r.ref(x.Elem(), hint+"Item")}
	case *types.Array:
		return &TypeRef{Kind: "list", Elem: r.ref(x.Elem(), hint+"Item")}
	case *types.Map:
		return &TypeRef{Kind: "map", Elem: r.ref(x.Elem(), hint+"Value")}
	case *types.Struct:
		if x.NumFields() == 0 {
			return &TypeRef{Kind: "empty"}
		}
		return &TypeRef{Kind: "named", Name: r.anonymous(x, hint)}
	case *types.Interface:
		return &TypeRef{Kind: "json"}
	}
	return &TypeRef{Kind: "json"}
}

// special are the types that encode themselves: time.Time as a string,
// json.RawMessage and anything with its own MarshalJSON as any value,
// and anything with MarshalText as a string.
func (r *reader) special(t types.Type) *TypeRef {
	if n, ok := t.(*types.Named); ok && n.Obj().Pkg() != nil {
		switch n.Obj().Pkg().Path() + "." + n.Obj().Name() {
		case "time.Time":
			return &TypeRef{Kind: "string", Time: true}
		case "time.Duration":
			return &TypeRef{Kind: "int"}
		case "encoding/json.RawMessage", "encoding/json.Number":
			return &TypeRef{Kind: "json"}
		}
	}
	if hasMethod(t, "MarshalJSON") {
		return &TypeRef{Kind: "json"}
	}
	if hasMethod(t, "MarshalText") {
		return &TypeRef{Kind: "string"}
	}
	return nil
}

func hasMethod(t types.Type, name string) bool {
	if _, ok := t.(*types.Pointer); ok {
		return false
	}
	for _, tt := range []types.Type{t, types.NewPointer(t)} {
		ms := types.NewMethodSet(tt)
		for i := 0; i < ms.Len(); i++ {
			if ms.At(i).Obj().Name() == name {
				return true
			}
		}
	}
	return false
}

func basicRef(b *types.Basic) *TypeRef {
	switch {
	case b.Info()&types.IsBoolean != 0:
		return &TypeRef{Kind: "bool"}
	case b.Info()&types.IsInteger != 0:
		return &TypeRef{Kind: "int"}
	case b.Info()&types.IsFloat != 0:
		return &TypeRef{Kind: "float"}
	case b.Info()&types.IsString != 0:
		return &TypeRef{Kind: "string"}
	}
	return &TypeRef{Kind: "json"}
}

// takenNames are names that a front end already uses, in Swift, SwiftUI,
// C# or Python: a type of the worker's called Task would hide Swift's Task
// throughout the app, so it is called WorkerTask in every front end.
var takenNames = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`
		Any Array Bool Character Data Date Decimal Dictionary Double Error Float Int Int64 Never Optional
		Range Result Set String Substring Task Timer Type URL UUID Void Calendar Locale Notification
		Bundle Process Thread Operation Measurement Duration Progress Predicate
		App Scene Window WindowGroup Settings View Text Image Button Label List Section Form Group Menu
		Link Toggle Picker Color Font Path Shape Table Grid Stack Spacer Divider Alert Binding State
		Environment Animation Transaction Gesture Commands Tab Toolbar Sheet
		JSONValue EmptyReply NamedRequest Vero VeroClient VeroModel VeroError Worker WorkerApi
		Object Exception Action Func Delegate Attribute Enum Math Convert Console Environment Guid
		TimeSpan DateTime DateTimeOffset Dictionary HashSet Queue Stack JsonElement JsonSerializer
		Window Application Control Brush Brushes Colors Thickness Visibility Point Size Rect
		Callable Optional Union Literal Protocol TypeVar Generic dataclass field`) {
		takenNames[n] = true
	}
}

// name gives a Go type a name in the front ends: its own, with Worker in
// front when a front end uses that name already, and with its package's
// in front when another type has it.
func (r *reader) name(obj *types.TypeName, want string) string {
	want = pascal(want)
	own := want
	if takenNames[want] {
		want = "Worker" + want
		defer func() {
			goName := own
			if obj.Pkg() != nil {
				goName = obj.Pkg().Name() + "." + own
			}
			r.renamed = append(r.renamed, fmt.Sprintf("the worker's %s is %s in the front ends, since they use the name %s already", goName, r.lastName, own))
		}()
	}
	if other, taken := r.names[want]; taken && other != obj {
		if obj.Pkg() != nil {
			want = pascal(obj.Pkg().Name()) + want
		}
		for i := 2; r.names[want] != nil && r.names[want] != obj; i++ {
			want = fmt.Sprintf("%s%d", strings.TrimRight(want, "0123456789"), i)
		}
	}
	r.names[want] = obj
	r.lastName = want
	return want
}

// reserve keeps a name for a type that vero gen makes itself.
func (r *reader) reserve(name string) {
	r.names[name] = types.NewTypeName(token.NoPos, nil, name, nil)
}

func (r *reader) namedStruct(n *types.Named, s *types.Struct) string {
	obj := n.Obj()
	if t, ok := r.named[obj]; ok {
		return t.Name
	}
	t := &APIType{Name: r.name(obj, obj.Name()), Doc: r.docs[obj.Pos()]}
	r.named[obj] = t
	r.order = append(r.order, t)
	t.Fields = r.fields(s, t.Name)
	return t.Name
}

func (r *reader) namedScalar(n *types.Named, b *types.Basic) string {
	obj := n.Obj()
	if t, ok := r.named[obj]; ok {
		return t.Name
	}
	t := &APIType{Name: r.name(obj, obj.Name()), Doc: r.docs[obj.Pos()], Alias: basicRef(b)}
	r.named[obj] = t
	r.order = append(r.order, t)
	return t.Name
}

// anonymous names a struct with no name of its own after where it is used.
func (r *reader) anonymous(s *types.Struct, hint string) string {
	obj := types.NewTypeName(token.NoPos, nil, hint, s)
	// The same anonymous struct, used twice, is one type.
	for o, t := range r.named {
		if o.Pkg() == nil && types.Identical(o.Type(), s) {
			return t.Name
		}
	}
	t := &APIType{Name: r.name(obj, hint)}
	r.named[obj] = t
	r.order = append(r.order, t)
	t.Fields = r.fields(s, t.Name)
	return t.Name
}

// fields are a struct's fields as encoding/json writes them: exported
// ones, by their tag's name, with embedded structs' fields brought up.
func (r *reader) fields(s *types.Struct, owner string) []*APIField {
	var out []*APIField
	seen := map[string]bool{}
	var walk func(s *types.Struct)
	walk = func(s *types.Struct) {
		for i := 0; i < s.NumFields(); i++ {
			f := s.Field(i)
			tag := reflect.StructTag(s.Tag(i)).Get("json")
			name, opts, _ := strings.Cut(tag, ",")
			if name == "-" && opts == "" {
				continue
			}
			if f.Embedded() && name == "" {
				t := types.Unalias(f.Type())
				if p, ok := t.(*types.Pointer); ok {
					t = p.Elem()
				}
				if es, ok := under(t).(*types.Struct); ok {
					walk(es)
					continue
				}
			}
			if !f.Exported() {
				continue
			}
			if name == "" {
				name = f.Name()
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			ref := r.ref(f.Type(), owner+pascal(f.Name()))
			if ref.Kind == "empty" {
				ref = &TypeRef{Kind: "json"}
			}
			if strings.Contains(","+opts+",", ",string,") && (ref.Kind == "int" || ref.Kind == "float" || ref.Kind == "bool") {
				ref = &TypeRef{Kind: "string"}
			}
			out = append(out, &APIField{JSON: name, Doc: r.docs[f.Pos()], Type: ref})
		}
	}
	walk(s)
	return out
}

// lowerCamel is pascal with its first word small: "SignIn" is "signIn",
// and "URLPath" is "urlPath".
func lowerCamel(s string) string {
	p := []rune(pascal(s))
	for i := 0; i < len(p); i++ {
		if i > 0 && i+1 < len(p) && unicode.IsLower(p[i+1]) {
			break
		}
		p[i] = unicode.ToLower(p[i])
	}
	return string(p)
}

// snake is a name in Python's style: "signIn" is "sign_in".
func snake(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i, c := range rs {
		if c == '-' || c == '.' || c == ' ' {
			b.WriteRune('_')
			continue
		}
		if unicode.IsUpper(c) {
			if i > 0 && (unicode.IsLower(rs[i-1]) || (i+1 < len(rs) && unicode.IsLower(rs[i+1]) && unicode.IsUpper(rs[i-1]))) {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToLower(c))
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

// docLines is a comment as lines, each with a prefix, or nothing.
func docLines(doc, prefix string) string {
	if doc == "" {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(doc, "\n") {
		b.WriteString(strings.TrimRight(prefix+line, " ") + "\n")
	}
	return b.String()
}
