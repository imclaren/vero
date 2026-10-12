package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// genAPI reads testdata/gen's worker, which has one of each kind of
// request and type.
func genAPI(t *testing.T) *API {
	t.Helper()
	api, err := readAPI("testdata/gen", "cmd/worker")
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func TestGenReadsWorker(t *testing.T) {
	api := genAPI(t)
	var names []string
	byName := map[string]*APIRequest{}
	for _, r := range api.Requests {
		names = append(names, r.Name)
		byName[r.Name] = r
	}
	if want := []string{"add", "count", "owner", "pause", "ping", "raw", "rename", "setSpeed"}; !slices.Equal(names, want) {
		t.Fatalf("requests %v, want %v", names, want)
	}
	if api.State == nil || api.State.Name != "Status" {
		t.Fatalf("state %+v, want Status", api.State)
	}
	types := map[string]*APIType{}
	for _, ty := range api.Types {
		types[ty.Name] = ty
	}
	var keys []string
	for _, f := range types["Status"].Fields {
		keys = append(keys, f.JSON+":"+refString(f.Type))
	}
	// changed comes up from the embedded Common; Secret ("-") and hidden
	// (unexported) are left out; score is a string by its tag; data is
	// base64; task is renamed.
	want := []string{"changed:string", "jobs:list(Job)", "speed:Speed", "labels:map(int)", "extra:json", "owner:optional(Owner)",
		"score:string", "tags:list(list(string))", "data:string", "any:json", "task:WorkerTask"}
	if !slices.Equal(keys, want) {
		t.Errorf("Status's fields\n %v\nwant\n %v", keys, want)
	}
	if types["Speed"] == nil || types["Speed"].Alias == nil || types["Speed"].Doc != "Speed is how fast to go." {
		t.Errorf("Speed is %+v, want another name for a string, with its comment", types["Speed"])
	}
	if len(api.Notes) != 1 || !strings.Contains(api.Notes[0], "main.Task is WorkerTask") {
		t.Errorf("notes %q, want one saying that Task is WorkerTask", api.Notes)
	}
	for name, want := range map[string]string{
		"rename": "id:int title:string note:RenameRequestNote -> Job",
		"ping":   " -> empty",
		"raw":    "value list(string) -> json",
		"owner":  " -> optional(Owner)",
		"count":  " -> int",
		"add":    " -> Status",
		"pause":  "id:int -> Status",
	} {
		r := byName[name]
		var got []string
		for _, f := range r.Fields {
			got = append(got, f.JSON+":"+refString(f.Type))
		}
		s := strings.Join(got, " ")
		if r.Value != nil {
			s = "value " + refString(r.Value)
		}
		if s += " -> " + refString(r.Reply); s != want {
			t.Errorf("%s is %q, want %q", name, s, want)
		}
	}
	if byName["rename"].Doc != "rename renames a job." {
		t.Errorf("rename's comment is %q", byName["rename"].Doc)
	}
}

func refString(t *TypeRef) string {
	switch t.Kind {
	case "named":
		return t.Name
	case "list", "map", "optional":
		return t.Kind + "(" + refString(t.Elem) + ")"
	}
	return t.Kind
}

// TestGenSwift builds the Swift with vero's Swift package, and decodes a
// state from an older worker that lacks fields and has nulls for lists.
func TestGenSwift(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	if _, err := exec.LookPath("swift"); err != nil {
		t.Skip("no swift")
	}
	root, _ := filepath.Abs("../..")
	dir := t.TempDir()
	srcs := filepath.Join(dir, "Sources", "Check")
	os.MkdirAll(srcs, 0o755)
	os.WriteFile(filepath.Join(dir, "Package.swift"), []byte(`// swift-tools-version:5.9
import PackageDescription
let package = Package(name: "Check", platforms: [.macOS(.v13)],
    dependencies: [.package(path: "`+root+`")],
    targets: [.executableTarget(name: "Check", dependencies: [.product(name: "Vero", package: "vero")])])
`), 0o644)
	os.WriteFile(filepath.Join(srcs, "WorkerAPI.swift"), []byte(swiftAPI(genAPI(t))), 0o644)
	os.WriteFile(filepath.Join(srcs, "main.swift"), []byte(`import Foundation
let old = #"{"jobs": null, "labels": null, "speed": "fast", "task": {}}"#
let s = try JSONDecoder().decode(Status.self, from: Data(old.utf8))
precondition(s.jobs == [] && s.labels == [:] && s.speed == "fast" && s.owner == nil && s.extra == .null)
// Go's time.Time is a Date: missing, it is Go's zero time, and it is read
// from RFC 3339 with nine fractional digits and written back as Go reads it.
precondition(s.changed == .distantPast)
let when = #"{"changed": "2026-10-07T05:35:02.123456789Z"}"#
let t = try JSONDecoder().decode(Status.self, from: Data(when.utf8))
precondition(abs(t.changed.timeIntervalSince1970 - 1791351302.123) < 0.001)
let back = String(data: try JSONEncoder().encode(t), encoding: .utf8)!
precondition(back.contains(#""changed":"2026-10-07T05:35:02.123Z""#), back)
let r = RenameRequest(id: 3, title: "new")
let sent = String(data: try JSONEncoder().encode(r), encoding: .utf8)!
precondition(sent.contains("\"id\":3") && RenameRequest.name == "rename")
print("ok")
`), 0o644)
	cmd := exec.Command("swift", "run", "-q", "Check")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("%v\n%s", err, out)
	}
}

