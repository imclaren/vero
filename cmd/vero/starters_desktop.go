package main

// The desktop starters: GTK for Linux, the BSDs and illumos; WPF for
// Windows; SwiftUI for macOS. Each shows the state the worker pushes as a
// tree of its fields, and a control for each request, and is the point to
// start a window of your own from.

var starters = map[string]*starter{}

func init() {
	starters["gtk"] = &starter{folder: "gtk", note: "a GTK 4 app in Python, for Linux, the BSDs and illumos, on vero_gtk.py, which uses libadwaita where the system has it; needs python3-gi and GTK 4", files: map[string]string{
		"README.md": gtkReadme, "main.py": gtkMain, "build.sh": gtkBuild, ".gitignore": gtkIgnore}, bindings: []string{"bindings/python/vero.py", "bindings/python/vero_gtk.py"}, toml: gtkToml}
	starters["wpf"] = &starter{folder: "windows", note: "a WPF app in C#, for Windows, on VeroWpf.cs, with Windows' Fluent look; needs the .NET 10 SDK (dotnet)", files: map[string]string{
		"README.md": wpfReadme, "App.xaml": wpfAppXaml, "App.xaml.cs": wpfAppCs, "MainWindow.xaml": wpfWindowXaml, "MainWindow.xaml.cs": wpfWindowCs,
		"Theme.xaml": wpfTheme, "{{pascal .Name}}.csproj": wpfProj, ".gitignore": wpfIgnore}, bindings: []string{"bindings/csharp/Vero.cs", "bindings/csharp/VeroWpf.cs"}, toml: wpfToml}
	starters["macos"] = &starter{folder: "macos", note: "a SwiftUI app, for macOS; needs Xcode's command line tools", files: map[string]string{
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
# vero works out what each system's package needs from uses: Python,
# PyGObject, GTK 4 and a font always, and libadwaita, the web view
# ("webview") and a keyring ("keyring") as named. extra names programs vero
# knows, such as "ffmpeg". Anything else goes in a system's own section,
# such as depends in [gtk.deb]. Debian, Fedora, openSUSE and Arch are always
# packaged; each other system is packaged when its section is here.
[gtk]
folder = "gtk"
entry = "main.py"
categories = "Utility;"
uses = ["adwaita", "keyring"]

[gtk.deb]
section = "utils"

[gtk.alpine]

[gtk.void]

[gtk.chimera]

[gtk.freebsd]

[gtk.dragonfly]

[gtk.netbsd]

[gtk.illumos]

[gtk.openbsd]
`

const gtkReadme = `# {{.Display}} for Linux, the BSDs and illumos

A GTK 4 {{if .MenuBar}}tray app{{else}}app{{end}} in Python, written by ` + "`vero add`" + ` for the worker in
` + "`../{{.Worker}}`" + `. It shows the state the worker pushes and offers each of its
requests; make it your own from there. ` + "`main.py`" + ` is the app's own;
` + "`vero.py`" + ` and ` + "`vero_gtk.py`" + ` are vero's, which ` + "`vero upgrade`" + ` replaces, and
` + "`worker_api.py`" + ` is what ` + "`vero gen`" + ` writes from the worker.

    ./build.sh      builds the worker beside this file
    ./main.py       runs it

It needs Python 3.9 or later with PyGObject and GTK 4.8 or later: on Debian
and Ubuntu, ` + "`python3-gi gir1.2-gtk-4.0`" + `, with ` + "`gir1.2-adw-1`" + ` for libadwaita. On a Mac, ` + "`build.sh`" + ` installs them with Homebrew and
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
"""{{.Display}}: the {{if .MenuBar}}tray app{{else if .Tray}}window app, with a tray icon,{{else}}window app{{end}} for Linux, the BSDs and illumos, on the worker in
../{{.Worker}}.

Written by vero add. It shows the worker's state, every field of
{{.State.Name}}, and a control for each request{{if .Requests}} ({{range $i, $r := .Requests}}{{if $i}}, {{end}}{{$r.Name}}{{end}}){{end}}.
Start from here and make it your own.

vero_gtk.py, beside it, does the plumbing: the worker, a single copy,
{{if .MenuBar}}the tray icon and its small window, {{end}}banners and toasts, light and dark, and
libadwaita where the system has it. worker_api.py, which vero gen writes,
has a typed method for each request, for a window of your own.
"""

from __future__ import annotations

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import vero_gtk as vg  # noqa: E402
from gi.repository import Gtk  # noqa: E402

# The requests the worker handles, and what each takes: name, then each
# field's JSON key and kind.
REQUESTS = [
{{- range .Requests}}
    ("{{.Name}}", [{{range .Fields}}("{{.JSON}}", "{{.Kind}}"), {{end}}]),
{{- end}}
]


class Main(vg.Window):
    """The main window, or, small, the window that the tray icon opens,
    which shows the state and opens the main window."""

    def __init__(self, app: vg.App, small: bool = False) -> None:
        super().__init__(app, "{{.Display}}", *((360, 480) if small else (480, 560)))
        box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=12,
                      margin_top=16, margin_bottom=16, margin_start=16, margin_end=16)

        # The state, as a tree of its fields.
        self.tree = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=4)
        box.append(self.tree)
        if small:
            button = Gtk.Button(label="Open {{.Display}}", halign=Gtk.Align.START)
            button.connect("clicked", lambda _b: app.show_window())
            box.append(button)

        # One row per request: entries for what it takes, and a button.
        self.entries: dict[str, list[tuple[str, str, Gtk.Entry]]] = {}
        for name, fields in ([] if small else REQUESTS):
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

        self.set_content(Gtk.ScrolledWindow(child=box, hscrollbar_policy=Gtk.PolicyType.NEVER))
        app.on_state(self.apply)

    def apply(self, state: dict) -> None:
        """Redraws the state as rows: scalars as "key: value", lists and
        nested objects indented under their key."""
        while child := self.tree.get_first_child():
            self.tree.remove(child)
        self.rows(state, 0)

    def rows(self, value, depth: int) -> None:
        if isinstance(value, dict):
            for key, v in value.items():
                if isinstance(v, (dict, list)):
                    self.row(f"{key}:", depth, bold=True)
                    self.rows(v, depth + 1)
                else:
                    self.row(f"{key}: {v}", depth)
        elif isinstance(value, list):
            for v in value:
                if isinstance(v, dict):
                    self.row(", ".join(f"{k}: {x}" for k, x in v.items() if not isinstance(x, (dict, list))), depth)
                    self.rows({k: x for k, x in v.items() if isinstance(x, (dict, list))}, depth + 1)
                else:
                    self.row(str(v), depth)

    def row(self, text: str, depth: int, bold: bool = False) -> None:
        label = vg.label(text, "heading" if bold else "")
        label.set_margin_start(depth * 16)
        self.tree.append(label)

    def send(self, name: str) -> None:
        """Sends a request with what its entries hold. The worker pushes
        the new state, and a refusal shows in the banner."""
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
        self.problem("")
        self.app.ask(name, data)


if __name__ == "__main__":
    vg.main(vg.App("{{.ID}}", "{{.Display}}", "{{.WorkerName}}", window=Main{{if .MenuBar}},
                   small_window=lambda app: Main(app, small=True){{end}}{{if .Tray}}, tray=True{{end}}))
`

const wpfToml = `# The Windows front end, which vero add wrote.
[wpf]
folder = "windows"
exe = "{{pascal .Name}}.exe"
`

const wpfReadme = `# {{.Display}} for Windows

A WPF {{if .MenuBar}}tray app{{else}}app{{end}} in C#, written by ` + "`vero add`" + ` for the worker in ` + "`../{{.Worker}}`" + `,
with Windows' Fluent look. It shows the state the worker pushes and offers each
of its requests; make it your own from there. The window's layout is in
` + "`MainWindow.xaml`" + `, and the app's spacing and type in ` + "`Theme.xaml`" + `.

    GOOS=windows go build -o {{.WorkerName}}.exe ../{{.Worker}}   # the worker, beside the app
    dotnet build                                               # on Windows, or on a Mac to check it compiles

` + "`Vero.cs`" + ` and ` + "`VeroWpf.cs`" + ` are vero's binding and library, which ` + "`vero upgrade`" + `
replaces, and ` + "`WorkerApi.cs`" + ` is what ` + "`vero gen`" + ` writes from the worker. The
packaging in vero-app.toml builds the app and puts the worker beside it.
`

const wpfProj = `<Project Sdk="Microsoft.NET.Sdk">
  <!-- Written by vero add. Vero.cs and VeroWpf.cs, beside it, are vero's
       binding and library; WorkerApi.cs is what vero gen writes from the
       worker. Windows Forms is for the tray icon. -->
  <PropertyGroup>
    <OutputType>WinExe</OutputType>
    <TargetFramework>net10.0-windows</TargetFramework>
    <UseWPF>true</UseWPF>
    <UseWindowsForms>true</UseWindowsForms>
    <ImplicitUsings>disable</ImplicitUsings>
    <Nullable>enable</Nullable>
    <RootNamespace>{{pascal .Name}}</RootNamespace>
    <AssemblyName>{{pascal .Name}}</AssemblyName>
    <EnableWindowsTargeting>true</EnableWindowsTargeting>
  </PropertyGroup>
</Project>
`

const wpfTheme = `<ResourceDictionary xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
                    xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml">
    <!-- The app's own spacing, type and icons, kept here so that the
         windows' XAML stays about layout. Colours come from Windows'
         Fluent theme, which follows light and dark mode; add the app's own
         here when it needs them. -->
    <Thickness x:Key="PagePadding">16</Thickness>
    <Thickness x:Key="RowGap">0,4,0,0</Thickness>
    <Thickness x:Key="SectionGap">0,12,0,0</Thickness>
    <FontFamily x:Key="Icons">Segoe Fluent Icons, Segoe MDL2 Assets</FontFamily>
    <Style x:Key="Heading" TargetType="TextBlock">
        <Setter Property="FontWeight" Value="SemiBold"/>
    </Style>
</ResourceDictionary>
`

const wpfAppXaml = `<vero:VeroApp x:Class="{{pascal .Name}}.App"
              xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
              xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
              xmlns:vero="clr-namespace:Vero">
    <Application.Resources>
        <ResourceDictionary Source="Theme.xaml"/>
    </Application.Resources>
</vero:VeroApp>
`

const wpfAppCs = `using System.Windows;
using Vero;

namespace {{pascal .Name}};

/// <summary>The app, which VeroApp runs: the worker, a single copy, {{if .MenuBar}}the
/// tray icon and its small window, {{else if .Tray}}the tray icon, {{end}}and Windows' Fluent look, light or
/// dark as Windows is. Written by vero add.</summary>
public partial class App : VeroApp
{
    public App() : base(new VeroAppOptions
    {
        Id = "{{.ID}}",
        Name = "{{.Display}}",
        Worker = "{{.WorkerName}}.exe",{{if .MenuBar}}
        MenuBar = true,{{end}}{{if .Tray}}
        Tray = true,{{end}}
    })
    { }

    protected override Window CreateWindow() => new MainWindow();{{if .MenuBar}}

    /// <summary>The small window that the tray icon opens.</summary>
    protected override Window CreateSmallWindow() => new MainWindow(small: true);{{end}}
}
`

const wpfWindowXaml = `<vero:VeroWindow x:Class="{{pascal .Name}}.MainWindow"
                 xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
                 xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
                 xmlns:vero="clr-namespace:Vero"
                 Title="{{.Display}}" Width="480" Height="560">
    <!-- Written by vero add: the worker's state as rows, and a control for
         each request. Make it your own. VeroWindow puts its banners and
         toasts around this. -->
    <ScrollViewer VerticalScrollBarVisibility="Auto">
        <StackPanel Margin="{StaticResource PagePadding}">
            <StackPanel x:Name="Tree"/>
            <Button x:Name="OpenMain" Content="Open {{.Display}}" HorizontalAlignment="Left"
                    Margin="{StaticResource SectionGap}" Visibility="Collapsed"/>
            <StackPanel x:Name="Requests" Margin="{StaticResource SectionGap}"/>
        </StackPanel>
    </ScrollViewer>
</vero:VeroWindow>
`

const wpfWindowCs = `using System.Collections.Generic;
using System.Text.Json;
using System.Windows;
using System.Windows.Controls;
using Vero;

namespace {{pascal .Name}};

/// <summary>The worker's state as rows, and a control for each request.
/// Written by vero add; make it your own. WorkerApi.cs, which vero gen
/// writes, has a typed method for each request, for a window of your
/// own.</summary>
public partial class MainWindow : VeroWindow
{
    // The requests the worker handles, and what each takes.
    static readonly (string name, (string key, string kind)[] fields)[] requests =
    {
{{- range .Requests}}
        ("{{.Name}}", new (string key, string kind)[] { {{range .Fields}}("{{.JSON}}", "{{.Kind}}"), {{end}} }),
{{- end}}
    };
    readonly Dictionary<string, List<(string key, string kind, TextBox box)>> entries = new();

    public MainWindow() : this(false) { }

    /// <summary>The main window, or, small, the window that the tray icon
    /// opens, which shows the state and opens the main window.</summary>
    public MainWindow(bool small)
    {
        InitializeComponent();
        if (small)
        {
            Width = 360;
            Height = 480;
            OpenMain.Visibility = Visibility.Visible;
            OpenMain.Click += (_, _) => App.ShowMainWindow();
        }
        foreach (var (name, fields) in small ? System.Array.Empty<(string, (string, string)[])>() : requests)
        {
            var row = new StackPanel { Orientation = Orientation.Horizontal, Margin = (Thickness)FindResource("RowGap") };
            var button = new Button { Content = name };
            button.Click += async (_, _) => await Send(name);
            row.Children.Add(button);
            entries[name] = new();
            foreach (var (key, kind) in fields)
            {
                var box = new TextBox { Width = 120, Margin = new Thickness(6, 0, 0, 0), ToolTip = $"{key} ({kind})" };
                row.Children.Add(box);
                entries[name].Add((key, kind, box));
            }
            Requests.Children.Add(row);
        }
        Loaded += (_, _) => App.OnState(Apply);
    }

    // Redraws the state as rows: scalars as "key: value", lists and nested
    // objects indented under their key.
    void Apply(JsonElement state)
    {
        Tree.Children.Clear();
        Rows(state, 0);
    }

    void Rows(JsonElement value, int depth)
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
                else Row($"{p.Name}: {p.Value}", depth);
            }
        }
        else if (value.ValueKind == JsonValueKind.Array)
        {
            foreach (var item in value.EnumerateArray())
            {
                if (item.ValueKind == JsonValueKind.Object)
                {
                    var parts = new List<string>();
                    foreach (var p in item.EnumerateObject())
                        if (p.Value.ValueKind is not (JsonValueKind.Object or JsonValueKind.Array)) parts.Add($"{p.Name}: {p.Value}");
                    Row(string.Join(", ", parts), depth);
                }
                else Row(item.ToString(), depth);
            }
        }
    }

    void Row(string text, int depth, bool bold = false)
    {
        var row = new TextBlock { Text = text, Margin = new Thickness(depth * 16, 1, 0, 1) };
        if (bold) row.Style = (Style)FindResource("Heading");
        Tree.Children.Add(row);
    }

    // Sends a request with what its entries hold. The worker pushes the new
    // state, and a refusal shows in the banner.
    async System.Threading.Tasks.Task Send(string name)
    {
        var data = new Dictionary<string, object?>();
        foreach (var (key, kind, box) in entries[name])
            data[key] = kind switch
            {
                "int" => long.TryParse(box.Text, out var n) ? n : 0,
                "float" => double.TryParse(box.Text, out var d) ? d : 0,
                "bool" => box.Text is "1" or "true" or "yes",
                _ => box.Text,
            };
        Problem("");
        await App.Ask(name, data);
    }
}
`

const macToml = `# The macOS front end, which vero add wrote: a Swift package.
[macos]
folder = "macos"
product = "{{pascal .Name}}"
`

const macReadme = `# {{.Display}} for macOS

