# Set up a Mac

vero needs a Mac with Apple silicon, [Homebrew](https://brew.sh), Go
(`brew install go`) and Xcode from the App Store.

```bash
GOBIN=$(brew --prefix)/bin go install github.com/imclaren/vero/cmd/vero@latest
vero setup
```

`vero setup` installs, with Homebrew, what each system needs, which is
colima and Docker, qemu, the .NET SDK, wasmtime, makensis, ffmpeg and
`vero-repo`. It makes the notarytool profile named `vero`, for which it asks
for an Apple ID, and it is safe to run again.

It cannot fetch two things. Windows needs Microsoft's
[Windows 11 ARM64 ISO](https://www.microsoft.com/en-us/software-download/windows11arm64)
in `~/vm/vero-windows/`, and Android needs `scripts/setup-android.sh`, since
its SDK takes about 5GB. [`scripts/doctor.sh`](../scripts/doctor.sh), in a
checkout of vero, says what is missing and how to add it, and changes nothing.
