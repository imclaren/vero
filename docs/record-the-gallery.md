# Record the gallery pictures

*Step 12 of 15 in [the vero guide](README.md).*

Every test run records the app's window as a GIF, plus a still of its
last frame. The recordings go on the install page, under "See it at
work", so that people can see the app on their own system before they
install it.

## Where recordings go

Each run writes `app.gif` and `app.png` to
`~/.cache/vero/test-results/APP/SYSTEM/`, and
`~/.cache/vero/test-results/index.html` shows them all on one page.

## Recording every system

```bash
vero ship --no-release
```

This tests and records every system that this Mac can run, including the
Mac itself. Look at the results page and check that each recording shows
the app doing something sensible. `vero release` then puts each passing
system's GIF on the install page.

The full `test-all.sh` run records too, when `[test]` has `record = true`.
A quick run does not, unless given `--record`.

## Recording the Mac on its own

```bash
path/to/vero/scripts/record-mac.sh --app vero-app.toml --bundle Hello.app \
    --env VERO_PROFILE=test
```

Three things to know:

- The bundle must have a bundle identifier of its own, different from the
  copy you use, so that its preferences are separate. Build it with
  `xcodebuild PRODUCT_BUNDLE_IDENTIFIER=com.example.hello.dev`.
- `--clean PATH` removes a file or folder under `~/Library` first, such
  as the settings left by the last recording, so that each recording
  starts like a fresh install.
- The terminal needs Screen Recording permission in System Settings.
  Without it, `--no-record` still plays the steps and reports whether
  they passed.

## vero's own gallery

Each of vero's `scripts/run-*.sh` scripts takes `--record out.gif`, as in
`scripts/run-freebsd.sh --record docs/screenshots/freebsd.gif`. That is
how the pictures in vero's README are made.

---

Next: [Release](release.md).
