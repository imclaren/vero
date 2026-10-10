# vero-repo

The packaging: every installer, repository, signature and index, made in
Go, and the site people install and update from. [PACKAGING.md](../../PACKAGING.md)
is the guide; `vero release` runs this, and this is the reference for it.

You describe your app once, in `vero-app.toml`, and `vero release`
makes the installers and a folder of plain files to put on any web
host: a page telling people how to install your app on their system,
signed repositories that their systems update from, and the downloads.

A release never needs a virtual machine, a container or anything but your
Mac: the one exception is a Flatpak, which needs Docker and is made only
if you ask for it. vero's own tests, below, do use VMs and containers, to
check that each system installs and updates from a site; you don't have to
run them.

| System | What your users get | What you need on your Mac |
|---|---|---|
| Debian, Ubuntu and Chromebooks (Linux on ChromeOS, turned on in Settings → Developers) | an apt repository: install once, then their usual updates | Go |
| Fedora, openSUSE | an rpm repository, the same | Go |
| Arch Linux | a pacman repository, the same, and a recipe for the AUR | Go |
| Alpine Linux | an apk repository, if you ask for one | Go |
| Void Linux | an xbps repository, if you ask for one | Go |
| Chimera Linux | an apk repository, if you ask for one | Go |
| Any Linux, with Flatpak | a Flatpak repository, if you ask for one | Docker and colima |
| FreeBSD, DragonFly | a pkg repository | Go |
| NetBSD, illumos | a pkgsrc repository, for pkgin | Go |
| OpenBSD | signed packages for pkg_add | Go |
| macOS | a disk image, a Sparkle appcast for updates, and a Homebrew cask | Xcode; a Developer ID and an Apple account to notarise |
| Windows | an installer, and a winget manifest; one for 32-bit Windows, and an MSIX for the Microsoft Store, if you ask | makensis and dotnet |
| Android | a signed .apk, and an .aab for Google Play if your build makes one | the Android SDK and a JDK |
| iPhone and iPad | an .ipa, for the App Store or TestFlight | Xcode and an Apple account |
| In a browser | the page, served from the site | Go |
| In a terminal, with WASI | bundles for each desktop, whose worker is one worker.wasm | Go |
| Plan 9 | a bundle with an install script | Go |

Each row is made when `vero-app.toml` has its section. What the table
calls Go is Go alone: vero's packaging tool, `vero-repo`, makes every
package, repository, signature and index itself.

## Try it on the example

vero's example describes itself in
[`example/vero-app.toml`](../../example/vero-app.toml), with a section for
every one of its front ends. From vero's folder, this builds its
installers into `dist/packages`, then its site into `dist/site`:

```bash
(cd cmd/vero && go install .)
vero release --app example/vero-app.toml
```

`go install` puts `vero` in Go's `bin` folder, which needs to be on your
`PATH`. Use `--targets` to build only some systems, which is handy on a
Mac without the Android SDK, say: `vero release --app
example/vero-app.toml --targets deb,macos`.

## macOS

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
- **Signing.** The app is signed with the Developer ID Application
  certificate in your keychain. When there are several, or you want
  another, `VERO_MAC_IDENTITY="Developer ID Application: Name (TEAMID)"`
  chooses one. With none, the app is signed ad hoc, which runs on any Mac
  once whoever opens it allows it in System Settings, under Privacy &
  Security, as the site's page tells them. The app is signed from the inside out,
  with the hardened runtime: first every program and library in it, then
  the bundles around them, deepest first (such as Sparkle's helper apps
  and services, then `Sparkle.framework`), then the app itself, then the
  disk image. The result is checked with `codesign --verify --deep
  --strict`, as Gatekeeper and notarisation check it. The disk image is
  notarised and stapled too, with the notarytool profile named `vero`,
  which `vero setup` makes, or the one `VERO_NOTARY_PROFILE` names.
  `entitlements` names a plist of entitlements for the app itself. The
  `.pkg` is signed with the keychain's Developer ID Installer certificate,
  or the one `VERO_MAC_INSTALLER` names, and is then notarised as well.
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

## Windows

`[wpf]` names the WPF project and the program it builds. vero publishes it
self-contained, so people need no .NET, and makes an installer for x64 and
one for ARM64, with `makensis`, which runs on a Mac. `arches = ["x64",
"arm64", "x86"]` adds one for 32-bit Windows, with the worker built for
386; winget lists each. The installers aren't
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
  `signtool` and a certificate your PC trusts, or register its files in
  developer mode, as `scripts/test-repo.sh --vm windows` does.

