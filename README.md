# vero

vero makes desktop apps from a Go worker and a native front end on each
system, and builds, tests and releases them for every system from one Mac.

| | |
|:---:|:---:|
| **macOS**<br><a href="example/menubar-app"><img src="docs/screenshots/macos.gif" width="300"></a><br>SwiftUI, on the Mac itself. | **Windows**<br><a href="example/wpf-app"><img src="docs/screenshots/windows.gif" width="300"></a><br>WPF, in a Windows 11 virtual machine. |
| **Linux**<br><a href="example/gtk-app"><img src="docs/screenshots/linux.gif" width="300"></a><br>GTK 4, in a container, and on Chromebooks. | **FreeBSD**<br><a href="example/gtk-app"><img src="docs/screenshots/freebsd.gif" width="300"></a><br>The same GTK app, in a virtual machine. |
| **OpenBSD**<br><a href="example/gtk-app"><img src="docs/screenshots/openbsd.gif" width="300"></a><br>The same GTK app, in a virtual machine. | **NetBSD**<br><a href="example/gtk-app"><img src="docs/screenshots/netbsd.gif" width="300"></a><br>The same GTK app, in a virtual machine. |
| **DragonFly**<br><a href="example/gtk-app"><img src="docs/screenshots/dragonfly.gif" width="300"></a><br>The same GTK app, emulated on x86. | **illumos**<br><a href="example/gtk-app"><img src="docs/screenshots/illumos.gif" width="300"></a><br>The same GTK app, on OpenIndiana. |

Install vero once, with Go and Xcode on the Mac.

```bash
GOBIN=$(brew --prefix)/bin go install github.com/imclaren/vero/cmd/vero@latest && vero setup
```

Then three commands do the rest.

```bash
vero new hello   # makes an app with a front end for each of these systems
vero ship        # in hello/, tests it everywhere and releases its installers and site
vero upgrade     # moves the app to a newer vero
```

[AGENTS.md](AGENTS.md) · [Recipes](docs/README.md) · [The lab](docs/lab.md) ·
[Go reference](https://pkg.go.dev/github.com/imclaren/vero) · [Licence](LICENSE)
