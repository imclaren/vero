# Test on Windows

Windows 11 for ARM runs in a qemu virtual machine on the Mac, made once
from Microsoft's ISO in `~/vm/vero-windows/`.

```bash
path/to/vero/scripts/run-windows.sh --iso ~/Downloads/win11.iso --install   # the first time
path/to/vero/scripts/setup-windows-display.sh                               # once, for 1024 by 768
path/to/vero/scripts/test-repo.sh --app vero-app.toml --vm windows          # install, run, update, uninstall
```

`test-repo.sh --vm windows` reaches the VM over SSH. It installs the app,
plays its steps, records it, updates it and removes it, and registers the
MSIX in developer mode when the app has `[wpf.msix]`. A VM installed before
SSH was set up gets it from `scripts/setup-windows-ssh.sh`, and
`run-windows.sh --ssh 2222` then opens port 2222 to it, with the key
`~/.ssh/vero-windows`. `dotnet build` in `windows/` checks on the Mac that
the app compiles, and `run-windows.sh --payload DIR` boots the VM with a
build of your own on a disc.
