# Taking your app to more platforms, and packaging it

vero can add front ends for more platforms to your app: `vero add` writes a
starter for each, wired to your worker, with a checklist of what to change.
`vero release` then builds your app's installers for every system, and a
website of plain files that people install from and get updates from:
signed repositories for Linux and the BSDs, a disk image and Sparkle
updates for macOS, an installer and winget for Windows, and more. A
release needs only your Mac, and no account with anyone; `vero
credentials` is the checklist for signing and the stores, when you want
them.

[Taking your app to more platforms, and packaging it](../PACKAGING.md) explains
both, step by step.
