# Test the app

[Build the app](build-the-app.md) left `jotter` with a screen on each
system. This page tests it the way a user would meet it: installed from
its own install site on Debian, on FreeBSD and on Windows, with the
window driven by a script and recorded. The scripts come from a checkout
of vero, shown here as `path/to/vero`:

```bash
git clone https://github.com/imclaren/vero path/to/vero
```

Every step was run on `jotter` as it stands at the end of Build the app.

## 1. Say what the test does

A test installs the app, opens its window, and then does what a person
would. What it does is a list of steps in `vero-app.toml`. Replace the
`pause = 3` step that `vero new` wrote with these three:

```toml
[[test.step]]
call = "addNote"
with = { text = "Buy milk" }

[[test.step]]
wait = { path = "notes.#", is = 1 }

[[test.step]]
pause = 2
```

The first sends the `addNote` request, as pressing Add would. The second
waits until the state has one note. `notes.#` is the length of the
`notes` list, and a path can also reach into it, as `notes.0.text` does.
The third pauses so that the recording shows the result.

Those are three of the four kinds of step:

| Step | What it does |
|---|---|
| `call = "name"`, `with = {...}` | Sends a request and waits for its reply. |
| `wait = { path = "...", is = ... }` | Waits until a value in the state matches. Instead of `is`: `not`, `at_least` or `contains`. `timeout` is in seconds. |
| `copy = { from = "...", to = "..." }` | Copies a file listed in `[test] files` onto the system. `{tmp}` in a path is a new empty folder. |
| `pause = N` | Waits N seconds. |

The test stops at the first step that fails. `[test] env` already sets
`VERO_PROFILE = "test"`, so the copy under test keeps its data apart from
any other copy, on every system.

## 2. Run it on Debian

```bash
path/to/vero/scripts/test-all.sh --app vero-app.toml --only debian
```

The first run makes a ready-made Debian image with the app's
dependencies installed, which takes a few minutes. After that you should
see this, in under a minute:

```
testing debian in the full run, with the update test, recorded; logs in /Users/iain/.cache/vero/test-runs
PASS debian (16s)
every result: /Users/iain/.cache/vero/test-results/index.html
```

In those sixteen seconds the test did five things, which its log at
`~/.cache/vero/test-runs/debian.log` shows as stages:

1. Released version 1.0.0 of the app to a site served from the Mac.
2. In a Debian container, added the apt repository the way the site's
   page says, and installed the app from it.
3. Started the app, played the three steps through vero's library inside
   the app, and recorded the window.
4. Released 1.0.1 into the same site.
5. Ran `apt upgrade` and checked that 1.0.1 arrived.

If step 2 installs something other than your app, the app's name clashes
with a package Debian already has. That is what happens to an app named
`hello`. Rename it.

## 3. Look at the result

```bash
open ~/.cache/vero/test-results/index.html
```

You should see one row for Debian, with a GIF of the window: the empty
list, then "Buy milk" with the time, then the pause. The same folder,
`~/.cache/vero/test-results/jotter/debian-trixie/`, has the GIF, a still
of the last frame, and the app's own log. A release puts each passing
system's GIF on the install page, so that people can see the app on
their own system before installing it.

## 4. Run it on FreeBSD

```bash
path/to/vero/scripts/test-all.sh --app vero-app.toml --only freebsd
```

FreeBSD has no container, so the test runs in a qemu virtual machine.
The first run downloads FreeBSD and installs GTK in it, which takes
several minutes. After that it starts from a snapshot of that, and you
should see:

```
PASS freebsd (94s)
```

The same stages run, with pkg in place of apt, plus one that brings the
snapshot up to date when the app's dependencies have changed. The
results page gains a FreeBSD row with the same window in FreeBSD's GTK.

NetBSD, OpenBSD, DragonFly and illumos work the same way, each in its
own VM. DragonFly and illumos are x86 only, so the Mac emulates them, and
the first illumos install takes up to an hour.

## 5. Run it on Windows

Windows needs a VM that vero cannot make without you. Download
Microsoft's
[Windows 11 ARM64 ISO](https://www.microsoft.com/en-us/software-download/windows11arm64),
then, once:

```bash
path/to/vero/scripts/run-windows.sh --iso ~/Downloads/win11.iso --install   # about 20 minutes
path/to/vero/scripts/setup-windows-display.sh
```

The install is unattended: it makes a local account, sets up SSH so that
the tests can reach the VM, and installs the .NET SDK in it. After that:

```bash
path/to/vero/scripts/test-all.sh --app vero-app.toml --only windows
```

You should see `PASS windows`. The test installs the app with the
Windows installer, plays the steps, records the window, updates the app
with a newer installer, and uninstalls it. The results page now shows
the WPF window from step 4 of Build the app, running for the first time.

## 6. Run everything

```bash
vero ship --no-release
```

This runs the test on every system this Mac can run, containers all at
once and up to two VMs at a time, and records the Mac app too. With
every VM already made it takes under an hour; DragonFly and illumos,
which are emulated, are most of that. While it runs:

```bash
path/to/vero/scripts/test-all.sh --status
```

says where each system is and how long it has left. A system that
passed before with nothing changed since is skipped. `--again` runs it
anyway.

For every day, the quick run tests Debian and Windows only, without the
update test and the recordings:

```bash
path/to/vero/scripts/test-all.sh --app vero-app.toml --quick
```

## When a test fails

The log is in `~/.cache/vero/test-runs/SYSTEM.log`, and the stage that
failed is the last `==` line in it. If the app started, the recording in
the results folder often shows what went wrong before the log does. A
step that fails names itself in the log, with the worker's reply.

The images and VMs take space. `path/to/vero/scripts/doctor.sh` says how
much, and `test-all.sh --prune` removes the ready-made copies that are
no longer needed.

---

Next: [Ship the app](ship-the-app.md).
