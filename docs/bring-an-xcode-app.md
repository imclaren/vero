# Bring an Xcode app

A Mac app that already has an Xcode project keeps it. vero adds the
worker, the generated binding and the other systems' front ends around
it.

## Steps

1. Put the worker in `cmd/worker` of a Go module, and run `vero add` in
   the module's folder. That writes the GTK and WPF front ends and a
   `vero-app.toml`.
2. In Xcode, add the package `https://github.com/imclaren/vero` with the
   rule **Up to Next Major Version**, and link its `Vero` product. Before
   1.0, "Up to Next Minor" would stop the app from getting new versions
   of vero.
3. In `vero-app.toml`, point `[macos]` at the project with `folder`,
   `project` and `scheme`. A release then builds the worker for Intel and
   Apple silicon and puts it in the app's `Contents/Resources`.
4. In the app's code, start the worker and claim the single-copy lock:

   ```swift
   @StateObject private var vero = VeroModel<Status>(bundledWorker: "worker", directoryName: "MyApp/bin")

   init() { VeroSingleCopy.yieldToRunningCopy() }
   ```

5. Run `vero gen`. It writes `WorkerAPI.swift` beside the scheme's
   sources, or wherever `swift` under `[gen]` says. If the project does
   not yet reference the file, `vero upgrade` adds it.

[example/menubar-app](../example/menubar-app) is a whole app built this
way.