A SwiftUI app, written by ` + "`vero add`" + ` for the worker in ` + "`../{{.Worker}}`" + `.
It opens a window that shows the state the worker pushes and offers each of
its requests. Make it your own from there. It is a Swift package, which
` + "`swift build`" + ` builds. ` + "`vero ship`" + ` makes it an app with its icon and the worker
inside, signs it, and puts it in the release.

    ./build.sh                        builds the worker and the app
    ./.build/debug/{{pascal .Name}}      runs it
`

const macPackage = `// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "{{pascal .Name}}",
    platforms: [.macOS(.v13)],
    dependencies: [{{.SwiftPackage}}],
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

// {{.Display}} for macOS, {{if .MenuBar}}in the menu bar, with a main window{{else if .Tray}}in a window, with an icon in the menu bar{{else}}in a window{{end}}, on the worker in ../{{.Worker}}.
// Written by vero add: it shows the worker's state, every field of
// {{.State.Name}}, and a control for each request. Make it your own.
// WorkerAPI.swift, beside it, is what vero gen writes from the worker: a
// type for the state and for each request, which a view of your own reads.

/// The requests the worker handles, and what each takes.
let requests: [(name: String, fields: [(key: String, kind: String)])] = [
{{- range .Requests}}
    ("{{.Name}}", [{{range .Fields}}("{{.JSON}}", "{{.Kind}}"), {{end}}]),
{{- end}}
]