## Android, iPhone and iPad

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

## The browser, WASI and Plan 9

`[web]` names the folder, its build, and the files it makes, which the
site serves at `web/`, so the page just links to it. `[wasi]` names a
terminal front end and the worker it drives, which is built once as
`worker.wasm` and bundled with the front end built for each desktop;
people run it with `wasmtime`. `[plan9]` names the Plan 9 front end,
bundled with the worker and an `rc` script that installs them.

## Arch Linux, Alpine, Void and Chimera

Arch users add your pacman repository, as the site's page says: pacman-key
trusts your key, `/etc/pacman.conf` gains a section named after your app,
whose `Server` is `.../arch/$arch`, and `pacman -Syu` keeps the app up to
date. pacman checks the signature of every package and of the index. The
packages are made by default, with the `.deb` and the `.rpm`; `[gtk.arch]`
says what they depend on, and `optdepends` what they can use if it's
there, as `"webkitgtk-6.0: signing in inside the app"`.

`[gtk.alpine]`, `[gtk.void]` and `[gtk.chimera]` make packages for
Alpine, Void and Chimera, with what each depends on, by its own names. apk
and xbps check every package with the RSA half of your signing key, which
the site publishes for them, and the page's commands put where each looks
for the keys it trusts: `/etc/apk/keys` for apk, and `/var/db/xbps/keys`
for xbps, named by its fingerprint. The same packages serve Void's glibc
and musl systems, since the worker needs neither.

Chimera's apk is apk-tools 3, which reads Alpine's kind of package and
index, signed as Alpine's are, besides its own newer kind; vero makes
Alpine's kind for it, in a repository of its own, with Chimera's names for
what the app needs. Chimera has FreeBSD's `fetch`, rather than curl or
wget, which a minimal install adds with `apk add chimerautils-extra`.

A private site can put a password in the address any of them fetches
from, as `https://NAME:PASSWORD@example.com/myapp/arch/$arch` in
`pacman.conf`, or the same in `/etc/apk/repositories` or
`/etc/xbps.d/NAME.conf` or `/etc/apk/repositories.d/NAME.list`; so can a
Flatpak remote's URL.

