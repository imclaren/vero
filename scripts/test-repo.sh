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
#   scripts/test-repo.sh --image opensuse/tumbleweed  # zypper, the same
#   scripts/test-repo.sh --flatpak                # Flatpak, from the .flatpakref
#   scripts/test-repo.sh --image archlinux        # the AUR recipe, with makepkg
#   scripts/test-repo.sh --vm freebsd             # pkg, in vero's FreeBSD VM
#   scripts/test-repo.sh --vm netbsd              # pkgin, in vero's NetBSD VM
#   scripts/test-repo.sh --vm openbsd             # pkg_add, in vero's OpenBSD VM
#   scripts/test-repo.sh --vm dragonfly           # pkg, in vero's DragonFly VM
#   scripts/test-repo.sh --vm illumos             # pkgin, with pkgsrc, in vero's OpenIndiana VM
#   scripts/test-repo.sh --mac                    # the disk image, on this Mac, and the appcast
#   scripts/test-repo.sh --vm windows             # the installer and the MSIX, in vero's Windows VM, over SSH
#   [--port 8642]
#
# A VM test starts the system's VM with its run script (scripts/run-*.sh
# --shell), which makes it the first time, and stops it at the end; the
# system's own packages - GTK and Python - come from its usual mirrors.
#
# The container tests need Docker and colima. Everything it makes
# is in ~/.cache/vero, the key included, so nothing in it is anyone's. The
# Flatpak test keeps GNOME's runtime, which it downloads from Flathub the
# first time, in a Docker volume called vero-test-flatpak; remove it with
# docker volume rm vero-test-flatpak.
set -e
VERO=$(cd "$(dirname "$0")/.." && pwd)
. "$VERO/scripts/lib/docker.sh"
IMAGE=debian:bookworm PORT=8642 FLATPAK="" VMSYS="" MAC=""
while [ $# -gt 0 ]; do
    case $1 in
        --image) IMAGE=$2; shift ;; --port) PORT=$2; shift ;;
        --flatpak) FLATPAK=yes ;;
        --vm) VMSYS=$2; shift ;;
        --mac) MAC=yes ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
    shift
done
[ -n "$VMSYS$MAC" ] || vero_docker
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
if [ -n "$MAC" ]; then
    # The disk image, on this very Mac: mounted, the app copied out and
    # started, as a person would; then the appcast's next entry checked,
    # which is what the app's Sparkle would fetch.
    URL="http://127.0.0.1:$PORT" TARGETS=macos
elif [ "$VMSYS" = windows ]; then
    # Windows, in vero's VM, over SSH: scripts/lib/windows-test.ps1 and
    # the installers are copied in, and run there a phase at a time. The
    # VM needs SSH set up, which run-windows.sh --install does, or
    # scripts/setup-windows-ssh.sh in a VM installed before.
    URL="http://10.0.2.2:$PORT" TARGETS=windows,msix
elif [ -n "$VMSYS" ]; then
    # A VM reaches this Mac at 10.0.2.2, as qemu's own network has it.
    URL="http://10.0.2.2:$PORT"
    TARGETS=$VMSYS
    VMDIR="$HOME/vm/vero-$VMSYS"
    case $VMSYS in
        freebsd) SSH_PORT=2222 PREFIX=/usr/local PY=python3
            INSTALL="fetch -q -o - $URL/install.sh | sh"
            UPDATE="pkg upgrade -y > /dev/null" ;;
        netbsd) SSH_PORT=2223 PREFIX=/usr/pkg PY=python3.12
            # pkgin, which the install page says to add if it's missing.
            SETUP="[ -x /usr/pkg/bin/pkgin ] || PKG_PATH=https://cdn.netbsd.org/pub/pkgsrc/packages/NetBSD/\$(uname -p)/\$(uname -r | cut -d. -f1-2)/All /usr/sbin/pkg_add pkgin"
            INSTALL="ftp -V -o - $URL/install.sh | sh"
            UPDATE="pkgin -y -f upgrade > /dev/null" ;;
        openbsd) SSH_PORT=2224 PREFIX=/usr/local PY=python3
            # Python crashes importing GTK 4 on OpenBSD 7.9 for ARM, with
            # or without a display, whatever imports it: so here, GTK is
            # checked for, not started.
            GTK="import os; assert os.path.exists('/usr/local/lib/girepository-1.0/Gtk-4.0.typelib')"
            INSTALL="ftp -V -o - $URL/install.sh | sh"
            UPDATE="PKG_PATH=$URL/openbsd/%a/:installpath pkg_add -u vero-example" ;;
        dragonfly) SSH_PORT=2225 PREFIX=/usr/local PY=python3.11
            INSTALL="fetch -q -o - $URL/install.sh | sh"
            # Just the example: an upgrade of everything takes in
            # whatever DragonFly's own repository is changing that day,
            # which has been known to remove packages the example needs.
            UPDATE="pkg upgrade -y vero-example > /dev/null" ;;
        illumos) SSH_PORT=2226 PREFIX=/opt/local PY=/opt/local/bin/python3.12
            # pkgsrc, as pkgsrc.smartos.org says to add it to OpenIndiana.
            SETUP="[ -x /opt/local/bin/pkgin ] || { cd /tmp && curl -fsSLO https://pkgsrc.smartos.org/packages/SmartOS/bootstrap/bootstrap-trunk-x86_64-20260811.tar.gz &&
                [ \$(/bin/digest -a sha1 bootstrap-trunk-x86_64-20260811.tar.gz) = e5e620ade4b45695aa385aea25227e94f49f078f ] &&
                gtar -zxpf bootstrap-trunk-x86_64-20260811.tar.gz -C / && /opt/local/bin/pkgin -y update > /dev/null; }"
            INSTALL="PATH=/opt/local/bin:/opt/local/sbin:\$PATH; curl -fsSL $URL/install.sh | sh"
            UPDATE="/opt/local/bin/pkgin -y -f upgrade > /dev/null" ;;
        *) echo "unknown VM: $VMSYS (freebsd, dragonfly, netbsd, openbsd or illumos)" >&2; exit 2 ;;
    esac
    : "${SETUP:=true}"
    WORKER="$PREFIX/lib/vero-example/worker -version"
    CHECK="PATH=$PREFIX/bin:\$PATH; command -v vero-example && ls $PREFIX/share/applications/dev.vero.example.desktop $PREFIX/lib/vero-example/vero.py && $PY -c \"$GTK\""
