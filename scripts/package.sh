#!/bin/sh
# Builds a vero app's installers from its vero-app.toml, into
# dist/packages: from a GTK front end, a .deb and an rpm for Linux, and a
# package for each of FreeBSD, DragonFly, NetBSD, illumos and OpenBSD that
# the file has a section for, plus a Flatpak if it asks for one; from a
# WPF one, a Windows installer. Each is for x86_64 and ARM64 where the
# system has both.
#
# From your app's folder:
#
#   path/to/vero/scripts/package.sh --app vero-app.toml --version 1.2.3 [--targets deb,rpm,freebsd,windows,...]
#
# vero-repo makes them all in Go, except the Windows installers, which
# package-windows.sh makes with makensis and dotnet, and a Flatpak, which
# needs Docker and colima. Then vero-repo build makes the site your users
# install from; the README says how.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
BIN="$HOME/.cache/vero/bin"
mkdir -p "$BIN"
# vero-repo: the one VERO_REPO names (vero ship sets it), or built from
# this checkout.
if [ -n "$VERO_REPO" ]; then cp "$VERO_REPO" "$BIN/vero-repo"; else go build -C "$VERO/cmd/vero-repo" -o "$BIN/vero-repo" .; fi
exec "$BIN/vero-repo" package --vero "$VERO" "$@"