@main
struct {{pascal .Name}}App: App {
    init() {
        VeroSingleCopy.yieldToRunningCopy(){{if .MenuBar}}
        // No Dock icon, except while the main window is open.
        VeroDock.follow(){{end}}
    }

    // The worker, started from beside this app (or from the bundle's
    // Resources, in an app bundle), and the last state it pushed.
    @StateObject private var vero = VeroModel<JSONValue>(bundledWorker: "{{.WorkerName}}", directoryName: "{{.Name}}/bin")

    var body: some Scene {
{{- if .MenuBar}}
        // In the menu bar, with no Dock icon once vero ship makes it an app.
        MenuBarExtra("{{.Display}}", systemImage: "circle.grid.2x2") {
            StarterView(small: true).environmentObject(vero)
        }
        .menuBarExtraStyle(.window)
        // The main window, opened from the menu bar's window.
        Window("{{.Display}}", id: "main") {
            StarterView().environmentObject(vero).veroOpensWhenAsked()
        }
        .windowResizability(.contentSize)
{{- else}}
        Window("{{.Display}}", id: "main") {
            StarterView().environmentObject(vero)
        }
        .windowResizability(.contentSize)
{{- if .Tray}}
        // An icon in the menu bar as well, from which the window opens again.
        MenuBarExtra("{{.Display}}", systemImage: "circle.grid.2x2") {
            StarterView(small: true).environmentObject(vero)
        }
        .menuBarExtraStyle(.window)
{{- end}}
{{- end}}
    }
}

