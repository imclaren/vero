# Release

`vero ship` tests every system and then releases. `vero release` releases
without the tests. Each runs in the app's folder.

```bash
vero ship --notes "What is new."                   # test, record and release
vero release --notes "What is new."                # release only
vero release --version 2.0.0 --targets deb,macos   # a version and installers of your choosing
vero-site -dir dist/site                           # look at the site on this Mac
```

- The version is the next after the newest on the site, and a release
  never goes backwards on any system.
- The first release makes the signing key, in a folder named after the
  publisher under `~/Library/Application Support/vero-repo`. Back it up and
  keep it out of git. Without it, installed copies never update again.
- Put the site's address in `site` in `vero-app.toml`, then release with
  `--upload user@host:/path`, or copy `dist/site` to any static host.
  `vero-site -dir DIR -domain downloads.example.com` serves it with HTTPS,
  and `kit/site` serves it from a Go server that the app has already.
- Unsigned, the Mac app asks for Open Anyway in Privacy & Security, and
  Windows for More info, then Run anyway. [Sign and publish](sign-and-publish.md)
  removes both.
