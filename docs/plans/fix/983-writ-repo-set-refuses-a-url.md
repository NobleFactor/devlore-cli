---
title: "writ repo set refuses a URL layer whose clone directory exists from an earlier, unregistered run"
issue: https://github.com/NobleFactor/devlore-cli/issues/983
status: active
created: 2026-10-21
updated: 2026-10-21
---

# Plan: writ repo set completes what an earlier run started, gives the branch asked for, and says what it did

Lane 45 of #949. One pull request.

## Summary

`writ repo set <layer> <url>` refuses when the clone's destination already exists and is not that layer's
registration (`cloneRepository`, `cmd/writ/writ/repo_cmd.go`: "clone destination ... already exists"). A clone that
lands and whose registration then fails leaves a directory that makes the same command fail on the rerun, so the
recovery the idempotency ruling promises ("running the same command again is the recovery", ruled 2026-09-26) does not
hold. The installers call `writ repo set` for each layer given, and tell the user to run them again on a failure.

Reviewing the fix, the owner ruled how `set` treats the branch and what it says, for both forms.

## Rulings

- 2026-10-21, of the branch: "if you say you want a different branch, we should give it to you with a narration that
  says, here's what I did." Branch given: switch to it, for a path or a URL; created from `origin` when only the remote
  has it; refused, registering nothing, when it exists nowhere or uncommitted changes block the switch. Branch not
  given: let the tree be, and say which branch it is on.
- 2026-10-21: "git switch is the right command to run; not git checkout."
- 2026-10-21, of being up to date with the branch asked for: fetch, switch, then fast-forward only (D2); agreed, "You're
  right. I was confusing fetch with pull."
- 2026-10-21, of what `set` says: for a path, "I found a repo at this location and it's on this branch"; for a URL,
  that it found the clone or cloned it there, and the branch it is on; for a branch asked for, that it switched to it,
  or that the branch is not there.

## Current State

At `0f5deff8`:

- `runRepoSet` settles the root (`settledRoot`), reports `unchanged` when the layer is already registered there, and
  otherwise clones (`cloneRepository`) when the location is a URL, else checks the path is a working tree
  (`validateWorkingTree`), then writes the registration.
- `cloneRepository` refuses any destination that exists, whatever it holds.
- `plannedRoot` refuses `--branch` with a path: "--branch applies to the repository-url form only".
- `git clone --branch` puts a fresh clone on the branch asked for; nothing switches the branch of a tree already on
  disk, and nothing says which branch a layer is on.
- `sourceOf` reads a tree's remote URL: the checked-out branch's upstream remote, else `origin`.
- The URL tests clone from `file://` source repositories (`sourceRepositoryNamed`).

## Requirements

### Requirement 1: The clone an earlier run left is used, not refused

- When a URL's destination already exists, `set` uses it, cloning nothing, if it is a git working tree whose remote
  (`sourceOf`) is the URL given (D1). Anything else there is refused, saying what it holds: not a git working tree, or
  a clone of another remote, named.

### Requirement 2: The branch asked for, by either form

- `--branch` applies to a path as well as a URL.
- Branch given, tree already on disk, in three steps (D2):
  1. `git fetch origin`, so `origin`'s branches are as they are now.
  2. `git switch <branch>`. When only `origin` has the branch, `git switch` creates the local branch tracking it. When
     the branch is in neither, `set` refuses and registers nothing. When uncommitted changes would be overwritten,
     `git switch` refuses, and so does `set`, giving git's reason and registering nothing.
  3. `git merge --ff-only origin/<branch>` when the branch is behind `origin`. When it has diverged, it has commits
     `origin` lacks: it is left as it is, and said so; the user's commits are theirs.
- Branch given, fresh clone: `git clone --branch`, as now.
- Branch not given: the tree is left on whatever it has checked out.
- A dry run switches nothing, and says what it would do.
- The switch happens before the registration is written, so a refusal leaves the layer as it was.

### Requirement 3: It says what it did

Before the registration line `set` says now:

