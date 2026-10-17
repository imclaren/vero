# The lab

*Reference page of [the vero guide](README.md).*

`vero ship` builds, tests and releases for the desktop systems: macOS,
Windows, Linux and the BSDs. vero also runs on more than those, and this
page is where the rest live. Each has a working example in the repository,
and `vero add mobile`, `vero add web`, `vero add wasi` and `vero add plan9`
write a front end for it in an app of your own. `vero release` packages
them, but `vero ship` neither tests nor records them, which is why they
are "the lab" and not the main list.

| System | Example | In one line |
|---|:---:|---|
| **Android** | <a href="../example/android-app"><img src="screenshots/android.gif" width="300"></a> | Kotlin, with the worker shipped as a native library. |
| **iOS** | <a href="../example/ios-app"><img src="screenshots/ios.gif" width="300"></a> | SwiftUI, with the worker compiled into the app. |
| **A browser** | <a href="../example/web-app"><img src="screenshots/web.gif" width="300"></a> | Front end and worker in one wasm module. |
| **WASI** | <a href="../example/wasi-app"><img src="screenshots/wasi.gif" width="300"></a> | A Go terminal front end running `worker.wasm` under wasmtime. |
| **Plan 9** | <a href="../example/plan9-app"><img src="screenshots/plan9.gif" width="300"></a> | libdraw in a rio window, in an emulated 9front VM. |
| **Solaris** | None | The worker builds. Nothing runs. |
| **AIX** | None | The worker builds. Nothing runs. |

## Android

The front end is Kotlin, and the worker ships inside the APK as a native
library, because that is the only place Android will run an executable
from. There is no Gradle; the build calls the six tools directly.
[`setup-android.sh`](../scripts/setup-android.sh) installs the SDK, a JDK
and Kotlin, about 5 GB in all, and
[`run-android.sh`](../scripts/run-android.sh) builds the example and runs
it in the emulator.

vero signs the `.apk` with a keystore that it makes, or with the one that
`VERO_ANDROID_KEYSTORE` names. Never lose it. Android installs an update
only when the same key signed it.

## iOS

The front end is SwiftUI, run in the Simulator by
[`run-ios.sh`](../scripts/run-ios.sh). An iOS app may not start another
program, so the worker is compiled into the app and runs on a goroutine,
with an in-memory pipe in place of the real one. Nothing above the pipe
changes. An app reaches real phones only through the App Store, which
needs `VERO_IOS_IDENTITY` and `VERO_IOS_PROFILE` set.

## A browser

A web page may not start a program either, so the front end and the
worker are compiled into one wasm module and talk over an in-memory
pipe. [`run-web.sh`](../scripts/run-web.sh) builds it and opens it in a
browser.

## WASI

The worker is a single `worker.wasm` that a WASI runtime such as
wasmtime starts, instead of an executable built for each machine. The
front end here is a Go program in the terminal. It has to be Go, because
the Python and C# bindings ask the worker to start a second copy of
itself, and a WASI sandbox cannot start anything. A WASI worker also has
only one thread, so it answers one request at a time and does nothing
between requests. The [WASI example](../example/wasi-app) explains what
that rules out.

## Plan 9

The front end draws with libdraw in a rio window, and
[`run-plan9.sh`](../scripts/run-plan9.sh) runs it in an emulated 9front
VM. `run-plan9.sh --test` runs vero's own test suite there, which is
worth doing because Plan 9's file lock is unlike every other system's.
The Go libdraw did not build for Plan 9 until
[9fans/go#147](https://github.com/9fans/go/pull/147), so the example pins
vero's fork, which carries that fix.

## Solaris and AIX

The worker builds for both, and nothing has been run. Oracle Solaris
needs an Oracle account to download, and its repository ships GTK 3, not
GTK 4. There is no public AIX media, so qemu has nothing to boot.

## What `vero add` does to the worker for iOS and the browser

On iOS and in a browser, an app may not start another program, so the
worker cannot be a separate process. `vero add` therefore moves the
worker's code into `internal/worker`, which the app compiles in and runs
on a goroutine, and leaves `cmd/worker` as a few lines that start it for
the other systems. That works automatically for a worker whose `main`
makes a `vero.WorkerOptions`, calls `vero.NewWorker` and ends with
`Serve`. For any other shape, or with `--keep-worker`, it writes a
placeholder and a note saying what to call.

## Every target that Go builds

The worker cross-compiles to every target in this table. The ones in
bold are built by `scripts/build-all.sh`, except `android/arm64`,
`ios/arm64` and `js/wasm`, which their examples' own `build.sh` build.
The targets marked † need a C toolchain, an Android NDK or the iOS
Simulator SDK. A test in [docs_test.go](../docs_test.go) fails when Go
adds a target that this table lacks, so the table is always complete.

| System | Targets |
|---|---|
| macOS | **`darwin/amd64`**, **`darwin/arm64`** |
| Windows | `windows/386`, **`windows/amd64`**, **`windows/arm64`** |
| Linux | `linux/386`, **`linux/amd64`**, `linux/arm`, **`linux/arm64`**, `linux/loong64`, `linux/mips`, `linux/mips64`, `linux/mips64le`, `linux/mipsle`, `linux/ppc64`, `linux/ppc64le`, `linux/riscv64`, `linux/s390x` |
| FreeBSD | `freebsd/386`, **`freebsd/amd64`**, `freebsd/arm`, **`freebsd/arm64`** |
| OpenBSD | `openbsd/386`, **`openbsd/amd64`**, `openbsd/arm`, **`openbsd/arm64`**, `openbsd/ppc64`, `openbsd/riscv64` |
| NetBSD | `netbsd/386`, **`netbsd/amd64`**, `netbsd/arm`, **`netbsd/arm64`** |
| DragonFly | **`dragonfly/amd64`** |
| illumos | **`illumos/amd64`** |
| Android | `android/386`†, `android/amd64`†, `android/arm`†, **`android/arm64`** |
| iOS | `ios/amd64`†, **`ios/arm64`** |
| A browser | **`js/wasm`** |
| WASI | **`wasip1/wasm`** |
| Plan 9 | `plan9/386`, **`plan9/amd64`**, `plan9/arm` |
| Solaris | **`solaris/amd64`** |
| AIX | **`aix/ppc64`** |
