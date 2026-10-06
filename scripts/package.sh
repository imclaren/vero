#!/bin/sh
# Builds a vero app's installers from its vero-app.toml: a .deb for each of
# Debian and Ubuntu's architectures from a GTK front end, and a Windows
# installer for x64 and ARM64 from a WPF one, into dist/packages.
#
# From your app's folder:
#
#   path/to/vero/scripts/package.sh --app vero-app.toml --version 1.2.3 [--targets linux,windows]
#
# It reads the file with vero-repo, which then runs package-linux.sh and
# package-windows.sh, so it needs what they do: Docker and colima for the
# .deb, makensis and dotnet for Windows. Then vero-repo build makes the
# site your users install from; the README says how.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
BIN="$HOME/.cache/vero/bin"
mkdir -p "$BIN"
go build -C "$VERO/cmd/vero-repo" -o "$BIN/vero-repo" .
exec "$BIN/vero-repo" package --vero "$VERO" "$@"
