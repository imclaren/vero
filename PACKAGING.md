# Taking your app to more platforms, and packaging it

[Back to vero's README](README.md)

Start with [a worked example](#a-worked-example), which takes a small app
from macOS to an install page in a few commands. The rest of the guide has
two parts. [Taking your app to more
platforms](#taking-your-app-to-more-platforms) adds front ends for the
systems you want; skip it if your app already runs everywhere you want it
to. [Creating app installers](#creating-app-installers) then builds the
installers for every system your app has, and the website people install
and update from.

## A worked example

The whole journey in a few minutes, on your Mac: a small app that runs on
macOS gains front ends for Linux and Windows, gets packaged, and gets an
install page. vero's example worker stands in for your app's. The rest of
this guide explains each step and the options.

**1. Install vero's tools,** and fetch vero itself for its packaging script.
Go puts the tools in `$(go env GOPATH)/bin`, which needs to be on your
`PATH`.

```bash
go install github.com/imclaren/vero/cmd/vero@latest
go install github.com/imclaren/vero/cmd/vero-repo@latest
go install github.com/imclaren/vero/kit/cmd/vero-site@latest
git clone https://github.com/imclaren/vero ~/vero
```

**2. Make the app:** a Go module with a worker and an icon.

```bash
mkdir hello && cd hello
go mod init example.com/hello
mkdir -p cmd/worker && cp ~/vero/example/worker/main.go cmd/worker/
cp ~/vero/example/icon.png .
go get github.com/imclaren/vero@latest
```

**3. Add front ends for macOS, Linux and Windows.** `vero add` reads the
worker and writes a starter app for each, which shows the worker's state
and has a button for each request. It also writes `vero-app.toml`, which
describes the app to the packaging, and `PORTING.md`, a checklist for your
own app.

```bash
vero add desktop
```

It asks for the three things the packaging needs that the worker cannot
tell it - a one-line summary, a description, and your name and email as
the publisher - and writes them into `vero-app.toml`. (`--summary`,
`--description` and `--publisher` answer from a script.)

**4. Make a signing key, then build the installers.** The key signs every
release, so that people's systems trust your updates. This builds the
Debian packages and the Mac disk image; leave out `--targets` to build
everything your Mac has the tools for.

```bash
vero-repo key --dir ~/vero-keys/hello --name "Your Name" --email you@example.com
~/vero/scripts/package.sh --app vero-app.toml --version 1.0.0 --targets deb,macos
```

**5. Build the install site, and look at it.** `vero-repo build` makes
`dist/site` and checks it. `vero-site` serves it.

```bash
vero-repo build --app vero-app.toml --key ~/vero-keys/hello --url http://localhost:8080
vero-site -dir dist/site -addr localhost:8080
```

Open the address `vero-site` prints: <http://localhost:8080>, or another
port if something on your Mac has that one. The page shows the install
commands for the visitor's system, and `/download?for=mac` hands out the
disk image.

**6. Release an update.** Change the app, then run steps 4 and 5 again
with `--version 1.0.1`, skipping `vero-repo key`. On a real server,
upload `dist/site` and use its address as `--url`.

## Taking your app to more platforms

An app written for one system - a Mac menu bar app, say - can run on the
others with a worker it already has. `vero add` writes a starter front end
for each system you ask for, wired to your worker, and vero's `kit`
packages do the few things a worker does one system's way. Then the
installers above package all of them.

### 1. Move what is one system's onto vero's packages

The `kit` module, `github.com/imclaren/vero/kit`, holds what a worker needs
from the system it runs on, with one API for all of them. It is a module of
its own, so that vero's library keeps no dependencies; `go get
github.com/imclaren/vero/kit` brings it in.

| Package | What it does | Where |
|---|---|---|
| `kit/keychain` | Keeps sign-ins and other secrets | The login keychain on macOS, the Credential Manager on Windows, the Secret Service on Linux and the BSDs, and a file only this user can read elsewhere |
| `kit/autostart` | Opens the app at sign-in | A LaunchAgent on macOS (from an app bundle, `SMAppService` from Swift is better), the Run key on Windows, an autostart `.desktop` file on Linux and the BSDs |
| `kit/notify` | Shows a notification from the worker | The desktop's notification service on Linux and the BSDs, a toast on Windows; on macOS and iOS notifications come from the app, so keep what is new in the state and let the front end notify |
| `kit/update` | Says whether a newer version is on your site | Reads the `latest.json` and the Sparkle appcast that `vero-repo build` writes |
| `kit/tools` | Finds helper programs the app ships, such as ffmpeg | Wherever the app is installed on each system, or a folder you name |
| `kit/site` | Serves your install site from your own Go server, with `/download?for=mac` (or `windows`, `linux`, `pkg` and so on) sending each visitor to their installer | For an app with a server already, or one whose downloads are for people who have signed in (`site.Private`); the site is plain files, so this is optional |
| `kit/cmd/vero-site` | Serves your install site on its own, with an HTTPS certificate from Let's Encrypt | For an app without a web server: `vero-site -dir /srv/myapp -domain downloads.example.com` |

Each builds for every system vero does, without cgo.

### 2. Add the front ends

In your app's folder, the one with its `go.mod`:

```bash
go install github.com/imclaren/vero/cmd/vero@latest
vero add                 # the desktop: macOS, GTK (Linux, the BSDs, illumos) and WPF (Windows)
vero add mobile          # Android and iOS
vero add all             # everything vero has a front end for
vero add all --except plan9,wasi
vero add windows android # any mix of groups and systems
```

It reads your worker - the state it pushes and the requests it handles -
and writes a starter for each system: a window that shows every field of
the state and a control for every request, so that it runs against your
worker as it is. Each starter's README says how to build and run it, with
vero's binding for that language copied beside it. Front ends you already
have are left alone, and each new one is added to `vero-app.toml` (made
if you have none), so that the installers above build them.

The starters are a first draft to make your own, not a design: the GTK one
is Python, the Windows one C#, the macOS and iOS ones SwiftUI, the Android
one Kotlin, the browser, WASI and Plan 9 ones Go. The shapes of your state
and requests are written out in each language, for the window you build
next.

### 3. Work through PORTING.md

`vero add` also writes `PORTING.md`: what in your worker needs attention
on the new systems, found by reading its code. cgo, which stops the worker
being cross-compiled from one Mac; programs it starts, which have to be
shipped for each system and which iOS and a browser forbid; listening on a
port, which the worker cannot do inside an app on iOS or in a browser;
secrets, sign-in items and notifications done one system's way, with the
`kit` package to use instead. Each with the files and lines.

On iOS and in a browser the worker runs inside the app, since neither
lets an app start a program. For those two, `vero add` moves the worker's
code into `internal/worker`, a package with a `Serve(in io.Reader, out
io.Writer) error` that the two front ends call, and leaves the command as
a few lines that parse the flags and call it; nothing else changes, and
the other systems go on starting the command. It does this for a worker
whose `main` makes a `vero.WorkerOptions`, calls `vero.NewWorker` with
it and ends with `Serve`, which is what vero's example and its guide
produce. For a `main` of another shape it says so, leaves the worker
where it is, and the two front ends get a placeholder with a note saying
what to point them at; `--keep-worker` asks for that in any case.

### 4. Ship helper programs for each system

A worker that runs a program it ships - ffmpeg, say - needs a build of it
for each system and architecture, and finds it with `kit/tools`, which
looks wherever the app is installed. (How the installers carry them is
coming.)

### 5. Release as usual

`scripts/package.sh --app vero-app.toml --version X`, then `vero-repo
build`, then upload the site: see [Creating app
installers](#creating-app-installers), below. The new systems' packages and
instructions are in the same site.

### From SwiftUI to the others

For a front end of your own, the same ideas in each toolkit:

| SwiftUI | GTK 4 (Python) | WPF (C#) |
|---|---|---|
| `WindowGroup`, `MenuBarExtra` | `Gtk.ApplicationWindow`; a status icon needs an extension on GNOME, so a window is the usual choice | `Window`; a tray icon with `NotifyIcon` from Windows Forms |
| `List`, `ForEach` | `Gtk.ListBox` with a row per item, or `Gtk.ListView` for many | `ListBox` or `ItemsControl` with an `ItemTemplate` |
| `NavigationSplitView` | `Gtk.Paned` with a sidebar `Gtk.ListBox` | a `Grid` with a `GridSplitter` |
| `.sheet`, `.alert` | `Gtk.Dialog`, `Gtk.AlertDialog` | a `Window` shown with `ShowDialog`, `MessageBox` |
| `@StateObject` model fed by events | `run_in_thread` from `vero.py` with `GLib.idle_add` | `await foreach` over `VeroClient.Events()` and `Dispatcher.Invoke` |
| a `WKWebView` | `WebKit.WebView` from WebKitGTK 6 | `WebView2` |

## Creating app installers


vero builds your app's installers, and the website your users install
them from and get updates from. You describe your app once, in a file
called `vero-app.toml`. vero then makes the installers, and a folder of
plain files to put on any web host. It holds a page telling people how to
install your app on their system, signed repositories that their systems
update from, and the downloads.

A release never needs a virtual machine, a container or anything but your
Mac: the one exception is a Flatpak, which needs Docker and is made only
if you ask for it. vero's own tests, below, do use VMs and containers, to
check that each system installs and updates from a site; you don't have to
run them.

| System | What your users get | What you need on your Mac |
|---|---|---|
| Debian, Ubuntu | an apt repository: install once, then their usual updates | Go |
| Fedora, openSUSE | an rpm repository, the same | Go |
| Arch Linux | a recipe for the AUR | Go |
| Any Linux, with Flatpak | a Flatpak repository, if you ask for one | Docker and colima |
| FreeBSD, DragonFly | a pkg repository | Go |
| NetBSD, illumos | a pkgsrc repository, for pkgin | Go |
| OpenBSD | signed packages for pkg_add | Go |
| macOS | a disk image, a Sparkle appcast for updates, and a Homebrew cask | Xcode; a Developer ID and an Apple account to notarise |
| Windows | an installer, and a winget manifest; an MSIX for the Microsoft Store if you ask | makensis and dotnet |
| Android | a signed .apk, and an .aab for Google Play if your build makes one | the Android SDK and a JDK |
| iPhone and iPad | an .ipa, for the App Store or TestFlight | Xcode and an Apple account |
| In a browser | the page, served from the site | Go |
| In a terminal, with WASI | bundles for each desktop, whose worker is one worker.wasm | Go |
| Plan 9 | a bundle with an install script | Go |

Each row is made when `vero-app.toml` has its section. What the table
calls Go is Go alone: vero's packaging tool, `vero-repo`, makes every
package, repository, signature and index itself.

### Try it on the example

vero's example describes itself in
[`example/vero-app.toml`](example/vero-app.toml), with a section for
every one of its front ends. From vero's folder, this builds its
installers into `dist/packages`, then its site into `dist/site`:

```bash
scripts/package.sh --app example/vero-app.toml --version 1.0.0
(cd cmd/vero-repo && go install .)
vero-repo key --name "Example Publisher" --email you@example.com --dir ~/.cache/vero/example-key
vero-repo build --app example/vero-app.toml --key ~/.cache/vero/example-key --url https://example.com/vero-example
```

`go install` puts `vero-repo` in Go's `bin` folder, which needs to be on
your `PATH`. `vero-repo key` won't replace a key it made before, so the
next time, start from `vero-repo build`. Use `--targets` to build only
some systems, which is handy on a Mac without the Android SDK, say:
`scripts/package.sh --app example/vero-app.toml --version 1.0.0 --targets deb,macos`.

[`scripts/test-repo.sh`](scripts/test-repo.sh) checks all of it for real.
It builds the example's site, serves it from your Mac, and installs the
example from it on a clean system, as the site tells people to. Then it
releases version 1.0.1 and checks that the system's own updates bring it.
It tries Debian, in a container, unless you say otherwise:

```bash
scripts/test-repo.sh --image ubuntu:24.04
scripts/test-repo.sh --image fedora:latest
scripts/test-repo.sh --image opensuse/tumbleweed
scripts/test-repo.sh --image archlinux
scripts/test-repo.sh --flatpak
scripts/test-repo.sh --vm freebsd         # also netbsd, openbsd, dragonfly, illumos
scripts/test-repo.sh --mac                # the disk image, on this Mac, and the appcast after an update
scripts/test-repo.sh --vm windows         # the installer and the MSIX, in vero's Windows VM
```

The containers need Docker and colima. A `--vm` test uses the system's VM,
which its `run-*.sh` script makes the first time; that downloads the
system and its GTK, a few gigabytes. The Windows VM has no way in but a
disc and no way out but its screen, so that test leaves a screenshot of
its results for you to read.

### Your own app

Run these from your app's folder, the one with its `go.mod`.

**1. Describe your app.** Copy
[`example/vero-app.toml`](example/vero-app.toml) beside your `go.mod`, and
change it to describe your app. The comments in it explain each setting.
Paths in it are relative to the file. Your worker's main package needs a
`var version`, which the build sets.

Each system names its packages differently, so `vero-app.toml` says what
your app needs from each, in its own section: `[gtk.deb]`, `[gtk.rpm]`,
`[gtk.arch]`, `[gtk.freebsd]` and so on. Leave out the sections of the
systems you don't want; a system is packaged for only when its section is
there. `[needs]` says what the app asks of the system, such as the
network.

**2. Make a signing key, once.**

```bash
vero-repo key --dir ~/vero-keys/myapp --name "Your Name or Company" --email you@example.com
```

This makes the folder you name with `--dir`. Without `--dir`, it's a
folder named after you in `~/Library/Application Support/vero-repo` on a
Mac, or `~/.config/vero-repo` on Linux. Its `private.asc` signs every
release, so that people's systems can tell your updates are really yours;
`signify.sec` signs the OpenBSD packages; `sparkle.sec` signs Mac updates;
and `android.keystore`, made the first time an Android app is built, signs
it. Back the folder up, keep it out of your repository, and never publish
anything in it but `key.asc`, the public half, which your site publishes.

If your Mac app already updates itself with Sparkle, keep its key, so that
copies already installed go on updating: `vero-repo key --dir DIR
--import-sparkle FILE`, where FILE holds the private key Sparkle's
`generate_keys -x` exported. Older Sparkle keys, which were kept
expanded rather than as a seed, are taken too.

**3. Build the installers.**

```bash
path/to/vero/scripts/package.sh --app vero-app.toml --version 1.2.3
```

This builds them into `dist/packages`, each for both x86_64 and ARM64
processors where the system has both. Use `--targets` to build only some
of them: `deb`, `rpm`, `flatpak`, `freebsd`, `dragonfly`, `netbsd`,
`illumos`, `openbsd`, `macos`, `windows`, `msix`, `android`, `ios`, `web`,
`wasi`, `plan9`; or `linux` for the `.deb` and `.rpm`, and `bsd` for the
BSDs and illumos. Use `--ldflags` to build more into your worker, such as
API keys from a file outside your repository.

**4. Build the site.**

```bash
vero-repo build --app vero-app.toml --key ~/vero-keys/myapp \
    --url https://example.com/myapp
```

This builds the site into `dist/site`. `--url` is where the site will be,
because the commands on its page and its repositories need to know. At the
end, `build` checks the site as `vero-repo check` would: that every
signature verifies with the keys the site publishes, that every index's
sizes and hashes match its files, that every link and manifest points at
a file that's there, that each package holds your worker and front end,
and that no system's version has gone backwards, which would leave people
never offered the update. It refuses to finish otherwise.

**5. Put it online.** Upload the contents of `dist/site` to any static web
host, so that they appear at the `--url` you gave. Then send people to
that address, where the page shows the commands for their system first.

If you'd rather run your own server, `vero-site` serves the folder by
itself, with the page, the repositories, `/download?for=SYSTEM` links and
`latest.json`. With `-domain` it gets an HTTPS certificate from Let's
Encrypt for that name:

```bash
go install github.com/imclaren/vero/kit/cmd/vero-site@latest
vero-site -dir /srv/myapp -domain downloads.example.com
```

To publish a release, copy the new `dist/site` over the old one, for
example with `rsync -a --delete dist/site/ server:/srv/myapp/`. If your
app already has a Go web server, serve the folder from it with
`kit/site` instead.

**6. Release an update.** Change your version, then repeat steps 3 to 5,
building into the same `dist/site`. vero keeps the three newest versions
of each installer; use `--keep` to change that. People get the update
with their usual updates: apt, dnf, zypper, pkg, pkgin and Flatpak find it
by themselves, a Mac app with Sparkle reads the appcast, and OpenBSD's
`pkg_add -u` does with the folder the page says to give it. On Arch, they
build the recipe again. Your app can read `latest.json` from the site to
tell Windows and Android users that a new version is out.

### macOS

`[macos]` names a SwiftPM package (`folder` and `product`) or an Xcode
project (`folder`, `project` and `scheme`). vero builds it for Apple
silicon and Intel in one app, puts the worker in `Contents/Resources`,
where vero's Swift package looks for it, and makes a disk image, with a
link to Applications to drag the app to. `pkg = true` makes an installer
package too, which installs the app into `/Applications`, never over a
copy somewhere else, and makes it the property of whoever installed it,
so that Sparkle can update it without an administrator's password.

The app's versions are always the release's: `CFBundleShortVersionString`
is `--version`, and `CFBundleVersion` is `--build`, or the version when
there's no build number, whatever the Xcode project says.

- **Files to bundle.** `resources` lists more files for
  `Contents/Resources`: helper programs your app runs, and their licences,
  say. `prebuild` is a command run first, in the app's folder, which can
  make or fetch them. `prepare` runs before each architecture is built,
  with `GOARCH` and `CC` set, for anything that has to be built per
  architecture.
- **A worker that needs cgo**, for the keychain, say: set `cgo = true` in
  `[worker]`. The Mac worker is then built with Xcode's clang for each
  architecture and joined, universal; everywhere else it's built without
  cgo, as before.
- **Signing.** With nothing set, the app is signed ad hoc, which runs on
  any Mac once whoever opens it allows it in System Settings, under
  Privacy & Security; the site's page tells them. To sign it properly, put
  your Developer ID in the environment: `VERO_MAC_IDENTITY="Developer ID
  Application: Name (TEAMID)"`. The app is signed from the inside out,
  with the hardened runtime: first every program and library in it, then
  the bundles around them, deepest first (such as Sparkle's helper apps
  and services, then `Sparkle.framework`), then the app itself, then the
  disk image. The result is checked with `codesign --verify --deep
  --strict`, as Gatekeeper and notarisation check it. With
  `VERO_NOTARY_PROFILE` naming a profile you made with `xcrun notarytool
  store-credentials`, the disk image is notarised and stapled too.
  `entitlements` names a plist of entitlements for the app itself.
  `VERO_MAC_INSTALLER` signs the `.pkg`, which is then notarised as well.
- **Updates.** The site's `macos/appcast.xml` is a Sparkle feed, each
  release signed with your Sparkle key. For your app to use it, its
  `Info.plist` needs `SUFeedURL`, the appcast's address, and
  `SUPublicEDKey`, the key's public half, which `vero-repo key` prints.
  With `feed` and `sparkle_public_key` in `[macos]`, vero writes them into
  the `Info.plist` it makes for a SwiftPM app; an Xcode project's own
  `Info.plist` has to say them itself. vero's example doesn't embed
  Sparkle, so its appcast is only checked, not fetched by the app. Sparkle
  compares `CFBundleVersion`; it's the version unless you give
  `package.sh --build N` to number builds on their own. If apps already
  installed look for the appcast somewhere else on the site, `appcast` in
  `[macos]` writes it there too, such as `appcast = "appcast.xml"` for the
  site's top.
- **Release notes.** `vero-repo build --notes "..."` (or `--notes-file`)
  makes a page of what's new, which the appcast links, so Sparkle shows it
  in its update prompt. Paragraphs are split by blank lines, and lines
  starting with `- ` become a list.
- **Downloads served elsewhere.** `download_url` in `[macos]` points the
  appcast and the cask at another server, if the disk images don't live
  on the site. `vero-repo build --no-page` leaves out the public page and
  `install.sh`, for an app distributed privately.
- **Homebrew.** `homebrew/NAME.rb` is a cask for the newest release, to
  put in a tap of your own or submit to homebrew-cask.

### Windows

`[wpf]` names the WPF project and the program it builds. vero publishes it
self-contained, so people need no .NET, and makes an installer for x64 and
one for ARM64, with `makensis`, which runs on a Mac. The installers aren't
signed, because signing needs your own code-signing certificate; until
you sign them, with `signtool` on Windows or `osslsigncode` on a Mac,
Windows SmartScreen warns people who download them, and the page tells
them how to get past it.

- **winget.** `winget/manifests/...` holds the three manifests winget
  installs the newest release from. To list your app, fork
  [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs), copy
  the folder under its `manifests`, and open a pull request; after each
  release, do the same for the new version. `winget_id` in `[wpf]` sets
  the identifier, `Publisher.App`, which is made from your names
  otherwise.
- **The Microsoft Store.** `[wpf.msix]` makes an MSIX, with `build = true`
  or `--targets msix`. Reserve your app's name in
  [Partner Center](https://partner.microsoft.com/dashboard), which gives
  you its `identity_name` and `publisher`; the Store signs what you submit,
  so vero leaves the MSIX unsigned. It declares a startup task when
  `[needs]` has `autostart`, and the internet when it has `network`. The
  package is stored uncompressed, as the Store expects, so it's bigger
  than the installer. To install one yourself for testing, sign it with
  `signtool` and a certificate your PC trusts, as
  `scripts/test-repo.sh --vm windows` does.

### Android, iPhone and iPad

`[android]` names the folder, the command that builds the app, and the
aligned, unsigned `.apk` it makes; vero signs it, and an `.aab` for Google
Play if the build makes one and `aab` names it. The keystore is yours
(`VERO_ANDROID_KEYSTORE`, `VERO_ANDROID_KEY_ALIAS` and
`VERO_ANDROID_PASSWORD`), or, without those, one vero makes beside your
signing key the first time and keeps: Android installs an update only
when the same key signed it, so never lose it. The build needs the Android
SDK and a JDK; `JAVA_HOME` says where the JDK is, or Homebrew's `openjdk`
is used.

`[ios]` names the folder, the build, and the app it makes for the
Simulator, which vero zips for testing. An app reaches phones only through
the App Store or TestFlight: name the app your build makes for devices as
`device_app`, put your `VERO_IOS_IDENTITY` ("Apple Distribution: ...") and
the path of its provisioning profile in `VERO_IOS_PROFILE`, and vero
signs it and makes the `.ipa` to upload with Transporter. `app_store_url`
is what the site's page links to.

### The browser, WASI and Plan 9

`[web]` names the folder, its build, and the files it makes, which the
site serves at `web/`, so the page just links to it. `[wasi]` names a
terminal front end and the worker it drives, which is built once as
`worker.wasm` and bundled with the front end built for each desktop;
people run it with `wasmtime`. `[plan9]` names the Plan 9 front end,
bundled with the worker and an `rc` script that installs them.

### The AUR, if you like

Arch users can build the recipe from your site, as its page says. To list
your app on the Arch User Repository, so that AUR helpers find it, make an
account at [aur.archlinux.org](https://aur.archlinux.org) and add your SSH
key to it. Then publish `dist/site/aur` there, as `NAME-bin`:

```bash
git clone ssh://aur@aur.archlinux.org/myapp-bin.git
cp dist/site/aur/PKGBUILD dist/site/aur/.SRCINFO myapp-bin/
cd myapp-bin && git add PKGBUILD .SRCINFO && git commit -m "Release 1.2.3" && git push
```

Do the same after each release.

### What's in the site

```
dist/site/
  index.html     how to install, on each system
  install.sh     installs in one command, on every Unix the site covers
  key.asc        your public key
  latest.json    the newest version, and where each download is
  apt/           the Debian and Ubuntu repository, signed
  rpm/           the Fedora and openSUSE repository, signed, and the .repo file that adds it
  aur/           the Arch recipe, PKGBUILD and .SRCINFO
  freebsd/       the FreeBSD repository for each processor, signed; its .conf; key.pem
  dragonfly/     the same, for DragonFly
  netbsd/        the NetBSD pkgsrc repository for each processor
  illumos/       the illumos pkgsrc repository, signed, and key.gpg, which the page adds to pkgsrc's keyring
  openbsd/       the OpenBSD packages for each processor, signed, and the key that checks them
  flatpak/       if you made one: the Flatpak repository, signed; the .flatpakref that installs
                 from it in one click; and the newest Flatpaks as single files
  macos/         the disk images, and appcast.xml, which Sparkle reads
  homebrew/      a cask for the newest release
  windows/       the Windows installers
  winget/        the winget manifests for the newest release
  android/       the signed .apk
  web/           the browser front end, as a page
  wasi/          the WASI bundles, one for each desktop
  plan9/         the Plan 9 bundles
```

The Flatpak repository holds many small files, which your web host has to
serve exactly as they are. `vero-repo check --app vero-app.toml --url URL
[--key DIR] dist/site` checks a site on its own, as `build` does at the
end; give `--key` and it checks the appcast's signatures too.

### Notes

The illumos packages are signed, because pkgsrc there, as SmartOS sets it
up, installs only signed packages: the page's commands add your key to its
keyring first. The NetBSD packages aren't signed, since NetBSD's pkgsrc
doesn't check unless it's set up to, so your site's HTTPS vouches for
them. Everything else that installs from your site checks your signature.

`scripts/package.sh` runs `vero-repo package`, which builds the Windows
installers with [`package-windows.sh`](scripts/package-windows.sh).
[`package-linux.sh`](scripts/package-linux.sh) builds `.deb` files on its
own, from flags rather than `vero-app.toml`. Each script's header lists
its options. [`run-linux.sh`](scripts/run-linux.sh) shows your Linux app
running before you package it.
