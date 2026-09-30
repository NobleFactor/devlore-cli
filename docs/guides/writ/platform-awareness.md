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

Platform-awareness uses **directory-level segment matching**. A project directory's name can carry a selector: the
words after the project that say which machines the directory is for. Writ and lore read selectors with one grammar,
and the [Selectors](/guides/selectors/) guide is where its rules live. This guide covers what you need to lay out a
project.

```
Home/
├── noblefactor/          # Base project (all platforms)
├── noblefactor.Darwin/   # macOS-specific variant
├── noblefactor.Linux/    # Linux-specific variant
└── noblefactor.Debian/   # Debian and every distribution descended from it, Ubuntu among them
```

When you run `writ deploy noblefactor`, writ deploys files from:
1. The base `noblefactor/` directory (always)
2. Any variant directories that match your current platform

On macOS, both `noblefactor/` and `noblefactor.Darwin/` are deployed. On Debian, and on Ubuntu, which descends from
Debian, `noblefactor/`, `noblefactor.Linux/` and `noblefactor.Debian/` are all deployed. Files in more specific
variants override files from less specific ones.

## Segment detection

Writ detects three built-in segments:

| Segment | Detection | Example values |
|---------|-----------|----------------|
| OS | Go's name for the operating system (`GOOS`), capitalized | `Darwin`, `Linux`, `Windows` |
| DISTRO | `ID` in os-release, on Linux only | `Debian`, `Ubuntu`, `Fedora`, `RHEL` |
| ARCH | Go's name for the architecture (`GOARCH`) | `amd64`, `arm64` |

A variant's OS word may name any link of the machine's **chain**. On Linux the chain runs `Unix`, `Linux`, then the
distributions DISTRO descends from, as os-release's `ID_LIKE` states them, then DISTRO itself. On Ubuntu that is
`Unix`, `Linux`, `Debian`, `Ubuntu`, so `noblefactor.Debian` applies there, while DISTRO stays `Ubuntu`. Every
operating system's chain is under [The chain](/guides/selectors/#the-chain).

The built-ins take `--segment` and `WRIT_SEGMENT_<NAME>`, as custom segments do, without being declared.

### OS family matching

`Unix` is the first link of every Unix's chain, so it matches macOS, Linux, FreeBSD, OpenBSD and NetBSD:

```
Home/
├── noblefactor.Unix/     # Matches macOS, Linux, FreeBSD, OpenBSD and NetBSD
├── noblefactor.Darwin/   # Matches macOS only
└── noblefactor.Windows/  # Matches Windows only
```

This is useful for shell configurations that work on any Unix-like system.

### Segment naming rules

Selector words are case-sensitive, and a name reads in a fixed order: the project, then one OS word, then the
architecture, then any custom segments. One OS word is enough, because the chain supplies the rest. Common patterns:

| Use | Don't use | Reason |
|-----|-----------|--------|
| `Darwin` | `darwin`, `macos` | The OS word is Go's name for it, capitalized |
| `Linux` | `linux` | The OS word is Go's name for it, capitalized |
| `Ubuntu` | `ubuntu` | os-release's `ID`, capitalized: see [spelling](/guides/selectors/#how-an-id-is-spelled) |
| `Debian` | `Linux.Debian` | One OS word; the chain supplies `Linux` |
| `Debian.arm64` | `arm64.Debian` | The OS comes before the architecture |

An architecture may be written either way: `arm64` or `aarch64`, `amd64` or `x86_64`.

A name that breaks these rules, or holds a word writ doesn't know, such as a misspelling, refuses the deploy before
anything changes, while a well-formed name for another machine, such as `noblefactor.Fedora` on Ubuntu, is skipped.
See [Grammar errors](/guides/selectors/#grammar-errors).

## Directory matching examples

Given these project variants on a macOS ARM machine (OS=Darwin, ARCH=arm64), whose chain is `Unix`, `Darwin`:

| Directory | Matches | Why |
|-----------|---------|-----|
| `noblefactor` | Yes | Base name, no suffixes |
| `noblefactor.Unix` | Yes | `Unix` is in the chain |
| `noblefactor.Darwin` | Yes | OS matches |
| `noblefactor.Darwin.arm64` | Yes | OS and ARCH match |
| `noblefactor.Linux` | No | `Linux` isn't in the chain |
| `noblefactor.Debian` | No | `Debian` isn't in the chain |
| `noblefactor.arm64` | Yes | ARCH matches |
| `noblefactor.arm64.Darwin` | Refused | Out of order: the OS comes before the architecture |

## Custom segments

Beyond automatic detection, writ supports custom segments for more granular control. Declare each one in
configuration, with every value it may take, so that a misspelling is refused instead of silently matching nothing:

```yaml
# ~/.config/devlore/config.yaml
writ:
  segments:
    - name: ROLE
      values: [desktop, server]
      value: desktop
    - name: SITE
      values: [aws, home]
```

`value` is this machine's value. `--segment` (`-s`) sets a value for one run, and so does `WRIT_SEGMENT_<NAME>` in the
environment; the flag beats the environment, which beats configuration. `writ deploy`, `writ upgrade`,
`writ reconcile` and `writ adopt` all take `--segment`:

```bash
writ deploy -s ROLE=desktop noblefactor
WRIT_SEGMENT_ROLE=server writ deploy noblefactor
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

A name carries the values in the order their segments are declared, and a segment with no value matches no directory.
See [Extra segments](/guides/selectors/#extra-segments).

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

When more than one directory provides the same file, writ applies them in order and the last applied wins. A project's
variants apply from general to specific: on Ubuntu, `noblefactor/`, then `noblefactor.Linux/`, then
`noblefactor.Debian/`. Within a layer, the implicit projects apply first, then the projects the last deployment
recorded that you didn't name, then the ones you name, left to right. Layers apply base, then team, then personal, so
a file in a later layer beats the same file in an earlier one. The full order, and how the OS, the architecture and
custom segments rank against each other, is under
[Which directories apply, and in what order](/guides/selectors/#which-directories-apply-and-in-what-order).

This order decides **files**, which land at one target path.

For `packages-manifest.yaml` files, variants are **combined** rather than
replaced, and so are layers: a manifest lands nowhere in the filesystem, so
there is no target path to arbitrate. How claims from several manifests are
merged is the [Packages Manifest](/guides/writ/packages-manifest/) guide's
subject; this guide only says that they are.

## The `common` project

The project name `common` is reserved and has special behavior (the same pattern
as Ansible's implicit `all` group, named `common` so it cannot be misread as
"every project"):

- **Always matched**: `common` and its variants (`common.Darwin`, `common.Linux`,
  etc.) are included in every `writ deploy` and `writ upgrade` selection
- **Implicit inclusion**: Users don't specify `common`; it's automatic, and naming it on the command line is refused. A
  bare `writ deploy` deploys the implicit set: `common` and one project per registered layer repository, named for the
  repository (`noblefactor-ops`, `devlore-cli`, `personal`), plus whatever the current deployment already holds
- **Base configuration**: Use `common/` for configuration that applies everywhere
- **Destruction stays explicit**: `writ decommission` never includes `common`
  implicitly

```
Home/
├── common/                   # Config for all machines (automatic)
├── common.Darwin/            # macOS additions (automatic on macOS)
├── noblefactor/              # Personal project (explicit)
└── microsoft/                # Work project (explicit)
```
