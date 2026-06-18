---
step: 23
title: "Row-4 eager-getter projection (cmd/star/star)"
status: complete — REGRADE from not-started; deliverable landed and directly proven
proof_run: 2026-06-17
parent: ../../phase-8.md
---

# Step 23 — Row-4 eager-getter projection (`cmd/star/star`)

**Status:** **`complete`** — regraded from `not-started → scoped`. The eager-property-projection deliverable has landed,
is directly tested, and the reds this step was gated on are green.

## What this step delivers

The reflection `goReceiver` surfaced every zero-arg getter as a **callable** builtin, while the `.star` consumers and
the [3.3 static-codegen](../../../architecture/3.3-static-starlark-codegen.md) contract read them as **eager
properties** (`config.get`, `ast.package_name`). The fix (decided 2026-05-31): an opt-in per-method `+devlore:property`
signal honored by the bridge projection, so scripts pass unedited. Gate: the 9 `TestLintCopyright_*` cases and
`TestSourceFile_StarlarkIntegration` (`cmd/star/star`) must be green before phase-8 closes.

## Evidence — landed and proven

| Layer | Evidence |
|---|---|
| Framework signal | `pkg/op/method.go:780-789` — `MethodModifiers` bit set; `ModifierProperty = 1 << 0`, "marks a zero-arg getter for property projection," set from a `+devlore:property` directive, valid only on zero-arg value-returning methods. `Method.Modifiers()` accessor at `:362`. |
| Bridge projection | `pkg/op/starlarkbridge/go_receiver.go:209,229,232` — a method tagged `op.ModifierProperty` "projects as an eager property: attribute access invokes the zero-arg method," instead of binding a callable builtin. |
| Codegen emits it | `cmd/star/provider/config/gen/provider.gen.go:20` (`"Get": {… Modifiers: op.ModifierProperty}`); `cmd/star/provider/goast/gen/func_decl.gen.go:19` (`"DeclKind": {… Modifiers: op.ModifierProperty}`). |
| Providers declare it | `cmd/star/provider/config/provider.go:51` (`+devlore:property` on `Get`); `cmd/star/provider/goast/source_file.go:660,827,907`. |
| **Direct bridge test** | `pkg/op/starlarkbridge/go_receiver_test.go:20-106` — a method announced with `op.ModifierProperty` must project as an eager property (Attr yields the **call result**, not the callable); a plain method projects as a callable builtin. Both arms asserted. |
| **Gated reds now green** | 2026-06-17 clean-tree `make test`: `cmd/star/star` is `ok`. `TestSourceFile_StarlarkIntegration` (`sourcefile_integration_test.go:116`) asserts `ast.package_name != "example"` — the eager-getter (no-parens) form — and passes; the 9 `TestLintCopyright_*` pass. |

## Disposition / grade

`complete`. The eager-getter projection is implemented end-to-end (signal → codegen → bridge), directly unit-tested at
the bridge, and the integration reds the row was gated on are green. The 2026-05-27 inventory that listed these as
phase-8 reds (and step 18's stale "21.2" sub-step) is obsolete.

Two notes, surfaced not acted on:
1. The sub-plan [phase-8/eager-property-projection.md](../eager-property-projection.md) still carries
   `status: in-progress` — stale for the bridge box (box 2), which is done; it may still track adjacent lore-command
   rewrite boxes outside this row's scope.
2. Row 22's own deliverable is complete, but the broader phase-8 **PR gate** (full `make test` green, owned by steps
   20/23) remains unmet via step 18 — a separate gate, not a reopening of this row.
