---
title: "Method Claims — deterministic, idempotent, sandboxed"
issue: https://github.com/NobleFactor/devlore-cli/issues/677
status: in-progress
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

## The model — ruled 2026-08-26

The first draft of this plan derived `RoleModule` / `RoleAction` from the claims. That was reverse-engineering a
broken scheme: the access directive it replaced was vague, and its role assignment cannot be reproduced from any
property of the code. Four providers with zero mutators were `both`; two with zero mutators were `immediate`.
Nothing distinguishes them but a judgment nobody wrote down.

**What we are actually building is `op.Graph` instances and `starlarkbridge.Runtime` instances, and they ask
different questions.**

### The graph accepts anything with an action signature

Every one of the 123 provider methods qualifies — `newAction` (`pkg/op/action_types.go:229`) already switches
over all five `MethodKind` values. There is nothing to derive and nothing to declare.

### The module surface is filtered per method, per runtime

The user selects modules; **the framework filters their methods** by the kind of runtime being built:

| Runtime | Admits |
| --- | --- |
| hermetic (graph planning) | methods claiming `deterministic` |
| non-hermetic (scripting, `star`) | everything |

`file` is the proof that this cannot be a provider-level property: a hermetic runtime wants `file.join` and not
`file.write_text`, and no single bit on the `file` provider can say so.

**`RoleModule` and `RoleAction` therefore dissolve.** `RoleAction` would be universal-minus-one, and `RoleModule`
is not a static property at all. `RoleRoot` survives untouched — it answers placement, not eligibility.

### Three exclusions, and only three

| What | Graph | Module surface | Why |
| --- | --- | --- | --- |
| `flow` | **only** here | excluded | its methods *are* the graph combinators; they mean nothing outside a graph |
| `plan` | excluded | **only** here, hermetic only | no scenario for planning the planner |
| `Compensate*` — 18 methods | excluded | excluded | already, at `generate.star:264` and `:325`; the recovery machinery dispatches them with an activation in hand |

Every other announced provider is eligible for both, subject to the per-method filter.

### Hermeticity is inherited, not inspected

