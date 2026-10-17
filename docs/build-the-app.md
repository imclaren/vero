# Build the app

The front page left you with `jotter` running on the Mac. This page turns
it into a small real app: a list of notes, with a sign-in that survives a
restart, and a screen of its own on each system. It takes about an hour.
Every step was run on a fresh `jotter`, so what you should see is what you
will see.

If you skipped the front page, run `vero new jotter && cd jotter` first.

## 1. Replace the worker

Replace `cmd/worker/main.go` with this. It keeps a list of notes and
answers two requests, one to add a note and one to delete one.

```go
// Command worker is the logic half of jotter: a list of notes.
package main

import (
	"flag"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/imclaren/vero"
)

// version has to increase on every release. vero ship sets it.
var version = "0.1.0"

type Note struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
	When string `json:"when"`
}

type Status struct {
	Notes []Note `json:"notes"`
}

type AddNote struct {
	Text string `json:"text"`
}

func main() {
	var opts vero.WorkerOptions
	opts.Version = version
	opts.RegisterFlags(flag.CommandLine)
	flag.Parse()
	opts.PrintVersionAndExit()

	w := vero.NewWorker(opts)
	state := vero.NewState(w, Status{Notes: []Note{}})
	nextID := 1

	vero.UpdateWith(state, "addNote", func(s *Status, req AddNote) error {
		text := strings.TrimSpace(req.Text)
		if text == "" {
			return fmt.Errorf("a note needs some text")
		}
		s.Notes = append(s.Notes, Note{ID: nextID, Text: text, When: time.Now().Format("15:04")})
		nextID++
		return nil
	})

	vero.UpdateWith(state, "deleteNote", func(s *Status, req vero.ID[int]) error {
		before := len(s.Notes)
		s.Notes = slices.DeleteFunc(s.Notes, func(n Note) bool { return n.ID == req.ID })
		if len(s.Notes) == before {
			return fmt.Errorf("no note with id %d", req.ID)
		}
		return nil
	})

	if err := w.Serve(); err != nil {
		w.Log("stopped: %v", err)
	}
}
```

Build it, regenerate the bindings, and run the Mac app:

```bash
go build ./... && vero gen
cd macos && ./build.sh
VERO_PROFILE=dev ./.build/debug/Jotter
```

You should see the starter window with `notes:` and nothing under it, then
a row with an **addNote** button and a text field, and a row with a
**deleteNote** button and an id field. Type something and press addNote.
The note appears under `notes:` with the time. Press addNote with the
field empty, and the worker's refusal, "a note needs some text", appears
in red.

What you just used:

- `Status` is the state. vero sends it to the window whenever it changes.
  Every field needs a `json` tag.
- `vero.UpdateWith` registers a handler that takes a struct and changes
  the state. It replies with the new state, so the window never shows a
  stale one. `vero.Update` is the same without a struct, and `vero.Handle`
  is for a request that replies with something other than the state.
- Returning an error sends the message to the window and changes nothing.
- `vero gen` rewrote `WorkerAPI.swift`, `worker_api.py` and `WorkerApi.cs`
  with a type for `Status`, `Note`, and each request. Never edit those
  three files.

## 2. A screen on the Mac

Replace `macos/Sources/Jotter/App.swift` with this. It is a text field, an
Add button, and a list with a Delete button on each row.

```swift
import SwiftUI
import Vero

@main
struct JotterApp: App {
    init() {
        VeroSingleCopy.yieldToRunningCopy()
    }

    // The worker, started from beside this app, and the last state it pushed.
    @StateObject private var vero = VeroModel<Status>(bundledWorker: "jotter-worker", directoryName: "jotter/bin")

    var body: some Scene {
        Window("jotter", id: "main") {
            NotesView().environmentObject(vero)
        }
    }
}

struct NotesView: View {
    @EnvironmentObject var vero: VeroModel<Status>
    @State private var text = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                TextField("A new note", text: $text).onSubmit(add)
                Button("Add", action: add).disabled(text.isEmpty)
            }
            List(vero.state?.notes ?? [], id: \.id) { note in
                HStack {
                    Text(note.text)
                    Spacer()
                    Text(note.when).foregroundStyle(.secondary)
                    Button("Delete") { vero.call(DeleteNoteRequest(id: note.id)) }
                }
            }
            if let problem = vero.problem {
                Text(problem).foregroundStyle(.red)
            }
        }
        .padding(16)
        .frame(minWidth: 420, minHeight: 320)
    }

    private func add() {
        vero.call(AddNoteRequest(text: text))
        text = ""
    }
}
```

