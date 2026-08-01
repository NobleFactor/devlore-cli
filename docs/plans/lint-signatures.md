---
title: "Class c: signatures — named results adopted, unparam retired"
issue: TBD
status: complete
created: 2026-08-01
updated: 2026-08-01
---

# Plan: Class c — signatures

Third rung of the 4b-3 ladder (rulings 2026-07-30/08-01: adopt named results; noctx stays
inline for a later rung; per-class discuss-then-fix).

## unnamedResult: 31 → 0 (+2 discovered)

Every flagged signature adopts result names drawn from its documented `Returns:` bullets
(or documented prose where a helper had no bullets) — e.g. `callerFrame(skip) (function,
file string, line int)`, `canonicalize(data) (canonical []byte, parsed any, err error)`,
`Subscribe() (events <-chan ControlEvent, cancel func())`. No naked returns introduced.
Body collisions (locals shadowing new result names) resolved by assignment or renames —
notably `Subscribe`'s bidirectional channel local (`ch`) versus its receive-only result,
and `sopsEncrypt`'s age-identity object renamed `ageIdentity` so the `identity` string
result keeps the documented name. Two same-shape siblings discovered mid-sweep
(`itemProduction.Execute`, `assignSlots`) named as well.

## unparam: 15 → 0 (12 removals, 1 constant-inline, 2 suppressions)

- Dead parameters removed with caller updates: `makeStubParagraph`'s `elem`;
  `resolvePayloadAction` and `subgraphFromInvocations` each lose an unused `env` (killing
  two banned identifiers), which cascaded to `assembleNode`/`assembleSubgraph` losing
  theirs.
- Always-nil/constant results removed: `classifyFloatingComment` (error),
  `formatCheck` (error), `parseStatusConfig` (error), `deployFixture` (`sourceRoot`);
  `genDeclNodeType` (constant "GenDecl") deleted outright, caller inlines the literal.
- Constant parameters inlined: `locate`'s `name` (→ `sopsConfigName`) and, by cascade,
  `xdgRelPath` (→ `xdgFallbackRelPath`); test-helper constants `wantPassed`,
  `resolveErr`, `slots`, `id` dropped at their definitions and callers.
- Suppressed with stated reasons: `assert.raise`'s `skip` (the stack-depth contract —
  hardcoding would break the first two-level helper) and `newPlanBuilder`'s always-nil
  error (its own doc records the intent).
- Drive-by per the class-1 ruling: two flag getters under bare
  `//nolint:errcheck // flag registered` converted to `assert.Must` (config.go).

## Open structure question (awaiting ruling)

`splitReservedKwargs` returns seven values; gocritic's tooManyResultsChecker wants a
carrier struct. Folding the five reserved kwargs into a struct is a structure decision
(not made unilaterally) — suppressed with a pointer to this doc until ruled.

## Verification

- gocritic 0 (was 31 + strays), unparam 0 (was 15), goimports/gofmt/whitespace 0.
- Remaining ladder: gosec 56 (d), revive 28 (e), noctx 14, unused 11 (f), complexity 61
  (chartered). Total 170.
- `make vet` pass; full `make test` pass (one live-caught regression during the sweep —
  a global rename briefly collapsed test-node IDs — found by the suite, fixed, re-run
  green); `gofmt -l` clean.
