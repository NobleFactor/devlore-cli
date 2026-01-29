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

When multiple layers (base, team, personal) contain package manifests, they
merge with precedence:

```
base/packages-manifest.yaml      →  foundational packages
  ↓
team/packages-manifest.yaml      →  team-specific additions
  ↓
personal/packages-manifest.yaml  →  personal overrides
```

Packages are deduplicated by name. A package in a higher layer (personal)
overrides the same package from a lower layer (base), including its features.

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
