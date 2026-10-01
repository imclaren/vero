# vero

**Go backend with native cross-platform GUI frontends.  Build on macOS, run anywhere Go runs.**

Run a Go binary embedded in a native macOS SwiftUI app. The Go binary and the SwiftUI app communicate over a pipe.  

[![Go reference](https://pkg.go.dev/badge/github.com/imclaren/vero.svg)](https://pkg.go.dev/github.com/imclaren/vero)

| Operating system | Builds on a Mac | Example | Build details |
|---|---|:---:|---|
| **macOS** — arm64, amd64 | yes | <a href="#run-the-macos-example"><img src="docs/screenshots/macos.gif" width="300"></a> | SwiftUI, in the menu bar. |
| **Windows** — arm64, amd64, 386 | yes | <a href="example/wpf-app"><img src="docs/screenshots/windows.gif" width="300"></a> | WPF, run in a Windows VM by `scripts/run-windows.sh` |
| **Linux** — 13 architectures | yes | <a href="example/gtk-app"><img src="docs/screenshots/linux.gif" width="300"></a> | GTK4, run in a Docker container by `scripts/run-linux.sh` |
| **FreeBSD** — amd64, arm64 | yes | <a href="example/gtk-app"><img src="docs/screenshots/freebsd.gif" width="300"></a> | GTK4, the same app as Linux, run in a VM by `scripts/run-freebsd.sh` |
| **OpenBSD** — 6 architectures | yes | — | The GTK4 app should run unchanged. There is no script for it because OpenBSD publishes an installer rather than a ready-made disk image, so it would need an unattended install driven by an `auto_install` response file bot build and display the UI. |
| **NetBSD** — 4 architectures | yes | — | As OpenBSD, but easier: NetBSD does publish a bootable arm64 image, so a script would download it, enable `sshd`, and install GTK4 with `pkgin`. |
| **DragonFly** — amd64 | yes | — | No script, because DragonFly runs on x86 only. An x86 VM is emulated rather than virtualised on an Apple Silicon Mac, which would take minutes-per-boot |
| **illumos** — amd64 | yes | — | As DragonFly: x86 only, and slow to emulate |
| **Android** — arm64 | yes | — | No UI app written. The worker would ship in `jniLibs` and be started from `nativeLibraryDir`, with no JNI: Kotlin would write and read JSON, exactly as Python and C# do |
| **Solaris, AIX, Plan 9** | yes | — | The worker builds but is untested.  Plan 9 can be run on a Mac, under qemu |
| **iOS** | the Go side does | — | iOS will not let an app start another program, so the worker cannot run beside the app as it does everywhere else. Compiling it into the app does work - the archive builds for iOS today - but vero has no mode for that yet: the worker and the supervisor would have to talk over an in-memory pipe, and restarting would mean nothing, because a worker on a goroutine dies only when the app does |
| **wasm** | yes, once it has a lock | — | Two answers, because there are two targets. Under WASI the worker needs nothing else: WASI gives it standard input and output, which is what it speaks, and the supervisor already starts whatever path and arguments it is given, so `wasmtime run worker.wasm` works. In a browser there is no process to start and no standard input to speak over, so the pipe would have to become a Web Worker and `postMessage` - a transport change rather than a lock change. Neither is written yet, and the lock itself is three lines: nothing else can hold it, so it can do nothing |

[macOS example](#run-the-macos-example) ·
[Windows, Linux and FreeBSD examples](#build-the-same-worker-for-windows-linux-and-freebsd)

If you already have Xcode and Go installed, the example below builds a running
app in about 5 minutes.

## Run the macOS example

The example is a menu bar app driving the Go worker (main.go below):

```bash
git clone https://github.com/imclaren/vero && cd vero
cd example/menubar-app && ./build.sh
./.build/debug/MenuBarExample
```

Look for the icon in the menu bar. The source is available at
[example/menubar-app](example/menubar-app).

## Create a vero macOS app on your Mac

### 1. Build the go worker

Get vero:

```bash
mkdir -p ~/vero-example/macos-app && cd ~/vero-example/macos-app
go mod init macos-app
go get github.com/imclaren/vero
```

Create `main.go` (file also available at
[example/worker/main.go](example/worker/main.go)):

```go
// Command worker is the logic half of the example: the part you would write.
//
// The flags come from vero.WorkerOptions: -json puts the event stream on
// stdout, -quiet silences the log, and -version is what the frontend asks
// before replacing the copy of this it has on disk.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"slices"
	"time"

	"github.com/imclaren/vero"
)

// version has to increase on every release, so the frontend can tell which
// of two copies is newer.
var version = "0.11.0"

type Job struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Progress int    `json:"progress"`
}

// Key is how an id in a request finds one job.
func (j Job) Key() int { return j.ID }

func (j *Job) Restart() { j.Phase, j.Progress = "waiting", 0 }

type Status struct {
	Jobs    []Job  `json:"jobs"`
	Working bool   `json:"working"`
	Since   string `json:"since"`
}

func working(s *Status) {
	s.Working = slices.ContainsFunc(s.Jobs, func(j Job) bool {
		return j.Phase != "waiting" && j.Phase != "done"
	})
}

func main() {
	var opts vero.WorkerOptions
	opts.Version = version
	opts.RegisterFlags(flag.CommandLine)
	flag.Parse()
	opts.PrintVersionAndExit()

	w := vero.NewWorker(opts)

	// The state.  vero watches it and sends it to the frontend whenever its
	// JSON changes, which is how the frontend tracks status without asking.
	// Each handler below replies with it as well.
	state := vero.NewState(w, Status{
		Jobs: []Job{
			{ID: 1, Name: "Photos", Phase: "waiting"},
			{ID: 2, Name: "Documents", Phase: "waiting"},
			{ID: 3, Name: "Team share", Phase: "waiting"},
		},
		Since: time.Now().Format("15:04:05"),
	})

	go work(w, state)

	// Update handles a request with no accompanying data.  "status" returns
	// the state.
	vero.Update(state, "status", func(*Status) error { return nil })

	// UpdateWith handles a request with accompanying data.  "restartJob"
	// takes the id of the job to restart.
	vero.UpdateWith(state, "restartJob", func(s *Status, req vero.ID[int]) error {
		if err := vero.Edit(s.Jobs, req.ID, (*Job).Restart); err != nil {
			return fmt.Errorf("no job with id %d", req.ID)
		}
		working(s)
		return nil
	})

	if err := w.Serve(); err != nil {
		w.Log("stopped: %v", err)
	}
}

// work is the business logic - we have added random state changes to
// demonstrate how this works.
func work(w *vero.Worker, state *vero.State[Status]) {
	phases := []string{"looking for changes", "scanning", "uploading", "done"}
	for {
		time.Sleep(time.Duration(200+rand.Intn(400)) * time.Millisecond)

		var name, phase, before string
		state.Do(func(s *Status) {
			j := &s.Jobs[rand.Intn(len(s.Jobs))]
			before = j.Phase
			switch i := slices.Index(phases, j.Phase); {
			case j.Phase == "waiting" || j.Phase == "done":
				j.Phase, j.Progress = phases[0], 0
			case j.Progress < 100:
				j.Progress = min(j.Progress+20+rand.Intn(30), 100)
			case i+1 < len(phases):
				j.Phase, j.Progress = phases[i+1], 0
			}
			name, phase = j.Name, j.Phase
			working(s)
		})

		if phase != before {
			w.Log("%s: %s", name, phase)
		}
	}
}
```

Build it for both architectures (old Intel Macs and new Apple Silicon Macs) and join them into one binary, so the app runs
on either. `lipo` comes with Xcode:

```bash
GOOS=darwin GOARCH=amd64 go build -o worker-amd64 . && \
GOOS=darwin GOARCH=arm64 go build -o worker-arm64 . && \
lipo -create worker-amd64 worker-arm64 -output worker
```

### 2. Create a new macOS SwiftUI Xcode project

In Xcode, File -> New -> Project -> macOS -> App, with Interface set to
SwiftUI. Then:

- File -> Add Package Dependencies... -> `https://github.com/imclaren/vero`
- drag `~/vero-example/macos-app/worker` into the project, ticking your app
  under **Add to targets**

### 3. Add the app to the project

Create `ExampleApp.swift` (file also available at
[example/menubar-app/Sources/MenuBarExample/ExampleApp.swift](example/menubar-app/Sources/MenuBarExample/ExampleApp.swift)):

```swift
import SwiftUI
import Vero

// What the worker pushes: the same shape as Status and Job in main.go.
struct Job: Decodable, Identifiable {
    let id: Int
    let name: String
    let phase: String
    let progress: Int
}

struct Status: Decodable {
    let jobs: [Job]
    let working: Bool
    let since: String
}

// One of these per handler on the worker. The name routes the request -
// "restartJob" is the vero.UpdateWith above - and Reply says what comes back,
// so nothing has to guess at it.
struct RestartJob: NamedRequest {
    static let name = "restartJob"
    typealias Reply = Status
    let id: Int
}

@main
struct ExampleApp: App {
    // The worker, the last state it pushed, and the reason it could not start
    // if it did not: everything a view needs to draw, created once, here.
    @StateObject private var vero = VeroModel<Status>(
        bundledWorker: "worker", directoryName: "Example/bin")

    var body: some Scene {
        MenuBarExtra {
            VStack(spacing: 10) {
                ForEach(vero.state?.jobs ?? []) { job in
                    HStack(spacing: 10) {
                        // The press. This goes to the "restartJob" handler in
                        // main.go. There is no reply to deal with here,
                        // because the worker pushes the new state, and that
                        // is what redraws the row.
                        Button("Restart") { vero.call(RestartJob(id: job.id)) }
                        Text(job.name)
                        Text(job.phase).foregroundStyle(.secondary)
                        ProgressView(value: Double(job.progress) / 100)
                    }
                }
            }
            .frame(width: 460)
            .padding(16)
        } label: {
            // Pushed as well, so the icon spins while the worker is busy
            // without anything here asking it whether it is.
            Image(systemName: vero.state?.working == true
                  ? "arrow.triangle.2.circlepath"
                  : "checkmark.circle")
        }
        .menuBarExtraStyle(.window)
    }
}
```

Delete the `ContentView.swift` and `<YourApp>App.swift` that Xcode generated:
`ExampleApp.swift` is the `@main` entry point.

### 4. Run it

Press Cmd-R in Xcode. The icon appears in the menu bar, and spins while the
worker has jobs in flight. Click the menu bar icon to see progress changes.
Click the button on a row to restart that job.

## Build the same worker for Windows, Linux and FreeBSD

All of them can be built on your Mac, with no C toolchain: the worker is pure
Go, and the frontend spawns it rather than loading a library.

| | | |
|---|---|---|
| [example/wpf-app](example/wpf-app) | Windows, WPF | `./scripts/run-windows.sh` |
| [example/gtk-app](example/gtk-app) | Linux, GTK4 | `./scripts/run-linux.sh` |
| [example/gtk-app](example/gtk-app) | FreeBSD, GTK4 - the same app, unchanged | `./scripts/run-freebsd.sh` |

To see the app running on three operating systems at once, `./scripts/setup.sh`
installs what is needed to run them and builds everything, and
`./scripts/run.sh` then opens macOS, Linux and Windows together on your Mac.
`./scripts/run-freebsd.sh` opens FreeBSD in a VM of its own.

`./scripts/build-all.sh` goes wider than the examples: it builds a worker for
sixteen targets across macOS, Windows, Linux, FreeBSD, OpenBSD, NetBSD,
DragonFly, illumos, Solaris, AIX and Plan 9. Anywhere Go produces an executable, vero runs - the Go
API needs nothing else, and the Python and C# bindings need only the worker.

## Licence

MIT
