---
step: 22
former_step: 19
title: "Resource foundation cleanup — the 13.0(k) arc plus sub-steps (d)–(n)"
status: in-progress — sub-steps (d)–(n) done; 13.0(k) items 1–2 done (Planned companions deleted; twelve Resource interfaces + k.12 boot test). k.13 is PARTIAL (2026-07-14 audit): the Pending/Active/Gone types exist and production→Active works (GetOrCreate/markActive), but the DISCOVERY-side Pending→Active/Gone transition is UNBUILT — DiscoverResource catalogs Pending and never verifies existence; markGone has no production caller. Plan approved 2026-07-14 (Resolve + Exists on the op.Resource interface; file real + eight assert.Unimplemented stubs; catalog-owned VerifyExistence called from file.DiscoverResource — Option A). First implementation attempt reverted 2026-07-14 pending two open design questions (§ Open design questions): the Exists()/ObservationBase.Exists collision, and Option A wiring existence at plan time (it must resolve at runtime). assert.Unimplemented kept. 13.0(n) writ graph executor (= step 33) also open.
proof_run: 2026-07-14 (audit of the sub-item ledger + the discovery-lifecycle gap)
parent: ../../phase-8.md
---

# Step 22 — Resource foundation cleanup (formerly 19)

**Status:** `in-progress`. Prerequisite for step 13 and everything downstream that touches Resources. The umbrella holds
two ledgers: the lettered sub-steps 22(d)–(n) — **all complete** (see the phase-8 table rows) — and the 13.0(k) sub-item
arc, **now nearly closed** save the discovery-side resource lifecycle (the plan below) and 13.0(n) (= step 33).

## The 13.0(k) arc

1. **Delete `<M>Planned` companions** — done (repo-wide: no `*Planned` companions remain).
2. **Twelve required Resource interfaces across all nine Resource-bearing providers** — done: `op.ResourceBase` shared
   implementations plus per-type overrides on file/git/appnet/pkg/service/mem/function/json/yaml; the k.12
   boot-discipline test (`TestBootDiscipline_EveryResourceTypeOverridesAddressing`,
   `pkg/op/inventory/discipline_test.go:28`) asserts no Resource type leaves `Addressing` at the default
   `AddressingUnknown` sentinel.
3. **Catalog operations on the addressing/digest contract** — k.10 (Resolve cascade), k.13 (lifecycle), k.14
   (audit-only — file-provider Compensate methods inspected method-by-method; no migration work remained).

**k.13 — PARTIAL (corrected 2026-07-14 audit).** The `Pending`/`Active`/`Gone` state (`ResourceState`) and the
catalog-owned transition helpers (`markActive` / `markGone`) exist, and the **production** path resolves correctly
(`GetOrCreate` → `markActive`, `resource_catalog.go:285`). But the **discovery** side is unbuilt:
`ResourceCatalog.Discover` (`:193`) returns a `Pending` cache-hit as-is (`:206`) and Links a fresh `Pending` on miss
(`:214`) **without verifying existence**, so a discovered resource never transitions; `markGone` (`:654`) has **no
production caller** (test-only), so the `Gone` transition is unreachable outside tests. This contradicts §6.2 of
[4-resource-management.md](../../../architecture/4-resource-management.md) (DiscoverResource miss → Active/Gone,
hit-`Pending` → in-place Active/Gone). Closing the discovery-side lifecycle is the **plan below**.

**k.15 — supersedes two bad corrections.** A first note claimed `(*ResourceCatalog).ResolvePending()` was wired into the
`Run` preflight — that method does not exist (repo-wide); the `Run` preflight (`graph_executor.go:392–414`) does
ledger-rehydrate + re-arm + variable-binding, not a pending-resolve pass. A follow-up "correction" then claimed pending
resolution is **handled in place by DiscoverResource** — **also wrong** (2026-07-14 audit): `DiscoverResource` does not
verify existence today (see k.13). Neither a preflight-batch nor an in-place-`Discover` resolver exists; the
discovery-side transition is genuinely unbuilt, and closing it is this step's remaining framework work (the plan below).