// TestGenPython runs the Python against a stand-in worker.
func TestGenPython(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("no python3")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "worker_api.py"), []byte(pythonAPI(genAPI(t))), 0o644)
	check := `import sys
sys.path.insert(0, sys.argv[1])
import worker_api as w

class Fake:
    def __init__(self):
        self.sent = []
    def call(self, name, payload):
        self.sent.append((name, payload))
        return {"id": 3, "title": "new"} if name == "rename" else None
    def latest(self):
        return {"jobs": None, "labels": None, "speed": "fast", "task": {}}

f = Fake()
api = w.Worker(f)
s = api.state()
assert s.jobs == [] and s.labels == {} and s.speed == "fast" and s.owner is None, s
job = api.rename(id=3, title="new")
assert job == w.Job(id=3, title="new"), job
assert f.sent[-1] == ("rename", {"id": 3, "title": "new", "note": {"text": ""}}), f.sent
api.raw(["a"])
assert f.sent[-1] == ("raw", ["a"])
assert api.owner() is None and api.ping() is None and api.count() == 0
api.pause(id=2)
assert f.sent[-1] == ("pause", {"id": 2})
assert w.Status.from_json(s.to_json()) == s
print("ok")
`
	os.WriteFile(filepath.Join(dir, "check.py"), []byte(check), 0o644)
	if out, err := exec.Command("python3", "-I", filepath.Join(dir, "check.py"), dir).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// TestGenCSharp builds the C# with vero's Vero.cs, which takes a while.
func TestGenCSharp(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	if _, err := exec.LookPath("dotnet"); err != nil {
		t.Skip("no dotnet")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "WorkerApi.cs"), []byte(csharpAPI(genAPI(t), "Genapp")), 0o644)
	vero, _ := os.ReadFile("../../bindings/csharp/Vero.cs")
	os.WriteFile(filepath.Join(dir, "Vero.cs"), vero, 0o644)
	os.WriteFile(filepath.Join(dir, "Check.csproj"), []byte(`<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net10.0</TargetFramework><Nullable>enable</Nullable><ImplicitUsings>enable</ImplicitUsings></PropertyGroup>
</Project>`), 0o644)
	os.WriteFile(filepath.Join(dir, "Program.cs"), []byte(`using System.Text.Json;
var s = Genapp.WorkerApi.ReadState(JsonDocument.Parse("{\"jobs\": null, \"labels\": null, \"speed\": \"fast\", \"task\": {}}").RootElement);
if (s.Jobs.Count != 0 || s.Labels.Count != 0 || s.Speed != "fast" || s.Owner != null) throw new Exception("decoded wrongly");
var sent = JsonSerializer.Serialize(new Genapp.RenameRequest { Id = 3, Title = "new" });
if (!sent.Contains("\"id\":3")) throw new Exception(sent);
Console.WriteLine("ok");
`), 0o644)
	cmd := exec.Command("dotnet", "run", "-v", "q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("%v\n%s", err, out)
	}
}
