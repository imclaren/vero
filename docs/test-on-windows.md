# Test on Windows

WPF only runs on Windows, so vero tests the Windows front end in a
Windows 11 ARM virtual machine under qemu on the Mac. The VM is made once
and then reused.

## Make the VM, once

Download Microsoft's
[Windows 11 ARM64 ISO](https://www.microsoft.com/en-us/software-download/windows11arm64)
and put it in `~/vm/vero-windows/`. Then, from a checkout of vero:

```bash
path/to/vero/scripts/run-windows.sh --iso ~/Downloads/win11.iso --install   # about 20 minutes
path/to/vero/scripts/setup-windows-display.sh                               # once, sets 1024 by 768
```

The install is unattended. It makes a local account, sets up SSH so that
scripts can reach the VM, and installs Go and the .NET SDK inside it. The
[Windows VM notes](../.windows/unattend/README.md) say what it does in
detail.

## Test the installer

```bash
path/to/vero/scripts/test-repo.sh --app vero-app.toml --vm windows
```

This builds the Windows installer, serves an install site from the Mac,
and over SSH installs the app in the VM, plays the test steps, records
the window, updates the app to a newer version, and uninstalls it. If
`vero-app.toml` has a `[wpf.msix]` section, the MSIX is installed in
developer mode too. The VM runs on a throwaway overlay, so a test never
changes its disk. The VM starts while the installers build, so there is
nothing to wait for.

`test-all.sh --quick` tests Windows and Debian together, without the
update test and recording, and is what to run day to day.

## Try your own build in the VM

```bash
cd windows && dotnet build                         # on the Mac: does it compile?
path/to/vero/scripts/run-windows.sh --payload out  # boot the VM with a build of your own
```

`--payload` packs a folder into a disc image that the VM sees as a CD
drive. Copy the folder from the CD to the desktop and run the exe.

## Getting a shell in the VM

```bash
path/to/vero/scripts/run-windows.sh --ssh 2222
ssh -i ~/.ssh/vero-windows -p 2222 vero@127.0.0.1
```

A VM that was installed before SSH was part of the setup gets it from
`scripts/setup-windows-ssh.sh`.
