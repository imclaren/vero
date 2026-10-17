# The vero guide

One page per step, in the order you meet them when you build an app. Read
it through once, or jump to the step you need. Each page says where it
sits and what comes next. The [front page](../README.md) has the short
version, a ten-minute walkthrough.

## Understand

1. [How vero works](how-vero-works.md). The worker, the front end, the
   state, requests and bindings, and a glossary of every term the guide
   uses.

## Set up

2. [Set up a Mac](set-up-a-mac.md). What to install before anything works.
3. [Describe the app](describe-the-app.md). What goes in `vero-app.toml`
   and what each section controls.

   If you already have a Mac app in Xcode,
   [Bring an Xcode app](bring-an-xcode-app.md) adds vero to it.

## Write the app

4. [Push state](push-state.md). How what the worker knows reaches the window.
5. [Add a request](add-a-request.md). How the window asks the worker to do something.
6. [Add a screen](add-a-screen.md). The same screen in SwiftUI, GTK and WPF.
7. [Secrets and sign-in](secrets-and-sign-in.md). Where tokens and passwords go, and OAuth.
8. [Build the worker for every system](build-the-worker-everywhere.md). When the worker will not build for one system.

## Test

9. [Write test steps](write-test-steps.md). Scripting what the test does once the window is open.
10. [Test on Linux and the BSDs](test-on-linux-and-the-bsds.md). Seeing and testing the app on Linux, FreeBSD and the rest, from the Mac.
11. [Test on Windows](test-on-windows.md). Making the Windows virtual machine and testing in it.
12. [Record the gallery pictures](record-the-gallery.md). Where the GIFs on the install page come from.

## Ship

13. [Release](release.md). Making a release, and what it produces.
14. [Sign and publish](sign-and-publish.md). Removing the "unknown developer" warnings, and getting into Homebrew, winget and the AUR.
15. [Updates](updates.md). How installed copies get the next release.

## Reference

### The vero command

`vero` with no arguments prints every command and its flags, and
`vero COMMAND --help` prints one command's.

| Command | What it does |
|---|---|
| `vero setup` | Installs what vero needs on this Mac, and says what is still missing. |
| `vero new NAME` | Makes a new app: a Go module with an example worker and a front end for each desktop system. |
| `vero add [front ends]` | Adds front ends to an app that already has a worker in `cmd/worker`. |
| `vero gen` | Writes each front end's typed binding from the worker's Go types. `--check` fails when one is stale. |
| `vero ship` | Tests and records the app on every system this Mac can run, then releases it. `--plan` says what it would do. `--no-release` stops before releasing. |
| `vero release` | Builds and packages a release without testing. |
| `vero install` | Builds the Mac app from the working copy and installs it, numbered just above the current release. |
| `vero upgrade` | Moves an app to this version of vero. |
| `vero credentials` | Says what is set up for signing and the stores. |
| `vero publish` | Publishes a release to Homebrew, winget or the AUR. |

`vero key`, `package`, `build`, `check`, `steps`, `show` and `plan` belong
to [vero-repo](../cmd/vero-repo/README.md), the packaging tool, and
`vero` passes them on. `--vero DIR`, or `VERO_DIR` for `vero setup` and
`vero ship`, uses a checkout of vero in place of the published module.

### Other reference pages

- [The kit](../kit/README.md). The Go packages for secrets, profiles,
  sleep, autostart, notifications, updates and helper programs.
- [Packaging for each system](../cmd/vero-repo/README.md). What each
  system's users get from a release, and each system's names for its
  processors.
- [The lab](lab.md). Android, iOS, the browser, WASI and Plan 9, and every
  target Go builds.
- [AGENTS.md](../AGENTS.md). The rules and the release checklist, as
  copied into every app for coding agents.
- [Contributing](../CONTRIBUTING.md). Running vero's own tests.
