# Build and run the example on all platforms using your Mac

[`./scripts/setup.sh`](../scripts/setup.sh) installs what every platform
needs, with Homebrew. Four things it cannot install for you:

- Homebrew itself, from [https://brew.sh](https://brew.sh);
- Go, with `brew install go`;
- Xcode, from the App Store;
- a Windows 11 ARM64 ISO, which Microsoft will not serve to a script.
  Download it from
  [https://www.microsoft.com/en-us/software-download/windows11arm64](https://www.microsoft.com/en-us/software-download/windows11arm64)
  and put it in `~/vm/vero-windows/`.

[`./scripts/doctor.sh`](../scripts/doctor.sh) says what is there and what is
missing, which VMs are made, and how much disk they need. The scripts
assume a Mac with Apple silicon.

[`./scripts/run.sh`](../scripts/run.sh) then builds and opens the example on
macOS (natively), and Linux and Windows (by showing the app in VMs) side
by side. Each other system has a `scripts/run-*.sh` of its own, in the
table in the [README](../README.md), which makes its VM the first time.

[`./scripts/test-all.sh`](../scripts/test-all.sh) tests the example, or your
app with `--app vero-app.toml`, on every system at once: it installs it
from a site it builds, starts it and plays its test steps, records each
system as a GIF and a picture, checks that the system's own updates bring
the next version, and shows every result on one page.

All targets in **bold** (e.g. `darwin/arm64`), other than `android/arm64`,
`ios/arm64` and `js/wasm`, can be built by running
[`./scripts/build-all.sh`](../scripts/build-all.sh).
[`android/arm64`](../example/android-app/build.sh),
[`ios/arm64`](../example/ios-app/build.sh) and
[`js/wasm`](../example/web-app/build.sh) have their own build scripts because
they do not build a standalone worker. To build targets not in bold, specify
them when building: `TARGETS=plan9/386 ./scripts/build-all.sh`.

† targets need a C toolchain: either an NDK for the three Androids that are
not `arm64`, or the Simulator SDK for `ios/amd64`. The other 43 targets only
need Go.
