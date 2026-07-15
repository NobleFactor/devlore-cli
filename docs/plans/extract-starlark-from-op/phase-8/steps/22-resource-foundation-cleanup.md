---
step: 22
former_step: 19
title: "Resource foundation cleanup — the 13.0(k) arc plus sub-steps (d)–(n)"
status: in-progress — sub-steps (d)–(n) done; 13.0(k) items 1–2 done (Planned companions deleted; twelve Resource interfaces + k.12 boot test). k.13 is PARTIAL (2026-07-14 audit): the Pending/Active/Gone types exist and production→Active works (GetOrCreate/markActive), but the DISCOVERY-side Pending→Active/Gone transition is UNBUILT — DiscoverResource catalogs Pending and never verifies existence; markGone has no production caller. Plan approved 2026-07-14 (Resolve + Exists on the op.Resource interface; file real + eight assert.Unimplemented stubs; catalog-owned VerifyExistence). First implementation attempt reverted 2026-07-14; BOTH design questions since SETTLED (§ Design rulings): Issue 1 — an observation is NOT a Resource (a metadata snapshot; demoted to a plain record whose identity comes from the resource it references by pointer value; catalog plumbing removed; the Exists() collision dissolves), Issue 2 — existence resolves at runtime via the executor's pre-flight resolve pass (never the plan-time DiscoverResource path). Plan revised accordingly. SLICES A+B+C LANDED 2026-07-14/15 (A: observation demoted to a record, catalog plumbing deleted, three orphaned gen/observation.gen.go removed; B: Resolve+Exists on op.Resource with loud ResourceBase defaults, git/pkg no-op Resolve deleted, ResourceCatalog.VerifyExistence + 5 tests; C: the executor pre-flight resolve pass over the cloned catalog — staging gate = existenceVerifiableTypes type-id set (file only), Gone = mark-don't-fail — devloretest's missing-file fixtures pass end to end with the sweep live). Slice D pending (Discover doc-comment corrections + the observation-on-receipt round-trip test). assert.Unimplemented kept (committed). 13.0(n) writ graph executor (= step 33) also open.
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

## Plan — observation refactoring + the discovery-side lifecycle — approved 2026-07-14, revised same day (rulings in)

Close the k.13 discovery-side gap per the settled rulings (§ Design rulings): demote observation out of the `Resource`
family (slice A), add `Resolve`/`Exists` to the `op.Resource` interface (slice B), and drive `Pending → Active/Gone`
from the executor's pre-flight resolve pass (slice C) — never from the plan-time `DiscoverResource` path, which only
introduces resources as `Pending`. Staged — **file** is implemented and tested; the other eight types are loudly
stubbed and land in later per-type steps.

