---
title: "Resource construction: the catalog mediates everything"
issue: https://github.com/NobleFactor/devlore-cli/issues/581
status: complete
created: 2026-08-20
updated: 2026-08-22
---

# Plan: Resource construction — the catalog mediates everything

## Summary

Implement the resource-construction design as ruled: **the graph's resource catalog represents input intent —
it is, in effect, what must exist when the graph runs.** The catalog serializes with the graph (mandatory
section, even when empty; absence is a hard pre-flight failure). At plan time, resource-typed parameters with
string values mint **pending** entries — no existence check, no disk contact; promise values record the
promise and nothing else; products are runtime facts with no plan-time presence. **File-resource** identity is the
slash-canonical **root-relative** path — plan-space file paths follow the git model (a leading slash anchors
at the fsroot), and the fsroot itself is a run parameter, unknown until execution; other schemes keep their
own identity forms (purl, service name, opaque, digest) and existence predicates. No string-to-resource
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
3. **File identity is rel; the fsroot binds at run.** This ruling is scoped to **file resources** — the
   full panoply (pkg, svc, git, appnet, the CAS types) keeps its own identity forms and existence
   predicates. Plan-space file paths follow the git model: a leading slash anchors at the fsroot;
   machine-absoluteness is inexpressible in a plan and arises only from the run's root choice.
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

