package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestChoose(t *testing.T) {
	for _, c := range []struct {
		names  []string
		except string
		want   string
	}{
		{nil, "", "macos gtk wpf"},
		{[]string{"mobile"}, "", "android ios"},
		{[]string{"all"}, "plan9,wasi", "macos gtk wpf android ios web"},
		{[]string{"windows", "linux", "desktop"}, "", "macos gtk wpf"},
	} {
		got, err := choose(c.names, c.except)
		if err != nil || strings.Join(got, " ") != c.want {
			t.Errorf("choose(%v, %q) = %v, %v; want %s", c.names, c.except, got, err, c.want)
		}
	}
	if _, err := choose([]string{"amiga"}, ""); err == nil {
		t.Error("an unknown front end was accepted")
	}
}

// TestAdd runs vero add on a copy of vero's example worker, and checks
// what it reads of the worker and what it writes.
func TestAdd(t *testing.T) {
	root, _ := filepath.Abs("../..")
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "worker"), 0o755)
	src, _ := os.ReadFile(filepath.Join(root, "example", "worker", "main.go"))
	os.WriteFile(filepath.Join(dir, "worker", "main.go"), src, 0o644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/jobs\n\ngo 1.22\n\nrequire github.com/imclaren/vero v0.0.0\n\nreplace github.com/imclaren/vero => "+root+"\n"), 0o644)
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}

	app, err := analyse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if app.Name != "jobs" || app.Worker != "worker" || app.WorkerName != "jobs-worker" || app.State.Name != "Status" {
		t.Errorf("analysed: %+v", app)
	}
	if len(app.Requests) != 2 || app.Requests[1].Name != "restartJob" || len(app.Requests[1].Fields) != 1 || app.Requests[1].Fields[0].JSON != "id" || app.Requests[1].Fields[0].Kind != "int" {
		t.Errorf("requests: %+v", app.Requests)
	}
	if job := app.Types["Job"]; job == nil || len(job.Fields) != 4 || job.Fields[3].JSON != "progress" {
		t.Errorf("types: %+v", app.Types)
	}

	added, err := add(app, []string{"gtk", "wpf", "macos", "web"}, false)
	if err != nil || strings.Join(added, " ") != "gtk wpf macos web" {
		t.Fatalf("add: %v, %v", added, err)
	}
	for _, f := range []string{"gtk/main.py", "gtk/vero.py", "windows/Jobs.csproj", "windows/Vero.cs", "macos/Sources/Jobs/App.swift", "web/main.go", "PORTING.md", "vero-app.toml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("not written: %s", f)
		}
	}
	toml, _ := os.ReadFile(filepath.Join(dir, "vero-app.toml"))
	for _, want := range []string{"name = \"jobs\"", "[worker]", "[gtk]", "[wpf]", "[macos]", "[web]"} {
		if !strings.Contains(string(toml), want) {
			t.Errorf("vero-app.toml lacks %s", want)
		}
	}
	py, _ := os.ReadFile(filepath.Join(dir, "gtk", "main.py"))
	if !strings.Contains(string(py), `("restartJob", [("id", "int"), ]),`) {
		t.Error("the GTK starter does not offer restartJob with its id")
	}
	porting, _ := os.ReadFile(filepath.Join(dir, "PORTING.md"))
	if !strings.Contains(string(porting), "`restartJob`") || !strings.Contains(string(porting), "Serve(in io.Reader") {
		t.Errorf("PORTING.md:\n%s", porting)
	}
	// Asked again, nothing is added: each is there already.
	if added, err := add(app, []string{"gtk"}, false); err != nil || len(added) != 0 {
		t.Errorf("added again: %v, %v", added, err)
	}
	// The front ends written build, where this Mac can build them: the
	// Python compiles, and the page's wasm builds.
	py3 := exec.Command("python3", "-m", "py_compile", "gtk/main.py")
	py3.Dir = dir
	if out, err := py3.CombinedOutput(); err != nil {
		t.Errorf("gtk/main.py: %v\n%s", err, out)
	}
	wasm := exec.Command("go", "build", "-o", os.DevNull, "./web")
	wasm.Dir = dir
	wasm.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if out, err := wasm.CombinedOutput(); err != nil {
		t.Errorf("web: %v\n%s", err, out)
	}
}

