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

### Phase 2 — portable file identity: rel, bound to the run's root (#546/#547) — status: complete 2026-08-21 (PR 1 `92c18eb1`; PR 2 `2c5f6e6a`; PR 3 in review — the completing PR)

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

### Phase 3 — plan-time claiming: inputs only, pending only — status: pending

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
4. Consequence for phase 1's stored section: every stored entry is Pending by construction. **RULED
   2026-08-21: the stored row drops `state` entirely** — the intent row becomes its own `{id, uri}` type
   (presence IS the pending claim), splitting from the trace's `LedgerEntrySnapshot`, whose
   state/etag/digest/producer vocabulary stays where observation lives. **Graph = intent; trace =
   observation** (the step-48 snapshot keeps the observed side). Fallout, small and known: the
   round-trip pins re-pin (bytes change; old documents die, schema stays 1); the star pins asserting
   `state == "pending"` flip to asserting the field's absence; the Go catalog pin keeps presence/absence
   and drops its state assertion; writ readback is untouched (it reads the trace). Known boundary: lenient
   decoding means a stray hand-edited `state:` is ignored, not refused — a codec-wide property, noted.
5. Acceptance: **both pins flip green with corrected assertions** — the stored document carries
   `original.txt` (pending), and asserts `duplicate.txt` **absent**, pinning the product ruling in both
   directions. **Delivered early — the pins flipped with phase 1**: planning already interned the
   resource-typed source, so serialization was the missing half and the destination's absence was already
   true. Phase 3's remaining substance shrank on audit (2026-08-20): `Discover` already performs no
   existence I/O — its body says so and the executor's pre-flight owns transitions — so phase 3 is the
   promise grammar verified plus pins for the no-I/O rule and the pre-flight transition-failure semantics
   (a pending resource that does not exist under the run's root fails the run — Q1 ruling).

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
behavior-matrix consumption table in 4-resource-management.md §3 records the ruled semantics). Placement
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
green is the phase's acceptance evidence.

## Open questions — all ruled 2026-08-20

1. ~~Which string parameters are output-naming?~~ — **RULED: none.** Products are runtime-created; nothing
   about a method's return enters the plan-time catalog. No convention, no annotation. Appendix A is
   rejected, not parked. (Supersedes docs/sketches/resource-construction.md:21–27.)
2. ~~Does the stored ledger carry `state`?~~ — **Dissolved by ruling 1.** With no plan-time existence checks
   and no plan-time products, every stored entry is Pending by construction. Graph = intent; trace =
   observation (step 48).
3. ~~Schema version bump~~ — **RULED: stays 1.** The mandatory section is the gate by itself: every existing
   graph document fails to load and is rewritten by re-planning. No version dance, no shim.
