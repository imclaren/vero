package main

// The desktop starters: GTK for Linux, the BSDs and illumos; WPF for
// Windows; SwiftUI for macOS. Each shows the state the worker pushes as a
// tree of its fields, and a control for each request, and is the point to
// start a window of your own from.

var starters = map[string]*starter{}

func init() {
	starters["gtk"] = &starter{folder: "gtk", note: "a GTK 4 window in Python, for Linux, the BSDs and illumos; needs python3-gi and GTK 4", files: map[string]string{
		"README.md": gtkReadme, "main.py": gtkMain, "build.sh": gtkBuild, ".gitignore": gtkIgnore}, bindings: []string{"bindings/python/vero.py"}, toml: gtkToml}
	starters["wpf"] = &starter{folder: "windows", note: "a WPF window in C#, for Windows; needs the .NET SDK (dotnet)", files: map[string]string{
		"README.md": wpfReadme, "App.xaml": wpfAppXaml, "App.xaml.cs": wpfAppCs, "MainWindow.xaml": wpfWindowXaml, "MainWindow.xaml.cs": wpfWindowCs,
		"{{pascal .Name}}.csproj": wpfProj, ".gitignore": wpfIgnore}, bindings: []string{"bindings/csharp/Vero.cs"}, toml: wpfToml}
	starters["macos"] = &starter{folder: "macos", note: "a SwiftUI menu bar app, for macOS; needs Xcode's command line tools", files: map[string]string{
		"README.md": macReadme, "Package.swift": macPackage, "Sources/{{pascal .Name}}/App.swift": macApp, "build.sh": macBuild, ".gitignore": macIgnore}, toml: macToml}
}

// What each starter's build makes, which doesn't belong in git: the worker
// built beside the front end, and the toolkit's own output.
const gtkIgnore = `{{.WorkerName}}
__pycache__/
*.pyc
`

const wpfIgnore = `bin/
obj/
{{.WorkerName}}.exe
`

const macIgnore = `.build/
.swiftpm/
{{.WorkerName}}
`

const buildIgnore = `.build/
`

const webIgnore = `main.wasm
wasm_exec.js
`

const wasiIgnore = `worker.wasm
{{.Name}}-wasi
`

const plan9Ignore = `{{.WorkerName}}
{{.Name}}-plan9
`

const gtkToml = `# The GTK front end, which vero add wrote: Linux, the BSDs and illumos.
[gtk]
folder = "gtk"
entry = "main.py"
categories = "Utility;"

[gtk.deb]
depends = "python3 (>= 3.10), python3-gi, gir1.2-gtk-4.0"
section = "utils"

[gtk.rpm]
requires = "python3 >= 3.10, python3-gobject, (typelib(Gtk) = 4.0 if openSUSE-release else (gtk4 and gobject-introspection))"

[gtk.arch]
depends = "python, python-gobject, gtk4"

[gtk.alpine]
depends = "python3, py3-gobject3, gtk4.0"

[gtk.void]
depends = "python3, python3-gobject, gtk4"
`

const gtkReadme = `# {{.Display}} for Linux, the BSDs and illumos

A GTK 4 front end in Python, written by ` + "`vero add`" + ` for the worker in
` + "`../{{.Worker}}`" + `. It shows the state the worker pushes and offers each of its
requests; make it your own from there.

    ./build.sh      builds the worker beside this file
    ./main.py       runs it

It needs Python 3.10 or later with PyGObject and GTK 4: on Debian and Ubuntu,
` + "`python3-gi gir1.2-gtk-4.0`" + `. On a Mac, ` + "`build.sh`" + ` installs them with Homebrew and
says which Python to run it with. To see it on a Linux from a Mac instead,
` + "`path/to/vero/scripts/run-linux.sh --app gtk --entry main.py --worker ./{{.Worker}} --worker-name {{.WorkerName}}`" + `.
`

