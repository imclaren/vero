# Describe the app

`vero-app.toml` sits beside the app's `go.mod` and tells vero everything
it needs to package and test the app that is not in the code: the app's
name and identifier, where each front end is, which systems to package
for, and what the automated test does. `vero new` writes a first version.
[example/vero-app.toml](../example/vero-app.toml) has a comment on every
setting, and this page is the map.

Paths in the file are relative to the file. Nothing secret goes in it;
signing certificates and passwords come from environment variables, which
[Sign and publish](sign-and-publish.md) lists.

## The app

These top-level settings describe the app to package managers, software
centres and the install page.

| Setting | What it is |
|---|---|
| `name` | The package and command name: lower case, digits and hyphens. |
| `display_name` | What people see in menus and on the install page. |
| `id` | A reverse-domain identifier of your own, such as `com.example.hello`. |
| `summary`, `description` | One line, and a paragraph or two. |
| `publisher` | `"Name <email>"`. Installers show the name. |
| `homepage`, `licence` | A URL, and an SPDX identifier such as `MIT`. |
| `icon` | A square PNG, 512 or 1024 pixels. |
| `site` | The address where the install site will live. Leave it out while you only build the site to look at. |
| `tray`, `menubar` | `tray = true` adds a tray or menu bar icon to a window app. `menubar = true` makes an app that lives there. |
| `schemes`, `opens` | Link schemes and file extensions the app opens, which each installer registers. |

## The worker

```toml
[worker]
package = "cmd/worker"    # the Go main package
name = "hello-worker"     # the binary's name; the front ends start it by this
```

`cgo = true` builds the Mac's worker with cgo and every other system's
without, for the rare worker that needs it.

## The front ends

A system is packaged when its section is present.

- `[macos]` names the Swift package (`folder` and `product`) or the Xcode
  project (`folder`, `project` and `scheme`). It also holds the update
  feed, extra resources to put in the bundle, and `pkg = true` for an
  installer package as well as the disk image.
- `[gtk]` names the folder and the `entry` file. Debian, Fedora, openSUSE
  and Arch are always packaged from it. Each other system is packaged
  when its own subsection is present: `[gtk.alpine]`, `[gtk.void]`,
  `[gtk.chimera]`, `[gtk.flatpak]`, `[gtk.freebsd]`, `[gtk.dragonfly]`,
  `[gtk.netbsd]`, `[gtk.openbsd]` and `[gtk.illumos]`. An empty section
  is enough.
- `[wpf]` names the project folder and the `exe`. `[wpf.msix]` adds an
  MSIX for the Microsoft Store.
- `[web]`, `[wasi]`, `[android]`, `[ios]` and `[plan9]` are
  [the lab](lab.md).

## What the GTK app depends on

vero works out each system's package dependencies for you. Python,
PyGObject, GTK 4 and a font are always included. `uses` under `[gtk]`
adds the optional pieces that vero knows about:

```toml
[gtk]
uses = ["adwaita", "webview", "keyring"]   # libadwaita, a web view, a keyring
extra = ["ffmpeg"]                         # programs the worker runs
```

Anything else goes in the system's own section, such as `depends` under
`[gtk.deb]`.

## Processors

Each system is packaged for x86-64 and ARM64 where it has them. `arches`
in a system's section narrows or widens that. The
[vero-repo README](../cmd/vero-repo/README.md) lists each system's names
for its processors.

## What the app needs from the system

```toml
[needs]
network = true
files = true
keyring = true
notifications = false
autostart = false
webview = false
```

These are the permissions that sandboxed formats ask for. A Flatpak may
reach the network, the home folder and the keyring only when the matching
setting is true, and the Microsoft Store listing declares them too.

## The tests

`[test]` and `[[test.step]]` say what the automated test does once the
app's window is open. [Write test steps](write-test-steps.md) covers them.

## Where generated code goes

`vero gen` writes each front end's binding beside vero's own files in
that front end. `[gen]` overrides that, for an Xcode project whose
sources are elsewhere:

```toml
[gen]
swift = "MyApp/Sources/WorkerAPI.swift"
```
