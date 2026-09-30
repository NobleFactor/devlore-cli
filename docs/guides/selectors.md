---
title: "Selectors"
description: "How writ and lore choose the directories that apply to a machine, and in what order"
tool: "devlore"
category: "reference"
order: 2
---

# Selectors

A selector is the part of a directory name that says which machines the directory is for. `noblefactor.Debian.arm64`
is the `noblefactor` project, for Debian-lineage machines on 64-bit ARM. writ reads selectors on its layers' project
directories, and lore reads them on a package's platform directories. Both use one grammar and one implementation,
`pkg/selector` (#944).

## The grammar

A name reads, in this order:

```text
[<project>][.<os>][.<arch>][.<extra>...]
```

| Part | What it holds | Example |
| --- | --- | --- |
| project | writ's names only: `common`, a project named for a layer repository, or a project you name | `noblefactor` |
| OS | one word of the machine's chain: `Unix`, the operating system, or a distribution in its lineage | `Debian` |
| architecture | the machine's architecture, in either of its spellings | `arm64` |
| extras | one value of each segment declared in configuration, in the order declared | `desktop` |

Every part after the project is optional, and each appears at most once. Order is significant, because it lets you see
at a glance what a directory holds. `common.Debian.arm64` is a valid name; `common.arm64.Debian` is not.

lore's names carry no project. A package's platform directories are `Common`, the name with no selector words, and the
names made of the other parts: `Unix`, `Debian`, `Debian.arm64`, `arm64`.

## The chain

The OS part names one link of the machine's chain. The chain runs from the most general link to the most specific.

On Linux it follows the lineage the distribution states in `/etc/os-release`:

```text
Unix → Linux → <ID_LIKE reversed> → <ID>
```

| Machine | `ID` | `ID_LIKE` | Chain |
| --- | --- | --- | --- |
| Debian | `debian` | — | `Unix`, `Linux`, `Debian` |
| Ubuntu | `ubuntu` | `debian` | `Unix`, `Linux`, `Debian`, `Ubuntu` |
| Linux Mint | `linuxmint` | `ubuntu debian` | `Unix`, `Linux`, `Debian`, `Ubuntu`, `Mint` |
| Pop!_OS | `pop` | `ubuntu debian` | `Unix`, `Linux`, `Debian`, `Ubuntu`, `Pop` |
| Fedora | `fedora` | — | `Unix`, `Linux`, `Fedora` |
| RHEL | `rhel` | `fedora` | `Unix`, `Linux`, `Fedora`, `RHEL` |
| CentOS Stream | `centos` | `rhel fedora` | `Unix`, `Linux`, `Fedora`, `RHEL`, `CentOS` |
| Rocky Linux | `rocky` | `rhel centos fedora` | `Unix`, `Linux`, `Fedora`, `CentOS`, `RHEL`, `Rocky` |
| no os-release | — | — | `Unix`, `Linux` |
| macOS | — | — | `Unix`, `Darwin` |
| FreeBSD, OpenBSD, NetBSD | — | — | `Unix`, then the OS |
| Windows | — | — | `Windows` |

So on Ubuntu, a directory suffixed `Linux`, `Debian` or `Ubuntu` applies. In the owner's words: "if i'm on an unbuntu
system, i will match linux, debian, and unbuntu. rationale: ubuntu is like debian, and both are linux platforms."

A name carries one link, because the chain supplies the rest. `common.Debian` is the permanent form: when you say
Debian, everyone knows you mean Debian Linux. `common.Linux.Debian` names two links and is an error.

### How an ID is spelled

A distribution's os-release `ID` is lowercase. As a selector word, it's spelled this way:

| `ID` | Word |
| --- | --- |
| `debian`, `ubuntu`, `fedora`, `rocky`, `arch`, `manjaro`, `alpine` | `Debian`, `Ubuntu`, `Fedora`, `Rocky`, `Arch`, `Manjaro`, `Alpine` |
| `rhel` | `RHEL` |
| `centos` | `CentOS` |
| `linuxmint` | `Mint` |
| `almalinux` | `AlmaLinux` |
| `opensuse`, `opensuse-leap`, `opensuse-tumbleweed` | `OpenSUSE` |
| `suse` | `SUSE` |
| any other | the `ID` with its first letter uppercased: `pop` is `Pop` |

A word that appears twice in a chain, as openSUSE Leap's does, is kept once, at its most specific position.

## The architecture

The architecture is Go's name for it, and a name may use either spelling of it: `arm64` or `aarch64` for 64-bit ARM
(`aarch64` is what the Linux kernel reports), `amd64` or `x86_64` for 64-bit x86.

## Extra segments

An extra segment narrows a directory beyond platform, for example to desktop machines. It must be declared in
configuration, with every value it may take, so that a misspelling is caught instead of silently matching nothing:

```yaml
writ:
  segments:
    - name: ROLE
      values: [desktop, server]
      value: desktop
    - name: SITE
      values: [aws, home]
```

- **The declaration's order is the grammar's.** `noblefactor.Debian.arm64.desktop.home` reads project, OS,
  architecture, ROLE, SITE.
- **Values are unique across every segment,** and none may be an OS, distribution or architecture word, because a name
  carries values, never segment names: each value names its segment.
- **A value comes from** `--segment NAME=value`, else `WRIT_SEGMENT_<NAME>`, else `value` in configuration. A segment
  with no value matches no directory.
- **A name or value that isn't declared is refused,** from the command line and from the environment alike.
- **The built-in segments** OS, DISTRO and ARCH take `--segment` and `WRIT_SEGMENT_*` without a declaration, and no
  extra may use their names. Overriding DISTRO replaces the distribution and keeps its detected lineage.

writ refuses a declaration that holds any of these, listing every one it finds:

- a segment with no name
- a name declared twice
- a name that's built in: OS, DISTRO or ARCH
- an empty value
- a value that's an OS, distribution or architecture word
- a value declared twice, by one segment or by two
- a `value` that isn't one of its segment's `values`

## Which directories apply, and in what order

The selector takes a list of directory names and answers which apply to this machine, and in which order. A name applies
when each of its words names this machine: its OS word is in the machine's chain, its architecture is the machine's,
and each extra's value is the segment's current value.

Directories are applied in order, and a file held by more than one comes from the last one applied:

1. **Projects, as the command line reads.** writ applies the implicit projects first: `common`, then the one named for
   each layer repository. Then it applies the projects the last deployment recorded that you didn't name, and then the
   projects you name, left to right. Naming an implicit project, or naming one project twice, is a command-line error.
2. **Within a project, by the order a name reads.** The OS word's depth in the chain decides first, then the
   architecture, then each extra in configured order. Each part narrows the one to its left.
3. **Layers, left to right.** writ selects and orders directories within each layer, and processes base, then team,
   then personal.

On Ubuntu arm64, the order of application for one project is:

```text
common, common.arm64, common.Unix, common.Unix.arm64, common.Linux, common.Linux.arm64,
common.Debian, common.Debian.arm64, common.Ubuntu, common.Ubuntu.arm64
```

So `common.Ubuntu` beats `common.Debian.arm64`, and `common.Debian.arm64` beats `common.Debian`. With
`writ deploy noblefactor thenobles`, a file in `thenobles/` beats the same file in `noblefactor/` and in any of
`common`'s directories.

lore applies a package's platform directories in the same order, with `Common` first. On Ubuntu:
`Common`, `Unix`, `Linux`, `Debian`, `Ubuntu`. Every applicable directory's phase script runs, general to specific.

## Grammar errors

A name that breaks the grammar refuses the run before anything changes, and the refusal lists every such name at once,
with the rule each breaks:

- **Out of order:** `common.arm64.Debian`, where the architecture comes before the OS.
- **Two words for one part:** `common.Linux.Debian`, which has two OS words.
- **A word no part holds:** `common.Debain`, a misspelling; or `common.Nixos` on a machine that isn't NixOS, because
  the vocabulary is closed. A distribution the spelling table doesn't know names only itself, on its own machine;
  it must be added to the table before a layer can name it for other machines.

writ judges every directory of every layer a run reads, and lore every platform directory of a package it plans. A
well-formed name for another machine, such as `common.Fedora` on Ubuntu, isn't an error: it's excluded silently,
because a shared layer carries directories for every machine it serves.

## os-release

The chain comes from os-release, the freedesktop.org specification that virtually every current Linux distribution
ships, including those without systemd: Debian, Ubuntu, Fedora, RHEL and its rebuilds, SUSE, Arch, Alpine, Gentoo and
Void.

- **Where it lives:** `/etc/os-release`, and `/usr/lib/os-release` when the first is missing. On most systems the first
  is a symbolic link to the second.
- **`ID`** is the distribution's identifier, lowercase: `ubuntu`, `debian`, `rhel`.
- **`ID_LIKE`** is optional: the distributions this one is like, closest first. It's the distribution's own statement
  of its lineage, which is why the chain follows it.
- **Where it's missing:** distributions older than about 2012, some minimal containers, Android and some embedded
  systems. There the machine is "Linux, distribution unknown", and the chain stops at `Linux`.

Detection reads os-release and nothing else; it has no side effects and never fails.

### Package managers follow the lineage too

lore installs native packages with the machine's default package manager. A distribution devlore doesn't list takes
the managers of the closest ancestor its `ID_LIKE` names that devlore does list: Pop!_OS uses Ubuntu's apt.
