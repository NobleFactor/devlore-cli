---
title: "Lore Overview"
description: "Cross-platform package deployment that captures tribal knowledge"
tool: "lore"
category: "overview"
order: 1
---

# Lore Overview

Lore is a cross-platform package deployment tool that captures tribal knowledge
about installing software. It delegates to native package managers when possible
and provides custom deployment scripts when necessary.

## Why lore

"Install this package" is rarely the whole story:

- **Docker** requires adding vendor repositories, removing conflicting packages,
  and installing five separate components
- **Pandoc** needs a PDF engine, which needs LaTeX, which needs tlmgr to
  install packages that aren't documented anywhere
- **kubectl** needs specific versions matched to your cluster, plus plugins,
  plus shell completions that aren't installed by default

Lore captures this knowledge once and shares it forever.
What took someone hours to figure out, you get in minutes.

## Core concepts

### Four-phase pipeline

Every package deployment follows a structured pipeline: **prepare → install → provision → verify**.
This model separates concerns, enables partial re-runs, and makes installation auditable.

See [The Pipeline](/guides/lore/pipeline/) for detailed phase documentation.

### Package manifests

A manifest is a YAML file defining a package's installation lifecycle, including
metadata, platform support, optional features, and phase scripts (Starlark).

See [Create Manifests](/guides/lore/create-manifests/) for the full specification.

### Registry

The DevLore Registry is a community-maintained collection of package manifests.
Lore resolves packages from the registry, selecting the right installation
method for your platform.

### Receipts

Every deployment produces a receipt recording what was installed, which
phases ran, and the resulting state. Receipts enable upgrades, reconciliation,
and decommissioning.

### Features

Packages can declare optional features that are enabled at deploy time:

```bash
lore deploy docker --with rootless --with compose
```

Features control which steps run in each phase, allowing a single manifest
to handle multiple installation variants.

## Guides

- [Deploy packages](/guides/lore/deploy-packages/) — Install software using lore
- [The pipeline](/guides/lore/pipeline/) — Understand the four-phase deployment model
- [Create manifests](/guides/lore/create-manifests/) — Write and publish package manifests
- [Plan bindings reference](/guides/lore/plan-bindings/) — Starlark API for phase scripts
- [Work with the registry](/guides/lore/registry/) — Search, inspect, and contribute packages
