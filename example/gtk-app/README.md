# The GTK example, for Linux, the BSDs and illumos

The same three jobs as every other example, in a GTK 4 window. This is
the front end that `vero new` writes for Linux, the BSDs and illumos,
in the form of a finished example. It drives the worker in
[../worker](../worker).

## Run it from the Mac

From a checkout of vero:

```bash
./scripts/run-linux.sh       # in a Debian container, shown on the Mac over VNC
./scripts/run-freebsd.sh     # in a FreeBSD virtual machine
```

There are `run-*.sh` scripts for NetBSD, OpenBSD, DragonFly and illumos
too. The first run of a VM downloads the system and installs GTK in it,
which takes a few minutes for a BSD and longer for illumos. After that
it boots in seconds. Each script takes `--shell` for a shell in the VM,
`--reset` to throw the VM away and start again, and `--record out.gif`
to record the window.

## Run it on Linux or a BSD directly

```bash
./build.sh                                        # builds the worker
sudo apt-get install python3-gi gir1.2-gtk-4.0    # Debian; on FreeBSD, pkg install gtk4 py312-pygobject
./main.py
```

## What is different here

Nothing on the Go side. The front end is Python, which cannot link Go,
so it starts the worker binary with `VERO_HOST` set. In that mode the
worker supervises a second copy of itself and relays JSON on its own
standard input and output. Restarts, backoff and the single-copy lock all
happen in Go, on the far side of the pipe, and the Python binding is only
the conversation. There is no shared library to build or to find, which
is why this one app runs on every system Go can produce an executable
for.

## The files

| | |
|---|---|
| [main.py](main.py) | the front end: three rows, a Restart button each |
| [build.sh](build.sh) | builds the worker |
| [../../bindings/python/vero.py](../../bindings/python/vero.py) | the binding: one process, one reader thread, JSON |

## Making one of your own

`vero new hello` writes an app with this front end, a SwiftUI one and a
WPF one, around a worker of your own. The [README](../../README.md) walks
through it.