*Status 2026-08-22: **every row is resolved.** Rows 1, 2, 4, and 6 closed with phases 0–3; rows 3 and 5 —
products interning into the discarded per-run clone, and run-time string re-parsing at Convert — closed
with phase 4 (#613/#614/#615). The divergence table is history.*

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
| 4 | [#586](https://github.com/NobleFactor/devlore-cli/issues/586) run time consumes the catalog — PR tasks [#609](https://github.com/NobleFactor/devlore-cli/issues/609), [#610](https://github.com/NobleFactor/devlore-cli/issues/610), [#611](https://github.com/NobleFactor/devlore-cli/issues/611) | #581 |
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

### Phase 2 — portable file identity: rel, bound to the run's root (#546/#547) — status: complete 2026-08-21 (PR 1 #596 `92c18eb1`; PR 2 #600 `2c5f6e6a`; PR 3 #601 `70d540c6` — all merged)

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

**Review findings (2026-08-21, from the live CI logs and the code):**

- Failure diagnosis: `TestDiscoverRegular_ForeignSchemeString_IsAPath` is **pure identity-form** (the tag
  URI embeds the backslashed machine-absolute path) — the identity mint fixes it directly.
  `TestSourceFile_StarlarkIntegration` is a machine path **interpolated into generated Starlark source**
  (`\U` reads as an invalid escape) and `TestWriteOnboardManifest…` is **mixed-separator narration**
  (`C:\…\001/packages-manifest.yaml`) — both are path-as-text at their surfaces, cured in the
  authoring-migration step, not by the mint.
- Sequencing constraint: **the grammar refusals and the harness migration must co-land** — once volume
  spellings are plan-time refusals, every `t.tmp`-authored script fails to plan (`t.tmp` returns
  `filepath.Join(tmpDir, name)`, machine-absolute). The identity mint has no such coupling: `SourcePath`
  already carries `Rel()`, so the mint lands first and alone.
- Providers already use `SourcePath.Abs()` at their I/O sites; step 4's four consumers are a bounded hunt.

**PR slicing:** PR 1 — the identity mint (URI specific part carries `SourcePath.Rel()`), the 4.1 amendment,
re-pinned format-identity tests; Windows 3 → 2. PR 2 — the plan-space grammar with both refusals plus the
full authoring migration (`t.tmp`, writ deploy, tests) and the re-diagnosed fixes for the two path-as-text
failures; Windows 2 → 0. The grammar also **reserves the root-qualification spelling** (#597 — named
multi-root design, filed 2026-08-21): a shape like `@name/rel` that cannot collide with the drive-letter
refusal, decided now so the little language does not break when multi-root lands. PR 3 — the four consumers onto `SourcePath.Abs()`, pre-flight as the one sentence,
closure.

**PR 1 record (2026-08-21):** the mint flips to the ruled opaque form — `"file:" + SourcePath.Rel()`
(4.1's amended row and sketch, already landed in phase 0). The empirical consumer hunt found ONE seam, not
four: writ's readback (`recordedIdentity`) parsed the URI's embedded path and keyed drift attribution by it;
everything else already used `SourcePath.Abs()`. The fix is design-true: `ResourceLedgerSnapshot` gains
`Root` — the run's bound fsroot, stamped by the executor at capture (§5.5: the trace records the binding) —
and readback joins the recorded root with the rel to derive its native keys. The foreign-scheme pin
re-asserts against the rel form (`file:https:/example.com/x`), which also makes it platform-neutral —
expected to clear on Windows. Escaping rels (`../…`) still mint until PR 2's grammar refuses authoring them.
Gate: make check 103 ok / 0 fail, GOOS=windows vet clean.

**Run-from-elsewhere caveat (USER, 2026-08-21): PR 3 is load-bearing, not closure hygiene.** The existence
check reads `r.RuntimeEnvironment().Root()` — the environment the resource object was *constructed*
against — and feeds `SourcePath.Abs()`, the construction-time absolute (`file.Resource.Exists`/`Resolve`;
`Resolve` even re-binds abs-first). Plan-and-run-in-one-session flows hide this because the roots coincide;
**save the graph and run later from elsewhere and they don't** — the run verifies against the load
environment's root, never the run's. PR 1 made the identity payload relocatable; PR 3 must make the
*verification* relocatable: pre-flight binds every pending rel against the run's root (rel-first activation
binding at the executor's resolve pass, where the run environment is in scope), and Abs derives from that
binding.

**The sixth discovery (2026-08-21, caught by the foreign-scheme pin on Windows):** `fsroot.makePath`'s rel
half was not platform-neutral — Windows `filepath.Clean` guards a colon-bearing first segment with a
leading `./` (drive-relative disambiguation), so the same input minted `./https:/example.com/x` on Windows
and `https:/example.com/x` elsewhere. Fixed where the capability lives: `fsroot.canonicalRel` (slash-path
Clean after separator normalization) renders every rel in both `makePath` branches and `fsroot.NewPath`,
with a platform-neutrality pin in the fsroot suite. Exactly the class #547 predicted ("the sixth is
waiting"); recorded on #547.

**PR 2 record (2026-08-21):** the plan-space grammar and the full co-landing, one PR as the sequencing
constraint demanded.

- **The grammar** (`file.NormalizePlanSpacePath`, registered per resource type via
  `op.RegisterPlanPathNormalizer`, applied at the planner's claiming seam — `normalizePlanSpaceValue` in
  `bindPresentValue`): `foo/bar` ≡ `/foo/bar`; volume/UNC/backslash spellings refuse; `@name/…` refuses
  as reserved for #597; escapes and the bare root refuse. One table pin covers every rule. Immediate-mode
  and programmatic construction are untouched — the grammar governs what a plan may say.
- **One root everywhere in devlore-test**: `t.tmp` returns plan-space rels (`.devlore/tmp/<name>`), the
  run anchors at the same workspace root as the script session, and the harness's Go-side helpers resolve
  rels through one `resolve` seam. Machine-absolute authored paths died by double-prefix exactly as
  predicted — the migration was forced, visible, and complete.
- **writ emits rels**: `PlanFileChain` renders source/origin/target through the new `deploy.PlanSpacePath`
  (rel against the planning environment's root — the run root writ already chooses); the encrypt planner
  follows; graph-origin annotations keep machine-absolute paths for the readback fold's keying.
- **Process-cwd leaks fixed as discovered**: `shell.exec`/`powershell.exec` anchor the command's cwd at
  the run root; `file.glob` resolves relative patterns against the root; `plan.save_definition`/
  `load_definition` resolve relative document paths against the session root.
- **The two Windows surfaces fixed**: the star integration fixture interpolates its path slash-form into
  starlark source (paths-in-text are neutral text); lore's onboard manifest path uses a native join
  instead of string concatenation.
- Gate: make check 103 ok / 0 fail (grammar pins + all scenario stars green), GOOS=windows vet clean.
  **Windows expectation: 2 → 0 — the first fully green CI of the campaign.** Any Windows remainder is
  re-diagnosed, not assumed.
- **First CI re-diagnosis (2026-08-21):** two names, neither assumed. `TestPlannedCopy_…` was the last
  unswept authoring surface — the Go judgment pin interpolated machine-absolutes into its star fixture and
  the grammar refused them on Windows exactly as designed; it now authors rels, which is also truer to its
  subject. `TestSourceFile_StarlarkIntegration`'s escape failure was CURED by the slash-form fix — the
  residue was a Windows handle leak: the test never called the documented `Application.Close`, so the
  session root pinned the temp dir at cleanup (the same class as the TestLintCopyright fix).

**PR 3 record (2026-08-21) — the activation binding; phase 2 closes.** The run-from-elsewhere caveat is
implemented: the executor's pre-flight resolve pass re-bases every pending entry onto the run's environment
and re-binds root-relative schemes rel-first through the new `op.RootBinder` seam (`file.Resource.BindRoot`;
`Resolve`'s abs-first rebind — the documented inverse — flips rel-first). The consequence stays
mark-don't-fail; phase 3 owns the transition-failure semantics. Pinned in both directions
(`TestRun_BindsPendingResourcesToTheRunRoot`, `TestRun_APendingRelAbsentUnderTheRunRootIsGone`): a graph
planned under root A and executed under root B verifies and observes under B — the trace records B as the
binding, the rel is Active when the file exists only under B, Gone when only under A. Step 4's consumer
sweep completed across PRs 1–3: readback (PR 1), providers already on the accessor, and Resolve's rebind
(this PR). Windows expectation: green stays green — the baseline is now zero. **The Windows CI leg went
28 → 0 during this phase; #547 closed.**

### Phase 3 — plan-time claiming: inputs only, pending only — status: complete 2026-08-22 (PRs A #602, B #603, C #604, D #605, C2 #606 — all merged)

**NOTE (USER, 2026-08-22): two e2e lore scenarios join this phase's development — Docker and Go Toolchain,
each covering deployment, upgrade, reconcile, and decommission. Docker first. Expected to take many passes;
development starts right away alongside the phase-3 design work. Parts were previously spec'd — locate and
fold in when the work begins.**

**RULED 2026-08-20, superseding the sketch's output section (:21–27) and rejecting Appendix A outright:**
products are runtime facts. A method that returns a resource creates it at execution; nothing about it enters
the catalog at plan time. The plan-time catalog is the graph's *input intent*, and it never touches the disk.

Items 1–3 of the original docket (string → pending, promise → recorded, strings stay plain) were
**delivered by phases 1–2 and are pinned green**. The live docket, consolidated from the #585 rulings:

1. **The claims taxonomy + scoped verification (RULED 2026-08-22).** Required by default; per-consumption
   tolerance via **`MissingResourcePolicy`** — `Stop` (0, the fail-safe zero value and default: missing
   fails the scope), `Ignore` (make the call; the provider handles absence, the receipt records it),
   `Skip` (do not dispatch; recorded as skipped) — **a warning always produced on detection**. The
   parameter's TYPE is the declaration (announce-time linkage to the single consumed parameter; ambiguity
   refuses) — no directive. Aggregation: Stop wins. Conditionality is **structural** — a graph is an
   object holding a root subgraph, there is only subgraph execution, and **each subgraph executor verifies
   the claims its own units consume when its scope starts** (a choose case verifies only when hit). Strict
   case stays: promise-less unconditional consumption of a mid-run product fails pre-flight. Open
   consequence for the implementing PR: what a skipped unit's promise consumers see.
2. **Pre-flight fails on unmet required intent** — the Q1 consequence, implemented per the taxonomy.
   Acceptance: `TestJudgmentPreflightFailFast` un-skips and flips green; the binding pin's Gone direction
   updates from dispatch-failure to the pre-flight verdict.
3. **Mutation targets go resource-typed** (Remove, Unlink; Move's source and RemoveAll sized at execution)
   with a `MissingResourcePolicy` parameter on the mutators; the authoring sweep renames `path` → `target`;
   writ decommission authors a non-Stop policy — a vanished target decommissions as a recorded no-op
   (ruled; the exact constant, Ignore vs Skip, chosen at this PR).
4. **The consumed-Gone guard** at the dispatch seam (post-conversion, pre-forward-call), honoring
   tolerance, with the **destroyer stamp** on `MarkGone` so the verdict names both units.
5. **Stateless intent rows** — `IntentEntry{ID, URI}`; presence IS the claim (ruled 2026-08-21).

**Directive-inventory rulings (USER, 2026-08-22), chartered here and sized at execution:**

- `+devlore:planner` **retires — inferred**: a package type named `<MethodName>Planner` implementing
  `op.Planner` links by convention (verified 4/4 on today's uses). No directive required.
- `+devlore:struct_param` **removed**: consumed by nothing (two declaration sites, zero readers), and
  `Convert`'s struct hydration already performs the conversion.
- `+devlore:lifetime` **removed**, with the dormant `Lifetime` machinery (`pkg/op/provider/lifetime.go`).
- `+devlore:access` retires separately per `3.6-method-classification.md` (its own design; no shim,
  removed in one pass, per the governing principle).
- Tolerance carries **no directive** — the `MissingResourcePolicy` parameter type is the declaration.
- The "wire" comment vocabulary ("wire parameter token" etc.) renames to parameter-token/announce
  vocabulary — cleanup pass run 2026-08-22 (background).

Surviving directive set: `+devlore:defaults`, `+devlore:property`, `+devlore:root` — everything else
derives from types, signatures, names, and graph structure.

**Execution status (2026-08-22): EXECUTED — the removals are in the tree.** The wire-vocabulary rename ran
earlier (zero matches). The removals landed the same day: `+devlore:planner` retired — the generator links
planners by convention (a package type named `<MethodName>Planner` is the method's planner), and the flow
gen file is byte-identical under inference, proving the convention reproduces the directive;
`+devlore:struct_param` removed — one premise correction: "zero readers" was false (the generator read it,
and its one surviving effect was `cfg`'s optionality), so `cfg` is now required per the required-by-default
posture and the one omitting fixture passes `cfg={}`; `+devlore:lifetime` removed with the dormant
`Lifetime` machinery and the generator's parsing (3.1-provider-loading.md records the ruling and marks its
lifetime sections rejected). `+devlore:access` retires separately per 3.6, as ruled. Gate: make check
103 ok / 0 fail, vet clean under linux/darwin/windows.

**PR B record (2026-08-22) — scoped verification lands; the fail-fast scenario flips green; a stranded
catalog exposed.**

- `MissingResourcePolicy` ships (`pkg/op/missing_resource_policy.go`): explicit values, Stop = 0 fail-safe,
  canonical lowercase document forms, `UnmarshalText` for authored strings; round-trip pinned.
- The pre-flight pass splits: `bindPendingResources` (binding only, every entry) stays at Run's start;
  **verification moved to `Subgraph.Execute`** — `verifyScopeClaims` walks the scope's own units' immediate
  resource slots, warns on every detection, and fails the scope under Stop with a reason-carrying failure
  (`ReasonPreflightFailed`). The root graph passes through the same seam — one starting line for every
  scope, exactly the ruled uniformity.
- **Acceptance delivered: `TestJudgmentPreflightFailFast` un-skipped and green** — the copy's missing
  source fails the run with the catalog's verdict before any dispatch. The new scoped-claims scenario
  (`test_judgment_scoped_claims.star`) pins the other direction: an unreached choose case's missing claim
  is never judged and the run succeeds.
- **The headline catch: five Go-side assemblers stranded their catalogs.** writ deploy/upgrade/
  decommission/encrypt and lore's builder assembled via `op.NewGraph(...)`, which supplies a FRESH catalog
  — every claim interned during planning stayed in the environment's catalog, and the serialized graphs
  carried empty `resources` sections since phase 1. Scoped verification exposed it (verify saw slot
  resources the binding pass never touched: "file already closed"). All five now attach the planning
  environment's catalog; writ and lore graphs finally travel with their claims.
- The deliberately strict case bit two star fixtures authored as promise-less name-coincidence ordering
  (`test_source.star`, `test_write_and_read.star`) — both re-authored legal (pre-existing file; promise-fed
  read), each carrying the refusal's why.
- The old global-pass pin split into two ruled pins: claim-driven probing (consumed probed once; unconsumed
  never; unenrolled never) and the Stop consequence (unmet intent, `preflight_failed` terminal, pristine
  planning catalog).
- Gate: make check 103 ok / 0 fail, GOOS=windows vet clean. Skip/Ignore behavior at dispatch is PR C/D
  work (the mutators gain the policy parameter in C; the guard honors it in D).

**PR C record (2026-08-22) — the mutators go resource-typed; scenario 1 completes its ruled shape.**

- `file.Remove(target *Regular, onMissing MissingResourcePolicy, …)` and
  `file.Unlink(target *SymbolicLink, onMissing, …)`: the target is a consumed, claimed resource; a missing
  target follows the policy (Stop errors; Ignore/Skip no-op at the provider — Skip's do-not-dispatch half
  is PR D's guard). Both policy directions pinned. `on_missing?=stop` defaults via a new capability the
  change forced: `parseDefaultExpression` learned named-type text vocabulary (TextUnmarshaler before the
  kind switch), so an enum default is spelled `stop`, never an ordinal.
- **Scenario 1's single entry now carries both consumer links** — the remove's target and the copy's source
  claim and deduplicate to the one row the star asserts; the interim note in the scenario retires.
- The sweep: writ decommission emits plan-space rels with **`on_missing="ignore"`** (the ruled posture —
  Ignore over Skip so the run RECORDS the no-op; a hand-removed target decommissions as history, not
  silence); writ migrate's builder renames its key; four star scripts and the file provider's direct-call
  tests move to typed targets.
- **Second latent catch: `SymbolicLink.Exists` followed the link.** Claim verification statted THROUGH a
  deployed link to a target outside the run's root and falsely marked the LINK Gone — decommission then
  refused its own unlink at conversion ("resource is known-gone"). A link's existence is the link:
  `SymbolicLink` now overrides `Exists` with lstat semantics. Second kind-semantics defect exposed by
  turning verification on.
- Deferred to PR D as chartered: the dispatch guard (Discover's hit-Gone verdict routed through the
  consumer's policy; the destroyer stamp). `Move`'s source and `RemoveAll` stay path-typed — chartered as
  the C2 follow-up rather than folded, per the sizing rule.
- Gate: make check 103 ok / 0 fail, GOOS=windows vet clean.

**PR D record (2026-08-22) — the guard, the stamp, and the Skip drop; phase 3 completes its docket.**

- **Skip is DROPPED (USER ruling)**: its undo story is trivially clean — nothing ran, nothing to undo —
  but its forward side (nil promises downstream; a trace that cannot tell "skipped" from "ran and produced
  nothing") buys machinery Ignore never needs; choose cases already express optional steps structurally.
  The enum is `{Stop 0, Ignore 1}`; "skip" now refuses at parse (pinned).
- **The destroyer stamp**: `MarkGone(r, destroyerID)` records the mutator's authorship (symmetric with
  producerID); the trace row gains `destroyed_by`; reactive Gone transitions stamp nothing.
- **The guard** lands at the ruled seam (`Method.Invoke`, post-conversion pre-forward-call): a consumed
  Gone entry warns and routes by policy — Stop fails on the **narrated verdict** ("consumes X, destroyed
  by unit N before it could run"), Ignore proceeds to the provider.
- **Scenario 1 sharpens to the narrated verdict** (`expect_error("file.copy.*destroyed by")`) — the full
  ruled semantics proven end to end. The new gone-tolerance scenario pins Ignore: two removes of one file,
  the second `on_missing="ignore"`, one deletion + one recorded no-op, run succeeds.
- Gate: make check 103 ok / 0 fail, GOOS=windows vet clean. Phase-3 remainder: C2 (Move's source,
  RemoveAll) chartered.

**C2 record (2026-08-22, merged as #606) — the last mutators; phase 3 closes without remainder.**

- `file.Move(source *Regular, destination_path, on_missing)` — a move destroys the source location, so the
  source claims, and success marks it Gone with the destroyer stamp. `file.RemoveAll(target *Directory,
  on_missing, prune, boundary)` — policy-gated like its siblings; missing-target pin rewritten to both
  directions. `Backup` delegates through the typed Move.
- The strict case caught one more star (`test_mkdir_and_remove_all` created its tree mid-run and removed
  it by name) — re-authored: `remove_all` consumes mkdir's promise.
- writ migrate's register emits its consumed source in plan space; one recorded kind-looseness: the layer
  registration moves a DIRECTORY through the `*Regular`-typed claim — dispatch observes the actual kind;
  kind-honest claims for directory moves ride the 3.6 method-classification work.
- Every file-scheme mutator is now a resource-typed consumer. Gate: make check 103 ok / 0 fail,
  GOOS=windows vet clean.

**Superseded 2026-08-23 by [#616](https://github.com/NobleFactor/devlore-cli/issues/616) (AnyEntry).** This
phase's `file.move_directory` existed only because a claim had to name a kind, and the same constraint had
just cost `file.move` the ability to move a symbolic link. #616 gave the taxonomy a claim that asserts
existence without asserting kind (`file.Any`), after which `move_directory` and `unlink` both retired into
kind-indifferent `move` and `remove`. The record is kept rather than rewritten: the kind-honest activation this
phase introduced is what made the gap visible, and the gap is what produced the better shape.
4. Consequence for phase 1's stored section: every stored entry is Pending by construction. **RULED
   2026-08-21: the stored row drops `state` entirely** — the intent row becomes its own `{id, uri}` type
   (presence IS the pending claim), splitting from the trace's `LedgerEntrySnapshot`, whose
   state/etag/digest/producer vocabulary stays where observation lives. **Graph = intent; trace =
   observation** (the step-48 snapshot keeps the observed side). Fallout, small and known: the
   round-trip pins re-pin (bytes change; old documents die, schema stays 1); the star pins asserting
   `state == "pending"` flip to asserting the field's absence; the Go catalog pin keeps presence/absence
   and drops its state assertion; writ readback is untouched (it reads the trace). Known boundary: lenient
   decoding means a stray hand-edited `state:` is ignored, not refused — a codec-wide property, noted.
   **Delivered — PR A #602:** `IntentEntry{ID, URI}` is its own type, the round-trip pins re-pinned, and
   the star assertions flipped to asserting the field's absence.
5. Acceptance: **both pins flip green with corrected assertions** — the stored document carries
   `original.txt` (pending), and asserts `duplicate.txt` **absent**, pinning the product ruling in both
   directions. **Delivered early — the pins flipped with phase 1**: planning already interned the
   resource-typed source, so serialization was the missing half and the destination's absence was already
   true. Phase 3's remaining substance shrank on audit (2026-08-20): `Discover` already performs no
   existence I/O — its body says so and the executor's pre-flight owns transitions — so phase 3 is the
   promise grammar verified plus pins for the no-I/O rule and the pre-flight transition-failure semantics
   (a pending resource that does not exist under the run's root fails the run — Q1 ruling). **Delivered —
   the fail-fast pin flipped green in PR B #603.**

### Phase 4 — run time consumes the catalog, never strings (#586) — status: complete 2026-08-22 (PRs 1 #613, 2 #614, 3 #615 — all merged; #609/#610/#611 closed; the explicit-conversion suite is 13/13 green)

**The rule this phase implements** (the sketch's :41 rule, now 4-resource-management.md §2/§5): **no
string-to-resource conversion ever happens at run time** — made precise: at graph dispatch a string may be
a **key, never a constructor**. A resource-typed slot value — captured object or rehydrated URI string
alike — resolves against the run catalog (the clone of the now-complete graph catalog), and resolution
retrieves the entry the plan already claimed or refuses; it never mints. Construction from strings survives
in exactly two places: load-time rehydration (a provider decoding its own emitted identity — an identity
decode, not a conversion) and immediate mode (a session concern, 4-resource-management.md §9 item 3).

**The three findings the steps rest on** (from the code, 2026-08-22):

- `Convert` steps 1–2 (identity/assignability) return a live captured `Resource` slot value as-is — the
  run catalog is never consulted at live dispatch. That works today only because `ResourceCatalog.Clone`
  shares Resource pointers (`pkg/op/resource_catalog.go` — entries copied as an interface slice; state
  lives in per-clone maps), so pre-flight's `BindRoot` mutation reaches the captured object by aliasing —
  a load-bearing accident, not design.
- `Convert` step 6 (`tryConstructResource`, pkg/op/convert.go) resolves a URI-string catalog hit correctly
  (the reload path), but a **miss falls through to fresh construction** — `buildCandidateAs`
  (pkg/op/provider/file/helpers.go) strips the provider's own `file:` prefix off arbitrary strings and
  mints a state-less, unclaimed resource mid-run.
- Products already intern through the run clone's `Resolve` reconciler at the provider layer, and the
  trace snapshots that ledger (step 48) — the original "discarded clone" divergence (table row 3) predates
  the graph=intent/trace=observation split, so item 2 of the original stub is an audit-plus-pin, not a
  rewrite.

Steps:

1. **Dispatch resolves by identity.** At graph dispatch, a resource-typed slot value — captured object or
   rehydrated URI string — resolves through the run catalog (`Current`/`Lookup`) so the dispatched object
   IS the run clone's entry: re-based, state-carrying, the same row pre-flight verified. First discovery
   of the implementing PR: where the seam lives (slot-fill vs a step-6 refinement) and how the environment
   names run-vs-session mode (an existing distinction on `RuntimeEnvironment`, or a new bit).
   **Discovery resolved (2026-08-22, USER-corrected): the seam is slot-fill at `Method.Invoke`, keyed off
   the activation.** `ActivationRecord` already carries dispatch kind as documented contract — `Graph` is
   non-nil exactly during graph dispatch (`Stack` and `CallerID` agree) — so the discrimination lives on
   the per-dispatch frame, NOT on the environment (a first cut added a `GraphDispatch` bit to
   `RuntimeEnvironment` and was reverted: the environment stays orthogonal ambient capability — the
   session/dispatch distinction is the activation's to carry) and NOT inside `Convert`'s cascade (which
   stays context-blind, serving planning and immediate mode unchanged). `WithCatalog` renamed
   `WithResourceCatalog` in passing (builder names its field). Known boundary, accepted: a resource-typed
   field nested inside a struct-hydrated parameter would convert inside `Convert`'s recursion, below the
   seam — no such parameter exists today; recorded like §5.4's lenient-decoding boundary.
2. **The miss becomes a refusal.** With the catalog complete by construction (every resource-typed input
   claimed at plan time), a graph-dispatch catalog miss is a typed error naming the URI — the catalog's
   verdict, before any disk contact. The fall-through to fresh construction gates on immediate mode only.
3. **Rehydration splits from conversion.** LoadGraph's re-interning constructs typed resources through the
   provider's rehydration path, which legitimately decodes its own `file:<rel>`; the catch-all
   `TrimPrefix` at the dispatch-facing `buildCandidateAs` seam retires with the dispatch string path.
4. **Clone's sharing is decided, not inherited.** Once dispatch resolves by identity, the pointer-aliasing
   between planning catalog and run clone stops being load-bearing — the implementing PR rules whether
   `Clone` deepens or the sharing stays as documented behavior. The pristine-planning-catalog pin extends
   to cover location: post-run, the planning session's objects still bind the planning root.
5. **Products reconcile against claimed entries — audit + pin.** Verify the §3 production matrix end to
   end on the run clone: a product at a claimed URI reaches the claimed entry (touch → Etag refresh; real
   change → shadow with `producerID`); a product at a fresh URI appends Active with the producer stamp;
   the trace tells the story. Fix what the audit contradicts; pin both directions.
   **Corrected by the PR 2 audit (2026-08-22): the parenthetical conflated §4.1's Resolve cascade with
   production.** The ruled matrix is unconditional for location production: a product at an occupied URI
   — claimed, produced, or Gone — SHADOWS (fresh generation, producer stamp, prior generation as
   history); the touch/Etag-refresh cascade is `Resolve`'s cache-hit behavior, not production's.
6. **Immediate mode unchanged, pinned.** The session string path stays: immediate file ops construct and
   `Discover`-intern into the session catalog; the step-2 refusal never fires there.
7. **Acceptance.** New judgment scenario — *save, reload, run*: every resource-typed slot dispatches the
   section-rehydrated catalog entry (object identity asserted), and a doctored slot URI that misses the
   catalog fails the run with the step-2 verdict — destination untouched, no disk contact from the miss.
   Judgment scenario 2 (relocate + reconcile) stays a recorded prediction — it needs the drivable
   reconcile surface, closure era.
8. **Windows expectation: green stays green** (baseline 0); any red is re-diagnosed, not assumed.

**The explicit-conversion docket (USER rulings, 2026-08-22).** The step-2 refusal closes the mint but
leaves run-computed paths — a regex over tool output, an opaque command's side-effect file — with no
sanctioned channel. Ruled: the conversion becomes an EXPLICIT operation, never implicit dispatcher
orchestration (an implicit discovery would turn the conversion seam into a third disk-touching site with
none of the unit machinery — no activation record, no receipt, no policy surface, no narration — and
`Convert` also serves plan time and immediate mode). Production and discovery are the sanctioned channels;
the dispatch seam only ever looks up. **Design-doc integration landed 2026-08-22**: the docket records in
4-resource-management.md as §2's dispatch bullet, §5.6 (key, never constructor), §5.7 (explicit discovery
and resolution — the seven rules), and §9 items 15–18.

1. **Two new file actions.** `file.discover(path, kind?="entry")` — lstat: the entry itself, no follow —
   and `file.resolve(path, kind?="entry")` — stat: what the chain designates, which is never a link;
   confinement-judged. `kind` is a named enum (`entry`, `regular`, `directory`, `symbolic_link`) with
   explicit values and `UnmarshalText`, the `on_missing?=stop` pattern. Results intern as discoveries
   (observed facts, no production claim — the taxonomy's existing category). The `entry` default makes the
   short spelling permissive; asserting a kind is opt-in strictness whose verdict sharpens at the action's
   own node. The enum shape was chosen over a kind-typed method family because only stat semantics with a
   kind argument can express "the computed path designates regular content, follow if needed" — the
   maybe-link case a kind-typed `discover` cannot say; the cost, knowingly carried, is that the declared
   result type is `Entry`, so an asserted-vs-consumed kind mismatch surfaces at the consumer's conversion
   instead of `ValidateGraph`.
2. **Stop-only — no `on_missing` parameter.** A missing target, kind mismatch, dangling chain, or
   confinement escape is the action's own error. An Ignore would return nothing and put a nil promise in
   every downstream slot — the exact machinery cost that had Skip dropped from the policy enum (#605);
   tolerance stays structural (probe + choose) or at the consumer.
3. **The runtime path grammar: plan-space plus under-root rebase.** Rels and anchored spellings normalize
   as authored; escapes and the `@name` reservation refuse as authored; a machine-absolute input rebases
   to its rel when it falls under the bound run root and refuses as a confinement violation otherwise.
   Run time may speak absolutes because the root is bound — phase 2's own ruling ("machine-absoluteness
   arises only from the run's root choice"), read from its far side; the doc states why run time may
   speak absolutes when plans may not.
4. **The follow doctrine.** Kinds are lstat-strict at consumption (step 23 ruling 5e), and the parameter
   type is the follow-policy declaration: `*Regular`/`*Directory` demand that kind, no follow;
   `*SymbolicLink` is the link itself; `Entry` accepts any kind, with the method assuming the kind-switch,
   confinement judgment, and interning duties for any follow it performs (precedent: `Observe(Entry)`).
   Implicit follow at the dispatch seam never happens: a silent follow aliases one disk entity under two
   catalog identities — mediation cannot see the join, so Gone-marking misses consumers — and a symlink is
   the disk's `../`, escaping the confinement the grammar enforces. The design-doc sentence: the kernel
   resolves names implicitly at open; this model resolves designation explicitly at a unit.
5. **An authored string into an `Entry`-typed slot refuses at plan time.** A claim asserts a kind and
   `Entry` asserts none (mechanically: plan-time claiming constructs the parameter's type, and an
   interface cannot be instantiated). Shaped refusal, not an incidental construction error; the author
   states the kind or feeds a discovery.
6. **Literal paths are legitimate input; the discriminator is starting-line vs mid-run existence (RULED
   2026-08-22).** The `path` parameter is string-typed, so the input is a literal, a promise, or anything
   the conversion cascade takes to string; the runtime grammar governs whatever arrives. Doctrine, as
   guidance not enforcement (no plan-time test can tell the cases apart): a file that must exist when the
   run starts is CLAIMED (a resource-typed slot; pre-flight's verdict); a file that comes into being
   mid-run — an opaque command's side effect at a known path — is DISCOVERED. Judgment scenario authored
   (`test_judgment_discover_after_exec.star`, wired skipped as `TestJudgmentDiscoverAfterExec`): exec
   writes, discover interns, consumer reads — whose sharp assertion is the ordering edge: list position
   does not order execution (the promise-ordering scenario's proof), so the exec→discover sequencing must
   be an explicit edge; the mechanism for a pure ordering edge is the implementing PR's decision.
7. **Claims are true when made (USER invariant); falseness is a mediation failure.** Four doors: false at
   birth — the activation gap (the kind-blind base `Resource.Exists` follow-stats through links and wrong
   kinds, pkg/op/provider/file/resource.go:227; activation's best-effort Etag/Digest capture swallows the
   5e `kindMismatchError`, pkg/op/resource_catalog.go:569–573); unmediated in-model production (step 5's
   audit closes it); opaque mutators; out-of-band change (both irreducible — the observation layer is
   their backstop). Chartered fix for door one: **kind-honest activation** — per-kind `Exists` (lstat +
   kind test, the PR C `SymbolicLink` override extended across the taxonomy), and the capture
   kind-mismatch becomes a verdict, not a swallowed error. Rider: audit writ for flows claiming linked
   paths as `*Regular` — writ claims links as links; re-author any found.
8. **The fail-fast boundary is stated, not overclaimed (RULED 2026-08-22).** Pre-flight's verdict covers
   CLAIMS — unmet intent fails before any dispatch. A discovery verifies at its own node, the earliest
   moment the fact exists, so discover/resolve failures are mid-run by nature; the design doc states this
   boundary beside the discover/resolve material so the fail-fast guarantee reads at its true scope.
   Companion fact of life, owned in the same breath (item 7's fourth door): nothing stops an out-of-band
   actor deleting a file under a running graph, short of a lockdown on the targeted fsroot directory —
   the observation layer and reconciliation are the designed response, not prevention.
9. **Run-start claiming for variable-fed resource slots (RULED 2026-08-22 — sequenced after phase 4).**
   A variable is resolvable the way a promise is: binding occurs only after execution has begun, so its
   claim belongs to the run's pre-flight — the chartered pass normalizes variable-fed resource slots
   through the grammar, mints pending entries into the run clone at the consuming subgraph's pre-flight,
   and verifies them with the scope's claims. Until it lands, the interim posture (PR 1/#609):
   `ValidateGraph` refuses a PLAIN variable into a resource-typed slot — flag/config/environment sources
   are string-valued by construction, so it can never succeed — while the reserved gather frame (`item`)
   is exempt: its records are plan-authored data that may carry claimed resources (the writ-adopt shape),
   and the dispatch seam backstops the string case.

**PR 1 record (2026-08-22, merged as #613 — develop `4fd1cd64`; #609 closed) — the dispatch seam lands on
the activation; the refusal's first catches; steps 1–4 delivered.**

- **The seam**: `Method.Invoke`'s slot conversion routes resource-typed parameters through
  `resolveDispatchResource`, gated by `activation.Graph` — the per-dispatch frame carries dispatch kind
  (step 1's resolved discovery; the environment stays orthogonal). A Resource value resolves by its URI;
  a string resolves as the key it is; any other type refuses; a run-catalog miss refuses with the §5.6
  verdict. Step 6's reload probe retired with it (its job moved to the seam), `Convert` stays
  context-blind, and `buildCandidateAs`'s prefix strip is documented as serving exactly rehydration and
  session construction.
- **Step 4 ruled copy-on-bind**: `bindPendingResources` binds a kind-preserving shallow copy and swaps it
  into the run clone (`ResourceCatalog.rebindEntry`); scoped verification resolves the canonical before
  probing (`VerifyExistence` probes the object it is handed — the slot's pristine capture would read the
  construction root); the planning session's objects stay pristine. The `lifecycleResource` pin's counter
  became a shared cell so probes through the copy stay observable.
- **The plan-time mirror landed as leaned**: `checkPromiseTypes` refuses a declared-string producer into
  a resource-typed slot; `checkVariableResourceSlots` is its variable twin (interim, item-exempt — docket
  item 9).
- **The refusal's first catches — writ adopt, both authoring surfaces.** The Go builder fed `file.move`
  run-computed machine-absolute strings through its gather records — migrated to plan-time claims
  (`file.DiscoverRegular`: identity minted with no disk contact, interned pending; the records carry the
  claimed resources, and the item projection delivers them to the seam). The two star fixtures bound the
  source via `plan.variable` — re-authored to authored claims, and the variable question produced the
  run-start-claiming ruling (docket item 9).
- **Executor hygiene as discovered**: the stale `resolvePendingResources` doc block removed (the function
  retired with scoped verification); the executor takes a value copy of the host's spec before stamping
  the run catalog, so run-only state never lands on the caller's object; `WithCatalog` renamed
  `WithResourceCatalog` (USER precision ruling; `WithWorkflowDispatcher` dissolved with the reverted
  environment bit).
- **Acceptance delivered — suite items 1–3 flip green** (statuses and evidence in the suite section; item
  2 refined into two walls during authoring: the integrity gate catches hand-alteration before the seam
  can, so the in-model miss is authored through the item-frame backstop). Four new judgment stars wired;
  Go pins in `pkg/op/method_test.go` and the pristine-location pin beside the root-binding pair.
- **First CI re-diagnosis (2026-08-22): two catches, neither assumed.** The quality gate flagged the new
  code itself — an unchecked type assertion in `shallowCopyResource` (now the house `assert.Type`) and
  `verifyScopeClaims` over the cognitive-complexity limit (the per-binding body extracted to
  `verifyClaimBinding`). The Windows test leg exposed the retired step-6 probe's THIRD client: the
  resume rearm retypes reloaded producer results through `Convert`, and without the probe the tag-URI
  string fell through to fresh construction — darwin silently swallowed the garbage stat, Windows'
  fsroot refused the colon-bearing rel ("path escapes from parent"). Fixed at the right layer: the
  rearm resolves recorded resource results by identity against the rehydrated catalog
  (`resolveRecordedResource` — rehydration's decode, §5.6), miss-tolerance intact, the dispatch seam
  the backstop.
- Gate: make check 103 ok / 0 fail; vet clean under darwin, windows, and linux; gofmt clean. PR 1's
  docket is complete.

**PR 2 record (2026-08-22, merged as #614 — develop `f03e9389`; #610 closed) — the production audit; two
residues of the superseded model fixed; steps 5–6 delivered.**

- **The audit's headline: `Shadow` still carried the superseded model's write-write conflict.** §4
  (revised 2026-08-20) rules same-URI production as run-time generations — "legal versioning when the
  plan ordered them, an authoring race when it did not" — but a different producer at an occupied URI
  ERRORED ("resource conflict: URI targeted by both"). The error is gone: different producers append
  generations, the namespace repoints, history survives; the dead error return retired with it
  (`Shadow(r, producerID) → id`; §2's surface listing updated). Shadow's doc header — which still
  described "the plan-time output registration operation" verbatim — rewritten to the ruled semantics.
- **The second catch: `GetOrCreate` returned the raw candidate.** It ignored Shadow's returned id,
  marked the CANDIDATE active (stamping Active under an empty id on the deference path), and handed
  producers an un-interned object whenever Shadow adopted an existing generation. Now it looks up and
  returns the canonical for whatever generation Shadow leaves current; and the producerless deference no
  longer adopts a Gone entry — revival always appends, per the matrix.
- **Step 5's parenthetical corrected** (noted at the step): production at an occupied location shadows
  unconditionally; the touch/Etag-refresh cascade is `Resolve`'s cache-hit behavior (§4.1), never
  production's.
- **The pins.** Catalog level: different-producers-append-generations (replacing the conflict pin — a
  pin of the superseded model), same-producer-appends, location-hit-shadows re-pinned with a REAL
  producer (the old pin passed "" and pinned the raw-candidate bug), producerless-adopts-canonical.
  Run-clone end to end (`pkg/op/provider/plan/production_matrix_test.go`): fresh-URI production (Active
  + producer stamp in the trace), claimed-URI production (the claim's activated generation plus the
  writer's shadow generation, both told by the trace), Gone revival (destroyer stamp on the Gone
  generation, the writer's Active revival). Immediate mode (step 6): the session product interns, and
  session-side Convert still constructs and claims — §5.6's second carve-out pinned. The coverage
  self-audit added three more: producerless-over-Gone appends (the deference's Gone guard), the pristine
  pin extended to products (a run's products never leak into the planning catalog), and
  `resolveRecordedResource` unit pins (hit returns the canonical; miss, non-string, and non-resource
  product types fall through unresolved — the rearm's tolerance, backstopped by the dispatch refusal).
- Doc hygiene: stale §6.2 references → §3; GetOrCreate's conflict sentence retired.
- Gate: make check 103 ok / 0 fail; vet clean under darwin, windows, and linux; gofmt clean.

**PR 3 record (2026-08-22, merged as #615 — develop `516db840`; #611 closed) — the explicit-conversion
docket delivered; suite items 4–13 green; phase 4's code is done.**

- **The two actions land as ruled**: `file.discover(path, kind?="entry", after?)` — lstat, no follow —
  and `file.resolve(path, kind?="entry", after?)` — stat, full chain, terminus identity, confinement
  judged against the RESOLVED root (macOS's symlinked temp roots would otherwise false-refuse).
  `EntryKind` is the named enum with explicit values and the full marshal/unmarshal surface; results
  intern as discoveries and transition Active through `VerifyExistence` (a discovery of a claimed path
  reaches the claimed entry — one identity, both doors). Stop-only throughout.
- **The ordering edge earned its type.** The scenario's sharp assertion caught `after any` silently not
  ordering: an invocation bound to an any-typed parameter captures the flow-combinator convention (the
  unit), not its promise. `op.OrderingEdge` carries the contract — the promise is the edge, the value
  discards by type — recorded as §5.7 rule 8. En route, the generator's defaults vocabulary was made
  uniform (`name=nil` always emits the bare-optional token for every type; the old behavior panicked the
  generator on struct-kinded nil defaults, hand-patched once to bootstrap regeneration).
- **The runtime grammar**: `NormalizeRuntimePath` = plan-space plus `fsroot.RelWithin`'s under-root
  rebase (the capability added to fsroot, where path/root questions live); the leading-slash sharpening
  is §5.7 rule 9. Per-OS Go table pins cover the machine-absolute directions; the star covers the
  promise-delivered escape.
- **Kind-honest activation (door one closed)**: per-kind `Exists` — lstat plus kind test on all three
  kinds (`SymbolicLink`'s lstat-only predicate was itself kind-blind; a regular at the path answered
  true). The writ audit's find: migrate's layer registration moved a DIRECTORY through a `*Regular`
  claim — the C2-recorded kind-looseness, refused at pre-flight the moment Exists became honest — fixed
  by `file.MoveDirectory` (`*Directory`-claimed sibling over the shared kind-agnostic move core) with
  writ migrate switched onto it.
- **The Entry-slot refusal** lands at `bindPresentValue` (shaped: "a claim asserts a kind and an
  interface asserts none"); `t.symlink` joins the harness as the OUT-OF-BAND actor (plain os.Symlink, no
  provider, no catalog) — exactly door four, which the kind-honest activation scenario needs at the
  starting line.
- **Suite items 4–13 all flipped green** (statuses and evidence in the suite section; item 2 of the
  authoring round: `shell.Result` does not convert to string, so the escape scenario's promise rides
  `file.read_text`). Go pins: the runtime-dialect table (per-OS), `RelWithin`, `EntryKind` round-trip +
  lstat-strict admits, and the kind-honest `Exists` matrix (the symlink-to-regular row is the door-one
  fix pinned).
- **CI re-diagnosis (2026-08-22): one catch, and a Windows note worth recording.** The quality gate
  flagged the new code — the Entry-slot refusal pushed `bindPresentValue` to cognitive complexity 26, so
  the authored (non-Invocation, non-Variable) arm extracted to `bindAuthoredValue`; behavior identical.
  The Windows legs went green FIRST TIME on a change set full of symlink scenarios (`t.symlink`, the
  lstat/stat pair, the dangling and escaping resolves, kind-honest activation) — the platform where
  symlink semantics diverge most; the identity work of phase 2 plus lstat-strict kinds is what makes
  that unremarkable.
- Gate: make check 103 ok / 0 fail; vet clean under darwin, windows, and linux; gofmt clean. Windows
  expectation: green stays green; any red is re-diagnosed, not assumed.

**PR slicing (task issues filed 2026-08-22, indexed by #586):** PR 1
([#609](https://github.com/NobleFactor/devlore-cli/issues/609)) — steps 1–4, the dispatch seam (opening
with the empirical audit of the seam location, this campaign's pattern). PR 2
([#610](https://github.com/NobleFactor/devlore-cli/issues/610)) — steps 5–7, the production audit and the
pins. PR 3 ([#611](https://github.com/NobleFactor/devlore-cli/issues/611)) — the explicit-conversion
docket: `file.discover`/`file.resolve` with the runtime grammar, the `Entry` plan-time refusal, and
kind-honest activation with the writ audit; phase closes. Acceptance rides the explicit-conversion
scenario suite (Judgment scenarios below): items 1–3 with PRs 1–2, items 4–13 with PR 3. Verification per
the plan's standing gate: `make check` (103/0), `make vet-all`, `gofmt -l`.

**Flagged, not decided:** the phase-3 note chartered the Docker and Go Toolchain e2e lore scenarios to run
alongside the campaign — they have not started, and phase 4 is the natural host if they are to move now;
the plan-time mirror of the step-2 refusal
(`checkPromiseTypes` / `typesAreInterconvertible` are documented as agreeing with dispatch,
pkg/op/validate.go:238, and step 2 changes dispatch's side) — leaning: graph-context narrowing so a
declared-string producer into a resource-typed slot refuses at `ValidateGraph`, step 2 the backstop for
undeclared producers, decided at the implementing PR. The key-versus-constructor sentence and the docket's
rules landed in 4-resource-management.md (§2, §5.6–5.7, §9 items 15–18) on 2026-08-22.

### Phase 5 — closure — status: complete 2026-08-22 (#587)

1. Sketches removed per phase 0's disposition — the deletions themselves landed with phase 0 (#592), so
   what remains is the `docs/architecture` statuses updated and the transport plan's supersession note
   finalized. **Done:** `4-resource-management.md`'s header note now reads *implementation complete*
   (the §5.6/§5.7 rules named); `3.5.4-file-provider.md` gains `file.discover` / `file.resolve` in the
   observer table, `file.move_directory` beside `file.move`, and the kind-honest-claims paragraph;
   `3.5.4-file-provider.status.md` records the new surface and its coverage; the transport plan's note
   is finalized — what replaced "recreate from slot URIs" is named (a slot URI is a key, never a
   constructor), so the Goal's "run it on another host" now holds for every addressing, not only the
   content-addressed ones.
2. ~~The two demonstration pins graduate from local red to committed green~~ — done: scenario 1's pin
   landed green with phase 1, and the fail-fast pin flipped green in PR B #603. A third joined them:
   `test_judgment_discover_after_exec.star`, authored red in #612 and flipped green in #615.
3. `4-resource-management.status.md` records the completed convergence. **Done:** the state paragraph
   declares it — *the design and the tree agree, and the campaign's divergence table has no surviving
   row* — with per-PR completion rows for #609/#610/#611 and an Outstanding list carrying only what
   genuinely outlives the campaign (the staged per-type rollout, the remote-execution abstraction,
   run-start claiming, and judgment scenario 2's wait for a drivable reconcile surface).

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

**Evidence (2026-08-20):** `test_judgment_1_delete_then_copy.star`, wired into the CI suite
(`TestJudgmentScenario1_DeleteThenCopy`) — five expectations, passing on the first run against phase 1. One
precision recorded during authoring: `file.remove` takes `path string` by step-23 ruling 2 (only content
reads take the resource), so the single catalog entry comes from the copy's resource-typed source — the
count prediction held; the phrasing "delete a named regular file resource" surfaced Remove's typing, and the
ruling followed (2026-08-20): **mutation targets are resource-typed consumers** — one pending entry, two
consumers; the remove transitions the entry to Gone at runtime and its receipt relocates the bytes for undo.
Step-23 ruling 2's path-typed mutation targets are overruled; the `file.remove` signature migration is
phase-3 claiming-grammar work (#585), after which this scenario's single entry carries both consumer
links. Refined the same day: **the second consumer sees Gone** — the copy fails on the catalog's verdict, not
by rediscovering the loss through its own I/O. Evidence of today's gap: the manual run's receipt shows the
entry still `pending` after the failed run — nothing transitions it yet; #585 closes both halves (the
behavior-matrix consumption table in 4-resource-management.md §3 records the ruled semantics). **Both
halves delivered 2026-08-22 (PRs C #604, D #605):** the typed target transitions the entry with the
destroyer stamp, and the scenario asserts the narrated verdict
(`expect_error("file.copy.*destroyed by")`) end to end. Placement
ruled the same day: the guard lives in the action dispatch seam — after `Method.Invoke`'s slot-to-argument
conversion, before the forward method call — the earliest point the complete consumed set exists (promises
resolve only at slot-fill), shared by graph and immediate dispatch; the catalog-resolve verdict is the
in-flight backstop, and the Gone transition stamps the destroying unit.

### Scenario 2 — relocate the tree, reconcile the graph

**Setup.** Create a graph; run it at fsroot location A. Copy the **entire tree** to a new fsroot location B
(bytes intact; inodes and mtimes new, as copies are). Reconcile the graph at B.

**Expected result — reconciliation succeeds and reports no content drift.** The graph's intent is rels, so
binding to B is fully defined: nothing in identity remembers A. Every rel exists under B. The **Etag screen
mismatches on every entry** — Etags are stat tuples, and relocation mints new inodes — which is by design:
the cheap screen escalates to the honest check. **Digests match**, so every entry classifies as **touch
drift**: Etag refreshed against B, no shadow, no repair, clean report. The graph is at home in its new root.

**Evidence:** deferred — the scenario needs a drivable reconcile surface (phase 4/closure era); it stays a
recorded prediction until then.

**The contrast that gives the scenario teeth:** under absolute identity, reconciliation at B finds zero
corresponding resources — every entry "missing," the graph unreconcilable without rewriting it. Scenario 2
is the relocation proof — the direct payoff of rel identity — and it exercises the reconciler's two-tier
cascade (Etag screen → Digest verdict) exactly as designed.

### Scenario — promise ordering

**Setup.** One unit produces a file; a second consumes it through the producer's promise. The list hands the
**consumer first** to `assemble_definition` — if ordering were positional, the copy would dispatch before its
source exists.

**Predictions.** Ordering comes from promises, so the write runs before the copy regardless of list
position; the promise-fed source mints **no** pending entry (a promise is recorded, not claimed); the
products are runtime facts — the stored catalog section is present and **empty**; the produced content flows
through the promise into the copy.

**Evidence (2026-08-20):** `test_judgment_promise_ordering.star`, wired into the CI suite
(`TestJudgmentPromiseOrdering`) — four expectations, passing on the first run. The consumer-first list is the
sharp assertion: execution ordered by the promise edge, not by position.

### Scenario — pre-flight fail-fast

**Setup.** A graph claims an existing file as pending intent; the file is deleted **after planning, before
the run**.

**Predictions (from the Q1 ruling).** Pre-flight's verification pass finds the pending file resource missing
relative to the run's fsroot — unmet intent — and **fails the run before any unit dispatches**; the error is
the catalog's verdict (`verify existence: resource … does not exist`), not a rediscovered copy error; the
destination is never created.

**Evidence (2026-08-20): red, exactly as predicted — this is phase 3's acceptance pin.**
`test_judgment_preflight_fail_fast.star`: two of three expectations pass (one pending entry claimed; the
destination never created); the error-shape expectation fails because today's run produced
`file.copy: openat vanishes.txt: no such file or directory` — the copy **dispatched** and rediscovered the
loss through its own I/O. `resolvePendingResources` says so itself: "Mark, don't fail." Wired as
`TestJudgmentPreflightFailFast` with a `t.Skip` naming the gap; #585 un-skips it, and the scenario flipping
green is the phase's acceptance evidence. **Flipped green 2026-08-22 (PR B #603):** un-skipped — the run
fails on the catalog's verdict before any dispatch, the destination never created.

### Scenario — discover after exec

**Setup.** An opaque shell command writes a file at a known relative path — a side effect no promise can
deliver. `file.discover` receives that path as a string **literal**; a copy consumes the discovery's
promise. The list hands the units consumer-first.

**Predictions (from the explicit-conversion rulings, 2026-08-22).** The stored catalog carries **no**
claim for the discovered path — a mid-run fact is not intent, and the string parameter mints nothing.
Discover interns the file at its own dispatch as a discovery (lstat, no follow, Stop-only), and the
consumer receives the discovered entry through the promise, reading the content the command wrote. The
sharp assertion is the **ordering edge**: list position does not order execution (the promise-ordering
scenario's proof), so the exec→discover sequencing must be an explicit edge — the literal-path flow is
exactly the case a data edge cannot order.

**Evidence (2026-08-22): authored red.** `test_judgment_discover_after_exec.star`, wired as
`TestJudgmentDiscoverAfterExec` with a `t.Skip` naming both gaps: `file.discover` unimplemented, and the
pure-ordering-edge mechanism undecided (no flow construct provides one today — Choose/Gather/Subgraph/
WaitUntil is the full set). The implementing PR decides the mechanism, completes the ordering line, and
un-skips; the scenario flipping green is part of PR 3's acceptance.

### The explicit-conversion scenario suite (chartered 2026-08-22 — all thirteen)

Chartered as predictions (USER, 2026-08-22): each item's status adjusts — **predicted** (not yet authored)
→ **authored red** (star script + skipped wiring) → **green** — with commentary updated as the work
progresses. Items 1–3 ride phase 4's PRs 1–2; items 4–13 are PR 3's acceptance evidence.

**A. The rule itself**

1. **Save, reload, run — identity all the way through.** Plan with a claimed source, save, reload, run:
   the dispatched source IS the section-rehydrated catalog entry (object identity), re-based to the run
   root — never a reconstructed twin. Status: **green** (2026-08-22, PR 1) —
   `test_judgment_reload_dispatch.star` (the reloaded slot string resolves and the copy flows) plus the
   Go pins: `resolveDispatchResource`'s canonical-pointer assertions (`pkg/op/method_test.go`) and
   `TestRun_PlanningCatalogStaysPristineInLocation` (the planning object still binds the plan root after
   a run under another root — copy-on-bind severed the aliasing).
2. **The miss refuses — two walls (refined during authoring).** Wall 1: a hand-doctored document never
   reaches the seam — slots live inside the canonical bytes, so the integrity gate refuses at load
   (`test_judgment_doctored_checksum.star`, "checksum mismatch"). Wall 2: an in-model miss — a gather
   item record carrying a raw string into a resource-typed slot (the item-frame backstop) — fails at
   dispatch with the §5.6 verdict, destination never created (`test_judgment_dispatch_miss.star`,
   "not in the run catalog"; the seam's own mechanics pinned in `pkg/op/method_test.go`). Status:
   **green** (2026-08-22, PR 1), both walls.
3. **The string-promise refusal.** A declared-string producer's promise feeds a `*Regular` slot: refused
   at `ValidateGraph` — the plan-time narrowing landed as leaned
   (`test_judgment_string_promise_refusal.star`, "returns a string, but the slot is resource-typed");
   undeclared producers meet the dispatch refusal (wall 2). Status: **green** (2026-08-22, PR 1).

**B. Discover and resolve**

4. **Discover after exec.** The literal path, the empty catalog section, the ordering edge. Status:
   **green** (2026-08-22, PR 3) — the `after=ran` edge completed and the skip lifted; the sharp
   assertion earned its keep twice: it exposed the `any`-type ordering collision (→ `op.OrderingEdge`)
   and a fixture gap (a raw shell redirect creates no parents).
5. **Kind assertion sharpens the verdict.** `discover(path, kind="regular")` over a directory fails AT
   the discover node with the kind-mismatch verdict; the consumer never dispatches. Status: **green**
   (2026-08-22, PR 3) — `test_judgment_discover_kind_verdict.star`.
6. **The entry default is permissive.** `discover(path)` of a directory feeding a `*Regular` slot:
   discover succeeds and the failure is the consumer's conversion — the knowingly-carried cost pinned as
   designed behavior, not a defect. Status: **green** (2026-08-22, PR 3) —
   `test_judgment_entry_default_consumer_mismatch.star` ("cannot fill").
7. **The lstat/stat pair.** One rel, a symlink to a regular file: `discover` interns the LINK (kind
   symbolic-link), `resolve` interns the REGULAR the chain designates, and a copy fed by the resolution
   reads the target's content. Status: **green** (2026-08-22, PR 3) —
   `test_judgment_lstat_stat_pair.star`.
8. **Resolve refuses the broken and the escaping.** A dangling chain stops at the resolve node; a link
   targeting outside the run root refuses with the CONFINEMENT verdict, not a raw I/O error. Status:
   **green** (2026-08-22, PR 3) — `test_judgment_resolve_dangling.star` (the target destroyed between
   link and resolve) and `test_judgment_resolve_escape.star` (a link to `..` — the root's parent, always
   present and always outside).
9. **The rebase in both directions.** Refined at authoring: `$PWD` is not portable through the star
   shell on Windows (MSYS spellings), so the machine-absolute directions — under-root rebase,
   outside-root refusal, volume and UNC spellings, the per-platform leading-slash reading — are pinned
   in Go (`TestNormalizeRuntimePath_TheRuntimeDialect`, running on all three CI platforms), and the star
   pins the promise-delivered escape (`test_judgment_runtime_escape_refusal.star`, the path riding a
   `read_text` promise — a second authoring finding: `shell.Result` does not convert to string).
   Status: **green** (2026-08-22, PR 3), split as described.

**C. Mediation — discoveries join the model**

10. **Claimed and discovered — one identity.** A path claimed at plan time by one unit and discovered
    mid-run by another dedups to ONE catalog entry with both consumers linked — the catalog mediating
    across the two doors. Status: **green** (2026-08-22, PR 3) —
    `test_judgment_claimed_and_discovered.star` (the stored intent carries exactly the one claim; the
    discovery reaches the claimed entry and the consumer reads through it).
11. **Discovered, then destroyed.** Discover interns; a typed remove destroys the file; the discovery's
    consumer fails on the narrated guard verdict ("destroyed by unit N"), never on its own I/O —
    discoveries get the same protection claims do. Status: **green** (2026-08-22, PR 3) —
    `test_judgment_discovered_then_destroyed.star`.
12. **Claims are true when made — kind-honest activation.** A `*Regular` claim over a symlink (second
    direction: over a directory) fails pre-flight with the kind verdict at activation — not a swallowed
    Etag error, not a mid-run surprise. Status: **green** (2026-08-22, PR 3) —
    `test_judgment_kind_honest_activation.star`, with the link created OUT-OF-BAND by the harness's new
    `t.symlink` (plain os call, no provider, no catalog — door four at the starting line; an in-model
    link would be seen coming). The directory direction and the symlink-to-regular row ride the Go
    matrix (`TestExists_IsKindHonest`).

**D. Plan-time refusals**

13. **The `Entry` slot refuses authored strings.** An authored literal into an `Entry`-typed parameter
    draws the shaped plan-time refusal ("state the kind or feed a discovery"), never an incidental
    construction error. Status: **green** (2026-08-22, PR 3) —
    `test_judgment_entry_slot_refusal.star` (`plan.file.observe(resource="some/path")`).

Existing green coverage — scenario 1, pre-flight fail-fast, promise ordering, scoped claims, gone
tolerance — pins the phase's remaining edges; scenario 2 (relocate + reconcile) stays deferred to the
closure era.

## Open questions — all ruled 2026-08-20

1. ~~Which string parameters are output-naming?~~ — **RULED: none.** Products are runtime-created; nothing
   about a method's return enters the plan-time catalog. No convention, no annotation. Appendix A is
   rejected, not parked. (Supersedes docs/sketches/resource-construction.md:21–27.)
2. ~~Does the stored ledger carry `state`?~~ — **Dissolved by ruling 1.** With no plan-time existence checks
   and no plan-time products, every stored entry is Pending by construction. Graph = intent; trace =
   observation (step 48).
3. ~~Schema version bump~~ — **RULED: stays 1.** The mandatory section is the gate by itself: every existing
   graph document fails to load and is rewritten by re-planning. No version dance, no shim.
