---
title: "The team layer's Darwin manifest installs shellcheck, the shell-lint gate's tool, and its deploy is the live run of #1151 and #1179"
issue: https://github.com/NobleFactor/devlore-cli/issues/1181
status: active
created: 2026-10-25
updated: 2026-10-25
---

# Plan: shellcheck on Darwin, and the live run

## Summary

**The problem.** #1151's Phase 4 and #1179's are one live run: a `writ deploy` whose package manager shows its progress on the terminal as it happens, and shows nothing under `--silent`. The run needs a package to install that nothing else depends on while it is gone. GNU tar and coreutils, which #1179 claimed, are in constant use. shellcheck is not claimed at all: the shell-lint gate runs it, and it is on Media-Server because it was installed by hand.

**The deliverable.** The team layer's Darwin manifest claims `pkg:port/shellcheck`, sorted by purl. After the merge, shellcheck is uninstalled and a deploy reinstalls it with MacPorts' progress live, then again under `--silent` with nothing shown. A follow-up pull request sets this plan, #1179's and #1151's `complete`, closing all three.

## Issue 1181

[The team layer's Darwin manifest installs shellcheck, the shell-lint gate's tool, and its deploy is the live run of #1151 and #1179](https://github.com/NobleFactor/devlore-cli/issues/1181). A task under feature [#1128](https://github.com/NobleFactor/devlore-cli/issues/1128) of Epic [#1118](https://github.com/NobleFactor/devlore-cli/issues/1118), The DevLore Broker.

## Goals

1. A deploy on Darwin installs shellcheck from MacPorts when it is missing.
2. The live run shows a package manager's progress as it happens, and nothing under `--silent`.
3. #1151, #1179 and #1181 close.

## Current State

| Where | Today |
| --- | --- |
| `Home/common.Darwin/packages-manifest.yaml` | claims `pkg:port/coreutils` and `pkg:port/gnutar` (#1179) |
| shellcheck on Media-Server | MacPorts `shellcheck @0.11.0_0`, installed by hand |
| `writ deploy` | reads the team layer from its main clone on `develop`, with no flag to read it elsewhere, so a claim reaches a deploy only once it merges |
| `docs/plans/task/1179-gnutar-manifest.md` Phase 4 | names GNU tar as the package uninstalled for the run |

## Requirements

### Requirement 1: the claim

`Home/common.Darwin/packages-manifest.yaml` gains `pkg:port/shellcheck`, after `pkg:port/gnutar` in purl order, with a comment naming the shell-lint gate, `build/star lint shell`, as the reason.

### Requirement 2: the run

On Media-Server, after the merge: `make install`, so the new `writ` deploys; `sudo port uninstall shellcheck`; `writ deploy`, which installs it with the port command's progress on the terminal as it happens; `sudo port uninstall shellcheck` again; `writ deploy --silent`, which installs it and shows nothing; `shellcheck --version` answers.

### Requirement 3: the plans close

A follow-up pull request ticks this plan's Phase 3 and sets it `complete`; ticks #1179's Phase 4, its package corrected from GNU tar to shellcheck, and sets it `complete`; and ticks #1151's Phase 4 and sets it `complete`. Its description closes #1151, #1179 and #1181.

## Implementation Phases

### Phase 1: the plan lands

- [x] Reviewed with the owner, status `approved`, committed with the claim on `feature/1181-shellcheck-manifest` -- 2026-10-25
- [x] #1181's documents table carries this plan's link -- 2026-10-25

### Phase 2: the pull request (Requirement 1)

- [x] `make quality-gate` passes; the description says AI assisted and ends with `Signed-off-by:` and `Assisted-by: Claude Code`; the script is `~/Workspace/NobleFactor/devlore-cli.1181-shellcheck-manifest.bash`; status `active`, with Phase 3 named as the reason: the run needs the claim on `develop` -- 2026-10-25

### Phase 3: after the merge (Requirements 2 and 3)

- [ ] The run, as Requirement 2 states it, its output reported
- [ ] The follow-up pull request, as Requirement 3 states it

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/task/1181-shellcheck-manifest.md` | Create | This plan |
| `Home/common.Darwin/packages-manifest.yaml` | Modify | Requirement 1 |
| `docs/plans/task/1179-gnutar-manifest.md`, `1151-narration-bytes.md` | Modify | Requirement 3, by the follow-up |

## Related Documents

- [1179-gnutar-manifest.md](1179-gnutar-manifest.md) and [1151-narration-bytes.md](1151-narration-bytes.md), whose Phase 4 this run is
- [packages-manifest.md](../../guides/writ/packages-manifest.md) § Format, entries sorted by purl

## Open Questions

1. **shellcheck is gone for a few minutes.** `make quality-gate` and `build/star lint shell` fail between the uninstall and the reinstall. Nothing else on Media-Server uses it.
