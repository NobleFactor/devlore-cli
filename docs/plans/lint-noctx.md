---
title: "noctx: every subprocess and request carries a context"
issue: TBD
status: complete
created: 2026-08-02
updated: 2026-08-02
---

# Plan: noctx — every subprocess and request carries a context

Sixth rung of the 4b-3 ladder, per the 2026-07-30 ruling (fix inline; mechanical plumbing
now, lifetime refinement later if needed). Twenty sites, sorted per the codebase's own
discipline: derive from the dispatch at hand, take the session context at edges, and say
so explicitly where only Background exists.

## The sorting

1. **Immediate-provider helpers (3)** — lint's `checkModTidy` gains a `ctx` parameter and
   setup's `PrecommitInstall` threads the session context: `RuntimeEnvironment.Context`,
   nil-guarded to Background for bare test-built environments.
2. **writ snapshot git helpers (5)** — `IsDirty` and the four `git worktree` helpers take
   `ctx` first; internal callers (`Pin`, `Close`, `CheckClean`, `verifyWorktree`) pass
   Background explicitly — that IS the current lifetime of these synchronous CLI paths,
   now stated rather than implied. Threading real command contexts through the exported
   snapshot API is the deferred refinement.
3. **Tests (12)** — server router/integration requests carry `t.Context()` (cancelled at
   test end); snapshot-test git fixtures likewise; devlore-test's two sites run where no
   `*testing.T` exists (`TestMain`, the shared `run` helper) and use Background with a
   comment saying exactly that.

## Verification

- noctx 20 → **0** uncapped; `make vet` and full `make test` pass; `gofmt -l` clean.
- Board after this rung: unused 11 (class f) + chartered complexity (gocognit 52,
  gocyclo 9).
