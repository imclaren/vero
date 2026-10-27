# The kit

The worker is plain Go and runs the same on every system, but a few
things an app needs differ by system: where secrets are kept, how a
notification is shown, how to stop the machine sleeping. The kit is one
Go package for each, with one API that works everywhere. It is a module
of its own, so that vero's core library has no dependencies:

```bash
go get github.com/imclaren/vero/kit
```

| Package | What it does |
|---|---|
| [keychain](https://pkg.go.dev/github.com/imclaren/vero/kit/keychain) | Keeps a secret in the system's secure store: the login keychain, the Credential Manager, or the Secret Service. |
| [profile](https://pkg.go.dev/github.com/imclaren/vero/kit/profile) | Keeps a test or development copy of the app apart from the real one, by `VERO_PROFILE`. |
| [awake](https://pkg.go.dev/github.com/imclaren/vero/kit/awake) | Keeps the computer from sleeping while work must finish, and says when it has woken. |
| [autostart](https://pkg.go.dev/github.com/imclaren/vero/kit/autostart) | Opens the app when the person signs in. |
| [notify](https://pkg.go.dev/github.com/imclaren/vero/kit/notify) | Shows a desktop notification from the worker, where the system allows that. |
| [update](https://pkg.go.dev/github.com/imclaren/vero/kit/update) | Says whether a newer version is on the app's site. |
| [tools](https://pkg.go.dev/github.com/imclaren/vero/kit/tools) | Finds a helper program the app ships, such as ffmpeg, wherever the app is installed. |
| [site](https://pkg.go.dev/github.com/imclaren/vero/kit/site) | Serves the install site from an app's own Go server. |
| [cmd/vero-site](https://pkg.go.dev/github.com/imclaren/vero/kit/cmd/vero-site) | Serves the install site on its own, with HTTPS from Let's Encrypt. |

Each package's documentation on pkg.go.dev has an example and says what
it does on each system. Every package builds for every system vero
supports, without cgo. [Build the app](../docs/build-the-app.md) builds the app using the `keychain`.
