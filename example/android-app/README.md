# The Android example

The same three jobs as every other example, on a phone.

```bash
scripts/setup-android.sh     # once: the SDK, a JDK, Kotlin and an emulator, about 5 GB
scripts/run-android.sh       # build, install and launch in the emulator
```

![the Android example](../../docs/screenshots/android.gif)

## What is different here

Nothing on the Go side. The worker is the one in [../worker](../worker),
cross-compiled for Android, started by the front end and spoken to over
a pipe, exactly as on the desktop.

Two things are particular to Android:

- **The worker ships as `lib/arm64-v8a/libworker.so`.** Android will only
  run an executable from the app's own native library folder, and only
  puts files named `lib*.so` there. The file is an ordinary Go
  executable, whatever its name says.
- **`android:extractNativeLibs="true"` in the manifest.** Without it the
  worker stays compressed inside the APK, and there is no path from which
  to start it.

There is no NDK and no JNI. With cgo off, `GOOS=android` builds without a
C toolchain, and Kotlin writes and reads JSON over the pipe just as the
Python and C# bindings do.

## The files

| | |
|---|---|
| [src/com/imclaren/vero/example/MainActivity.kt](src/com/imclaren/vero/example/MainActivity.kt) | the front end: three rows, a Restart button each |
| [../../bindings/kotlin/Vero.kt](../../bindings/kotlin/Vero.kt) | the binding: one process, one reader thread, JSON |
| [AndroidManifest.xml](AndroidManifest.xml) | the application, and the two settings above |
| [build.sh](build.sh) | the worker, then `kotlinc`, `d8`, `aapt2`, `apksigner` and `adb` |

There is no Gradle. The build calls the six tools that Gradle would
call, which is shorter than the project files needed to drive Gradle,
and the point of the example is the worker rather than the build system.
