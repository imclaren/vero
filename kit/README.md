# kit

This page lists the kit's packages, which give a worker what it needs from
the system on which it runs, with one API for every system. The kit is a
module of its own, `github.com/imclaren/vero/kit`, so that vero's library
keeps no dependencies, and `go get github.com/imclaren/vero/kit` adds it to
an app. [Build the worker for every system](../docs/build-the-worker-everywhere.md)
explains when a worker needs it.

| Package | What it does | Where |
|---|---|---|
| `kit/keychain` | Keeps sign-ins and other secrets | The login keychain on macOS, the Credential Manager on Windows, the Secret Service on Linux and the BSDs, and a file only this user can read elsewhere |
| `kit/profile` | Keeps a test or demo copy of the app apart from the copy in use. With `VERO_PROFILE=demo`, `profile.Dir("myapp")` is `myapp-demo` in the user's configuration folder, and `kit/keychain` keeps its items under `com.example.myapp.demo` | Every system. vero's tests set it through `vero-app.toml`'s `[test] env`, and vero's Swift package follows it for the folder from which it runs the worker |
| `kit/awake` | Keeps the computer from sleeping while the worker has work that must finish, such as a sync, and says when the computer has woken, so that the worker can look again at what changed | `caffeinate` on macOS, `SetThreadExecutionState` on Windows, and logind's sleep inhibitor on Linux and the BSDs that run it; telling of a wake works on every system |
| `kit/autostart` | Opens the app at sign-in | A LaunchAgent on macOS (from an app bundle, `SMAppService` from Swift is better), the Run key on Windows, an autostart `.desktop` file on Linux and the BSDs |
| `kit/notify` | Shows a notification from the worker | The desktop's notification service on Linux and the BSDs, a toast on Windows; on macOS and iOS notifications come from the app, so the worker keeps what is new in the state and lets the front end notify |
| `kit/update` | Says whether a newer version is on your site | Reads the `latest.json` and the Sparkle appcast that `vero-repo build` writes |
| `kit/tools` | Finds helper programs the app ships, such as ffmpeg | Wherever the app is installed on each system, or a folder you name |
| `kit/site` | Serves your install site from your own Go server, with `/download?for=mac` (or `windows`, `linux`, `pkg` and so on) sending each visitor to their installer | For an app with a server already, or one whose downloads are for people who have signed in (`site.Private`); the site is plain files, so this is optional |
| `kit/cmd/vero-site` | Serves your install site on its own, with an HTTPS certificate from Let's Encrypt | For an app without a web server, as in `vero-site -dir /srv/myapp -domain downloads.example.com` |

Each builds for every system vero does, without cgo.

## Helper programs

A worker that runs a program that the app ships, such as ffmpeg, needs a
build of that program for each system and architecture, and finds it with
`kit/tools`, which looks wherever the app is installed. On the Mac,
`resources` in `[macos]` puts such programs in the app, and on Linux,
`resources` in `[gtk.flatpak]` puts them in the Flatpak, as
[example/vero-app.toml](../example/vero-app.toml) describes.
