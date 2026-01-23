---
title: "Manage Environments"
description: "Deploy, update, and remove configuration projects"
tool: "writ"
category: "tutorial"
order: 2
---

# Manage Environments

This guide covers the full lifecycle of managing your environment with writ:
deploying projects, handling conflicts, checking status, upgrading templates,
and removing deployments.

## Deploy projects

Deploy one or more projects from your repository:

```bash
writ add noblefactor
writ add noblefactor thenobles
```

Use `all` to deploy every project in the repository:

```bash
writ add all
```

### Conflict resolution

When a target file already exists and isn't a writ-managed symlink, you have
four strategies:

```bash
# Stop on first conflict (default)
writ add noblefactor

# Back up conflicting files with timestamps
writ add --conflict=backup noblefactor

# Overwrite without backup
writ add --conflict=overwrite noblefactor

# Skip conflicting files and continue
writ add --conflict=skip noblefactor
```

### Custom segments

Override platform detection with custom segment values:

```bash
writ add -s ROLE=desktop noblefactor
writ add -s ROLE=server -s DISPLAY=headless noblefactor
```

### Dry run

Preview what writ would do without making changes:

```bash
writ add --dry-run noblefactor
```

## Check status

View the state of deployed projects:

```bash
# Scan all deployed files
writ status

# Check specific project
writ status noblefactor
```

Status indicators:

| Symbol | Meaning |
|--------|---------|
| `✓ Linked` | Symlink exists and points to project |
| `✓ Copied` | Template/secret was copied and exists |
| `⚠ Conflict` | File exists but isn't a writ symlink |
| `✗ Missing` | Project file has no corresponding symlink |
| `? Orphan` | Symlink points to nonexistent source |

### Drift detection

Check whether templates or secrets have been modified since deployment:

```bash
writ status --drift
```

Drift indicators:

| Symbol | Meaning |
|--------|---------|
| `↑ Stale` | Source changed since deployment |
| `M Modified` | Target was edited locally |
| `! Conflict` | Both source and target changed |

## Upgrade templates and secrets

When source templates or secrets change, regenerate the copied files:

```bash
# Upgrade all copied files
writ upgrade

# Upgrade specific project
writ upgrade noblefactor

# Force overwrite locally modified files
writ upgrade --force
```

Upgrade only affects copied files (templates and decrypted secrets).
Symlinks always point to the source and don't need upgrading.

## Adopt existing files

Bring existing configuration files under writ management:

```bash
# Adopt a single file
writ adopt noblefactor .zshrc

# Adopt multiple files
writ adopt noblefactor .zshrc .bashrc .config/nvim/init.lua

# Adopt an entire directory recursively
writ adopt noblefactor .config/nvim

# Adopt into team layer
writ adopt --layer=team shared .editorconfig
```

The file is moved into the project directory and replaced with a symlink.

### Adopt from lore receipt

After installing software with lore, adopt the generated configuration:

```bash
writ adopt --from-receipt
```

This reads the lore deployment receipt and moves any generated config files
into your environment repository.

## Remove deployments

Remove deployed files for a project:

```bash
writ remove noblefactor
```

Safety behavior depends on state tracking:

| State | Behavior |
|-------|----------|
| Signed state file | Safe removal with drift detection |
| Unsigned state file | Warning, requires `--force` |
| No state file | Error: cannot safely remove |

### Decommission software

Remove both configuration and associated software:

```bash
writ remove --decommission noblefactor
```

This delegates to `lore decommission --orphans-only` to remove packages
that were installed via the project's `packages.manifest` and are no longer
referenced by any other project.

## Inspect details

Get detailed information about a project or specific file:

```bash
# Project details
writ inspect noblefactor

# Specific file details (source, checksums, drift status)
writ inspect ~/.zshrc

# Alternative output formats
writ inspect noblefactor --format yaml
writ inspect noblefactor --format json
```
