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

`gtk/README.md` gives the worker's name. Each VM is made the first time,
which downloads the system and its GTK. DragonFly and illumos are x86 only,
so the Mac emulates them, and the first illumos install takes up to an
hour. The `run-*.sh` scripts for vero's example take `--shell` for a shell
in the VM and `--reset` to make it again. A test that fails leaves its log
in `~/.cache/vero/test-runs/`.