const gtkBuild = `#!/bin/sh
# Builds the worker beside main.py. On a Mac it also installs GTK 4 and
# PyGObject with Homebrew if they are missing, so that main.py runs here
# as it would on Linux.
set -e
cd "$(dirname "$0")"
go build -o {{.WorkerName}} ../{{.Worker}}

PYTHON=python3
if [ "$(uname)" = Darwin ] && command -v brew >/dev/null 2>&1; then
    # Homebrew's PyGObject is built for Homebrew's Python, not Xcode's.
    PYTHON="$(brew --prefix)/bin/python3"
    if ! "$PYTHON" -c 'import gi; gi.require_version("Gtk", "4.0"); from gi.repository import Gtk' >/dev/null 2>&1; then
        echo "installing GTK 4 and PyGObject with Homebrew (once)"
        brew install pygobject3 gtk4
    fi
fi
echo "built: $PYTHON main.py runs it"
`

const gtkMain = `#!/usr/bin/env python3
"""{{.Display}}: a GTK 4 front end for the worker in ../{{.Worker}}.

Written by vero add. It shows the worker's state - every field of
{{.State.Name}} - and a control for each request{{if .Requests}} ({{range $i, $r := .Requests}}{{if $i}}, {{end}}{{$r.Name}}{{end}}){{end}}.
Start from here and make it your own.
"""

import json
import os
import sys

import gi

gi.require_version("Gtk", "4.0")
from gi.repository import GLib, Gtk  # noqa: E402

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from vero import NotRunning, Vero, VeroError, run_in_thread  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))

# The requests the worker handles, and what each takes: name, then each
# field's JSON key and kind.
REQUESTS = [
{{- range .Requests}}
    ("{{.Name}}", [{{range .Fields}}("{{.JSON}}", "{{.Kind}}"), {{end}}]),
{{- end}}
]


class Window(Gtk.ApplicationWindow):
    def __init__(self, app: Gtk.Application, vero: Vero) -> None:
        super().__init__(application=app, title="{{.Display}}")
        self.vero = vero
        self.set_default_size(460, -1)
        box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=12,
                      margin_top=16, margin_bottom=16, margin_start=16, margin_end=16)
        self.set_child(box)

        # The state, as a tree of its fields.
        self.tree = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=4)
        box.append(self.tree)

        # One row per request: entries for what it takes, and a button.
        self.entries: dict[str, list[tuple[str, str, Gtk.Entry]]] = {}
        for name, fields in REQUESTS:
            row = Gtk.Box(spacing=6)
            button = Gtk.Button(label=name)
            button.connect("clicked", lambda _b, n=name: self.send(n))
            row.append(button)
            self.entries[name] = []
            for key, kind in fields:
                entry = Gtk.Entry(placeholder_text=f"{key} ({kind})", hexpand=True)
                row.append(entry)
                self.entries[name].append((key, kind, entry))
            box.append(row)

        self.problem = Gtk.Label(xalign=0, wrap=True)
        self.problem.add_css_class("dim-label")
        box.append(self.problem)

        try:
            if (status := self.vero.latest()):
                self.apply(status)
        except NotRunning:
            pass
        run_in_thread(self.vero, lambda s: GLib.idle_add(self.apply, s))

    def apply(self, status: dict) -> bool:
        """Redraws the state as rows: scalars as "key: value", lists and
        nested objects indented under their key."""
        while child := self.tree.get_first_child():
            self.tree.remove(child)
        self.rows(status, 0)
        return False

    def rows(self, value, depth: int) -> None:
        if isinstance(value, dict):
            for key, v in value.items():
                if isinstance(v, (dict, list)):
                    self.row(f"{key}:", depth, bold=True)
                    self.rows(v, depth + 1)
                else:
                    self.row(f"{key}: {v}", depth)
        elif isinstance(value, list):
            for i, v in enumerate(value):
                if isinstance(v, dict):
                    self.row(", ".join(f"{k}: {x}" for k, x in v.items() if not isinstance(x, (dict, list))), depth)
                    self.rows({k: x for k, x in v.items() if isinstance(x, (dict, list))}, depth + 1)
                else:
                    self.row(str(v), depth)

    def row(self, text: str, depth: int, bold: bool = False) -> None:
        label = Gtk.Label(label=text, xalign=0, margin_start=depth * 16)
        if bold:
            label.add_css_class("heading")
        self.tree.append(label)

    def send(self, name: str) -> None:
        """Sends a request with what its entries hold, and draws the reply."""
        data = {}
        for key, kind, entry in self.entries[name]:
            text = entry.get_text()
            if kind == "int":
                data[key] = int(text or 0)
            elif kind == "float":
                data[key] = float(text or 0)
            elif kind == "bool":
                data[key] = text.lower() in ("1", "true", "yes")
            else:
                data[key] = text
        try:
            reply = self.vero.call(name, data)
            self.problem.set_text("")
            if isinstance(reply, dict):
                self.apply(reply)
        except VeroError as e:
            self.problem.set_text(str(e))


class Application(Gtk.Application):
    def __init__(self) -> None:
        super().__init__(application_id="{{.ID}}")
        self.vero: Vero | None = None

    def do_activate(self) -> None:
        if self.vero is None:
            self.vero = Vero(os.path.join(HERE, "{{.WorkerName}}"))
        Window(self, self.vero).present()

    def do_shutdown(self) -> None:
        if self.vero:
            self.vero.stop()
        Gtk.Application.do_shutdown(self)


if __name__ == "__main__":
    sys.exit(Application().run(sys.argv))
`

