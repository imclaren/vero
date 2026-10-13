# Set up your Mac

This page installs vero and the tools that each system needs, so that one
Mac can build, run, test and release an app for all of them. It is done
once, before you start an app.

## What you need first

vero's scripts assume a Mac with Apple silicon. Four things cannot be
installed for you:

- Homebrew, from [https://brew.sh](https://brew.sh);
- Go, with `brew install go`;
- Xcode, from the App Store;
- for Windows, Microsoft's Windows 11 ARM64 ISO, in `~/vm/vero-windows/`.
  Microsoft will not serve it to a script, so download it yourself from
  [https://www.microsoft.com/en-us/software-download/windows11arm64](https://www.microsoft.com/en-us/software-download/windows11arm64).

Certificates from the Apple Developer Program are needed only to sign the
Mac app. [Package and release](release.md#signing-stores-and-listings)
says when you want them.

## Install vero

Install the `vero` command into Homebrew's folder of programs, which is
already on your `PATH`, and let it set up the rest.

```bash
GOBIN=$(brew --prefix)/bin go install github.com/imclaren/vero/cmd/vero@latest
vero setup
```

`vero setup` installs, with Homebrew, the tools that each system needs.
These are colima and Docker for Linux, qemu for the virtual machines of
Windows, the BSDs, illumos and Plan 9, the .NET SDK for Windows, wasmtime
and node for wasm, makensis for the Windows installers, and ffmpeg for the
recordings. It installs `vero-repo`, which builds the installers. It makes
the notarytool profile named `vero`, for which it asks for your Apple ID,
your team ID and an app-specific password. Then it lists anything that is
still missing. It is safe to run again, since it leaves alone anything
that is already there.

Android is left out, because its SDK and emulator take about 5GB.
`scripts/setup-android.sh`, in a checkout of vero, installs them.

`vero-site` serves a release's site, for a look at it on your Mac, and can
also serve it on your own server.

```bash
go install github.com/imclaren/vero/kit/cmd/vero-site@latest
```

## Check what is there

[`scripts/doctor.sh`](../scripts/doctor.sh) says what this Mac has for
building, testing, recording and releasing, and gives the command that adds
each thing that is missing. It also says which virtual machines are made
and how much disk they need. It changes nothing, and `vero setup` runs it
at the end.

## A checkout of vero

The examples and the scripts that run them are in vero's repository.

```bash
git clone https://github.com/imclaren/vero && cd vero
./scripts/setup.sh
```

[`scripts/setup.sh`](../scripts/setup.sh) installs what every example
needs and then builds them. `--all` adds Android, and `--no-build` only
installs. It runs one script for each system, and you can run a single one
if you need only that system:

- [`setup-macos.sh`](../scripts/setup-macos.sh) checks for Go and Xcode;
- [`setup-ios.sh`](../scripts/setup-ios.sh) checks for Xcode and downloads a Simulator runtime;
- [`setup-linux.sh`](../scripts/setup-linux.sh) installs colima and Docker;
- [`setup-windows.sh`](../scripts/setup-windows.sh) installs qemu and the .NET SDK, and helps you find the ISO;
- [`setup-freebsd.sh`](../scripts/setup-freebsd.sh), [`setup-netbsd.sh`](../scripts/setup-netbsd.sh), [`setup-openbsd.sh`](../scripts/setup-openbsd.sh), [`setup-dragonfly.sh`](../scripts/setup-dragonfly.sh), [`setup-illumos.sh`](../scripts/setup-illumos.sh) and [`setup-plan9.sh`](../scripts/setup-plan9.sh) install qemu;
- [`setup-wasm.sh`](../scripts/setup-wasm.sh) installs wasmtime and node;
- [`setup-android.sh`](../scripts/setup-android.sh) installs the Android SDK, a JDK and Kotlin, and makes an emulator;
- [`setup-release.sh`](../scripts/setup-release.sh) installs makensis and ffmpeg, for releases and recordings.

`vero setup`, `vero new` and `vero ship` take `--vero DIR`, or the
`VERO_DIR` environment variable, to use a checkout in place of the
published vero.

Next, [start an app](start.md).