```bash
./build.sh
VERO_PROFILE=dev ./.build/debug/Jotter
```

You should see the text field and the list. Type a note and press Return.
It appears in the list with the time, and the field clears. Press Delete
on a row and it goes.

What changed from the starter:

- `VeroModel<Status>` instead of `VeroModel<JSONValue>`. The model now
  decodes the state into the `Status` type that `vero gen` wrote, so
  `vero.state?.notes` is a `[Note]`.
- `vero.call(AddNoteRequest(text: text))` sends a request. The request
  types come from `WorkerAPI.swift`, and the reply is typed as `Status`.
  You do not need the reply: the worker pushes the new state as well, and
  the list redraws from that.
- `vero.problem` is the worker's last refusal, or nil.

## 3. The same screen in GTK

Replace `gtk/main.py` with this.

```python
#!/usr/bin/env python3
"""jotter for Linux, the BSDs and illumos: a list of notes."""

from __future__ import annotations

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import vero_gtk as vg  # noqa: E402
from gi.repository import Gtk  # noqa: E402
from worker_api import Status, Worker  # noqa: E402


class Main(vg.Window):
    def __init__(self, app: vg.App, small: bool = False) -> None:
        super().__init__(app, "jotter", 480, 560)
        box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=12,
                      margin_top=16, margin_bottom=16, margin_start=16, margin_end=16)

        row = Gtk.Box(spacing=6)
        self.entry = Gtk.Entry(placeholder_text="A new note", hexpand=True)
        self.entry.connect("activate", lambda _e: self.add())
        button = Gtk.Button(label="Add")
        button.connect("clicked", lambda _b: self.add())
        row.append(self.entry)
        row.append(button)
        box.append(row)

        self.notes = Gtk.ListBox(selection_mode=Gtk.SelectionMode.NONE)
        box.append(self.notes)

        self.set_content(Gtk.ScrolledWindow(child=box, hscrollbar_policy=Gtk.PolicyType.NEVER))
        app.on_state(self.apply)

    def apply(self, state: dict) -> None:
        """Redraws the list from the state the worker pushed."""
        while child := self.notes.get_first_child():
            self.notes.remove(child)
        for note in Status.from_json(state).notes:
            row = Gtk.Box(spacing=6, margin_top=4, margin_bottom=4)
            text = vg.label(note.text)
            text.set_hexpand(True)
            row.append(text)
            row.append(vg.label(note.when, "dim-label"))
            delete = Gtk.Button(label="Delete")
            delete.connect("clicked", lambda _b, id=note.id: self.delete(id))
            row.append(delete)
            self.notes.append(row)

    def add(self) -> None:
        text = self.entry.get_text()
        self.entry.set_text("")
        self.app.ask(lambda: Worker(self.app.vero).add_note(text=text))

    def delete(self, id: int) -> None:
        self.app.ask(lambda: Worker(self.app.vero).delete_note(id=id))


if __name__ == "__main__":
    vg.main(vg.App("com.example.jotter", "jotter", "jotter-worker", window=Main))
```

GTK runs on the Mac, so you can see it here first:

```bash
cd ../gtk && ./build.sh
VERO_PROFILE=dev $(brew --prefix)/bin/python3 main.py
```

The first `build.sh` installs GTK 4 and PyGObject with Homebrew, which
takes a few minutes. You should see the same window as on the Mac, in
GTK. Add and delete work the same way. Press Add with the field empty,
and the worker's refusal shows in a banner at the top of the window.

To see it on Linux itself, from a checkout of vero:

```bash
path/to/vero/scripts/run-linux.sh --app gtk --entry main.py \
    --worker ./cmd/worker --worker-name jotter-worker --env VERO_PROFILE=dev
```

That cross-compiles the worker, starts the app in a Debian container, and
opens it on the Mac over VNC.

What is the same as on the Mac, in GTK's words:

- `app.on_state(self.apply)` calls `apply` with each state the worker
  pushes, on the main loop. `Status.from_json` turns it into the typed
  `Status` from `worker_api.py`.
- `app.ask(lambda: Worker(app.vero).add_note(text=text))` sends a request
  on a thread, through the typed `Worker` from `worker_api.py`. A refusal
  goes to the window's banner.
