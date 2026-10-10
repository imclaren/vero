# The GTK example, for Linux, the BSDs and illumos

Run the following script to build and run this example on a Mac. This script
builds the app in a container, runs it on a virtual display, and opens it in
Screen Sharing:

```bash
git clone https://github.com/imclaren/vero && cd vero
./scripts/run-linux.sh
```

The same app, unchanged, on FreeBSD - in a VM, because there is no FreeBSD
container to run on a Mac:

```bash
./scripts/run-freebsd.sh
```

The first FreeBSD run downloads the official cloud image and installs GTK4 in
it, which takes a few minutes; after that it boots in seconds.
`--reset` throws the VM away and starts again.

## Create a vero Linux app on your Mac

This builds the app above outside the repository, so what you end up with is
yours to change. It drives the same worker as the macOS app.

### 1. Nothing to install

The worker is pure Go, so it cross-compiles on your Mac, and there is no
shared library to build on Linux at all. Docker only comes into it at the end,
to *run* the example without a Linux machine.

### 2. Build the go worker

The worker is [example/worker/main.go](../worker/main.go), and it does not need
the repository cloned:

```bash
mkdir -p ~/vero-example/gtk-app && cd ~/vero-example/gtk-app
go mod init gtk-app
go get github.com/imclaren/vero

CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    go build -o worker github.com/imclaren/vero/example/worker
```

### 3. Add the app's files to the same directory

The app is [main.py](main.py), and [vero.py](../../bindings/python/vero.py) is
the Python binding. Download both:

```bash
base=https://raw.githubusercontent.com/imclaren/vero/main
curl -O $base/bindings/python/vero.py
curl -O $base/example/gtk-app/main.py
chmod +x main.py
```

`vero.py` starts the worker with `VERO_HOST=1`, which makes it supervise a
second copy of itself and answer on its standard input and output. Restarts,
backoff and the single-worker lock are all in the worker, so the binding is
only the conversation.

### 4. Run it on your Mac (in a container)

This runs the app on a virtual display and opens it in Screen Sharing:

```bash
docker build -t vero-gtk - <<'EOF'
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      python3 python3-gi gir1.2-gtk-4.0 libgtk-4-1 xvfb x11vnc xauth \
    && rm -rf /var/lib/apt/lists/*
EOF

docker run --rm -p 5901:5900 -v "$PWD":/app -w /app vero-gtk sh -c '
    Xvfb :99 -screen 0 480x440x24 &
    sleep 2
    export DISPLAY=:99
    python3 main.py &
    sleep 4
    x11vnc -display :99 -forever -nopw -listen 0.0.0.0'

open vnc://localhost:5901
```

On a Linux machine, run it directly instead:

```bash
sudo apt-get install -y python3-gi gir1.2-gtk-4.0
chmod +x main.py && ./main.py
```

On FreeBSD, the same thing with its own names:

```bash
pkg install -y gtk4 py312-pygobject
chmod +x main.py && ./main.py
```

Nothing in the app is Linux-specific. It needs GTK4, PyGObject and a worker
binary built for the machine it runs on - and because the binding spawns the
worker rather than loading a library, there is no shared library to find for
either of them.

## See also

- [example/gtk-app](.) - this app, finished and runnable
- [README](../../README.md) - the same worker with a macOS SwiftUI app
- [example/wpf-app](../wpf-app) - the same worker with a Windows WPF app
