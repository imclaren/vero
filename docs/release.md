# Package and release

This page builds an app's installers for every system, and the site from
which people install the app and get its updates. A release needs only
your Mac, and no account with anyone. Signing and the stores can come
later.

## vero ship

`vero ship` does everything in one command. In the app's folder, it
builds and tests the app on every system it can, records each one, and
releases it. Given a Go file, as in `vero ship counter.go`, it first makes
the app from the file.

```bash
vero ship
```

Before it does anything else, it prints what it will build and test on
this Mac, what it will skip and why, and how long each virtual machine
takes to make the first time. It stops at the first failure, and says
where its log is.

- `--plan` says what it would do, and stops.
- `--no-release` tests and records, but does not release.
- `--only "macos debian"` tests only those systems, and `--skip` tests all
  but the ones that it names.
- `--no-vms` tests in containers only, which is quicker, and `--no-mac`
  does not record the Mac app.
- `--version`, `--notes`, `--url` and `--upload` are as for `vero release`,
  below.

### What you get

| | |
|---|---|
| The app | A Go module with the worker in `cmd/worker`, a SwiftUI app in `macos/`, a GTK app in `gtk/`, a WPF app in `windows/`, an icon, and `vero-app.toml`, which describes it. |
| Installers | A signed and notarised `.dmg` for the Mac, packages for Debian, Fedora, openSUSE, Arch, Alpine, Void, Chimera, FreeBSD, DragonFly, NetBSD, OpenBSD and illumos, and installers for Windows. |
| A site | `dist/site`, plain files to put on any web server, with a page that offers each system its installer, and signed repositories through which every system updates the app. |
| Recordings | Each system's window as a GIF and a picture, in `~/.cache/vero/test-results/`, with one page of every result, which the site's page shows too. |

### What it cannot do

- It cannot make an app for iOS, Android or the browser by itself, since
  each of those runs the worker inside the app. `vero add ios`, `vero add
  android` and `vero add web` write those front ends, and `vero release`
  packages them.
- It does not choose where the site lives. Put its address in `site` in
  `vero-app.toml`, and `vero ship --upload user@host:/path` copies the
  site to your server, or copy `dist/site` there yourself. Until then,
  the site is made for `http://localhost:8080`, for a look at it.
- It does not sign the Windows installers, since that needs a certificate
  of your own. Until they are signed, Windows asks people once whether to
  run them, as the site's page explains.

After the first time, change the worker or the front ends and run
`vero ship` in the app's folder again. Each release takes the next
version.

## vero release, step by step

`vero release` packages the app without testing it. Run it in the app's
folder, the one with its `go.mod`.

**1. Describe the app.** `vero new` and `vero add` write `vero-app.toml`.
For an app without one, copy [`example/vero-app.toml`](../example/vero-app.toml)
beside the app's `go.mod`, and change it. [`vero-app.toml`](../PACKAGING.md)
lists the settings. The worker's main package needs a `var version`, which
the build sets.

**2. Release.**

```bash
vero release
```

It works out the version, which is the next after the newest that the
site has. The first time, it makes the signing key, which signs this
release and every later one, so that people's systems trust your updates.
Back up its folder. It builds every installer for which your Mac has the
tools, builds the site into `dist/site` and checks it, and says how each
installer was signed. With no credentials set, it releases everything but
an iOS app for phones, which only Apple's store can carry.

```bash
vero release --version 2.0.0 --notes "..."     # a version that you choose, with notes for the Mac's update prompt
vero release --targets deb,macos               # only some of the installers
```

**3. Look at it.** `vero-site` serves the site on your Mac.

```bash
vero-site -dir dist/site
```

Open the address that it prints. The page shows the install commands for
the visitor's system, and `/download?for=mac` hands out the disk image.

**4. Put it online.** Put the site's address in `site` in
`vero-app.toml`, so that the page and the repositories point there, and
release again. Then upload `dist/site` to that address. Any static web
host serves it, and `--upload` copies it for you.

```toml
site = "https://example.com/hello"
```

```bash
vero release --upload you@host:/srv/hello
```

To run your own server, `vero-site` serves the folder by itself, with the
page, the repositories, the `/download?for=SYSTEM` links and
`latest.json`. With `-domain`, it gets an HTTPS certificate from Let's
Encrypt for that name. An app that already has a Go web server can serve
the folder from it with `kit/site` instead.

```bash
vero-site -dir /srv/hello -domain downloads.example.com
```

[`vero-repo`](../cmd/vero-repo/README.md) describes what each system's
users get, the files in the site, and the three commands that
`vero release` runs, each of which can be run on its own.

## Signing, stores and listings

A release needs no account with anyone. The site's page tells people the
one click that an unsigned app asks of them. On a Mac, that is Open Anyway
in Privacy & Security, and on Windows it is More info, then Run anyway.
The Linux and BSD repositories are signed with your key and ask nothing.

For the rest, `vero credentials` is the checklist. It says what is set up,
finds what it can on your Mac, such as a Developer ID in your keychain and
the export line that uses it, and prints the exact commands for what is
not set up.

```bash
vero credentials
```

| For | What it takes | Then |
|---|---|---|
| macOS without Open Anyway | The Apple Developer Program, a Developer ID Application certificate in your keychain, a Developer ID Installer certificate for a `.pkg`, and the notarytool profile named `vero`, which `vero setup` makes. | Nothing more. `VERO_MAC_IDENTITY`, `VERO_MAC_INSTALLER` and `VERO_NOTARY_PROFILE` choose others. |
| Windows without SmartScreen | A code-signing certificate, as a `.pfx`, and `brew install osslsigncode`. | `VERO_WINDOWS_CERT` and `VERO_WINDOWS_CERT_PASSWORD`. |
| Android, with a keystore that you already ship with | The keystore. | `VERO_ANDROID_KEYSTORE`, `VERO_ANDROID_KEY_ALIAS` and `VERO_ANDROID_PASSWORD`. Without them, vero makes a keystore and keeps it beside your key. |
| iOS, on phones | The Apple Developer Program, an Apple Distribution certificate and an App Store profile. | `VERO_IOS_IDENTITY` and `VERO_IOS_PROFILE`. The `.ipa` goes up with Transporter. |
| Homebrew | `gh`, signed in. | `vero publish --homebrew` publishes to a tap of your own, `USER/homebrew-tap`, which it makes the first time. |
| winget | `gh`, signed in. | `vero publish --winget` opens a pull request to microsoft/winget-pkgs from a fork of yours. |
| The AUR | An account at aur.archlinux.org with your SSH key. | `vero publish --aur` publishes `NAME-bin`, which it makes the first time. |
| The Microsoft Store, Google Play and Flathub | An account with each. Flathub reviews by hand. | Upload the `.msix` or the `.aab` that vero built, or open a pull request to flathub/flathub. |

The next `vero release` reads the environment variables. Each kind of
publishing is a command of its own, run after the release, and
`vero publish --all` does each of the three for which the site has files.

Next, [updates](updates.md).
