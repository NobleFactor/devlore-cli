---
title: "Platform Awareness"
description: "Configure platform-specific variants for cross-platform environments"
tool: "writ"
category: "concept"
order: 3
---

# Platform Awareness

Writ automatically detects your operating system and selects platform-specific
project variants during deployment. This lets you maintain a single repository
that works across macOS, Linux, and Windows.

## How it works

Platform-awareness uses **directory-level segment matching**. Project directories
can have suffixes indicating which platforms they apply to:

```
Home/
├── noblefactor/              # Base project (all platforms)
├── noblefactor.Darwin/       # macOS-specific variant
├── noblefactor.Linux/        # Linux-specific variant
└── noblefactor.Linux.Debian/ # Debian/Ubuntu-specific variant
```

When you run `writ deploy noblefactor`, writ deploys files from:
1. The base `noblefactor/` directory (always)
2. Any variant directories that match your current platform

On macOS, both `noblefactor/` and `noblefactor.Darwin/` are deployed.
On Debian Linux, `noblefactor/`, `noblefactor.Linux/`, and `noblefactor.Linux.Debian/`
are all deployed. Files in more specific variants override files from less specific ones.

## Segment detection

Writ detects segments automatically in this order:

| Segment | Detection | Example values |
|---------|-----------|----------------|
| OS | `uname -s` | `Darwin`, `Linux`, `Windows` |
| DISTRO | `/etc/os-release` | `Debian`, `Ubuntu`, `Fedora`, `RHEL` |
| ARCH | `uname -m` | `amd64`, `arm64` |

Segments are matched in order: OS → DISTRO → ARCH → custom. Empty or unset
segments are skipped during matching.

### OS family matching

The special `Unix` segment matches both `Darwin` and `Linux`:

```
Home/
├── noblefactor.Unix/     # Matches macOS AND Linux
├── noblefactor.Darwin/   # Matches macOS only
└── noblefactor.Windows/  # Matches Windows only
```

This is useful for shell configurations that work on any Unix-like system.

### Segment naming rules

Segment names in directory suffixes are case-sensitive and must exactly match
the detected values. Common patterns:

| Use | Don't use | Reason |
|-----|-----------|--------|
| `Darwin` | `darwin`, `macos` | Matches `uname -s` output |
| `Linux` | `linux` | Matches `uname -s` output |
| `arm64` | `aarch64` | Normalized architecture |
| `amd64` | `x86_64` | Normalized architecture |
| `Ubuntu` | `ubuntu` | Matches distro ID |

## Directory matching examples

Given these project variants on a macOS ARM machine (OS=Darwin, ARCH=arm64):

| Directory | Matches | Why |
|-----------|---------|-----|
| `noblefactor` | Yes | Base name, no suffixes |
| `noblefactor.Darwin` | Yes | OS matches |
| `noblefactor.Darwin.arm64` | Yes | OS + ARCH match (DISTRO skipped) |
| `noblefactor.Linux` | No | OS doesn't match |
| `noblefactor.Debian` | No | DISTRO not set on macOS |
| `noblefactor.arm64` | Yes | ARCH matches |

## Custom segments

Beyond automatic detection, writ supports custom segments for more granular control.
Specify segments at deploy time:

```bash
writ deploy -s ROLE=desktop noblefactor
writ deploy -s ROLE=server noblefactor
```

Create directories with custom segment suffixes:

```
Home/
├── noblefactor/                    # All machines
├── noblefactor.Darwin/             # macOS only
├── noblefactor.desktop/            # ROLE=desktop
├── noblefactor.Darwin.desktop/     # macOS + ROLE=desktop
└── noblefactor.server/             # ROLE=server
```

Multiple segments can be combined:

```bash
writ deploy -s ROLE=desktop -s SITE=aws noblefactor
```

## File organization

Files within a project directory don't have platform suffixes—the suffixes are
on the directory name. Place platform-specific files in the appropriate variant
directory:

```
Home/
├── noblefactor/
│   ├── .zshrc                # Shared shell config
│   ├── .gitconfig            # Shared git config
│   └── packages-manifest.yaml   # Common packages
├── noblefactor.Darwin/
│   ├── .zshrc                # macOS shell config (overrides base)
│   └── packages-manifest.yaml   # macOS-only packages (merged with base)
└── noblefactor.Linux/
    ├── .zshrc                # Linux shell config (overrides base)
    └── packages-manifest.yaml   # Linux-only packages (merged with base)
```

## Precedence rules

When multiple variants provide the same file, writ uses the most specific match:

1. Most specific match (platform + all custom segments)
2. Platform-only match
3. Base project (no suffix)

For `packages-manifest.yaml` files, variants are **merged** rather than replaced.
This lets you define common packages in the base and platform-specific packages
in variants.

## The `all` project

The project name `all` is reserved and has special behavior:

- **Always matched**: `all` and its variants (`all.Darwin`, `all.Linux`, etc.)
  are included for every `writ deploy` operation
- **Implicit inclusion**: Users don't need to specify `all`—it's automatic
- **Base configuration**: Use `all/` for configuration that applies everywhere

```
Home/
├── all/                      # Config for all machines (automatic)
├── all.Darwin/               # macOS additions (automatic on macOS)
├── noblefactor/              # Personal project (explicit)
└── microsoft/                # Work project (explicit)
```
