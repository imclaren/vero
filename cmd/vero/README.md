# The vero command

This page is the reference for the `vero` command, which makes an app,
writes its front ends, and builds, tests and releases it. The
[guides](../../docs/README.md) show the commands in the order in which a
developer uses them.

```bash
GOBIN=$(brew --prefix)/bin go install github.com/imclaren/vero/cmd/vero@latest
```

## The commands

| Command | What it does | Guide |
|---|---|---|
| `vero setup` | Installs what vero needs on this Mac, then says what is missing. | [Set up your Mac](../../docs/setup.md) |
| `vero new NAME` | Makes an app, a Go module with vero's example worker, and then runs `vero add` in it. | [Start an app](../../docs/start.md) |
| `vero add` | Writes a front end for each system, `vero-app.toml` and `PORTING.md`. | [The front ends](../../docs/front-ends.md) |
| `vero gen` | Writes each front end's requests from the worker's types, in Swift, C# and Python. | [Write the worker](../../docs/worker.md#the-front-ends-types) |
| `vero ship [FILE.go]` | Makes an app from a worker file, or takes the one in this folder, tests and records it everywhere, and releases it. | [Package and release](../../docs/release.md#vero-ship) |
| `vero release` | Builds the installers and the site. | [Package and release](../../docs/release.md#vero-release-step-by-step) |
| `vero credentials` | Says what is set up for signing and the stores, and how to set up the rest. | [Package and release](../../docs/release.md#signing-stores-and-listings) |
| `vero publish` | Publishes to Homebrew, winget or the AUR. | [Package and release](../../docs/release.md#signing-stores-and-listings) |
| `vero install` | Builds the Mac app from this folder and installs it, numbered just above the release that is out. | [Updates](../../docs/updates.md#trying-a-build-on-your-own-mac) |
| `vero local-version` | Prints that number, for a prebuild script that numbers Xcode's own builds in the same way. | [Updates](../../docs/updates.md#trying-a-build-on-your-own-mac) |
| `vero upgrade` | Brings an app made with vero 0.13 or later up to this vero, and says what to change in its own code. | [Updates](../../docs/updates.md#moving-an-app-to-a-newer-vero) |
| `vero key`, `package`, `build`, `check`, `steps`, `show`, `plan` | `vero-repo`'s own commands, which `vero` runs for you. | [vero-repo](../vero-repo/README.md#step-by-step) |

`release`, `credentials`, `publish` and the commands in the last row are
handed on to [vero-repo](../vero-repo/README.md), which `vero` installs
the first time that it needs it.

## Flags

```
vero setup [--vero DIR]
vero ship [FILE.go] [--name NAME] [--module PATH] [--plan] [--no-release] [--no-vms] [--no-mac]
          [--only LIST] [--skip LIST] [--version 1.2.3] [--notes TEXT] [--upload user@host:/path] [--url URL]
          [--vero DIR]
vero new NAME [front ends, as for add] [--module PATH] [--vero DIR] [add's flags]
vero add [desktop|mobile|all|macos|gtk|wpf|android|ios|web|wasi|plan9 ...] [--except LIST] [--dir DIR]
         [--menubar | --tray] [--summary TEXT] [--description TEXT] [--publisher "Name <email>"]
         [--keep-worker] [--force]
vero gen [--check] [--out DIR] [--dir DIR]
vero install [--published 1.2.3] [--ldflags FLAGS] [--dir DIR]
vero local-version [--published 1.2.3] [--dir DIR]
vero upgrade [--version v0.18.0] [--force] [--dir DIR]
vero release [--version 1.2.3] [--targets ...] [--notes "..."] [--upload user@host:/path]
vero credentials
vero publish --homebrew | --winget | --aur | --all
```

`--vero DIR`, or the `VERO_DIR` environment variable, uses a checkout of
vero in place of the published one. `--dir DIR` names the app's folder,
which is otherwise the current one.

## vero add

In the app's folder, the one with its `go.mod`, `vero add` writes the
front ends that are named.

```bash
vero add                 # the desktop: macOS, GTK (Linux, the BSDs, illumos) and WPF (Windows)
vero add mobile          # Android and iOS
vero add all             # everything for which vero has a front end
vero add all --except plan9,wasi
vero add windows android # any mix of groups and systems
```

It reads the worker and writes a starter for each system, as
[The front ends](../../docs/front-ends.md#the-starters) describes. The GTK
starter is Python, the Windows one C#, the macOS and iOS ones SwiftUI, the
Android one Kotlin, and the browser, WASI and Plan 9 ones Go. Front ends
that are already there are left alone, unless `--force` is given, and
each new one is added to `vero-app.toml`, which is made if there is none.
`vero add` then runs `vero gen`.

When it first writes `vero-app.toml` at a terminal, it asks for a one-line
summary, a description and the publisher, as `"Name <email>"`.
`--summary`, `--description` and `--publisher` give them from a script.
`--tray` and `--menubar` choose the shape of the app, as
[Start an app](../../docs/start.md#a-window-app-a-tray-app-or-both)
describes.

For iOS and the browser, it moves the worker's code into
`internal/worker`, as [Write the worker](../../docs/worker.md#a-worker-inside-the-app)
describes, and `--keep-worker` leaves it where it is.

It also writes `PORTING.md`, which lists what in the worker needs
attention on the new systems, with the files and lines, and what to do
next. [Write the worker](../../docs/worker.md#a-worker-for-every-system)
explains what it finds.

## vero upgrade

`vero upgrade` brings an app made with vero 0.13 or later up to this
vero. [Updates](../../docs/updates.md#moving-an-app-to-a-newer-vero)
describes what it changes. vero's files say at their top that vero wrote
them, and `.vero-files.json`, which belongs in git, records each one's
hash. A file that has been changed since is left alone and named, and
`--force` replaces it all the same.

## vero install

`vero install` builds the Mac app from the checkout, signs it, and puts it
in `/Applications`, numbered just above the newest release, as in
`0.5.0.202610121530`. It finds the release's number in a `latest.json` in
the app's folder, on a `releases` branch or at the app's site, and
`--published 1.2.3` gives it. `--ldflags` adds to the worker's build flags
what a release builds into it, such as an OAuth client's ID.
[Updates](../../docs/updates.md#trying-a-build-on-your-own-mac) explains
the numbering.
