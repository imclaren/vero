#!/bin/sh
# Builds a vero app's installers from its vero-app.toml, into
# dist/packages: from a GTK front end, a .deb, an .rpm and a Flatpak, and
# from a WPF one, a Windows installer, each for x86_64 and ARM64.
#
# From your app's folder:
#
#   path/to/vero/scripts/package.sh --app vero-app.toml --version 1.2.3 [--targets deb,rpm,flatpak,windows]
#
# It reads the file with vero-repo, which runs package-linux.sh for the
# .deb and package-windows.sh for Windows, builds the .rpm itself, and
# makes the Flatpak in a container. So it needs Docker and colima for
# Linux, and makensis and dotnet for Windows. Then vero-repo build makes
# the site your users install from; the README says how.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
BIN="$HOME/.cache/vero/bin"
mkdir -p "$BIN"
go build -C "$VERO/cmd/vero-repo" -o "$BIN/vero-repo" .
exec "$BIN/vero-repo" package --vero "$VERO" "$@"
