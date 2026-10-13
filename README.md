# vero

**A Go worker with a native front end on every system. Build on a Mac, and run anywhere Go runs.**

A vero app has two halves. The worker is a Go program that keeps the app's
state and answers its requests. The front end is a native app for each
system, written in SwiftUI on the Mac, GTK on Linux, the BSDs and illumos,
and WPF on Windows. It starts the worker, shows the state that the worker
pushes, and sends it requests over a pipe. vero consists of the Go
library, a binding for each front end's language, and the `vero` command,
which writes the front ends, builds and tests the app on every system from one Mac, and packages
it with installers and a site from which people install and update it.

[![Go reference](https://pkg.go.dev/badge/github.com/imclaren/vero.svg)](https://pkg.go.dev/github.com/imclaren/vero)

[Quick start](#quick-start) ·
[Guides](#guides) ·
[Reference](#reference) ·
[All documentation](docs/README.md) ·
[Licence](#licence)

The table shows every system on which Go runs, what your Mac needs to
build and run vero's example there, and how it runs. Each picture links to
that system's example.

| Operating system | Example | Required on your Mac to build and run | Build and run on your Mac |
|---|:---:|---|---|
| **macOS**<br>**`darwin/amd64`**, **`darwin/arm64`** | <a href="example/menubar-app"><img src="docs/screenshots/macos.gif" width="300"></a> | Xcode, from the App Store. [`setup-macos.sh`](scripts/setup-macos.sh) checks that Xcode is installed | Builds and runs. SwiftUI, in the menu bar, built by running [`example/menubar-app/build.sh`](example/menubar-app/build.sh), as the [quick start](#quick-start) shows. |
| **Windows**<br>`windows/386`, **`windows/amd64`**, **`windows/arm64`** | <a href="example/wpf-app"><img src="docs/screenshots/windows.gif" width="300"></a> | qemu, the .NET SDK and a Windows 11 ARM64 ISO, which can be installed by running [`setup-windows.sh`](scripts/setup-windows.sh) | Builds and runs. WPF, started in a Windows VM by running [`scripts/run-windows.sh`](scripts/run-windows.sh), which with `--ssh` lets scripts reach it over SSH, as [the VM's notes](.windows/unattend/README.md) describe. |
| **Linux**<br>`linux/386`, **`linux/amd64`**, `linux/arm`, **`linux/arm64`**, `linux/loong64`, `linux/mips`, `linux/mips64`, `linux/mips64le`, `linux/mipsle`, `linux/ppc64`, `linux/ppc64le`, `linux/riscv64`, `linux/s390x` | <a href="example/gtk-app"><img src="docs/screenshots/linux.gif" width="300"></a> | Docker and colima, which can be installed by running [`setup-linux.sh`](scripts/setup-linux.sh) | Builds and runs. GTK4, started in a Docker container by running [`scripts/run-linux.sh`](scripts/run-linux.sh). The same app runs on a Chromebook when Linux is turned on in Settings → Developers. |
| **FreeBSD**<br>`freebsd/386`, **`freebsd/amd64`**, `freebsd/arm`, **`freebsd/arm64`** | <a href="example/gtk-app"><img src="docs/screenshots/freebsd.gif" width="300"></a> | qemu, which can be installed by running [`setup-freebsd.sh`](scripts/setup-freebsd.sh) | Builds and runs. GTK4, the same app as Linux, NetBSD and OpenBSD, started in a VM by running [`scripts/run-freebsd.sh`](scripts/run-freebsd.sh). |
| **OpenBSD**<br>`openbsd/386`, **`openbsd/amd64`**, `openbsd/arm`, **`openbsd/arm64`**, `openbsd/ppc64`, `openbsd/riscv64` | <a href="example/gtk-app"><img src="docs/screenshots/openbsd.gif" width="300"></a> | qemu, which can be installed by running [`setup-openbsd.sh`](scripts/setup-openbsd.sh) | Builds and runs. GTK4, the same app as Linux, FreeBSD and NetBSD. Started in a VM by running [`scripts/run-openbsd.sh`](scripts/run-openbsd.sh). OpenBSD ships an installer rather than a disk image, so the first run installs it, answering `autoinstall(8)` from a response file. |
| **NetBSD**<br>`netbsd/386`, **`netbsd/amd64`**, `netbsd/arm`, **`netbsd/arm64`** | <a href="example/gtk-app"><img src="docs/screenshots/netbsd.gif" width="300"></a> | qemu, which can be installed by running [`setup-netbsd.sh`](scripts/setup-netbsd.sh) | Builds and runs. GTK4, the same app as Linux, FreeBSD and OpenBSD, started in a VM by running [`scripts/run-netbsd.sh`](scripts/run-netbsd.sh). |
| **DragonFly**<br>**`dragonfly/amd64`** | <a href="example/gtk-app"><img src="docs/screenshots/dragonfly.gif" width="300"></a> | qemu, which can be installed by running [`setup-dragonfly.sh`](scripts/setup-dragonfly.sh) | Builds and runs. GTK4, the same app as the other BSDs, started in a VM by running [`scripts/run-dragonfly.sh`](scripts/run-dragonfly.sh). DragonFly is x86 only, so that VM is emulated rather than virtualised. It takes a minute to boot, and an hour to install GTK4 the first time. |
| **illumos**<br>**`illumos/amd64`** | <a href="example/gtk-app"><img src="docs/screenshots/illumos.gif" width="300"></a> | qemu, which can be installed by running [`setup-illumos.sh`](scripts/setup-illumos.sh) | Builds and runs. GTK4, the same app as the BSDs, on OpenIndiana in a VM. [`scripts/run-illumos.sh --install`](scripts/run-illumos.sh) installs it, which takes between half an hour and an hour because illumos is x86 only and the Mac emulates it, and `run-illumos.sh` runs it afterwards. |
| **Android**<br>`android/386`†, `android/amd64`†, `android/arm`†, **`android/arm64`** | <a href="example/android-app"><img src="docs/screenshots/android.gif" width="300"></a> | The Android SDK, a JDK and Kotlin, which can be installed by running [`setup-android.sh`](scripts/setup-android.sh) | Builds and runs in the emulator, built and started by running [`scripts/run-android.sh`](scripts/run-android.sh). |
| **iOS**<br>`ios/amd64`†, **`ios/arm64`** | <a href="example/ios-app"><img src="docs/screenshots/ios.gif" width="300"></a> | Xcode and a Simulator runtime, which can be installed by running [`setup-ios.sh`](scripts/setup-ios.sh) | Builds and runs in the Simulator, not on a phone. An app may not start a program on iOS, so the worker is compiled into the app and runs on a goroutine. Built and launched by running [`scripts/run-ios.sh`](scripts/run-ios.sh). |
| **wasm**, a browser<br>**`js/wasm`** | <a href="example/web-app"><img src="docs/screenshots/web.gif" width="300"></a> | A browser | Builds and runs. A page may not start a program, so the worker is compiled into the same wasm as the UI, as on iOS. Built and opened by running [`scripts/run-web.sh`](scripts/run-web.sh). |
| **wasm**, WASI<br>**`wasip1/wasm`** | <a href="example/wasi-app"><img src="docs/screenshots/wasi.gif" width="300"></a> | wasmtime, which can be installed by running [`setup-wasm.sh`](scripts/setup-wasm.sh) | Builds and runs, as [example/wasi-app](example/wasi-app) shows. Your Go program starts `wasmtime`, which runs `worker.wasm`. It has to be a Go program, because the Python, C# and Kotlin bindings would ask `worker.wasm` to start a second copy of itself, which it cannot do in a sandbox. `worker.wasm` also answers one request at a time, and can do nothing in between. |
| **Plan 9**<br>`plan9/386`, **`plan9/amd64`**, `plan9/arm` | <a href="example/plan9-app"><img src="docs/screenshots/plan9.gif" width="300"></a> | qemu, which can be installed by running [`setup-plan9.sh`](scripts/setup-plan9.sh) | Builds and runs. The same three rows drawn with libdraw in a rio window, started in a VM by running [`scripts/run-plan9.sh`](scripts/run-plan9.sh), and `--test` runs vero's test suite there instead. Plan 9 is x86 only, so the VM is emulated. The Go libdraw that it uses did not build for Plan 9 until [9fans/go#147](https://github.com/9fans/go/pull/147), so the example pins vero's fork, which carries that fix. |
| **Solaris**<br>**`solaris/amd64`** | None | None | Builds by running [`scripts/build-all.sh`](scripts/build-all.sh), and does not run. Oracle Solaris needs an Oracle account and licence to download, and runs on x86, which this Mac emulates rather than virtualises. Oracle's repository ships GTK 3, not GTK 4, so the GTK example would not run there as written, although the worker and vero's tests would. |
| **AIX**<br>**`aix/ppc64`** | None | None | Builds by running [`scripts/build-all.sh`](scripts/build-all.sh), and does not run. There is no public AIX media, since IBM provides it only under entitlement, with POWER hardware. qemu can emulate `pseries`, but there is nothing for it to boot. |

## Quick start

To see vero at work, build and run the example, a menu bar app that drives
[the example worker](example/worker). This needs Go and Xcode.

```bash
git clone https://github.com/imclaren/vero && cd vero
cd example/menubar-app && ./build.sh
./.build/debug/MenuBarExample
```

Look for its icon in the menu bar. `scripts/run.sh`, from the top of the
checkout, opens the same example on Linux and Windows beside it.

To make an app of your own, install the `vero` command, let it install what
each system needs, and make the app.

```bash
GOBIN=$(brew --prefix)/bin go install github.com/imclaren/vero/cmd/vero@latest
vero setup
vero new hello
cd hello/macos && ./build.sh
```

`build.sh` prints the command that runs the app. The guides below go
through each step.

## Guides

Read these in order. Each one ends with a link to the next.

1. [Set up your Mac](docs/setup.md) installs vero and the tools that each system needs.
2. [Start an app](docs/start.md) makes a new app with `vero new`, or brings an existing one to vero.
3. [Write the worker](docs/worker.md) explains the Go half of the app, and what makes it build for every system.
4. [The front ends](docs/front-ends.md) explains the native half of the app on each system, and how to write one by hand.
5. [Build, run and test on every system](docs/test.md) runs the app in containers and virtual machines on your Mac, and records each system at work.
6. [Package and release](docs/release.md) builds the installers and the site from which people install the app.
7. [Updates](docs/updates.md) explains how each system updates the app, and how to move an app to a newer vero.

## Reference

- [The `vero` command](cmd/vero/README.md) lists every command and what `vero add` writes.
- [`vero-app.toml`](PACKAGING.md) lists the settings that describe an app to the packaging.
- [`vero-repo`](cmd/vero-repo/README.md) describes the installers and the site for each system, and the packaging commands that `vero release` runs.
- [kit](kit/README.md) lists the Go packages that give a worker one API for secrets, notifications, sign-in items, sleep and updates on every system.
- [The bindings and the examples](docs/bindings.md) lists the library for each language and the example app on each system.
- [Contributing](CONTRIBUTING.md) explains how to test vero itself.
- [The plan](PLAN.md) is the plan for vero 0.18.

## Licence

MIT