const wpfToml = `# The Windows front end, which vero add wrote.
[wpf]
folder = "windows"
exe = "{{pascal .Name}}.exe"
`

const wpfReadme = `# {{.Display}} for Windows

A WPF front end in C#, written by ` + "`vero add`" + ` for the worker in ` + "`../{{.Worker}}`" + `.
It shows the state the worker pushes and offers each of its requests; make it
your own from there.

    GOOS=windows go build -o {{.WorkerName}}.exe ../{{.Worker}}   # the worker, beside the app
    dotnet build                                               # on Windows, or on a Mac to check it compiles

` + "`Vero.cs`" + ` is vero's C# binding, copied here by vero add (copy it again from
vero's bindings/csharp when you update vero). The packaging in
vero-app.toml builds the app and puts the worker beside it.
`

const wpfProj = `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <OutputType>WinExe</OutputType>
    <TargetFramework>net8.0-windows</TargetFramework>
    <UseWPF>true</UseWPF>
    <Nullable>enable</Nullable>
    <RootNamespace>{{pascal .Name}}</RootNamespace>
    <AssemblyName>{{pascal .Name}}</AssemblyName>
    <EnableWindowsTargeting>true</EnableWindowsTargeting>
  </PropertyGroup>
</Project>
`

const wpfAppXaml = `<Application x:Class="{{pascal .Name}}.App"
             xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
             xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
             StartupUri="MainWindow.xaml"/>
`

const wpfAppCs = `using System.Windows;

namespace {{pascal .Name}};

public partial class App : Application { }
`

const wpfWindowXaml = `<Window x:Class="{{pascal .Name}}.MainWindow"
        xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
        Title="{{.Display}}" Width="480" SizeToContent="Height">
    <!-- Written by vero add: the worker's state as rows, and a control for
         each request. Make it your own. -->
    <StackPanel Margin="16">
        <StackPanel x:Name="Tree"/>
        <StackPanel x:Name="Requests" Margin="0,12,0,0"/>
        <TextBlock x:Name="Problem" Foreground="Gray" TextWrapping="Wrap" Margin="0,8,0,0"/>
    </StackPanel>
</Window>
`

