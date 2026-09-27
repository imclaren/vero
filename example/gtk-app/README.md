# Linux example

Run the following script to build and run this example on a Mac. This script
builds the app in a container, runs it on a virtual display, and opens it in
Screen Sharing:

```bash
git clone https://github.com/imclaren/vero && cd vero
./scripts/run-linux.sh
```

## Create a vero Linux app on your Mac

This builds the app above outside the repository, so what you end up with is
yours to change. It drives the same worker as the macOS app.

### 1. Install colima and Docker on your Mac

`libvero.so` has to be compiled on Linux, so the next step builds it in a
container. Everything else runs on your Mac.

```bash
brew install colima docker && colima start
```

### 2. Build the go worker

The worker is [example/worker/main.go](../worker/main.go), and `libvero.so` is the C shim the
app loads. Neither needs the repository cloned, but the library has to be
compiled on Linux, so that part happens in a container:

```bash
mkdir -p ~/vero-example/gtk-app && cd ~/vero-example/gtk-app
go mod init gtk-app
go get github.com/imclaren/vero

docker run --rm -v "$PWD":/src -w /src \
    -e GOCACHE=/tmp/gocache -e GOPATH=/tmp/go -e GOTOOLCHAIN=auto \
    golang:1.24-bookworm \
    sh -c 'CGO_ENABLED=1 go build -buildmode=c-shared -o libvero.so github.com/imclaren/vero/cshim'

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
