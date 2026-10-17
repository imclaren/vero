# Updates

Each release reaches people through their system's own updates. apt, dnf,
zypper, pacman, apk, xbps, pkg, pkgin, IPS and Flatpak find it by themselves,
and OpenBSD's `pkg_add -u` finds it in the folder that the site names.

- **The Mac** reads the site's Sparkle appcast. Add the `VeroUpdates`
  product of vero's Swift package, and use `VeroUpdater`, which installs as
  soon as the app is idle and waits while its `isBusy` is true. Set `feed`
  and `sparkle_public_key`, which `vero key` prints, in `[macos]`. An Xcode
  project puts `SUFeedURL` and `SUPublicEDKey` in its own `Info.plist`.
- **An app that already uses Sparkle** keeps its key, with
  `vero key --dir DIR --import-sparkle FILE`.
- **Windows** updates through winget once the app is listed there. Besides
  that, the app asks `kit/update`, which reads the site's `latest.json`, and
  tells the person that a new version is out.

To try a build beside the release, `vero install` builds the Mac app,
signs it and puts it in `/Applications`, numbered just above the release,
as in `0.5.0.202610121530`. The release that follows replaces it as usual.
`--ldflags` adds what a release builds into the worker, and
`vero local-version` prints the number for an Xcode prebuild script.
