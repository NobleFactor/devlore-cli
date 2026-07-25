---
step: 14
title: "Orphan detection at plan-end — unattached invocations fail in plan.assemble_definition"
former_step: 17
former_title: "Orphan detection at plan-end"
status: complete — behavioral tests landed 2026-07-03 (detection path proven in Go and .star)
proof_run: 2026-07-03
parent: ../../phase-8.md
---

# Step 14 — Orphan detection at plan-end (formerly step 17)

**Status:** `complete`. The mechanism was already implemented and correct; the deliverable — an orphan actually
producing the error — is now proven from both APIs.

## What this step delivers

Per the design Goal ("anything the author constructs but doesn't attach fails at plan time as an orphan"), the check
lives in `plan.Provider.AssembleDefinition` (`pkg/op/provider/plan/provider.go:205–215`):

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

The equivalence to "walk from root, mark reachable" holds because every `AddChild` stamps a `parentID` on the added
unit (`executable_unit.go:187` — including `error_action=` assignments), so empty-`parentID` == "never rooted."

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail · — not gradable (unwritten).

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| — | (53 `.star` fixtures) | the **no-orphan** path — every valid graph passes the scan without error | ☑ | ✅ incidental |
| 1 | `TestAssembleDefinition_OrphanInvocation_Errors` (`pkg/op/provider/plan/provider_test.go`) | **THE DELIVERABLE**, Go API — two planned invocations, one rooted: `AssembleDefinition` returns nil graph and an error containing `"orphan invocation"` and the orphan's label | ☑ | ✅ |
| 2 | `test_orphan_unattached.star` (registered as `TestOrphanUnattached`, `runner_test.go`) | the same from `.star` — `t.expect_error("orphan invocation")` over the plan-validation-only path (`runner.go:337`): the script plans two `plan.file.mkdir` invocations, roots one, and never calls `t.run` | ☑ | ✅ |

**Coverage of the detection path: 2 / 2** (the step doc's chartered names `TestAssemble_OrphanInvocation_Errors` /
`test_orphan_*.star` realized under the current method name `AssembleDefinition`).

## Proof run

Verified 2026-07-03: `pkg/op/provider/plan` and `cmd/devlore-test/devloretest` pass under `make test` with both tests
present. The fixture rides the harness path the 2026-06-16 audit identified as ready (`runner.go:319` tolerates a
script error when an error expectation exists; `buildResult` → `tc.Check` matches the pattern against the raised
assemble error).
