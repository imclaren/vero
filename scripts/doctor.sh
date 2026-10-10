#!/bin/sh
# Says what this Mac has for building, testing, recording and releasing
# vero apps, and what is missing, with the command that adds each. It
# changes nothing.
#
#   scripts/doctor.sh
missing=0
have() { command -v "$1" >/dev/null 2>&1; }
line() { printf '  %-12s %s\n' "$1" "$2"; }
check() {
    # check NAME COMMAND "what it is for" "how to add it"
    if have "$2"; then line "$1" "yes"; else line "$1" "MISSING: $3. $4"; missing=$((missing + 1)); fi
}

echo "Tools"
check Homebrew brew "installs the rest" "See https://brew.sh."
check Go go "builds every worker" "brew install go"
if xcode-select -p 2>/dev/null | grep -q Xcode.app; then line Xcode yes; else line Xcode "MISSING: the Mac and iOS apps. From the App Store, then: sudo xcode-select -s /Applications/Xcode.app"; missing=$((missing + 1)); fi
check Docker docker "the Linux tests and packages" "scripts/setup-linux.sh"
check colima colima "runs Docker" "scripts/setup-linux.sh"
check qemu qemu-system-aarch64 "the BSD, illumos, Plan 9 and Windows VMs" "brew install qemu"
check .NET dotnet "the Windows app" "brew install dotnet"
check makensis makensis "the Windows installer" "scripts/setup-release.sh"
check ffmpeg ffmpeg "recordings, as GIFs and pictures" "scripts/setup-release.sh"
check wasmtime wasmtime "the WASI example" "scripts/setup-wasm.sh"
check node node "the web example's tests" "scripts/setup-wasm.sh"
check sdkmanager sdkmanager "Android" "scripts/setup-android.sh (about 5GB)"
if have docker && ! docker info >/dev/null 2>&1; then line "" "Docker is not answering: colima start"; fi

echo
echo "VMs, made by each scripts/run-*.sh the first time (the size is the most each can grow to)"
for vm in freebsd:20G:"a few minutes" netbsd:12G:"a few minutes" openbsd:12G:"about 15 minutes" \
    dragonfly:16G:"the best part of an hour" illumos:32G:"about an hour and a half" windows:40G:"about 20 minutes, from an ISO you download" plan9:0G:"a minute"; do
    name=${vm%%:*} rest=${vm#*:} size=${rest%%:*} took=${rest#*:}
    if [ -d "$HOME/vm/vero-$name" ]; then
        line "$name" "made, $(du -sh "$HOME/vm/vero-$name" 2>/dev/null | cut -f1) on disk"
    else
        line "$name" "not made yet: up to $size, $took"
    fi
done
[ -d "$HOME/vm/vero-windows" ] || ls "$HOME"/vm/vero-windows/*.iso >/dev/null 2>&1 || line "" "Windows needs Microsoft's Windows 11 ARM64 ISO in ~/vm/vero-windows/: scripts/setup-windows.sh says where to get it"

echo
free=$(df -g "$HOME" | awk 'NR==2 {print $4}')
echo "Disk: ${free}GB free. Every VM at once can grow to about 130GB, Xcode takes about 15GB and Android about 5GB."

echo
[ $missing -eq 0 ] && echo "Nothing is missing." || echo "$missing missing: scripts/setup.sh adds all but Xcode and the Windows ISO."
