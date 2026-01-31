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

## Setting up a repository

Writ is VCS-agnostic. Use your preferred version control system to manage
repositories, then register them with writ.

### Clone an existing repository

```bash
# Clone with git
git clone git@github.com:me/environment.git ~/environment

# Register with writ
writ config set writ.repos.personal ~/environment
```

### Create a new repository

```bash
# Create directory structure
mkdir -p ~/environment/myproject/Home/.config

# Initialize git (optional)
cd ~/environment
git init

# Register with writ
writ config set writ.repos.personal ~/environment
```

### Register an existing directory

If you already have a configuration directory:

```bash
writ config set writ.repos.personal ~/Workspace/Personal/Configs
```

## List registered repositories

```bash
writ config get writ.repos
```

## Remove a repository registration

Unregister a repository from writ (does not delete files):

```bash
writ config unset writ.repos.team
```

To also clean up deployed files, decommission projects first:

```bash
# Decommission specific projects from the team layer
writ decommission shared-tools backend-config

# Then unregister the repository
writ config unset writ.repos.team
```

## Repository structure

A repository contains one or more projects (subdirectories), plus optional
metadata files:

```
environment/
├── .age-recipients          # Encryption recipients
├── .gitignore
├── noblefactor/             # Project: personal config
│   ├── Home/
│   │   ├── .zshrc
│   │   └── .config/git/config
│   └── packages-manifest.yaml
├── thenobles/               # Project: family-shared config
│   └── Home/
│       └── .config/shared/
└── work/                    # Project: work-specific overrides
    └── Home/
        ├── .config/git/config   # Overrides noblefactor's git config
        └── .npmrc
```

## Multi-layer deployment

Deploy from multiple layers simultaneously:

```bash
writ deploy all
```

Writ scans all registered repositories and deploys projects from each.
When the same file path appears in multiple layers, the highest-precedence
layer wins silently.

To see which layer a file comes from:

```bash
writ inspect ~/.config/git/config
```

## Configuration storage

Repository registrations are stored in `~/.config/devlore/config.d/writ.yaml`:

```yaml
writ:
  repos:
    personal: ~/environment
    team: /path/to/team-repo
```

Edit directly or use:

```bash
writ config set writ.repos.team /path/to/team-repo
writ config get writ.repos
```
