---
step: 43
title: "Reflect once at registration — encapsulate dynamic dispatch behind typed adapters"
status: COMPLETE 2026-07-19 — both waves landed in one slice: the compensation adapters (index-build + NewMethod) and the forward adapter (the variadic decision baked at NewMethod); every residual reflect call lives inside a registration-built closure; suite + vet green
proof_run: n/a (charter)
parent: ../../phase-8.md
---

# Step 43 — Reflect once at registration; dynamic dispatch invisible to callers

**Status:** COMPLETE 2026-07-19 — both waves in one slice (step 27's floor collapsed the shape variance that
made two waves prudent).

## Landed (2026-07-19)

1. **Wave 1 — compensation.** `compensatingAction` drops its raw `reflect.Method` + shape bit for a baked
   `invoke func(receiver, activation, undoState) error`, built once in the index assembly and closing over the
   mandated activation-first shape (step 27's floor made it single-shape by construction — the two-shape branch
   the charter anticipated no longer exists to encode). `invokeCompensatingAction` is deleted; the unwind path
   calls `comp.invoke(...)` — a plain call. `Method.Undo` likewise dispatches through `undoInvoke`, baked at
   `NewMethod` beside the shape validation; the dead `undoFirstParamIsActivation` field is gone (invariantly
   true under the floor), and the step-27 by-design note narrows to the do-side bit, which genuinely varies.
2. **Wave 2 — forward.** `Method.Invoke`'s per-call variadic branch is decided once at `NewMethod`
   (`doInvoke` binds `Func.CallSlice` or `Func.Call` as a method value); the invoke tail is a plain adapter
   call. `compileDispatcher` (`receiverType.Do`) already satisfied the invariant — first-use-compiled closures
   cached in the dispatch table — and is unchanged.