const wpfWindowCs = `using System;
using System.IO;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Windows;
using System.Windows.Controls;
using Vero;

namespace {{pascal .Name}};

// The shapes the worker pushes and takes, as in ../{{.Worker}}: for a window
// of your own, bind to these instead of the generic rows below.
{{range $name, $s := .Types}}public record {{$s.Name}}({{range $i, $f := $s.Fields}}{{if $i}}, {{end}}[property: JsonPropertyName("{{$f.JSON}}")] {{csType $f}} {{pascal $f.Name}}{{end}});
{{end}}
public partial class MainWindow : Window
{
    private readonly VeroClient _vero;
    // The requests the worker handles, and the entries for what each takes.
    private static readonly (string name, (string key, string kind)[] fields)[] _requests =
    {
{{- range .Requests}}
        ("{{.Name}}", new (string key, string kind)[] { {{range .Fields}}("{{.JSON}}", "{{.Kind}}"), {{end}} }),
{{- end}}
    };
    private readonly System.Collections.Generic.Dictionary<string, System.Collections.Generic.List<(string, string, TextBox)>> _entries = new();

    public MainWindow()
    {
        InitializeComponent();
        foreach (var (name, fields) in _requests)
        {
            var row = new StackPanel { Orientation = Orientation.Horizontal, Margin = new Thickness(0, 4, 0, 0) };
            var button = new Button { Content = name, Padding = new Thickness(8, 3, 8, 3) };
            button.Click += async (_, _) => await Send(name);
            row.Children.Add(button);
            _entries[name] = new();
            foreach (var (key, kind) in fields)
            {
                var box = new TextBox { Width = 120, Margin = new Thickness(6, 0, 0, 0), ToolTip = key + " (" + kind + ")" };
                row.Children.Add(box);
                _entries[name].Add((key, kind, box));
            }
            Requests.Children.Add(row);
        }
        _vero = new VeroClient(Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "{{.WorkerName}}.exe"));
        _ = ReadEvents();
    }

    private async System.Threading.Tasks.Task ReadEvents()
    {
        await foreach (var element in _vero.Events())
            Dispatcher.Invoke(() => Apply(element));
    }

    // Redraws the state as rows: scalars as "key: value", lists and nested
    // objects indented under their key.
    private void Apply(JsonElement state)
    {
        Tree.Children.Clear();
        Rows(state, 0);
    }

    private void Rows(JsonElement value, int depth)
    {
        if (value.ValueKind == JsonValueKind.Object)
        {
            foreach (var p in value.EnumerateObject())
            {
                if (p.Value.ValueKind is JsonValueKind.Object or JsonValueKind.Array)
                {
                    Row(p.Name + ":", depth, bold: true);
                    Rows(p.Value, depth + 1);
                }
                else Row(p.Name + ": " + p.Value, depth);
            }
        }
        else if (value.ValueKind == JsonValueKind.Array)
        {
            foreach (var item in value.EnumerateArray())
            {
                if (item.ValueKind == JsonValueKind.Object)
                {
                    var parts = new System.Collections.Generic.List<string>();
                    foreach (var p in item.EnumerateObject())
                        if (p.Value.ValueKind is not (JsonValueKind.Object or JsonValueKind.Array)) parts.Add(p.Name + ": " + p.Value);
                    Row(string.Join(", ", parts), depth);
                }
                else Row(item.ToString(), depth);
            }
        }
    }

    private void Row(string text, int depth, bool bold = false) =>
        Tree.Children.Add(new TextBlock { Text = text, Margin = new Thickness(depth * 16, 1, 0, 1), FontWeight = bold ? FontWeights.SemiBold : FontWeights.Normal });

    // Sends a request with what its entries hold, and draws the reply.
    private async System.Threading.Tasks.Task Send(string name)
    {
        var data = new System.Collections.Generic.Dictionary<string, object?>();
        foreach (var (key, kind, box) in _entries[name])
            data[key] = kind switch
            {
                "int" => long.TryParse(box.Text, out var n) ? n : 0,
                "float" => double.TryParse(box.Text, out var d) ? d : 0,
                "bool" => box.Text is "1" or "true" or "yes",
                _ => box.Text,
            };
        try
        {
            var reply = await _vero.CallAsync<object, JsonElement>(name, data);
            Problem.Text = "";
            if (reply.ValueKind == JsonValueKind.Object) Apply(reply);
        }
        catch (VeroException e)
        {
            Problem.Text = e.Message;
        }
    }
}
`

