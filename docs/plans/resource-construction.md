---
title: "Resource construction: the catalog mediates everything"
issue: https://github.com/NobleFactor/devlore-cli/issues/581
status: draft
created: 2026-08-20
updated: 2026-08-20
---

# Plan: Resource construction — the catalog mediates everything

## Summary

Implement the resource-construction design as ruled: **the graph's resource catalog represents input intent —
it is, in effect, what must exist when the graph runs.** The catalog serializes with the graph (mandatory
section, even when empty; absence is a hard pre-flight failure). At plan time, resource-typed parameters with
string values mint **pending** entries — no existence check, no disk contact; promise values record the
promise and nothing else; products are runtime facts with no plan-time presence. Identity is the
slash-canonical **root-relative** path — plan-space paths follow the git model (a leading slash anchors at
the fsroot), and the fsroot itself is a run parameter, unknown until execution. No string-to-resource
conversion happens at run time. The design integrates into `docs/architecture/4-resource-management.md` as
the body (Appendix A deleted as rejected; the transport plan's content-only narrowing superseded), and the
sketches retire when integration is complete.

## The rulings this plan implements (2026-08-20)

1. **The graph's catalog is input intent, stored with the graph.** A resource-typed parameter's string value
   mints a **pending** entry at plan time (no existence check); a promise value records the promise, no
   entry; a product is a runtime fact, no plan-time presence. String-typed parameters (`destination_path`)
   stay plain values.
2. **Pre-flight fails hard when a graph has no resource catalog — even an empty one is mandatory.** The
   serialized section is always present; a document without it does not load, and a graph without a catalog
   does not dispatch.
3. **Identity is rel; the fsroot binds at run.** Plan-space paths follow the git model: a leading slash
   anchors at the fsroot; machine-absoluteness is inexpressible in a plan and arises only from the run's
   root choice.
4. **Activation never changes identity.** The graph is the only place pending resources are *stored*; a run
   activates them on the clone (the graph document is never mutated by a run). Pre-flight transitions
   Pending → Active by verifying each rel under the bound fsroot — a *state and binding* event, never an
   identity event. After activation the identity is the same slash-canonical rel it was as pending — it must
   be: identity is the catalog key, so rewriting it would orphan the correspondence between the graph's
   intent, the clone's active entries, and the trace's journal, and would break dedup mid-run. The
   `SourcePath` becomes the fully bound `fsroot.Path` triad: `Rel()` = the identity, verbatim; `Root()` =
   the run's fsroot; `Abs()` = derived, OS-native, carries all I/O, never serialized. **Identity lives in
   the rel; location lives in the Path; activation joins them without letting them trade places.**
5. **The design integrates into the resource-management design doc; sketches are mined and then removed.**

The demonstrations that forced the rulings (both red, both become acceptance with phase-3-corrected
assertions):

- `pkg/op/provider/plan/catalog_contract_test.go` — Go pin: plan `file.copy` from Starlark, save, reload;
  assert the source **present as pending** and the destination **absent** (a product, not intent). Today:
  the document has **no catalog section at all**.
- `cmd/devlore-test/devloretest/data/test_graph_catalog_contract.star` — the same contract front to back in
  the product's own harness (`devlore-test run`).

## Where the design already lives (the record)

| Statement | Where |
| --- | --- |
| "Inputs are strings; outputs are pending Resources. The catalog mediates everything." | docs/sketches/resource-construction.md:7 |
| String input → `catalog.GetOrCreate(uri, factory)`; the slot stores the canonical Resource | docs/sketches/resource-construction.md:13–17 |
| ~~Output shadowing at plan time~~ — **superseded 2026-08-20**: products are runtime facts; no plan-time entry | docs/sketches/resource-construction.md:23–27 |
| Pre-flight = iterate the catalog ledger; fail-fast on missing discoveries; pending skipped | docs/sketches/resource-construction.md:35–37 |
| "No string-to-Resource conversion ever happens at run time." | docs/sketches/resource-construction.md:41 |
| The graph is *defined* as carrying the planning ResourceCatalog | docs/architecture/2-execution-graph.md:24 |
| Pre-flight verifies "against the target and the catalog"; required "because a graph is planned once and executed on many machines" | docs/architecture/4-resource-management.md:42, 181–182 |
| ~~`file` URIs: RFC 8089 absolute~~ — **amended 2026-08-20**: catalog identity is root-relative (git model) | docs/architecture/4.1-resource-identity.md:182 |
| The trace already serializes a full catalog snapshot (`LedgerEntrySnapshot`: id, URI, producer, state, Etag, Digest) | step 48, landed 2026-07-16 |

## Where the implementation diverged (the defects)

| Divergence | Where |
| --- | --- |
| The graph document serializes **content-addressed entries only**; every file resource is dropped; the section vanishes entirely when no content resources exist | `packContent` — pkg/op/graph.go:895–898; `ContentResources` filter — pkg/op/resource_catalog.go:140 |
| String-typed path parameters (`destination_path`) never become resources at any point | `file.Provider.Copy` signature and every planner path |
| Products intern only at execution, into the discarded per-run clone; the stored graph never learns them | pkg/op/graph_executor.go:302 |
| Identity minted as `"file://" + Abs()` — two slashes, OS-native separators; `file://C:\…` on Windows | `buildCandidateAs` — pkg/op/provider/file/helpers.go:114 (and the 4.1:212 sketch line that mirrors it) |
| Run time re-derives resources from strings via prefix-strip at Convert | pkg/op/provider/file/helpers.go:106; pkg/op/convert.go step 6 |
| The plan-time output-claiming design parked as "design only — not implemented" | docs/architecture/4-resource-management.md Appendix A |

## Epic and issue placement

**Epic: #444 — The resource model (`Epic:ResourceModel`).**
[#581](https://github.com/NobleFactor/devlore-cli/issues/581) *"The catalog travels with the graph"* is this
plan's feature; the identity half rides the existing
[#546](https://github.com/NobleFactor/devlore-cli/issues/546) *"Paths and URIs: neutral identity, native
access"*, whose bug [#547](https://github.com/NobleFactor/devlore-cli/issues/547) covers the 3 remaining
Windows known-failures.

| Phase | Task | Feature |
| --- | --- | --- |
| 0 | [#582](https://github.com/NobleFactor/devlore-cli/issues/582) design integration + sketch disposition | #581 |
| 1 | [#583](https://github.com/NobleFactor/devlore-cli/issues/583) serialize + enforce the section | #581 |
| 2 | [#584](https://github.com/NobleFactor/devlore-cli/issues/584) rel identity + authoring migration | #546 |
| 3 | [#585](https://github.com/NobleFactor/devlore-cli/issues/585) plan-time claiming | #581 |
| 4 | [#586](https://github.com/NobleFactor/devlore-cli/issues/586) run time consumes the catalog | #581 |
| 5 | [#587](https://github.com/NobleFactor/devlore-cli/issues/587) closure | #581 |

## Phases

### Phase 0 — design integration (docs only) — status: complete 2026-08-20

1. `docs/architecture/4-resource-management.md`: new §"The catalog travels with the graph" — the serialized
   ledger (every addressing; entries as pending intent; content entries additionally carry packed bytes),
   the mandatory-even-empty section, the pre-flight hard-fail rule, and plan-time claiming per the phase-3
   ruling: resource-typed inputs only — string values mint pending entries, promise values record the
   promise, products are runtime facts. **Appendix A is deleted as rejected** (not preserved-as-parked), and
   the sketch's own output-shadowing section (:21–27) is recorded as superseded by the same ruling.
2. `docs/architecture/4.1-resource-identity.md`: amend the `file` scheme ruling — catalog identity is the
   slash-canonical root-relative path (git model: leading slash anchors at the fsroot; the fsroot is a run
   parameter); the RFC-absolute form survives only as the *rendered* form against a bound root. Fix the
   :212 sketch line with it.
3. `docs/plans/extract-starlark-from-op/phase-8/function-resource-slots-and-transport.md`: annotate step 3 as
   superseded on the reference-exclusion point (content-only transport was too narrow; the Goal's own
   "including on another host" requires the ledger).
4. Sketch disposition: `resource-construction.md` and `catalog-reconciler-logic.md` retire (deleted) once
   their content is integrated; `01/02-stack-comparison.md` integrate into
   [3.6-method-classification.md](../architecture/3.6-method-classification.md) (immediate-vs-plan dispatch
   is its subject) and retire; `package-signatures.md` is `Epic:LorePackaging` material — moves to that
   epic's design home, not this plan's scope beyond the move.

### Phase 1 — the catalog section: serialize + enforce — status: complete 2026-08-20

1. `graphData` gains a mandatory `resources` section: every current-generation ledger entry as intent —
   `{id, uri, state: pending}` (per the phase-3 ruling, plan-time entries are all Pending; no producer
   stamps, no Etag/Digest — those are trace material); content-addressed entries additionally carry their
   packed bytes as today. Present even when empty (`"resources": []`, no omitempty).
2. `LoadGraph` re-interns the section into the graph's catalog: location entries restore as pending ledger
   entries; content entries unpack as today. **A document without the section is a hard load error.**
3. Executor pre-flight asserts the graph carries a catalog before dispatch — the resolve pass's first act,
   failing the run with `ReasonPreflightFailed` when absent.
4. Round-trip pin: pack → unpack → pack byte-identical, including the empty section.
5. Windows baseline expectation: unchanged (3).

### Phase 2 — portable identity: rel, bound to the run's root (#546/#547) — status: pending

**RULED 2026-08-20: named resources are computed relative to some fsroot, and the fsroot is not known until
the run.** The motivating cases are the product's own scopes: a **home**-scope graph binds to the account
running it — `$HOME` differs per user, so the same graph deploys into `/Users/a`, `/home/b`, or
`C:\Users\c` depending on who runs it; a **system**-scope graph binds to the host — the Windows system root
is `%SystemDrive%\` (the #392 ruling), `/` on unix. One graph, late binding, per run. Identity is therefore
the **slash-canonical root-relative path** — the `fsroot.Path` serialization doctrine (`rel` is the half
that serializes) applied to the catalog. No volume, no home layout, no separator variance in identity; the
root is a run parameter, which is also what makes one graph relocatable across prefixes (writ deploy's
actual shape) and what #571's declared-roots direction extends naturally into `(root-name, rel)`.

1. Resource identity mints from `SourcePath.Rel()`; the tag URI's specific part carries the rel form.
   `docs/architecture/4.1-resource-identity.md` amends: the `file` scheme row's absolute-URI ruling is
   superseded for catalog identity by the rel ruling (and the :212 sketch line goes with it).
2. **Authored path semantics — RULED 2026-08-20, the git model: a leading slash is interpreted as relative
   to the fsroot.** Plan-space paths are a portable little language:
   - `foo/bar` and `/foo/bar` both name rel `foo/bar` — the leading slash is the anchored spelling, and
     both normalize to the same slash-canonical identity;
   - machine-absoluteness is **inexpressible in a plan** — it arises only from the run's root choice
     (`root=/` makes plan `/etc/x` mean machine `/etc/x`);
   - a volume or drive-letter spelling (`C:\x`, `\\server\share`) is malformed plan input — plan-time
     refusal;
   - a rel that escapes (`../`) is intent confinement can never satisfy — plan-time refusal.
   The planning session's own root plays **no role in identity**; it serves only immediate-mode I/O during
   planning.
3. **Authoring-surface migration** (chartered here, sized in the task issues): surfaces that today pass
   machine-absolute paths into plans switch to plan-space paths — `devlore-test`'s `t.tmp()` returns
   plan-space names with the harness supplying the root; writ deploy's planner emits rels against its chosen
   target root (it already owns that root); tests author rels. Under the new semantics a machine-absolute
   authored path double-prefixes and fails pre-flight, so the migration is forced, visible, and complete
   before phase 2 closes.
4. The four consumers that keyed on the native absolute form (the documented 2026-08-18 revert) migrate onto
   the resource's native accessor (`SourcePath.Abs()`), which the resource derives against the live root —
   the resource owns both forms; consumers stop parsing identity.
5. Pre-flight becomes one crisp sentence: **every pending rel must exist under the run's root** — one root
   handle, one pass, kernel-enforced.
6. Windows baseline expectation: the #547 family clears — `TestDiscoverRegular_ForeignSchemeString_IsAPath`,
   `TestSourceFile_StarlarkIntegration`, `TestWriteOnboardManifest_WritesTheManifestAndNamesThePath` —
   **3 → 0** if all three prove to be pure identity-form failures; any remainder is re-diagnosed, not
   assumed.

### Phase 3 — plan-time claiming: inputs only, pending only — status: pending

**RULED 2026-08-20, superseding the sketch's output section (:21–27) and rejecting Appendix A outright:**
products are runtime facts. A method that returns a resource creates it at execution; nothing about it enters
the catalog at plan time. The plan-time catalog is the graph's *input intent*, and it never touches the disk.

1. Resource-typed parameter, **string value** → mint a **pending** resource from the string into the catalog.
   No existence check at plan time; pending, never resolved (the executor's pre-flight owns transitions —
   [4-resource-management.md](../architecture/4-resource-management.md) §78–81 already rules this).
2. Resource-typed parameter, **promise value** → record the promise binding; **no catalog entry** — identity
   arrives when the producer runs.
3. String-typed parameters (`destination_path`, `mode`, `user`, …) stay plain values. No output-naming
   convention, no `+devlore:output` — there is nothing to declare because there is nothing tracked.
4. Consequence for phase 1's stored section: every stored entry is Pending by construction — `{id, uri,
   state: pending}` rows, no producer stamps, no Etag/Digest. **Graph = intent; trace = observation** (the
   step-48 snapshot keeps the observed side).
5. Acceptance: **both pins flip green with corrected assertions** — the stored document carries
   `original.txt` (pending), and asserts `duplicate.txt` **absent**, pinning the product ruling in both
   directions. **Delivered early — the pins flipped with phase 1**: planning already interned the
   resource-typed source, so serialization was the missing half and the destination's absence was already
   true. Phase 3's remaining substance is the claiming discipline itself: plan-time minting must be
   pending-only with no existence I/O (today's Discover can still resolve at plan time), and the promise
   grammar verified.

### Phase 4 — run time consumes the catalog, never strings — status: pending

1. Dispatch conversion resolves slot values against the run catalog (cloned from the now-complete graph
   catalog); the `file://` prefix-strip re-parsing in `buildCandidateAs` retires (sketch :41).
2. Products at execution update the *pending* entries the plan claimed (state transition, metadata), rather
   than minting parallel identities in a throwaway clone.
3. The catch-all string path remains only for immediate mode, which is a session concern, not a graph one
   (4-resource-management.md §8 item 3).

### Phase 5 — closure — status: pending

1. Sketches removed per phase 0's disposition; `docs/architecture` statuses updated; the transport plan's
   supersession note finalized.
2. The two demonstration pins graduate from local red to committed green (they land with phase 3's PR).
3. `4-resource-management.status.md` records the completed convergence.

## Verification

Every phase: `make check`, `make vet` under GOOS windows and linux, `gofmt -l`, and the Windows known-failure
set diffed name-for-name against the baseline (3 at `84b416f8`), with the byte-count guard. Phase 2 is the
only phase expected to move the count; any other movement is a defect in the phase that caused it.

## Judgment scenarios

The scenarios the feature is judged by. Each is authored as a prediction before implementation and graduates
to a devlore-test case; the implementation is correct when the harness observes exactly the prediction. The
first is recorded here (2026-08-20); more accrete as they are set.

### Scenario 1 — delete, then copy, the same named resource

**Setup.** Plan a graph of two operations against the same named regular file resource, rel `data.txt`:
first `file.delete(data.txt)`, then `file.copy(source = data.txt, destination_path = copy.txt)` — the source
authored as the same string in both, no promise between the nodes. The file exists under the run's root when
the run starts. Run the graph.

**Expected graph catalog — exactly one entry.** `{id, uri: data.txt, state: pending}`. The same string in
both operations dedups to one canonical identity — the catalog mediating. `copy.txt` is **absent**: a
string-typed parameter naming a product, which is a runtime fact. The whole intent: *"`data.txt` must exist
under the run's root."*

**Expected outcome — pre-flight passes; the run fails at the copy node.** Pre-flight verifies the one
pending rel under the root (Pending → Active, Etag/Digest captured) — intent was satisfied at the starting
line, which is all pre-flight claims. Delete dispatches: file removed, entry Gone, receipt taken. Copy
dispatches: source gone → node failure → run fails; the delete's receipt compensates and `data.txt` is
restored. The failure is the model working correctly: the plan encoded self-contradictory intent, and the
intent model deliberately does not simulate lifecycle at plan time. (As planned there is no data edge
between the nodes, so the contradiction is ordering-dependent — reversed order succeeds — which is exactly
why plan time cannot adjudicate it.)

**Expected trace catalog — the observed story.** `data.txt`: pending → Active at pre-flight (with captured
Etag + Digest) → Gone after the delete, with the compensation in the receipt journal. `copy.txt`: **no
product entry** — the copy never produced; the trace records the node's failure, not a resource.

**The sentence the scenario proves:** the graph says what must be true; the trace says what happened; the
gap between them is the run's story.

### Scenario 2 — relocate the tree, reconcile the graph

**Setup.** Create a graph; run it at fsroot location A. Copy the **entire tree** to a new fsroot location B
(bytes intact; inodes and mtimes new, as copies are). Reconcile the graph at B.

**Expected result — reconciliation succeeds and reports no content drift.** The graph's intent is rels, so
binding to B is fully defined: nothing in identity remembers A. Every rel exists under B. The **Etag screen
mismatches on every entry** — Etags are stat tuples, and relocation mints new inodes — which is by design:
the cheap screen escalates to the honest check. **Digests match**, so every entry classifies as **touch
drift**: Etag refreshed against B, no shadow, no repair, clean report. The graph is at home in its new root.

**The contrast that gives the scenario teeth:** under absolute identity, reconciliation at B finds zero
corresponding resources — every entry "missing," the graph unreconcilable without rewriting it. Scenario 2
is the relocation proof — the direct payoff of rel identity — and it exercises the reconciler's two-tier
cascade (Etag screen → Digest verdict) exactly as designed.

## Open questions — all ruled 2026-08-20

1. ~~Which string parameters are output-naming?~~ — **RULED: none.** Products are runtime-created; nothing
   about a method's return enters the plan-time catalog. No convention, no annotation. Appendix A is
   rejected, not parked. (Supersedes docs/sketches/resource-construction.md:21–27.)
2. ~~Does the stored ledger carry `state`?~~ — **Dissolved by ruling 1.** With no plan-time existence checks
   and no plan-time products, every stored entry is Pending by construction. Graph = intent; trace =
   observation (step 48).
3. ~~Schema version bump~~ — **RULED: stays 1.** The mandatory section is the gate by itself: every existing
   graph document fails to load and is rewritten by re-planning. No version dance, no shim.
