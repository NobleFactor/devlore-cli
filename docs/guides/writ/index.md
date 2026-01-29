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
directory structure. When deployed, each file becomes a symlink:

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

Files with `.tmpl` extension are processed as Go templates during deployment.
The rendered output is copied (not symlinked) to the target:

```
# .gitconfig.tmpl
[user]
    name = {{.UserName}}
    email = {{.UserEmail}}
```

Template variables come from the config file (`writ config set`).

### Secrets

Files ending in `.age` are decrypted during deployment using your SSH key
or age identity. The decrypted content is copied to the target:

```
noblefactor/
└── .config/
    └── github/token.age      # Encrypted at rest, decrypted on deploy
```

### State tracking

Each deployment produces a state file recording what was deployed, with
checksums for drift detection. State files can be optionally signed for
tamper detection.

## Guides

- [Manage environments](/guides/writ/manage-environments/) — Deploy, update, and remove projects
- [Platform awareness](/guides/writ/platform-awareness/) — Configure platform-specific variants
- [Packages manifest](/guides/writ/packages-manifest/) — Declare software dependencies
- [Secrets management](/guides/writ/secrets/) — Encrypt sensitive files with age
- [Repositories](/guides/writ/repositories/) — Manage layered repositories
