# vero's documentation

This page is the index of vero's documentation. The guides follow a
developer's path from an empty Mac to an app that people install and
update on every system, and the reference pages hold the details to which
the guides link.

## Guides

Read these in order. Each one ends with a link to the next.

1. [Set up your Mac](setup.md) installs vero and the tools that each system needs.
2. [Start an app](start.md) makes a new app with `vero new` or `vero ship`, or brings an existing app to vero, and describes the app's folder.
3. [Write the worker](worker.md) explains the Go half of the app, its state and requests, and what makes it build for every system.
4. [The front ends](front-ends.md) explains the native half of the app on each system, the starters that `vero add` writes, and how to add vero to an Xcode project by hand.
5. [Build, run and test on every system](test.md) runs the app in containers and virtual machines on your Mac, tests its installers, and records each system at work.
6. [Package and release](release.md) builds the installers and the install site with `vero ship` or `vero release`, and covers signing and the stores.
7. [Updates](updates.md) explains how each system updates the app, how to try a build beside the release, and how to move an app to a newer vero.

## Reference

- [The `vero` command](../cmd/vero/README.md) lists every command and flag, and what `vero add` writes.
- [`vero-app.toml`](../PACKAGING.md) lists the settings that describe an app to the packaging and the tests.
- [`vero-repo`](../cmd/vero-repo/README.md) describes the installers, repositories and site for each system, the processors for which each is packaged, and the packaging commands that `vero release` runs.
- [kit](../kit/README.md) lists the Go packages that give a worker one API for secrets, notifications, sign-in items, sleep, helper programs and updates.
- [The bindings and the examples](bindings.md) lists vero's library for each language and the example app on each system.
- [The package documentation](https://pkg.go.dev/github.com/imclaren/vero) describes the Go API.
- The scripts in [scripts](../scripts) each describe their options in a comment at the top.

## For vero's own developers

- [Contributing](../CONTRIBUTING.md) explains how to test vero itself.
- [vero's Windows VM](../.windows/unattend/README.md) explains how the Windows virtual machine is installed and reached over SSH.
- [The plan](../PLAN.md) is the plan for vero 0.18.
