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
| Lowest macOS | 13 | 13 | 15 | 12, with a second Mac worker built by Go 1.26 for it |
| Sandboxed | No | No | Yes | No |
| Mac releases | vero, with Sparkle | vero, with Sparkle | The App Store with fastlane, and direct with Sparkle | Its own release.sh, with Sparkle |
| The worker | `cmd/worker` | `cmd/worker` | `core/`, linked into the app as a C archive | In ~/src/syncclient, and it replaces itself |
| Windows and Linux | Small apps (660 lines of Python, 685 of C#) | Large apps (about 4,800 lines each) | None | None |
| Running with no window | No | Yes, syncing mail | Yes, in the menu bar | Yes, syncing folders, and as a daemon with no app |

## What goes into vero, and what stays in each app

| Piece | Needed by | Where it goes |
|---|---|---|
| The app's shell: a window with a header bar, banners for problems and updates, and the system's light or dark mode, which the app can also read (treehouse darkens messages with it) | Every app | vero's libraries |
| A single copy, which brings the running copy forward and passes it what the second copy was asked to open: a link, such as `mailto:` or `calmtasks://task/1`, or files | Every app | vero's libraries (the Mac already has `VeroSingleCopy`) |
| A tray icon whose picture and tooltip follow the state, with the app's own menu items, Open and Quit | treehouse (a window app that keeps running), calmtasks and calmdocs (tray apps) | vero's libraries, with `tray = true` for a window app and `menubar = true` for an app that is only in the tray |
| A small window from the tray icon, of the size the app asks for (calmdocs' is 650 by 500), with no taskbar button unless the main window is open | calmtasks, calmdocs | vero's libraries, when `menubar = true` |
| Running with the window closed, and starting at sign-in, hidden | treehouse, calmtasks, calmdocs | vero's libraries and `kit/autostart` |
| Dialogs: open files, save a file, choose a folder, a question with buttons, and asking for some text or a password | Every app | vero's libraries |
| Opening a link, a file or a folder, and showing a file in its folder | Every app (calmdocs' worker does this today, on the Mac only) | vero's libraries |
| A sign-in page that stops at an address, with the system's browser and a pasted address where there is no web view | audiobooks | vero's libraries |
| Showing HTML with scripts off, and a web view whose scripts and the app talk both ways, with printing to PDF. Where there is no web view, the library says so, and the app shows text or hides the feature. Private content never goes to the browser. | treehouse (mail, the editor, add-ins) | vero's libraries |
| A toast with an Undo button, and an undo and redo stack of the worker's action ids, on Ctrl+Z and Ctrl+Shift+Z | treehouse, calmtasks | vero's libraries |
| Notifications with buttons, whose buttons reach the app | treehouse | vero's libraries. `kit/notify` stays for workers with no app. |
| The network coming back, so that the worker syncs at once | calmtasks, treehouse, calmdocs | vero's libraries, which tell the app, which asks the worker |
| A password for sign-ins on a system with no keyring | treehouse, calmtasks, calmdocs | `kit/keychain` (a file encrypted with a key from the password, as treehouse's vault is) and a library dialog |
| The update check, and putting off an update while the app is busy | Every app | `kit/update` with a standard request, and the Swift package's Sparkle setup |
| Keeping the computer awake while syncing, and hearing of sleep and wake | calmdocs | New `kit/awake` |
| One worker at a time, when the worker runs with no app | calmdocs | vero's Go package, which takes the same lock as the supervisor |
| Registering link schemes and file types: `mailto:` and `.eml`, `.ics` and `.vcf` for treehouse, `calmtasks://` and opening files for calmtasks | treehouse, calmtasks | `vero-app.toml` (`schemes` and `opens`), which vero-repo writes into the `.desktop` file and the Windows installer |
| How each app looks: rows, avatars, empty states, settings pages, a search provider, maps | Each app | Each app, with the library's widgets on Linux, the BSDs and illumos, which are libadwaita's where it is installed |

Every app runs on every system and processor on which audiobooks runs
today, as its README lists them. No app drops one of them by moving to
0.18, and calmtasks and calmdocs gain all of them.

A piece goes into vero when it is plumbing that every app would otherwise
write, or when two apps need it. How an app looks stays in the app.

## Each piece on each system

The libraries cannot assume any of four things: libadwaita, a web view, a
D-Bus session bus, or a keyring. Each is missing somewhere that audiobooks
runs. Debian 12 has libadwaita 1.2 and GTK 4.8. SmartOS's pkgsrc, from
which illumos gets GTK today, has no web view and no keyring. A BSD or
illumos desktop may start without a session bus. Every piece therefore has
a stated form for each case, and vero's tests run each case.

| Piece | Windows | GTK with libadwaita | GTK without libadwaita | Without a session bus | Without a web view | Without a keyring |
|---|---|---|---|---|---|---|
| Window and banners | Fluent theme, `ThemeMode="System"` | `Adw.ToolbarView`, `Adw.Banner` | A header bar and a revealer | The same | The same | The same |
| Light or dark | The system's | `Adw.StyleManager` | The portal's colour scheme, or GTK's dark setting | GTK's dark setting, and the app's own switch | The same | The same |
| Single copy and passing on links and files | A mutex and a named pipe | `Gio.Application` with command-line and open handling | The same | A lock file and a socket in the runtime folder, which the library uses in place of D-Bus | The same | The same |
| Tray | `NotifyIcon` from Windows Forms, which works on x64, arm64 and x86 | StatusNotifierItem and its menu over D-Bus, written in the library without libappindicator. Where no tray is shown, as on GNOME without its extension, the background portal keeps the app running. | The same | No tray. A tray app opens as an ordinary window, and a window app that runs in the background asks before closing, since nothing could reopen it | The same | The same |
| Notifications | A toast with buttons, through Windows' notification API and the app's id, which the installer registers. A button press reaches the app through the single copy. | `Gio.Notification` with buttons | The same | A banner in the window | The same | The same |
| Dialogs | WPF's and Windows' own | `Gtk.FileDialog` and `Gtk.AlertDialog` on GTK 4.10 or later, `Gtk.FileChooserNative` and a message dialog before | The same | The same, with GTK's own file chooser | The same | The same |
| Showing a file in its folder | `explorer /select` | The file manager's `ShowItems` over D-Bus | The same | Opening the folder | The same | The same |
| Web content | WebView2, which the installer adds | WebKitGTK 6 | The same | The same | Sign-in pages in the browser with a pasted address. HTML as text. Script bridges report that they are missing. | The same |
| Sign-ins kept | Credential Manager | The Secret Service | The same | The password dialog | The same | The password dialog |
| Autostart | The Run key | An autostart `.desktop` file, or the background portal in a Flatpak | The same | The autostart file | The same | The same |

The libraries need GTK 4.8 and PyGObject 3.42 at least, as Debian 12
has, and Python 3.9, since OpenIndiana's PyGObject is for Python 3.9
alone (checked in its repository on 12 October 2026). The package indexes show no older floor on
any system that audiobooks supports.

## Workers on every system

The front ends are only half of it. Each worker must build and run on
every system and processor too, and vero builds workers without cgo.
Checked on 12 October 2026:

| Worker | Result | What it needs |
|---|---|---|
| audiobooks | Builds for every target | Nothing |
| treehouse | Builds for every target | Nothing |
| calmtasks (`core/`) | Fails on DragonFly, illumos, NetBSD arm64 and OpenBSD riscv64, because `modernc.org/sqlite` has no port there | Move to `ncruces/go-sqlite3`, as treehouse uses |
| calmdocs (syncclient) | Fails on every target tried | Replace the Mac-only sleep notifier with `kit/awake`. Fix `ulimit` on 32-bit systems and Windows. Watch folders by polling on illumos, where FEN needs cgo. Let `osdetails` accept any system. Replace `mattn/go-sqlite3`, which needs cgo, with `ncruces/go-sqlite3`. |

`ncruces/go-sqlite3` builds for every target, and a test program that
writes and reads a database in WAL mode ran under emulation on Linux on
amd64, 386, arm, arm64, riscv64, loong64, mipsle, mips64le, s390x, ppc64
and ppc64le. It still has to be run on each BSD and on illumos, in vero's
virtual machines, because file locking differs there.

So that a new app finds this at once and not at release, `vero ship`
and `vero-repo plan` build the worker for every target first, and name
each target that fails and the package that stops it.

## Every system and processor that Go supports

`go tool dist list` (Go 1.27.1) names 47 pairs of system and processor.
This is what vero builds for each, and why. "In" means every vero app
ships it, as audiobooks does today. vero builds every worker without cgo.
The Mac's worker is the one exception, and only when the app asks.

Go's build settings for a processor are `GOAMD64`, `GO386`, `GOARM`,
`GOARM64`, `GOMIPS`, `GOMIPS64`, `GOPPC64`, `GORISCV64` and `GOWASM`.
vero leaves each at Go's default, which is the oldest processor Go
supports, except where the table says otherwise.

| Go's pair | Build settings | Packages, with each system's name for the processor | In or out | Why |
|---|---|---|---|---|
| darwin/amd64 and darwin/arm64 | Both in one universal app | The Mac app, from vero's Swift package, with Sparkle | In | macOS 13 or later with Go 1.27, which builds for macOS 13. calmdocs keeps macOS 12, so its app carries a second worker built with Go 1.26, the last Go to build for macOS 12, which it runs only on macOS 12. |
| windows/amd64 | GOAMD64=v1 | Installer, x64 | In | |
| windows/arm64 | GOARM64=v8.0 | Installer, arm64 | In | |
| windows/386 | GO386=sse2 | Installer, x86 | In | .NET 10 runs on 32-bit Windows 10. |
| linux/amd64 | GOAMD64=v1 | deb amd64, rpm x86_64, Arch x86_64, Alpine x86_64, Void x86_64, Chimera x86_64, Flatpak x86_64 | In | |
| linux/arm64 | GOARM64=v8.0 | deb arm64, rpm aarch64, Arch aarch64, Alpine aarch64, Void aarch64, Chimera aarch64, Flatpak aarch64 | In | |
| linux/386 | GO386=sse2 | deb i386, rpm i686 (openSUSE), Arch Linux 32 pentium4, Alpine x86, Void i686 | In | vero builds with SSE2, so a processor older than the Pentium 4 cannot run it. Debian 13's i386 can be added to a 64-bit system but no longer installed alone. |
| linux/arm | GOARM=7 | rpm armv7hl (openSUSE), Arch armv7h, Alpine armv7, Void armv7l | In | ARMv7 with hardware floating point. |
| linux/arm | GOARM=6 | deb armhf, rpm armv6hl (openSUSE), Alpine armhf, Void armv6l | In | ARMv6 runs on the first Raspberry Pis, whose Raspberry Pi OS calls it armhf, and on every ARMv7. |
| linux/arm | GOARM=5 | deb armel | In | ARMv5, without hardware floating point. |
| linux/riscv64 | GORISCV64=rva20u64 | deb, rpm (openSUSE), Arch, Alpine, Chimera | In | |
| linux/ppc64le | GOPPC64=power8 | deb ppc64el, rpm ppc64le, Arch POWER powerpc64le, Alpine ppc64le, Chimera ppc64le | In | |
| linux/ppc64 | GOPPC64=power8 | deb ppc64 (Debian's ports), Arch POWER powerpc64, Chimera ppc64 | In | POWER8 is the oldest that Go builds for, so these do not run on older big-endian machines, such as the Power Mac G5, which these systems also support. |
| linux/s390x | Go's default | deb s390x, rpm s390x, Alpine s390x | In | |
| linux/loong64 | Go's default | deb loong64 (Debian unstable), Arch loong64, Alpine loongarch64, Chimera loongarch64 | In | |
| linux/mips64le | GOMIPS64=hardfloat | deb mips64el | In | Debian 12 is the last Debian for it, so it gets plain GTK 4. |
| linux/mipsle | GOMIPS=hardfloat | deb mipsel | In | The same, and Debian 12 has no web view for it. |
| linux/mips and linux/mips64 | | None | Out | Big-endian MIPS has no desktop system. Debian dropped it after Debian 10. |
| freebsd/amd64 and freebsd/arm64 | Go's defaults | pkg amd64, aarch64 | In | |
| freebsd/386 | GO386=sse2 | pkg i386 | In | |
| freebsd/arm | GOARM=7 | pkg armv7 | In | FreeBSD's packages for it are older, with Python 3.11, but have libadwaita 1.7.6. |
| dragonfly/amd64 | GOAMD64=v1 | pkg x86_64 | In | |
| netbsd/amd64, netbsd/arm64 and netbsd/386 | Go's defaults, GO386=sse2 | pkgsrc x86_64, aarch64, i386 | In | NetBSD has no GTK 4 web view. |
| netbsd/arm | | None | Out | NetBSD's binary packages for 32-bit ARM (earmv7hf) have GTK 3 but no GTK 4, in 10.0, 10.1, 10.0_2026Q3 and 11.0, checked on 12 October 2026. |
| openbsd/amd64 and openbsd/arm64 | Go's defaults | amd64, aarch64 | In | |
| openbsd/386 | GO386=sse2 | i386 | In | No web view. |
| openbsd/arm | GOARM=7 | arm | In | No libadwaita, so plain GTK 4, and no web view. |
| openbsd/riscv64 | Go's default | riscv64 | In | |
| openbsd/ppc64 | GOPPC64=power8 | powerpc64 | In, for apps that need nothing it lacks | OpenBSD 7.9 has GTK 4 (4.22.3) and PyGObject (3.56.2) for it, but no libadwaita, web view or ffmpeg. So apps get plain GTK 4 and no web view there, and audiobooks, which needs ffmpeg, leaves it out. vero-repo adds powerpc64 to OpenBSD's processors. |
| illumos/amd64 | GOAMD64=v1 | pkgsrc x86_64 today; IPS packages from OpenIndiana's repository after the move | In | |
| solaris/amd64 | | None | Out | Oracle's repository is to be checked for GTK 4. A command-line program could be offered without it. |
| aix/ppc64 | | None | Out | AIX has no GTK 4. A command-line program could be offered. |
| plan9/386, plan9/amd64 and plan9/arm | | None | Out | Plan 9 has no GTK. vero's Plan 9 starter is for workers with a text front end. |
| android/386, android/amd64, android/arm and android/arm64 | | None | Out | iPhone, iPad and Android come after the desktop, as decided on 11 October. vero has an Android starter. |
| ios/amd64 and ios/arm64 | | None | Out | The same. ios/amd64 is the simulator. |
| js/wasm | | None | Out | A worker in a browser cannot keep files or sign-ins. vero has a web starter for workers that need neither. |
| wasip1/wasm | | None | Out | No windows. vero has a WASI starter for workers with a text front end. |

That is 31 pairs in and 16 out. Of those out, solaris/amd64 waits only on
a check for GTK 4, and the six mobile pairs wait for their own plan. A
processor that an app cannot use is left out of that system's `arches`
in its `vero-app.toml`, as audiobooks leaves out OpenBSD's powerpc64 for
want of ffmpeg, and its README says why.

## Phase 1. vero 0.18, untagged

1. **Libraries.** Done on 12 October 2026: `vero_gtk.py` (dbb66ed),
   tested by `scripts/test-gtk-libs.sh` on Debian 13 and Ubuntu 24.04, each
   with libadwaita and plain, and on Debian 12, each with and without a
   session bus; and `VeroWpf.cs` with `VeroWebView.cs` (a9d37b3), tested by
   `scripts/test-wpf-libs.sh` in the Windows VM on arm64, x64 and x86, and
   with Windows' own notifications. `kit/update` answers their standard
   `vero.update` request (fb96814).
   `vero.py` and `Vero.cs` stay the bindings and the step
   player. Beside them, `vero_gtk.py` and `VeroWpf.cs` hold the pieces in
   the table. They are still copied into each app, not installed from pip
   or NuGet, so that every package builds without a network.
2. **Starters in two shapes.** Done on 12 October 2026 (63067fc), with
   `vero add --menubar` and `menubar = true` in `vero-app.toml` for the
   tray shape, and checked by `scripts/test-starters.sh`: both shapes
   build on the Mac and for Windows, run in the Windows VM, and run on
   Debian 13 and Debian 12. `vero add` runs `vero gen`, and the Mac starter
   uses its request types.
   `vero new` and `vero add` write either a
   window app or, with `menubar = true`, a menu bar app on the Mac and a
   tray app on Windows and Linux. The WPF starter targets .NET 10, turns on
   the Fluent theme with `ThemeMode="System"`, keeps its layout in XAML,
   puts colours and spacing in `Theme.xaml`, and uses Segoe Fluent Icons.
   The GTK starter uses the library's window, which is an
   `Adw.ApplicationWindow` with an `Adw.ToolbarView` and a header bar where
   libadwaita 1.5 or later is installed, and a plain GTK 4 window with a
   header bar where it is not. Both starters call the libraries for
   everything in the table.
3. **Dependencies worked out by vero-repo.** Done on 12 October 2026 (599e96f, 99cdee4): `cmd/vero-repo/deps.go` has the names, and the example installs from them on Debian 13 and 12, Ubuntu 24.04 and Fedora so far. `[gtk]` names what the app
   uses, such as `uses = ["adwaita", "webview", "keyring"]`, and `extra`
   lists what only that app needs, such as audiobooks' ffmpeg. vero-repo
   writes each system's package names from these and adds a font on every
   system. Sections that list every dependency in full are still accepted.
4. **`vero upgrade`.** Done on 12 October 2026 (b1132fa), and tried on copies of audiobooks and treehouse. calmtasks and calmdocs run it once their workers move to `cmd/worker`. In an existing app, from vero 0.13 onwards, it
   updates `go.mod` and the kit, sets the Xcode project's pin on vero to
   Up to Next Major, copies the bindings and libraries afresh, rewrites
   the toml's dependency lists in the new form, and reports each change.
   Where an app's code must change, such as a renamed Swift call between
   0.13 and 0.18, it says where.
5. **Sandboxed Mac apps.** Done on 12 October 2026 (d5efdd9): each program in a sandboxed app is signed with the sandbox and inherit entitlements. calmtasks proves it in Phase 4. vero-repo signs the worker inside a sandboxed
   app with the sandbox and inherit entitlements, which lets a sandboxed
   app run it as a program of its own. calmtasks no longer needs its C
   archive, and its App Store build can bundle the worker as the others
   do.
6. **Every system and processor that audiobooks runs on.** audiobooks
   ships for many processors, such as i386, armhf, riscv64 and loong64, and
   on systems with no libadwaita or one too old, such as Debian 12, the
   only Debian left for mips64el and mipsel, and OpenBSD on 32-bit ARM.
   Every app must run on all of them.

   So `vero_gtk.py` uses libadwaita 1.5 or later where it is installed, and
   plain GTK 4 where it is not. Each piece in the table works both ways:
   the window and its header bar, toasts, banners, dialogs and settings
   pages. An app builds its windows from the library's pieces, and uses a
   widget that only libadwaita has only after asking the library whether
   libadwaita is there. Apps then look like GNOME apps where libadwaita is
   installed, and like plain GTK apps elsewhere, but work the same
   everywhere.

   vero-repo makes libadwaita a dependency on each system and processor
   whose packages have 1.5 or later, and leaves it out elsewhere. vero's
   container and virtual-machine tests run every app both ways, with
   `VERO_GTK_PLAIN=1` making the library use plain GTK 4 where libadwaita
   is installed.

   The package indexes, checked for each processor on 12 October 2026,
   give these versions. "Web view" is WebKitGTK's GTK 4 build (WebKit 6.0).

   | System and processors | libadwaita | Web view |
   |---|---|---|
   | Debian 13: amd64, arm64, armhf, armel, i386, ppc64el, riscv64, s390x | 1.7.6 | Yes |
   | Debian unstable and ports: loong64, ppc64 | 1.10.0 | Yes |
   | Debian 12, all processors, and mips64el and mipsel, which Debian 13 and the ports dropped | 1.2.2, so plain GTK 4 (4.8.3, PyGObject 3.42.2) | Yes, except on mipsel |
   | Ubuntu 24.04: amd64, arm64, armhf, ppc64el, riscv64, s390x | 1.5.0 | Yes |
   | Fedora 44: x86_64, aarch64, ppc64le, s390x | 1.9.0 | Yes |
   | openSUSE Tumbleweed: x86_64, i586, aarch64, ppc64le, riscv64, armv7hl, armv6hl, s390x. Fedora has no i686, armv7hl or armv6hl, and only an unofficial riscv64, so the rpm on these processors is for openSUSE. | 1.9.4 | Yes |
   | Arch Linux and its ports: x86_64, aarch64, armv7h, powerpc64le, powerpc64, loong64, riscv64 | 1.9.0 to 1.10.0 | Yes |
   | Arch Linux 32: pentium4 | 1.5.2 | No |
   | Alpine 3.24 and edge: all nine processors | 1.9.2 and 1.10.0 | Yes |
   | Void: x86_64, aarch64, armv7l, armv6l, i686 | 1.9.1 | Yes |
   | Chimera: x86_64, aarch64, ppc64le, ppc64, riscv64, loongarch64 | 1.9.2 to 1.10.0 | Yes |
   | FreeBSD 14: amd64, aarch64, i386 | 1.8.5 | Yes |
   | FreeBSD 14: armv7 | 1.7.6 | Yes |
   | DragonFly 6.6 | 1.7.2 | Yes |
   | NetBSD 10: x86_64, i386 and aarch64 | 1.9.1, and 1.8.0 on aarch64 | No |
   | OpenBSD: amd64, aarch64 | 1.9.4 | Yes |
   | OpenBSD: i386 | 1.9.4 | No |
   | OpenBSD 7.9: riscv64 | 1.9.0 | Yes |
   | OpenBSD 7.9: arm | None, so plain GTK 4 (4.22.3) | No |
   | illumos: SmartOS's pkgsrc, used today | None, so plain GTK 4 | No |
   | illumos: OpenIndiana's own repository | 1.6.10 | Yes (2.54.1) |

   So plain GTK 4 is needed on Debian 12, which is the only Debian for
   mips64el and mipsel, on OpenBSD's 32-bit ARM, and on illumos until it
   moves. The web view is missing on NetBSD, on OpenBSD i386 and arm, on
   Arch Linux 32 and on SmartOS's pkgsrc, where pages are shown as the
   library's table says. OpenBSD 8.0 has published packages only for
   amd64, aarch64 and i386 so far, and its arm and riscv64 packages are
   checked when they appear.

   vero's tests use Debian 13 by default, with Debian 12 kept as the test
   of the plain GTK 4 path (3eea4b4).

   Done on 12 October 2026: vero-repo makes IPS packages and a static IPS
   repository, and `scripts/test-repo.sh --vm illumos` installs from it,
   runs the app's steps and updates it, in vero's OpenIndiana VM.
   OpenIndiana has the GTK 4 web view (webkitgtk2-60, 2.54.1), and its
   PyGObject is for Python 3.9 alone, which is the libraries' floor. It
   has no ffmpeg of its own that vero found, which audiobooks needs.

   illumos moves from SmartOS's pkgsrc to OpenIndiana's own repository.
   SmartOS is a server system, and its pkgsrc lacks libadwaita and a GTK 4
   web view, and its GTK 4 has crashed loading an app's icon. vero-repo
   makes IPS packages and an IPS repository for illumos, which depend on
   OpenIndiana's GTK 4 (4.16), PyGObject (3.54), libadwaita (1.6.10),
   libsecret and gnome-keyring, and the illumos test installs from it in
   vero's OpenIndiana virtual machine. Whether OpenIndiana's WebKitGTK
   2.52.5 includes the GTK 4 web view is checked there. Without it, illumos
   shows pages in the browser, as any system without a web view does.
   With its keyring, sign-ins on illumos move from a file into
   gnome-keyring. The move is not needed for the libraries, which run on
   SmartOS's plain GTK 4 as audiobooks does today, so it can come last in
   Phase 1.
7. **Typed requests generated from the worker.** Done on 11 and 12 October 2026 (9766078): `vero add`, `vero upgrade` and `vero ship` run it. The worker's handlers are
   typed in Go (`vero.Handle[Req, Rep]`), but the front ends call them by
   name with untyped JSON, and each Mac app keeps its own `Requests.swift`
   by hand. `vero gen` reads the worker's handlers and writes the matching
   types and calls for each front end: Swift structs that are `Codable`, C#
   records and Python dataclasses. `vero ship` and `vero upgrade` run it,
   so that a field renamed in Go fails to build in Swift and C# and fails
   a type check in Python, rather than failing when a test runs.
8. **Library files protected from edits.** Done on 12 October 2026 (b1132fa): `.vero-files.json` records each file's hash. Every file that vero copies
   into an app says that vero wrote it and that it is not to be edited,
   and vero records its hash. `vero upgrade` stops and names any such file
   that has been edited. An app that needs something different changes it
   through the library's settings or subclasses.
9. **Local builds numbered above the release.** Done on 12 October 2026 (e3187f0): `vero install`, and `vero local-version` for a prebuild script. A local build numbered
   below the newest release is replaced by it, because Sparkle installs
   any release with a higher number and vero keeps whichever worker has
   the higher number. treehouse found this, and its
   `macos/scripts/install.sh` numbers the app and the worker as
   `<published>.<date and time>`, which is above the release that is out
   and below the next. `vero install` does the same for every app, reading
   the published version from the app's `latest.json`, and the Mac's
   prebuild uses the same number for Xcode's own builds.

10. **Workers checked on every target.** Done on 12 October 2026 (5000cff). `ncruces/go-sqlite3` has since also run on emulated ARMv5 and ARMv6 processors. `vero ship` and `vero-repo plan`
    build the worker for every system and processor first, and name each
    one that fails and why. vero's documentation names `ncruces/go-sqlite3`
    as the SQLite for workers.
11. **Kit additions.** Done on 12 October 2026: `kit/update`'s `vero.update` (fb96814), `kit/keychain`'s password for files and its `vero.keychain`, `vero.unlock` and `vero.setPassword` requests, `kit/awake` (0feeecf), and `WorkerOptions.Lock` (4d7f4e5). The libraries ask for the password with `unlock_sign_ins` and `UnlockSignIns`. These are the Go side of the pieces in the table:
    - `kit/keychain` keeps sign-ins in a file encrypted with a key from a
      password where there is no keyring, and tells the front end that it
      needs the password;
    - `kit/update` registers a standard update request, which every
      front end's library asks;
    - a new `kit/awake` keeps the computer awake while the worker asks, and
      reports sleep and wake;
    - vero's Go package lets a worker with no app take the same lock as
      the supervisor.
12. **macOS older than Go supports.** Done on 12 October 2026 (d5efdd9), and tried on an app for macOS 12, whose two workers report 12.0 and 13.0 as their minimums. An app whose `[macos]` minimum is
    older than the current Go builds for, as calmdocs' macOS 12 is, gets
    two workers in its Mac app: one built with the current Go, and one
    built with the last Go that builds for that macOS (Go 1.26 for macOS
    12). The Swift package runs the older one only on the older macOS, so
    that everyone else runs a Go that still receives security fixes. Every
    other system's worker is built with the current Go. vero says so, and
    vero's kit and the app's worker must stay buildable with Go 1.26 for
    as long as the app supports macOS 12.
13. **Link schemes and file types.** Done on 12 October 2026 (c90eb90), with the Windows installer's entries checked in the VM. `vero-app.toml` gains `schemes` and
    `opens`, which vero-repo writes into the `.desktop` file and the
    Windows installer. The installer also registers the app's id, which
    Windows needs for notifications with buttons.
14. **Cancelling a request.** Done on 12 October 2026, asked for by
    treehouse for its grid of thumbnails. Each handler's context also ends
    when its request is cancelled. A front end cancels with
    `{"t":"cancel","id":N}`, which the host passes on; a cancel for a
    request that has finished, or that never was, is ignored. Swift cancels
    with the calling task, through `withTaskCancellationHandler`, and frees
    the waiting thread at once. C# takes a `CancellationToken`, and vero gen
    writes each method with and without one. Python and Kotlin take a
    `Cancel`. A cancelled call is not shown as a failure. An older worker
    answers the cancel with an error that is dropped. The Swift side needs
    the C archive's new `VeroCallTagged` and `VeroCancel`, so until the tag
    `scripts/build-cvero.sh` builds the archive locally, and `Package.swift`
    uses it when it is there. `scripts/test-bindings.sh` checks Swift and C#
    against a real worker; Go's and Python's own tests check the rest.
15. **A worker on disk that differs from the app's.** Done on 12 October
    2026, asked for by treehouse. `VeroWorkerBundle(..., replace:
    .whenDifferent)` replaces the worker on disk whenever it differs from
    the bundled one, whatever their versions. With the default,
    `.whenNewer`, a worker kept although it differs is logged plainly.
16. **Web views for mail, and HTML to PDF.** Done on 12 October 2026,
    asked for by treehouse. The web view helpers take a policy that the app
    sets and hand links in the app's own schemes to a callback (`on_link`,
    `LinkClicked`, `onLink`), and the Swift package gains `VeroHtmlView`.
    Each library writes HTML to PDF files offscreen, one after another
    through one hidden web view: `PdfWriter` with WebKitGTK's print
    operation, `VeroPdfWriter` with `PrintToPdfAsync`, and `VeroPDFWriter`
    with `createPDF`, whose PDF is one page as long as the message. The GTK
    and WPF library tests and `scripts/test-bindings.sh` check them.
17. **IPS publishers for illumos' dependencies.** Done on 12 October 2026.
    OpenIndiana's ffmpeg is `video/ffmpeg` in its second publisher,
    `hipster-encumbered`, which the site's install commands add before
    installing an app that uses ffmpeg. OpenIndiana's `pkg` cannot send a
    password, so an app with private downloads, as audiobooks has, takes
    the token in the repository's path instead.

vero's own tests check both starter shapes on every system that vero
tests, and `vero ship counter.go` is run again as a new app would be.
On Linux they run each library piece in four cases: with libadwaita,
without it (`VERO_GTK_PLAIN=1`), without a session bus, and without a web
view. The BSD and illumos virtual machines run the same, and run a worker
that uses SQLite.

## Phase 2. audiobooks, a window app

audiobooks is small, so it proves `vero upgrade` and the window shape
quickly.

1. Run `vero upgrade`.
2. Move `gtk/main.py` onto `vero_gtk.py`, and `windows/` onto
   `VeroWpf.cs`, .NET 10 and the Fluent theme. Amazon's sign-in uses the
   library's web page.
3. Move every front end, the Mac's included, to the generated requests.
4. Run `vero ship --no-release` on every system.
5. Update the README's list of systems: libadwaita where the system has
   it, the GTK 4 web view and keyring on OpenIndiana, and any system whose
   features differ, with the reason, as it already explains FreeBSD on
   32-bit ARM.

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
   Its 43 handlers that reply with untyped JSON reply with Go types first.
4. Register `mailto:` and `.eml`, `.ics` and `.vcf` through `schemes` and
   `opens`, with the worker's requests to open them, and offer to make
   treehouse the default mail app.
5. Move the vault's password into `kit/keychain`'s password form, which
   the library asks for at start-up where there is no keyring.
6. Run `vero ship --no-release` on every system.

## Phase 4. calmtasks, a sandboxed menu bar app

1. Move the worker from `core/` to `cmd/worker` in a module at the top of
   the repository, drop `libcalmtaskscore.a` and `build-core.sh`, and bundle
   the worker as a program, signed for the sandbox.
2. Move from `modernc.org/sqlite` to `ncruces/go-sqlite3`, so that the
   worker builds for every system.
3. Move the export's download from Swift into the worker, which saves the
   file where the person chooses.
4. Run `vero upgrade` from 0.14, and move the Mac app onto vero's Swift
   package as the other apps use it.
5. Move the Mac app to the generated requests.
6. Add a demo mode and a `[test]` with `VERO_PROFILE`.
7. Run `vero add desktop` with `menubar = true`, which gives calmtasks its
   first Windows and Linux apps, as tray apps. The tray window holds what
   the Mac's menu bar window holds: quick add, the flagged tasks, the sync
   status, Settings and Quit. `calmtasks://` and opening files are
   registered, and Ctrl+Z undoes through the library's undo stack.
8. Run `vero ship --no-release` on every system. Direct releases move to
   vero. The App Store build stays with fastlane, without Sparkle.

## Phase 5. calmdocs, a menu bar app whose worker also runs alone

1. Move the worker from ~/src/syncclient into sync-macos as `cmd/worker`,
   drop its own updates and the separate publishing of the worker to the
   server, and replace them with vero's updates.
2. Make the worker build and run on every system: `kit/awake` in place of
   the Mac's sleep notifier and `caffeinate`, `ulimit` fixed for 32-bit
   systems and Windows, polling in place of FEN on illumos, `osdetails`
   accepting any system, and `ncruces/go-sqlite3` in place of
   `mattn/go-sqlite3`.
3. Keep the sign-in in `kit/keychain` rather than in `conf.yaml`, and the
   settings in `kit/profile`'s folder. Opening folders and pages moves from
   the worker to the front ends. The worker takes the single-worker lock
   when it runs with no app.
4. Run `vero upgrade` from 0.13, and move the Mac app to the generated
   requests. Sparkle's rule of installing only while nothing syncs moves
   into the Swift package's update setup.
5. Add a demo mode and a `[test]` with `VERO_PROFILE`.
6. Run `vero add desktop` with `menubar = true`, as for calmtasks, with a
   650 by 500 tray window, and a tray icon that shows syncing and idle.
7. Run `vero ship --no-release` on every system. Releases move to vero, and
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
4. **Every system that audiobooks runs on.** Every app runs on every
   system and processor on which audiobooks runs today. libadwaita is used
   where it is installed and is not required, so Debian 12 (with mips64el
   and mipsel) and OpenBSD on 32-bit ARM stay. This replaces the earlier decision to require
   libadwaita 1.5 and drop Debian 12, and was decided on 12 October 2026.

## Decisions from the check

These come from checking the plan against all four apps and a new app,
and were approved on 12 October 2026.

5. **Left out of the first Windows and Linux versions of calmtasks:** the
   map and the current location (a task's place is shown as text, with a
   link to a map in the browser), a quick-add shortcut that works from any
   app, and stand-ins for Spotlight, Services and Shortcuts. Each can come
   later in the app alone.
6. **Notifications with buttons on Windows** use Windows' own notification
   API, which adds Microsoft's Windows SDK library to the app: 24 MB,
   measured on 12 October 2026, which takes a self-contained arm64 build
   from 192 MB to 216 MB. Only an app whose `[wpf] uses` names notifications gets it, which
   is treehouse today. Others show notifications without buttons from the
   tray icon.
7. **The tray on Windows** keeps Windows Forms' `NotifyIcon`, which
   treehouse uses today and which works on every Windows processor.
8. **Without a session bus**, there is no tray on Linux, the BSDs or
   illumos. A tray app then opens as an ordinary window, and a window app
   that runs in the background says, before its window closes, that
   closing it stops the app.
9. **calmdocs on macOS 12.** calmdocs keeps macOS 12, probably even after
   Go 1.28 is released, when Go 1.26 stops receiving security fixes. Only
   the people on macOS 12 then run a worker built with an unsupported Go,
   through the second Mac worker in Phase 1 item 12. Decided on 12 October
   2026.
