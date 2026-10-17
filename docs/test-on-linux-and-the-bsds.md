# Test on Linux and the BSDs

*Step 10 of 15 in [the vero guide](README.md).*

The GTK front end is one app that runs unchanged on Linux, the BSDs and
illumos. vero tests it on each of those systems from the Mac. Linux
distributions run in Docker containers, and each BSD and illumos in a
qemu virtual machine.

There are two different things you might want. The first is to see the
app running on Linux while you work on it. The second is to test the
installer: install the app the way a user would, from its own install
site, and play the test steps. Both use scripts from a checkout of vero,
shown here as `path/to/vero`.

## See the app running

```bash
path/to/vero/scripts/run-linux.sh --app gtk --entry main.py \
    --worker ./cmd/worker --worker-name hello-worker --env VERO_PROFILE=dev
```

This cross-compiles the worker, starts the app in a Debian container on
a virtual display, and opens it on the Mac over VNC. `--app` is the
front end's folder, `--entry` its main file, and `--worker-name` the name
in `[worker]` of `vero-app.toml`. Pass `--record out.gif` to record it
instead of watching.

For FreeBSD, vero's own example has `scripts/run-freebsd.sh`, and each
other BSD has one too. These take `--shell` for a shell inside the VM and
`--reset` to throw the VM away and make it again.

## Test the installer on one system

```bash
path/to/vero/scripts/test-repo.sh --app vero-app.toml --image fedora:latest
path/to/vero/scripts/test-repo.sh --app vero-app.toml --vm freebsd
```

`--image` names a Docker image, and `--vm` one of `freebsd`, `netbsd`,
`openbsd`, `dragonfly` or `illumos`. The script builds the app's
packages, builds an install site with a throwaway key, serves it from the
Mac, and then, inside the container or VM, adds the repository the way
the site's instructions say, installs the app, starts it, plays the steps
from `[[test.step]]`, and records the window. It then releases a newer
version into the same site and checks that the system's own update
mechanism brings it.

The result goes in `~/.cache/vero/test-results/`, and
`~/.cache/vero/test-results/index.html` shows every system tested so far.

## Test every system at once

```bash
path/to/vero/scripts/test-all.sh --app vero-app.toml --quick    # Debian and Windows
path/to/vero/scripts/test-all.sh --app vero-app.toml            # everything
path/to/vero/scripts/test-all.sh --app vero-app.toml --only "debian freebsd"
```

`test-all.sh` runs `test-repo.sh` for each system, containers all at
once and up to two VMs at a time. The quick run is for every day. The
full run adds the update test, the recordings, and DragonFly and illumos,
which are emulated and slow. A system that already passed with nothing
changed since is skipped. `--again` runs it anyway. While a run is going,
`test-all.sh --status` says where each system is.

`vero ship` runs all of this for you before a release, so you only need
these scripts directly when you want one system, or want to look at a
failure.

## Ready-made copies

Each system starts from a ready-made copy with the app's dependencies
and the test's tools already installed: a Docker image named
`vero-ready-SYSTEM` for a container, and a snapshot under
`~/vm/vero-SYSTEM/` for a VM. A test runs on a throwaway copy of it, so
nothing a test does changes the VM itself.

The copy is made again when the system or the app's dependencies change,
and in any case once a week for a container and once a month for a VM.
`--fresh` ignores the copy and starts from the bare system.

The first time a VM is needed, it is downloaded and installed, which
takes minutes for a BSD and up to an hour for illumos. DragonFly and
illumos are x86 only, so the Mac emulates them, and they carry on from a
saved running state rather than booting each time.

## Disk space

The images, VMs and results take space. `scripts/doctor.sh` shows how
much each kind takes, and `test-all.sh --prune` removes copies that are
no longer needed. A test refuses to start if it would leave less than
10 GB free, or whatever `VERO_MIN_FREE_GB` says.

## When a test fails

The log is in `~/.cache/vero/test-runs/`. The recording, if there is one,
often shows what went wrong before the log does.

---

Next: [Test on Windows](test-on-windows.md).
