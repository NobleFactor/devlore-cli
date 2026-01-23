---
title: "Repositories"
description: "Manage layered environment repositories"
tool: "writ"
category: "tutorial"
order: 5
---

# Repositories

Writ organizes your environment files into layered repositories. Each layer
has a defined precedence, letting organizations provide shared defaults
that individuals can override.

## Layer precedence

```
personal > team > base
```

When files from different layers target the same path, the higher-precedence
layer wins:

| Layer | Purpose | Example |
|-------|---------|---------|
| `base` | Organization-wide defaults | Company security policies, shared tooling |
| `team` | Team-specific config | Backend team's database tools, frontend linting |
| `personal` | Individual preferences | Editor config, shell aliases, custom scripts |

## Initialize a repository

Create a new empty repository:

```bash
# Personal layer (default location: ~/.local/share/devlore/repos/personal/)
writ repo init --layer=personal

# Team layer
writ repo init --layer=team
```

Clone an existing repository:

```bash
writ repo init --layer=personal git@github.com:me/dotfiles.git
writ repo init --layer=team git@github.com:company/team-configs.git
```

For advanced git options, clone manually then register:

```bash
git clone --branch main --depth 1 git@github.com:co/repo.git ~/repo
writ repo add --layer=team ~/repo
```

## Register an existing repository

If you already have a dotfiles directory, register it without cloning:

```bash
writ repo add --layer=personal ~/Workspace/Personal/Home/Configs
```

## List repositories

```bash
writ repo list
```

```
Layer      Path                                          Status
personal   ~/.local/share/devlore/repos/personal/       ok
team       ~/.local/share/devlore/repos/team/           ok
```

## Remove a repository

Unregister a repository from writ (does not delete files):

```bash
writ repo remove --layer=team
```

To also clean up deployed files, remove projects first:

```bash
writ remove all --layer=team
writ repo remove --layer=team
```

## Repository structure

A repository contains one or more projects (subdirectories), plus optional
metadata files:

```
repos/personal/
├── .age-recipients          # Encryption recipients
├── .gitignore
├── noblefactor/             # Project: personal config
│   ├── .zshrc
│   ├── .config/git/config
│   └── packages.manifest
├── thenobles/               # Project: family-shared config
│   └── .config/shared/
└── work/                    # Project: work-specific overrides
    ├── .config/git/config   # Overrides noblefactor's git config
    └── .npmrc
```

## Multi-layer deployment

Deploy from multiple layers simultaneously:

```bash
writ add all
```

Writ scans all registered repositories and deploys projects from each.
When the same file path appears in multiple layers, the highest-precedence
layer wins silently.

To see which layer a file comes from:

```bash
writ inspect ~/.config/git/config
```

## Configuration storage

Repository registrations are stored in `~/.config/devlore/config.yaml`:

```yaml
writ:
  repos:
    personal: ~/.local/share/devlore/repos/personal
    team: ~/.local/share/devlore/repos/team
  target: Home
```

Edit directly or use:

```bash
writ config set writ.repos.team /path/to/team-repo
writ config get writ.repos
```
