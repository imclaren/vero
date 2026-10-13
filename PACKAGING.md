# vero-app.toml

This page lists the settings in `vero-app.toml`, the file that describes
an app to vero's packaging and tests. `vero new` and `vero add` write it
beside the app's `go.mod`, and [`example/vero-app.toml`](example/vero-app.toml)
has a comment on every setting. Paths in it are relative to the file.
[Package and release](docs/release.md) goes through the steps that use it.

## The app

| Setting | What it is |
|---|---|
| `name` | The package and command name, in lower case letters, digits and hyphens. |
| `display_name` | The name that people see, in menus and on the install page. |
| `id` | A reverse-domain name of your own, such as `com.example.myapp`, by which systems tell apps apart. |
| `version` | Usually given with `--version` instead, or worked out by `vero release`. |
| `summary` | One line about the app. |
| `description` | A paragraph or two, for package managers and software centres. A blank line starts a new paragraph. |
| `publisher` | Who publishes the app, as `"Name <email>"`. |
| `homepage` | The app's web page. |
| `site` | Where the install site will be. Leave it out to build a site for a look at it on your Mac. |
| `licence` | An SPDX licence identifier. |
| `icon` | A square PNG of 512 or 1024 pixels. |
| `tray`, `menubar` | A window app with a tray icon, or a tray app, as [Start an app](docs/start.md#a-window-app-a-tray-app-or-both) describes. |
| `schemes`, `opens` | The link schemes that the app opens, such as `"mailto"`, and the file types that it opens, by extension, such as `"eml"`. Each system's installer registers them. |

## The worker

`[worker]` names the worker's main `package` and the program's `name`,
to which `.exe` is added on Windows. The front ends start the worker from
beside themselves by this name. `cgo = true` builds the Mac's worker with
cgo, as [Write the worker](docs/worker.md#a-worker-for-every-system)
explains. The worker's main package needs a `var version`, which the
build sets.

## The front ends

Each front end has a section, and a system is packaged when its section is
there. [`vero-repo`](cmd/vero-repo/README.md) describes each section's
settings and what each system's users get.

| Section | Front end | Details |
|---|---|---|
| `[gtk]` | GTK, for Linux, the BSDs and illumos | [Linux, the BSDs and illumos](#linux-the-bsds-and-illumos), below |
| `[macos]` | SwiftUI, for macOS | [macOS](cmd/vero-repo/README.md#macos) |
| `[wpf]`, `[wpf.msix]` | WPF, for Windows and the Microsoft Store | [Windows](cmd/vero-repo/README.md#windows) |
| `[android]`, `[ios]` | Android, iPhone and iPad | [Android, iPhone and iPad](cmd/vero-repo/README.md#android-iphone-and-ipad) |
| `[web]`, `[wasi]`, `[plan9]` | A browser, a terminal with WASI, and Plan 9 | [The browser, WASI and Plan 9](cmd/vero-repo/README.md#the-browser-wasi-and-plan-9) |

### Linux, the BSDs and illumos

`[gtk]` names the app's `folder`, its `entry`, which is the Python file
that starts it, the files to `include` beside it, and its menu
`categories`.

Each system names its packages differently, so vero works them out from
what `[gtk]` says the app uses.

```toml
[gtk]
uses = ["adwaita", "webview", "keyring"]
extra = ["ffmpeg"]
```

Every app gets Python, PyGObject, GTK 4 and, where a system brings none, a
font. `uses` adds libadwaita, the web view and a keyring, which vero's GTK
library uses where they are installed. They are recommended where a
package manager can say so, and libadwaita is left out on each system and
processor that lacks it. `extra` names programs that vero knows, such as
ffmpeg. Anything else that the app needs goes in a system's own section,
such as `depends` in `[gtk.deb]`, and is added after the packages that vero chooses.
[cmd/vero-repo/deps.go](cmd/vero-repo/deps.go) has the names. An app
without `uses` or `extra` lists everything in each section itself.

Debian, Fedora, openSUSE and Arch are always packaged, with `[gtk.deb]`,
`[gtk.rpm]` and `[gtk.arch]` for their settings. Each other system is
packaged when its section is there, even an empty one. These are
`[gtk.alpine]`, `[gtk.void]`, `[gtk.chimera]`, `[gtk.flatpak]`,
`[gtk.freebsd]`, `[gtk.dragonfly]`, `[gtk.netbsd]`, `[gtk.illumos]` and
`[gtk.openbsd]`. A Flatpak is made only when `[gtk.flatpak]` has
`build = true`, or `--targets flatpak` asks for one, since it needs Docker. Each section's `arches` narrows or
widens the processors for which it is packaged, as
[`vero-repo`](cmd/vero-repo/README.md#processors) describes, and
`[[gtk.data]]` adds files to the desktop, such as a search provider, as
[the site's files](cmd/vero-repo/README.md#what-is-in-the-site) describe.

## What the app needs

`[needs]` says what the app asks of the system, with each of these set to
`true` or `false`:

- `network`;
- `files`;
- `keyring`;
- `notifications`;
- `autostart`;
- `webview`.

The Microsoft Store's package, for one, declares a startup task for
`autostart` and the internet for `network`.

## The tests

`[test]` and its `[[test.step]]` entries say what vero's tests do with
the app once its window is open. [Build, run and test on every
system](docs/test.md#steps-and-a-recording-of-the-app-at-work) describes
them.

## Secrets

Nothing secret goes in `vero-app.toml`. Signing certificates, passwords and
keystores are given in environment variables, which
[Package and release](docs/release.md#signing-stores-and-listings) lists.
