# How vero works


This page explains the pieces of a vero app and the words the rest of
the documentation uses. Nothing on it is needed to follow the
walkthroughs, which start on the [front page](../README.md). Read it
when you want to know why.

On this page:

1. [The idea](#the-idea)
2. [The parts at a glance](#the-parts-at-a-glance)
3. [The state](#the-state)
4. [Requests and handlers](#requests-and-handlers)
5. [Generated bindings](#generated-bindings)
6. [Starting, stopping and restarting](#starting-stopping-and-restarting)
7. [The files in an app](#the-files-in-an-app)
8. [vero-app.toml](#vero-apptoml)
9. [The kit](#the-kit)
10. [Testing on every system](#testing-on-every-system)
11. [Releasing](#releasing)
12. [The vero command](#the-vero-command)
13. [Glossary](#glossary)

## The idea

A desktop app has two jobs. One is the real work: syncing files, talking to
a server, indexing a library, whatever the app is for. The other is showing
that work to a person in a window that looks and feels right on their
system.

vero splits those two jobs into two programs.

- **The worker** does the real work. It is a Go program, and it is the same
  program on every system. You write it once.
- **The front end** draws the window. There is one for each system, written
  in that system's own toolkit: SwiftUI on the Mac, GTK 4 (in Python) on
  Linux and the BSDs, and WPF (in C#) on Windows. `vero new` writes a
  starter for each, and you adapt it.

The front end starts the worker as a child process and talks to it over a
pipe. Everything that crosses the pipe is JSON, one message per line. The
front end never links against Go, and the worker never touches a toolkit.
That is the whole architecture.

```
   ┌───────────────────┐     JSON over a pipe     ┌───────────────────┐
   │  Front end        │ ───── requests ────────> │  Worker           │
   │  SwiftUI / GTK /  │ <──── replies ────────── │  Go, no cgo       │
   │  WPF              │ <──── state events ───── │  one for all OSes │
   └───────────────────┘                          └───────────────────┘
```

Because the worker is plain Go with no C dependencies, it cross-compiles
from a Mac to every system Go supports. vero uses that to build, test and
release the whole app from one Mac. Linux is tested in Docker containers,
the BSDs and Windows in virtual machines, and the Mac on itself.

## The parts at a glance

Five things make up a vero app. Each has a section below.

| Part | What it is | Where it lives |
|---|---|---|
| The worker | A Go program that does the app's work | `cmd/worker` |
| The state | One Go struct holding everything the window shows | in the worker |
| A request | A named message from the window to the worker, answered by a handler | in the worker |
| A binding | Generated code giving each front end typed access to the state and the requests | one file per front end |
| A front end | The native window for one system | `macos/`, `gtk/`, `windows/` |

Only three kinds of message cross the pipe between a front end and the
worker. The front end sends **requests**. The worker sends a **reply** to
each request, and sends a **state event** on its own whenever its state
changes.

## The state

The worker keeps everything the window needs to draw in one Go struct. In
the starter app it is called `Status` and looks like this:

```go
type Status struct {
    Jobs    []Job  `json:"jobs"`
    Working bool   `json:"working"`
    Since   string `json:"since"`
}
```

You hand that struct to `vero.NewState`. From then on, vero watches it, and
each time its JSON changes it sends the whole state to every window. The
front end never asks for it and never polls. It just redraws when a state
event arrives.

Because the state is shared between your code and vero's encoder, it sits
behind a lock. You change it only inside `state.Do`, which takes the lock
for you:

```go
state.Do(func(s *Status) { s.Jobs[0].Progress = 50 })
```

Slow work, such as a network call, must not happen while the lock is held.
Do the slow work in a goroutine of your own, then write the result back
with `state.Do`. `state.Every` runs a function under the lock on a timer,
for periodic work.

## Requests and handlers

A request is a message with a name and, optionally, some data. The worker
answers it with a **handler**, a Go function you register under that name.
There are four ways to register one, and the difference is what the
handler takes and what it replies with.

| Function           | Takes                           | Replies with   |
|--------------------|---------------------------------|----------------|
| `vero.Update`      | nothing                         | the new state  |
| `vero.UpdateWith`  | a struct of your own            | the new state  |
| `vero.UpdateItem`  | the key of one item in a list   | the new state  |
| `vero.Handle`      | a struct of your own            | a value of your own |

The first three run under the state's lock and reply with the state as it
is after the handler ran. That means the window can never show a stale
state after its own action. The last, `vero.Handle`, is for a request that
returns something that is not the state, such as a page of search results.
It gets a context that is cancelled when the front end stops waiting, so a
slow handler should watch `ctx.Done()`.

If a handler returns an error, the error's message goes back to the front
end and nothing else changes. The front end shows it and carries on. This
is different from the worker crashing, which the front end also notices, but
handles by waiting for a restart.

## Generated bindings

You do not write the front end's side of the protocol by hand. After you
change the state struct or any handler, run `vero gen`. It reads the Go
types and writes a typed binding for each front end:

- `macos/Sources/<App>/WorkerAPI.swift`
- `gtk/worker_api.py`
- `windows/WorkerApi.cs`

Each one has a type for the state, a type for each request, and a method
or struct for calling it. Never edit these files. `vero gen --check` tells
you when they are out of date.

## Starting, stopping and restarting

The front end does not run the worker directly. It runs a **supervisor**,
which starts the worker, restarts it with a backoff if it crashes, and
tells the front end whether the worker is starting, running, or gone. On
the Mac, the supervisor is part of vero's Swift package. In the GTK and WPF
front ends, the worker binary itself acts as the supervisor: started with
`VERO_HOST` set, it launches a second copy of itself to do the work and
relays messages to it. Either way, the front end only ever opens one pipe.

A worker can also hold a **single-copy lock**, so that two windows, or a
window and a background daemon, cannot both run the same worker for the
same user. The second one is told that another is running.

On iOS and in a browser, an app may not start another program at all. There,
the worker is compiled into the app and runs on a goroutine, with an
in-memory pipe in place of the real one. Nothing above the pipe changes.
Those targets live in [the lab](lab.md).

## The files in an app

`vero new jotter` makes this:

```
jotter/
├── cmd/worker/main.go      the worker: state, handlers, and the real work
├── go.mod
├── vero-app.toml           describes the app to the packaging and the tests
├── icon.png
├── macos/                  SwiftUI front end (a Swift package)
│   ├── Sources/Jotter/App.swift
│   ├── Sources/Jotter/WorkerAPI.swift     generated
│   └── build.sh
├── gtk/                    GTK 4 front end, in Python
│   ├── main.py
│   ├── vero.py, vero_gtk.py              vero's library, copied in
│   ├── worker_api.py                     generated
│   └── build.sh
├── windows/                WPF front end, in C#
│   ├── MainWindow.xaml, MainWindow.xaml.cs, App.xaml, App.xaml.cs
│   ├── Vero.cs, VeroWpf.cs               vero's library, copied in
│   ├── WorkerApi.cs                      generated
│   └── Jotter.csproj
├── testdata/               files that the tests use
├── PORTING.md              a checklist of what to finish
├── AGENTS.md               rules for coding agents working on the app
└── .vero-files.json        which files vero wrote, so vero upgrade can tell
```

Three kinds of file live in an app, and it matters which is which.

1. **Your code.** The worker, the front ends' windows, and `vero-app.toml`.
2. **vero's library, copied in.** `vero.py`, `vero_gtk.py`, `Vero.cs`,
   `VeroWpf.cs`, `AGENTS.md`. Do not edit these. `vero upgrade` replaces
   them with each new version of vero and refuses to overwrite one you have
   changed.
3. **Generated code.** `WorkerAPI.swift`, `worker_api.py`, `WorkerApi.cs`.
   Do not edit these either. Run `vero gen` after changing the worker.

## vero-app.toml

This file describes the app to the packaging and to the tests. The
important parts are:

- The app's name, bundle identifier, publisher, licence, icon and, once you
  have one, the address of its install site.
- `[worker]`, which names the worker's package and the name of its binary.
- One section per front end: `[macos]`, `[gtk]` and `[wpf]`. A system is
  packaged when its section is present. Under `[gtk]`, a subsection such as
  `[gtk.freebsd]` or `[gtk.alpine]` adds that system to the release.
- `[test]` and `[[test.step]]`, which say what the automated test does
  once the app's window is open.

[example/vero-app.toml](../example/vero-app.toml) has a comment on each
setting.

## The kit

The worker is plain Go and runs the same everywhere, but some things are
different on each system: where secrets are kept, how to show a
notification, how to stop the machine sleeping. The **kit** is a separate Go
module, `github.com/imclaren/vero/kit`, with one package for each of these.
Each package has one API and does the right thing on every system. The
[kit README](../kit/README.md) lists them.

Two kit packages matter from the start:

- `kit/profile` keeps a development or test copy of the app apart from the
  real one. When `VERO_PROFILE` is set to a name, the app's files, settings
  and secrets all carry that name, so a test run can never touch your own
  data. Every development copy should run with it set.
- `kit/keychain` stores secrets in the system's own keychain.

## Testing on every system

vero's tests install the app the way a user would, from the app's own
install site, and then play the steps in `[[test.step]]`. A step can call a
request, wait for a value in the state, copy a file in, or pause. The test
runs on each system in turn:

- The Mac, on itself.
- Each Linux distribution, in a Docker container.
- Each BSD, in a qemu virtual machine.
- Windows, in a qemu virtual machine.
- DragonFly and illumos, in emulated virtual machines, which are slow.

Each system starts from a **ready-made copy**: a container image or a VM
snapshot with the app's dependencies already installed, so a test does not
start from a bare system every time. The full run also records each
window as a GIF, which the install page shows.

[Test the app](test-the-app.md) walks through it.

## Releasing

`vero release` builds everything and writes it to `dist/site`. That folder
is a complete install site: an `index.html` that sends each visitor to the
right download, an installer for each system, package repositories for the
Linux and BSD package managers, a Sparkle appcast and a `latest.json` for
updates, and a Homebrew cask and winget manifest. Copy the folder to any
static web host, or give `--upload user@host:/path` and vero copies it for
you.

`vero ship` is `vero release` with the full test run in front of it. It
stops at the first system that fails.

The first release makes a signing key. Back it up. Installed copies check
updates against that key, and a lost key means they never update again.

[Ship the app](ship-the-app.md) walks through it, and lists what each
system's users get.

## The vero command

`vero` with no arguments prints every command and its flags, and
`vero COMMAND --help` prints one command's.

| Command | What it does |
|---|---|
| `vero setup` | Installs what vero needs on this Mac, and says what is still missing. |
| `vero new NAME` | Makes a new app: a Go module with an example worker and a front end for each desktop system. |
| `vero add [front ends]` | Adds front ends to an app that already has a worker in `cmd/worker`. |
| `vero gen` | Writes each front end's typed binding from the worker's Go types. `--check` fails when one is stale. |
| `vero ship` | Tests and records the app on every system this Mac can run, then releases it. `--plan` says what it would do. `--no-release` stops before releasing. |
| `vero release` | Builds and packages a release without testing. |
| `vero install` | Builds the Mac app from the working copy and installs it, numbered just above the current release. |
| `vero upgrade` | Moves an app to this version of vero. |
| `vero credentials` | Says what is set up for signing and the stores. |
| `vero publish` | Publishes a release to Homebrew, winget or the AUR. |

`vero key`, `package`, `build`, `check`, `steps`, `show` and `plan` belong
to [vero-repo](../cmd/vero-repo/README.md), the packaging tool, and
`vero` passes them on. `--vero DIR`, or `VERO_DIR` for `vero setup` and
`vero ship`, uses a checkout of vero in place of the published module.

## Glossary

- **Worker.** The Go program that does the app's work. One for all systems.
- **Front end.** The native window for one system. SwiftUI, GTK or WPF.
- **State.** The one Go struct that holds everything the window shows.
  vero pushes it to the front end whenever it changes.
- **Request.** A named message from the front end to the worker.
- **Handler.** The Go function that answers a request.
- **Reply.** The worker's answer to a request. Usually the new state.
- **Event.** A message the worker sends without being asked. Today the
  only kind is a state event.
- **Supervisor.** What starts the worker, restarts it, and reports whether
  it is running.
- **Host mode.** The worker binary acting as its own supervisor, which is
  how the Python and C# front ends run it.
- **Binding.** The generated file that gives a front end typed access to
  the state and the requests.
- **Kit.** The Go module of per-system helpers: secrets, notifications,
  sleep, autostart, updates.
- **Profile.** A name, set in `VERO_PROFILE`, that keeps one copy of the
  app's data apart from another.
- **Site.** The folder of installers and repositories that a release
  produces, served as plain files.
- **Ready-made copy.** A container image or VM snapshot with an app's
  dependencies installed, from which a test starts.
- **Step.** One line of the automated test: call a request, wait for a
  value, copy a file, or pause.
- **Recording.** The GIF of a test run on one system, shown on the install
  page.
- **The lab.** The targets beyond the desktop: Android, iOS, the browser,
  WASI and Plan 9. They build and run, but `vero ship` does not test them.
- **System.** An operating system, or a distribution of one. vero treats
  Debian, Fedora and FreeBSD as three systems.

