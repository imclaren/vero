# Windows example

Run the following script to build and run this example on a Mac. This script
builds the app, puts it on a disc image, and boots a Windows VM which runs the
app:

```bash
git clone https://github.com/imclaren/vero && cd vero
./scripts/run-windows.sh
```

## Create a vero Windows app on your Mac

This builds the app above outside the repository, so what you end up with is
yours to change. It drives the same worker as the macOS app.

### 1. Install the .NET SDK on your Mac

```bash
curl -sSL https://dot.net/v1/dotnet-install.sh | bash -s -- --channel 8.0
export PATH="$HOME/.dotnet:$PATH"
```

That is the only toolchain needed. The worker is pure Go, so it
cross-compiles here, and there is no C library to build for Windows at all.

### 2. Build the go worker

The worker is [example/worker/main.go](../worker/main.go), and it does not need
the repository cloned:

```bash
mkdir -p ~/vero-example/wpf-app && cd ~/vero-example/wpf-app
go mod init wpf-app
go get github.com/imclaren/vero

CGO_ENABLED=0 GOOS=windows GOARCH=arm64 \
    go build -o worker.exe github.com/imclaren/vero/example/worker
```

### 3. Add the app's files to the same directory

The app is [VeroExample.csproj](VeroExample.csproj), [App.xaml](App.xaml),
[App.xaml.cs](App.xaml.cs), [MainWindow.xaml](MainWindow.xaml) and
[MainWindow.xaml.cs](MainWindow.xaml.cs), and
[Vero.cs](../../bindings/csharp/Vero.cs) is the C# binding. Download all six:

```bash
base=https://raw.githubusercontent.com/imclaren/vero/main
curl -O $base/bindings/csharp/Vero.cs
for f in VeroExample.csproj App.xaml App.xaml.cs MainWindow.xaml MainWindow.xaml.cs; do
    curl -O $base/example/wpf-app/$f
done
```

`Vero.cs` starts `worker.exe` with `VERO_HOST=1`, which makes it supervise a
second copy of itself and answer on its standard input and output. Restarts,
backoff and the single-worker lock are all in the worker, so the binding is
only the conversation.

### 4. Build the exe

```bash
dotnet publish -c Release -r win-arm64 --self-contained \
    -p:EnableWindowsTargeting=true -o out
cp worker.exe out/
```

### 5. Run it on your Mac (on a virtual machine)

The first time, install Windows into the VM from a
[Microsoft](https://www.microsoft.com/en-us/software-download/windows11arm64)
ISO. The install is unattended, and happens once:

```bash
brew install qemu
git clone https://github.com/imclaren/vero
vero/scripts/run-windows.sh --iso ~/Downloads/win11.iso --install --payload out
```

After that, this boots the same VM with your latest build. `--payload out`
packs `out/` into a disc image, which the VM sees as a CD drive:

```bash
vero/scripts/run-windows.sh --payload out
```

Windows opens in a window on your Mac. In it, copy the `vero` folder from the
CD drive to your home directory, and run `VeroExample.exe` inside it.

## See also

- [example/wpf-app](.) - this app, finished and runnable
- [README](../../README.md) - the same worker with a macOS SwiftUI app
- [example/gtk-app](../gtk-app) - the same worker with a Linux GTK4 app
