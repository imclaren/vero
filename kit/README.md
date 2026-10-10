# kit

What a worker needs from the system it runs on, with one API for all of
them. A module of its own, `github.com/imclaren/vero/kit`, so that vero's
library keeps no dependencies; `go get github.com/imclaren/vero/kit`
brings it in. [PACKAGING.md](../PACKAGING.md) is the guide that uses it.

| Package | What it does | Where |
|---|---|---|
| `kit/keychain` | Keeps sign-ins and other secrets | The login keychain on macOS, the Credential Manager on Windows, the Secret Service on Linux and the BSDs, and a file only this user can read elsewhere |
| `kit/profile` | Keeps a test or demo copy of the app apart from the copy in use: with `VERO_PROFILE=demo`, `profile.Dir("myapp")` is `myapp-demo` in the user's configuration folder, and `kit/keychain` keeps its items under `com.example.myapp.demo` | Every system; vero's tests set it through `vero-app.toml`'s `[test] env`, and vero's Swift package follows it for the folder it runs the worker from |
| `kit/autostart` | Opens the app at sign-in | A LaunchAgent on macOS (from an app bundle, `SMAppService` from Swift is better), the Run key on Windows, an autostart `.desktop` file on Linux and the BSDs |
| `kit/notify` | Shows a notification from the worker | The desktop's notification service on Linux and the BSDs, a toast on Windows; on macOS and iOS notifications come from the app, so keep what is new in the state and let the front end notify |
| `kit/update` | Says whether a newer version is on your site | Reads the `latest.json` and the Sparkle appcast that `vero-repo build` writes |
| `kit/tools` | Finds helper programs the app ships, such as ffmpeg | Wherever the app is installed on each system, or a folder you name |
| `kit/site` | Serves your install site from your own Go server, with `/download?for=mac` (or `windows`, `linux`, `pkg` and so on) sending each visitor to their installer | For an app with a server already, or one whose downloads are for people who have signed in (`site.Private`); the site is plain files, so this is optional |
| `kit/cmd/vero-site` | Serves your install site on its own, with an HTTPS certificate from Let's Encrypt | For an app without a web server: `vero-site -dir /srv/myapp -domain downloads.example.com` |

Each builds for every system vero does, without cgo.

## Helper programs

A worker that runs a program it ships - ffmpeg, say - needs a build of it
for each system and architecture, and finds it with `kit/tools`, which
looks wherever the app is installed. (How the installers carry them is
coming.)
