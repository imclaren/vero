# The kit

The worker is plain Go and runs the same on every system. But some
things an app needs are different on each system: where secrets are
kept, how a notification is shown, how to stop the machine sleeping.
The kit is a set of Go packages that each do one of those things, with
one API that works everywhere.

The kit is its own module, so that vero's core library has no
dependencies:

```bash
go get github.com/imclaren/vero/kit
```

Every package builds for every system vero supports, without cgo.
[Build the worker for every system](../docs/build-the-worker-everywhere.md)
explains when a worker needs the kit rather than a package of its own.

## Packages

### keychain

Stores secrets in the system's secure store: the login keychain on
macOS, the Credential Manager on Windows, the Secret Service on Linux and
the BSDs. Where there is none, a file that only this user can read.

```go
item := keychain.Item{Service: "com.example.hello", Account: "token/" + user}
err := item.Save([]byte(token))
saved, err := item.Load()
```

[Secrets and sign-in](../docs/secrets-and-sign-in.md) has more.

### profile

Keeps a test or development copy of the app apart from the copy that a
person uses. With `VERO_PROFILE=demo`, `profile.Dir("myapp")` is the
folder `myapp-demo` in the user's configuration folder instead of
`myapp`, and keychain items go under `com.example.myapp.demo`. The copy
under test can neither read nor change the other's data.

```go
dir, err := profile.Dir("myapp")   // where this copy keeps its files
```

The keychain package applies the profile by itself. vero's tests set it
through `[test] env` in `vero-app.toml`, and vero's Swift package
follows it for the folder it runs the worker from.

### awake

Keeps the computer from sleeping while the worker has work that must
finish, and says when the computer has woken, so that the worker can
check what changed meanwhile.

```go
release, err := awake.Hold("Syncing your folders")
defer release()

stop := awake.OnWake(func() { resync() })
```

`Hold` uses caffeinate on macOS, `SetThreadExecutionState` on Windows,
and logind's sleep inhibitor on Linux and the BSDs that run it. Where
there is nothing to use, as on illumos, it returns `ErrUnsupported` and
a release that does nothing. `OnWake` works everywhere.

### autostart

Opens the app when the person signs in: a LaunchAgent on macOS, a Run
registry entry on Windows, and an autostart `.desktop` file on Linux,
the BSDs and illumos.

```go
err := autostart.Enable(app)
on, err := autostart.Enabled(app)
```

On macOS, an app in a bundle is better off asking from Swift with
`SMAppService`, which lists the app under Login Items in System
Settings. Use this package on macOS only for a worker that runs outside
an app.

### notify

Shows a desktop notification from the worker, on the systems that let a
program other than the app do so: over D-Bus on Linux and the BSDs, and
as a toast on Windows.

```go
err := notify.Send(ctx, notify.Notification{Title: "Done", Body: "3 files synced"})
```

On macOS and iOS, only the app may notify, and `Send` returns
`ErrFrontEnd`. The pattern that works everywhere is for the worker to
put what is new in the state and let the front end show the
notification.

### update

Tells the app whether a newer version is on its site, by reading the
`latest.json` or the Sparkle appcast that a release writes. Sparkle
handles the Mac on its own; this is for the other systems, or for an
app's own "Check for updates" button.

```go
result, err := update.Check(ctx, "https://example.com/myapp/latest.json", version)
```

`update.Handler` wraps that as a ready-made request handler.

### tools

Finds a helper program that the app ships beside the worker, such as
ffmpeg, wherever the app is installed on each system: inside the bundle
on macOS, beside the worker on Windows, under the app's lib folder on
Linux and the BSDs.

```go
path, err := tools.Find("ffmpeg")   // ready for os/exec
```

Ship one build of the program for each system and architecture. On the
Mac, `resources` in `[macos]` puts them in the bundle, and for a
Flatpak, `resources` in `[gtk.flatpak]` does the same.

### site

Serves the install site from an app's own Go server. The site is plain
files, so this is optional; any web host serves it. Use it when the
downloads should live beside the server's own pages, or should be
available only to people who have signed in.

```go
http.Handle("/", site.Handler(site.Options{Dir: "dist/site"}))
```

It also answers `/download?for=windows&arch=x64` with a redirect to that
installer, so that a download page need not know version numbers.
`site.Private` wraps the handler with a sign-in check.

### cmd/vero-site

A standalone server for the install site, with an HTTPS certificate from
Let's Encrypt, for an app that has no web server of its own:

```bash
vero-site -dir /srv/myapp -domain downloads.example.com
```
