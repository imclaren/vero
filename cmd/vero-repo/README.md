# vero-repo

This page is the reference for vero's packaging, which makes every
installer, repository, signature and index in Go, and the site from which
people install and update an app. `vero release` runs it, and
[Package and release](../../docs/release.md) is the guide.
[`vero-app.toml`](../../PACKAGING.md) lists the settings that describe the
app.

A release never needs a virtual machine, a container or anything but your
Mac. The one exception is a Flatpak, which needs Docker and is made only
if you ask for one. vero's tests do use virtual machines and containers,
to check that each system installs and updates from a site, as
[Build, run and test on every system](../../docs/test.md) describes.

| System | What your users get | What you need on your Mac |
|---|---|---|
| Debian, Ubuntu and Chromebooks (Linux on ChromeOS, turned on in Settings → Developers) | An apt repository, installed once and then updated with their usual updates | Go |
| Fedora, openSUSE | An rpm repository, in the same way | Go |
| Arch Linux | A pacman repository, in the same way, and a recipe for the AUR | Go |
| Alpine Linux | An apk repository, if you ask for one | Go |
| Void Linux | An xbps repository, if you ask for one | Go |
| Chimera Linux | An apk repository, if you ask for one | Go |
| Any Linux, with Flatpak | A Flatpak repository, if you ask for one | Docker and colima |
| FreeBSD, DragonFly | A pkg repository | Go |
| NetBSD | A pkgsrc repository, for pkgin | Go |
| illumos (OpenIndiana) | An IPS repository, for pkg, with GTK 4, libadwaita and PyGObject from OpenIndiana's own | Go |
| OpenBSD | Signed packages for pkg_add | Go |
| macOS | A disk image, a Sparkle appcast for updates, and a Homebrew cask | Xcode, and a Developer ID and an Apple account to notarise |
| Windows | An installer and a winget manifest, and, if you ask, one for 32-bit Windows and an MSIX for the Microsoft Store | makensis and dotnet |
| Android | A signed .apk, and an .aab for Google Play if your build makes one | The Android SDK and a JDK |
| iPhone and iPad | An .ipa, for the App Store or TestFlight | Xcode and an Apple account |
| In a browser | The page, served from the site | Go |
| In a terminal, with WASI | Bundles for each desktop, whose worker is one worker.wasm | Go |
| Plan 9 | A bundle with an install script | Go |

Each row is made when `vero-app.toml` has its section. Where the table
says Go, it means Go alone, since `vero-repo` makes every package,
repository, signature and index itself.

## Try it on the example

vero's example describes itself in
[`example/vero-app.toml`](../../example/vero-app.toml), with a section for
every one of its front ends. From vero's folder, this builds its
installers into `dist/packages`, then its site into `dist/site`.

```bash
(cd cmd/vero && go install .)
vero release --app example/vero-app.toml
```

`go install` puts `vero` in Go's `bin` folder, which needs to be on your
`PATH`. `--targets` builds only some systems, which helps on a Mac without
the Android SDK, as in `vero release --app example/vero-app.toml --targets
deb,macos`.

## macOS

`[macos]` names a SwiftPM package (`folder` and `product`) or an Xcode
project (`folder`, `project` and `scheme`). vero builds it for Apple
silicon and Intel in one app, puts the worker in `Contents/Resources`,
where vero's Swift package looks for it, and makes a disk image, with a
link to Applications, onto which people drag the app. `pkg = true` makes an installer
package too, which installs the app into `/Applications`, never over a
copy somewhere else, and makes it the property of whoever installed it,
so that Sparkle can update it without an administrator's password.

The app's versions are always the release's, whatever the Xcode project
says. `CFBundleShortVersionString` is `--version`, and `CFBundleVersion`
is `--build`, or the version when there is no build number.

- **Files to bundle.** `resources` lists more files for
  `Contents/Resources`, such as helper programs that the app runs, and
  their licences. `prebuild` is a command run first, in the app's folder, which can
  make or fetch them. `prepare` runs before each architecture is built,
  with `GOARCH` and `CC` set, for anything that has to be built per
  architecture.
- **A worker that needs cgo**, for the keychain, say, sets `cgo = true`
  in `[worker]`. The Mac worker is then built with Xcode's clang for each
  architecture and joined into one universal program. Everywhere else, it
  is built without cgo, as before.