**Slice A — observation refactoring (Ruling 1: an observation is not a `Resource`). LANDED 2026-07-14.** One
discovery beyond the plan: the codegen had emitted `gen/observation.gen.go` announcement files (`op.AnnounceType` of
the observation's Resource-shaped methods) in file/git/pkg; the generator, re-run against the record shape, no longer
emits them, so the three orphaned outputs were deleted (a stale-generator-output removal, not a generated-file edit).

1. Demote `Observation` to a plain metadata-snapshot record: `ObservationBase` stops embedding `op.ResourceBase` and
   stops satisfying `op.Resource`; it keeps the `OfResource` back-link, the existence-at-observation-time flag, and
   the measurement fields. **Identity comes from the resource the record references by pointer value** (ruled
   2026-07-14): an observation mints no URI and carries no content hash of its own — `NewObservationBase` drops the
   tag-URI / `goType` / sha256 machinery, and the per-provider constructors drop their canonical-hash computation.
2. Delete the catalog's observation plumbing: `RecordObservation`, `CurrentObservation`, the `currentObservations`
   map, and the snapshot's `CurrentObservations` field + rehydrate loop.
3. Rework the three providers' observation types (`file`/`git`/`pkg` `observation.go`) onto the record shape;
   `Provider.Observe` stays an **announced action** returning the record — its result rides the receipt/trace (the
   tracked-and-traced surface; resume re-observes rather than reconstructs).
4. Test fallout: `fakeObservation` and the observation/catalog tests reshape; gen dry-run tests stay green.

**Slice B — `Resolve` + `Exists` on the `Resource` interface (collision-free after slice A). LANDED 2026-07-14.**
Tests: `TestCatalog_VerifyExistence_{PresentMarksActive,MissingMarksGone,ActiveShortCircuits}` (the `lifecycleResource`
fixture gained a counting `Exists` override) and `TestResourceBase_{Resolve,Exists}_DefaultPanicsUnimplemented`. A
pre-existing orphaned doc comment (a function-less `Resolve` description above the `// Actions` delineator in
`resource.go`) was replaced by the real default. `VerifyExistence`'s error return is informational — the caller owns
the reaction (the slice-C pass records the `Gone` mark and decides independently whether the run proceeds).

1. **`Resolve() error`** — locate the resource by URI (rebind to the execution fsroot) and verify reachability, exactly
   as `file.Resource.Resolve` (`file/resource.go:438`) does today. It is not a clean existence signal on its own (file's
   `Resolve` returns nil for a not-exist path) and it populates **no** metadata — that stays `Provider.Observe`.
2. **`Exists() bool`** — new; the existence predicate the catalog reads without interpreting `Resolve`'s error.
   `file.Resource.Exists` (`:338`) is the model (a fresh stat → true / false).
3. Both are added to the **`op.Resource` interface** (`resource.go:42`), so the catalog dispatches polymorphically —
   all nine types implement, `file` real + eight stubbed; `op.ResourceBase` supplies the loud defaults via
   `assert.Unimplemented` (per-type message via `ResourceType()`), and git/pkg's no-op `Resolve` methods are deleted so
   they fall to the default.
4. `ResourceCatalog.VerifyExistence(resource)` — reads `Exists()`, applies `markActive` / `markGone`; the transition
   stays catalog-owned (§6.1's ownership table).

**`assert.Unimplemented`** — landed (committed 2026-07-14): mirrors `assert.Unreachable`; `Unimplemented(what string)`
→ `raise(2, "unimplemented: "+what)`; a loud stub that fails fast if reached.

**Per-type.**

| Type | `Resolve()` | `Exists()` |
|---|---|---|
| **file** | real (rebind + stat; hard errors → error) | real (existing `Exists() bool`) |
| appnet · function · git · json · mem · pkg · service · yaml | `assert.Unimplemented("<pkg>.Resource.Resolve")` | `assert.Unimplemented("<pkg>.Resource.Exists")` |

**Slice C — the pre-flight resolve pass (Ruling 2: resolve at runtime, never plan time). LANDED 2026-07-15.** All nine
`DiscoverResource` functions stay introduction-only (catalog `Pending`). `GraphExecutor.resolvePendingResources` —
called from `Run`'s pre-flight after the fresh/resume branch, before `PhaseRunning` — sweeps
`ResourceCatalog.pendingEntries()` on the per-run catalog and drives each participating entry through
`VerifyExistence`. Both confirmations realized as recommended:

1. **Staging gate** — the `existenceVerifiableTypes` type-id set (`graph_executor.go`, helper region) enrolls `file`
   only; `participatesInExistenceVerification` filters the sweep, so the eight `assert.Unimplemented` stubs are never
   reached. Each per-type step adds its id; the gate dissolves when all nine participate.
2. **`Gone` = mark, don't fail** — the pass discards `VerifyExistence`'s informational error; a `Gone` resource is a
   recorded fact and consumers fail at their own dispatch (Q2). Probes are stat-class read-only, so the pass also runs
   in dry-run (the variable-binding precedent).

Proof: `TestRun_PreflightResolvesPendingResources` (participating entries probed exactly once; a non-enrolled type
never probed; a missing resource does not fail the run; the graph's planning catalog stays `Pending` — transitions
land on the clone only), plus devloretest end to end — the missing-file fixtures that broke the reverted attempt
(`file.exists` on phantom.txt, archive/encryption fake paths) pass with the sweep live.

**Slice D — tests + docs.**

1. Pre-flight resolves a plan-introduced file resource: existing file → `Active`; missing → `Gone` (run behavior per
   the confirmed `Gone` semantics); `markGone` finally reached from a production path.
2. An observation record rides a receipt: a `file.observe` result round-trips the trace (retype via the `Convert`
   cascade; the re-observe resume stance unchanged).
3. Correct `Discover`'s doc comment (`resource_catalog.go:172–179`) + the inline `:213` "preflight pass" comment to
   the settled model. (`4-resource-management.md` §6.1/§6.2 already carry the rulings — done 2026-07-14.)
4. `make test` green; standing reds (step 28 pwsh, step 33 writ) unchanged.

**Scope.** IN: slices A–D. OUT (deferred, loudly stubbed): real `Resolve` / `Exists` for the other eight types — each
a later per-type step; the panic-stub is the tripwire.

## Design rulings (settled 2026-07-14)

The first implementation attempt was reverted (it introduced two problems); both are settled by ruling.

**Ruling 1 — an observation is not a `Resource`; it leaves the catalog (resolves the `Exists()` collision at the
root).** The attempt collided: adding `Exists()` to `op.Resource` broke `ObservationBase`, whose `Exists bool` field
("present at observation time") shadowed the promoted method — `Observation` embedded `Resource`, so an observation
had to answer the resource-**now** question with a snapshot-**then** fact. The collision was the symptom of a taxonomy
error. An observation is a point-in-time **metadata snapshot** — a fact *about* a thing, not a thing whose existence
can be asked ("if you can't ask whether the thing exists, it isn't that thing"). It leaves the `Resource` family and
the catalog entirely; its **identity comes from the resource it references by pointer value** (no minted URI, no
content hash); it is tracked and traced through the execution record (receipts/trace), matching the resume stance
(re-observe-and-verify, not reconstruction). The evidence that shaped the ruling: `Observe` IS live surface (an
announced action in file/git/pkg — deletion rejected); the identity/state split is load-bearing (a `Resource` carries
identity only; runtime state lives on observations); and the catalog's observation plumbing has zero production
callers (`RecordObservation` / `CurrentObservation` / `currentObservations`), so leaving the catalog loses nothing.
Realized as slice A.

**Ruling 2 — existence resolves at runtime, in the executor's pre-flight resolve pass.** The reverted attempt wired
`VerifyExistence` into `file.DiscoverResource` — but `DiscoverResource` is the **plan-time introduction** path (the
string→`Resource` construction that param-binding uses), so verifying existence there made it a *planning*
precondition and broke planning of any operation on a not-yet-existent or intentionally-missing file (`file.exists`,
`file.write`, `archive.extract`, …); 13 tests failed this way. At plan time a resource is **introduced** from a string
(a URI or source path) and cataloged `Pending`, deliberately unresolved; it is **expected to resolve at runtime**.

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
not the provider `Resolve()` / `Observe` candidates (superseded). Realized as slice C.

## Remaining to reach `complete`

1. **The discovery-side resource lifecycle** — the plan above (file implemented + tested; the other eight stubbed).
2. **13.0(n) — the writ graph executor** — the only not-started 13.0 item; subsumed by step 33 (`writ migrate` full
   rewrite), so this umbrella closes when step 33 lands.

Detailed sub-item history: [phase-8/13.0-n.md](../13.0-n.md) (which uses its own internal sub-step numbering — not phase
step numbers).
