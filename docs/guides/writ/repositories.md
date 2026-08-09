---
title: "Repositories"
description: "Manage layered environment repositories"
tool: "writ"
category: "tutorial"
order: 6
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

## Registering repositories

Registration is `writ repo`. The location is a local working-tree-root, or a
repository URL — which clones first (`git clone`'s own grammar: the optional
trailing working-tree-root is the destination):

```bash
writ repo add personal ~/Workspace/Personal              # register an existing working tree
writ repo add team git@github.com:acme/team-env.git      # clone to the writ-owned home
writ repo add personal git@github.com:me/env.git ~/Workspace/Personal
writ repo add personal git@github.com:me/env.git ~/Workspace/Personal --branch writ-layout
writ repo                                                # list registrations (writ repo list / ls)
writ repo remove team                                    # unregister (writ repo rm team)
```

Without a destination, a URL clones to `XDG_DATA_HOME/devlore/writ/repos/<layer>` —
right for consume-only base and team layers; your personal layer usually names the
working tree you edit. **After placement the repository is entirely yours**: writ
performs no hidden git operations, ever — updating layer content is `git pull`
followed by `writ upgrade`.

A registration is a symlink in the writ layers directory
(`XDG_DATA_HOME/devlore/writ/layers/<layer>`) pointing at the working tree —
packaging, not configuration. Registrations never appear in `config.yaml`,
and `writ repo remove` never deletes repository files.

To also clean up deployed files, decommission projects first:

```bash
writ decommission shared-tools backend-config
writ repo remove team
```

## Setting up a repository

Writ layers are git repositories — deploy plans against pinned git history, so a
layer must be a git working tree (`writ repo add` checks, and refuses anything
else):

```bash
# One step: clone and register
writ repo add personal git@github.com:me/environment.git ~/environment

# Or create a new one
mkdir -p ~/environment/Home/myproject
cd ~/environment && git init
writ repo add personal ~/environment
```

## Repository structure

A repository holds a `Home/` tree (deployed into `$HOME`) and optionally a
`System/` tree (deployed into `/`). Directly under each sits one directory per
**project**, with platform variants as dot-suffixed siblings — see
[Platform awareness](/guides/writ/platform-awareness/) for the segment values:

```
environment/
├── .gitignore
└── Home/
    ├── common/                       # Reserved: deploys implicitly, everywhere
    ├── noblefactor/                  # Project: every platform
    │   ├── .config/git/config
    │   └── packages-manifest.yaml    # Optional: the project's software
    ├── noblefactor.Unix/             # Variant: Darwin and Linux only
    │   └── local/bin/my-script
    ├── thenobles/                    # Project: family-shared config
    │   └── .config/shared/family.conf
    └── thenobles.Darwin/             # Variant: macOS only
        └── local/bin/Backup-TimeCapsule
```

Everything inside a project directory is home-relative: `Home/noblefactor/.config/git/config`
deploys to `~/.config/git/config`. A file named `<name>.template` renders with
segment data and deploys as `<name>`.

## Multi-layer deployment

Deployment always draws from every registered layer simultaneously:

```bash
writ deploy noblefactor
```

Writ scans all registered repositories and deploys the selected projects from
each — plus the reserved `common` project, which is always included implicitly.
When the same file path appears in multiple layers, the highest-precedence
layer wins.

To see what is deployed, where it came from, and whether it has drifted:

```bash
writ status
```