- **Signing.** The app is signed with the Developer ID Application
  certificate in your keychain. When there are several, or you want
  another, `VERO_MAC_IDENTITY="Developer ID Application: Name (TEAMID)"`
  chooses one. With none, the app is signed ad hoc, which runs on any Mac
  once whoever opens it allows it in System Settings, under Privacy &
  Security, as the site's page tells them. The app is signed from the
  inside out, with the hardened runtime. First come every program and
  library in it, then the bundles around them, deepest first, such as
  Sparkle's helper apps and services and then `Sparkle.framework`, then
  the app itself, and last the disk image. The result is checked with `codesign --verify --deep
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
  `Info.plist` has to give them itself. vero's example does not embed
  Sparkle, so its appcast is only checked, and the app does not fetch it.
  Sparkle compares `CFBundleVersion`, which is the version unless
  `package.sh --build N` numbers builds on their own. If apps already
  installed look for the appcast somewhere else on the site, `appcast` in
  `[macos]` writes it there too, such as `appcast = "appcast.xml"` for the
  site's top.
- **Release notes.** `vero-repo build --notes "..."` (or `--notes-file`)
  makes a page of what is new, to which the appcast links, so Sparkle shows it
  in its update prompt. Paragraphs are split by blank lines, and lines
  starting with `- ` become a list.
- **Downloads served elsewhere.** `download_url` in `[macos]` points the
  appcast and the cask at another server, if the disk images are not kept
  on the site. `vero-repo build --no-page` leaves out the public page and
  `install.sh`, for an app distributed privately.
- **Homebrew.** `homebrew/NAME.rb` is a cask for the newest release, to
  put in a tap of your own or submit to homebrew-cask.

## Windows

`[wpf]` names the WPF project and the program it builds. vero publishes it
self-contained, so people need no .NET, and makes an installer for x64 and
one for ARM64, with `makensis`, which runs on a Mac. `arches = ["x64",
"arm64", "x86"]` adds one for 32-bit Windows, with the worker built for
386, and winget lists each. The installers are not signed, because
signing needs your own code-signing certificate. Until you sign them, with
`signtool` on Windows or `osslsigncode` on a Mac, Windows SmartScreen
warns people who download them, and the page tells them how to get past
the warning.

- **winget.** `winget/manifests/...` holds the three manifests from which
  winget installs the newest release. To list your app, fork
  [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs), copy
  the folder under its `manifests`, and open a pull request; after each
  release, do the same for the new version. `winget_id` in `[wpf]` sets
  the identifier, `Publisher.App`, which is otherwise made from your
  names.
- **The Microsoft Store.** `[wpf.msix]` makes an MSIX, with `build = true`
  or `--targets msix`. Reserve your app's name in
  [Partner Center](https://partner.microsoft.com/dashboard), which gives
  you its `identity_name` and `publisher`. The Store signs what you
  submit, so vero leaves the MSIX unsigned. It declares a startup task when
  `[needs]` has `autostart`, and the internet when it has `network`. The
  package is stored uncompressed, as the Store expects, so it is bigger
  than the installer. To install one yourself for testing, sign it with
  `signtool` and a certificate your PC trusts, or register its files in
  developer mode, as `scripts/test-repo.sh --vm windows` does.

## Android, iPhone and iPad

`[android]` names the folder, the command that builds the app, and the
aligned, unsigned `.apk` that it makes. vero signs it, and an `.aab` for
Google Play if the build makes one and `aab` names it. The keystore is
yours, given in `VERO_ANDROID_KEYSTORE`, `VERO_ANDROID_KEY_ALIAS` and
`VERO_ANDROID_PASSWORD`. Without those, vero makes one beside your signing
key the first time, and keeps it. Android installs an update only when the
same key signed it, so never lose it. The build needs the Android SDK and
a JDK. `JAVA_HOME` says where the JDK is, and otherwise Homebrew's
`openjdk` is used.

`[ios]` names the folder, the build, and the app it makes for the
Simulator, which vero zips for testing. An app reaches phones only through
the App Store or TestFlight. Name the app that your build makes for
devices as `device_app`, put your `VERO_IOS_IDENTITY` ("Apple Distribution: ...") and
the path of its provisioning profile in `VERO_IOS_PROFILE`, and vero
signs it and makes the `.ipa` to upload with Transporter. The site's page
links to `app_store_url`.

## The browser, WASI and Plan 9

`[web]` names the folder, its build, and the files it makes, which the
site serves at `web/`, so that the page links to them. `[wasi]` names a
terminal front end and the worker it drives, which is built once as
`worker.wasm` and bundled with the front end built for each desktop.
People run it with `wasmtime`. `[plan9]` names the Plan 9 front end,
bundled with the worker and an `rc` script that installs them.

## Arch Linux, Alpine, Void and Chimera

Arch users add your pacman repository, as the site's page says. pacman-key
trusts your key, `/etc/pacman.conf` gains a section named after your app,
whose `Server` is `.../arch/$arch`, and `pacman -Syu` keeps the app up to
date. pacman checks the signature of every package and of the index. The
packages are made by default, with the `.deb` and the `.rpm`; `[gtk.arch]`
says what they depend on, and `optdepends` what they can use if it is
installed, as `"webkitgtk-6.0: signing in inside the app"`.

`[gtk.alpine]`, `[gtk.void]` and `[gtk.chimera]` make packages for
Alpine, Void and Chimera, with what each depends on, by its own names. apk
and xbps check every package with the RSA half of your signing key, which
the site publishes for them. The page's commands put that key in
`/etc/apk/keys` for apk, and in `/var/db/xbps/keys` for xbps, named by
its fingerprint, which are where each looks for the keys that it trusts. The same packages serve Void's glibc
and musl systems, since the worker needs neither.

Chimera's apk is apk-tools 3, which reads Alpine's kind of package and
index, signed as Alpine's are, besides its own newer kind. vero makes
Alpine's kind for it, in a repository of its own, with Chimera's names for
what the app needs. Chimera has FreeBSD's `fetch`, rather than curl or
wget, which a minimal install adds with `apk add chimerautils-extra`.

A private site can put a password in the address any of them fetches
from, as `https://NAME:PASSWORD@example.com/myapp/arch/$arch` in
`pacman.conf`, or the same in `/etc/apk/repositories` or
`/etc/xbps.d/NAME.conf` or `/etc/apk/repositories.d/NAME.list`, and so can
a Flatpak remote's URL.

