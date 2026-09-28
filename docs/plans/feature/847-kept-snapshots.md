---
title: "Kept snapshots: a lifetime keeps its layer worktrees, pinned by private refs; links target them; reconcile runs against the ref that produced each entry"
issue: https://github.com/NobleFactor/devlore-cli/issues/847
status: draft
created: 2026-09-27
updated: 2026-09-27
---

# Plan: kept snapshots

## Summary

Ruled 2026-09-27 by the owner, while lane 5 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916)
([#924](https://github.com/NobleFactor/devlore-cli/issues/924), reconcile repairs) was stopped on a finding:

> reconcile should run against the git ref that produced the result. we deploy, we upgrade, we reconcile. when we
> deploy, we have one ref. when we upgrade we have another ref. when we reconcile we run against the ref that
> produced the result.

> we must keep worktrees to support linkage. i realize this is a design change. we cannot properly support a
> deployment lifetime without keeping our snapshot and the mechanism for retaining this is a worktree.

This supersedes the 2026-08-08 ruling that links target the origin because snapshots are removed when the run
ends, and it widens [#847](https://github.com/NobleFactor/devlore-cli/issues/847) (pinned or live, per layer) into
the model for every lifetime.

**Draft. Not chartered, nothing built.** Four rulings are made, five remain (below); where this lands on #916 is
one of them. This document exists so that nothing ruled on 2026-09-27 is lost before the replan.

## Today

Read 2026-09-27 at `5509da4d`.

| Component | Today |
| --- | --- |
| deploy's pin (`cmd/writ/writ/deploy/deploy.go:179`) | each layer pinned to a git worktree snapshot of HEAD under `~/.cache/devlore/snapshots`, removed when the run ends |
| the plan (`cmd/writ/writ/deploy/plan.go:380`, `:531`) | records each layer's `commit_hashes` and `dirty_layers`; a file's `source` is the origin clone's path, its `read_from` the snapshot's |
| links | target the origin clone's working tree, so a pull or an uncommitted edit changes the deployment at once |
| upgrade | pins nothing and records no commit ([#844](https://github.com/NobleFactor/devlore-cli/issues/844)) |
| reconcile (`cmd/writ/writ/reconcile/reconcile.go:196`) | reads the working-tree file at the recorded source path; never reads the recorded commit |

## Rulings, 2026-09-27

1. **A private ref namespace pins each snapshot's commit, not tags.** `refs/devlore/...`. The functional delta, as
   put: tags are pushed by `git push --tags` and then fetched by every other clone, deleted by
   `git fetch --prune-tags`, and seen by `git tag` and `git describe` (the docs-publish workflow runs `describe`
   unfiltered). A private ref pins identically and none of those touch it.
2. **Snapshots are writable.** "Reconcile detects drift with the data at hand and can easily restore content."
   Read-only snapshots were rejected: they take away the in-place edit.
3. **Retention is a branch cut from the pinned commit, and the developer opens the pull request.** "I'm talking
   about the PR process in this case." The origin is the layer repository's remote and the way back is a pull
   request. Retain cuts a branch from the pinned commit in the layer clone and commits the edit there: durable at
   once, and the pull request's base does the three-way merge if the default branch has moved on. The branch is
   `retain/<host>/<date>`: "clear about the source: a committer who retained these changes on a specific host on a
   specific date." Rejected: writ pushing and opening the pull request itself; the edit handed to the working tree
   uncommitted.
4. **An edit is always retained.** A plain `writ reconcile` that finds an edit always cuts the retain branch. The
   developer may then restore the record, with a command flag on reconcile: the no-command-flags ruling
   ([#762](https://github.com/NobleFactor/devlore-cli/issues/762) Requirement 2, #924) is broken for this,
   knowingly. Restoring changes only what the deployment shows; the branch stays, and the developer tosses it,
   merges it, and upgrades as they like. What upgrade then does depends on what they did with the branch.
   Restoring by default was rejected: silent data loss.

**Amended by the owner:** the sparse checkout covers every scope directory -- `Home`, `System` and, especially on
Windows, `ProgramFiles`, `ProgramData` and whatever `writ.scopes` adds
([#926](https://github.com/NobleFactor/devlore-cli/issues/926)).

**Noted by the owner, 2026-09-27: the narration carries the choice.**

> make note that the reconcile narrations will be critical to understanding the result of reconciliation and the
> path forward for the deployment. It's a simple choice that needs to be well understood: retain the changes, toss
> the changes, and optionally follow-up with a PR to the branch of origin or elsewhere.

So what `writ reconcile` narrates is a requirement, not polish. For every edit it finds it says what it did (the
edit was retained on `retain/<host>/<date>`, and whether the record was restored), what the deployment now shows,
and the path forward: keep the change and take it back by a pull request -- to the branch the pinned commit came
from, or elsewhere -- or toss the branch. The pull request's target is the developer's choice, not only the
default branch.

## Defaults, recorded rather than ruled

- **Snapshots are state, not cache,** as [#846](https://github.com/NobleFactor/devlore-cli/issues/846) already
  says; each is `git worktree lock`ed with its lifetime as the reason.
- **One snapshot per layer per run.** Sharing one snapshot per commit across lifetimes was withdrawn: with writable
  snapshots, a redeploy at the same commit would inherit another lifetime's edits.
- **The branch name is dated to the minute, in UTC:** `retain/<host>/<yyyy-mm-ddThhmmZ>`, so two retains on one host
  in one day do not collide.
- **The snapshot stays detached at the retain commit,** never checked out on the branch. Git refuses to delete a
  branch checked out in any worktree, so otherwise the developer could not toss it.
- **Further edits go onto the same retain branch** as new commits while it is unmerged; reconcile recognizes an
  edit already retained and cuts nothing new.
- **Copies have no snapshot to commit into.** A rendered template or a decrypted secret cannot give its edit back
  to its source; for a changed copy, "always preserve" means archiving the edited file to the recovery site before
  any restore, the mechanism the replace policy already uses. Retain branches apply to links.
- **Pruning or rebuilding a snapshot refuses one that holds an edit not yet retained.**

## The lifecycle, as ruled

```
 deploy ──► an edit ──► reconcile ─────────────────────► you ────────────────► upgrade
   │          │           │                                 │                     │
 pin C,     lands in    cuts retain/<host>/<date> from C,   toss the branch, or   pins what the checkout
 snapshot   the         commits the edit; restores the      open a PR and merge   now holds; a new
 @C, links  snapshot    record only if asked, by a flag     it                    snapshot, links follow
 into it    (writable)                                                            (ruling still open)
```

## Still to rule

The owner asked for the count: five remain.

1. **Upgrade and links.** Whether upgrade re-points links to the new snapshot as well as refreshing copies. Revises
   lane 11 ([#928](https://github.com/NobleFactor/devlore-cli/issues/928), "the stale copies only");
   [#845](https://github.com/NobleFactor/devlore-cli/issues/845) overlaps.
2. **Writing into the owner's clones.** Refs, worktree registrations and retain branches in clones registered by
   path: an exception to [#792](https://github.com/NobleFactor/devlore-cli/issues/792)'s rule that writ never
   touches them.
3. **The words and the exit code against the ref.** `stale` becomes "the checkout moved past the record", an
   upgrade opportunity rather than drift; a copy whose working-tree source is gone is restorable from the ref; an
   edit and a retained edit need words. Revises lane 4 ([#756](https://github.com/NobleFactor/devlore-cli/issues/756))
   and lane 2's words ([#923](https://github.com/NobleFactor/devlore-cli/issues/923)).
4. **#847's per-layer live policy.** #847 proposed that `personal` stay live, its links at the working tree. The
   rulings above keep snapshots for every layer, and writable snapshots with retain branches serve the authored
   layer too; whether live survives is open. Found while writing this document.
5. **The replan.** Where this lands on #916. Lane 5 (#924) waits on it.

Also to place with the replan: `docs/guides/development-process.md` in noblefactor-ops needs an exception for
machine-originated retain branches, which no issue tracks.

## What the rulings change elsewhere

| Where | Change |
| --- | --- |
| #924, lane 5 | stopped 2026-09-27. Its open question 3, the restore precondition, is superseded: a restore read from the ref always reproduces the record. Its "no reverse sync" reason lapses for links, which retain branches now carry back. Its no-command-flags point is broken for restore |
| #762 Requirement 2 | "no command flags" superseded in part: restore is a flag |
| `docs/architecture/5.1-reconciliation.md`, `10-command-line-interface.md` §3.1 | dated notes, 2026-09-27 |
| #844, #845, #846 | prerequisites, as #847 lists them: upgrade pins, upgrade's whole job, snapshots out of the cache |

## Related Documents

- [#847](https://github.com/NobleFactor/devlore-cli/issues/847) -- the feature this widens
- [924-reconcile-repair.md](../task/924-reconcile-repair.md) -- lane 5, stopped on this
- [5.1-reconciliation.md](../../architecture/5.1-reconciliation.md)
- [10-command-line-interface.md](../../architecture/10-command-line-interface.md)
