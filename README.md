# vero

**Go backend with native cross-platform GUI frontends.  Build on macOS, run anywhere Go runs.**

Run a Go binary embedded in a native macOS SwiftUI app. The Go binary and the SwiftUI app communicate over a pipe.  

Use the same Go backend to communicate over a pipe with native frontends for:
- macOS
- Windows
- Linux
- *BSD (FreeBSD, OpenBSD, NetBSD, DragonFly and illumos)

[![Go reference](https://pkg.go.dev/badge/github.com/imclaren/vero.svg)](https://pkg.go.dev/github.com/imclaren/vero)

<table>
<tr>
<td align="center" valign="top" width="25%"><a href="#run-the-macos-example"><img src="docs/screenshots/macos.gif" width="100%"></a><br><sub><b>macOS</b> — SwiftUI, in the menu bar</sub></td>
<td align="center" valign="top" width="25%"><a href="example/wpf-app"><img src="docs/screenshots/windows.gif" width="100%"></a><br><sub><b>Windows</b> — WPF</sub></td>
<td align="center" valign="top" width="25%"><a href="example/gtk-app"><img src="docs/screenshots/linux.gif" width="100%"></a><br><sub><b>Linux</b> — GTK4</sub></td>
<td align="center" valign="top" width="25%"><a href="#build-the-same-worker-for-windows-linux-and-freebsd"><img src="docs/screenshots/freebsd.gif" width="100%"></a><br><sub><b>FreeBSD</b> — GTK4, unchanged</sub></td>
</tr>
</table>

[macOS example](#run-the-macos-example) ·
[Windows, Linux and FreeBSD examples](#build-the-same-worker-for-windows-linux-and-freebsd) ·
[What runs where](#what-runs-where)

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
thirteen targets across macOS, Windows, Linux, FreeBSD, OpenBSD, NetBSD,
DragonFly and illumos. Anywhere Go produces an executable, vero runs - the Go
API needs nothing else, and the Python and C# bindings need only the worker.

## What runs where

vero is two programs: the Go **worker**, and the native app that drives it.
They are joined in one of two ways.

- **Compiled in.** The Go code is built as a C archive and linked into the
  app, so both halves are one executable. This is what the Swift package does
  on macOS, and it needs a platform Go can build a C archive for.
- **Started as a process.** The app runs the worker as an ordinary program and
  talks to it over a pipe. The worker supervises itself - it launches a second
  copy to do the work and restarts it if it dies - so the app only has to
  write and read JSON. This is what the Python and C# bindings do, and it
  needs nothing but a worker binary.

The second way is why the list is long: Go builds an executable for 35 of its
ports, while the C library needed by the first way exists on a handful.

| Operating system | Can you build the worker? | Is there a working example? | What that means in practice |
|---|---|---|---|
| macOS | yes - arm64, amd64 | yes, SwiftUI in the menu bar | the only platform where Go is compiled into the app rather than started beside it |
| Windows | yes - arm64, amd64, 386 | yes, WPF | the C# app starts `worker.exe`; nothing is loaded, so no DLL to ship or match to the architecture |
| Linux | yes - 13 architectures | yes, GTK4 | the Python app starts the worker; releases carry amd64 and arm64 |
| FreeBSD | yes - amd64, arm64 | yes, the Linux GTK4 example unchanged | proof that starting the worker is enough: Go cannot build its C library here at all |
| OpenBSD | yes - 6 architectures | not written | a GTK or Qt app would work the same way; nothing in vero is Linux-specific |
| NetBSD | yes - 4 architectures | not written | as above |
| DragonFly | yes - amd64 | not written | as above |
| illumos | yes - amd64 | not written | as above |
| Android | yes - arm64 | not written | a Kotlin app could start the worker from its library directory, with no JNI |
| Solaris, AIX, Plan 9 | no | - | vero's single-worker lock uses `flock`, which these do not provide. About thirty lines with `fcntl` would fix it |
| iOS | no | - | the sandbox forbids starting another process, so an iOS app would have to compile the worker in and lose the supervision that comes with running it separately |
| wasm | no | - | there are no processes at all, so neither way of joining the halves exists |

## Licence

MIT
