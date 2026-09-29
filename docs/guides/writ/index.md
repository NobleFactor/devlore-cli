---
title: "Writ Overview"
description: "Environment manager with platform-aware symlinks"
tool: "writ"
category: "overview"
order: 1
---

# Writ Overview

Writ orchestrates your portable environment — configuration, scripts, utilities,
templates, and software manifests. One command deploys your environment.
Platform-aware projects adapt automatically. Templates handle machine-specific values.

## Why writ

Environment management shouldn't require:

- Manual symlink creation across dozens of files
- Platform-specific setup scripts that drift out of sync
- Secret values leaking into git repositories
- Rebuilding everything from memory on a new machine

Writ solves this by letting you declare your environment once and deploy it
everywhere you work.

## Core concepts

### Projects

A project is a directory in your repository whose contents mirror your home
directory structure. When deployed, each plain file becomes a symlink.
Templates, secrets and packages manifests are the exceptions, below:

```
repos/personal/noblefactor/
├── .zshrc                    → ~/.zshrc
├── .config/
│   ├── git/config            → ~/.config/git/config
│   └── nvim/init.lua         → ~/.config/nvim/init.lua
└── .local/bin/
    └── my-script             → ~/.local/bin/my-script
```

### Layered repositories

Writ supports multiple repositories with defined precedence (`personal > team > base`).
When files from different layers target the same path, the higher-precedence layer wins.
This lets organizations provide shared defaults that individuals can override.

See [Repositories](/guides/writ/repositories/) for setup and configuration.

### Platform awareness

Projects can have platform-specific variants using directory suffixes.
Writ detects your OS and deploys matching directories automatically:

```
noblefactor/           # Base project (all platforms)
├── .zshrc
└── .config/nvim/

noblefactor.Darwin/    # macOS-specific additions
├── .zshrc             # Overrides base .zshrc on macOS
└── .config/alacritty/

noblefactor.Linux/     # Linux-specific additions
└── .config/systemd/
```

### Templates

Files ending in `.tmpl` are Go templates. Writ renders each one during
deployment and copies the result to the target without the suffix; it is not
symlinked:

```
# .gitconfig.tmpl, deployed as .gitconfig
[user]
    name = {{ .user_name }}
[core]
    excludesFile = {{ .ConfigHome }}/git/ignore
```

Every template sees `.OS`, `.ARCH`, `.Hostname`, `.Home`, `.Username`, the
segment values under `.Segments` (`.Segments.OS`, `.Segments.DISTRO`, …), and
the XDG homes `.ConfigHome`, `.DataHome`, `.StateHome` and `.CacheHome`. Your
own variables live under `writ.vars` in the configuration file, and a
template names them in lower case: `USER_NAME` there is `.user_name` here.
Like every setting, a variable can also come from the environment or the
command line, the command line winning
([configuration](https://github.com/NobleFactor/devlore-cli/blob/develop/docs/architecture/configuration.md)).
Today writ reads variables from the configuration file only
([#975](https://github.com/NobleFactor/devlore-cli/issues/975)).

A file ending in `.template` is not writ's: other tools use that word for
their own templates, so writ links it untouched.

### Secrets

Files ending in `.sops` are [sops](https://github.com/getsops/sops)-encrypted.
Writ decrypts each one during deployment and copies the plaintext to the
target without the suffix. `.tmpl.sops` is decrypted, then rendered:

```
noblefactor/
└── .config/
    ├── app/secrets.yaml.sops      # encrypted at rest, decrypted on deploy
    └── app/config.yaml.tmpl.sops  # decrypted, then rendered
```

Name the plaintext's format before `.sops`: `.yaml` or `.yml`, `.json`, `.env`
or `.ini`. A secret without one deploys as a JSON wrapper today
([#977](https://github.com/NobleFactor/devlore-cli/issues/977)).

#### Where writ finds your keys

Writ decrypts with the keys sops finds on its own
([encryption provider](https://github.com/NobleFactor/devlore-cli/blob/develop/docs/architecture/3.5.13-encryption-provider.md#key-custody-and-break-glass-recovery)).
The default age key file depends on the platform. This is how sops v3.12.1,
the version writ is built with, resolves it:

| Platform | Default age key file |
| --- | --- |
| Linux | `$XDG_CONFIG_HOME/sops/age/keys.txt`; `~/.config/sops/age/keys.txt` when `XDG_CONFIG_HOME` is unset |
| macOS | `$XDG_CONFIG_HOME/sops/age/keys.txt`; `~/Library/Application Support/sops/age/keys.txt` when `XDG_CONFIG_HOME` is unset |
| Windows | `%AppData%\sops\age\keys.txt`. sops does not read `XDG_CONFIG_HOME` on Windows |

On every platform sops also tries the SSH keys `~/.ssh/id_ed25519` and
`~/.ssh/id_rsa` (on Windows, `~` is `%USERPROFILE%`).

sops tries every key it finds. These environment variables add keys to the
defaults above rather than replacing them:

| Variable | What it holds |
| --- | --- |
| `SOPS_AGE_KEY` | age identities, one per line |
| `SOPS_AGE_KEY_FILE` | the path of an age key file |
| `SOPS_AGE_KEY_CMD` | a command that prints age identities |
| `SOPS_AGE_SSH_PRIVATE_KEY_FILE` | the path of an SSH private key |
| `SOPS_AGE_SSH_PRIVATE_KEY_CMD` | a command that prints an SSH private key |

To keep your age key at the same path on every platform,
`~/.config/sops/age/keys.txt`, where devlore keeps configuration everywhere,
point `SOPS_AGE_KEY_FILE` at it. On Linux with `XDG_CONFIG_HOME` unset, that is
already the default, and setting the variable changes nothing.

```bash
# bash or zsh: add to ~/.bashrc or ~/.zshrc
export SOPS_AGE_KEY_FILE="$HOME/.config/sops/age/keys.txt"
```

```powershell
# PowerShell: add to $PROFILE for the sessions it starts
$env:SOPS_AGE_KEY_FILE = "$HOME\.config\sops\age\keys.txt"

# or set it once for every new process of your user
[Environment]::SetEnvironmentVariable('SOPS_AGE_KEY_FILE', "$HOME\.config\sops\age\keys.txt", 'User')
```

### State tracking

Each deployment produces a state file recording what was deployed, with
checksums for drift detection. State files can be optionally signed for
tamper detection.

## Guides

- [Manage environments](/guides/writ/manage-environments/) — Deploy, update, and remove projects
- [Platform awareness](/guides/writ/platform-awareness/) — Configure platform-specific variants
- [Packages manifest](/guides/writ/packages-manifest/) — Declare software dependencies
- [Repositories](/guides/writ/repositories/) — Manage layered repositories
- [Graphs and traces](/guides/writ/graphs-and-traces/) — Audit runs, detect drift, verify documents