**The AUR, if you like.** Arch users can also build the recipe in
`dist/site/aur`, which installs the newest `.deb`. To list your app on the
Arch User Repository, so that AUR helpers find it, make an account at
[aur.archlinux.org](https://aur.archlinux.org) and add your SSH key to it.
Then publish `dist/site/aur` there, as `NAME-bin`:

```bash
git clone ssh://aur@aur.archlinux.org/myapp-bin.git
cp dist/site/aur/PKGBUILD dist/site/aur/.SRCINFO myapp-bin/
cd myapp-bin && git add PKGBUILD .SRCINFO && git commit -m "Release 1.2.3" && git push
```

Do the same after each release.

## What's in the site

```
dist/site/
  index.html     how to install, on each system
  install.sh     installs in one command, on every Unix the site covers
  key.asc        your public key
  latest.json    the newest version, and where each download is
  apt/           the Debian, Ubuntu and Chromebook repository, signed
  rpm/           the Fedora and openSUSE repository, signed, and the .repo file that adds it
  arch/          the Arch Linux repository for each processor, signed
  aur/           the Arch recipe, PKGBUILD and .SRCINFO
  alpine/        the Alpine repository for each processor, signed, and the key that checks it
  chimera/       the same, for Chimera
  void/          the Void repository, signed, with the key in each processor's repodata
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

A Flatpak runs on GNOME's runtime, which has GTK, Python and PyGObject but
not every program an app might run. `resources` in `[gtk.flatpak]` puts
more files beside the worker, such as the app's own ffmpeg: `{arch}` in a
path is the architecture, so `"ffmpeg/{arch}/ffmpeg"` takes each
architecture's own. `prepare` is a command run before each architecture's
Flatpak is made, with `ARCH` and `GOARCH` set, which can fetch them.

An app that adds something to the desktop, such as a GNOME search provider or a
D-Bus service that starts it, lists those files in `[[gtk.data]]`. Every
Linux, BSD and illumos package and the Flatpak installs them. `from` is
the file in your repository, and `to` is where it goes, under the
package's prefix, which is /usr on Linux, /usr/local on FreeBSD and /app in
a Flatpak. With `expand = true`, `{prefix}`, `{id}` and `{name}` in the
file become the prefix, the app's ID and its name.

```toml
[[gtk.data]]
from = "linux/search-provider.ini"
to = "share/gnome-shell/search-providers/com.example.myapp.search-provider.ini"

[[gtk.data]]
from = "linux/dbus.service"
to = "share/dbus-1/services/com.example.myapp.SearchProvider.service"
expand = true
```

The menu entry each package installs is named after the app's ID, as
`com.example.myapp.desktop`, so that is the `DesktopId` a search provider
gives. The Flatpak shows a search provider, a KRunner plugin or a D-Bus
service to the desktop only when its file name begins with the app's ID,
and its bus name should begin with the ID too, since that is the only
name a Flatpak may own without asking. A D-Bus service's `Exec` line is
replaced there with the command that runs the Flatpak.

The Flatpak repository holds many small files, which your web host has to
serve exactly as they are. `vero-repo check --app vero-app.toml --url URL
[--key DIR] dist/site` checks a site on its own, as `build` does at the
end; give `--key` and it checks the appcast's signatures too.

## Notes

Each system is packaged for every architecture vero builds for there,
x86-64 and ARM64 for most, unless its section in `vero-app.toml` narrows
them with `arches = ["x86_64"]`, as when the system's own packages lack
something the app needs on ARM. `vero-repo` refuses an architecture the
system doesn't have. Narrowing them doesn't move a repository, so adding
one back later needs nothing from the people who installed the app.

Other architectures are packaged only when a section names them, and
naming any architecture means naming all you want, so `arches =
["x86_64", "aarch64", "riscv64"]` adds RISC-V to the usual two. Each
package names them as its system does, and `arches` takes any of those
names, or Go's. On Linux:

| Architecture | `[gtk.deb]` | `[gtk.rpm]` | `[gtk.arch]` | `[gtk.alpine]` | `[gtk.void]` | `[gtk.chimera]` | `[gtk.flatpak]` |
|---|---|---|---|---|---|---|---|
| x86-64 | amd64 | x86_64 | x86_64 | x86_64 | x86_64 | x86_64 | x86_64 |
| ARM64 | arm64 | aarch64 | aarch64 | aarch64 | aarch64 | aarch64 | aarch64 |
| RISC-V | riscv64 | riscv64 | riscv64 | riscv64 | | riscv64 | |
| POWER | ppc64el | ppc64le | powerpc64le | ppc64le | | ppc64le | |
| 32-bit ARM (ARMv7) | | armv7hl | armv7h | armv7 | armv7l | | |
| 32-bit ARM (ARMv6) | armhf | armv6hl | | armhf | armv6l | | |
| 32-bit ARM (ARMv5) | armel | | | | | | |
| 32-bit x86 | i386 | i686 | pentium4 | x86 | i686 | | |
| LoongArch | loong64 | | loong64 | loongarch64 | | loongarch64 | |
| POWER, big-endian | ppc64 | | powerpc64 | | | ppc64 | |
| IBM Z | s390x | s390x | | s390x | | | |
| MIPS64 | mips64el | | | | | | |
| MIPS | mipsel | | | | | | |

On the BSDs:

| Architecture | `[gtk.freebsd]` | `[gtk.netbsd]` | `[gtk.openbsd]` |
|---|---|---|---|
| x86-64 | amd64 | x86_64 | amd64 |
| ARM64 | aarch64 | aarch64 | aarch64 |
| 32-bit x86 | i386 | i386 | i386 |
| 32-bit ARM | armv7 | | arm |
| RISC-V | | | riscv64 |

DragonFly and illumos are x86-64 only.

Where a FreeBSD or DragonFly package needs other packages on one
architecture, `by_arch` replaces `deps`, `python` or both there, by the
system's name for it:

```toml
[gtk.freebsd.by_arch.armv7]
deps = { python3 = "lang/python3", py311-pygobject = "devel/py-pygobject@py311" }
python = "python3.11"
```

32-bit ARM comes in three: ARMv7 with hardware floating point, built with
`GOARM=7`; ARMv6 with it, with `GOARM=6`, as Alpine's armhf and Void's
armv6l have it, for the first Raspberry Pis; and ARMv5, without, with
`GOARM=5`, as Debian's armel. Debian's own armhf is ARMv7, but Raspberry
Pi OS's, by the same name, is ARMv6, so the armhf `.deb` is built for
ARMv6, which runs on both. Arch Linux ARM has only ARMv7 now. The rpm's
32-bit ARM is openSUSE's, which has both ARMv7 and ARMv6; Fedora has
none. Arch's POWER is Arch POWER's.

32-bit x86 needs a Pentium 4 or later, since the worker is built with
`GO386=sse2`. That is what Arch Linux 32 calls pentium4: its pacman takes
that name on a processor with SSE2, and looks only in that folder of the
repository. Go's code for POWER, either way round, needs a POWER8 or
later, so not a Power Mac G5.
Void has i686 packages only for glibc, so there is no i686-musl one. The
MIPS packages are little-endian, as Debian 12's are.

The illumos packages are signed, because pkgsrc there, as SmartOS sets it
up, installs only signed packages: the page's commands add your key to its
keyring first. The NetBSD packages aren't signed, since NetBSD's pkgsrc
doesn't check unless it's set up to, so your site's HTTPS vouches for
them. Everything else that installs from your site checks your signature.

On illumos the icon isn't put in the icon theme. GTK 4 from SmartOS's
pkgsrc crashes there reading a PNG, and a GTK 4 app loads its window's
icon from the theme, so the app would not start. The package keeps the
icon in `share/<name>/` instead, and the menu entry names it by its whole
path: the menu, MATE's on OpenIndiana, is GTK 3, which reads it safely.
The app's window has no icon of its own there.

`scripts/package.sh` runs `vero-repo package`, which builds the Windows
installers with [`package-windows.sh`](../../scripts/package-windows.sh).
[`package-linux.sh`](../../scripts/package-linux.sh) builds `.deb` files on its
own, from flags rather than `vero-app.toml`. Each script's header lists
its options. [`run-linux.sh`](../../scripts/run-linux.sh) shows your Linux app
running before you package it.

## Step by step

`vero release` runs three commands, each of which can be run on its own:
`vero-repo key` once, then `scripts/package.sh` and `vero-repo build` for
each release. `vero` hands `key`, `package`, `build` and `check` on to
`vero-repo`, so `vero key ...` is the same thing.

**The signing key.**

```bash
vero-repo key --dir ~/vero-keys/myapp --name "Your Name or Company" --email you@example.com
```

This makes the folder you name with `--dir`. Without `--dir`, it's a
folder named after you in `~/Library/Application Support/vero-repo` on a
Mac, or `~/.config/vero-repo` on Linux. Its `private.asc` signs every
release, so that people's systems can tell your updates are really yours,
its RSA half signing the FreeBSD, DragonFly, Alpine and Void packages too;
`signify.sec` signs the OpenBSD packages; `sparkle.sec` signs Mac updates;
and `android.keystore`, made the first time an Android app is built, signs
it. Back the folder up, keep it out of your repository, and never publish
anything in it but `key.asc`, the public half, which your site publishes.

If your Mac app already updates itself with Sparkle, keep its key, so that
copies already installed go on updating: `vero-repo key --dir DIR
--import-sparkle FILE`, where FILE holds the private key Sparkle's
`generate_keys -x` exported. Older Sparkle keys, which were kept
expanded rather than as a seed, are taken too.

**The installers.**

```bash
path/to/vero/scripts/package.sh --app vero-app.toml --version 1.2.3
```

This builds them into `dist/packages`, each for both x86_64 and ARM64
processors where the system has both. Use `--targets` to build only some
of them: `deb`, `rpm`, `pacman`, `alpine`, `void`, `chimera`, `flatpak`, `freebsd`,
`dragonfly`, `netbsd`, `illumos`, `openbsd`, `macos`, `windows`, `msix`,
`android`, `ios`, `web`, `wasi`, `plan9`; or `linux` for all of Linux's
but the Flatpak, and `bsd` for the BSDs and illumos. Use `--ldflags` to build more into your worker, such as
API keys from a file outside your repository.

**The site.**

```bash
vero-repo build --app vero-app.toml --key ~/vero-keys/myapp \
    --url https://example.com/myapp
```

This builds the site into `dist/site`; `--url` is where it will be,
because the commands on its page and its repositories need to know, and
`--keep` is how many versions of each installer to keep, three unless
said. At the end, `build` checks the site as `vero-repo check` would: that every
signature verifies with the keys the site publishes, that every index's
sizes and hashes match its files, that every link and manifest points at
a file that's there, that each package holds your worker and front end,
and that no system's version has gone backwards, which would leave people
never offered the update. It refuses to finish otherwise.

## vero's own tests, and your app's

[`scripts/test-repo.sh`](../../scripts/test-repo.sh) checks all of it for real.
It builds the example's site, serves it from your Mac, and installs the
example from it on a clean system, as the site tells people to. It checks
that the worker answers, that the command, menu entry and front end are
there, that Python can load GTK 4, and that GTK 4 can read the app's icon.
Then it starts the installed app on a virtual display, plays the steps its
`vero-app.toml` gives (below), and records the window as it goes, as a GIF.
Last, it releases version 1.0.1 and checks that the system's own updates
bring it. Each stage is numbered, with the time so far. It tries Debian, in
a container, unless you say otherwise:

```bash
scripts/test-repo.sh --image ubuntu:24.04
scripts/test-repo.sh --image fedora:latest
scripts/test-repo.sh --image opensuse/tumbleweed
scripts/test-repo.sh --image archlinux
scripts/test-repo.sh --image alpine
scripts/test-repo.sh --image chimeralinux/chimera
scripts/test-repo.sh --image ghcr.io/void-linux/void-glibc
scripts/test-repo.sh --flatpak
scripts/test-repo.sh --vm freebsd         # also netbsd, openbsd, dragonfly, illumos
scripts/test-repo.sh --mac                # the disk image, on this Mac, and the appcast after an update
scripts/test-repo.sh --vm windows         # the installer and the MSIX, in vero's Windows VM, over SSH
```

`--app path/to/vero-app.toml` tests your own app the same way, on every
system but the Mac, which has `scripts/record-mac.sh` instead (below).
`--no-launch` leaves out starting the app, and `--limit` is how long its
steps may take, 600 seconds unless you say. Two systems can be tested at
once, each with its own `--port`.

The containers need Docker and colima. A `--vm` test uses the system's VM,
which its `run-*.sh` script makes the first time; that downloads the
system and its GTK, a few gigabytes. The Windows test reaches vero's
Windows VM over SSH: it installs, starts, updates and uninstalls the
installer, and registers the MSIX's files in developer mode and starts the
app, for an app with `[wpf.msix]`. A VM made with
`scripts/run-windows.sh --install` has SSH; one made before that gets it
with `scripts/setup-windows-ssh.sh`, which the
[VM's notes](../../.windows/unattend/README.md) describe.

### Steps, and a GIF of your app at work

`[[test.step]]`s in `vero-app.toml` say what the test does with the app
once its window is open. vero's binding in the app plays them (vero.py,
Vero.cs and the Swift package all do), so the window shows each one as a
person would see it:

```toml
[test]
files = ["testdata/sample.mp3"]          # copied onto the system, as {files}
env = { MYAPP_PROFILE = "test" }         # set for the app as it starts

[[test.step]]
call = "setLibrary"                      # a request, by name
with = { path = "{tmp}" }                # {tmp} is a new empty folder

[[test.step]]
copy = { from = "sample.mp3", to = "{tmp}/imports/" }

[[test.step]]
wait = { path = "stats.books", at_least = 1 }   # or is, not, contains
timeout = 300

[[test.step]]
pause = 3                                # so the recording shows it
```

A wait's `path` goes through the app's state by key and list index, and
`#` is a list's length: `jobs.#`, `jobs.0.phase`. The test stops at the
first step that fails. Each system's GIF, steps and log go in
`~/.cache/vero/test-results/APP/SYSTEM/`, and
`~/.cache/vero/test-results/index.html` shows every app and system tested
so far. `vero release` puts the GIF of each system that passed on the
install page, under "See it at work"; `vero-repo build --recordings
DIR` does the same from a folder you choose.

On the Mac, `scripts/record-mac.sh --app vero-app.toml --bundle My.app`
does the same with an app you've built: it starts it on this Mac, as you,
and captures its window. So the copy it starts must not touch the copy you
use. Give it its own settings through `[test] env`, build it with its own
bundle ID (`PRODUCT_BUNDLE_IDENTIFIER=...`), and check that before
recording; `--clean` clears what the last recording left under
`~/Library`. Capturing a window needs Screen Recording permission for the
terminal.

Some packages have been checked only by vero's own checks, not installed
on the system they are for: Debian's armel, openSUSE's armv7hl and
armv6hl, Alpine's armhf, Void's armv6l, Arch Linux 32's pentium4, Arch
POWER's, FreeBSD's armv7, and Chimera's on anything but ARM64, which
`test-repo.sh` installs in Chimera's container.