// TestScan: what PORTING.md flags, from a worker that does those things.
func TestScan(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "keys.go"), []byte("package main\n\nimport \"C\"\nimport \"os/exec\"\n\nfunc f() { exec.Command(\"ffmpeg\") }\n"), 0o644)
	app := &App{Dir: dir}
	app.scan()
	var titles []string
	for _, n := range app.Notes {
		titles = append(titles, n.Title)
	}
	if strings.Join(titles, ", ") != "cgo, programs the worker runs" {
		t.Errorf("notes: %v", titles)
	}
}

// TestStateVariable: a worker that passes its state as a variable, keeping
// it in a vero.State[Status] field, as audiobooks' does.
func TestStateVariable(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "cmd", "worker"), 0o755)
	os.WriteFile(filepath.Join(dir, "cmd", "worker", "main.go"), []byte(`package main

import "github.com/imclaren/vero"

type Status struct {
	Books int `+"`json:\"books\"`"+`
}

type app struct{ state *vero.State[Status] }

func main() {
	w := vero.NewWorker(vero.WorkerOptions{})
	status := Status{}
	a := app{state: vero.NewState(w, status)}
	vero.Update(a.state, "sync", func(*Status) error { return nil })
}
`), 0o644)
	app, err := analyse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(app.State.Fields) != 1 || app.State.Fields[0].JSON != "books" || len(app.Requests) != 1 {
		t.Errorf("state %+v, requests %+v", app.State, app.Requests)
	}
}

// TestDescribe checks that vero add asks for the words vero-app.toml needs
// only at a terminal, only when writing the file, and only for what the
// flags left blank - and that what it is told lands in the file, quoted.
func TestDescribe(t *testing.T) {
	in := strings.NewReader("A small app\nIt says \"hello\".\nAnn <ann@example.com>\n")
	var out strings.Builder
	got := describe(Description{}, true, in, &out, true)
	if got.Summary != "A small app" || got.Text != `It says "hello".` || got.Publisher != "Ann <ann@example.com>" {
		t.Errorf("asked and got %+v", got)
	}
	if !strings.Contains(out.String(), "summary") {
		t.Errorf("no prompt was shown:\n%s", out.String())
	}

	// Not at a terminal: nothing is read, nothing is asked.
	out.Reset()
	in = strings.NewReader("should not be read\n")
	got = describe(Description{Summary: "from a flag"}, true, in, &out, false)
	if got.Summary != "from a flag" || got.Text != "" || out.Len() != 0 || in.Len() == 0 {
		t.Errorf("off a terminal: %+v, prompt %q", got, out.String())
	}

	// The file exists already: its words are its own, so nothing is asked.
	out.Reset()
	got = describe(Description{}, false, strings.NewReader("x\n"), &out, true)
	if got.Summary != "" || out.Len() != 0 {
		t.Errorf("with a file already: %+v, prompt %q", got, out.String())
	}

	// What was said is what the file gets, with quotes escaped.
	dir := t.TempDir()
	f := readToml(filepath.Join(dir, "vero-app.toml"))
	app := &App{Name: "hello", Display: "Hello", ID: "com.example.hello", Worker: "cmd/worker", WorkerName: "hello-worker",
		Description: Description{Summary: "A small app", Text: `It says "hello".`, Publisher: "Ann <ann@example.com>"}}
	if err := f.write(app, "[gtk]\nfolder = \"gtk\"\n"); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(f.path)
	for _, want := range []string{`summary = "A small app"`, `description = "It says \"hello\"."`, `publisher = "Ann <ann@example.com>"`} {
		if !strings.Contains(string(text), want) {
			t.Errorf("vero-app.toml lacks %s:\n%s", want, text)
		}
	}
	// And blanks stay blank, with the placeholder publisher that vero-repo
	// accepts until it is changed.
	f = readToml(filepath.Join(dir, "other.toml"))
	app.Description = Description{}
	f.write(app, "")
	text, _ = os.ReadFile(f.path)
	if !strings.Contains(string(text), `summary = ""`) || !strings.Contains(string(text), `publisher = "Your Name <you@example.com>"`) {
		t.Errorf("blank description:\n%s", text)
	}
}
