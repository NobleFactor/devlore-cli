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

Registration is `writ repo set`. A layer has exactly one registration, so setting a
layer that is already registered re-points it — the vocabulary `config set` and
`config unset` already use for a keyed value. The location is a local
working-tree-root, or a repository URL — which clones first (`git clone`'s own
grammar: the optional trailing working-tree-root is the destination):

```bash
writ repo set personal ~/Workspace/Personal              # register an existing working tree
writ repo set team git@github.com:acme/team-env.git      # clone to the writ-owned home, repos/team-env
writ repo set personal git@github.com:me/env.git ~/Workspace/Personal
writ repo set personal git@github.com:me/env.git ~/Workspace/Personal --branch writ-layout
writ repo set personal ~/env                             # re-point: "personal: was …, now ~/env"
writ repo list                                           # every layer, with its source and owner
writ repo unset team                                     # unregister; safe to run twice
```

Setting a layer to the root it already has says `unchanged` and exits 0. Unsetting
a layer that is not registered exits 0 too: it is the state you asked for.

Without a destination, a URL clones into the writ-owned home,
`XDG_DATA_HOME/devlore/writ/repos/`, under the name `git clone` would give it: the
URL's last path component with a trailing `/` and a `.git` suffix stripped, so
`git@github.com:acme/team-env.git` lands at `repos/team-env`. The directory says
which repository it holds — `base`, `team` and `personal` are the layers, not the
repositories — and `writ repo list` reads layer, repository and path in one line.
Two layers whose repositories share a name would clone to one directory: git
refuses to clone into an existing one, and writ refuses first, naming both layers,
before anything is cloned. The writ-owned home is right for consume-only base and
team layers; your personal layer usually names the working tree you edit. **After
placement the repository is entirely yours**: writ performs no hidden git
operations, ever — updating layer content is `git pull` followed by `writ upgrade`.

A registration is a symlink in the writ layers directory
(`XDG_DATA_HOME/devlore/writ/layers/<layer>`) pointing at the working tree —
packaging, not configuration. Registrations never appear in `config.yaml`.

**Which trees writ removes, and which it never touches.** A clone writ made in its
own home — `XDG_DATA_HOME/devlore/writ/repos/<repository>`, where a URL without a
destination lands — is writ's, and goes with its registration: `writ repo unset`
removes it, and so does a `writ repo set` that re-points the layer elsewhere. A
working tree you registered by path, anywhere else on disk, is yours; unsetting it
removes the registration alone. `writ repo list` reports which is which in its
`owner` column, and `--dry-run` on either verb says what would go without going
near it.

To also clean up deployed files, decommission projects first:

```bash
writ decommission shared-tools backend-config
writ repo unset team
```

## Setting up a repository

Writ layers are git repositories — deploy plans against pinned git history, so a
layer must be a git working tree (`writ repo set` checks, and refuses anything
else):

```bash
# One step: clone and register
writ repo set personal git@github.com:me/environment.git ~/environment

# Or create a new one
mkdir -p ~/environment/Home/myproject
cd ~/environment && git init
writ repo set personal ~/environment
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
each — plus the implicit set: the reserved `common` project and, for every
registered layer, a project named for its repository (`noblefactor-ops`,
`devlore-cli`, `personal` in the layout above), which may live in any layer's
`Home` and deploys with its platform variants like any other. A bare
`writ deploy` deploys the implicit set and whatever the current deployment
already holds; naming a project adds it.
When the same file path appears in multiple layers, the highest-precedence
layer wins.

To see what is deployed, where it came from, and whether it has drifted:

```bash
writ reconcile
```