const macToml = `# The macOS front end, which vero add wrote: a Swift package.
[macos]
folder = "macos"
product = "{{pascal .Name}}"
`

const macReadme = `# {{.Display}} for macOS

A SwiftUI menu bar app, written by ` + "`vero add`" + ` for the worker in ` + "`../{{.Worker}}`" + `.
It shows the state the worker pushes and offers each of its requests; make it
your own from there. It is a Swift package, which ` + "`swift build`" + ` builds; for an
app with an icon, a Dock presence and the rest, make an Xcode project that uses
` + "`Sources/{{pascal .Name}}/App.swift`" + ` and bundles the worker in Contents/Resources.

    ./build.sh                        builds the worker and the app
    ./.build/debug/{{pascal .Name}}      runs it
`

const macPackage = `// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "{{pascal .Name}}",
    platforms: [.macOS(.v13)],
    dependencies: [.package(url: "https://github.com/imclaren/vero", from: "0.15.0")],
    targets: [
        .executableTarget(name: "{{pascal .Name}}", dependencies: [.product(name: "Vero", package: "vero")])
    ]
)
`

const macBuild = `#!/bin/sh
# Builds the worker and the app. Run from this directory.
set -e
cd "$(dirname "$0")"
go build -o {{.WorkerName}} ../{{.Worker}}
swift build
# A SwiftPM executable is not an app bundle, so the worker goes beside it.
cp {{.WorkerName}} .build/debug/{{.WorkerName}}
echo "run it with:  ./.build/debug/{{pascal .Name}}"
`

