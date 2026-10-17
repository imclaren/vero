# The Windows example

The same three jobs as every other example, in a WPF window. This is the
front end that `vero new` writes for Windows, in the form of a finished
example. It drives the worker in [../worker](../worker).

## Run it from the Mac

From a checkout of vero:

```bash
./scripts/run-windows.sh
```

This builds the app and the worker, puts them on a disc image, and boots
vero's Windows 11 virtual machine with it. The VM has to exist first;
step 5 of [Test the app](../../docs/test-the-app.md) makes it. Inside
Windows, copy the folder from the CD drive to the desktop and run the
exe.

## Run it on Windows directly

```powershell
.\build.ps1
.\bin\Release\net8.0-windows\VeroExample.exe
```

This needs the .NET SDK and Go.

## What is different here

Nothing on the Go side. Like the GTK example, the C# front end starts
`worker.exe` with `VERO_HOST` set, and the worker supervises a second
copy of itself. The binding is the conversation over the pipe, and
nothing else.

## The files

| | |
|---|---|
| [MainWindow.xaml](MainWindow.xaml), [MainWindow.xaml.cs](MainWindow.xaml.cs) | the front end: three rows, a Restart button each |
| [App.xaml](App.xaml), [App.xaml.cs](App.xaml.cs), [VeroExample.csproj](VeroExample.csproj) | the application and its project |
| [build.ps1](build.ps1) | builds the worker and the app, on Windows |
| [../../bindings/csharp/Vero.cs](../../bindings/csharp/Vero.cs) | the binding: one process, one reader thread, JSON |

## Making one of your own

`vero new hello` writes an app with this front end, a SwiftUI one and a
GTK one, around a worker of your own. The [README](../../README.md) walks
through it.
