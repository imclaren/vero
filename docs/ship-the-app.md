# Ship the app

[Test the app](test-the-app.md) left `jotter` passing on Debian, FreeBSD
and Windows. This page releases it: a site that people install from,
with an installer for each system and updates built in. Then it removes
the "unknown developer" warnings and puts the app in Homebrew. Every
step was run on `jotter`.

## 1. Make the first release

```bash
vero release --notes "First release."
```

It builds the worker for every system, then each system's packages,
the Mac app for Apple silicon and Intel, and the Windows installers for
x64 and ARM64. That takes a few minutes. At the end you should see:

```
checked dist/site: latest.json, the apt repository, the rpm repository, the Arch Linux repository, ...
built dist/site: jotter 1.0.0

signed:
  Linux             with your key; nothing more to do
  the BSDs, illumos with your key; nothing more to do
  macOS             with your Developer ID, not notarised (vero-repo credentials)
  Windows           not signed: SmartScreen asks once, as the page says (a certificate: vero-repo credentials)

released jotter 1.0.0 into dist/site
  look at it:   vero-site -dir dist/site
  publish it:   put site = "https://..." in vero-app.toml (where the folder will be), release again, and upload dist/site there
```

The two lines under "signed" for the Mac and Windows depend on what you
have set up; step 6 is about them. `dist/site` is a complete install
site, as plain files. It holds an `index.html` that sends each visitor to
the right download, `latest.json` with every download's address and
version, and for each system what its users get:

| System | What its users get |
|---|---|
| Debian, Ubuntu and Chromebooks | An apt repository. Added once, then updated with the rest of the system. |
| Fedora, openSUSE | An rpm repository. |
| Arch Linux | A pacman repository, and a recipe for the AUR. |
| Alpine, Void and Chimera | An apk or xbps repository, when the system's section is in `vero-app.toml`. |
| Any Linux, with Flatpak | A Flatpak repository and a `.flatpakref`, when asked for. |
| FreeBSD, DragonFly | A pkg repository. |
| NetBSD | A pkgsrc repository for pkgin. Unsigned, because pkgsrc checks no signatures, so HTTPS has to vouch for it. |
| OpenBSD | Signed packages for `pkg_add`. |
| illumos | An IPS repository on OpenIndiana, served as plain files. |
| macOS | A notarised disk image, an optional `.pkg`, a Sparkle appcast and a Homebrew cask. |
| Windows | An installer for x64 and ARM64, a winget manifest, and an MSIX when asked for. |

The version is 1.0.0 the first time. Each release after that is the next
patch version, unless `--version` says otherwise, and a release never
goes backwards on any system.

## 2. Look at the site

```bash
go run github.com/imclaren/vero/kit/cmd/vero-site@latest -dir dist/site
```

Open http://localhost:8080. You should see the install page, titled
"Install jotter": the app's name, summary and icon, one section per
system with the command or the download for it, and, once a full test
run has recorded them, the GIF of each system under "See it at work".
Follow the Mac link, and the disk image downloads. The same server
answers `/download?for=mac`, and `windows`, `linux` and so on, with a
redirect to the right installer, so a page of your own can link to a
download without knowing the version.

## 3. Back up the signing key

The first release made a key, in a folder named after the publisher:

```
~/Library/Application Support/vero-repo/example-publisher/
```

Copy that folder somewhere safe now, and never commit it. Every
installed copy of the app checks each update against this key. If the
key is lost, installed copies never update again, and the only way out
is for every person to install the app afresh.

## 4. Put the site online

Decide where the site will live, and tell `vero-app.toml`:

```toml
site = "https://example.com/jotter"
```

The site's address is built into the repositories and the appcast, so
release again now that it is set. Then either copy `dist/site` to any
static web host, or let vero copy it:

```bash
vero release --notes "First release." --upload user@host:/srv/jotter
```

