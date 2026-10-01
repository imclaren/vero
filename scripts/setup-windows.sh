#!/bin/sh
# Installs what the Windows example needs, and finds you an ISO.
#
#   scripts/setup-windows.sh
#
# Three things: qemu to run the VM, the .NET SDK to build the WPF
# application, and a Windows 11 ARM64 ISO, which Microsoft will not let a
# script fetch - see below.
set -e
ROOT=$(cd "$(dirname "$0")/.." && pwd)
VM=${VM:-$HOME/vm/vero-windows}

command -v brew >/dev/null 2>&1 || { echo "Homebrew is needed: https://brew.sh" >&2; exit 1; }

for pair in qemu-system-aarch64:qemu dotnet:dotnet; do
    cmd=${pair%%:*}; formula=${pair#*:}
    command -v "$cmd" >/dev/null 2>&1 && continue
    echo "installing $formula"
    brew install "$formula" >/dev/null 2>&1 ||
        { echo "    brew install $formula failed" >&2; exit 1; }
done

mkdir -p "$VM"
# By size, not by name: run-windows.sh keeps its own ISOs here - the build on
# a CD, and the answer file - and both are megabytes.  Windows is gigabytes.
ISO=$(find "$VM" "$HOME/Downloads" -maxdepth 1 -name '*.iso' -size +3G 2>/dev/null | head -1)

if [ -n "$ISO" ]; then
    echo "found $ISO"
    echo
    echo "install Windows into the VM once, with:"
    echo "  scripts/run-windows.sh --iso \"$ISO\" --install"
    exit 0
fi

# No automated download.  Microsoft serves the ISO from a session the page
# itself opens: the link is signed, expires in 24 hours, and is refused
# without the cookies that go with it.  Every "direct link" to one is either
# someone's mirror or a link that has already expired, and neither belongs in
# a build script.
cat <<TXT
No Windows 11 ARM64 ISO found.  Microsoft does not publish a fixed URL for
one, so this is the manual part:

  1. open https://www.microsoft.com/en-us/software-download/windows11arm64
  2. under "Download Windows 11 Disk Image (ISO) for Arm64 devices",
     choose the edition and your language, and download it
  3. move it to $VM/

Then install Windows into the VM once - it takes about 20 minutes, and only
ever happens once:

  scripts/run-windows.sh --iso $VM/<the file>.iso --install

Opening that page now.
TXT
command -v open >/dev/null && open "https://www.microsoft.com/en-us/software-download/windows11arm64"
exit 0