3. **The census** (the charter's verification 1): the only `Func.Call`/`fn.Call` sites under `pkg/op` sit
   inside registration-built closures — `method.go` (the undo adapter body), `receiver_registry.go` (the
   compensating-action adapter body), `receiver_type.go` (compileDispatcher's cached closure). The invoke
   paths — `Method.Invoke`, `Method.Undo`, the recovery stack — carry none. Go offers no cheaper generic
   call than `reflect.Value.Call`, so the mechanism necessarily lives inside the baked closures; every
   signature-derived *decision* (shape, variadic form, argument layout) is paid exactly once at registration.
4. Callers observed no change (verification 2 — the suite is green untouched); the trace save/load/resume
   suites, including step 21's resumed-unwind tests, exercise the rebuilt-adapter path end to end
   (verification 3); `make vet` clean.

## Superseded charter (2026-07-11, for the record)

**Status as chartered:** `not-started`. Chartered 2026-07-11 out of the step-40/42 design discussion.
**Orthogonal** to the
terminology purge ([step 40](40-complement-to-receipt.md)) and the `Compensator` interface unification
([step 42](42-compensator-interface.md)): those change the *names* and the *shape*; this changes *when the reflection
is paid* and *who can see it*. It touches the same call sites as step 42, so the two must be coordinated (see
Sequencing).

## Goal

Two invariants govern this step:

1. **Invisible to callers.** The dynamic dispatch — resolve a compensating action by name, then call it — is fully
   encapsulated behind an ordinary typed call surface. A caller invokes a compensator (`compensator.Compensate(env)`
   after step 42, or the adapter's `invoke(...)`) and never touches `reflect`; swapping the reflected call for a baked
   adapter changes nothing a caller can observe.
2. **Reflect at most once.** Reflection is paid exactly once per registered method — at registration — and never again
   per call. This is a **system-wide** invariant, not a compensation-local one: forward dispatch converges on it too
   (see Scope).

## What this step does

Compensation invocation reflects on **every call**. `invokeCompensator` calls `comp.method.Func.Call(goArgs)`
(`recovery_stack.go`), and the not-yet-migrated fallback `Method.Undo` calls `m.undo.Func.Call(goArgs)`
(`method.go`). The *dispatch* is necessarily dynamic — a receipt names its compensating action by string
(`Receipt.CompensatingAction()`), resolved through the registry's compensating-action index, so that a serialized
receipt can be re-resolved on resume. But the *call* need not reflect each time: the name→method resolution and the
argument-shape decision (`firstParamIsActivation`, the undo-state type) are known at **registration**, when
`CompensatorByName` builds the index (`receiver_registry.go`).

Bake a typed adapter once, at index-build time, and store it on the compensating-action entry instead of invoking the
raw `reflect.Method` per call:

```go
// Built once per Compensate<Name> when the compensating-action index is assembled. Closes over the resolved
// reflect.Method and the activation-first shape; the invoke path calls it directly — no per-call reflection.
type compensatingAction struct {
	providerReceiverType ProviderReceiverType
	compensatorType      reflect.Type                                              // the undo-state type it accepts
	invoke               func(receiver any, activation *ActivationRecord, undoState any) error
}
```

`invokeCompensator` (→ `compensatingAction.invoke` after step 42) then becomes a direct func call. The reflection —
and the argument-type check that a `reflect.Call` performs — is paid once per registered compensating action, not once
per rollback, and it is localized to the index build rather than scattered across the invoke path.

This is a performance + type-locality refinement, not a behavior change: compensation runs only on rollback (a rare,
already-failing path), so the win is not hot-path throughput but a single, guarded reflection site and a plain call
everywhere else. The registration-time guards that already validate the shape (`isLegalCompensator`, the recorded
`compensatorType`) become the one place a mismatch is caught, instead of a latent `reflect.Call` panic at unwind.

## Scope — compensation first, forward to follow

The end state is a single dispatch-adapter mechanism honoring "reflect once at registration" in **both** directions —
compensation *and* forward `Method.Invoke`. Compensation lands **first**, to work out the specifics (the adapter
signature, where it is built, how resume rebinds it) on the low-risk path (rollback-only). The same pattern then applies
to forward dispatch. Compensation-first is a de-risking sequence, **not** a scope boundary: "reflect no more than once"
is the governing constraint everywhere, so the forward path is in the end state, not a maybe.

- **Wave 1 — compensation.** `invokeCompensator` + the `Method.Undo` fallback (the Execution plan below).
- **Wave 2 — forward dispatch.** Apply the same registration-time adapter to `Method.Invoke`. Forward dispatch is
  hot-path (per-dispatch, not rollback-only), so this wave also lifts reflection off the throughput-sensitive path — a
  larger, higher-risk rework of the core loop, taken once wave 1 has proven the pattern.

## Execution plan (wave 1 — compensation)

Each phase gofmt-clean with `make test` green (modulo the standing step-18 gate set) before the next; commit per phase.

1. **Bake the adapter at index build.** In `CompensatorByName`'s `compensatorOnce.Do` (`receiver_registry.go`), build
   an `invoke func(receiver, activation, undoState) error` per `Compensate<Name>` that closes over the `reflect.Method`
   and the `firstParamIsActivation` shape, and store it on the entry. Keep `compensatorType` for the registration-time
   type check.
2. **Call the adapter.** Replace `invokeCompensator`'s `comp.method.Func.Call` with a call through the stored `invoke`;
   do the same for the `Method.Undo` fallback (or fold `Method.Undo` onto the same adapter path). No caller sees a
   change — the invisible-to-callers invariant is what makes this a pure internal swap.
3. **Verify.** No `.Func.Call` remains on the compensation invoke path; `make test` green (modulo the step-18 gate);
   `make vet` clean; the trace save/load/resume suites still pass (resume rebinds the compensate closure, which resolves
   the adapter from the rehydrated index — the adapter is rebuilt from bytes exactly as the raw method was).

## Sequencing

- **After step 40** (operates on the *compensating-action* vocabulary, not "complement").
- **Coordinate with step 42.** Step 42 relocates the invoke — `invokeCompensator` becomes `compensatingAction.invoke`,
  and the leaf undo moves into `ReceiptBase.Compensate`. Cleanest to land **after step 42**, so the adapter replaces the
  reflection *inside* the already-methodized invoke rather than being reworked by it. If it lands before 42, 42 inherits
  the adapter call unchanged.

## Verification

1. No per-call `reflect.Value.Call` (`.Func.Call`) remains on the compensation invoke path; the reflection is confined
   to the index build in `CompensatorByName`. (Wave 2 extends the same check to `Method.Invoke`.)
2. Callers of the invoke path are unchanged — the swap is invisible above the dispatch boundary.
3. `make test` green modulo the step-18 gate set; `make vet` clean; the trace save/load/resume suites prove a resumed
   run rebuilds and invokes the adapter correctly (the serialized name still round-trips to a working call).
