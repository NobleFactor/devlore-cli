---
title: "Writ Repo Remote"
status: complete
created: 2026-08-09
updated: 2026-08-09
---

# Plan: Writ Repo Remote

## Summary

`writ repo add`'s location operand becomes polymorphic (ruled 2026-08-09):

```
writ repo add <layer> <working-tree-root>
writ repo add <layer> <repository-url> [<working-tree-root>]
```

A repository URL triggers a `git clone`; the optional third positional is `git clone
<url> [<directory>]`'s own grammar, and it names the same concept as the local form's
second positional — in every spelling, the working-tree-root is where the layer lives.
When omitted, the clone lands in the writ-owned home
(`XDG_DATA_HOME/devlore/writ/repos/<layer>`) — right for consume-only base/team layers;
personal layers usually name a destination (`~/Workspace/Personal`).

## Rulings

1. **Positional destination**, not `--into` — git-clone's grammar, one operand concept.
2. **Post-placement the repository is entirely the user's**: writ performs no hidden git
   operations, ever. Updating layer content is `git pull` + `writ upgrade`.
3. **The VCS-agnostic claim dies**: deploy pins layers from git history (the scenario
   proved non-repos are refused), so writ layers ARE git repositories. The guide says so
   honestly. (The claim survived the #352 rewrite unexamined — one occurrence, corrected
   here.)
4. **Add-time validation**: the working-tree-root must be a git working tree (`.git`
   present) — failing at `add` beats failing at `deploy`'s pin.
5. **`--branch <name>`** rides as the one flag (git-clone's own), immediately useful:
   `writ repo add personal <url> --branch devlore-cli/writ-layer` is the pre-dogfooding
   registration in one line.

## Mechanics

- **URL detection is git-clone's own rule**: `://` anywhere, or scp-like
  `[user@]host:path` — a colon before any slash with more than one character before it
  (a single letter is a Windows drive, the lesson already paid for in #351).
- **Atomicity**: the clone fully lands, then the symlink; a failed clone into a
  destination we created is cleaned up and nothing registers. Clone output streams to
  stderr (auth prompts and progress stay visible).
- **Three-arg local form is an error** ("a working-tree-root takes no destination").
- Tests: URL-detection table, local-with-destination error, add-time `.git` validation,
  and a real offline clone via a `file://` URL into both the default home and a
  positional destination.

## Delivered (2026-08-09)

The polymorphic location, `--branch`, the writ-owned repos home (`cli.WritReposDir`),
add-time working-tree validation, clone atomicity with cleanup, and the guide correction
(the VCS-agnostic claim replaced by the honest git basis; the URL form documented with the
no-hidden-git-operations promise). Tests: the URL-detection table (schemes, scp-like,
drive letters, relative colons), every malformed combination, and real offline clones via
`file://` into both the default home and a positional destination — with remove proven to
leave the clone untouched.

Landed as [PR #353](https://github.com/NobleFactor/devlore-cli/pull/353); squash-merged to
`develop` 2026-08-09.
