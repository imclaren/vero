# Secrets and sign-in

Keep tokens and passwords with `kit/keychain`, which uses the login
keychain on the Mac, the Credential Manager on Windows and the Secret
Service on Linux and the BSDs.

```go
item := keychain.Item{Service: "com.example.hello", Account: "token/" + user}
err := item.Save([]byte(token))
saved, err := item.Load()   // nil, with no error, when nothing is kept
```

- With `VERO_PROFILE` set, every item is kept under the profile's own
  service, so a test copy never sees the person's secrets. A Go test sets
  `File: true` and a `Path` in a temporary folder.
- On the Mac, an item belongs to the code signature that wrote it. Sign the
  worker with the same identity in development and in a release, or the
  Mac asks the person after every build.
- For an OAuth sign-in, use `vero_gtk.SignInPage` on GTK and
  `VeroSignInPage` on Windows, which stop at the redirect and fall back to
  the browser where there is no web view, and Apple's
  `ASWebAuthenticationSession` on the Mac.
- Nothing secret goes in `vero-app.toml` or git. An OAuth client's ID is
  built into the worker with `vero release --ldflags "-X main.clientID=..."`.
