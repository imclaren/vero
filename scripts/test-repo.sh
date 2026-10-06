#!/bin/sh
# Tests vero's packaging from start to finish on the example app, as a
# user would meet it: builds the packages, builds the site with a
# throwaway key, serves it from this Mac, and in a clean container adds
# the repository the way the site says, installs the example, and checks
# its worker. Then it releases 1.0.1 into the same site and checks that the
# system's own updates bring it.
#
#   scripts/test-repo.sh                          # Debian's apt
#   scripts/test-repo.sh --image ubuntu:24.04     # another apt system
#   scripts/test-repo.sh --image fedora:latest    # dnf, which checks every signature
#   scripts/test-repo.sh --flatpak                # Flatpak, from the .flatpakref
#   scripts/test-repo.sh --image archlinux        # the AUR recipe, with makepkg
#   [--port 8642]
#
# Needs Docker and colima, as package-linux.sh does. Everything it makes
# is in ~/.cache/vero, the key included, so nothing in it is anyone's. The
# Flatpak test keeps GNOME's runtime, which it downloads from Flathub the
# first time, in a Docker volume called vero-test-flatpak; remove it with
# docker volume rm vero-test-flatpak.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
. "$VERO/scripts/lib/docker.sh"
IMAGE=debian:bookworm PORT=8642 FLATPAK=""
while [ $# -gt 0 ]; do
    case $1 in
        --image) IMAGE=$2; shift ;; --port) PORT=$2; shift ;;
        --flatpak) FLATPAK=yes ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
    shift
done
vero_docker
CACHE="$HOME/.cache/vero"
KEY="$CACHE/example-key" SITE="$CACHE/example-site" PACKAGES="$CACHE/example-packages"
# The container reaches this Mac by this name: colima and Docker Desktop
# both answer it.
URL="http://host.docker.internal:$PORT"
NAME=vero-test-repo
mkdir -p "$CACHE/bin"
go build -C "$VERO/cmd/vero-repo" -o "$CACHE/bin/vero-repo" .
REPO="$CACHE/bin/vero-repo"
[ -f "$KEY/private.asc" ] || "$REPO" key --dir "$KEY" --name "vero example" --email example@example.com
rm -rf "$SITE"

# GTK 4 for Python, as the example starts it: the package's dependencies
# have to bring it, typelibs and all.
GTK="import gi; gi.require_version('Gtk', '4.0'); from gi.repository import Gtk"

# What to build, and how to install, check and update, on this system.
if [ -n "$FLATPAK" ]; then
    IMAGE=debian:trixie TARGETS=flatpak
    # The volume keeps GNOME's runtime between runs; the example, and
    # the remote its .flatpakref added, go, so that it's installed afresh.
    SETUP="apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq flatpak curl ca-certificates > /dev/null
        flatpak uninstall -y --noninteractive dev.vero.example > /dev/null 2>&1 || true
        for r in \$(flatpak remotes --columns=name); do [ \$r = flathub ] || flatpak remote-delete --force \$r; done"
    INSTALL="flatpak install -y --noninteractive $URL/flatpak/vero-example.flatpakref > /dev/null"
    WORKER="flatpak run --command=/app/lib/vero-example/worker dev.vero.example -version"
    CHECK="ls /var/lib/flatpak/exports/share/applications/dev.vero.example.desktop /var/lib/flatpak/exports/share/icons/hicolor/128x128/apps/dev.vero.example.png && flatpak run --command=python3 dev.vero.example -c \"$GTK\""
    UPDATE="flatpak update -y --noninteractive > /dev/null"
    RUN="--privileged -v vero-test-flatpak:/var/lib/flatpak"
else
    INSTALL="curl -fsSL $URL/install.sh | sh"
    WORKER="/usr/lib/vero-example/worker -version"
    RUN=""
    CHECK="command -v vero-example && ls /usr/share/applications/dev.vero.example.desktop /usr/share/metainfo/dev.vero.example.metainfo.xml /usr/lib/vero-example/vero.py && python3 -c \"$GTK\""
    case $IMAGE in
        archlinux*)
            # Arch's image is x86_64 only: Docker runs it emulated here,
            # where pacman's download sandbox can't start, so it's turned
            # off. The recipe is built by an ordinary user, as makepkg
            # insists.
            TARGETS=deb RUN="--platform linux/amd64"
            SETUP="sed -i 's/^#DisableSandbox/DisableSandbox/' /etc/pacman.conf
                grep -q '^DisableSandbox' /etc/pacman.conf || sed -i '/^\[options\]/a DisableSandbox' /etc/pacman.conf
                pacman -Syu --noconfirm --needed base-devel sudo curl > /dev/null 2>&1
                useradd -m builder && echo 'builder ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/builder"
            INSTALL="su builder -c 'rm -rf ~/pkg && mkdir ~/pkg && cd ~/pkg && curl -fsSLO $URL/aur/PKGBUILD && makepkg -si --noconfirm > /dev/null 2>&1'"
            UPDATE="$INSTALL" ;;
        fedora*)
            TARGETS=rpm
            SETUP="dnf install -y -q curl > /dev/null"
            # dnf refuses a package or an index that isn't signed; this
            # checks that the package is.
            CHECK="$CHECK && rpm -qi vero-example | grep -A1 '^Signature' | grep -q RSA"
            # --refresh: dnf looks for a new index every six hours
            # otherwise.
            UPDATE="dnf upgrade --refresh -y -q > /dev/null" ;;
        *)
            TARGETS=deb
            SETUP="apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq curl ca-certificates > /dev/null"
            UPDATE="apt-get update -qq && apt-get upgrade -y -qq > /dev/null" ;;
    esac
fi

release() {
    rm -rf "$PACKAGES"
    (cd "$VERO" && scripts/package.sh --app example/vero-app.toml --version "$1" --targets "$TARGETS" --out "$PACKAGES")
    "$REPO" build --app "$VERO/example/vero-app.toml" --packages "$PACKAGES" --key "$KEY" --url "$URL" --out "$SITE"
}
in_container() { docker exec "$NAME" sh -c "$1"; }
cleanup() {
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    [ -n "$SERVER" ] && kill "$SERVER" 2>/dev/null || true
}
trap cleanup EXIT

echo "== releasing 1.0.0"
release 1.0.0
go build -o "$CACHE/bin/serve" "$VERO/scripts/lib/serve.go"
"$CACHE/bin/serve" "$SITE" "$PORT" &
SERVER=$!
docker rm -f "$NAME" >/dev/null 2>&1 || true
# shellcheck disable=SC2086
docker run -d --name "$NAME" $RUN --add-host=host.docker.internal:host-gateway "$IMAGE" sleep infinity >/dev/null

echo "== installing it as the site says, in $IMAGE${FLATPAK:+, with Flatpak}"
in_container "$SETUP"
in_container "$INSTALL"
got=$(in_container "$WORKER")
[ "$got" = 1.0.0 ] || { echo "FAIL: the installed worker says $got, not 1.0.0" >&2; exit 1; }
in_container "$CHECK" >/dev/null ||
    { echo "FAIL: the package lacks its command, menu entry, metadata, binding, signature or GTK" >&2; exit 1; }
echo "ok: 1.0.0 installed from the repository, and its worker answers"

echo "== releasing 1.0.1 into the same site"
release 1.0.1
in_container "$UPDATE"
got=$(in_container "$WORKER")
[ "$got" = 1.0.1 ] || { echo "FAIL: after updating, the worker says $got, not 1.0.1" >&2; exit 1; }
echo "ok: the system's updates brought 1.0.1"
echo "PASS"
