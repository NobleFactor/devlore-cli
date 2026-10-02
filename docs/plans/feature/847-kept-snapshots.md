---
title: "Kept snapshots: a lifetime keeps its layer worktrees, pinned by private refs; links target them; reconcile runs against the ref that produced each entry"
issue: https://github.com/NobleFactor/devlore-cli/issues/847
status: draft
created: 2026-09-27
updated: 2026-09-28
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

**The design rulings are complete (2026-09-28), and the work is placed on #916 as lanes 16 to 19 with lanes 5, 11,
12 and 13 revised (below).** Draft: each lane gets its own plan when its branch opens; nothing is built yet.

## Invariant: the maintainer owns every merge

Stated by the owner 2026-09-28 and written into the design at
[5.1-reconciliation.md](../../architecture/5.1-reconciliation.md), the kept-snapshots note in its header, with a
pointer from [10-command-line-interface.md](../../architecture/10-command-line-interface.md) §3.1. Every ruling below
is held to it, and so is every implementation step: writ never merges, commits to, moves or pushes any branch except
its own `retain/*` branches, so every change that reaches HEAD, the default branch or main is a merge the maintainer
makes, by a pull request.

## Today

Read 2026-09-27 at `5509da4d`.

| Component | Today |
| --- | --- |
| deploy's pin (`cmd/writ/writ/deploy/deploy.go:179`) | each layer pinned to a git worktree snapshot of HEAD under `~/.cache/devlore/snapshots`, removed when the run ends |
| the plan (`cmd/writ/writ/deploy/plan.go:380`, `:531`) | records each layer's `commit_hashes` and `dirty_layers`; a file's `source` is the origin clone's path, its `read_from` the snapshot's |
| links | target the origin clone's working tree, so a pull or an uncommitted edit changes the deployment at once |
| upgrade | pins nothing and records no commit ([#844](https://github.com/NobleFactor/devlore-cli/issues/844)) |
| reconcile (`cmd/writ/writ/reconcile/reconcile.go:196`) | reads the working-tree file at the recorded source path; never reads the recorded commit |

## Rulings, 2026-09-27 and 2026-09-28

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

5. **Upgrade moves the links** (2026-09-28). When the checkout moves from C to D, upgrade pins D and relinks the
   layer's links to the snapshot of D as well as re-rendering copies, so one lifetime shows one commit per layer,
   links and copies agreeing. This revises lane 11 ([#928](https://github.com/NobleFactor/devlore-cli/issues/928),
   "the stale copies only"), as #847 first proposed: "relink every target from the old generation to the new." A
   file new in D waits for deploy; a file gone in D keeps its link on C's snapshot and is reported for deploy.
6. **Writ writes into the owner's clones** (2026-09-28). The refs, retain branches and locked worktree registrations
   live in the registered layer clone, whoever owns it: an exception to
   [#792](https://github.com/NobleFactor/devlore-cli/issues/792), scoped to refs under `refs/devlore/`, branches
   under `retain/` and locked worktree registrations, which writ removes when done with. Writ never touches the
   working tree, the index, HEAD or any other branch. Rejected: a writ-owned mirror of each layer, which keeps #792
   whole at the price of a second place to look and a second hop to a pull request.

7. **The words and the exit code, against the ref** (2026-09-28, accepted as proposed). "When the system matches the
   record, we exit 0; otherwise we exit 1" -- under `--dry-run` too; 66 when never deployed. A plain run restores
   only what it can without discarding anything and preserves the rest. Revises lanes 2 and 4 (#923, #756); it is
   [#972](https://github.com/NobleFactor/devlore-cli/issues/972), lane 18.

   | Word | Means, against the ref | A plain `writ reconcile` | Exit after |
   | --- | --- | --- | --- |
   | `linked` | the link points into the recorded snapshot, and the file is as committed | nothing | 0 |
   | `copied` | the copy holds the recorded content | nothing | 0 |
   | `stale` | as recorded, but the checkout has moved past the recorded commit and this file differs there | nothing; names `writ upgrade` | 0 |
   | `absent` | the target is missing | restores it from the ref | 0 |
   | `dangling` | the snapshot a link points into is gone | rebuilds it from the ref | 0 |
   | `edited` (new) | the file behind a link differs from its commit, or an editor replaced the link with a real file holding the edit | retains it on `retain/<host>/<date>`; restores only with the flag | 1 until tossed, merged and upgraded, or restored |
   | `changed` | another occupant: another link, or a copy whose content moved | archives it to the recovery site; restores only with the flag | 1 until restored |
   | `unpinned` (new) | the pinned ref is gone and its commit unreachable | nothing possible; names `writ deploy` | 1 |
8. **Live is retired** (2026-09-28). Every layer is pinned; links never point at a working tree. "Conceptually, i
   don't see a difference based on repo ownership. keeping personal, team, or base outside our data home should not
   impact behavior ... i don't want personal to be live." #847's per-layer pinned-or-live policy is gone.
9. **The replan** (2026-09-28, reading a): inside #916, the scope lanes first, then kept snapshots, then the revised
   lanes. "If what you mean by (a) is that we get scope right before we deal with sparse checkouts, then i roll with
   (a)." The owner approved splitting the words and exit code into their own lane (18) and closing #473 and #844
   through lanes 17 and 11.

**Amended by the owner:** the sparse checkout covers every scope directory -- `Home`, `System` and, especially on
Windows, `ProgramFiles`, `ProgramData` and whatever `writ.scopes` adds
([#926](https://github.com/NobleFactor/devlore-cli/issues/926)).

**Amended by the owner, 2026-10-02: lifetimes are generations, and the pin belongs to the run** (lane 7 of #916,
[#926](https://github.com/NobleFactor/devlore-cli/issues/926), open question 5; `5.1-reconciliation.md`
§ Lifetimes are generations). A scoped deploy carries the other scopes forward, so one lifetime can hold Home at one
commit of a layer and System at another: `--scope Home` today and `--scope System` a week later pin the layer
wherever its HEAD is each time. So each scope's run records, for each layer, the commit it deployed from. A snapshot
is one per layer and commit, holding the scopes deployed from that commit, not one per layer per lifetime, and
pruning collects a snapshot, as it collects a trace, when no kept lifetime's run refers to it. A deploy without
`--scope` still puts every scope of a layer at one commit. Reconcile already reads each entry against the ref that
produced it (ruling 3).

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

## The lanes, on #916

Approved 2026-09-28. Existing lanes keep their numbers; the order is C, D, E and B′, F.

| Lane | Issue | What | PR | Waits on |
| --- | --- | --- | --- | --- |
| 6, 7, 8 | [#925](https://github.com/NobleFactor/devlore-cli/issues/925), [#926](https://github.com/NobleFactor/devlore-cli/issues/926), [#927](https://github.com/NobleFactor/devlore-cli/issues/927) | scope, `--scope`, the environment prefix | C | -- |
| 16 | [#846](https://github.com/NobleFactor/devlore-cli/issues/846) | snapshots are kept state: the state home, locked, never leaked | D | lane 7 |
| 17 | [#971](https://github.com/NobleFactor/devlore-cli/issues/971) | deploy pins by a private ref and links into a sparse snapshot; closes #473; absorbs #960 | D | lane 16 |
| 11 | [#928](https://github.com/NobleFactor/devlore-cli/issues/928) | upgrade pins its own ref and moves links with copies; closes #844 | E | lane 17 |
| 12 | [#929](https://github.com/NobleFactor/devlore-cli/issues/929) | decommission ends the lifetime; snapshots and refs kept for pruning | E | lane 17 |
| 18 | [#972](https://github.com/NobleFactor/devlore-cli/issues/972) | reconcile reads against the ref: the words and the exit code | B′ | lane 17 |
| 5 | [#924](https://github.com/NobleFactor/devlore-cli/issues/924) | reconcile repairs: restore, retain, the flag, the narration | B′ | lane 18 |
| 13 | [#930](https://github.com/NobleFactor/devlore-cli/issues/930) | pruning, design only: receipts, refs, snapshots; never an unmerged `retain/*` branch | F | lanes 11, 12 |
| 19 | [noblefactor-ops#250](https://github.com/NobleFactor/noblefactor-ops/issues/250) | the development process allows `retain/*` branches | ops | -- |

**#960 folds into lane 17** (agreed 2026-09-28 with the session that held it as schedule #949's lane 15): deploy loses
a removed source precisely because it replaces the lifetime, so deploy's handling of the replaced lifetime's links is
designed once, against snapshots. personal#236 (26 dangling links on DANOBLE-UD24-1) waits on it; the links can be
removed by hand meanwhile, and only reconcile's false clean waits for lanes 17 and 18. One pull request at a time in
`cmd/writ/writ`: that session's #959 first, then lane 6.

The process exception for machine-originated retain branches is lane 19, noblefactor-ops#250.

## What the rulings change elsewhere

| Where | Change |
| --- | --- |
| #928, lane 11 | ruling 5: upgrade moves links as well as copies |
| #792 | ruling 6: an exception, scoped to `refs/devlore/`, `retain/` and locked worktree registrations |
| #924, lane 5 | stopped 2026-09-27. Its open question 3, the restore precondition, is superseded: a restore read from the ref always reproduces the record. Its "no reverse sync" reason lapses for links, which retain branches now carry back. Its no-command-flags point is broken for restore |
| #762 Requirement 2 | "no command flags" superseded in part: restore is a flag |
| `docs/architecture/5.1-reconciliation.md`, `10-command-line-interface.md` §3.1 | dated notes, 2026-09-27 |
| #844, #845, #846 | prerequisites, as #847 lists them: upgrade pins, upgrade's whole job, snapshots out of the cache |
| #846, #971, lanes 16 and 17 | 2026-10-02: a snapshot per layer and commit, the pin recorded per run; deploy's cleanup of a replaced scope's runs (#960) |
| #928, #931, lanes 11 and 14 | 2026-10-02: upgrade and adopt write a new lifetime from the current one's runs, rather than into the current one |
| #929, lane 12 | 2026-10-02: decommission with nothing named removes everything and ends the lifetime, asking first when interactive; narrowed by `--scope` or project names, it writes a new lifetime without what it removed |
| #930, lane 13 | 2026-10-02: pruning collects traces and snapshots by reference |

## Related Documents

- [#847](https://github.com/NobleFactor/devlore-cli/issues/847) -- the feature this widens
- [924-reconcile-repair.md](../task/924-reconcile-repair.md) -- lane 5, stopped on this
- [5.1-reconciliation.md](../../architecture/5.1-reconciliation.md)
- [10-command-line-interface.md](../../architecture/10-command-line-interface.md)
