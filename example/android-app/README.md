# The Android example

The same three jobs as every other example, on a phone:

```bash
scripts/setup-android.sh     # once: the SDK, a JDK, Kotlin, an emulator
scripts/run-android.sh       # build, install, launch
```

![the Android example](../../docs/screenshots/android.gif)

## What is different here

Nothing, on the Go side. The worker is the one in [../worker](../worker),
cross-compiled for Android, started by the frontend and spoken to over a pipe
- the arrangement every platform but iOS and the browser uses.

Two things are particular to Android:

* **The worker ships as `lib/arm64-v8a/libworker.so`.** Android will only run
  an executable from the application's own native library directory, and only
  puts files named `lib*.so` there. It is an ordinary Go executable, not a
  library, whatever the name says.
* **`android:extractNativeLibs="true"`.** Without it the worker stays
  compressed inside the APK, where there is no path to start it from.

There is no NDK and no JNI. `CGO_ENABLED=0` is what lets `GOOS=android` build
without a C toolchain, and Kotlin writes and reads JSON over the pipe exactly
as the Python and C# bindings do.

## The files

| | |
|---|---|
| [src/com/imclaren/vero/example/MainActivity.kt](src/com/imclaren/vero/example/MainActivity.kt) | the frontend: three rows, a Restart button each |
| [../../bindings/kotlin/Vero.kt](../../bindings/kotlin/Vero.kt) | the client: one process, one reader thread, JSON |
| [AndroidManifest.xml](AndroidManifest.xml) | the application, and the two settings above |
| [build.sh](build.sh) | the worker, `kotlinc`, `d8`, `aapt2`, `apksigner`, `adb` |

No Gradle. The build is the six tools Gradle would call, which is shorter than
the project files needed to drive it, and the point of the example is the
worker rather than the build system.
