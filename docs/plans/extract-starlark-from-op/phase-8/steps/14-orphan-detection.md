---
step: 14
title: "Orphan detection at plan-end — unattached invocations fail in plan.assemble"
former_step: 17
former_title: "Orphan detection at plan-end"
status: incomplete — mechanism present and correct; the detection path has no test
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 14 — Orphan detection at plan-end (formerly step 17)

**Status:** `incomplete`. The row labels this `complete (2026-05-24)`. The mechanism is implemented and looks correct,
and its **negative path runs in every test** — but **no test exercises the actual deliverable** (an orphan producing an
error). The row overstates: detection is unproven.

## What this step delivers

Per the design Goal ("anything the author constructs but doesn't attach fails at plan time as an orphan"), the check
lives in `plan.Provider.Assemble` (`pkg/op/provider/plan/provider.go:205–215`):

```go
var orphans []error
for _, invocation := range p.invocations.All() {
    if invocation.Target.ParentID() == "" {
        orphans = append(orphans, fmt.Errorf(
            "orphan invocation %q (target %q has no parent)", invocation.Label, invocation.Target.ID()))
    }
}
if len(orphans) > 0 {
    return nil, errors.Join(orphans...)
}
```

The equivalence to "walk from root, mark reachable" holds because every `AddChild` stamps a `parentID` on the added unit
(`executable_unit.go:187` — including `error_action=` assignments), so empty-`parentID` == "never rooted." Well
documented at `provider.go:127/144–145` and `validate.go:171` (orphan + bubble-up + type errors aggregate together).

## Test matrix

| # | Test | Proves | Grade |
|---|---|---|---|
| — | (53 `.star` fixtures) | the **no-orphan** path — every valid graph passes the scan without error | ✅ incidental |
| — | `TestAssemble_OrphanInvocation_Errors` / `test_orphan_*.star` | **THE DELIVERABLE** — a constructed-but-unattached invocation makes `Assemble` return the `"orphan invocation … has no parent"` error | ☐ unwritten |

**Coverage of the detection path: 0.** No Go test (`go test -run Orphan ./pkg/op/...` → "no tests to run"; the only
`Orphan` tests are `cmd/writ/.../reconcile_test.go`, an unrelated deleted-symlink `StateOrphan`). No `.star` fixture
constructs an orphan — the `expect_error` fixtures assert type-mismatch, variadic, fatal, and copy errors only.

## The harness already supports the missing test

`runner.go:337` documents the plan-time-validation-only path: a script may build a graph and assert a plan error via
`t.expect_error(pattern)` (`test_context.go:368`) without calling `t.run`. A fixture that builds an invocation, never
attaches it, calls `plan.assemble`, and asserts `t.expect_error("orphan invocation")` is the one-file gap.

## To reach `complete`

1. Add `test_orphan_unattached.star`: construct e.g. `plan.file.mkdir(...)`, do **not** include it in any
   `plan.subgraph(body=…)` / `plan.gather(body=…)` / the `plan.assemble` root set, call `plan.assemble([…])`, and assert
   `t.expect_error("orphan invocation")`.
2. (Optional) A Go-level `TestAssemble_OrphanInvocation_Errors` against `plan.Provider` for a registry-only unit test.
