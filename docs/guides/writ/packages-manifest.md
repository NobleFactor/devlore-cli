---
title: "Packages Manifest"
description: "Declare software dependencies for writ projects"
tool: "writ"
category: "reference"
order: 4
---

# Packages Manifest

A `packages-manifest.yaml` (or `.json`) file declares software dependencies for
a writ project. When you run `writ deploy`, writ delegates to lore to install
packages from the manifest.

## Location

Place the manifest in your project directory:

```
my-environment/              # Your environment repo (wherever you keep it)
└── noblefactor/             # Project
    ├── packages-manifest.yaml
    └── Home/                # Target
        ├── .zshrc
        └── .config/
            └── nvim/
                └── init.lua
```

## Format

Each package is either a simple name or a single-key map with options:

```yaml
packages:
  # Simple packages
  - gh
  - jq
  - ripgrep

  # Packages with features
  - neovim:
      with: [lsp, treesitter]
  - docker:
      with: [rootless, compose]
```

### Simple packages

For packages without options, use a plain string:

```yaml
packages:
  - gh
  - jq
  - ripgrep
```

### Packages with features

To enable optional features, use a single-key map where the key is the package
name and the value contains a `with` array:

```yaml
packages:
  - neovim:
      with: [lsp, treesitter]
  - docker:
      with: [rootless, compose, buildx]
```

Features are passed to lore as `--with` flags. Available features for each
package are defined in the lore registry.

## How it works

```
writ deploy noblefactor
  │
  ├── Create symlinks for configuration files
  │
  └── Found packages-manifest.yaml
      └── Delegate to: lore deploy @packages-manifest.yaml
          │
          └── For each package:
              ├── Resolve from registry
              └── Run four-phase pipeline
```

Writ manages configuration (symlinks, templates, secrets). Lore manages software
installation. The manifest bridges the two.

## Package resolution

Package names in the manifest are resolved against the lore registry. The
registry contains full lifecycle manifests with:

- Platform-specific installation methods
- Prepare, install, provision, verify phases
- Package manager selection logic

You don't specify *how* to install packages in the manifest—that knowledge
lives in the registry.

Until the devlore provider lands (devlore-cli#877), a manifest entry that names
a registry package is noted in the deploy's output and not deployed; native
packages deploy as before.

### Package manager preference

On macOS, where both Homebrew and MacPorts are common, set your preference in
the lore configuration (not per-package):

```yaml
# ~/.config/devlore/config.yaml
lore:
  macos:
    package_manager: port  # prefer MacPorts, fall back to Homebrew
```

## Layer merging

Manifests **combine**; only files collide. A file lands at one target path, so
when two layers or two variant directories provide it, the most specific one
wins and the rest are set aside — the overlay rule described under
[Platform Awareness](/guides/writ/platform-awareness/). A manifest lands
nowhere. It is a set of package claims, and every claim from every layer and
every variant directory contributes:

```
base/packages-manifest.yaml      →  foundational packages
  ↓
team/packages-manifest.yaml      →  team additions
  ↓
personal/packages-manifest.yaml  →  personal additions
```

No manifest overrides another. When two claims name the same package, writ
merges them by these rules, and says what it decided as a note in the deploy's
output — a note, because nothing is wrong; you simply cannot see one manifest's
claims from another.

**Each claim goes to one package manager first.** A bare name (`jq`) goes to
your preferred manager for the platform (see *Package manager preference*
above). A prefixed name (`brew:jq`) goes to that manager, and is refused if the
manager is not available. Claims that went to different managers are different
packages: `brew:jq` and `port:jq` are two packages, and a plain `jq` is
whichever of them your preference picked.

**A prefixed claim satisfies a plain one.** `jq` in one manifest and `brew:jq`
in another install brew's jq, once. The plain claim asked for jq and got it; the
prefixed claim asked for brew's and got that.

**Two prefixed claims on two managers both install.** `brew:jq` and `port:jq`
were each written on purpose, so both are installed. Two managers then provide
one command name, and `writ reconcile` reports the pair, since which one you
get depends on your `PATH`.

**Features add up.** A package claimed plainly in one manifest and with a
feature in another is installed with that feature. A later claim adds to an
earlier one; it never replaces it.

**Two versions of one package are settled by the package manager.** If you
claim `jq@1.6` in one manifest and `jq@1.7` in another, writ asks the manager
whether two versions of jq can be installed together here. Where they can, both
are installed. Where they cannot, that is a version conflict: the deploy stops
and names both manifests, rather than choosing one for you. A plain `jq` beside
a `jq@1.7` is not a conflict — the plain claim accepts any version, and the pin
satisfies it. Today writ does not yet ask the manager: two different pins are
refused on every manager, naming both manifests, until the pkg provider's
broker lands (devlore-cli#868).

**Already installed means nothing to do.** Your preference says what to install
when something must be installed. A plain claim already met by any manager on
the machine installs nothing. A prefixed claim is met only by its own manager:
`brew:jq` with port's jq present installs brew's jq beside it. "Installed" means
installed at a version that satisfies the claim; `jq@1.7` with 1.6 present is
not met.

**A deploy never uninstalls.** Nothing here removes a package another manager
provided. That is `writ decommission`'s job, by name.

## Platform variants

Package manifests follow the same segment matching as other project files.
Place platform-specific manifests in variant directories:

```
Home/
├── noblefactor/
│   └── packages-manifest.yaml       # Common packages
├── noblefactor.Darwin/
│   └── packages-manifest.yaml       # macOS-only packages
└── noblefactor.Linux/
    └── packages-manifest.yaml       # Linux-only packages
```

On macOS, packages from both `noblefactor/` and `noblefactor.Darwin/` are
merged. See [Platform Awareness](/guides/writ/platform-awareness/) for details.

## JSON format

The manifest can also be written as JSON:

```json
{
  "packages": [
    "gh",
    "jq",
    "ripgrep",
    {"neovim": {"with": ["lsp", "treesitter"]}},
    {"docker": {"with": ["rootless", "compose"]}}
  ]
}
```

## Schema

The embedded JSON schema is available via:

```bash
writ schema packages-manifest
```

This outputs the schema for editor integration and validation.
