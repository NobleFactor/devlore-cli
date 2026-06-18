---
step: 24
title: "ActivationRecord-first invariant — codegen-enforced (hard exit gate)"
status: not-started — confirmed (optional/detected model still in place; nothing of the mandate/enforce deliverable exists)
proof_run: 2026-06-17
parent: ../../phase-8.md
---

# Step 24 — ActivationRecord-first invariant (hard exit gate)

**Status:** `not-started`. The row's label is accurate. The codebase runs the **optional, detected** activation model
that this step proposes to **replace** with a **mandatory, codegen-enforced** one. None of the three deliverable pieces
(codegen rejection · always-inject · discrimination removal) exists.

## What this step delivers

Every announced provider method MUST declare `*op.ActivationRecord` as its first parameter (after the receiver).
Codegen rejects, with a compile-time error, any provider method whose first parameter is not `*op.ActivationRecord`.
Because activation then becomes uniformly present, the `firstParamIsActivation` / `undoFirstParamIsActivation`
discrimination in `pkg/op/method.go` collapses away (closing the `TODO(david-noble)`), and the bridge always injects
the activation.

## Evidence — not started

| Deliverable piece | Current state |
|---|---|
| Discrimination removed | **Present, not removed.** `firstParamIsActivation` / `undoFirstParamIsActivation` fields at `pkg/op/method.go:64-65`; computed at `:111` (`methodType.In(1) == activationRecordType`); the conditional `if m.firstParamIsActivation { goArgs = append(goArgs, activation) }` at `:508`, and Undo's at `:469`. The `TODO (david-noble) Get rid of firstParamIsActivation and undoFirstParamIsActivation` is live at `:50-51`. |
| Always-inject | **No.** Injection is conditional on `firstParamIsActivation` (`method.go:508`) — methods without the param get no activation. |
| Codegen / registration rejection | **No.** `pkg/op/receiver_type.go:400-404` *detects-and-skips* a leading `*ActivationRecord` (`NumIn() >= 2 && In(1) == activationRecordType`) and tolerates both shapes — detection, not enforcement. No codegen pass rejects a non-conforming method. |
| Methods conform | **No.** Getters and pure utilities carry no leading activation param: `file.Root()` (`provider.go:61`), `file.Exists(resource *Resource)` (`:907`), `file.IsDir(resource *Resource)` (`:1059`), `file.Join(parts ...string)` (`:1185`), `file.Name(path string)` (`:1196`), `file.Parent(path string)` (`:1207`). The row's intent that these "gain a leading `*op.ActivationRecord` they ignore" is unapplied. |

The compensation-companion path (`method.go:271-301`) likewise still classifies two shapes (1-param vs.
`*ActivationRecord`+complement) rather than mandating the activation-first shape.

## Disposition / grade

`not-started` — accurate. The current mechanism is precisely the optional/detected design the invariant is meant to
supersede; the codegen rejection, the always-inject bridge change, and the `method.go` field/branch removal must land
together and none has. This is a hard phase-8 exit gate (it cannot close until the invariant holds), and it sits behind
the step-18 exit gate / step-20 PR gate.