elif [ -n "$FLATPAK" ]; then
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
        opensuse*)
            TARGETS=rpm
            SETUP="zypper -n -q install curl gzip > /dev/null"
            CHECK="$CHECK && rpm -qi vero-example | grep -A1 '^Signature' | grep -q RSA"
            UPDATE="zypper -n -q refresh > /dev/null && zypper -n -q update > /dev/null" ;;
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

# release VERSION [BUILD [NOTES]]: the Mac's build number, when it isn't the
# version, and what's new, for the update prompt.
release() {
    rm -rf "$PACKAGES"
    (cd "$VERO" && scripts/package.sh --app example/vero-app.toml --version "$1" ${2:+--build "$2"} --targets "$TARGETS" --out "$PACKAGES")
    "$REPO" build --app "$VERO/example/vero-app.toml" --packages "$PACKAGES" --key "$KEY" --url "$URL" --out "$SITE" ${3:+--notes "$3"}
}
if [ -n "$MAC" ] || [ "$VMSYS" = windows ]; then
    in_container() { sh -c "$1"; }
elif [ -n "$VMSYS" ]; then
    SSH="ssh -i $VMDIR/key -p $SSH_PORT -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o IdentitiesOnly=yes root@127.0.0.1"
    in_container() { $SSH "$1"; }
else
    in_container() { docker exec "$NAME" sh -c "$1"; }
fi
cleanup() {
    if [ -n "$MAC" ]; then
        pkill -f "$CACHE/mac-test/" 2>/dev/null || true
        hdiutil detach -quiet "$CACHE/mac-test/mnt" 2>/dev/null || true
        rm -rf "$CACHE/mac-test" "$HOME/Library/Application Support/Example"
    elif [ "$VMSYS" = windows ]; then
        [ -n "$QEMU" ] && windows_stop
    elif [ -n "$VMSYS" ]; then
        if [ -f "$VMDIR/qemu.pid" ]; then
            $SSH 'PATH=/sbin:/usr/sbin:$PATH; poweroff 2>/dev/null || shutdown -p now' >/dev/null 2>&1 || true
            sleep 15
            kill "$(cat "$VMDIR/qemu.pid")" 2>/dev/null || true
            rm -f "$VMDIR/qemu.pid"
        fi
    else
        docker rm -f "$NAME" >/dev/null 2>&1 || true
    fi
    [ -n "$SERVER" ] && kill "$SERVER" 2>/dev/null || true
}
trap cleanup EXIT