const macApp = `import SwiftUI
import Vero

// The shapes the worker pushes and takes, as in ../{{.Worker}}. A window
// of your own reads vero.state through these.
{{range $name, $s := .Types}}struct {{$s.Name}}: Decodable {
{{range $s.Fields}}    let {{camel .Name}}: {{swiftType .}}?
{{end}}    private enum CodingKeys: String, CodingKey {
{{range $s.Fields}}        case {{camel .Name}} = "{{.JSON}}"
{{end}}    }
}

{{end}}
/// Anything JSON, for fields the worker leaves open.
enum JSONValue: Decodable {
    case string(String), number(Double), bool(Bool), null, array([JSONValue]), object([String: JSONValue])
    init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null }
        else if let b = try? c.decode(Bool.self) { self = .bool(b) }
        else if let n = try? c.decode(Double.self) { self = .number(n) }
        else if let s = try? c.decode(String.self) { self = .string(s) }
        else if let a = try? c.decode([JSONValue].self) { self = .array(a) }
        else { self = .object(try c.decode([String: JSONValue].self)) }
    }
}

extension JSONValue: Encodable {
    func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .string(let s): try c.encode(s)
        case .number(let n): try c.encode(n)
        case .bool(let b): try c.encode(b)
        case .null: try c.encodeNil()
        case .array(let a): try c.encode(a)
        case .object(let o): try c.encode(o)
        }
    }
}

// One of these per handler on the worker: the name routes the request.
{{range .Requests}}struct {{pascal .Name}}Request: NamedRequest {
    static let name = "{{.Name}}"
    typealias Reply = JSONValue
{{range .Fields}}    let {{camel .Name}}: {{swiftReqType .}}
{{end}}{{if .Fields}}    private enum CodingKeys: String, CodingKey {
{{range .Fields}}        case {{camel .Name}} = "{{.JSON}}"
{{end}}    }
{{end}}}
{{end}}
/// The requests the worker handles, and what each takes.
let requests: [(name: String, fields: [(key: String, kind: String)])] = [
{{- range .Requests}}
    ("{{.Name}}", [{{range .Fields}}("{{.JSON}}", "{{.Kind}}"), {{end}}]),
{{- end}}
]

@main
struct {{pascal .Name}}App: App {
    init() { VeroSingleCopy.yieldToRunningCopy() }

    // The worker, started from beside this app (or from the bundle's
    // Resources, in an app bundle), and the last state it pushed.
    @StateObject private var vero = VeroModel<JSONValue>(bundledWorker: "{{.WorkerName}}", directoryName: "{{.Name}}/bin")

    var body: some Scene {
        MenuBarExtra("{{.Display}}", systemImage: "circle.grid.2x2") {
            StarterView().environmentObject(vero)
        }
        .menuBarExtraStyle(.window)
    }
}

/// The state as rows, and a control for each request. Written by vero add;
/// make it your own.
struct StarterView: View {
    @EnvironmentObject var vero: VeroModel<JSONValue>
    @State private var entries: [String: [String: String]] = [:]

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let s = vero.state { rows(s, depth: 0) }
            if let p = vero.problem { Text(p).foregroundStyle(.red) }
            ForEach(requests, id: \.name) { r in
                HStack {
                    Button(r.name) { send(r.name) }
                    ForEach(r.fields, id: \.key) { f in
                        TextField("\(f.key) (\(f.kind))", text: Binding(
                            get: { entries[r.name]?[f.key] ?? "" },
                            set: { entries[r.name, default: [:]][f.key] = $0 }))
                    }
                }
            }
        }
        .padding(16)
        .frame(width: 460)
    }

    @ViewBuilder private func rows(_ v: JSONValue, depth: Int) -> some View {
        switch v {
        case .object(let o):
            ForEach(o.keys.sorted(), id: \.self) { k in
                switch o[k]! {
                case .object, .array:
                    Text("\(k):").bold().padding(.leading, CGFloat(depth * 16))
                    AnyView(rows(o[k]!, depth: depth + 1))
                default:
                    Text("\(k): \(text(o[k]!))").padding(.leading, CGFloat(depth * 16))
                }
            }
        case .array(let a):
            ForEach(Array(a.enumerated()), id: \.offset) { _, item in
                if case .object(let o) = item {
                    Text(o.keys.sorted().compactMap { k in
                        if case .object = o[k]! { return nil }
                        if case .array = o[k]! { return nil }
                        return "\(k): \(text(o[k]!))"
                    }.joined(separator: ", ")).padding(.leading, CGFloat(depth * 16))
                } else {
                    Text(text(item)).padding(.leading, CGFloat(depth * 16))
                }
            }
        default:
            Text(text(v))
        }
    }

    private func text(_ v: JSONValue) -> String {
        switch v {
        case .string(let s): return s
        case .number(let n): return n == n.rounded() ? String(Int(n)) : String(n)
        case .bool(let b): return b ? "true" : "false"
        case .null: return "-"
        default: return "…"
        }
    }

    private func entry(_ name: String, _ key: String) -> String { entries[name]?[key] ?? "" }
    private func int(_ name: String, _ key: String) -> Int { Int(entry(name, key)) ?? 0 }
    private func double(_ name: String, _ key: String) -> Double { Double(entry(name, key)) ?? 0 }
    private func bool(_ name: String, _ key: String) -> Bool { ["1", "true", "yes"].contains(entry(name, key).lowercased()) }

    /// Sends a request with what its entries hold. A refusal shows as
    /// vero.problem, and the worker pushes the new state.
    private func send(_ name: String) {
        switch name {
{{- range $r := .Requests}}
        case "{{$r.Name}}":
            vero.call({{pascal $r.Name}}Request({{range $i, $f := $r.Fields}}{{if $i}}, {{end}}{{camel $f.Name}}: {{if eq $f.Kind "int"}}int("{{$r.Name}}", "{{$f.JSON}}"){{else if eq $f.Kind "float"}}double("{{$r.Name}}", "{{$f.JSON}}"){{else if eq $f.Kind "bool"}}bool("{{$r.Name}}", "{{$f.JSON}}"){{else if eq $f.Kind "string"}}entry("{{$r.Name}}", "{{$f.JSON}}"){{else}}.null{{end}}{{end}}))
{{- end}}
        default:
            break
        }
    }
}
`
