---
title: "Getting Started"
description: "Install DevLore and deploy your first environment"
tool: "devlore"
category: "tutorial"
order: 1
---

# Getting Started with DevLore

DevLore is a two-tool suite for managing portable development environments.
**Writ** manages your dotfiles and configuration through platform-aware symlinks.
**Lore** handles software installation by capturing tribal knowledge about packages.

Together, they let you declare your environment once and deploy it everywhere you work.

## What you'll learn

- Install writ and lore
- Initialize an environment repository
- Deploy your first project
- Install software from a manifest

## Install

Download the latest release for your platform:

```bash
# macOS (Apple Silicon)
curl -L https://github.com/NobleFactor/devlore-cli/releases/latest/download/writ-darwin-arm64 -o writ
curl -L https://github.com/NobleFactor/devlore-cli/releases/latest/download/lore-darwin-arm64 -o lore

# Linux (amd64)
curl -L https://github.com/NobleFactor/devlore-cli/releases/latest/download/writ-linux-amd64 -o writ
curl -L https://github.com/NobleFactor/devlore-cli/releases/latest/download/lore-linux-amd64 -o lore

chmod +x writ lore
sudo mv writ lore /usr/local/bin/
```

Or use the self-install command for shell completions and man pages:

```bash
writ self-install --prefix=~/.local
lore self-install --prefix=~/.local
```

## Initialize a repository

A writ repository is a directory containing your dotfiles organized into projects.
Each project is a subdirectory whose files get symlinked to your home directory.

```bash
writ repo init --layer=personal
```

This creates a new git repository at `~/.local/share/devlore/repos/personal/` with
the standard directory structure.

## Create your first project

A project is simply a directory in your repository. Files inside it mirror
the structure of your home directory:

```bash
cd ~/.local/share/devlore/repos/personal/
mkdir -p noblefactor/.config/git

# Move your existing gitconfig into the project
mv ~/.config/git/config noblefactor/.config/git/config

# Add more files
cp ~/.zshrc noblefactor/.zshrc
```

## Deploy the project

```bash
writ add noblefactor
```

Writ creates symlinks from your home directory to the project files:

```
~/.zshrc → repos/personal/noblefactor/.zshrc
~/.config/git/config → repos/personal/noblefactor/.config/git/config
```

## Check status

```bash
writ status noblefactor
```

```
noblefactor (personal)
  ✓ Linked  .zshrc
  ✓ Linked  .config/git/config
```

## Add software with lore

If your project includes a `packages-manifest.yaml` file, writ automatically
delegates to lore for software installation:

```yaml
# noblefactor/packages-manifest.yaml
packages:
  - gh
  - jq
  - ripgrep
  - neovim:
      with: [lsp]
```

```bash
writ add noblefactor
# → symlinks configuration files
# → calls lore to install gh, jq, ripgrep, neovim
```

See [Packages Manifest](/guides/writ/packages-manifest/) for the full format reference.

Or install packages directly with lore:

```bash
lore deploy gh jq ripgrep neovim
```

## Next steps

- [Manage environments](/guides/writ/manage-environments/) — Learn conflict handling, removal, and upgrades
- [Platform awareness](/guides/writ/platform-awareness/) — Configure platform-specific variants
- [Secrets management](/guides/writ/secrets/) — Encrypt sensitive files with age
- [Deploy packages](/guides/lore/deploy-packages/) — Use lore's four-phase pipeline
- [Create manifests](/guides/lore/create-manifests/) — Package tribal knowledge for sharing