/// The state as rows, and a control for each request; or, small, the
/// state and a button that opens the main window, for the menu bar.
struct StarterView: View {
    var small = false
    @EnvironmentObject var vero: VeroModel<JSONValue>
    @Environment(\.openWindow) private var openWindow
    @State private var entries: [String: [String: String]] = [:]

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let s = vero.state { rows(s, depth: 0) }
            if let p = vero.problem { Text(p).foregroundStyle(.red) }
            if small {
                Divider()
                Button("Open {{.Display}}") { VeroDock.open("main", with: openWindow) }
                Button("Quit {{.Display}}") { NSApplication.shared.terminate(nil) }
            }
            ForEach(small ? [] : requests, id: \.name) { r in
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
        .frame(width: small ? 340 : 460)
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

    /// Sends a request, from WorkerAPI.swift, with what its entries hold. A
    /// refusal shows as vero.problem, and the worker pushes the new state.
    private func send(_ name: String) {
        switch name {
{{- range $r := .Requests}}
        case "{{$r.Name}}":
            vero.call({{pascal $r.Name}}Request({{$first := true}}{{range $f := $r.Fields}}{{if or (eq $f.Kind "int") (eq $f.Kind "float") (eq $f.Kind "bool") (eq $f.Kind "string")}}{{if not $first}}, {{end}}{{$first = false}}{{swiftLabel $f.JSON}}: {{if eq $f.Kind "int"}}int("{{$r.Name}}", "{{$f.JSON}}"){{else if eq $f.Kind "float"}}double("{{$r.Name}}", "{{$f.JSON}}"){{else if eq $f.Kind "bool"}}bool("{{$r.Name}}", "{{$f.JSON}}"){{else}}entry("{{$r.Name}}", "{{$f.JSON}}"){{end}}{{end}}{{end}}))
{{- end}}
        default:
            break
        }
    }
}
`