**The AUR.** Arch users can also build the recipe in
`dist/site/aur`, which installs the newest `.deb`. To list your app on the
Arch User Repository, so that AUR helpers find it, make an account at
[aur.archlinux.org](https://aur.archlinux.org) and add your SSH key to it.
Then publish `dist/site/aur` there, as `NAME-bin`, or let
`vero publish --aur` do it.

```bash
git clone ssh://aur@aur.archlinux.org/myapp-bin.git
cp dist/site/aur/PKGBUILD dist/site/aur/.SRCINFO myapp-bin/
cd myapp-bin && git add PKGBUILD .SRCINFO && git commit -m "Release 1.2.3" && git push
```

Do the same after each release.

## What is in the site

These are the files that a release puts in `dist/site`.

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
  illumos/       the illumos IPS repository, as plain files at the paths pkg asks a depot for, and the packages
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
more files beside the worker, such as the app's own ffmpeg. `{arch}` in a
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
end. With `--key`, it checks the appcast's signatures too.

## Processors

Each system is packaged for every architecture for which vero builds
there, which is x86-64 and ARM64 for most, unless its section in `vero-app.toml` narrows
them with `arches = ["x86_64"]`, as when the system's own packages lack
something that the app needs on ARM. `vero-repo` refuses an architecture
that the system does not have. Narrowing them does not move a repository,
so adding one back later needs nothing from the people who installed the
app.

Other architectures are packaged only when a section names them, and
naming any architecture means naming all you want, so `arches =
["x86_64", "aarch64", "riscv64"]` adds RISC-V to the usual two. Each
package names them as its system does, and `arches` takes any of those
names, or Go's. These are the names on Linux.

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

These are the names on the BSDs.

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
system's name for the architecture.

```toml
[gtk.freebsd.by_arch.armv7]
deps = { python3 = "lang/python3", py311-pygobject = "devel/py-pygobject@py311" }
python = "python3.11"
```

32-bit ARM comes in three kinds. ARMv7, with hardware floating point, is
built with `GOARM=7`. ARMv6, with hardware floating point, is built with
`GOARM=6`, as Alpine's armhf and Void's armv6l have it, for the first
Raspberry Pis. ARMv5, without it, is built with `GOARM=5`, as Debian's
armel. Debian's own armhf is ARMv7, but Raspberry
Pi OS's, by the same name, is ARMv6, so the armhf `.deb` is built for
ARMv6, which runs on both. Arch Linux ARM has only ARMv7 now. The rpm's
32-bit ARM is openSUSE's, which has both ARMv7 and ARMv6; Fedora has
none. Arch's POWER is Arch POWER's.

32-bit x86 needs a Pentium 4 or later, since the worker is built with
`GO386=sse2`. That is what Arch Linux 32 calls pentium4. Its pacman takes
that name on a processor with SSE2, and looks only in that folder of the
repository. Go's code for POWER, either way round, needs a POWER8 or
later, so not a Power Mac G5.
Void has i686 packages only for glibc, so there is no i686-musl one. The
MIPS packages are little-endian, as Debian 12's are.

The illumos packages are IPS packages for OpenIndiana's own `pkg`, and
depend on OpenIndiana's GTK 4, libadwaita, PyGObject (for its Python 3.9)
and web view. An IPS repository is usually served by `pkg.depotd`; vero
writes it as plain files at the paths `pkg` asks a depot for, so any web
host serves it, and signs its catalog as `pkgsend` does
([ips.go](ips.go)). The page's commands add it as a publisher of its own,
and `pkg update` brings each new version. The NetBSD packages are not
signed, since NetBSD's pkgsrc does not check a signature unless it is set
up to do so, so your site's HTTPS vouches for them. Everything else that installs from your site checks your signature.

## Step by step

`vero release` runs three commands, each of which can be run on its own.
`vero-repo key` runs once, and `scripts/package.sh` and `vero-repo build`
run for each release. `vero` hands `key`, `package`, `build` and `check`
on to `vero-repo`, so `vero key ...` is the same command. `vero-repo`'s
other commands are these:

- `credentials` and `publish`, which [Package and release](../../docs/release.md#signing-stores-and-listings) describes;
- `plan`, which says what `vero ship` will build and test on this Mac, and what it cannot;
- `steps`, which writes what vero's tests play against the installed app;
- `show`, which prints the app's names as shell assignments, for scripts.

### The signing key

```bash
vero-repo key --dir ~/vero-keys/myapp --name "Your Name or Company" --email you@example.com
```

This makes the folder that `--dir` names. Without `--dir`, it is a folder
named after you in `~/Library/Application Support/vero-repo` on a Mac, or
in `~/.config/vero-repo` on Linux. Its `private.asc` signs every release,
so that people's systems can tell that your updates are really yours, and
its RSA half signs the FreeBSD, DragonFly, Alpine and Void packages too.
`signify.sec` signs the OpenBSD packages, `sparkle.sec` signs the Mac's
updates, and `android.keystore`, made the first time an Android app is
built, signs that app. Back the folder up, keep it out of your
repository, and never publish anything in it but `key.asc`, the public
half, which your site publishes.

If your Mac app already updates itself with Sparkle, keep its key, so that
copies already installed go on updating. `vero-repo key --dir DIR
--import-sparkle FILE` takes it, where `FILE` holds the private key that
Sparkle's `generate_keys -x` exported. Older Sparkle keys, which were kept
expanded rather than as a seed, are taken too.

### The installers

```bash
path/to/vero/scripts/package.sh --app vero-app.toml --version 1.2.3
```

This builds them into `dist/packages`, each for both x86_64 and ARM64
processors where the system has both. `--targets` builds only some of
them, from this list:

- `deb`, `rpm`, `pacman`, `alpine`, `void`, `chimera` and `flatpak`, or `linux` for all of Linux's but the Flatpak;
- `freebsd`, `dragonfly`, `netbsd`, `illumos` and `openbsd`, or `bsd` for all of them;
- `macos`, `windows` and `msix`;
- `android`, `ios`, `web`, `wasi` and `plan9`.

`--ldflags` builds more into your worker, such as API keys from a file
outside your repository.

[`scripts/package.sh`](../../scripts/package.sh) runs `vero-repo package`,
which builds the Windows installers with
[`package-windows.sh`](../../scripts/package-windows.sh).
[`package-linux.sh`](../../scripts/package-linux.sh) builds `.deb` files
on its own, from flags rather than `vero-app.toml`. Each script's header
lists its options.

### The site

```bash
vero-repo build --app vero-app.toml --key ~/vero-keys/myapp \
    --url https://example.com/myapp
```

This builds the site into `dist/site`. `--url` is where it will be, which
the commands on its page and its repositories need. `--keep` is how many
versions of each installer to keep, three unless it is given. At the end,
`build` checks the site as `vero-repo check` would. It checks that every
signature verifies with the keys that the site publishes, that every
index's sizes and hashes match its files, that every link and manifest
points to a file that is there, that each package holds your worker and
front end, and that no system's version has gone backwards, which would
leave people never offered the update. It refuses to finish otherwise.

`--notes` and `--notes-file` give the Mac's release notes, `--no-page`
leaves out the public page, and `--recordings DIR` puts the GIFs from a
folder of vero's test results on the page.
