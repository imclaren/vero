# Build, run and test on every system

This page runs an app on every system from your Mac, in containers and
virtual machines, and tests it there. Each test installs the app as its
users would, plays the steps that you give it, and records the window at
work.

## Run the example everywhere

From a checkout of vero, [`scripts/run.sh`](../scripts/run.sh) builds the
example and opens it on three systems side by side, natively on macOS, in
a Linux container shown over VNC, and in a Windows virtual machine. The
first time, it runs `scripts/setup.sh`, and `--iso` gives it the Windows
ISO to install.

Each other system has a script of its own, which the
[README's table](../README.md) names. Each one makes its virtual machine
the first time, and most take the same flags:

- `--record out.gif` records the app instead of showing it;
- `--no-open` starts the app but opens no viewer;
- `--shell` opens a shell in the virtual machine;
- `--reset` throws the virtual machine away and starts again.

`scripts/run-windows.sh --ssh 2222` lets scripts reach the Windows virtual
machine over SSH, as [its notes](../.windows/unattend/README.md) describe.

## Run your own app

The README of each front end that `vero add` wrote says how to build and
run it. Two of vero's scripts also run an app of your own.

- [`scripts/run-linux.sh`](../scripts/run-linux.sh) runs a GTK app in a
  Debian container and shows it on your Mac, with `--app` for the app's
  folder, `--entry` for its Python file, and `--worker` and
  `--worker-name` for its worker. Its header lists the other options.
- [`scripts/run-windows.sh --payload DIR`](../scripts/run-windows.sh)
  boots the Windows virtual machine with `DIR` on a disc.

## Build every target

[`scripts/build-all.sh`](../scripts/build-all.sh) builds the worker for
every target in bold in the [README's table](../README.md), other than
`android/arm64`, `ios/arm64` and `js/wasm`, into `dist/`. Those three have
their own build scripts,
[`example/android-app/build.sh`](../example/android-app/build.sh),
[`example/ios-app/build.sh`](../example/ios-app/build.sh) and
[`example/web-app/build.sh`](../example/web-app/build.sh), because they do
not build a worker that runs on its own. `TARGETS` names others, as in
`TARGETS=plan9/386 ./scripts/build-all.sh`.

From your own app's folder, the same script builds your worker.

```bash
WORKER=./cmd/worker CSHIM=none WPF_APP=none LDFLAGS="-s -w -X main.version=1.2.3" \
    TARGETS="windows/amd64 linux/amd64" path/to/vero/scripts/build-all.sh
```

The targets marked † in the table need a C toolchain, which is an NDK for
the three Androids that are not `arm64`, or the Simulator SDK for
`ios/amd64`. The other 43 targets need only Go.

## Test on every system

[`scripts/test-all.sh`](../scripts/test-all.sh) tests an app on every
system at once, a few at a time. On each, it installs the app from a site
that it builds, starts it, plays its test steps, records it as a GIF and a
picture, and checks that the system's own updates bring the next version.
Then it shows every result on one page,
`~/.cache/vero/test-results/index.html`.

```bash
scripts/test-all.sh                              # vero's example, everywhere
scripts/test-all.sh --app path/to/vero-app.toml  # your app
```

`--only "debian fedora freebsd"` tests only those systems, `--skip
"illumos dragonfly"` tests all but those, `--no-vms` tests only in the
containers and the Flatpak, and `--parallel` says how many run at once,
three unless it is given. Each system's log is in
`~/.cache/vero/test-runs/`. `vero ship --no-release` runs the same tests
for an app, and records the Mac as well.

### One system at a time

[`scripts/test-repo.sh`](../scripts/test-repo.sh) tests one system. It
builds the site, serves it from your Mac, and installs the app on a clean
system, in the way that the site describes. It checks that the worker answers,
that the command, the menu entry and the front end are there, that Python
can load GTK 4, and that GTK 4 can read the app's icon. Then it starts the
installed app on a virtual display, plays the steps, and records the
window. Last, it releases the next version and checks that the system's
own updates bring it. Each stage is numbered, with the time so far.
Without options, it tests Debian in a container.

```bash
scripts/test-repo.sh --image ubuntu:24.04
scripts/test-repo.sh --image fedora:latest
scripts/test-repo.sh --image opensuse/tumbleweed
scripts/test-repo.sh --image archlinux
scripts/test-repo.sh --image alpine
scripts/test-repo.sh --image chimeralinux/chimera
scripts/test-repo.sh --image ghcr.io/void-linux/void-glibc
scripts/test-repo.sh --flatpak
scripts/test-repo.sh --vm freebsd         # also netbsd, openbsd, dragonfly, illumos
scripts/test-repo.sh --mac                # the disk image on this Mac, and the appcast after an update
scripts/test-repo.sh --vm windows         # the installer and the MSIX, in vero's Windows VM, over SSH
```

`--app path/to/vero-app.toml` tests your own app in the same way, on every
system but the Mac. `--no-launch` leaves out starting the app, and
`--limit` is how long its steps may take, 600 seconds unless it is given.
Two systems can be tested at once, each with its own `--port`.

The containers need Docker and colima. A `--vm` test uses the system's
virtual machine, which its `run-*.sh` script makes the first time. That
downloads the system and its GTK, which take a few gigabytes. The Windows
test reaches vero's Windows virtual machine over SSH. It installs, starts,
updates and uninstalls the installer, and, for an app with `[wpf.msix]`,
registers the MSIX's files in developer mode and starts the app. A virtual
machine made with `scripts/run-windows.sh --install` has SSH, and one made
before that gets it with `scripts/setup-windows-ssh.sh`.

### The Mac

[`scripts/record-mac.sh`](../scripts/record-mac.sh) records an app that
you have built on this Mac.

```bash
scripts/record-mac.sh --app vero-app.toml --bundle My.app
```

It starts the app as you, so the copy that it starts must not touch the
copy that you use. Give the test copy its own settings through
`[test] env`, build it with its own bundle ID
(`PRODUCT_BUNDLE_IDENTIFIER=...`), and check both before recording.
`--clean` removes what the last recording left under `~/Library`.
Capturing a window needs Screen Recording permission for the terminal.
[kit/profile](../kit/README.md) keeps a test copy's settings and secrets
apart from the copy in use.

## Steps, and a recording of the app at work

`[[test.step]]` entries in `vero-app.toml` say what a test does with the
app once its window is open. vero's library in the app plays them, in
Python, C# and Swift alike, so that the window shows each one as a person
would see it. `vero add` writes a starter `[test]` with examples from your
worker.

```toml
[test]
files = ["testdata/sample.mp3"]          # copied onto the system, as {files}
env = { MYAPP_PROFILE = "test" }         # set for the app as it starts

[[test.step]]
call = "setLibrary"                      # a request, by name
with = { path = "{tmp}" }                # {tmp} is a new empty folder

[[test.step]]
copy = { from = "sample.mp3", to = "{tmp}/imports/" }

[[test.step]]
wait = { path = "stats.books", at_least = 1 }   # or is, not, contains
timeout = 300

[[test.step]]
pause = 3                                # so that the recording shows it
```

A wait's `path` goes through the app's state by key and by list index, and
`#` is a list's length, as in `jobs.#` and `jobs.0.phase`. The test stops
at the first step that fails. Each system's GIF, steps and log go in
`~/.cache/vero/test-results/APP/SYSTEM/`. A release puts the GIF of each
system that passed on the install page, under "See it at work".

## What has not been installed

Some packages have been checked only by vero's own checks, and not
installed on the system for which they are made. These are Debian's armel,
openSUSE's armv7hl and armv6hl, Alpine's armhf, Void's armv6l, Arch Linux
32's pentium4, Arch POWER's, FreeBSD's armv7, and Chimera's on anything
but ARM64, which `test-repo.sh` installs in Chimera's container.

Next, [package and release](release.md).
