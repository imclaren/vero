# Updates

*Step 15 of 15 in [the vero guide](README.md).*

Once an app is installed, each release reaches the person through
whatever updates the rest of their system. There is nothing to build into
the app for most systems.

## Linux and the BSDs

The install site is a package repository for each system, and the first
install adds it. After that, apt, dnf, zypper, pacman, apk, xbps, pkg,
pkgin, IPS and Flatpak find new releases on their own. On OpenBSD,
`pkg_add -u` looks in the folder the site names.

## The Mac

The Mac app updates itself with Sparkle, reading the appcast on the site.
To turn that on:

1. Add the `VeroUpdates` product of vero's Swift package to the app.
2. Make one `VeroUpdater` and keep it for the life of the app. It gives
   you a `checkForUpdates()` for a menu item and installs a downloaded
   update as soon as the app is idle. Set its `isBusy` while the app
   must not be interrupted.
3. In `[macos]` of `vero-app.toml`, set `feed` to the appcast's address
   and `sparkle_public_key` to the key that `vero key` prints. An Xcode
   project sets the same two as `SUFeedURL` and `SUPublicEDKey` in its own
   `Info.plist`.

An app that already uses Sparkle keeps its existing key:
`vero key --dir DIR --import-sparkle FILE`.

## Windows

Windows updates the app through winget once it is listed there. Apart
from that, the app can ask `kit/update`, which reads the site's
`latest.json` and says whether a newer version is out, so that the app
can tell the person and open the download.

## Trying a build beside the release

```bash
vero install
```

This builds the Mac app from the working copy, signs it, and puts it in
`/Applications` with a version just above the current release, such as
`0.5.0.202610121530`. The next real release replaces it like any other
update. `--ldflags` passes what a release would build into the worker,
and `vero local-version` prints the same version number, for an Xcode
prebuild script that wants to match it.

---

That is the last step. Back to [the vero guide](README.md).
