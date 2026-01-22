# devlore-cli

Monorepo for the **lore** and **writ** command-line tools.

- **lore** — The tribal knowledge package deployer
- **writ** — Dotfiles manager with platform-aware symlinks

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

## On-Demand Generation

Both tools generate shell completions and man pages on demand:

```bash
# Shell completions
lore completion bash              # Output to stdout
lore completion bash --install    # Install to XDG_DATA_HOME

writ completion zsh               # Output to stdout
writ completion zsh --install     # Install to XDG_DATA_HOME

# Man pages
lore man                          # Display with pager
lore man --install                # Install to XDG_DATA_HOME/man/man1

writ man deploy                   # Display man page for subcommand
writ man --install                # Install all man pages
```

## XDG Compliance

All paths follow the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html):

| Artifact | Default Path |
|----------|--------------|
| Config | `$XDG_CONFIG_HOME/lore/` (~/.config/lore/) |
| Data | `$XDG_DATA_HOME/lore/` (~/.local/share/lore/) |
| Cache | `$XDG_CACHE_HOME/lore/` (~/.cache/lore/) |
| State | `$XDG_STATE_HOME/lore/` (~/.local/state/lore/) |
| Man pages | `$XDG_DATA_HOME/man/man1/` |
| Bash completions | `$XDG_DATA_HOME/bash-completion/completions/` |
| Zsh completions | `$XDG_DATA_HOME/zsh/site-functions/` |
| Fish completions | `$XDG_CONFIG_HOME/fish/completions/` |

## Project Structure

```
devlore-cli/
├── cmd/
│   ├── lore/main.go           # lore entry point
│   └── writ/main.go           # writ entry point
├── internal/
│   ├── cli/                   # Shared CLI infrastructure
│   │   ├── completion.go      # completion command
│   │   ├── man.go             # man command
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