Platform verification at preflight, originally scoped into k.15, moved out — tracked as #282 under step 16's preflight
scope.

## Plan — discovery-side resource lifecycle (`Resolve` + `Exists`) — approved 2026-07-14

Close the k.13 discovery-side gap: `DiscoverResource` verifies existence and drives `Pending → Active` (exists) /
`Pending → Gone` (missing), per §6.2. Staged — **file** is implemented and tested; the other eight types are loudly
stubbed and land in later per-type steps.

**Contract.**

1. **`Resolve() error`** — locate the resource by URI (rebind to the execution fsroot) and verify reachability, exactly
   as `file.Resource.Resolve` (`file/resource.go:438`) does today. It is not a clean existence signal on its own (file's
   `Resolve` returns nil for a not-exist path) and it populates **no** metadata — that stays `Provider.Observe`.
2. **`Exists() bool`** — new; the existence predicate the catalog reads without interpreting `Resolve`'s error.
   `file.Resource.Exists` (`:338`) is the model (a fresh stat → true / false).
3. Both are added to the **`op.Resource` interface** (`resource.go:42`), so the catalog dispatches polymorphically — all
   nine types implement, `file` real + eight stubbed.

**New — `assert.Unimplemented`.** Mirrors `assert.Unreachable` (`pkg/assert/assert.go:161`):
`Unimplemented(what string)` → `raise(2, "unimplemented: "+what)`; a loud stub that fails fast if reached. Placed
alphabetically before `Unreachable`.

**Per-type.**

| Type | `Resolve()` | `Exists()` |
|---|---|---|
| **file** | real (rebind + stat; hard errors → error) | real (existing `Exists() bool`) |
| appnet · function · git · json · mem · pkg · service · yaml | `assert.Unimplemented("<pkg>.Resource.Resolve")` | `assert.Unimplemented("<pkg>.Resource.Exists")` |

(git and pkg currently have no-op `Resolve` methods; those are replaced by the `assert.Unimplemented` stub.)

**Wiring — Option A (catalog-owned, localized to file's discovery path).** All nine `DiscoverResource` functions route
through `ResourceCatalog.Discover`, so putting the existence check inside the shared `Discover` would call a panic-stub
on every non-file discovery (pkg rehydrate, git/service resume, content types — all live today) and turn green tests
red. Instead:

- Add a catalog-owned, exported `VerifyExistence(resource)` step → reads `Exists()`, applies `markActive` (true) or
  `markGone` + returns an error (false). The transition stays catalog-owned (§6.2, `4-resource-management.md:557`).
- Call it **only from `file.DiscoverResource`** (`file/resource.go:114`), after `catalog.Discover` returns the entry.
  The other eight `DiscoverResource` functions are unchanged (catalog `Pending`, as today); their `Resolve` / `Exists`
  stubs are never reached at runtime — pure tripwires. Each later per-type step opts its type in by implementing
  `Resolve` / `Exists` and adding the `VerifyExistence` call.

**Tests (file only).**

1. `Discover` miss + file exists → `Active`; missing → `Gone` + error.
2. `Discover` hit-`Pending` + exists → in-place `Active`; missing → `Gone`.
3. hit-`Active` → returns existing (no re-check); hit-`Gone` → error.
4. `markGone` reached from the discovery path (closes the dead-code finding).

**Docs (on implementation).** Correct `Discover`'s doc comment (`:172–179`) + the inline `:213` "preflight pass"
comment; this step doc's k.13 / k.15 (done above); and §6.1 / §6.2 of `4-resource-management.md` (the existence verdict
is `Exists()`, the transition step is `VerifyExistence`, staged file-first).

**Scope.** IN: the interface methods, `assert.Unimplemented`, file real + eight stubbed, the Option-A wiring, file
tests, doc reconciliation. OUT (deferred, loudly stubbed): real `Resolve` / `Exists` for the other eight — each a later
per-type step; the panic-stub is the tripwire.

