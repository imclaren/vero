# Secrets and sign-in

*Step 7 of 15 in [the vero guide](README.md).*

Tokens, passwords and anything else that must not sit in a plain file go
through `kit/keychain`. It stores each secret in the system's own secure
store: the login keychain on the Mac, the Credential Manager on Windows,
and the Secret Service on Linux and the BSDs. Where there is none, it
falls back to a file only this user can read.

## Storing and reading a secret

```go
item := keychain.Item{Service: "com.example.hello", Account: "token/" + user}

err := item.Save([]byte(token))
saved, err := item.Load()   // nil with no error when nothing is stored
err = item.Remove()
```

`Service` is your app's identifier, and `Account` names which secret.

## Keeping test secrets apart

When `VERO_PROFILE` is set, every item is stored under a service name
that includes the profile. A development or test copy of the app can
therefore never read, or overwrite, the secrets of the copy you really
use. A Go test should go further and set `File: true` with a `Path` in a
temporary folder, so that it never touches a keychain at all.

## On the Mac, sign the worker consistently

On the Mac, a keychain item belongs to the code signature that wrote it.
If a development build of the worker is signed differently from the last
one, the Mac asks the person for permission after every rebuild. Sign the
worker with the same identity in development as in a release.

## OAuth sign-in

A sign-in that goes through a web page needs a web view that stops at the
redirect and hands the code back to the app:

- On the Mac, use Apple's `ASWebAuthenticationSession`.
- In the GTK app, use `vero_gtk.SignInPage`.
- In the WPF app, use `VeroSignInPage`.

The GTK and WPF pages fall back to the system browser where there is no
web view.

## What never goes in the repository

Nothing secret goes in `vero-app.toml` or in git. An OAuth client's ID,
which is not secret but should not be hard-coded for every fork, is
built into the worker at release time:

```bash
vero release --ldflags "-X main.clientID=..."
```

---

Next: [Build the worker for every system](build-the-worker-everywhere.md).