| Case | It says |
| --- | --- |
| A path | `<layer>: found a repository at <root>, on <branch>` |
| A URL, cloned | `<layer>: cloned <url> into <root>, on <branch>` |
| A URL, found | `<layer>: found the clone of <url> at <root>, on <branch>` |
| A branch switched to | `, switched from <old> to <branch>` at the end of that line |
| Fetched | `<layer>: fetched origin` before it |
| Fast-forwarded | `<layer>: fast-forwarded <branch> to origin/<branch>` |
| Diverged | `<layer>: <branch> has commits origin lacks; left as it is` |

A detached HEAD is said as `detached at <commit>`. The registration line (`now <root>`, `was ..., now ...`) follows,
as now; `unchanged` stays for a layer already registered there, its branch line said too.

### Requirement 4: Tests

- URL, the clone an earlier run left at the default home, unregistered: registered, not cloned again (no "Cloning
  into"), said as found. Fails at `0f5deff8`.
- URL, a clone of another remote at the destination: refused, naming it. A directory that is not a working tree:
  refused.
- Path with `--branch`, a local branch: switched, said so. A branch only `origin` has, pushed after the tree's last
  fetch: fetched, switched, the local branch tracking it. A branch nowhere: refused, the layer not registered.
  Uncommitted changes the switch would overwrite: refused, the layer not registered, the tree untouched. A local
  branch behind `origin`: fast-forwarded. A local branch with commits `origin` lacks: left as it is, said so.
- No `--branch`: the tree's branch unchanged, and said.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/983-writ-repo-set-refuses-a-url` and reviewed with the owner, its branch handling
      ruled 2026-10-21

### Phase 2: The fix (Requirements 1 to 4)

- [x] `runRepoSet`, `plannedRoot` and the narration, with `existingClone`, `bringToBranch`, `treeBranch` and `gitRun`;
      the help says what `--branch` does. The ten new tests, and `TestRepo_Set_Errors`'s branch case, fail against
      `repo_cmd.go` at `1e79048f` and pass with the fix; `make test` passes. The tests read the narration from
      `os.Stderr`, to which the root binds writ's narrator when the command starts

### Phase 3: Merge

- [x] `make check-all`, finished green as the PR script runs it, before the script is called ready: at `6817465b`,
      exit 0, read before this tick. Corrected
      2026-10-21: `77e9fd57` ticked this box for `acd17da6` while `make check-all` had failed, exit 2, on two lint
      findings: `runRepoSet`'s cognitive complexity 33 (gocognit, over 20) and `plannedRoot`'s now-unused `branch`
      (unparam). The tick was committed in the same command that printed the failure, unread. Fixed: the tree's
      settling is `settleTree` and the dry run `emitDryRun`, and `branch` leaves `plannedRoot` and `settledRoot`
- [x] PR script written, shown, and handed over; the PR resolves #983
- [ ] CI on the pull request, every leg; nothing acts on the checks while the script's gate watches them

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/writ/writ/repo_cmd.go` | Modify | Requirements 1 to 3 |
| `cmd/writ/writ/repo_cmd_test.go` | Modify | Requirement 4 |
| The `writ repo set` help (`repo_cmd.go`); its generated reference, `docs/cli/`, follows and is not tracked | Modify | Requirement 2 |

## Decisions

- **D1. The remote is compared as written.** The rerun this serves is the same command with the same URL, which
  `git clone` wrote as the clone's `origin` verbatim. Normalizing URLs (a trailing `.git`, ssh against https) would
  accept clones of what may be another repository.
- **D2. Fetch, switch, fast-forward only.** With `--branch` and a tree already on disk, `set` fetches `origin` first,
  every time: only a fetch can say what `origin` has now. A fetch updates the remote-tracking refs alone, never the
  user's branches or files. The switch then finds a branch pushed since, and `git merge --ff-only` brings a branch that
  is behind up to date, refusing to merge one that has diverged, which is left as it is. Without `--branch`, no fetch.

## Open Questions

None.
