# Updates

This page explains how an app that people have installed gets each new
release, how to try a build on your own Mac beside the release, and how to
move an app to a newer vero.

## Releasing an update

Change the app and run `vero release`, or `vero ship`, again. The version
goes up by one, and people get it with their usual updates.

- apt, on Debian, Ubuntu and Chromebooks with Linux turned on in Settings
  → Developers, and dnf, zypper, pacman, apk, xbps, pkg, pkgin and Flatpak
  find it by themselves.
- OpenBSD's `pkg_add -u` finds it in the folder that the site's page says
  to give it.
- A Mac app with Sparkle reads the site's appcast, as the next section
  describes.
- On Windows and Android, the app can read `latest.json` from the site, or
  ask `kit/update`, and tell people that a new version is out.

A release never goes backwards. `vero-repo` checks that no system's
version is lower than before, since people would then never be offered
the update.

## The Mac app and Sparkle

The site's `macos/appcast.xml` is a Sparkle feed, and each release in it
is signed with your Sparkle key. vero's Swift package has a `VeroUpdates`
product, whose `VeroUpdater` is Sparkle set up for an app that people
leave open. It installs an update as soon as the app is not busy, rather
than when the app quits, and its `isBusy` holds the update back while the
worker is in the middle of something.

The app's `Info.plist` needs `SUFeedURL`, the appcast's address, and
`SUPublicEDKey`, the public half of the key, which `vero-repo key` prints.
With `feed` and `sparkle_public_key` in `[macos]` in `vero-app.toml`, vero
writes both into the `Info.plist` of a Swift package app. An Xcode
project's own `Info.plist` has to give them itself.
[`vero-repo`](../cmd/vero-repo/README.md#macos) has the rest of the Mac's
settings.

A Mac app that already updates itself with Sparkle keeps its key, so that
copies already installed go on updating.
[`vero-repo key --import-sparkle`](../cmd/vero-repo/README.md#the-signing-key)
takes it.

## Trying a build on your own Mac

`vero install` builds the Mac app from your checkout, signs it, and puts
it in `/Applications` in place of the copy there. It numbers the build
just above the newest release, as in `0.5.0.202610121530`, which is the
release's number followed by the date and time. A build numbered below
the release would be replaced by it, since Sparkle installs any release
with a higher number and vero keeps whichever worker is newer. This one is
above the release that is out and below the next one, which replaces it
as usual.

vero finds the release's number in a `latest.json` in the app's folder,
on a `releases` branch, or at the app's site, and `--published 1.2.3`
gives it. `--ldflags` adds to the worker's build flags what a release
builds into it, such as an OAuth client's ID. `vero local-version` prints
the number, for a prebuild script that numbers Xcode's own builds in the
same way.

## Moving an app to a newer vero

`vero upgrade`, in the app's folder, brings an app made with vero 0.13 or
later up to the vero that you have installed. It moves `go.mod`, the kit,
the Mac app's dependency on vero's Swift package, which it sets to Up to
Next Major Version, and the Windows app, to .NET 10 with Windows Forms for
the tray icon. It copies vero's files into each front end afresh, writes
the dependencies in `vero-app.toml` as `uses` and `extra`, runs `vero gen`,
and adds `WorkerAPI.swift` to an Xcode project that needs it. Then it
lists what it changed, and what to change in the app's own code, such as a
type of the app's own that `WorkerAPI.swift` now defines.

`.vero-files.json` records the hash of each file that vero wrote. A file
that has been changed since is left alone, and `vero upgrade` names it.
`--force` replaces it all the same, and `--version v0.18.0` upgrades to a
version other than the one that you have installed.

[The `vero` command](../cmd/vero/README.md) lists every command and its
flags.
