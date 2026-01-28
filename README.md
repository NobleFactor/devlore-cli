# devlore-cli

Monorepo for the **lore** and **writ** command-line tools.

- **lore** — The tribal knowledge package deployer
- **writ** — Portable environment orchestrator

## Building

```bash
make build          # Build both binaries to bin/
make lore           # Build lore only
make writ           # Build writ only
```

## Installing

```bash
make install        # Install binaries to ~/.local/bin (or GOBIN)
make install-all    # Install binaries, completions, and man pages
```

## Self-Install

Use `self-install` to install binaries, man pages, and shell completions:

```bash
# Install to ~/.local (default)
writ self-install ~/.local
lore self-install ~/.local

# Specify shells explicitly (auto-detects by default)
writ self-install ~/.local --shell bash --shell zsh

# Man pages
lore man                          # Display with pager
writ man deploy                   # Display man page for subcommand
```

## XDG Compliance

All paths follow the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html):

| Artifact | Default Path |
|----------|--------------|
| Shared config | `$XDG_CONFIG_HOME/devlore/config.yaml` |
| Tool configs | `$XDG_CONFIG_HOME/devlore/config.d/{writ,lore}.yaml` |
| Registry cache | `$XDG_CACHE_HOME/devlore/registry/` |
| Downloads cache | `$XDG_CACHE_HOME/devlore/downloads/` |
| Writ layers | `$XDG_DATA_HOME/devlore/writ/layers/` |
| Writ receipts | `$XDG_STATE_HOME/devlore/writ/receipts/` |
| Lore receipts | `$XDG_STATE_HOME/devlore/lore/receipts/` |
| Man pages | `<prefix>/share/man/man1/` |
| Bash completions | `<prefix>/share/bash-completion/completions/` |
| Zsh completions | `<prefix>/share/zsh/site-functions/` |
| Fish completions | `<prefix>/share/fish/vendor_completions.d/` |
| PowerShell completions | `<prefix>/share/powershell/completions/` |

## Project Structure

```
devlore-cli/
├── cmd/
│   ├── lore/main.go           # lore entry point
│   └── writ/main.go           # writ entry point
├── internal/
│   ├── cli/                   # Shared CLI infrastructure
│   │   ├── man.go             # man command
│   │   ├── selfinstall.go     # self-install command
│   │   ├── version.go         # version command
│   │   └── xdg.go             # XDG path helpers
│   ├── lore/                  # Lore-specific commands
│   └── writ/                  # Writ-specific commands
├── docs/                      # Generated documentation
├── Makefile
└── go.mod
```

## License

MIT License. See [LICENSE](LICENSE) for details.