- `vg.Window`, `vg.label` and `vg.main` are from `vero_gtk.py`, which also
  handles the single copy, light and dark, and libadwaita where the system
  has it.

## 4. The same screen in WPF

Replace `windows/MainWindow.xaml` with this:

```xml
<vero:VeroWindow x:Class="Jotter.MainWindow"
                 xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
                 xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
                 xmlns:vero="clr-namespace:Vero"
                 Title="jotter" Width="480" Height="560">
    <DockPanel Margin="{StaticResource PagePadding}">
        <DockPanel DockPanel.Dock="Top" Margin="{StaticResource SectionGap}">
            <Button DockPanel.Dock="Right" Content="Add" Margin="6,0,0,0" Click="Add_Click"/>
            <TextBox x:Name="Entry" KeyDown="Entry_KeyDown"/>
        </DockPanel>
        <ItemsControl x:Name="Notes">
            <ItemsControl.ItemTemplate>
                <DataTemplate>
                    <DockPanel Margin="{StaticResource RowGap}">
                        <Button DockPanel.Dock="Right" Content="Delete" Tag="{Binding Id}" Click="Delete_Click"/>
                        <TextBlock DockPanel.Dock="Right" Text="{Binding When}" Opacity="0.6" Margin="8,0" VerticalAlignment="Center"/>
                        <TextBlock Text="{Binding Text}" VerticalAlignment="Center"/>
                    </DockPanel>
                </DataTemplate>
            </ItemsControl.ItemTemplate>
        </ItemsControl>
    </DockPanel>
</vero:VeroWindow>
```

and `windows/MainWindow.xaml.cs` with this:

```csharp
using System.Text.Json;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Input;
using Vero;

namespace Jotter;

/// <summary>A list of notes, drawn from the state the worker pushes.</summary>
public partial class MainWindow : VeroWindow
{
    public MainWindow() : this(false) { }

    public MainWindow(bool small)
    {
        InitializeComponent();
        Loaded += (_, _) => App.OnState(Apply);
    }

    void Apply(JsonElement state) => Notes.ItemsSource = WorkerApi.ReadState(state).Notes;

    async void Add_Click(object sender, RoutedEventArgs e) => await Add();

    async void Entry_KeyDown(object sender, KeyEventArgs e)
    {
        if (e.Key == Key.Enter) await Add();
    }

    async Task Add()
    {
        var text = Entry.Text;
        Entry.Text = "";
        await App.Ask(() => App.Vero.AddNoteAsync(new AddNoteRequest { Text = text }));
    }

    async void Delete_Click(object sender, RoutedEventArgs e)
    {
        var id = (long)((Button)sender).Tag;
        await App.Ask(() => App.Vero.DeleteNoteAsync(new DeleteNoteRequest { Id = id }));
    }
}
```

WPF only runs on Windows, but the Mac can check that it compiles:

```bash
cd ../windows && dotnet build
```

You should see `Build succeeded.` Running it needs the Windows virtual
machine, which [Test the app](test-the-app.md) sets up. There you will
see the same window a third time.

What is the same, in WPF's words:

- `App.OnState(Apply)` calls `Apply` with each state, on the window's
  thread. `WorkerApi.ReadState` turns it into the typed `Status`.
- `App.Ask(() => App.Vero.AddNoteAsync(...))` sends a request through the
  typed extension methods in `WorkerApi.cs`. A refusal goes to the
  window's banner.
- `VeroWindow`, `VeroApp` and the `PagePadding`, `SectionGap` and `RowGap`
  resources are from `VeroWpf.cs` and `Theme.xaml`.

## 5. Keep a secret

Most apps sign in to something, and the token has to live somewhere
safer than a file. The kit's `keychain` package puts it in the system's
own secure store: the login keychain on the Mac, the Credential Manager
on Windows, and the Secret Service on Linux and the BSDs.

Add the kit to the module:

```bash
cd .. && go get github.com/imclaren/vero/kit
```

Then make four changes to `cmd/worker/main.go`. Import the package:

```go
import (
	// ...
	"github.com/imclaren/vero"
	"github.com/imclaren/vero/kit/keychain"
)
```

Give the state a field, add a request type, and name the keychain item:

```go
type Status struct {
	Notes    []Note `json:"notes"`
	SignedIn bool   `json:"signedIn"`
}

type SignIn struct {
	Token string `json:"token"`
}

// token is where the sign-in token is kept: the keychain, under this app's id.
var token = keychain.Item{Service: "com.example.jotter", Account: "token"}
```

