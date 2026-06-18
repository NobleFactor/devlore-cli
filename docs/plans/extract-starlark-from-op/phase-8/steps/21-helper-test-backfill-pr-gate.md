---
step: 21
title: "Framework-helper direct-test backfill + phase-8 PR gate"
status: not-started — confirmed (no backfill tests; makeMethod still synthetic); PR gate unmet
proof_run: 2026-06-17
parent: ../../phase-8.md
---

# Step 21 — Framework-helper direct-test backfill + phase-8 PR gate

**Status:** `not-started` for the deliverable; the **PR gate it owns is unmet** (full `make test` is not green — see
[step 18](18-resolve-test-failures.md)).

## What this step delivers

Two paired outcomes:

1. Close the direct-test gap that steps 15/16 flagged: Phase 6.0's convertibility helpers, step-16's
   `checkPromiseTypes`, and the pre-existing `Method.ResultType` all landed with **zero direct unit tests**, relying on
   indirect `.star` coverage.
2. Extend the `pkg/op/validate_test.go` helper surface so those direct tests are writable — chiefly extend `makeMethod`
   to construct a **real `reflect.Method`** over mock Go provider types, so `Method.ResultType` is exercisable without
   the receiver-registry plumbing.

Named test families: `TestValidateGraph_CheckPromiseTypes_{Match,Mismatch,MissingProducer,NoMethod,NoParameter}`,
`TestTypesAreInterconvertible_{Identity,Assignability,SourceConverter,TargetConverter,Incompatible,NilSafeProbe}`,
`TestSubgraph_MergeBubbled_{Convertible,PreferSourceSide,IrreconcilableTypes}`,
`TestMethod_ResultType_{FirstReturn,ErrorOnly,NoOutput,Compensable}` (~15–20 functions).

## Evidence — not started

- A tree-wide `grep` for `TestValidateGraph_CheckPromiseTypes`, `TestTypesAreInterconvertible`,
  `TestSubgraph_MergeBubbled`, `TestMethod_ResultType` returns **zero** hits — none of the named tests exist.
- `makeMethod` (`pkg/op/validate_test.go:27`) still returns a **synthetic** `&Method{parameters: params}` built from
  hand-written `Parameter` specs — no `reflect.Method`. The substep (i) extension has not happened, so
  `Method.ResultType` remains directly untestable through this helper.
- This is the same gap steps 15 and 16 recorded: `op.typesAreInterconvertible` has no direct test (step 15), and
  `checkPromiseTypes` has no direct test (step 16). Step 20 is where both get closed; neither has been.

## PR gate — unmet

Step 20 carries the phase-8 PR gate: not PR-eligible to `develop` until the full `make test` suite is green. The
2026-06-17 clean-tree run is **10 packages red** (7 build failures + 4 test reds; see step 18). The gate is unmet, and
feeds the cross-phase demo-milestone criterion 16 (full `make test` green).

## Disposition / grade

`not-started` — accurate. Direct-test backfill absent; `makeMethod` unextended; PR gate unmet (gated on step 18).
