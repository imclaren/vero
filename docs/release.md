# Release

A release builds the app for every system, packages it, and writes a
complete install site to `dist/site`. Two commands do it, and both run in
the app's folder.

```bash
vero ship --notes "What is new."      # test and record every system, then release
vero release --notes "What is new."   # release without testing
```

Use `vero ship` for a real release and `vero release` when you have just
tested and only want to package again.

## Your first release, step by step

1. Run `vero gen --check`, `go test ./...` and `go vet ./...`.
2. Run `vero ship --plan`. It builds the worker for every system and says
   which fail. Fix those first;
   [Build the worker for every system](build-the-worker-everywhere.md)
   lists the usual causes.
3. Run `vero ship --no-release`. This installs and tests the app on every
   system the Mac can run and records each one. Open
   `~/.cache/vero/test-results/index.html` and check that every system
   passed and that each recording looks right.
4. Run `vero ship --notes "First release."`. The notes appear in the Mac's
   update prompt.
5. **Back up the signing key.** The first release makes one, in a folder
   named after the publisher under
   `~/Library/Application Support/vero-repo`. Keep it out of git and
   somewhere safe. Installed copies check updates against this key. If it
   is lost, every installed copy stops updating, for ever.
6. Put the site online. Set `site` in `vero-app.toml` to the address where
   it will live, release again, and either copy `dist/site` to any static
   web host or pass `--upload user@host:/path` to have vero copy it.
7. Commit, and tag the release.

## Later releases

The version is the next patch after the newest one on the site, unless
`--version` says otherwise. A release never goes backwards on any system.

```bash
vero release --version 2.0.0 --targets deb,macos   # choose the version and the installers
vero-site -dir dist/site                           # serve the site on this Mac, to look at it
```

## What the site holds

`dist/site` is plain files: an `index.html` that sends each visitor to
the right installer, an installer for each system, a package repository
for each Linux and BSD package manager, a Sparkle appcast and a
`latest.json` for updates, and a Homebrew cask and winget manifest. The
[vero-repo README](../cmd/vero-repo/README.md) lists exactly what each
system's users get.

To serve it yourself with HTTPS, `vero-site -dir DIR -domain downloads.example.com`
gets a certificate from Let's Encrypt. An app that already has a Go
server can serve it with `kit/site`.

## Unsigned releases

A release works without any developer account. The Mac then shows a
warning and asks the person to choose Open Anyway in Privacy & Security,
and Windows shows More info, then Run anyway.
[Sign and publish](sign-and-publish.md) removes both warnings.