Read the token when the worker starts, so that the state says whether
the person is signed in:

```go
	saved, _ := token.Load()
	state := vero.NewState(w, Status{Notes: []Note{}, SignedIn: saved != nil})
```

And add two handlers before `w.Serve()`:

```go
	vero.UpdateWith(state, "signIn", func(s *Status, req SignIn) error {
		if err := token.Save([]byte(req.Token)); err != nil {
			return err
		}
		s.SignedIn = true
		return nil
	})

	vero.Update(state, "signOut", func(s *Status) error {
		if err := token.Remove(); err != nil {
			return err
		}
		s.SignedIn = false
		return nil
	})
```

Give the Mac screen a button for it. In `NotesView`, before the
`if let problem` line:

```swift
            if vero.state?.signedIn == true {
                Button("Sign out") { vero.call(SignOutRequest()) }
            } else {
                Button("Sign in") { vero.call(SignInRequest(token: "demo")) }
            }
```

Then regenerate and run:

```bash
go build ./... && vero gen
cd macos && ./build.sh
VERO_PROFILE=dev ./.build/debug/Jotter
```

You should see a Sign in button under the list. Press it, and it becomes
Sign out. Quit the app and start it again: it still says Sign out,
because the token came back from the keychain. From another terminal:

```bash
security find-generic-password -s com.example.jotter.dev -a token -w
```

prints `demo`. The `.dev` on the end is `VERO_PROFILE`. A development or
test copy keeps its secrets under its own name, so it can never read or
overwrite the secrets of the copy you really use. That is why every
command on this page sets `VERO_PROFILE`.

The kit has the same shape for the other things that differ by system:
notifications, opening at sign-in, keeping the machine awake, checking
for updates, and finding helper programs the app ships. Each is one
package with one API, listed in [the kit](../kit/README.md).

## 6. Check it builds for every system

```bash
cd .. && vero ship --plan
```

You should see:

```
jotter on this Mac:
  the worker builds  every one of the 14 systems and processors it is packaged for
  builds             deb, rpm, pacman, alpine, void, chimera, freebsd, dragonfly, netbsd, illumos, openbsd, macos, windows
  tests and records  macos, debian, ubuntu, fedora, opensuse, arch, alpine, chimera, void, freebsd, netbsd, openbsd, dragonfly, illumos, windows
```

The worker is built without cgo for every system, which is what lets one
Mac build for all of them. If a line says the worker fails for a system,
the cause is nearly always a Go package that needs cgo, or that has no
port for that system. The fixes, in order of how often they are needed:

- For SQLite, use `github.com/ncruces/go-sqlite3`, which builds
  everywhere. `mattn/go-sqlite3` needs cgo, and `modernc.org/sqlite` has
  no port for DragonFly, illumos, NetBSD on ARM64 or OpenBSD on RISC-V.
- For anything that differs by system, use the kit, as in step 5.
- For a system call whose shape differs, use `golang.org/x/sys` instead
  of `syscall`.
- For code that only one system has, give it a build tag and write a
  version for the others that does nothing.

## 7. Describe the app

Open `vero-app.toml`. `vero new` filled it in with placeholders. Change
these before anyone else sees the app:

- `display_name`, what people see in menus and on the install page.
- `summary` and `description`, one line and a paragraph for package
  managers and software centres.
- `publisher`, as `"Name <email>"`.
- `icon`, a square PNG, 512 or 1024 pixels. The one there is vero's.

The rest can wait. [example/vero-app.toml](../example/vero-app.toml) has
a comment on every setting, for when you need one.

## If your Mac app has an Xcode project

Keep it. In Xcode, add the package `https://github.com/imclaren/vero`
with the rule **Up to Next Major Version** and link its `Vero` product.
In `vero-app.toml`, point `[macos]` at the project with `folder`,
`project` and `scheme` instead of `product`. Start the worker with
`VeroModel<Status>(bundledWorker: "jotter-worker", directoryName: "jotter/bin")`
and call `VeroSingleCopy.yieldToRunningCopy()` in the app's `init`, as
`App.swift` above does. `vero gen` then writes `WorkerAPI.swift` beside
the scheme's sources, or wherever `swift` under `[gen]` says.

---

Next: [Test the app](test-the-app.md).
