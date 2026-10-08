# vero's Windows VM

WPF is Windows-only, so vero runs its Windows example, and tests its
Windows installers, in a Windows 11 ARM VM under qemu:
[`scripts/run-windows.sh`](../../scripts/run-windows.sh).

## What is here

- `autounattend.xml`: installs Windows without the forty minutes of
  clicking. It bypasses the TPM, Secure Boot and RAM checks a VM can't
  satisfy, makes a local account (Windows 11 otherwise insists on a
  Microsoft account), logs in by itself, and at first login runs
  `setup-ssh.ps1`, then `setup.ps1`.
- `setup.ps1`: installs Go, the .NET 8 SDK, git and a C compiler with
  winget. cgo needs a compiler, and Go doesn't include one.

`setup-ssh.ps1` isn't kept here: it's
[`scripts/lib/windows-ssh.ps1`](../../scripts/lib/windows-ssh.ps1), which
`run-windows.sh --install` puts on the answer disc with what it needs
beside it. It installs the virtio drivers, without which Windows on ARM
has no network in qemu, and Microsoft's OpenSSH for Windows, which lets in
only your key, `~/.ssh/vero-windows`.

## Making the VM

```bash
scripts/run-windows.sh --iso ~/Downloads/win11.iso --install   # once: about 20 minutes
scripts/run-windows.sh --ssh 2222                              # every time after
ssh -i ~/.ssh/vero-windows -p 2222 vero@127.0.0.1              # a PowerShell in the VM
```

A VM installed before the answer disc set up SSH gets it with
[`scripts/setup-windows-ssh.sh`](../../scripts/setup-windows-ssh.sh),
which types the one command that needs typing at the VM.

With SSH, a script copies files in with `scp` and runs commands without
touching the VM's desktop, as `scripts/test-repo.sh --vm windows` does.
