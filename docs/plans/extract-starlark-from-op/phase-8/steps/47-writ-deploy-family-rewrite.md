---
step: 47
title: "writ deploy family — rewrite onto the sealed graph + the trace store (the StateView crater)"
status: slices 1+2+3 LANDED 2026-07-15 — THE REPOSITORY BUILDS AND TESTS GREEN (zero failures; the writ/docgen reds are gone two slices early); slice 4 (stubs deletion, manifest glue, sops test, --conflict wiring, step 18/22 closure) next
parent: ../../phase-8.md
---

# Step 47 — writ deploy family rewrite

**Chartered 2026-07-15** from the settled design in [writ-deploy-family.md](../writ-deploy-family.md) (nine settled
items, no open questions; the design round ran 2026-07-15). This is the deploy-family crater formerly tracked as
"step 33 slice C" — chartered as its own step per step 33's ruling that the crater needs its own spec.

## Why

`cmd/writ/writ/commands.go` (1,497 lines) + `graph_builder.go` (479 lines) are written against the ANCIENT
framework end to end — the mutable graph API, `*op.ReceiverRegistry` as a type, the deleted
`execution.StateView` subsystem, dead `cli.LoadLatestReceipt`-shaped calls, `op.ImmediateOf`, the old `op.Origin`
struct, the old `lore.Planner` seam. They are the last build reds in the repo: `cmd/writ`, `cmd/writ/writ`, and
`cmd/docgen` (red solely via its import). The full verified inventory lives in the design doc's Current state.

## Slices (fed one at a time)

1. **Slice 1 — store index + readback + deploy.**
   - The `internal/cli` run index: append-only NDJSON at `DevloreStateHome()/index.ndjson`, appended by
     `WriteGraph` (`{at, event, tool, scope, graph_checksum}`) and `WriteTrace`
     (`{at, event, graph_checksum, trace_file}`); torn last line reads as absent.
   - The writ-owned readback package (a `cmd/writ/writ/` sibling of `adopt`/`migrate`): the time-ordered,
     best-effort fold per scope over writ-tool documents — successful link/copy/render marks a target deployed,
     successful unlink/remove marks it gone, failed nodes don't count; index⇄document reconciliation in both
     directions; zero surviving knowledge still refuses destructive consumers.
   - Deploy rewritten onto the lore.Build pattern: tree walk + snapshot pinning kept verbatim; per-file action
     chains and manifest-resolved lore units planned via `plan.Provider`; `op.NewOriginBase("writ", scope, bag)`
     (bag: source root, target root, projects, segments, layers, commit hashes, dirty layers); per-scope assembly
     and confined runs (System then Home, fail-forward); `cli.WriteGraph` + per-run `cli.WriteTrace` win-or-lose;
     summary via `Trace.Summarize()` (port `formatGraphSummary`). Dry-run serializes the graphs to stdout.
   - **Scenario 2's critical path** ([demo-milestone.md](../../demo-milestone.md)).
2. **Slice 2 — decommission + upgrade** (pure readback consumers).
   - Decommission: readback → per-scope `file.unlink`/`file.remove` graphs; NO signature gate (step 46 wires it
     when the signer exists); the "no deployment history" refusal stays.
   - Upgrade: readback filtered to copied files → re-planned render/decrypt chains; the conservative interim —
     any differing target skips with a warning, `--force` overwrites (full source-changed vs. target-modified
     attribution flips on when step 48 lands).
3. **Slice 3 — `writ status`** (replaces `writ reconcile`; the internal `reconcile` package renames to match).
   - Four sections: layers (the registered tree under `WritLayersDir()`); per-scope deployed inventory (each
     entry classified, with source layer/project and the repairing command per finding — missing/stale →
     `writ deploy`, orphan → `writ decommission`); packages-via-writ (fact-of-record from `pkg.*` nodes); store
     health (index-vs-document detection findings).
   - A missing index is a hard error. No `--fix` — the report names the repairing lifecycle command instead.
   - Drift checks run the Etag-then-Digest cascade; before step 48 lands, differing copied entries report as
     modified-or-stale (indeterminate).
4. **Slice 4 — close.**
   - Delete `internal/execution` (whole package) and the three stubs (`inspect`, `list`, `receipt show|list`) —
     registrations and builder functions.
   - `cmd/docgen` greens; **steps 18 and 22 close** (the writ build green at last); docs + master rows.

## Slice 1 — LANDED 2026-07-15

