#!/bin/sh
# Runs the GTK library's tests (bindings/python/test_vero_gtk.py) in
# containers, in each case the library must handle:
#
#   debian:trixie        libadwaita 1.7 and WebKitGTK 6
#   debian:trixie plain  the same with VERO_GTK_PLAIN=1, as plain GTK 4
#   ubuntu:24.04         libadwaita 1.5.0, the oldest the library uses
#   debian:bookworm      libadwaita 1.2, which is too old, and GTK 4.8
#
# each with a session bus (dbus-run-session) and without one. It needs
# Docker, and Go to build the example worker for the containers.
#
#   scripts/test-gtk-libs.sh [image ...]
set -eu
cd "$(dirname "$0")/.."
arch=$(docker info --format '{{.Architecture}}' 2>/dev/null || echo x86_64)
case $arch in aarch64|arm64) goarch=arm64 ;; *) goarch=amd64 ;; esac
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
GOOS=linux GOARCH=$goarch CGO_ENABLED=0 go build -o "$work/worker" ./example/worker
# A worker that keeps a sign-in with kit/keychain, in a module of its own,
# since vero's own module does not need the kit.
root=$(pwd)
kc="$work/keychainworker-src"
mkdir -p "$kc"
grep -v '^//go:build ignore' bindings/python/testdata/keychainworker/main.go >"$kc/main.go"
printf 'module keychainworker\n\ngo 1.26\n\nrequire (\n\tgithub.com/imclaren/vero v0.0.0\n\tgithub.com/imclaren/vero/kit v0.0.0\n)\n\nreplace github.com/imclaren/vero => %s\n\nreplace github.com/imclaren/vero/kit => %s/kit\n' "$root" "$root" >"$kc/go.mod"
(cd "$kc" && go mod tidy >/dev/null 2>&1 && GOOS=linux GOARCH=$goarch CGO_ENABLED=0 go build -o "$work/keychainworker" .)
rm -rf "$kc"
cp bindings/python/vero.py bindings/python/vero_gtk.py bindings/python/test_vero_gtk.py "$work/"

images=${*:-"debian:trixie ubuntu:24.04 debian:bookworm"}
status=0
# The oldest Python the libraries must run on is 3.9, which is what
# OpenIndiana's PyGObject is for.
if (cd bindings/python && tar cf - vero.py vero_gtk.py) |
    docker run --rm -i python:3.9-slim sh -c "mkdir /t && cd /t && tar xf - && python3 -m py_compile vero.py vero_gtk.py"; then
    echo "== Python 3.9: the libraries compile"
else
    echo "FAIL: the libraries do not compile with Python 3.9"
    status=1
fi
for image in $images; do
	packages="python3-gi gir1.2-gtk-4.0 gir1.2-adw-1 xvfb xauth dbus dbus-x11 fonts-dejavu-core"
	case $image in *trixie*|*forky*|*sid*|ubuntu*) packages="$packages gir1.2-webkit-6.0" ;; esac
	for plain in "" 1; do
		case $image in *bookworm*) [ -n "$plain" ] && continue ;; esac
		label="$image${plain:+ plain}"
		adwaita=0 webview=0
		case $image in *trixie*|*forky*|*sid*|ubuntu*) webview=1; [ -z "$plain" ] && adwaita=1 ;; esac
		echo "== $label"
		if ! tar -C "$work" -cf - . | docker run --rm -i -e VERO_GTK_PLAIN="$plain" -e VERO_EXPECT_ADWAITA=$adwaita -e VERO_EXPECT_WEBVIEW=$webview "$image" sh -c "
			set -e
			apt-get update -qq >/dev/null
			DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends $packages >/dev/null 2>&1
			mkdir /t && cd /t && tar xf -
			export VERO_TEST_WORKER=/t/worker VERO_TEST_KEYCHAIN_WORKER=/t/keychainworker
			# WebKit cannot start its sandbox in an unprivileged container.
			export WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1
			echo '-- with a session bus'
			xvfb-run -a dbus-run-session -- python3 -m unittest test_vero_gtk${VERO_TEST_ONLY:+.GtkLibraryTests.$VERO_TEST_ONLY} 2>&1 | tail -${VERO_TEST_LINES:-25}
			echo '-- without a session bus'
			DBUS_SESSION_BUS_ADDRESS=unix:path=/nonexistent/vero-test-bus xvfb-run -a python3 -m unittest test_vero_gtk${VERO_TEST_ONLY:+.GtkLibraryTests.$VERO_TEST_ONLY} 2>&1 | tail -${VERO_TEST_LINES:-25}
		" | tee "$work/out.txt"; then
			status=1
		fi
		grep -q '^FAILED\|^Error\|Traceback' "$work/out.txt" && status=1
		# A container that did not start prints nothing, which is a failure too.
		[ "$(grep -c '^OK' "$work/out.txt")" -eq 2 ] || { echo "FAIL: $label did not run both ways"; status=1; }
	done
done
exit $status
