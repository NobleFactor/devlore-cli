---
step: 52
title: "Test backfill, round 2 — direct coverage for the gaps the step-39 matrices surface"
status: not-started — chartered 2026-07-22; intake OPEN while step 39 runs; execute before the PR to develop
parent: ../../phase-8.md
---

# Step 52 — Test backfill, round 2

**Status:** `not-started` — chartered 2026-07-22. The successor to
[step 24](24-helper-test-backfill-pr-gate.md) (closed 2026-07-18 with its enumerated scope delivered — it does not
reopen): as step 39 writes each provider's verified test matrix, the gaps it surfaces are **intaken here** rather
than left scattered in status docs. The intake stays open while step 39 runs; the execution slice lands the tests
and closes before the PR to develop.

## The bar

Step 24's standard: **direct** tests at the method (not only end-to-end fixture coverage), named per contract,
existence greped at close. Rationale — failure localization, edge cases fixtures cannot stage, contract pinning
(see the step-24 record). Per-provider status docs keep their matrix rows; this step is the executable intake.

## Intake (accumulating; sources named)

From **3.5.3 plan** ([status](../../../../architecture/3.5.3-plan-provider.status.md)):

1. `Plan` error paths — unknown action name, malformed kwargs (the Go door's rejections).
2. `Origin` — a dedicated unit (scope stamping).
3. `Clear` — a dedicated unit (ledger reset observable via `InvocationRegistry`).

From **3.5.4 file** ([status](../../../../architecture/3.5.4-file-provider.status.md)):

4. `WalkTree` Go units — the fold contract (entry order, `include_gitignored`), an erroring `fn` mid-walk (stack
   holds only completed entries' receipts), and `CompensateWalkTree` incl. nil / empty stack (the analog every flow
   combinator companion already has).
5. `Name` / `Parent` / `Root` units (path algebra; `Join` has 3).
6. `Move` forward-path units (the census shows 5 `TestCompensateMove_*` but a thin forward set).
7. `RemoveAll` — beyond the single existing unit (subtree archival + restore).
8. An `Observe` fixture (no `.star` coverage) and broader field assertions beyond the single Go unit.
9. A dedicated `find` fixture (`file.find` is fixture-uncovered; `glob` has one).

From **3.5.2 flow** ([status](../../../../architecture/3.5.2-flow-provider.status.md), re-verified 2026-07-22):

10. `.star` fixture variants of WaitUntil's edge rows — match-after-N and body-error are proven at executor level
    (`TestWaitUntil_MatchAfterNPolls`, `TestWaitUntil_BodyErrorFailsImmediately`); no fixture-level variants exist.

**Not double-ledgered** (tracked by their own steps, per the step-24 rule): choose's end-to-end reload replay
(step 10), the public-API pause-mid-combinator resume test (step 31 outstanding item 2 / step 36's affordance).

Later step-39 docs append their gaps here as they land.

## Exit

All intaken rows delivered (or explicitly re-dispositioned with reasons), test names greped at close, `make test`
green, and every referencing status doc's gap cell updated to name the landed test.

## Relationship to other steps

- **Step 24** — the precedent and the bar; closed, never reopened.
- **Step 39** — the intake source; each new provider doc's matrix feeds this list.
- **The PR to develop** — this step closes before it (the PR gate itself — full `make test` green — is already met
  and unaffected by adding tests).