What landed: `internal/cli` run index (`index.go` + `WriteGraph`/`WriteTrace` appends + tests); the
`cmd/writ/writ/deploy` package (Execute / BuildGraphs / the four pipeline chains / pinning / reporting +
integration tests over real files); the `cmd/writ/writ/readback` package (the time-ordered best-effort fold +
integration tests incl. nuked-trace finding and missing-index error); `commands.go`'s `runDeployV2` rewired thin
onto `deploy.Execute` (the deploy section of the crater is clean; the StateView set remains for slices 2–4).

**Framework fix that rode along**: `Subgraph.linkChildren` resolved load-path placeholder children in ID-sorted
order and re-broke topological ties differently than the write side, so any intact document with independent
branches failed `op.LoadGraph`'s checksum integrity check — every deploy graph reproduced it. The document's
child order (the write side's topological order) is now authoritative (`loadChildOrder`), with the stable sort
still correcting hand-authored documents; pinned by `TestGraph_SaveLoad_RoundTrip_TieOrderPreserved`.

Discovered, recorded for later slices / rulings:

1. **Conflict semantics — RESOLVED 2026-07-15, chartered as step 49.** The sealed `file.link` archives an
   occupied target to the recovery site and replaces it, and `op.ConflictPolicy` is read by nothing. Ruling:
   the enum collapses to `stop` | `skip` | `replace` (replace always archives — compensation requires it),
   enforced by the FILE PROVIDER at the write seam reading the announced "runtime" config section live, floor
   `stop`; writ's `--conflict` feeds the cli config layer, wired in slice 4 so the flag activates when step 49
   lands. Until then deploy's real semantics are replace-always. Charter:
   [49-conflict-policy-enforcement.md](49-conflict-policy-enforcement.md).
2. **Deferred defaults on the plan path — RESOLVED 2026-07-15 (fix applied).** The planner stuffs the
   parsed-but-unresolved `DeferredDefault` into the omitted slot (planner.go:273) and only the starlark bridge's
   DIRECT-invocation path resolved it — an unfinished half of `DeferredDefault`'s own documented contract,
   latent for every plan in either language (all existing fixtures pass `chmod` explicitly; the affected surface
   is the four `{{ umask … }}` defaults on file Copy/Mkdir/WriteBytes/WriteText). Fixed at the dispatch seam:
   `Method.Invoke` resolves `DeferredDefault` against the live environment and filled sibling slots before
   `Convert` — where both finally exist. Pinned by `TestPlannedDeferredDefault_ResolvesAtDispatch` (plans
   `file.write_text` omitting chmod, runs, asserts the umask-derived mode). Deploy keeps its explicit slots —
   its modes are semantic, not defaults.
3. **Template `Env` — RESOLVED 2026-07-15 (fix applied; the trade-off is a feature).** The old in-process
   `data["Env"]` function can't ride the serialized data map; the render-time home is the template provider's
   `FuncMap` (`renderFuncs`), so `{{ Env "KEY" }}` works again and resolves at dispatch time on the rendering
   machine. Ruled: graphs are transportable and SHOULD render differently under different environments —
   plan-time resolution would instead embed environmental values (potentially secrets) into the persisted graph
   documents. Pinned by `TestRenderText_EnvFunc`.
4. **Manifest planner glue — approach RULED 2026-07-15, lands in slice 4.** Detection already lives in
   `pkg/platform` (`Detect()`/`New()`); lore's `detectPlatform` only RENDERS the canonical dotted token
   ("Darwin", "Linux.Debian" — with distro-family grouping), and that token vocabulary is devlore-wide (writ's
   segments and variant directories speak it too). Ruling: `pkg/platform` gains the canonical token rendering
   (`Token()` — `"<OS>[.<DistroFamily>]"`); `lore.detectPlatform` collapses onto it; writ's command glue calls
   the same to construct the `lore.Planner`. One vocabulary, one home. Until wired, manifests are reported and
   skipped.
5. **Sops integration coverage — approach RULED 2026-07-15, lands in slice 4.** No new machinery: lift the
   encryption package's fixture pattern (`sopsEncrypt(t, plainYAML)` generates an in-process age identity and
   sops-encrypts; `t.Setenv("SOPS_AGE_KEY", …)` supplies the ambient identity —
   pkg/op/provider/encryption/provider_test.go) into a deploy integration test: a `secret.sops` and a
   `config.template.sops` through `deploy.Execute`, asserting decrypted content, 0600 mode, and the
   decrypt+render chain end to end.

## Slice 2 — LANDED 2026-07-15

