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
writ deploy noblefactor
writ deploy noblefactor thenobles
```

Use `all` to deploy every project in the repository:

```bash
writ deploy all
```

### Conflict resolution

When a target file already exists and isn't a writ-managed symlink, you have
four strategies:

```bash
# Stop on first conflict (default)
writ deploy noblefactor

# Back up conflicting files with timestamps
writ deploy --conflict=backup noblefactor

# Overwrite without backup
writ deploy --conflict=overwrite noblefactor

# Skip conflicting files and continue
writ deploy --conflict=skip noblefactor
```

### Custom segments

Override platform detection with custom segment values (e.g., `-s ROLE=desktop`).
See [Platform Awareness](/guides/writ/platform-awareness/#custom-segments) for details.

### Dry run

Preview what writ would do without making changes:

```bash
writ deploy --dry-run noblefactor
```

## Check status

Perform full-stack drift detection across symlinks, copied files, and packages:

```bash
# Scan all deployed files
writ reconcile

# Check specific project
writ reconcile noblefactor

# Also check package installation status
writ reconcile --packages
```

Status indicators:

| Symbol | Meaning |
|--------|---------|
| `✓ Linked` | Symlink exists and points to project |
| `✓ Copied` | Template/secret was copied and exists |
| `✓ Installed` | Package is installed and verified (with `--packages`) |
| `⚠ Conflict` | File exists but isn't a writ symlink |
| `✗ Missing` | Project file has no corresponding symlink |
| `? Orphan` | Symlink points to nonexistent source |
| `↑ Stale` | Source changed since deployment |
| `M Modified` | Target was edited locally |
| `! Conflict` | Both source and target changed |

### Automatic repair

Fix detected issues automatically:

```bash
writ reconcile --fix
```

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

## Migrate existing dotfiles

The `writ migrate` command uses AI to analyze your existing dotfiles repository
and create a migration plan to writ's layered structure:

```bash
writ migrate ~/dotfiles
```

This analyzes your dotfiles, detects the current structure (GNU Stow, chezmoi,
custom scripts, etc.), and generates a migration plan.

### Configuring the AI provider

By default, writ uses [Ollama](https://ollama.ai) for local inference. To use
a cloud provider:

```bash
# GitHub Models (free with GitHub account)
DEVLORE_MODEL_PROVIDER=github DEVLORE_MODEL_API_KEY=$(gh auth token) \
  writ migrate ~/dotfiles

# Anthropic Claude
writ --model-provider=anthropic --model-api-key=sk-... migrate ~/dotfiles
```

### Migration workflow

```bash
# 1. Generate migration plan (dry run)
writ migrate --dry-run ~/dotfiles

# 2. Review the plan, then execute
writ migrate ~/dotfiles

# 3. Link the migrated directory as a layer
writ migrate --link ~/dotfiles
```

See [writ migrate](/cli/writ/migrate/) for all options.

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
writ decommission noblefactor
```

Safety behavior depends on state tracking:

| State | Behavior |
|-------|----------|
| Signed state file | Safe removal with drift detection |
| Unsigned state file | Warning, requires `--force` |
| No state file | Error: cannot safely remove |

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
