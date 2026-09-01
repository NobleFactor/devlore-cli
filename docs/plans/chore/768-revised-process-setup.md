---
title: "Set up to execute four threads under the revised process"
issue: https://github.com/NobleFactor/devlore-cli/issues/768
status: in-progress
created: 2026-09-01
updated: 2026-09-01
---

# Plan: Set up to execute four threads under the revised process

## Summary

Work is now organized as four threads, executed in order, one open worktree at a time. Before the first of
them starts, every thread needs a current plan, and one status document carries a tick that is wrong. This
chore lands those documents. No code changes.

## Goals

1. **Every thread has a current plan** stating where it stands and what remains.
2. **The stale tick is corrected**, and the correction is recorded rather than quietly applied.
3. **Thread 4 gets the plan it has never had**, including the configuration finding that thread 3's
   migration note depends on.

## Current State

| Thread | Plan | State before this chore |
| --- | --- | --- |
| 1 — [#740](https://github.com/NobleFactor/devlore-cli/issues/740) output conventions | [cli-output-conventions.md](../cli-output-conventions.md) | Current through 2026-08-30; no statement of where the epic stands |
| 2 — resource management, [#625](https://github.com/NobleFactor/devlore-cli/issues/625) | [sealed-provider-resources.md](../sealed-provider-resources.md) | `status: approved`, unchanged since 2026-08-24; five phases had landed since |
| 3 — [#762](https://github.com/NobleFactor/devlore-cli/issues/762) writ lifecycle | [feature/762-lifecycle-scopes.md](../feature/762-lifecycle-scopes.md) | Current; phase 1 closed by #767 |
| 4 — [#441](https://github.com/NobleFactor/devlore-cli/issues/441) configuration | none | No plan; scheduled nowhere |

`10-command-line-interface.status.md` carried "`writ` consumes the set it registers" as unchecked after
[#753](https://github.com/NobleFactor/devlore-cli/issues/753) and
[#754](https://github.com/NobleFactor/devlore-cli/issues/754) both closed in PR #747 — the document
reported landed work as outstanding.

## Requirements

### Requirement 1: The revised process, written down upstream

The process itself is recorded in `noblefactor-ops`
([#133](https://github.com/NobleFactor/noblefactor-ops/issues/133),
`docs/guides/development-process.md`), not here, because it governs every repository. That work is
upstream of this chore and completes its own cycle separately.

Its rules, which this chore is the first work to follow:

- One open worktree at a time; more than one issue may be resolved in it; no pull request until every
  issue in that worktree is resolved.
- Issues are logged on discovery, and where each is resolved is decided at that moment.
- An issue blocks when we cannot proceed without it *and* it is orthogonal or large enough to be its own
  work.
- The plan is committed before the work begins, and every commit updates every document it touches.

### Requirement 2: Thread order, stated once

1. **#740** — one output convention, every app.
2. **Resource management** — `Epic:ResourceModel`, #625 at 5 of 10 phases, #644 next.
3. **#762** — the writ lifecycle surface, phases 2–4.
4. **#441** — unified configuration.

Threads 1 and 3 were entangled: #762 renames the package thread 1's next phase rewrites. They are now
separated, thread 1 goes first, and `git mv` still records a clean rename afterwards.

One dependency runs the other way: thread 2's judgment scenario 2 needs a drivable reconcile surface,
which is thread 3's phase 2. Recorded in both plans; not a reordering.

## Implementation Phases

### Phase 1: The documents (status: in progress)

- [x] `docs/plans/feature/441-unified-configuration.md` — thread 4's plan, new
- [x] `docs/architecture/10-command-line-interface.status.md` — the stale tick corrected, with the
      correction recorded in place
- [x] `docs/plans/cli-output-conventions.md` — a "Where we are" section: four items remaining, three
      needing issues, the intended slicing, and the thread order
- [x] `docs/plans/sealed-provider-resources.md` — a "Where we are" section: five of ten phases landed,
      every remaining phase's issue, the thread's other open work, and the four items outstanding on
      `4-resource-management.status.md`
- [x] This plan

**Files**:

- `docs/plans/chore/768-revised-process-setup.md` - Create
- `docs/plans/feature/441-unified-configuration.md` - Create
- `docs/architecture/10-command-line-interface.status.md` - Modify
- `docs/plans/cli-output-conventions.md` - Modify
- `docs/plans/sealed-provider-resources.md` - Modify

## Test Plan

No code. The claims are verifiable by search and by issue state.

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | The corrected tick reflects reality | issue state | #753 or #754 is not closed |
| 2 | Every remaining phase of #625 has an issue | issue state | One of #644–#649 does not exist or is closed |
| 3 | The unknown-key finding is accurate | search | `configuration.md` no longer says unknown keys are reported, or the status document's Go-typed-path box is ticked |

**Not covered:** whether the thread order survives contact with the work. It is a plan, and the first
thread that disproves it will say so in its own plan.

## Migration Path

None. Documents only.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/chore/768-revised-process-setup.md` | Create | This plan |
| `docs/plans/feature/441-unified-configuration.md` | Create | Thread 4's plan |
| `docs/architecture/10-command-line-interface.status.md` | Modify | Correct the stale tick |
| `docs/plans/cli-output-conventions.md` | Modify | Where thread 1 stands |
| `docs/plans/sealed-provider-resources.md` | Modify | Where thread 2 stands |

## Related Documents

- [cli-output-conventions.md](../cli-output-conventions.md) — thread 1
- [sealed-provider-resources.md](../sealed-provider-resources.md) — thread 2
- [feature/762-lifecycle-scopes.md](../feature/762-lifecycle-scopes.md) — thread 3
- [feature/441-unified-configuration.md](../feature/441-unified-configuration.md) — thread 4
- [10-command-line-interface.status.md](../../architecture/10-command-line-interface.status.md)
- [4-resource-management.status.md](../../architecture/4-resource-management.status.md)
- noblefactor-ops [#133](https://github.com/NobleFactor/noblefactor-ops/issues/133) — the process guide

## Open Questions

- [ ] Three of thread 1's four remaining items have no issue. They are filed when that thread starts, not
      here, so this chore does not open issues it will not resolve.
