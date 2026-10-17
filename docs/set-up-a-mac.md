# Set up a Mac

*Step 2 of 15 in [the vero guide](README.md).*

Everything vero does happens on one Mac with Apple silicon: writing the
app, building it for every system, testing it in containers and virtual
machines, and releasing it. This page gets that Mac ready.

## What you need before you start

- A Mac with Apple silicon (M1 or later).
- [Homebrew](https://brew.sh).
- Go, from `brew install go`.
- Xcode, from the App Store. Open it once so that it installs its command
  line tools. It is needed to build the Mac front end.

## Install vero

```bash
GOBIN=$(brew --prefix)/bin go install github.com/imclaren/vero/cmd/vero@latest
vero setup
```

`vero setup` uses Homebrew to install the tools that the other systems
need. Each is used for one thing:

| Tool | What vero uses it for |
|---|---|
| colima and Docker | Linux containers, to test the Linux packages and build a Flatpak |
| qemu | Virtual machines for the BSDs, illumos and Windows |
| the .NET SDK | Building the WPF front end on the Mac |
| makensis | Building the Windows installer |
| wasmtime | Running a WASI worker |
| ffmpeg | Making the GIF recordings of each test |
| vero-repo | The packaging tool, which `vero release` runs for you |

It also makes a notarisation profile named `vero`. For that it asks for
your Apple ID, your team ID and an app-specific password from
appleid.apple.com. Notarisation is what lets a Mac open a downloaded app
without a warning. If you do not have an Apple developer account yet,
decline the question. Nothing else depends on it, and you can run
`vero setup` again later.

## Two things it cannot fetch for you

- **Windows.** Microsoft's
  [Windows 11 ARM64 ISO](https://www.microsoft.com/en-us/software-download/windows11arm64)
  has to be downloaded by hand. Put it in `~/vm/vero-windows/`.
  [Test on Windows](test-on-windows.md) makes the virtual machine from it.
- **Android.** The SDK is about 5 GB, so it is installed only when you ask,
  with `scripts/setup-android.sh` from a checkout of vero.

## Check what is missing

`scripts/doctor.sh`, in a checkout of vero
(`git clone https://github.com/imclaren/vero`), lists each system, what
it needs on this Mac, and whether it is there. It changes nothing.

---

Next: [Describe the app](describe-the-app.md).
