# Test on Linux and the BSDs

The GTK app runs unchanged on Linux, the BSDs and illumos. Linux runs in a
Docker container, and each BSD and illumos in a qemu virtual machine.

```bash
# The app, live in a Debian container, shown on the Mac over VNC.
path/to/vero/scripts/run-linux.sh --app gtk --entry main.py \
    --worker ./cmd/worker --worker-name hello-worker --env VERO_PROFILE=dev

# Installed from the app's own site, with its steps played and recorded.
path/to/vero/scripts/test-repo.sh --app vero-app.toml --image fedora:latest
path/to/vero/scripts/test-repo.sh --app vero-app.toml --vm freebsd   # or netbsd, openbsd, dragonfly, illumos
```

`test-repo.sh` starts each system from a ready-made copy: a Docker image
named `vero-ready-SYSTEM` for a container, and a snapshot in
`~/vm/vero-SYSTEM/ready-KEY/` for a VM, with the app's dependencies and
the test's tools installed. Each run uses a throwaway copy of it, so the
VM's own disk is never changed by a test. The copy is made again when the
system or what the app needs changes, each week for a container and each
month for a VM, and `--fresh` starts from the system itself. A FreeBSD
or illumos snapshot is brought up to date first, and DragonFly and
illumos, which are emulated and slow to boot, carry on from a running
state saved with the snapshot.
`test-all.sh --prune` removes the copies that are no longer needed, and
`scripts/doctor.sh` shows the space that each kind takes. A system is
refused when it would leave less than `VERO_MIN_FREE_GB` (10 GB) free.

`gtk/README.md` gives the worker's name. Each VM is made the first time,
which downloads the system and its GTK. DragonFly and illumos are x86 only,
so the Mac emulates them, and the first illumos install takes up to an
hour. The `run-*.sh` scripts for vero's example take `--shell` for a shell
in the VM and `--reset` to make it again. A test that fails leaves its log
in `~/.cache/vero/test-runs/`.