What landed: the `cmd/writ/writ/decommission` package (readback-fed removal — `file.unlink` per linked entry,
`file.remove` per copied entry, per-scope graphs confined at the recorded target root, `--prune` bounded there,
zero-knowledge refusal, removal inventory annotated so the next fold clears the targets) and the
`cmd/writ/writ/upgrade` package (readback-fed regeneration with the settled conservative interim: missing →
regenerate freely; up-to-date → no-op; differing/unverifiable → skip + warn, `--force` overwrites; comparison by
fresh in-process render through the same `template.Provider` the graph uses; sops entries unverifiable by
design). Deploy exported the family seams (`PlanFileChain`, `RenderData`, `CommonAncestor`); readback entries
gained `TargetRoot` from the origin annotations. `commands.go` rewired both commands thin; the old machinery
(state-view loaders, the hand-built upgrade chains, `builtinTemplateData`/`formatGraphSummary`,
`sortGraphsByScope`) deleted; decommission's `--force` flag removed (its only meaning was signature gating —
returns with step 46); `graph_builder.go` fully orphaned and deleted. Tests: nine integration tests across the
two packages, including the unlink safety refusal (a link the user replaced with a real file is never deleted)
and fold-after-decommission emptiness.

**Framework fix that rode along**: `resultOrNil` (pkg/op/action_types.go) now normalizes typed-nil results to
untyped nil, mirroring `compensatorOrNil`'s existing load-bearing guard — removal actions return a nil
`*Resource` product, and the typed nil stored on the receipt panicked trace serialization (the yaml encoder
invoked the promoted `MarshalYAML` through the nil pointer). Decommission was the first-ever consumer to
serialize a nil-product action's trace.

## Slice 3 — LANDED 2026-07-15 — and the build went green

What landed: the `cmd/writ/writ/status` package — `writ status` replaces `writ reconcile` per the settled
rename. Four sections per settled item 5: the registered layer tree under `WritLayersDir()` (absent / directory
/ link with resolved target / broken-link); the deployed inventory classified against the live filesystem
(Linked / Copied / Missing→`writ deploy` / Conflict / Orphan→`writ decommission` /
Modified-or-stale→`writ upgrade`, the interim indeterminate class until step 48; encrypted content not
compared); packages-via-writ (fact-of-record `pkg.*` receipts, collected by a readback extension —
`Inventory.Packages`); and store health (folded runs + missing-piece findings). A missing run index is the
ruled hard error. Report-only — no `--fix`; text and `--json` presentations. The receipt-signature check
arrives with step 46. Five integration tests cover the clean shape, every finding class with its repair
pointer, the link-mode layer report, and the missing-index error.

**The crater is gone.** Replacing the reconcile section removed the last dead-API references in
`cmd/writ/writ`; the package — and with it `cmd/writ` and `cmd/docgen` — builds and tests green. `make test`
reports ZERO failures repository-wide for the first time in weeks, two slices ahead of the plan (the step-18/22
gate condition is now met; their formal closure rides slice 4 as chartered). The orphaned `reconcile` package
and `internal/execution` (the 5-line `GraphBuilder` remnant) are deleted with this slice; slice 4 keeps the
stubs deletion, the manifest glue, the sops integration test, and the `--conflict` wiring.

## Interlocks

1. **Step 46** (graph signing + `writ verify`) follows this step; it wires decommission's signed/unsigned gate
   and status's receipt-signature check. Its charter's "reconcile" consumer reference follows the status rename.
2. **Step 48** (ledger content identity) is independent — lands before or alongside; slices 2/3 consume the
   recorded Etag/Digest pair when present.
3. **Demo Scenario 2**: `writ deploy noblefactor thenobles` against the user's restructured `../Personal`.

## Test plan

1. Deploy end-to-end over a layered `testdata` tree: links + template + (sops-gated) secret; per-scope graphs;
   graph + trace persisted; index appended; summary correct; `--conflict` modes; collision reporting; dry-run
   serialization; dirty-layer refusal + `--allow-dirty`.
2. Readback round-trip: deploy → inventory matches the planned nodes; decommission removes exactly the inventory;
   a decommission trace folds entries back out; upgrade regenerates only cleanly-matching copied entries.
3. Status classifications over crafted target states (linked / copied / missing / conflict / orphan +
   modified-or-stale for differing copies); index-missing → error; nuked-document → missing-piece finding.
4. Stub deletion: the commands are gone from `--help`; `internal/execution` no longer referenced anywhere.
