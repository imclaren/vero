# Sign and publish

A release needs no account with anyone. Signing removes the warnings that
the Mac and Windows show for an unsigned app, and publishing puts the app
in the package directories that people already use. Both are optional,
and each is set up separately.

```bash
vero credentials
```

This says what is set up, finds what it can in the keychain, and prints
the commands for the rest. `vero release` reads the environment variables
below.

## Signing

| System | What you need | What to set |
|---|---|---|
| The Mac | The Apple Developer Program, a Developer ID Application certificate, and a Developer ID Installer certificate if you make a `.pkg` | Nothing, if there is one of each in the keychain. Otherwise `VERO_MAC_IDENTITY`, `VERO_MAC_INSTALLER` and `VERO_NOTARY_PROFILE` choose which to use. |
| Windows | A code-signing certificate as a `.pfx`, and `brew install osslsigncode` | `VERO_WINDOWS_CERT` and `VERO_WINDOWS_CERT_PASSWORD` |

Linux and BSD packages are signed with the key that the first release
made. Nothing to set up there.

## Publishing

| Where | What you need | Command |
|---|---|---|
| Homebrew | `gh`, signed in to GitHub | `vero publish --homebrew` |
| winget | `gh`, signed in to GitHub | `vero publish --winget` |
| The AUR | An account at aur.archlinux.org with your SSH key | `vero publish --aur` |
| The Microsoft Store | A Partner Center account | Build an MSIX with `[wpf.msix]` and upload it by hand |
| Flathub | A GitHub account | Open a pull request to flathub/flathub by hand |

`vero publish --all` publishes to Homebrew, winget and the AUR, whichever
of them the site has files for.

Android and iOS signing are described on [the lab](lab.md) page.
