#!/bin/sh
# Packages a vero app with a WPF front end as Windows installers, x64 and
# ARM64, and x86 if asked, from a Mac: the worker cross-compiled (build-all.sh), the app
# published self-contained (no .NET to install), and NSIS's makensis -
# which runs on the Mac - making an installer of each.
#
# From beside your app's go.mod:
#
#   path/to/vero/scripts/package-windows.sh --name myapp --version 1.2.3 \
#       --app windows --exe myapp.exe --worker ./cmd/worker --worker-name myapp-worker.exe \
#       --icon icon.png --publisher "Your Name or Company" [--url https://example.com] \
#       [--id myapp] [--startup "Open myapp when I sign in"] [--webview2] [--out dist/packages] \
#       [--arches "x64 arm64 x86"]
#
#   --app DIR      the WPF project's folder; dotnet publish builds it
#   --arches LIST  the installers to make: x64 and arm64 unless it says;
#                  x86 is for 32-bit Windows
#   --startup TEXT offers, ticked, to open the app at sign-in, as TEXT
#   --webview2     for an app that shows web pages with Microsoft Edge
#                  WebView2: the installer installs it where it's missing.
#                  Windows 11 has it, and so does most of Windows 10; for
#                  the rest, Microsoft's own small bootstrapper goes inside
#                  the installer, and downloads WebView2 when it runs.
#
# Each installs for the person running it, with no administrator needed,
# in %LOCALAPPDATA%\Programs\NAME: an update replaces it, closing the copy
# that is running first. Not signed: Windows' SmartScreen warns about an
# installer until a code-signing certificate signs it, which this leaves to
# you (signtool, osslsigncode), since a certificate is yours alone.
#
# Needs: brew install makensis dotnet. Nothing here names anyone: the
# publisher and the rest are what you pass.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
ROOT=$PWD
[ -f "$ROOT/go.mod" ] || { echo "run this from beside your app's go.mod" >&2; exit 2; }

ARCHES="x64 arm64" NAME="" VERSION="" APP="" EXE="" WORKER="" WORKER_NAME="" ICON="" PUBLISHER="" URL="" ID="" STARTUP="" WEBVIEW2=""
OUT="$ROOT/dist/packages"
while [ $# -gt 0 ]; do
    case $1 in
        --name) NAME=$2 ;; --version) VERSION=$2 ;; --app) APP=$2 ;; --exe) EXE=$2 ;;
        --worker) WORKER=$2 ;; --worker-name) WORKER_NAME=$2 ;; --icon) ICON=$2 ;;
        --publisher) PUBLISHER=$2 ;; --url) URL=$2 ;; --id) ID=$2 ;; --startup) STARTUP=$2 ;; --out) OUT=$2 ;;
        --ldflags) LDEXTRA=$2 ;; --arches) ARCHES=$2 ;;
        --webview2) WEBVIEW2=yes; shift; continue ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
    shift 2
done
for v in NAME VERSION APP EXE WORKER WORKER_NAME ICON PUBLISHER; do
    eval "[ -n \"\$$v\" ]" || { echo "--$(echo $v | tr 'A-Z_' 'a-z-') is needed" >&2; exit 2; }
done
command -v makensis >/dev/null || { echo "makensis is missing - brew install makensis" >&2; exit 1; }
DOTNET=$(command -v dotnet 2>/dev/null || true)
[ -z "$DOTNET" ] && [ -x "$HOME/.dotnet/dotnet" ] && DOTNET="$HOME/.dotnet/dotnet"
[ -n "$DOTNET" ] || { echo "dotnet is missing - brew install dotnet" >&2; exit 1; }
ID=${ID:-$NAME}
APP=$(cd "$APP" && pwd)
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
STAGE=$(mktemp -d "$HOME/.cache/vero-win.XXXXXX")
trap 'rm -rf "$STAGE"' EXIT

echo "building the worker"
DIST="$ROOT/dist"
# goarch ARCH: Go's name for one of Windows'.
goarch() { case $1 in x64) echo amd64 ;; x86) echo 386 ;; *) echo "$1" ;; esac; }
TARGETS=""
for arch in $ARCHES; do TARGETS="$TARGETS windows/$(goarch "$arch")"; done
WORKER=$WORKER CSHIM=none WPF_APP=none TARGETS="$TARGETS" \
    LDFLAGS="-s -w -X main.version=$VERSION ${LDEXTRA:-}" sh "$VERO/scripts/build-all.sh" >/dev/null
python3 "$VERO/scripts/lib/ico.py" "$ICON" "$STAGE/app.ico"
if [ -n "$WEBVIEW2" ]; then
    # Microsoft's Evergreen bootstrapper, which Microsoft lets apps ship:
    # https://learn.microsoft.com/microsoft-edge/webview2/concepts/distribution
    echo "fetching the WebView2 bootstrapper"
    curl -fsSL -o "$STAGE/MicrosoftEdgeWebview2Setup.exe" "https://go.microsoft.com/fwlink/p/?LinkId=2124703"
fi

for arch in $ARCHES; do
    goarch=$(goarch "$arch")
    echo "publishing for win-$arch"
    "$DOTNET" publish "$APP" -c Release -r "win-$arch" --self-contained -p:EnableWindowsTargeting=true \
        -p:Version="$VERSION" -o "$STAGE/$arch" -v quiet >/dev/null
    cp "$DIST/worker-windows-$goarch.exe" "$STAGE/$arch/$WORKER_NAME"
    set -- -V2 -DNAME="$NAME" -DVERSION="$VERSION" -DARCH="$arch" -DEXE="$EXE" -DWORKER="$WORKER_NAME" \
        -DPUBLISHER="$PUBLISHER" -DURL="$URL" -DID="$ID" -DICON="$STAGE/app.ico" -DSRC="$STAGE/$arch" \
        -DOUT="$OUT/$NAME-$VERSION-$arch-setup.exe"
    [ -n "$STARTUP" ] && set -- "$@" -DSTARTUP="$STARTUP"
    [ -n "$WEBVIEW2" ] && set -- "$@" -DWEBVIEW2="$STAGE/MicrosoftEdgeWebview2Setup.exe"
    makensis "$@" "$VERO/scripts/lib/installer.nsi"
    # Signed with the publisher's certificate when there is one, from the
    # environment, as the Mac's identity is. Without, Windows SmartScreen
    # asks once, and the site's page says what to choose.
    if [ -n "${VERO_WINDOWS_CERT:-}" ]; then
        command -v osslsigncode >/dev/null || { echo "VERO_WINDOWS_CERT is set, but osslsigncode is missing - brew install osslsigncode" >&2; exit 1; }
        exe="$OUT/$NAME-$VERSION-$arch-setup.exe"
        osslsigncode sign -pkcs12 "$VERO_WINDOWS_CERT" ${VERO_WINDOWS_CERT_PASSWORD:+-pass "$VERO_WINDOWS_CERT_PASSWORD"} \
            -n "$NAME" ${URL:+-i "$URL"} -h sha256 -t http://timestamp.digicert.com \
            -in "$exe" -out "$exe.signed" >/dev/null
        mv "$exe.signed" "$exe"
        echo "signed $exe"
    fi
    echo "built $OUT/$NAME-$VERSION-$arch-setup.exe"
done
