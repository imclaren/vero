# Taking your app to more platforms, and packaging it

[Back to vero's README](README.md)

Three commands do all of it: `vero add` writes a front end for each
system, `vero release` builds the installers and the site, and `vero
credentials` is the checklist for signing and the stores, when you want
them. Start with [a worked example](#a-worked-example), which takes a
small app from macOS to an install page with them. The rest of the guide
has two parts. [Taking your app to more
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

**1. Install vero's tool.** Go puts it in `$(go env GOPATH)/bin`, which
needs to be on your `PATH`. (`vero` installs `vero-repo`, the packaging,
the first time it needs it; `vero-site` serves a site, for looking at
one.)

```bash
go install github.com/imclaren/vero/cmd/vero@latest
go install github.com/imclaren/vero/kit/cmd/vero-site@latest
```

**2. Make the app:** a Go module with a worker and an icon.

```bash
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/imclaren/vero@latest
VERO=$(go list -m -f '{{.Dir}}' github.com/imclaren/vero)
mkdir -p cmd/worker && cp "$VERO/example/worker/main.go" cmd/worker/
cp "$VERO/example/icon.png" .
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

**4. Release.** One command: it makes the signing key the first time
(which signs this and every later release, so that people's systems trust
your updates: back the folder up), builds every installer your Mac has
the tools for - here the Debian packages, the rpms, the Mac disk image and
the Windows installers - and builds the install site into `dist/site`,
checked.

```bash
vero release
```

It ends by saying how each installer was signed. Nothing in it needs an
account with anyone: the Mac app is signed ad hoc and the Windows
installer not at all, and the site's page tells people the one click each
asks for; the Linux packages are signed with your key, as they must be.
Step 5 of [Creating app installers](#creating-app-installers) is for
when you want more.

**5. Look at it.** `vero-site` serves the site.

```bash
vero-site -dir dist/site
```

Open the address it prints. The page shows the install commands for the
visitor's system, and `/download?for=mac` hands out the disk image.

**6. Put it online.** Put the site's address in `vero-app.toml`, so the
page and the repositories point there, and release again; then upload
`dist/site` to that address, or let `vero release` do it.

```toml
site = "https://example.com/hello"
```

```bash
vero release --upload you@host:/srv/hello
```

**7. Release an update.** Change the app, run `vero release` again: the
version goes up by one, and people get it with their usual updates.

## Taking your app to more platforms

An app written for one system - a Mac menu bar app, say - can run on the
others with the worker it already has.

**1. Add the front ends.** In your app's folder, the one with its
`go.mod`:

```bash
go install github.com/imclaren/vero/cmd/vero@latest
vero add                 # the desktop: macOS, GTK (Linux, the BSDs, illumos) and WPF (Windows)
vero add mobile          # Android and iOS
vero add all             # everything vero has a front end for
```

It reads your worker - the state it pushes and the requests it handles -
and writes a starter for each system: a window that shows every field of
the state and a control for every request, so that it runs against your
worker as it is. Front ends you already have are left alone. On iOS and
in a browser the worker runs inside the app, and `vero add` arranges
that too, by moving the worker's code into a package those two can run.

**2. Build and run each one.** Each starter's folder has a README with
the two commands: `./build.sh`, then run it. The starters are a first
draft to make your own, not a design; the shapes of your state and
requests are written out in each language, for the window you build
next. [cmd/vero/README.md](cmd/vero/README.md) has what each starter is,
and the same ideas in each toolkit for a window of your own.

**3. Work through PORTING.md.** `vero add` also writes `PORTING.md`:
what in your worker needs attention on the new systems, found by reading
its code - cgo, programs it starts, a port it listens on, secrets and
notifications done one system's way - each with the files and lines, and
what to change it to. Most of those are a `kit` package:
[kit/README.md](kit/README.md) lists them.

**4. Release.** `vero release`, as below: the new systems' packages and
instructions are in the same site.

## Creating app installers

vero builds your app's installers, and the website your users install
them from and get updates from: a page saying how to install on each
system, signed repositories their systems update from, and the
downloads, as plain files to put on any web host. A release never needs
a virtual machine, a container or anything but your Mac.
[cmd/vero-repo/README.md](cmd/vero-repo/README.md) has what each system
gets and what your Mac needs for it.

From your app's folder, the one with its `go.mod`:

**1. Describe your app.** Copy
[`example/vero-app.toml`](example/vero-app.toml) beside your `go.mod`, and
change it to describe your app. The comments in it explain each setting.
Paths in it are relative to the file, and `site` is where the install
site will be. Your worker's main package needs a `var version`, which
the build sets.

Each system names its packages differently, so `vero-app.toml` says what
your app needs from each, in its own section: `[gtk.deb]`, `[gtk.rpm]`,
`[gtk.arch]`, `[gtk.freebsd]` and so on. Leave out the sections of the
systems you don't want; a system is packaged for only when its section is
there. `[needs]` says what the app asks of the system, such as the
network.

**2. Release.**

```bash
vero release
```

It works out the version (the next after the newest the site has), makes
the signing key the first time, builds every installer your Mac has the
tools for, builds the site into `dist/site` and checks it, and says how
each installer was signed. With no credentials set it releases everything
but an iOS app for phones, which only Apple's store can carry: see step
5, below.

```bash
vero release --version 2.0.0 --notes "..."     # a version you choose, with what's new for the Mac's update prompt
vero release --targets deb,macos               # some of it
```

**3. Put it online.** Upload `dist/site` to the address `site` gives -
any static web host does, or `--upload` does it for you:

```bash
vero release --upload you@host:/srv/myapp
```

If you'd rather run your own server, `vero-site` serves the folder by
itself, with the page, the repositories, `/download?for=SYSTEM` links and
`latest.json`; with `-domain` it gets an HTTPS certificate from Let's
Encrypt for that name. If your app already has a Go web server, serve the
folder from it with `kit/site` instead.

```bash
go install github.com/imclaren/vero/kit/cmd/vero-site@latest
vero-site -dir /srv/myapp -domain downloads.example.com
```

**4. Release an update.** Change the app and run `vero release` again.
People get it with their usual updates: apt, dnf, zypper, pkg, pkgin and
Flatpak find it by themselves, a Mac app with Sparkle reads the appcast,
and OpenBSD's `pkg_add -u` does with the folder the page says to give it.
On Arch, they build the recipe again. Your app can read `latest.json`
from the site to tell Windows and Android users that a new version is
out.

**5. Signing, stores and listings.** A release needs no account with anyone, and the site's page tells people
the one click an unsigned app asks of them: on a Mac, Open Anyway in
Privacy & Security; on Windows, More info, then Run anyway. The Linux
and BSD repositories are signed with your key and ask nothing. For the
rest, `vero credentials` is the checklist: it says what is set up, finds
what it can on your Mac (a Developer ID in your keychain, say, and the
export line that uses it), and prints the exact commands for what isn't.

```bash
vero credentials
```

| For | What it takes | Then |
|---|---|---|
| macOS: no Open Anyway | the Apple Developer Program; a Developer ID Application certificate in your keychain; `xcrun notarytool store-credentials` once | `VERO_MAC_IDENTITY`, `VERO_NOTARY_PROFILE` (and `VERO_MAC_INSTALLER` for a `.pkg`) in your shell |
| Windows: no SmartScreen | a code-signing certificate, as a `.pfx`; `brew install osslsigncode` | `VERO_WINDOWS_CERT`, `VERO_WINDOWS_CERT_PASSWORD` |
| Android: a keystore you already ship with | the keystore | `VERO_ANDROID_KEYSTORE`, `VERO_ANDROID_KEY_ALIAS`, `VERO_ANDROID_PASSWORD`; without them vero makes one and keeps it beside your key |
| iOS: phones | the Apple Developer Program; an Apple Distribution certificate and an App Store profile | `VERO_IOS_IDENTITY`, `VERO_IOS_PROFILE`; the `.ipa` goes up with Transporter |
| Homebrew | `gh` signed in | `vero publish --homebrew`: a tap of your own, `USER/homebrew-tap`, made the first time |
| winget | `gh` signed in | `vero publish --winget`: a pull request to microsoft/winget-pkgs from a fork of yours |
| the AUR | an account at aur.archlinux.org with your SSH key | `vero publish --aur`: `NAME-bin`, made the first time |
| the Microsoft Store, Google Play, Flathub | an account with each; Flathub reviews by hand | upload the `.msix` or `.aab` vero built; a pull request to flathub/flathub |

The environment variables are read by the next `vero release`; the
publishing is a command each after it. `vero publish --all` does
whichever of the three the site has files for.

## Looking things up

- [cmd/vero/README.md](cmd/vero/README.md): what `vero add` writes, and the same ideas in each toolkit
- [cmd/vero-repo/README.md](cmd/vero-repo/README.md): what each system gets, each system's settings, what's in the site, and the steps `vero release` is made of
- [kit/README.md](kit/README.md): the packages a worker uses instead of one system's way
