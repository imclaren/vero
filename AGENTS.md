# Working on a vero app

These are the rules and commands for a coding agent that makes or changes
an app built on vero. The [recipes](docs/README.md) hold the detail. Every
command here runs on a Mac with Apple silicon, Go and Xcode.

## The shape of an app

- **The worker** is a Go program in `cmd/worker`. It holds the app's state,
  answers the front ends' requests, and does the app's work.
- **The state** is one Go struct, made with `vero.NewState`. vero pushes
  it to every window whenever its JSON changes, so no front end asks for it.
- **A request** is a named handler on the worker. `vero.Update` takes no
  data, `vero.UpdateWith` takes a struct, `vero.UpdateItem` names one item
  of a list by its `Key()`, and each replies with the new state.
  `vero.Handle` replies with a value of its own.
- **The front ends** start the worker and talk to it in JSON over a pipe.
  They are SwiftUI in `macos/`, GTK 4 in Python in `gtk/` for Linux, the
  BSDs and illumos, and WPF in C# in `windows/`.
- **`vero-app.toml`** describes the app to the packaging and the tests.
  **`PORTING.md`** lists what in the worker needs attention on each system.

## Make an app, or bring one to vero

```bash
vero new hello             # a window app, with the three desktop front ends
vero new hello --tray      # a window app with an icon in the menu bar and the tray
vero new hello --menubar   # an app that lives in the menu bar and the tray
vero ship counter.go       # an app from one Go file, tested and released
```

To bring an app that has a Go worker already, move the worker into
`cmd/worker` of the app's module, run `vero add` in the module's folder,
and work through `PORTING.md`. An existing Xcode project keeps its project,
as [Bring an Xcode app](docs/bring-an-xcode-app.md) describes. `vero new`
puts this file into the app, and `vero upgrade` puts it into an app that
`vero new` did not make.

## Rules for the code

- Never edit the files that vero copies into the app. These are `vero.py`,
  `vero_gtk.py`, `Vero.cs`, `VeroWpf.cs`, `VeroWebView.cs` and this
  `AGENTS.md`, and `vero upgrade` replaces them. Change what vero's
  libraries do through their settings or by subclassing.
- Never edit the generated files `WorkerAPI.swift`, `WorkerApi.cs` and
  `worker_api.py`. After any change to a request, to what it takes, or to
  the state, run `vero gen`. `vero gen --check` fails when one is stale.
- A handler holds the state's lock. Slow work goes in a goroutine that
  writes back through `state.Do`, and a `vero.Handle` handler stops when
  `ctx.Done()` closes.
- Keep `var version` in the worker's main package. The release sets it.
- The worker is built without cgo for every system. Use the
  [kit](kit/README.md) for secrets, notifications, sign-in items, sleep and
  updates, and give anything else that is specific to one system a build tag.
- Commit `.vero-files.json`. It records what vero wrote, so that
  `vero upgrade` can tell vero's copy from an edit.

## Never touch the person's real data

A copy of the app that an agent runs must keep its settings, data and
secrets apart from the copy that the person uses.

- The worker keeps its files in `profile.Dir(name)` and its secrets in
  `kit/keychain`. Both follow `VERO_PROFILE`, as does vero's Swift package.
- Run every development or test copy with `VERO_PROFILE` set, such as
  `VERO_PROFILE=dev ./.build/debug/Hello`. The tests set it in `[test] env`,
  and `vero ship` records the Mac only when they do.
- On the Mac, a copy built with Xcode also needs a bundle ID of its own, so
  that its preferences are apart from the real app's. Build it with
  `xcodebuild PRODUCT_BUNDLE_IDENTIFIER=com.example.hello.dev`.
- Before the first run, check that the copy's folder, preferences and
  keychain items all carry the profile's name.

## Build and test

The scripts are in a checkout of vero (`git clone
https://github.com/imclaren/vero`), shown here as `path/to/vero`.

```bash
cd macos && ./build.sh                    # the Mac app; it prints the command that runs it
go test ./... && go vet ./...             # the worker
vero gen --check                          # the generated requests are current
vero ship --plan                          # what builds and tests on this Mac, and what cannot

path/to/vero/scripts/test-all.sh --app vero-app.toml --quick             # every day: Debian and Windows
path/to/vero/scripts/test-all.sh --app vero-app.toml                     # the full run, before a release
path/to/vero/scripts/test-all.sh --app vero-app.toml --only "debian freebsd"
path/to/vero/scripts/test-all.sh --app vero-app.toml --no-vms            # containers only
path/to/vero/scripts/test-repo.sh --app vero-app.toml --vm windows       # one system
```

`test-all.sh` installs the app on each system from a site that it builds
and plays the `[[test.step]]` entries. The full run also checks that an
update arrives, records each window when `[test]` has `record = true`, and
adds DragonFly and illumos, which are emulated and slow. `--update` and
`--record` add those to a quick run. Each system starts from a ready-made
copy with the app's dependencies installed, and a system that passed with
nothing changed since is skipped, unless `--again` is given. Results are in
`~/.cache/vero/test-results/index.html` and logs in
`~/.cache/vero/test-runs/`. Its systems are debian, debian12,
ubuntu, fedora, opensuse, arch, alpine, chimera, void, flatpak, freebsd,
netbsd, openbsd, dragonfly, illumos and windows. `scripts/doctor.sh` says
what this Mac lacks for each.

## Develop on the Mac first

- Build each feature on the Mac and test it there with `VERO_PROFILE` set.
- Before each release, bring the GTK and WPF front ends up to the Mac in one
  batch, then run `test-all.sh` on every system.
- A bug fix goes to every front end at once, in the same change, even
  when it was found on one system only.
- A change to the worker reaches every system without a port. Run
  `vero gen` and rebuild each front end.

## Release

1. Run `vero gen --check`, `go test ./...` and `go vet ./...`.
2. Run `vero ship --plan`, and fix every system on which the worker fails
   to build.
3. Run `vero ship --no-release`, which tests and records every system that
   this Mac can run. Open `~/.cache/vero/test-results/index.html` and check
   that every system passed and that each recording shows the app at work.
4. Run `vero ship --notes "What is new."`, or `vero release --notes "..."`
   to package without testing again. The version is the next after the
   newest on the site, unless `--version` gives one.
5. Put the site's address in `site` in `vero-app.toml`, and upload with
   `--upload user@host:/path`.
6. Back up the signing key's folder, which the first release makes. A
   lost key means that installed copies never update again.
7. Commit, and tag the release.

`vero credentials` says what is set up for signing and the stores.

## Move to a newer vero

```bash
GOBIN=$(brew --prefix)/bin go install github.com/imclaren/vero/cmd/vero@latest
vero upgrade
```

`vero upgrade` moves `go.mod`, the kit, the Swift package and the Windows
project to the new vero, copies vero's files afresh, runs `vero gen`, and
lists what it changed and what to change in the app's own code. A copied
file that the app has edited is left alone and named. Move the edit into
the app's own code and run it again, or pass `--force` to replace the file.
Then build and test on every system before the next release.
