# Describe the app

`vero-app.toml`, beside the app's `go.mod`, describes the app to the
packaging and the tests. `vero new` and `vero add` write it, and
[example/vero-app.toml](../example/vero-app.toml) has a comment on every
setting. Paths in it are relative to the file.

- **The app.** `name`, `display_name`, `id`, `summary`, `description`,
  `publisher`, `homepage`, `licence`, `icon`, and `site`, the install
  site's address. `tray` or `menubar` sets the app's shape, and `schemes`
  and `opens` the links and file types that it opens.
- **The worker.** `[worker]` names its `package` and its program's `name`.
- **The front ends.** A system is packaged when its section is there, as
  `[macos]`, `[gtk]`, `[wpf]`, and `[gtk.freebsd]` and its kind for each
  other system. [vero-repo](../cmd/vero-repo/README.md) lists what each
  system's users get and the names of its processors.
- **Packages.** `uses` in `[gtk]` names what the app uses of `adwaita`,
  `webview` and `keyring`, and `extra` names programs such as `ffmpeg`.
  vero works out each system's packages from them.
- **What the app needs.** `[needs]` sets `network`, `files`, `keyring`,
  `notifications`, `autostart` and `webview`, which the Flatpak and the
  Microsoft Store declare.
- **The tests.** `[test]` and `[[test.step]]`, as in
  [Write test steps](write-test-steps.md).
