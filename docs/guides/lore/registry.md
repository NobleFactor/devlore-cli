---
title: "The Registry"
description: "Search, inspect, and contribute packages to the DevLore Registry"
tool: "lore"
category: "concept"
order: 5
---

# The Registry

The DevLore Registry is a community-maintained collection of package manifests.
It captures tribal knowledge about installing software — the repositories to add,
the dependencies to resolve, the configuration steps that upstream docs don't mention.

## How it works

When you run `lore deploy kubectl`, lore:

1. Checks the local registry cache for a `kubectl` manifest
2. Resolves the best installation method for your platform
3. Runs the four-phase pipeline defined in the manifest

The registry is a git repository. Your local cache is synchronized with
`lore update`.

## Search for packages

```bash
lore search kubectl
lore search --platform darwin docker
```

Search returns matching packages with their descriptions and platform support.

## Inspect a package

View full details before installing:

```bash
lore inspect docker
lore inspect kubectl --format yaml
```

Shows:

- Package name, version, description
- Supported platforms
- Available features
- Dependencies
- Phase scripts
- Deployment history (if previously installed)

## Resolve installation method

See how a package would be installed on your system without actually
installing it:

```bash
lore resolve docker
```

```
docker (v24.0) on darwin/arm64:
  prepare: Add Docker repository
  install: brew install --cask docker
  provision: -
  verify: docker --version
```

## Update the registry cache

Synchronize your local cache with the central registry:

```bash
lore update
```

This fetches the latest manifests, new packages, and updated installation
methods.

## Contribute a package

The registry grows through community contributions. To add a package:

1. Create and test the manifest locally:

```bash
lore manifest create mypackage
# ... write phase scripts ...
lore manifest validate mypackage
lore manifest test mypackage
```

2. Publish to the registry:

```bash
lore publish mypackage
```

3. The automated CI runs your manifest on macOS, Linux, and Windows
4. Community reviewers check the manifest
5. Once merged, the package is available to all users

## Security

### Audit log

Every package fetch and installation is recorded in the audit log:

```bash
lore audit
lore audit --since 7d --package docker
```

Events logged:

| Event | Description |
|-------|-------------|
| `pmm.fetch` | Package fetched with signature status |
| `pmm.verify` | Signature verification result |
| `privilege.request` | Sudo/elevation request |
| `binary.download` | Upstream binary download with hash |
| `phase.execute` | Pipeline phase execution |

### Signature verification

Manifests in the registry are signed. Lore verifies signatures before
execution and logs verification results to the audit log.

### Privilege escalation

When a phase script requires `sudo`, lore records the escalation request
in the audit log with the exact command being elevated.

## Air-gapped environments

For systems without internet access, create self-extracting bundles:

```bash
lore bundle @manifest -o workstation-bundle.sh --platform linux/fedora
```

The bundle includes all manifests, phase scripts, and downloaded artifacts
needed for offline deployment.

Transfer the bundle to the air-gapped system and run it:

```bash
./workstation-bundle.sh
```

## Onboarding from documentation

Extract package requirements from existing wiki pages or scripts:

```bash
lore onboard --from https://wiki.acme.com/backend-setup
```

Lore uses AI to:

1. Parse installation steps from the document
2. Match them to known registry packages
3. Flag organization-specific items for review
4. Generate a `packages-manifest.yaml` and config files

Then deploy the result:

```bash
lore deploy @packages-manifest.yaml
writ adopt --from-receipt
```
