# vero-repo

`vero-repo` is the packaging tool. It makes every installer, package
repository, signature and index that a release needs, and the install
site that holds them. You rarely run it yourself: `vero release` and
`vero ship` run it, and `vero` passes its own subcommands (`key`,
`package`, `build`, `check`, `steps`, `show` and `plan`) on to it.
Everything is written in Go and runs on the Mac, except a Flatpak, which
needs Docker.

[Ship the app](../../docs/ship-the-app.md) lists what a release gives
each system's users. This page holds the one table that is only needed
when an app narrows or widens the processors it is built for.

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