# mac_test: the disk image on this Mac, then the appcast after an update.
mac_test() {
    T="$CACHE/mac-test"
    rm -rf "$T" && mkdir -p "$T/mnt"
    DMG=$(ls "$SITE"/macos/vero-example-1.0.0-macos.dmg)
    echo "== opening $DMG on this Mac"
    hdiutil attach -quiet -nobrowse -mountpoint "$T/mnt" "$DMG"
    APP=$(ls -d "$T/mnt"/*.app)
    ditto "$APP" "$T/$(basename "$APP")"
    hdiutil detach -quiet "$T/mnt"
    APP="$T/$(basename "$APP")"
    codesign --verify --deep --strict "$APP" || { echo "FAIL: the app's signature doesn't verify" >&2; exit 1; }
    short=$(plutil -extract CFBundleShortVersionString raw "$APP/Contents/Info.plist")
    build=$(plutil -extract CFBundleVersion raw "$APP/Contents/Info.plist")
    [ "$short" = 1.0.0 ] && [ "$build" = 100 ] || { echo "FAIL: the app says $short ($build), not 1.0.0 (100)" >&2; exit 1; }
    # The installer package: it installs into /Applications, not over a
    # copy elsewhere, and hands the app to whoever installed it.
    PKG="$SITE/macos/vero-example-1.0.0-macos.pkg"
    [ -f "$PKG" ] || { echo "FAIL: no .pkg on the site" >&2; exit 1; }
    curl -fsSL "$URL/latest.json" | grep -q '"macos-pkg"' || { echo "FAIL: latest.json has no macos-pkg" >&2; exit 1; }
    pkgutil --expand "$PKG" "$T/pkg"
    INFO=$(cat "$T"/pkg/*.pkg/PackageInfo)
    case $INFO in *"<relocate>"*) case $INFO in *"<relocate/>"*) ;; *) echo "FAIL: the .pkg relocates the app" >&2; exit 1 ;; esac ;; esac
    sh -n "$T"/pkg/*.pkg/Scripts/postinstall && grep -q 'chown -R' "$T"/pkg/*.pkg/Scripts/postinstall || { echo "FAIL: the .pkg has no postinstall" >&2; exit 1; }
    echo "ok: the .pkg installs into /Applications and gives the app to its user"
    got=$("$APP/Contents/Resources/worker" -version)
    [ "$got" = 1.0.0 ] || { echo "FAIL: the bundled worker says $got, not 1.0.0" >&2; exit 1; }
    open "$APP"
    sleep 6
    pgrep -f "$APP/Contents/MacOS/" >/dev/null || { echo "FAIL: the app didn't stay running" >&2; exit 1; }
    pgrep -f "Application Support/Example/bin/worker" >/dev/null || { echo "FAIL: the app didn't start its worker" >&2; exit 1; }
    pkill -f "$APP/Contents/MacOS/"
    echo "ok: 1.0.0 installed from the disk image, and the app started its worker"
    echo "== releasing 1.0.1 into the same site"
    release 1.0.1 101 "Faster.

- One thing"
    # What Sparkle would fetch: the newest item, its signature checked
    # with the key the app would carry.
    "$REPO" check --app "$VERO/example/vero-app.toml" --url "$URL" --key "$KEY" "$SITE" >/dev/null
    newest=$(curl -fsSL "$URL/macos/appcast.xml" | grep -o '<sparkle:version>[^<]*' | head -1 | cut -d'>' -f2)
    [ "$newest" = 101 ] || { echo "FAIL: the appcast's newest build is $newest, not 101" >&2; exit 1; }
    curl -fsSL "$URL/macos/vero-example-1.0.1-notes.html" | grep -q '<li>One thing</li>' || { echo "FAIL: no release notes for 1.0.1" >&2; exit 1; }
    curl -fsSL "$URL/macos/appcast.xml" | grep -q 'releaseNotesLink>[^<]*vero-example-1.0.1-notes.html' || { echo "FAIL: the appcast doesn't link the notes" >&2; exit 1; }
    curl -fsSL "$URL/homebrew/vero-example.rb" | grep -q 'version "1.0.1"' || { echo "FAIL: the cask isn't 1.0.1" >&2; exit 1; }
    echo "ok: the appcast offers 1.0.1, signed, with its notes, and the cask has it"
    # An app with Sparkle.framework inside, signed as vero signs one:
    # nested code first, and Gatekeeper's strict check passes.
    SPARKLE_VERSION=2.6.4
    SPARKLE="$CACHE/sparkle-$SPARKLE_VERSION"
    if [ ! -d "$SPARKLE/Sparkle.framework" ]; then
        mkdir -p "$SPARKLE"
        curl -fsSL "https://github.com/sparkle-project/Sparkle/releases/download/$SPARKLE_VERSION/Sparkle-$SPARKLE_VERSION.tar.xz" | tar -xJf - -C "$SPARKLE"
    fi
    (cd "$VERO/cmd/vero-repo" && VERO_SPARKLE_FRAMEWORK="$SPARKLE/Sparkle.framework" go test -count=1 -run TestSignWithSparkle . >/dev/null) \
        || { echo "FAIL: an app with Sparkle.framework doesn't sign" >&2; exit 1; }
    echo "ok: an app with Sparkle.framework inside signs and verifies"
    echo "PASS"
}

# windows_test: the installers in vero's Windows VM, over SSH: the NSIS
# installer, installed, started on the desktop, updated to 1.0.1 and
# uninstalled; then the MSIX's files, registered, started and removed.
windows_test() {
    . "$VERO/scripts/lib/windows-disc.sh"
    VMDIR="$HOME/vm/vero-windows"
    SSHPORT=2222
    [ -f "$VMDIR/disk.qcow2" ] || { echo "no Windows VM in $VMDIR: scripts/run-windows.sh --install makes one" >&2; exit 1; }
    [ -f "$WINDOWS_KEY" ] || { echo "no $WINDOWS_KEY: scripts/setup-windows-ssh.sh sets up SSH into the VM" >&2; exit 1; }
    windows_running && { echo "the Windows VM is running already: stop it first" >&2; exit 1; }
    PAY="$CACHE/windows-test"
    rm -rf "$PAY" && mkdir -p "$PAY"
    cp "$PACKAGES"/*-setup.exe "$PACKAGES"/*.msix "$VERO/scripts/lib/windows-test.ps1" "$PAY/"
    echo "== booting the Windows VM (about a minute)"
    "$VERO/scripts/run-windows.sh" --headless --payload "$PAY" --ssh "$SSHPORT" >"$CACHE/windows-vm.log" 2>&1 &
    QEMU=$!
    windows_wait "$SSHPORT" 300 || { echo "FAIL: the VM never answered over SSH: scripts/setup-windows-ssh.sh sets it up" >&2; exit 1; }
    windows_ssh "$SSHPORT" "New-Item -ItemType Directory -Force C:/vero-test | Out-Null; Remove-Item -Recurse -Force C:/vero-test/*"
    windows_scp "$SSHPORT" "$PAY"/* vero@127.0.0.1:C:/vero-test/
    # phase NAME: one of windows-test.ps1's phases, its lines printed.
    phase() {
        windows_ssh "$SSHPORT" "powershell -NoProfile -ExecutionPolicy Bypass -File C:/vero-test/windows-test.ps1 -Phase $1 -Name vero-example -Exe VeroExample.exe -Worker worker.exe -Identity ExamplePublisher.VeroExample" | tr -d '\r' | tee -a "$PAY/results.txt"
    }
    # expect LINE: windows-test.ps1 printed that line.
    expect() { grep -qx "$1" "$PAY/results.txt" || { echo "FAIL: no \"$1\" from the VM" >&2; exit 1; }; }
    echo "== the NSIS installer"
    phase nsis
    for line in "nsis-worker: 1.0.0" "nsis-app: True" "nsis-startmenu: True" "nsis-uninstaller: True" "nsis-runs: True"; do expect "$line"; done
    python3 "$VERO/scripts/lib/qmp.py" /tmp/vero-qmp.sock shot "$PAY/nsis.ppm" && sips -s format png "$PAY/nsis.ppm" --out "$PAY/nsis.png" >/dev/null
    echo "ok: 1.0.0 installed and running (the desktop: $PAY/nsis.png)"
    echo "== releasing 1.0.1, and installing it over 1.0.0"
    release 1.0.1
    cp "$PACKAGES"/*-1.0.1-*-setup.exe "$PAY/"
    windows_scp "$SSHPORT" "$PAY"/*-1.0.1-*-setup.exe vero@127.0.0.1:C:/vero-test/
    phase nsis-update
    expect "nsis-updated: 1.0.1"
    phase nsis-done
    expect "nsis-uninstalled: True"; expect "nsis-uninstall-entry-gone: True"
    echo "ok: 1.0.1 installed over it, and uninstalled cleanly"
    echo "== the MSIX"
    phase msix
    for line in "msix-installed: True" "msix-worker: 1.0.0" "msix-runs: True"; do expect "$line"; done
    phase msix-done
    expect "msix-removed: True"
    echo "ok: the MSIX's app ran, and was removed"
    echo "PASS"
}

echo "== releasing 1.0.0"
if [ -n "$MAC" ]; then release 1.0.0 100; else release 1.0.0; fi
go build -o "$CACHE/bin/serve" "$VERO/scripts/lib/serve.go"
"$CACHE/bin/serve" "$SITE" "$PORT" &
SERVER=$!

if [ -n "$MAC" ]; then
    mac_test
    exit 0
fi
if [ "$VMSYS" = windows ]; then
    windows_test
    exit 0
fi
if [ -n "$VMSYS" ]; then
    "$VERO/scripts/run-$VMSYS.sh" --shell --no-open >/dev/null
    IMAGE="the $VMSYS VM"
else
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    # shellcheck disable=SC2086
    docker run -d --name "$NAME" $RUN --add-host=host.docker.internal:host-gateway "$IMAGE" sleep infinity >/dev/null
fi

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