`function.call` was recorded as undecidable because its hermeticity depends on the callable passed in. It is not:
the callable is evaluated against the same filtered surface, so in a hermetic runtime a Starlark function cannot
reach anything the runtime did not admit. `template.render_*` resolves the same way once
[#683](https://github.com/NobleFactor/devlore-cli/issues/683) lands and `Env` reads declared variables. Both
"undecidable" entries were artifacts of analysing methods outside a runtime.

### Empty versus emptied

- A provider with **no methods to offer** is skipped silently — `elevation.Provider` while dormant, `mem` which
  registers a resource scheme and has no `Provider` at all.
- A provider whose methods are **all filtered out** is a loud error. Selecting `shell` for a planning runtime is
  a mistake the author should hear at construction, not discover as a `NameError` three lines later.

## Steps

### A — The type (`pkg/op`)

| # | Step | Anchor | ✓ |
| --- | --- | --- | --- |
| 1 | Declare `MethodClaims` as a bit set over `ClaimDeterministic` / `ClaimIdempotent` / `ClaimSandboxed` — alphabetical, explicit values, no `iota`. **Not** named `kind`: `MethodKind` already means return shape | `pkg/op/method.go:581` | ✅ |
| 2 | Add `claims MethodClaims` to `Method` and to `MethodMetadata`; accessor `Claims()` beside `Kind()` and `Modifiers()` | `pkg/op/method.go:52`, `:223` | ✅ |
| 3 | **Infer mutation** from `MethodCompensableFunction` or a `*op.RecoveryStack` return — reads the existing classification, adds no signature analysis. Sufficient, **not** necessary: the ~8 uncompensated mutators are caught by absence of claims, not by inference | `pkg/op/method.go:143` | ✅ |
| 4 | **Default to no claims** — an unclaimed method guarantees nothing and is admitted nowhere that requires a guarantee | `NewMethod` | ✅ |

**Phase A landed 2026-08-25.** `MethodClaims` is declared in `pkg/op/method.go`'s SUPPORTING TYPES region,
alphabetically ahead of `MethodKind`, over `ClaimDeterministic` / `ClaimIdempotent` / `ClaimSandboxed` with
explicit hex values. `Method` gained the `claims` field plus `Claims()` and `setClaims()`, and `MethodMetadata`
gained `Claims` — the same rails `Modifiers` already used, threaded at `pkg/op/receiver_registry.go:223`.

`Method.Mutates()` is the derived companion: `kind == MethodCompensableFunction`, which covers both `*Receipt`
and `*RecoveryStack` returns (`method.go:849` caches `recoveryStackType` for that same check). Its doc records
the asymmetry — the inference is sufficient, not necessary, because `shell.exec` and `git.pull` mutate without
returning either.

`make vet` green.

### B — The generator (`generate.star`)

| # | Step | Anchor | ✓ |
| --- | --- | --- | --- |
| 5 | `parse_claims(doc, method_name)` — comma-separated, on the `parse_defaults` pattern: `split(",")`, strip each, fail on an unknown claim, fail on a duplicate, fail on a second directive line | `generate.star:178` | ✅ |
| 6 | **Validate each claim against the method's shape** — a `deterministic` claim on a method that mutates is a build failure, mirroring how the property directive is rejected on non-getters | `generate.star:384` | ✅ |
| 7 | Emit claims into `MethodMetadata`, **sorted**, so a regenerated file never churns on author ordering. The template's three-way branch (planner / property / plain) becomes a `metadata_extras` list, or adding a fourth field makes it six branches | `generate.star:631`, `provider.gen.go.template:21` | ✅ |
| 8 | **Retire the role computation.** `RoleModule` and `RoleAction` stop being announced; `RoleRoot` stays. The hardcoded `access` value and the three-way role branch both go | `generate.star:1333`, `:1452` | ✅ |
| 9 | Declare the two exclusions — `flow` graph-only, `plan` module-only. **Mechanism unruled**: two provider directives, or a property of the announcement | — | ✅ |

**Note on the dead `pure` field.** `generate.star:345` computes `pure = "error" not in m.returns` and `:404`
writes it into the descriptor, where **nothing reads it** — zero template hits. It means *infallible*, not
effect-free, which is the same conflation `op.Action`'s doc comment carries (step 25). Removing it is trivial and
in scope for step 8's cleanup.

**Phase B landed 2026-08-26.** `parse_claims` follows the `parse_defaults` precedent — comma-separated on one
line, sorted on the way out so author order never churns a regenerated file. Shape validation refuses
`deterministic` on a receipt or recovery-stack return, quoting the signature back:
`method Mkdir: claims deterministic but returns "(*Directory, *Receipt, error)", which mutates by construction`.

The template's three-way branch (planner / property / plain) collapsed to two via a `metadata_extras` list, so a
fourth optional field costs nothing. `regex.Match` carries the first real claim in the tree.

**The dispatch zone survived, redefined.** `+devlore:surface=` maps straight onto it: `graph` is `RoleAction`
alone, `module` is `RoleModule` alone, absent is both. `RoleModule` now means *eligible for a module surface*,
not *immediate mode* — which METHODS reach a given runtime is phase C's question. `AnnounceProvider` needed no
signature change and the registry needed no restructure.

Two dead fields retired en route: `access_title` and `pure` (`"error" not in m.returns` — infallible, not
effect-free), both written and read by nothing.

**Verified after regeneration** (103 codegen invocations, confirmed run rather than assumed): `flow` announces
`RoleAction|RoleRoot`, `plan` announces `RoleModule`, the other sixteen announce both, `mem` announces none.
**`make check` exits 0 — 103 packages passing, zero failures, down from 34 failing packages and 201 failing
tests.**

### C — The runtime filter (`starlarkbridge`)

| # | Step | Anchor | ✓ |
| --- | --- | --- | --- |
| 10 | Add a `Hermetic()` [RuntimeOption] — options run before the surface is built (`runtime.go:60`), so it can walk `rt.modules`, read each method's `Claims()`, and record the verdict | `runtime.go:412` | ✅ |
| 11 | **Consult the filter at install time, not after.** `applyDenials` wraps a *global* in a `filteredReceiver`; a root provider installs each method as its own top-level global, which the deny map cannot reach. One `admits(module, method)` predicate, asked in both the attribute path and the root path | `runtime.go:85`, `:110`, `:310` | ✅ |
| 12 | Skip a provider with no methods **silently** — `elevation` while dormant, `mem` which has no `Provider` | `runtime.go:85` | ✅ |
| 13 | Fail **loudly** when every method of a selected provider is filtered out | `runtime.go:85` | ✅ |
| 14 | **Annotate the providers.** Apply `+devlore:claim=` to every method that qualifies — roughly 28 `deterministic` (`regex` 8, `plan`'s construction surface, `file`'s path algebra, `json`, `yaml`, `platform`, `ui`) plus the `sandboxed` I/O family. **Ordering constraint, found 2026-08-26**: step 15 cannot precede this. Only `regex.Match` claims anything today, so making `lore` hermetic first would trip step 13's loud failure on every provider it selects | — | ☐ |
| 15 | Wire the option: `lore`'s planning runtime hermetic, `star`'s scripting runtime not | `cmd/lore/lore/builder.go:111`, `cmd/star/star/application.go:84` | ☐ |
| 16 | `DenyAttributes` keeps working unchanged — explicit caller-driven narrowing is a separate concern from the hermeticity filter | `runtime.go:451` | ✅ |

**Phase C landed 2026-08-26** (steps 10–13, 16). `Hermetic()` is a `RuntimeOption`; `admits(method)` is
`!rt.hermetic || method.Claims()&op.ClaimDeterministic != 0`; `partitionMethods` returns both the admitted count
and the refused names, because the two zero cases mean different things.

The filter is consulted **at install time**, not through `applyDenials`, so it reaches both paths: a non-root
provider is wrapped in a `filteredReceiver` carrying the refused set, and a root provider's install loop skips a
refused method outright. `DenyAttributes` is untouched — caller-driven narrowing stays a separate concern.

**The root path turned out to be still dormant.** `flow` declaring `surface=graph` means it never reaches the
module surface at all, so no provider is `RoleModule|RoleRoot` and that branch remains reserved exactly as its
comment claims. The filter covers it anyway, against the day one appears.

Proved by `TestNewRuntime_Hermetic_AdmitsOnlyClaimingMethods` (a fixture with one claiming and one silent method:
non-hermetic admits both, hermetic admits one) and `TestNewRuntime_Hermetic_ProviderWithNothingAdmitted_Fails`.
`make test` green, 103 packages.

### D — Verifying the claims

| # | Step | ✓ |
| --- | --- | --- |
| 17 | **The capability denylist** — eleven packages, alphabetical: `crypto/rand`, `math/rand`, `net`, `net/http`, `os`, `os/exec`, `os/signal`, `os/user`, `runtime`, `syscall`, `time`. **Not a maintained list**: it enumerates the ways a Go program reaches outside its own process, which the OS interface fixes | ☐ |
| 18 | **Flag calls and function values only** — types and constants pass. `os.FileMode` in a signature and `os.O_CREATE` in an argument are fine; `os.Getenv` and `time.Now()` are not. This removes any need for a per-symbol list | ☐ |
| 19 | **Add `goast.references`** beside `goast.calls`. `calls` inspects `*ast.CallExpr` only, so it walks straight past `"Env": os.Getenv` in a `FuncMap` — the one confirmed violation in the catalog (#683). **Hard requirement**, not a refinement | ☐ |
| 20 | **Claim propagation** — a claiming method requires every provider method and local helper it calls to pass the same check. Induction, not enumeration; **no allowlist anywhere** | ☐ |
| 21 | **`deterministic`** — flag any denylisted reference in the method's own body, then propagate. **Name the reach, never render a verdict**: "claims deterministic but calls `os.Getenv` at `provider.go:110`", the shape step 6 already uses when it quotes the offending signature back. The author reads evidence and knows what to change; "not deterministic" sends them hunting | ☐ |
| 22 | **`idempotent`** — no static check exists. Carries a **test obligation** instead: a method claiming it must have a test that applies it twice and asserts convergence. Rides #635 | ☐ |
| 23 | **`sandboxed`** — flag any `os` reference not routed through an `fsroot.Dir`. `fsroot.Dir` is the **one declared exception**, asserted by hand in `pkg/fsroot` and never analyzed through | ☐ |
| 24 | **Gate the build** — ruled 2026-08-25. A false claim fails codegen at the site of the claim, beside the existing directive validation at `generate.star:384` | ☐ |

**Why gating is defensible here.** The rejected design was an SSA call graph (`x/tools/go/ssa` + RTA/VTA).
`fsroot.Dir` and `platform.Platform` are interfaces, so RTA over-approximates every call through them to every
implementation in the program — false positives arising from *analysis imprecision*, which a developer cannot fix
by editing their own code. Under claim propagation plus a capability denylist, every false positive has a source
edit that resolves it: claim the helper, route the I/O through `fsroot.Dir`, or drop the claim. That is the
difference that makes a build error the right response rather than a warning.

### E — Regenerate and prove

| # | Step | ✓ |
| --- | --- | --- |
| 25 | Regenerate, and **verify codegen actually ran**. A `.star`-only change does not make grouped targets stale — that produced a false negative on 2026-08-24 | ☐ |
| 26 | **Enumerate, do not sample**: every provider that carried `RoleAction` before 2026-08-25 carries it again | ☐ |
| 27 | Negative test: a deliberately false `deterministic` claim on a method reading the ambient environment fails the build | ☐ |
| 28 | Negative test: `os.Getenv` stored as a function value is caught, proving step 14 | ☐ |
| 29 | `make check` green on Darwin **and** under `GOOS=linux` | ☐ |

### F — The record

| # | Step | ✓ |
| --- | --- | --- |
| 30 | **Correct `op.Action` and `op.FallibleAction`'s doc comments** (`pkg/op/action.go:30`, `:41`) — they describe effects and measure fallibility | ☐ |
| 31 | Rewrite `3.6-method-classification.md` onto the claim namespace; the pure/query/action framing is superseded | ☐ |
| 32 | `3.5-provider-catalog.md`'s role column regenerated from announced roles, not from a directive | ☐ |
| 33 | Survey which methods can claim `idempotent`; file what the survey finds | ☐ |

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
