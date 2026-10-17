# Sign and publish

A release needs no account with anyone. `vero credentials` says what is set
up, finds what it can in the keychain, and prints the commands for the
rest. The next `vero release` reads these environment variables.

| For | What it takes | Set |
|---|---|---|
| The Mac, without Open Anyway | The Apple Developer Program, a Developer ID Application certificate, and a Developer ID Installer certificate for a `.pkg` | Nothing, or `VERO_MAC_IDENTITY`, `VERO_MAC_INSTALLER` and `VERO_NOTARY_PROFILE` to choose others |
| Windows, without SmartScreen | A code-signing `.pfx`, and `brew install osslsigncode` | `VERO_WINDOWS_CERT`, `VERO_WINDOWS_CERT_PASSWORD` |
| Homebrew, winget | `gh`, signed in | `vero publish --homebrew`, `vero publish --winget` |
| The AUR | An account at aur.archlinux.org with your SSH key | `vero publish --aur` |
| The Microsoft Store, Flathub | An account with each | Upload the `.msix` built by `[wpf.msix]`, or open a pull request to flathub/flathub |

`vero publish --all` publishes to every one of Homebrew, winget and the
AUR for which the site has files. Android and iOS signing are on the
[lab](lab.md) page.
