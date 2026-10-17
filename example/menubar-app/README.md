# The macOS example

The same three jobs as every other example, as a SwiftUI menu bar app.
It drives the worker in [../worker](../worker).

## Run it

```bash
./build.sh
./.build/debug/MenuBarExample
```

Look for the icon in the menu bar.

## What is different here

The front end is Swift, which can link Go directly, so this example
links the worker's supervisor as a C archive built from
[../../cshim](../../cshim) and starts the worker binary from beside the
app. vero's Swift package does the same inside an app that `vero new`
writes.

## The files

| | |
|---|---|
| [Sources](Sources) | the front end |
| [Package.swift](Package.swift) | the Swift package, depending on vero's |
| [build.sh](build.sh) | builds the C archive, the worker and the app |

The end of [Build the app](../../docs/build-the-app.md) says how to add
vero to a Mac app of your own that has an Xcode project.
