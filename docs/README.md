# Recipes

Each page here answers one question. Read them in order the first time,
or jump to the one you need.

## Start here

| Page | The question it answers |
|---|---|
| [How vero works](how-vero-works.md) | What are the worker, the front end, the state and a request, and how do they fit together? |
| [Set up a Mac](set-up-a-mac.md) | What do I install before anything works? |
| [Describe the app](describe-the-app.md) | What goes in `vero-app.toml`, and what does each section control? |
| [Bring an Xcode app](bring-an-xcode-app.md) | I already have a Mac app in Xcode. How do I add vero to it? |

## Write the app

| Page | The question it answers |
|---|---|
| [Push state](push-state.md) | How does what the worker knows reach the window? |
| [Add a request](add-a-request.md) | How does the window ask the worker to do something? |
| [Add a screen](add-a-screen.md) | How do I build the same screen in SwiftUI, GTK and WPF? |
| [Secrets and sign-in](secrets-and-sign-in.md) | Where do tokens and passwords go, and how does OAuth work? |
| [Build the worker for every system](build-the-worker-everywhere.md) | The worker will not build for one system. What now? |

## Test

| Page | The question it answers |
|---|---|
| [Write test steps](write-test-steps.md) | How do I script what the test does once the window is open? |
| [Test on Linux and the BSDs](test-on-linux-and-the-bsds.md) | How do I see and test the app on Linux, FreeBSD and the rest, from the Mac? |
| [Test on Windows](test-on-windows.md) | How do I make the Windows virtual machine and test in it? |
| [Record the gallery pictures](record-the-gallery.md) | Where do the GIFs on the install page come from? |

## Ship

| Page | The question it answers |
|---|---|
| [Release](release.md) | How do I make a release, and what does it produce? |
| [Sign and publish](sign-and-publish.md) | How do I remove the "unknown developer" warnings and get into Homebrew, winget and the AUR? |
| [Updates](updates.md) | How do installed copies get the next release? |

## Reference

| Page | What it holds |
|---|---|
| [The kit](../kit/README.md) | The Go packages for secrets, profiles, sleep, autostart, notifications, updates and helper programs. |
| [Packaging for each system](../cmd/vero-repo/README.md) | What each system's users get from a release, and each system's names for its processors. |
| [The vero command](../cmd/vero/README.md) | Every command in one table. |
| [The lab](lab.md) | Android, iOS, the browser, WASI and Plan 9, and every target Go builds. |
| [AGENTS.md](../AGENTS.md) | The rules and the release checklist, as copied into every app for coding agents. |
| [Contributing](../CONTRIBUTING.md) | Running vero's own tests. |