`vero-site -dir /srv/jotter -domain downloads.example.com` serves the
folder on its own with an HTTPS certificate from Let's Encrypt, for a
host with nothing else on it. An app that already has a Go server can
serve it with `kit/site`.

## 5. Release again, and watch the update arrive

Change something in the worker, then:

```bash
vero release --notes "Notes can be deleted."
```

You should see `released jotter 1.0.1 into dist/site` at the end, and
`dist/site/latest.json` now says 1.0.1. Upload as before. Installed
copies get it through whatever updates the rest of their system:

- On Linux and the BSDs, the first install added the site as a package
  repository, so apt, dnf, zypper, pacman, apk, xbps, pkg, pkgin, IPS and
  Flatpak bring the update on their own. The test on the previous page
  checked exactly this.
- On the Mac, the app updates itself with Sparkle, reading the appcast on
  the site. To turn that on, add the `VeroUpdates` product of vero's
  Swift package to `macos/Package.swift`, make one `VeroUpdater` in the
  app and keep it, and give a menu item its `checkForUpdates()`. In
  `[macos]` of `vero-app.toml`, set `feed` to the appcast's address and
  `sparkle_public_key` to what `vero key` prints.
- On Windows, the app updates through winget once it is listed there
  (step 7). Until then, the worker can ask `kit/update`, which reads the
  site's `latest.json`, and the app can tell the person and open the
  download.

## 6. Remove the warnings

Without signing, the Mac asks the person to choose Open Anyway in
Privacy & Security, and Windows shows More info, then Run anyway.
Everything else needs nothing.

```bash
vero credentials
```

You should see a line per thing, with a tick or a cross and, for each
cross, the command that fixes it:

```
✓ everything signing key          in /Users/you/Library/Application Support/vero-repo/example-publisher
✓ macOS      Developer ID         found in your keychain
✗ macOS      notarisation         no profile
      xcrun notarytool store-credentials vero --apple-id you@example.com --team-id TEAMID --password <an app-specific password from appleid.apple.com>
✓ macOS      Homebrew tap         ready: gh is signed in
✗ Windows    code signing         not set; SmartScreen warns once
      buy a code-signing certificate (an OV one from any CA: Sectigo, DigiCert, SSL.com; EV ones skip SmartScreen at once), export it as a .pfx, then
      export VERO_WINDOWS_CERT=/path/to/cert.pfx VERO_WINDOWS_CERT_PASSWORD=...   (and brew install osslsigncode)
✓ Windows    winget               ready: gh is signed in
✗ Arch       AUR                  no account, or your SSH key isn't on it
```

For the Mac you need the Apple Developer Program and a Developer ID
Application certificate in your keychain, which `vero release` finds by
itself, plus the notarisation profile that `vero setup` offered to make.
For Windows you need a code-signing certificate as a `.pfx`, named in
the two environment variables shown. The next `vero release` signs with
whatever is set.

## 7. Publish to Homebrew, winget and the AUR

```bash
vero publish --homebrew     # a tap on your GitHub account
vero publish --winget       # a pull request to microsoft/winget-pkgs
vero publish --aur          # a package on aur.archlinux.org
vero publish --all          # whichever of these the site has files for
```

Homebrew and winget need `gh` signed in to GitHub. The AUR needs an
account at aur.archlinux.org with your SSH key on it. The Microsoft
Store and Flathub are by hand: build an MSIX with `[wpf.msix]` and
upload it in Partner Center, or open a pull request to flathub/flathub.

## 8. From now on

A release is one command, which tests everything first and stops at the
first failure:

```bash
vero ship --notes "What is new."
```

That is `vero ship --no-release` from the previous page, followed by
`vero release --upload` with the address in `vero-app.toml`. Then commit
and tag.

To move the app to a newer vero:

```bash
GOBIN=$(brew --prefix)/bin go install github.com/imclaren/vero/cmd/vero@latest
vero upgrade
```

`vero upgrade` replaces vero's own files in the app, runs `vero gen`, and
lists what changed and what to change in your own code. Build and test
on every system before the next release.
