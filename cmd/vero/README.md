# The vero command

`vero` makes an app, writes its front ends, and builds, tests and releases
it. Run it with no arguments for every command and its flags, and with a
command and `--help` for that command's flags.

```bash
GOBIN=$(brew --prefix)/bin go install github.com/imclaren/vero/cmd/vero@latest
```

`--vero DIR`, or `VERO_DIR` for `vero setup` and `vero ship`, uses a
checkout of vero in place of the published one. [AGENTS.md](../../AGENTS.md)
shows the commands in the order in which an app uses them.
