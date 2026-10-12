# vero 0.18: front ends that keep up with vero

This is the plan for vero 0.18 and for moving the four apps built on vero
onto it: audiobooks, treehouse, calmtasks (tasks-macos) and calmdocs
(sync-macos). It is also what a new app gets from `vero new` and `vero
ship`.

vero changes once, as an untagged 0.18 on main. Every app is built against
that copy, and vero is tagged only when all four have made the move
described below and pass on every system. A gap that any app finds is
fixed in vero before the tag, so that no app goes back to vero afterwards.

## Why

Today `vero add` writes a Windows and a Linux app once, and from then on
they belong to the app. A better starter helps only apps made after it,
and each existing app has to copy the improvements by hand. The Mac does
not have this problem, because its app depends on vero's Swift package and
gains what each version adds.

vero 0.18 gives Windows and Linux the same arrangement. What every app would
otherwise write for itself moves into libraries that each version of vero
updates. The starters become thin, and `vero upgrade` brings an existing
app up to date.

## The four apps

| | audiobooks | treehouse | calmtasks | calmdocs |
|---|---|---|---|---|
| vero today | 0.17 | 0.17 | 0.14 | 0.13 |
| The Mac app | A window | A window | In the menu bar | In the menu bar |
| Lowest macOS | 13 | 13 | 15 | 12 |
| Sandboxed | No | No | Yes | No |
| Mac releases | vero, with Sparkle | vero, with Sparkle | The App Store with fastlane, and direct with Sparkle | Its own release.sh, with Sparkle |
| The worker | `cmd/worker` | `cmd/worker` | `core/`, linked into the app as a C archive | In ~/src/syncclient, and it replaces itself |
| Windows and Linux | Small apps (660 lines of Python, 685 of C#) | Large apps (about 4,800 lines each) | None | None |
| Running with no window | No | Yes, syncing mail | Yes, in the menu bar | Yes, syncing folders, and as a daemon with no app |

## What goes into vero, and what stays in each app

| Piece | Needed by | Where it goes |
|---|---|---|
| The app's shell, with WPF's Fluent theme and libadwaita, following the system's light or dark mode | Every app | vero's libraries |
| A single copy, which brings the running copy forward | Every app | vero's libraries (the Mac already has `VeroSingleCopy`) |
| An app that lives in the tray, with a small window from its icon, as the Mac's menu bar apps do | calmtasks, calmdocs | vero's libraries, when `menubar = true` |
| Running with the window closed, and starting at sign-in | treehouse, calmtasks, calmdocs | vero's libraries and `kit/autostart` |
| A web page, with the system's browser in its place where there is no web view | audiobooks, treehouse | vero's libraries |
| A toast with an Undo button | treehouse, calmtasks (its worker has undo) | vero's libraries |
| Notifications with buttons | treehouse | vero's libraries and `kit/notify` |
| How each app looks: rows, avatars, empty states, settings pages | Each app | Each app, with libadwaita's own widgets on Linux |

A piece goes into vero when it is plumbing that every app would otherwise
write, or when two apps need it. How an app looks stays in the app.

GNOME shows no tray icons unless an extension is installed. Ubuntu
installs one, but Fedora and Debian do not. Where there is no tray, the
library keeps a menu bar app running through the background portal, and
opening the app from the menu shows its small window.

## Phase 1. vero 0.18, untagged

1. **Libraries.** `vero.py` and `Vero.cs` stay the bindings and the step
   player. Beside them, `vero_gtk.py` and `VeroWpf.cs` hold the pieces in
   the table. They are still copied into each app, not installed from pip
   or NuGet, so that every package builds without a network.
2. **Starters in two shapes.** `vero new` and `vero add` write either a
   window app or, with `menubar = true`, a menu bar app on the Mac and a
   tray app on Windows and Linux. The WPF starter targets .NET 10, turns on
   the Fluent theme with `ThemeMode="System"`, keeps its layout in XAML,
   puts colours and spacing in `Theme.xaml`, and uses Segoe Fluent Icons.
   The GTK starter uses `Adw.Application`, `Adw.ToolbarView` and a header
   bar, and needs libadwaita 1.5 or later. Both call the libraries for
   everything in the table.
3. **Dependencies worked out by vero-repo.** `[gtk]` names what the app
   uses, such as `uses = ["adwaita", "webview", "keyring"]`, and `extra`
   lists what only that app needs, such as audiobooks' ffmpeg. vero-repo
   writes each system's package names from these and adds a font on every
   system. Sections that list every dependency in full are still accepted.
4. **`vero upgrade`.** In an existing app, from vero 0.13 onwards, it
   updates `go.mod` and the kit, sets the Xcode project's pin on vero to
   Up to Next Major, copies the bindings and libraries afresh, rewrites
   the toml's dependency lists in the new form, and reports each change.
   Where an app's code must change, such as a renamed Swift call between
   0.13 and 0.18, it says where.
5. **Sandboxed Mac apps.** vero-repo signs the worker inside a sandboxed
   app with the sandbox and inherit entitlements, which lets a sandboxed
   app run it as a program of its own. calmtasks no longer needs its C
   archive, and its App Store build can bundle the worker as the others
   do.
6. **libadwaita 1.5 on every system and processor that the apps ship.**
   audiobooks ships for many processors, such as i386, armhf, riscv64 and
   loong64. vero's container and virtual-machine tests check each. A
   system without libadwaita 1.5 is dropped, as Debian 12 is, and the
   download page says why.

   The package indexes, checked on 11 October 2026, give these versions:

   | System | libadwaita |
   |---|---|
   | Debian 12 | 1.2.2, too old, so Debian 12 is dropped |
   | Debian 13 | 1.7.6, on amd64, arm64, armel, armhf, i386, ppc64el, riscv64 and s390x. The other processors that audiobooks ships for are Debian's ports, which follow unstable (1.10). |
   | Ubuntu 24.04 | 1.5.0 |
   | Fedora | 1.9.4 |
   | openSUSE Tumbleweed | 1.9.4. Leap 15.6 has 1.4.2 and is not supported. |
   | Alpine | 1.9.2 |
   | Void | 1.9.1 |
   | Chimera | 1.10.0 |
   | FreeBSD | 1.8.5 |
   | DragonFly | 1.7.2 |
   | NetBSD 10 and 11 | 1.9.1 on x86_64 and i386, 1.8.0 on aarch64 |
   | OpenBSD | 1.9.4 |
   | illumos | None in SmartOS's pkgsrc, from which vero installs GTK on OpenIndiana today. OpenIndiana's own repository has 1.6.10, and vero moves to it. |

   vero's tests use Debian 12 by default today, and move to Debian 13.

   illumos moves from SmartOS's pkgsrc to OpenIndiana's own repository.
   SmartOS is a server system, and its pkgsrc lacks libadwaita and a GTK 4
   web view, and its GTK 4 has crashed loading an app's icon. vero-repo
   makes IPS packages and an IPS repository for illumos, which depend on
   OpenIndiana's GTK 4 (4.16), PyGObject (3.54), libadwaita (1.6.10),
   libsecret and gnome-keyring, and the illumos test installs from it in
   vero's OpenIndiana virtual machine. Whether OpenIndiana's WebKitGTK
   2.52.5 includes the GTK 4 web view is checked there. Without it, illumos
   shows pages in the browser, as any system without a web view does.
7. **Typed requests generated from the worker.** The worker's handlers are
   typed in Go (`vero.Handle[Req, Rep]`), but the front ends call them by
   name with untyped JSON, and each Mac app keeps its own `Requests.swift`
   by hand. `vero gen` reads the worker's handlers and writes the matching
   types and calls for each front end: Swift structs that are `Codable`, C#
   records and Python dataclasses. `vero ship` and `vero upgrade` run it,
   so that a field renamed in Go fails to build in Swift and C# and fails
   a type check in Python, rather than failing when a test runs.
8. **Library files protected from edits.** Every file that vero copies
   into an app says that vero wrote it and that it is not to be edited,
   and vero records its hash. `vero upgrade` stops and names any such file
   that has been edited. An app that needs something different changes it
   through the library's settings or subclasses.
9. **Local builds numbered above the release.** A local build numbered
   below the newest release is replaced by it, because Sparkle installs
   any release with a higher number and vero keeps whichever worker has
   the higher number. treehouse found this, and its
   `macos/scripts/install.sh` numbers the app and the worker as
   `<published>.<date and time>`, which is above the release that is out
   and below the next. `vero install` does the same for every app, reading
   the published version from the app's `latest.json`, and the Mac's
   prebuild uses the same number for Xcode's own builds.

vero's own tests check both starter shapes on every system that vero
tests, and `vero ship counter.go` is run again as a new app would be.

## Phase 2. audiobooks, a window app

audiobooks is small, so it proves `vero upgrade` and the window shape
quickly.

1. Run `vero upgrade`.
2. Move `gtk/main.py` onto `vero_gtk.py` and libadwaita, and `windows/` onto
   `VeroWpf.cs`, .NET 10 and the Fluent theme. Amazon's sign-in uses the
   library's web page.
3. Move every front end, the Mac's included, to the generated requests.
4. Run `vero ship --no-release` on every system.

## Phase 3. treehouse, a large window app that runs in the background

[PLATFORMS.md](https://github.com/imclaren/treehouse/blob/main/PLATFORMS.md)
in treehouse has the detail.

1. Stage 1, the groundwork: a version stamped on local workers, the
   superseded scripts removed, `kit/update` in place of its own check, and
   `vero upgrade`.
2. Stages 3 and 4: Windows and Linux reshaped on the libraries, split into
   files by area, with `linux/` renamed to `gtk/`, the tray and running in
   the background, and a first run in vero's Windows virtual machine.
3. Move every front end, the Mac's included, to the generated requests.
   treehouse's 86 requests are the largest test of `vero gen`.
4. Run `vero ship --no-release` on every system.

## Phase 4. calmtasks, a sandboxed menu bar app

1. Move the worker from `core/` to `cmd/worker` in a module at the top of
   the repository, drop `libcalmtaskscore.a` and `build-core.sh`, and bundle
   the worker as a program, signed for the sandbox.
2. Run `vero upgrade` from 0.14, and move the Mac app onto vero's Swift
   package as the other apps use it.
3. Move the Mac app to the generated requests.
4. Add a demo mode and a `[test]` with `VERO_PROFILE`.
5. Run `vero add desktop` with `menubar = true`, which gives calmtasks its
   first Windows and Linux apps, as tray apps.
6. Run `vero ship --no-release` on every system. Direct releases move to
   vero. The App Store build stays with fastlane, without Sparkle.

## Phase 5. calmdocs, a menu bar app whose worker also runs alone

1. Move the worker from ~/src/syncclient into sync-macos as `cmd/worker`,
   drop its own updates and the separate publishing of the worker to the
   server, and replace them with vero's updates.
2. Run `vero upgrade` from 0.13, and move the Mac app to the generated
   requests.
3. Add a demo mode and a `[test]` with `VERO_PROFILE`.
4. Run `vero add desktop` with `menubar = true`, as for calmtasks.
5. Run `vero ship --no-release` on every system. Releases move to vero, and
   `scripts/release.sh` is retired.

The daemon that runs with no app is packaged later, as `calmdocs-headless`,
once vero can make headless packages.

## Phase 6. The tag and the releases

When all four apps pass on every system:

1. Tag vero v0.18.0 and publish its release.
2. Point every app at the tag.
3. Release each app.

What remains for each app after the tag, such as treehouse's Phases A to C
on Windows and Linux, and the Windows and Linux apps of calmtasks and
calmdocs growing to match their Mac apps, is work in the app alone. It
should not need vero again.

## Decisions

These were decided on 11 October 2026.


1. **The tag waits for all four apps.** Only their moves are waited for
   (the libraries, `vero upgrade`, the generated requests, the layout and
   a pass on every system),
   not the features each app adds afterwards.
2. **calmtasks on the App Store.** vero makes direct releases. The App
   Store build stays with fastlane.
3. **The repositories' names.** tasks-macos and sync-macos will hold
   Windows and Linux apps too. They keep their names for now.
4. **Debian 12.** libadwaita 1.5 drops Debian 12 for every app, including
   audiobooks, which supports it today.
