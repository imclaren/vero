# Record the gallery pictures

Every test records the app's window as a GIF and a picture of its last
frame, in `~/.cache/vero/test-results/APP/SYSTEM/`, and
`~/.cache/vero/test-results/index.html` shows them all on one page.

- `vero ship --no-release` records every system that this Mac can run,
  the Mac included. `vero release` then puts each passing system's GIF on
  the install page, under "See it at work".
- On the Mac alone, `path/to/vero/scripts/record-mac.sh --app vero-app.toml
  --bundle Hello.app --env VERO_PROFILE=test` records a build of the app.
  The bundle must have an ID of its own, `--clean PATH` removes what the
  last recording left under `~/Library`, and the terminal needs Screen
  Recording permission.
- For vero's own gallery, each of vero's `scripts/run-*.sh` takes
  `--record out.gif`, as in `scripts/run-freebsd.sh --record
  docs/screenshots/freebsd.gif`.