## Open design questions (2026-07-14 session — settle before re-implementing)

The first implementation attempt was reverted (it introduced two problems). Both must be settled before re-implementing.

**Issue 1 — `Exists()` collides with `ObservationBase.Exists` (a real design issue).** Adding `Exists()` to the
`op.Resource` interface collides with the existing `Exists bool` field on `ObservationBase` (`op/observation.go:51`),
because `Observation` is an interface that **embeds `Resource`** (`:24`) — so an `Observation` is-a `Resource` and must
carry both. Go forbids a field and a promoted method sharing a name (the field shadows the method), so `ObservationBase`
silently stops satisfying the enlarged `Resource`. It is not a rename nuisance: the two meanings are genuinely
distinct — `Resource.Exists()` is "does this resource exist **now**" (the runtime predicate the catalog reads), while
`ObservationBase.Exists` is "was the observed thing present **at observation time**" (a recorded measurement). "Exists"
is the right word in both domains, which is exactly why they collide. Options:

1. Rename the `Resource` predicate — keep existence on `Resolve`, or use `Present()` / `IsPresent()` — leaving
   `Observation.Exists` untouched.
2. Rename the `Observation` field (`present` / `observed`) and let `Observation` override `Exists()` — but then
   `Observation.Exists()` reads as "the observed thing exists," conflating the two meanings.
3. Revisit whether `Observation` should embed `Resource` at all — it embeds it for identity (URI / ID), but a
   measurement is not something you query for existence. The largest change, but possibly the actual root.

**Issue 2 — the Option-A wiring fires at plan time; existence resolves at runtime.** The *Wiring* section above ("call
`VerifyExistence` from `file.DiscoverResource`") is wrong: `DiscoverResource` is the **plan-time introduction** path —
the string→`Resource` construction that param-binding uses — so verifying existence there makes it a *planning*
precondition and breaks planning of any operation on a not-yet-existent or intentionally-missing file (`file.exists`,
`file.write`, `archive.extract`, …); the reverted attempt failed 13 tests this way. At plan time a resource is
**introduced** from a string (a URI or source path) and cataloged `Pending`, deliberately unresolved; it is
**expected to resolve at runtime**. So `VerifyExistence` must be driven from the runtime resolution point, not
`DiscoverResource`.

**The resolution point — verified against the code (2026-07-14).** Plan-time resources are cataloged in the graph's
`ResourceCatalog` as `Pending` (`markActive` fires only from production, `GetOrCreate:285`), and are **not** discovered
or observed at plan time (`Provider.Observe` has no production caller — only a stale comment at `git/provider.go:196`).
At pre-flight, `GraphExecutor.Run` **clones** that planning catalog onto the `RuntimeEnvironment`
(`graph_executor.go:372`, `spec.WithCatalog(graph.ResourceCatalog().Clone())`), leaving the planning catalog pristine;
from there the executor owns the per-run catalog. Resources are **expected to resolve there, at pre-flight** — but that
resolve pass **does not exist today**: the fresh-run pre-flight only clones the catalog, binds variables, and mints a
recovery stack (`:409–415`); no `resolve` / `VerifyExistence` / `markActive` / `markGone` appears anywhere in
`graph_executor.go`, so a cataloged resource stays `Pending`. **This is the k.13 gap.** So the fix is a **pre-flight
resolve pass in the executor** over the cloned catalog, driving `Pending → Active/Gone` — not `DiscoverResource`, and
not the provider `Resolve()` / `Observe` candidates (superseded). Revise the *Wiring* section accordingly.

## Remaining to reach `complete`

1. **The discovery-side resource lifecycle** — the plan above (file implemented + tested; the other eight stubbed).
2. **13.0(n) — the writ graph executor** — the only not-started 13.0 item; subsumed by step 33 (`writ migrate` full
   rewrite), so this umbrella closes when step 33 lands.

Detailed sub-item history: [phase-8/13.0-n.md](../13.0-n.md) (which uses its own internal sub-step numbering — not phase
step numbers).
