---
title: "Method Claims — deterministic, idempotent, sandboxed"
issue: https://github.com/NobleFactor/devlore-cli/issues/677
status: draft
created: 2026-08-25
updated: 2026-08-25
---

# Plan: Method Claims — deterministic, idempotent, sandboxed

**Epic:** [#530 — The execution graph](https://github.com/NobleFactor/devlore-cli/issues/530)
**Design:** [3.6-method-classification.md](../architecture/3.6-method-classification.md) ·
[2.4-hermeticity-guarantees.md](../architecture/2.4-hermeticity-guarantees.md)
**Related:** [#563](https://github.com/NobleFactor/devlore-cli/issues/563) (per-runtime module selection),
[#571](https://github.com/NobleFactor/devlore-cli/issues/571) (star's session-root anchor),
[#680](https://github.com/NobleFactor/devlore-cli/issues/680) (`template.Env`)

## Summary

The provider-level access directive was deleted from every file in the repository on 2026-08-25 — 80 references
across 38 files. Nothing replaced it. Namespace placement now falls to a hardcoded default for all nineteen
providers, which empties the `plan.*` namespace. This plan implements the per-method scheme that takes the
directive's place, restoring namespace placement and making hermeticity enforceable for the first time.

**The scheme is a claim namespace, not a classification enum.** `3.6` framed the replacement as three exclusive
tiers — `pure` / `query` / `action`. Design review on 2026-08-25 rejected that framing on three grounds, each
established against the tree:

1. **`action` collided with `op.Action`.** Every provider method is an action; `op.Action`, `op.FallibleAction`,
   and `op.CompensableAction` are all actions. Naming one tier "action" competes with the word that already means
   *unit of work in the graph*.
2. **The tiers were a proxy for hermeticity, and the proxy leaked both ways.** `platform.os()` reads a value
   supplied to the runtime environment, so it is a declared input, not a host read. `ui.print()` narrates, which
   changes nothing a plan reads back. Both are hermetic; the tier vocabulary called them `query` and `action`.
3. **Mutation is not always compensable.** Roughly eight methods mutate without returning a receipt — `shell.exec`,
   `powershell.exec`, `git.checkout`, `git.pull`, `pkg.update`, `plan.save_definition`, `plan.run`,
   `elevator.elevate`. Inference from a receipt return is sufficient but not necessary, so a tier derived from it
   would be silently incomplete.

What is actually wanted is the **guarantee a method makes**, asserted by its author and checked where checkable.
That is a claim, and claims are a family rather than a single flag.

**The vocabulary is Bazel's.** Neither Go nor the kernel ever named the property: `os.Root` names a *type*,
`RESOLVE_BENEATH` and `RESOLVE_IN_ROOT` name *resolution flags*, and `os/root.go` leaves the property in prose —
"beneath a root directory", "within a single directory tree", "outside the root". None of them needed an adjective
that classifies a function, because none of them was building a predicate over methods. The build-systems world
did name it, and kept the two halves apart: **sandboxing is the mechanism** — what an action can reach is
restricted — and **hermeticity is the outcome** — same inputs, same outputs. `sandboxed` and `deterministic` are
those two, borrowed rather than coined. `root`-based names were rejected because the word carries three meanings
here: `/`, a designated subtree treated as a root, and the superuser.

## Goals

1. **Claim per method, not per provider** — the annotation being replaced could not describe a provider whose
   methods differ, and its `both` value was the evidence
2. **Open a namespace, not a flag** — `deterministic`, `idempotent`, and `sandboxed` are independent and span both
   hermetic and mutating methods, so a single enum cannot hold them
3. **Fail closed** — an unclaimed method guarantees nothing: over-restrictive, never unsafe
4. **Verify what is verifiable** — `deterministic` against the call graph, `sandboxed` against the I/O API;
   `idempotent` carries a test obligation because nothing static can check it
5. **Restore `plan.*`** — placement derives from the claims plus the inferred mutation, and the planned namespace
   repopulates
6. **Make hermeticity expressible** — a hermetic runtime becomes a module selection, not an aspiration
7. **Feed the knowledge base** — claims are durable per-method facts, destined for
   [7-registry-knowledge.md](../architecture/7-registry-knowledge.md) in the fullness of time

## What already exists — surveyed 2026-08-25

Two findings reshape the work. Both were found by reading `pkg/op/method.go`, after the first draft of this plan
was written without it.

### `op.Method` already carries a field named `kind`

`pkg/op/method.go:52` holds `kind MethodKind`, "classified from return signature", over five values declared at
`pkg/op/method.go:581`: `MethodAction`, `MethodFallibleAction`, `MethodFunction`, `MethodFallibleFunction`,
`MethodCompensableFunction`. **This is a different axis from `3.6`'s** — it classifies a method's *shape*, not its
*effects*. Adding a second field called "kind" is a naming defect before it is a code defect, which is why step 1
is a naming decision rather than an edit.

**The consequence is favourable:** `action` inference is nearly free. `MethodCompensableFunction` already means
"returns `(T, *Receipt, error)`." The inference `3.6` describes reads a field that exists.

### `MethodModifiers` is the precedent for adding an orthogonal per-method axis

`pkg/op/method.go:619` documents itself as "orthogonal to `MethodKind`… codegen-emitted onto `MethodMetadata` and
threaded onto the constructed `Method`." `+devlore:property` → `ModifierProperty` already runs the entire path:
directive parsing (`generate.star:193`), shape validation (`generate.star:384`), metadata emission
(`generate.star:631`), and threading onto the constructed method. The new axis follows the same rails end to end.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| The access directive | ✅ Removed | 0 references repo-wide, all file types, 2026-08-25 |
| Return-signature classification | ✅ Exists | `MethodKind`, five values, `pkg/op/method.go:581` |
| Per-method axis machinery | ✅ Exists | `MethodModifiers`, the pattern to follow |
| Effect classification | ❌ Missing | `3.6` is design-only: "No implementation" |
| `RoleAction` | ❌ Absent | all 19 providers generate `RoleModule` only; `plan.*` is empty |
| `pure` verification | ❌ Missing | no call-graph check exists |
| Hermetic runtime | ❌ Not expressible | all three runtimes call `WithModules(registry.Modules()...)` |

**The tree is red because of this.** Measured 2026-08-25 with `make test`: **34 packages failing, 69 passing, 201
individual tests failing.** Thirteen of the thirty-four are `*/gen` packages — the role regression exactly; the
rest are consumers whose fixtures reach `plan.*`. The elimination landed ahead of its replacement, and this plan
is what makes the working tree committable.

## Measured baseline — the survey of 2026-08-25

All 123 provider methods, classified against the existing contract tiers and then against the claims:

| Contract tier | `MethodKind` | Action type | Count |
| --- | --- | --- | --- |
| Pure `(...) T` | `MethodAction`, `MethodFunction` | `op.action` → `op.Action` | 22 |
| Fallible `(...) (T, error)` | `MethodFallibleAction`, `MethodFallibleFunction` | `op.fallibleAction` → `op.FallibleAction` | 73 |
| Compensable `(...) (T, *Receipt, error)` | `MethodCompensableFunction` | `op.compensableAction` → `op.CompensableAction` | 28 |

**The contract tiers measure fallibility, and their doc comments claim they measure effects.** `op.Action`
(`pkg/op/action.go:30`) says "no side effects", yet holds `platform`'s five and `ui`'s five.
`op.FallibleAction` (`pkg/op/action.go:41`) says "has side effects", yet holds `regex`'s eight and
`json`/`yaml`'s seven. Correcting those doc comments is step 20.

Expected claim distribution, from the same survey:

| Claim | Approx. count | Representative members |
| --- | --- | --- |
| `sandboxed` | ~24 | the `file` read and mutate families, `archive`, `encryption` — everything already routed through `fsroot` |
| `deterministic` | ~40 | `regex` (8), `json` (4), `yaml` (3), `platform` (5), `ui` (6), `file`'s path algebra (4), `plan`'s construction surface (9) |
| `idempotent` | to be surveyed | `file.mkdir`, `file.link`, `pkg.install`, `service.enable` are candidates; none verified |

`platform` and `ui` are the cases that decided the design: both are `deterministic` — `platform` reads a value
supplied to the runtime environment at construction (`pkg/op/provider/platform/provider.go:71`, resolved once in
`Detect()`), and `ui` narrates, which changes nothing a plan reads back.

## Steps

### A — The type (`pkg/op`)

| # | Step | Anchor | ✓ |
| --- | --- | --- | --- |
| 1 | Declare `MethodClaims` as a bit set over `ClaimDeterministic` / `ClaimIdempotent` / `ClaimSandboxed` — alphabetical, explicit values, no `iota`. **Not** named `kind`: `MethodKind` already means return shape | `pkg/op/method.go:581` | ☐ |
| 2 | Add `claims MethodClaims` to `Method` and to `MethodMetadata`; accessor `Claims()` beside `Kind()` and `Modifiers()` | `pkg/op/method.go:52`, `:223` | ☐ |
| 3 | **Infer mutation** from `MethodCompensableFunction` or a `*op.RecoveryStack` return — reads the existing classification, adds no signature analysis. Sufficient, **not** necessary: the ~8 uncompensated mutators are caught by absence of claims, not by inference | `pkg/op/method.go:143` | ☐ |
| 4 | **Default to no claims** — an unclaimed method guarantees nothing and is admitted nowhere that requires a guarantee | `NewMethod` | ☐ |

### B — The generator (`generate.star`)

| # | Step | Anchor | ✓ |
| --- | --- | --- | --- |
| 5 | `parse_claims(doc, method_name)` — comma-separated, on the `parse_defaults` pattern: `split(",")`, strip each, fail on an unknown claim, fail on a duplicate, fail on a second directive line | `generate.star:178` | ☐ |
| 6 | **Validate each claim against the method's shape** — a `deterministic` claim on a compensable method is a build failure, mirroring how the property directive is rejected on non-getters | `generate.star:384` | ☐ |
| 7 | Emit claims into `MethodMetadata`, **sorted**, so a regenerated file never churns on author ordering | `generate.star:631` | ☐ |
| 8 | **Derive roles.** Replace the hardcoded access value and the three-way role branch — a provider's role set becomes the union over its methods | `generate.star:1333`, `:1452` | ☐ |

**Step 8 is the crux.** Placement moves from provider-level to per-method, and the provider's role set is derived
rather than declared. The retired directive's `both` value was never a provider property; it was the union,
spelled by hand.

### C — Registry and dispatch

| # | Step | Anchor | ✓ |
| --- | --- | --- | --- |
| 9 | Bucket from the derived roles | `pkg/op/receiver_registry.go:983` | ☐ |
| 10 | Confirm the dispatch branch reads them unchanged — it already branches on `Roles().Dispatch()` | `pkg/op/starlarkbridge/runtime.go:85` | ☐ |
| 11 | Confirm `plan.*` repopulates through the planner lookups | `pkg/op/provider/plan/provider.go:595`, `:799` | ☐ |

### D — Verifying the claims

| # | Step | ✓ |
| --- | --- | --- |
| 12 | **The capability denylist** — eleven packages, alphabetical: `crypto/rand`, `math/rand`, `net`, `net/http`, `os`, `os/exec`, `os/signal`, `os/user`, `runtime`, `syscall`, `time`. **Not a maintained list**: it enumerates the ways a Go program reaches outside its own process, which the OS interface fixes | ☐ |
| 13 | **Flag calls and function values only** — types and constants pass. `os.FileMode` in a signature and `os.O_CREATE` in an argument are fine; `os.Getenv` and `time.Now()` are not. This removes any need for a per-symbol list | ☐ |
| 14 | **Add `goast.references`** beside `goast.calls`. `calls` inspects `*ast.CallExpr` only, so it walks straight past `"Env": os.Getenv` in a `FuncMap` — the one confirmed violation in the catalog (#683). **Hard requirement**, not a refinement | ☐ |
| 15 | **Claim propagation** — a claiming method requires every provider method and local helper it calls to pass the same check. Induction, not enumeration; **no allowlist anywhere** | ☐ |
| 16 | **`deterministic`** — flag any denylisted reference in the method's own body, then propagate | ☐ |
| 17 | **`idempotent`** — no static check exists. Carries a **test obligation** instead: a method claiming it must have a test that applies it twice and asserts convergence. Rides #635 | ☐ |
| 18 | **`sandboxed`** — flag any `os` reference not routed through an `fsroot.Dir`. `fsroot.Dir` is the **one declared exception**, asserted by hand in `pkg/fsroot` and never analyzed through | ☐ |
| 19 | **Gate the build** — ruled 2026-08-25. A false claim fails codegen at the site of the claim, beside the existing directive validation at `generate.star:384` | ☐ |

**Why gating is defensible here.** The rejected design was an SSA call graph (`x/tools/go/ssa` + RTA/VTA).
`fsroot.Dir` and `platform.Platform` are interfaces, so RTA over-approximates every call through them to every
implementation in the program — false positives arising from *analysis imprecision*, which a developer cannot fix
by editing their own code. Under claim propagation plus a capability denylist, every false positive has a source
edit that resolves it: claim the helper, route the I/O through `fsroot.Dir`, or drop the claim. That is the
difference that makes a build error the right response rather than a warning.

### E — Regenerate and prove

| # | Step | ✓ |
| --- | --- | --- |
| 20 | Regenerate, and **verify codegen actually ran**. A `.star`-only change does not make grouped targets stale — that produced a false negative on 2026-08-24 | ☐ |
| 21 | **Enumerate, do not sample**: every provider that carried `RoleAction` before 2026-08-25 carries it again | ☐ |
| 22 | Negative test: a deliberately false `deterministic` claim on a method reading the ambient environment fails the build | ☐ |
| 23 | Negative test: `os.Getenv` stored as a function value is caught, proving step 14 | ☐ |
| 24 | `make check` green on Darwin **and** under `GOOS=linux` | ☐ |

### F — The record

| # | Step | ✓ |
| --- | --- | --- |
| 25 | **Correct `op.Action` and `op.FallibleAction`'s doc comments** (`pkg/op/action.go:30`, `:41`) — they describe effects and measure fallibility | ☐ |
| 26 | Rewrite `3.6-method-classification.md` onto the claim namespace; the pure/query/action framing is superseded | ☐ |
| 27 | `3.5-provider-catalog.md`'s role column regenerated from announced roles, not from a directive | ☐ |
| 28 | Survey which methods can claim `idempotent`; file what the survey finds | ☐ |

## Decisions

| # | Decision | Status |
| --- | --- | --- |
| 1 | The syntax and vocabulary | **Ruled 2026-08-25** — `+devlore:claim=` opens a namespace over `deterministic`, `idempotent`, `sandboxed`, comma-separated on one line. Vocabulary taken from Bazel, which keeps the two halves distinct: **sandboxing is the mechanism, hermeticity the outcome** |
| 2 | Gate the build or warn on the claim checks | **Ruled 2026-08-25 — gate.** A false claim is a build error, not a review finding |
| 3 | Allowlist or denylist | **Ruled 2026-08-25 — denylist.** An allowlist of safe packages is unbounded and rots; measured across the twenty providers, it would have needed ~26 entries against 11 for the capability denylist |

## Verification

- `make check` green on Darwin and `GOOS=linux`
- Every provider's pre-2026-08-25 roles restored, enumerated from the full source rather than a capped grep
- `plan.*` dispatch resolves every planned provider
- A deliberately false claim fails the build, including one stored as a function value
- Every method claiming `idempotent` has a double-application test
- No document describes namespace placement as coming from a provider-level directive
- `op.Action`'s doc comment no longer claims its members have no side effects

## Open questions carried from 3.6

1. Does a host-reading method belong in immediate scope at all?
2. **Resolved 2026-08-25** — the claim checks gate the build.
3. Does expressibility (§4a) gate or warn?
4. `template.Env` — **ruled and filed as [#683](https://github.com/NobleFactor/devlore-cli/issues/683)**: it
   consults declared runtime variables through `op.VariableResolver`, the way `make` does, not the ambient
   environment. That removes the only method whose hermeticity depended on argument content.
5. `function.call` remains undecidable — its guarantee is a property of the callable passed in.
