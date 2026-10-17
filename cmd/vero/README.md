# The vero command

`vero` is the one command you use day to day. It makes an app, writes
its front ends, generates their bindings, and builds, tests and releases
the whole thing.

```bash
GOBIN=$(brew --prefix)/bin go install github.com/imclaren/vero/cmd/vero@latest
```

Run `vero` with no arguments for every command and its flags, and
`vero COMMAND --help` for one command's flags.

## The commands

| Command | What it does |
|---|---|
| `vero setup` | Installs what vero needs on this Mac, and says what is still missing. |
| `vero new NAME` | Makes a new app: a Go module with an example worker and a front end for each desktop system. |
| `vero add [front ends]` | Adds front ends to an app that already has a worker in `cmd/worker`. |
| `vero gen` | Writes each front end's typed binding from the worker's Go types. `--check` fails when one is stale. |
| `vero ship` | Tests and records the app on every system this Mac can run, then releases it. `--plan` says what it would do. `--no-release` stops before releasing. |
| `vero release` | Builds and packages a release without testing. |
| `vero install` | Builds the Mac app from the working copy and installs it, numbered just above the current release. |
| `vero upgrade` | Moves an app to this version of vero. |
| `vero credentials` | Says what is set up for signing and the stores. |
| `vero publish` | Publishes a release to Homebrew, winget or the AUR. |

`vero key`, `package`, `build`, `check`, `steps`, `show` and `plan` are
[vero-repo](../vero-repo/README.md)'s own commands, which `vero` hands on.

## Using a checkout of vero

`--vero DIR`, or `VERO_DIR` for `vero setup` and `vero ship`, uses a
checkout of vero in place of the published module. That is how to try a
change to vero against an app.

[AGENTS.md](../../AGENTS.md) shows the commands in the order an app uses
them.
