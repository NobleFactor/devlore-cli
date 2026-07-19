---
step: 27
former_step: 24
title: "ActivationRecord-first invariant — codegen-enforced (hard exit gate)"
status: COMPLETE 2026-07-18 — the hard exit gate is CLEARED: the required floor is conformed (slice 1) and enforced at generation time and registration time (slice 2); suite green
proof_run: 2026-06-17
parent: ../../phase-8.md
---

# Step 27 — ActivationRecord-first invariant (hard exit gate)

**Status:** re-chartered 2026-07-18; survey complete; implementation pending approval.

## The required-floor rule (re-charter, 2026-07-18)

The blanket mandate ("every announced method takes `*op.ActivationRecord` first") is SUPERSEDED. The record's
unique cargo is dispatch identity (the unit and graph — providers already hold the environment), and the mutation
families cannot function without it. The rule:

1. **Required — activation-first** for the methods that cannot correctly run without dispatch identity:
   - **Compensable actions** (a receipt or recovery stack among the returns) — they claim production and commit
     receipts;
   - **Compensating actions** (the `Compensate*` companions) — the recovery machinery dispatches them with an
     activation in hand.
2. **Permitted everywhere else.** A fallible or pure action MAY take activation-first when it has use for
   dispatch identity — and real ones do: `json.Parse`/`yaml.Parse` are receiptless producers claiming production
   via `activationRecord.Unit` (a CAS mint needs no undo, hence no receipt); future reads may want attribution.
   This is exactly today's model on the read side — no method changes shape, no declaration marker is needed, and
   the bridge's detect-and-inject machinery is the MECHANISM serving the rule, not a wart: `method.go`'s
   `firstParamIsActivation` / `undoFirstParamIsActivation` bits stay by design, and the live TODO closes as
   by-design with the rationale documented on the fields.
3. **Cross-cutting concerns stay at the dispatch layer.** Read attribution, step 30's caller id, retry, and
   cancellation are framework concerns; the framework holds the activation at every dispatch whether or not the
   method receives it. Where a non-mutating method needs a unit reference as an argument to reason about, it may
   also take one explicitly as data (`plan.Provider.Plan(name, unit, kwargs)` — the established pattern).
4. **Enforcement — the floor only**: the generator validates that every compensable action and every
   `Compensate*` companion is activation-first and fails generation on violation — the compile-time exit gate; a
   registration-time assert in `receiver_type.go` backstops hand-announced types. Generation output and the
   injection mechanics change NOT AT ALL — the one new thing is the required-side check, which kills the real bug
   class (a mutator that forgets the activation cannot claim production or stamp receipts correctly).

## Survey (2026-07-18, against the required floor)

Census: 25 compensable · 20 compensating · 97 fallible · 21 pure announced methods.

**Required-but-missing (24)** — the conformance backlog:

| Provider | Methods |
|----------|---------|
| file | `Remove`, `RemoveAll`, `Unlink`, `WalkTree` (returns the walk's recovery stack), `CompensateFileMutation` |
| service | `Enable`, `Disable`, `Start`, `Stop`, `Restart` + their five `Compensate*` companions |
| pkg | `Install`, `Remove`, `Upgrade`, `CompensatePackageMutation` |
| encryption | `DecryptSopsFile`, `EncryptFile` + their two `Compensate*` companions |
| git | `CompensateClone` |

**On the permissive side (informational, no action):** `json.Parse` and `yaml.Parse` use their activations for
production claims — the permissive rule's proof case; `flow.Complete`/`Degraded`/`Failed` carry activations their
bodies currently ignore — legal under the rule (available for the step-41 flow-driver work if wanted).

Notably: the blanket mandate's 127-method backlog contained zero violations of the required floor — all 127 were
fallible or pure methods, already correct under the permissive rule.

## Slice plan (pending approval)

1. **Slice 1 — COMPLETE 2026-07-18**: all 24 methods gained `activationRecord *op.ActivationRecord` first
   (file's delete trio + `WalkTree` + `CompensateFileMutation`; service's five mutators + five companions;
   pkg's three + `CompensatePackageMutation`; encryption's two + two; `git.CompensateClone`), each with the
   step-27 doc bullet. Callers: one production site (starcode's `WalkTree`, minting a bare non-graph
   activation) and ~93 test call sites; the generated starlark surface is byte-identical (the generator skips
   the leading activation). Suite green.
2. **Slice 2 — COMPLETE 2026-07-18 (enforcement, both layers):**
   - **Generation time (the compile-time gate)**: `generate.star` gains `validate_activation_floor`, run over
     the UNFILTERED provider method list (so `Compensate*` companions, excluded from the starlark surface, are
     validated too). Compensator detection is exact-token (`*Receipt` / `*op.RecoveryStack` and their qualified
     forms) — the first run caught its own substring bug by false-positing on the unexported `stageWrite`
     (returns `*ReceiptSpec`), fixed with token matching and an unexported-method skip. The whole tree
     generates clean under the check.
   - **Registration time (the backstop for hand-announced types)**: `op.NewMethod` rejects a compensable
     method without the leading activation (gated on provider methods via `enforceCompanions`); the
     compensation-companion classification MANDATES `(receiver, *ActivationRecord, compensator)` — the
     two-shape tolerance is gone; and the compensating-action index asserts the same floor as it builds.
     The backstop immediately caught two real violations in `pkg/op`'s own executor test fixtures
     (`compensationFailingFixture`/`compensationCleanFixture`), now conformed.
   - **The TODO closes as by-design**: the `method.go` discrimination fields carry the required-floor
     rationale — they are the mechanism serving the permissive read side, validated rather than tolerated.
   - Two new tests pin both rejection paths (`TestNewReceiverType_RequiredFloor_*`); star reinstalled from the
     fixed extension; generation output for every conforming provider is byte-identical.

## Superseded charter (2026-06-17, for the record)

The original charter mandated the blanket rule. Its evidence table follows unchanged.

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
`*ActivationRecord`+compensator) rather than mandating the activation-first shape.

## Disposition / grade

`not-started` — accurate. The current mechanism is precisely the optional/detected design the invariant is meant to
supersede; the codegen rejection, the always-inject bridge change, and the `method.go` field/branch removal must land
together and none has. This is a hard phase-8 exit gate (it cannot close until the invariant holds), and it sits behind
the step-18 exit gate / step-20 PR gate.
