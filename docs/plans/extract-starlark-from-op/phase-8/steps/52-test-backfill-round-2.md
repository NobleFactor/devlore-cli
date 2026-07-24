---
step: 52
title: "Test backfill, round 2 — direct coverage for the gaps the step-39 matrices surface"
status: tests + fixtures landed & verified 2026-07-24 — suite green (0 FAIL); 4 fixtures re-dispositioned; per-provider status-doc gap-cell sweep is the final Exit step
parent: ../../phase-8.md
---

# Step 52 — Test backfill, round 2

**Status:** tests + fixtures landed & verified 2026-07-24 (suite green, 0 FAIL); the per-provider status-doc gap-cell
sweep is the final Exit step (chartered 2026-07-22). See the Disposition below. The successor to
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

From **3.5.9 powershell** ([status](../../../../architecture/3.5.9-powershell-provider.status.md)) — the only
provider with zero coverage of any kind:

11. `powershell.Exec` Go units — success / failure / empty-command (the shell trio's analogs), `pwsh`-gated with a
    skip guard on machines without it.
12. A `powershell.exec` fixture (the `test_shell_exec.star` analog), same gating.

From **3.5.7 template** ([status](../../../../architecture/3.5.7-template-provider.status.md)):

13. `template.render_bytes` fixture coverage — `RenderBytes` has one Go unit but no fixture of its own; coverage
    currently rides `RenderText`'s fixtures.

From **3.5.10 git** ([status](../../../../architecture/3.5.10-git-provider.status.md)):

14. `git.Checkout` / `git.Pull` dedicated units — both are fixture-covered only (`test_git.star`); the command-hook
    seam `Clone` uses supports argv/error assertions for them too.
15. `git.Observe` — no unit and no fixture (HEAD / ref / bare / dirty / remotes fact assertions).

From **3.5.12 appnet** ([status](../../../../architecture/3.5.12-appnet-provider.status.md)):

16. An `appnet.download` fixture — the action has 4 Go units but no `.star` coverage.

From **3.5.13 encryption** ([status](../../../../architecture/3.5.13-encryption-provider.status.md)):

17. `CompensateEncryptFile` dedicated units — the encrypt side has a round-trip but no compensation-path analogs of
    the decrypt side's three (`RemovesFile` / `EmptyPath` / `MissingFile`).

From **3.5.14 function** ([status](../../../../architecture/3.5.14-function-provider.status.md)):

18. `function.Call` dedicated units — invocation is exercised only through lambda-bodied combinator fixtures;
    direct units for arg/kwarg passing, error propagation, and a non-callable resource are absent.

**Not double-ledgered** (tracked by their own steps, per the step-24 rule): choose's end-to-end reload replay
(step 10), the public-API pause-mid-combinator resume test (step 31 outstanding item 2 / step 36's affordance).

Later step-39 docs append their gaps here as they land.

## Disposition (closed 2026-07-24) — suite green, 0 FAIL

**Delivered — Go units** (direct, per the step-24 bar; test names greppable in each file):

- 1–3 plan → `plan/provider_units_test.go` (6 units: 4 `Plan` error-door + `Origin` + `Clear`).
- 4–7 file → `file/provider_backfill_test.go` (15 units: WalkTree(+Compensate), Name/Parent/Root, Move, RemoveAll).
- 8 file observe, Go half → `file/observe_fields_test.go` (`TestObserve_ReportsFileFields` — Size/Mode/ModTime,
  beyond the existing Exists-only unit).
- 11 powershell → `powershell/provider_test.go` (3 units, `pwsh`-gated via `requirePwsh`).
- 14–15 git → `git/checkout_pull_observe_test.go` (10 units; Checkout/Pull argv asserted via the dry-run narration
  seam — they have no argv hook, unlike Clone).
- 17 encryption → `encryption/provider_test.go` (3 `CompensateEncryptFile` units mirroring the decrypt analogs).
- 18 function → `function/provider_test.go` (3 `Call` units).

**Delivered — fixtures** (+ their Go runners in `devloretest/backfill_test.go` — fixtures run only via an explicit
per-file Go test, not by glob):

- 9 file find → `data/test_file_find.star` + `TestFileFind`.
- 13 template render_bytes → `data/test_template_render_bytes.star` + `TestTemplateRenderBytes`.

**Re-dispositioned — the `.star` harness cannot stage these:**

- 8 observe *fixture*: `Observe` consumes a resolved `Entry`, not a path — no clean `.star` construction. The Go unit
  above covers it.
- 10 wait_until fixtures: both edges are already proven at the executor level (`TestWaitUntil_MatchAfterNPolls`,
  `TestWaitUntil_BodyErrorFailsImmediately`); match-after-N needs mutable per-poll state Starlark fixtures lack, and
  body-error is redundant with `test_flow_fatal.star`.
- 12 powershell / 16 appnet fixtures: the harness has **no fixture-level skip**, so a `pwsh`- or network-dependent
  fixture would break `make test` on machines/CI without them. The gated Go units (11) cover powershell; appnet already
  has 4 Go units.

## Exit

All intaken rows delivered (or explicitly re-dispositioned with reasons), test names greped at close, `make test`
green, and every referencing status doc's gap cell updated to name the landed test.

## Relationship to other steps

- **Step 24** — the precedent and the bar; closed, never reopened.
- **Step 39** — the intake source; each new provider doc's matrix feeds this list.
- **The PR to develop** — this step closes before it (the PR gate itself — full `make test` green — is already met
  and unaffected by adding tests).
