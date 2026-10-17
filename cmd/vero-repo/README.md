# vero-repo

`vero-repo` is the packaging tool. It makes every installer, package
repository, signature and index that a release needs, and the install
site that holds them. You rarely run it yourself: `vero release` and
`vero ship` run it, and `vero` passes its own subcommands (`key`,
`package`, `build`, `check`, `steps`, `show` and `plan`) on to it.

Everything is written in Go and runs on the Mac. The one exception is a
Flatpak, which needs Docker.

## What a release gives each system's users

| System | What its users get |
|---|---|
| Debian, Ubuntu and Chromebooks | An apt repository. Added once, then updated with the rest of the system. |
| Fedora, openSUSE | An rpm repository. |
| Arch Linux | A pacman repository, and a recipe for the AUR. |
| Alpine, Void and Chimera | An apk or xbps repository, when the system's section is in `vero-app.toml`. |
| Any Linux, with Flatpak | A Flatpak repository and a `.flatpakref`, when asked for. |
| FreeBSD, DragonFly | A pkg repository. |
| NetBSD | A pkgsrc repository for pkgin. Unsigned, because pkgsrc checks no signatures, so HTTPS has to vouch for it. |
| OpenBSD | Signed packages for `pkg_add`. |
| illumos | An IPS repository on OpenIndiana, served as plain files. |
| macOS | A notarised disk image, an optional `.pkg`, a Sparkle appcast and a Homebrew cask. |
| Windows | An installer for x64 and ARM64, a winget manifest, and an MSIX when asked for. |

A private site can carry a password in the address that each system
fetches from, such as `https://NAME:PASSWORD@example.com/myapp/arch/$arch`.

## Processors

By default, each system is packaged for x86-64 and ARM64 where it has
them. Most apps never need to change that. If yours does, `arches` in a
system's section of `vero-app.toml` lists the processors to package for,
which can be fewer than the default or more. It takes either Go's names
(`amd64`, `arm64`, `riscv64`) or the system's own, which are these.

| Architecture | `[gtk.deb]` | `[gtk.rpm]` | `[gtk.arch]` | `[gtk.alpine]` | `[gtk.void]` | `[gtk.chimera]` | `[gtk.flatpak]` |
|---|---|---|---|---|---|---|---|
| x86-64 | amd64 | x86_64 | x86_64 | x86_64 | x86_64 | x86_64 | x86_64 |
| ARM64 | arm64 | aarch64 | aarch64 | aarch64 | aarch64 | aarch64 | aarch64 |
| RISC-V | riscv64 | riscv64 | riscv64 | riscv64 | | riscv64 | |
| POWER | ppc64el | ppc64le | powerpc64le | ppc64le | | ppc64le | |
| 32-bit ARM (ARMv7) | | armv7hl | armv7h | armv7 | armv7l | | |
| 32-bit ARM (ARMv6) | armhf | armv6hl | | armhf | armv6l | | |
| 32-bit ARM (ARMv5) | armel | | | | | | |
| 32-bit x86 | i386 | i686 | pentium4 | x86 | i686 | | |
| LoongArch | loong64 | | loong64 | loongarch64 | | loongarch64 | |
| POWER, big-endian | ppc64 | | powerpc64 | | | ppc64 | |
| IBM Z | s390x | s390x | | s390x | | | |
| MIPS64 | mips64el | | | | | | |
| MIPS | mipsel | | | | | | |

| Architecture | `[gtk.freebsd]` | `[gtk.netbsd]` | `[gtk.openbsd]` |
|---|---|---|---|
| x86-64 | amd64 | x86_64 | amd64 |
| ARM64 | aarch64 | aarch64 | aarch64 |
| 32-bit x86 | i386 | i386 | i386 |
| 32-bit ARM | armv7 | | arm |
| RISC-V | | | riscv64 |

DragonFly and illumos are x86-64 only. The armhf `.deb` is built for ARMv6,
which runs on Debian's armhf and on Raspberry Pi OS's. 32-bit x86 needs a
Pentium 4 or later, and POWER needs a POWER8 or later. These packages have
passed vero's checks but have not been installed on their systems:
Debian's armel, openSUSE's armv7hl and armv6hl, Alpine's armhf, Void's
armv6l, Arch Linux 32's pentium4, Arch POWER's, FreeBSD's armv7, and
Chimera's on anything but ARM64.
