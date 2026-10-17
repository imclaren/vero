# Bring an Xcode app

A Mac app that has its own Xcode project keeps it.

1. Put the worker in `cmd/worker` of a Go module, and run `vero add` in the
   module's folder for the other systems.
2. In Xcode, add the package `https://github.com/imclaren/vero`, with the
   rule **Up to Next Major Version**, and its product `Vero`. Before 1.0,
   Up to Next Minor would leave the app behind.
3. Name the project in `vero-app.toml`, with `folder`, `project` and
   `scheme` in `[macos]`. The release builds the worker for Intel and Apple
   silicon and puts it in `Contents/Resources`.
4. Start the worker with
   `VeroModel<Status>(bundledWorker: "worker", directoryName: "MyApp/bin")`,
   and call `VeroSingleCopy.yieldToRunningCopy()` in the app's `init`.
5. Run `vero gen`, which writes `WorkerAPI.swift` beside the scheme's
   sources, or where `swift` in `[gen]` says. `vero upgrade` adds the file
   to a project that lacks it.

[example/menubar-app](../example/menubar-app) is a whole app built this way.
