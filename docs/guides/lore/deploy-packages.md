---
title: "Deploy Packages"
description: "Install software using lore's package deployment system"
tool: "lore"
category: "tutorial"
order: 2
---

# Deploy Packages

This guide covers installing software with lore, from simple single-package
installs to manifest-driven deployments with features and receipts.

## Install packages directly

Specify packages by name on the command line:

```bash
lore deploy kubectl gh docker
```

Lore resolves each package from the registry, determines the best installation
method for your platform, and runs the four-phase pipeline.

## Enable features

Packages can offer optional features. Enable them with `--with`:

```bash
lore deploy docker --with rootless --with compose --with buildx
```

Features control which steps run during installation. A package's available
features are listed in its manifest.

## Deploy from manifests

Use `@` to deploy from a manifest file:

```bash
lore deploy @packages.manifest
```

Deploy from multiple manifests:

```bash
lore deploy @base.manifest @team.manifest
```

Mix manifests with direct packages:

```bash
lore deploy @team.manifest neovim --with lsp
```

## Confidence levels

When lore resolves a package, it assigns a confidence level based on how
well the manifest matches your platform:

| Level | Meaning | Behavior |
|-------|---------|----------|
| HIGH | Native package manager, well-tested | Installs automatically |
| MEDIUM | Custom script, good platform match | Installs automatically |
| LOW | Uncertain match or untested platform | Prompts for confirmation |

Control low-confidence behavior:

```bash
# Skip all LOW CONFIDENCE items
lore deploy @manifest --known-only

# Proceed without prompting
lore deploy @manifest --force
```

## Parallel installation

Install multiple packages concurrently:

```bash
lore deploy @manifest --parallel=4
```

Packages with dependencies are still ordered correctly.

## Dry run

Preview what lore would do without making changes:

```bash
lore deploy --dry-run kubectl gh docker
```

## Receipts

Every deployment produces a receipt recording what was installed:

```bash
# Save to specific path
lore deploy @manifest --receipt=~/deployments/workstation.yaml
```

Receipts are stored in `~/.local/state/lore/receipts/` by default.

## Upgrade packages

Upgrade previously deployed packages to newer versions:

```bash
lore upgrade @workstation
```

This re-runs the pipeline for each package, using the latest manifest
from the registry.

## Reconcile state

Compare what should be installed against actual system state:

```bash
lore reconcile @workstation
```

Reports drift: missing binaries, changed versions, or removed configurations.

## Decommission packages

Remove packages and clean up their resources:

```bash
# Remove specific packages
lore decommission docker kubectl

# Remove from receipt
lore decommission @workstation

# Remove only orphaned packages (not referenced by any manifest)
lore decommission --orphans-only
```

## Integration with writ

When writ deploys a project containing `packages.manifest`, it automatically
calls lore:

```bash
writ add noblefactor
# → Symlinks config files
# → Calls: lore deploy @packages.manifest
```

When writ removes a project:

```bash
writ remove --decommission noblefactor
# → Removes symlinks
# → Calls: lore decommission --orphans-only
```

## Inspect packages

View detailed information about a deployed or available package:

```bash
lore inspect docker
lore inspect kubectl --format yaml
```

Shows the resolved manifest, platform support, features, dependencies,
and deployment history.
