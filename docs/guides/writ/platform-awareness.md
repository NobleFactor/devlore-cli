---
title: "Platform Awareness"
description: "Configure platform-specific variants for cross-platform environments"
tool: "writ"
category: "concept"
order: 3
---

# Platform Awareness

Writ automatically detects your operating system and selects platform-specific
file variants during deployment. This lets you maintain a single repository
that works across macOS, Linux, and Windows.

## How it works

When deploying a project, writ checks each file for platform-specific suffixes.
If a variant matching your current platform exists, it takes precedence over
the base file.

Platform suffixes are appended to the filename:

```
.zshrc              # Base file (used when no platform variant matches)
.zshrc.Darwin       # Used on macOS
.zshrc.Linux        # Used on Linux
.zshrc.Windows      # Used on Windows
```

The deployed symlink always points to the correct variant without the suffix:

```bash
# On macOS:
~/.zshrc → repos/personal/noblefactor/.zshrc.Darwin

# On Linux:
~/.zshrc → repos/personal/noblefactor/.zshrc.Linux
```

## Platform detection

Writ detects the following platform values automatically:

| Value | System |
|-------|--------|
| `Darwin` | macOS (any architecture) |
| `Linux` | Linux distributions |
| `Windows` | Windows (via WSL or native) |

## Directory variants

Entire directories can have platform suffixes. All files within a
platform-specific directory are deployed only on that platform:

```
noblefactor/
├── .config/
│   ├── alacritty/              # Shared config
│   │   └── alacritty.toml
│   ├── alacritty.Darwin/       # macOS-specific alacritty config
│   │   └── alacritty.toml
│   └── systemd.Linux/          # Linux-only systemd units
│       └── user/
│           └── backup.service
```

## Custom segments

Beyond OS detection, writ supports custom segments for more granular control.
Define segments at deploy time:

```bash
writ add -s ROLE=desktop noblefactor
writ add -s ROLE=server noblefactor
```

Name files with segment suffixes:

```
.config/
├── polybar/config.ROLE=desktop     # Only on desktop machines
└── nginx/nginx.conf.ROLE=server    # Only on servers
```

Multiple segments can be combined:

```bash
writ add -s ROLE=desktop -s DISPLAY=hidpi noblefactor
```

## Precedence rules

When multiple variants could apply, writ uses this precedence order:

1. Most specific match (platform + custom segments)
2. Platform-only match
3. Base file (no suffix)

If no variant matches and no base file exists, the file is skipped.

## Platform-specific packages

The same mechanism works for `packages.manifest` files, allowing
platform-specific software lists:

```
noblefactor/
├── packages.manifest           # Common packages (jq, gh, ripgrep)
├── packages.manifest.Darwin    # macOS packages (brew-only tools)
└── packages.manifest.Linux     # Linux packages (apt/dnf-only tools)
```

Writ merges the base manifest with the platform-specific one before
delegating to lore.
