# Sourced by run-windows.sh and setup-windows-ssh.sh: makes the folder that
# goes on a disc for vero's Windows VM, so that windows-ssh.ps1 can set up
# SSH there with no internet:
#
#   windows_ssh_disc DIR VERO   puts in DIR, from vero's folder VERO:
#                           drivers\ (the virtio drivers for
#                           Windows on ARM), OpenSSH-ARM64.zip,
#                           vero-windows.pub and setup-ssh.ps1
#
# The downloads are pinned and checked, and kept in ~/.cache/vero/windows.
# The key is ~/.ssh/vero-windows, made the first time; it lets this Mac in,
# and nothing else does.

# Red Hat's virtio drivers for Windows, from the Fedora project: the
# network card's among them. Windows on ARM has none for qemu's devices.
VIRTIO_VERSION=0.1.302
VIRTIO_URL="https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/archive-virtio/virtio-win-$VIRTIO_VERSION-1/virtio-win-$VIRTIO_VERSION.iso"
VIRTIO_SHA256=303f7ae40dad495d6ae474fdc571df58958a4dbc5c37a522d80f9a203867949d
# Microsoft's OpenSSH for Windows, for ARM64: Windows' own OpenSSH comes
# from Windows Update, which the VM can't reach until it has a network.
OPENSSH_VERSION=10.0.0.0p2-Preview
OPENSSH_URL="https://github.com/PowerShell/Win32-OpenSSH/releases/download/$OPENSSH_VERSION/OpenSSH-ARM64.zip"
OPENSSH_SHA256=698c6aec31c1dd0fb996206e8741f4531a97355686b5431ef347d531b07fcd42
WINDOWS_KEY="$HOME/.ssh/vero-windows"

# fetch_checked URL SHA256 FILE: downloads URL to FILE once, and checks it.
fetch_checked() {
    if [ ! -f "$3" ] || [ "$(shasum -a 256 "$3" | cut -d' ' -f1)" != "$2" ]; then
        echo "downloading $(basename "$3")"
        curl -fsSL -o "$3.part" "$1"
        got=$(shasum -a 256 "$3.part" | cut -d' ' -f1)
        [ "$got" = "$2" ] || { rm -f "$3.part"; echo "$(basename "$3"): checksum $got, not $2" >&2; return 1; }
        mv "$3.part" "$3"
    fi
}

windows_ssh_disc() {
    out=$1 vero=$2
    cache="$HOME/.cache/vero/windows"
    mkdir -p "$cache" "$out"
    fetch_checked "$VIRTIO_URL" "$VIRTIO_SHA256" "$cache/virtio-win-$VIRTIO_VERSION.iso"
    fetch_checked "$OPENSSH_URL" "$OPENSSH_SHA256" "$cache/OpenSSH-ARM64-$OPENSSH_VERSION.zip"
    # Only the drivers for Windows 11 on ARM64: a tenth of the ISO.
    if [ ! -d "$cache/drivers-$VIRTIO_VERSION" ]; then
        mnt=$(mktemp -d)
        hdiutil attach -quiet -nobrowse -readonly -mountpoint "$mnt" "$cache/virtio-win-$VIRTIO_VERSION.iso"
        mkdir -p "$cache/drivers-$VIRTIO_VERSION.part"
        for d in "$mnt"/*/w11/ARM64; do
            cp -R "$d" "$cache/drivers-$VIRTIO_VERSION.part/$(basename "$(dirname "$(dirname "$d")")")"
        done
        hdiutil detach -quiet "$mnt"
        # Read-only, as the ISO had them: writable, so they can be removed.
        chmod -R u+w "$cache/drivers-$VIRTIO_VERSION.part"
        mv "$cache/drivers-$VIRTIO_VERSION.part" "$cache/drivers-$VIRTIO_VERSION"
    fi
    [ -f "$WINDOWS_KEY" ] || ssh-keygen -q -t ed25519 -N "" -C "vero's Windows VM" -f "$WINDOWS_KEY"
    chmod -R u+w "$cache/drivers-$VIRTIO_VERSION"
    [ -d "$out/drivers" ] && chmod -R u+w "$out/drivers"
    rm -rf "$out/drivers"
    cp -R "$cache/drivers-$VIRTIO_VERSION" "$out/drivers"
    cp "$cache/OpenSSH-ARM64-$OPENSSH_VERSION.zip" "$out/OpenSSH-ARM64.zip"
    cp "$WINDOWS_KEY.pub" "$out/vero-windows.pub"
    cp "$vero/scripts/lib/windows-ssh.ps1" "$out/setup-ssh.ps1"
}

# windows_ssh PORT COMMAND: runs a PowerShell command in the VM over SSH.
windows_ssh() {
    port=$1; shift
    ssh -i "$WINDOWS_KEY" -p "$port" -o BatchMode=yes -o ConnectTimeout=8 \
        -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="$HOME/.ssh/vero-windows-known" \
        -o LogLevel=ERROR vero@127.0.0.1 "$@"
}

# windows_scp PORT FILE... DEST: copies files into the VM.
windows_scp() {
    port=$1; shift
    scp -q -i "$WINDOWS_KEY" -P "$port" -o BatchMode=yes -o StrictHostKeyChecking=accept-new \
        -o UserKnownHostsFile="$HOME/.ssh/vero-windows-known" "$@"
}

# windows_wait PORT SECONDS: waits until the VM answers over SSH.
windows_wait() {
    i=0
    while [ "$i" -lt "$2" ]; do
        windows_ssh "$1" "hostname" >/dev/null 2>&1 && return 0
        sleep 5; i=$((i + 5))
    done
    return 1
}

# windows_running: whether vero's Windows VM is running: its disk is open,
# whatever other VMs are.
windows_running() { pgrep -qf "${VM:-$HOME/vm/vero-windows}/disk.qcow2"; }

# windows_stop: shuts the VM down as Windows would, through qemu's monitor,
# and waits until qemu has gone.
windows_stop() {
    [ -S /tmp/vero-qmp.sock ] || return 0
    python3 - /tmp/vero-qmp.sock <<'PY' || true
import json, socket, sys
s = socket.socket(socket.AF_UNIX); s.connect(sys.argv[1]); f = s.makefile("rw"); f.readline()
for c in ("qmp_capabilities", "system_powerdown"):
    f.write(json.dumps({"execute": c}) + "\n"); f.flush(); f.readline()
PY
    i=0
    while windows_running && [ $i -lt 90 ]; do sleep 2; i=$((i + 1)); done
}
