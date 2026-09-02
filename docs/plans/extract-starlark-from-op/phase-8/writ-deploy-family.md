---
title: "writ deploy family — command plan & design (the StateView crater rewrite)"
status: IMPLEMENTED (2026-07-16) — step 47 complete, all four slices landed, the repository green; ten settled items; companions: step 48 (drift attribution), step 49 (--conflict enforcement), step 46 (signature gates)
created: 2026-07-15
parent: ../phase-8.md
---

# writ deploy family — plan & design

> **Renamed back, 2026-09-01.** This document records `writ status` replacing `writ reconcile` at
> phase-8 step 47. [#762](https://github.com/NobleFactor/devlore-cli/issues/762) returned the name once
> repair was chartered, and [#774](https://github.com/NobleFactor/devlore-cli/issues/774) landed it: the
> command is `writ reconcile` again. The text below is kept as the record of the original decision.

## Purpose

The deploy family is writ's core: put a layered configuration tree into effect on a machine, and manage what was
put into effect afterward. Four real commands plus three stubs, all in `cmd/writ/writ/commands.go` +
`graph_builder.go`:

1. **`writ deploy [--conflict=stop|backup|overwrite|skip] [-s KEY=value] [--allow-dirty] <project>...`** — walk
   the layered source tree (base → team → personal), select platform/segment variants, resolve cross-layer
   collisions, and emit per-scope graphs (System first, unconfined-ish root `/`; then Home, confined to `$HOME`):
   `file.link` for plain files, `template.render_*` → write for templates, `encryption.decrypt` for secrets,
   `file.copy` for forced copies, and package nodes resolved from `packages-manifest.yaml` through the lore
   Planner. Layer sources are pinned to git-worktree snapshots before planning (`snapshot.PinAll`) with a dirty
   check (`--allow-dirty` to override); the pin's commit hashes ride the graph metadata. Fail-forward across
   scopes; dry-run serializes the graphs to stdout.
2. **`writ decommission [--prune] [--force] <project>...`** — remove what deploy created: discover the deployed
   state per scope, emit `file.unlink` (symlinks) / `file.remove` (copied files) graphs, System/Home ordered,
   fail-forward. Help text promises safety gating on state signature (signed → full drift detection; unsigned →
   `--force` required; no state → refuse).
3. **`writ upgrade [--force] [<project>...]`** — regenerate copied files (templates, secrets) from current
   sources; symlinks untouched. Help text promises the drift matrix: source-changed-only → regenerate;
   target-modified-only or both-changed → skip + warn, `--force` overwrites.
4. **`writ reconcile [--drift] [--fix] [--json] [<project>...]`** — drift report (and promised repair): with
   projects, build a fresh tree and compare; without, read the deployed state (falling back to a target-directory
   scan) and classify each entry (Linked / Copied / Conflict / Missing / Orphan / Stale / Modified /
   drift-Conflict). `--fix` is declared but the current implementation only reports.
5. **Stubs**: `inspect`, `list`, `receipt show|list` — all return "not yet implemented".

## Current state (verified 2026-07-15)

The two files (1,497 + 479 lines) are written against the ANCIENT framework end to end — the `execution.StateView`
build errors are just the visible edge. The full dead-API inventory:

1. **The mutable graph API**: zero-arg `op.NewGraph()`, `g.AddNode(node)`, `g.Root.SetEdges/AddEdge`, writes to
   `g.Origin` / `g.Collisions` fields, hand-built `op.NewNode(id, action)` + `node.SetSlot` + `node.Origin =` /
   `node.Layer =`, `op.Edge{From, To}`. The sealed graph is immutable — built from units via
   `op.NewGraph(op.NewGraphSpec()...)` or `plan.Provider.AssembleDefinition`, no setters, no exported node
   construction.
2. **`*op.ReceiverRegistry` as a type** (7 signatures across the two files). It is now a function returning the
   unexported `*receiverRegistry`. (`BuildAction` still exists on it, but the sealed planning path goes through
   `plan.Provider` — hand-built actions bypass planners, policies, and slot validation.)
3. **The deleted StateView subsystem**: `execution.StateView` / `FileEntry` / `NewStateViewBuilder` /
   `ViewOptions` — the receipt-derived "what is deployed" view (entries by rel-target, `IsCopied`, `Project`,
   `Layer`, `ReceiptCount`, `DistinctScopes`). `internal/execution` now holds ONLY the 5-line `GraphBuilder`
   interface, whose only consumers are these two files — the package dies with this rewrite.
4. **Dead `internal/cli` calls**: `cli.LoadLatestReceipt("writ", "")` and `cli.LatestReceiptPath("writ", "")` do
   not exist. The real store (internal/cli/receipts.go): `WriteGraph` (graphs keyed by checksum under
   `GraphsDir()`, idempotent), `WriteTrace` (timestamped traces under `ReceiptsDir()/<graph-checksum>/` +
   `latest.yaml` symlink), `LoadLatestTrace(graphChecksum)`, `LoadTrace(path)`. One graph → many traces;
   trace→graph lookup by checksum. There is NO enumeration/load-by-tool-or-scope API yet (open question 1).
5. **`op.ImmediateOf`** (dead; the reporting read is `op.ImmediateBinding.Resolve(nil, nil)` per the migrate
   helper) and **`op.GraphExecutionSummary` / `g.Summary()`** (dead; the sealed tally is `Trace.Summarize()` →
   `op.Summary` with `ByAction()` / `Skipped()` / `Failed()` — `formatGraphSummary` ports nearly as-is).
6. **`op.Origin` as a struct with writ fields** (`Tool`, `Scope`, `SourceRoot`, `TargetRoot`, `Projects`,
   `Segments`, `Layers`, `CommitHashes`, `DirtyLayers`). Sealed `Origin` is an interface — `Tool()` / `Scope()` /
   `Annotations()` — with `op.NewOriginBase(tool, scope, annotations)` the one concrete carrier; tool-specific
   metadata lives in the annotation bag with typed read-side projections (lore stamps
   `{packages, platform, features, settings}` exactly this way).
7. **The lore integration**: old `lore.Planner{ActionRegistry: reg}` + `planner.PlanPackages(g, m)`. Current:
   `Planner{Platform, RegistryClient, Features, Settings, DryRun}` +
   `PlanPackages(provider *plan.Provider, sharedEnv, manifestPath) ([]string, []op.ExecutableUnit, error)` —
   units for assembly, not nodes into a mutable graph.
8. **`WithSops` is gone** from `op.RuntimeEnvironmentSpec` (builders: Application/Catalog/Modules/Platform/
   Result/Root/Status). The secrets pipeline's sops wiring is an open verification item (question TBD).
9. **Latent breakage riding along**: `commands.go:242` assigns to an undeclared `path` (a slice-1 edit of mine,
   masked by "too many errors"); `newInspectCmd`'s example text references `.ReceiverName`; reconcile's `--fix`
   is declared but unimplemented; `upgradeFile` carries two step-15 TODOs tied to the dropped `Node.Status`.
10. **`cmd/docgen` is red solely through its import of `cmd/writ/writ`** — the family rewrite greens it for free.

**Healthy and kept** (green, framework-agnostic): `tree` (layered walk, segment/platform variant selection,
processing pipelines, collision resolution), `snapshot` (git-worktree pinning), `reconcile` (report model,
classify/format layer), `segment`, the cobra surfaces and flag vocabularies, and the long-form help semantics
(conflict modes, drift matrix, status indicators) — those are the product spec to preserve.

## The replacement model (verified against lore.Build — the reference pattern)

`cmd/lore/lore/builder.go:90` is the sealed-era shape this family adopts:

1. **Plan**: a shared planning `RuntimeEnvironment` + `plan.NewProvider(env)`; planners produce
   `[]op.ExecutableUnit`.
2. **Stamp**: `op.NewOriginBase("writ", scope, op.NewAnnotationMap(map[string]any{...}))` — the writ bag
   (source root, target root, projects, segments, layers, commit hashes, dirty layers) rides annotations.
3. **Assemble**: `op.NewGraph(op.NewGraphSpec().WithOrigin(origin).WithUnits(units...))` (or
   `plan.Provider.AssembleDefinition` where policies/error actions are wired — the adopt/migrate pattern).
4. **Persist the plan**: `cli.WriteGraph(graph)` — the checksum-keyed immutable document.
5. **Run**: `op.NewGraphExecutor(graph, spec)` per scope with a confined root; `cli.WriteTrace(executor.Trace())`
   win or lose (step-21 R4).
6. **Read back**: deployed state = (graph document, latest trace) pairs — the trace's `GraphChecksum` keys the
   subdirectory; `Trace.Summarize()` tallies; node slots (immediates: `source`, `path`) supply the file
   inventory, read via the reporting helper pattern (`immediateString`).

## Target design (proposal — the open questions govern)

1. **A writ-side deployed-state readback** replaces `StateView` for decommission / upgrade / reconcile: enumerate
   writ graphs (+ latest traces), filter by `Origin.Tool()=="writ"` and scope, and project the node inventory
   (action name, source, target, project/layer from annotations or node metadata). Home and shape = open
   question 1.
2. **deploy**: keep tree walk + snapshot pinning verbatim; the tree's per-file action chains become planned
   invocations via `plan.Provider` (`file.link`, `template.render_*`, `encryption.decrypt`, `file.copy`);
   manifests resolve through `lore.Planner.PlanPackages` into the same unit list; per-scope assembly with origin
   annotations; `WriteGraph` + per-run `WriteTrace`; summary from `Trace.Summarize()`.
3. **decommission**: readback → per-scope `file.unlink` / `file.remove` graphs (same assembly seam); the
   signed/unsigned safety gate integrates with step 46 (its verify surface; sequencing interlocks — question TBD).
4. **upgrade**: readback filtered to copied files → re-planned render/decrypt chains for those entries; the drift
   matrix needs recorded content identity from deploy time (open question — trace/catalog vs. new recording).
5. **status** (settled rename; formerly reconcile): keep the report layer; entries source from the readback
   instead of `StateView`; each finding names its repairing command (missing/stale → `writ deploy`; orphan →
   `writ decommission`); the receipt-signature check calls the step-46 surface when it lands.
6. **Deletions**: `internal/execution` (whole package), the dead `cli.LoadLatestReceipt`-shaped calls, the
   hand-built node/edge machinery in both files, `upgradeFile`'s ancient one-off graph assembly.
7. **Stubs stay stubs** (`inspect`, `list`, `receipt`) unless ruled otherwise.

## Test plan (sketch — firms up as questions settle)

1. Deploy end-to-end over a layered `testdata` tree: links + a template + (sops-gated) a secret; per-scope
   graphs; trace persisted; summary correct; `--conflict` modes; collision reporting.
2. Readback round-trip: deploy → readback inventory matches the planned nodes; decommission removes exactly the
   inventory; upgrade regenerates only copied entries.
3. Reconcile classifications over crafted target states (linked / missing / conflict / orphan).
4. Dry-run serialization; dirty-layer refusal + `--allow-dirty`.

## Settled (2026-07-15)

1. **Readback home (question 1b)**: a writ-owned package (a `cmd/writ/writ/` sibling of `adopt` / `migrate`)
   implementing the readback, reading through the `internal/cli` store API plus its own directory enumeration. No
   `pkg/op` changes; the one store-layer extension is the run index (settled item 4), written beside the existing
   store bookkeeping.
2. **`writ status` replaces `writ reconcile`.** "Reconcile" promises mutation the command does not perform; the
   command is a report — what should be present, where each entry should come from, what's missing or different.
   `status` is the established name for actual-vs-intended reports (`git status`, `systemctl status`) and reads
   naturally beside the lifecycle verbs. "reconcile" retires entirely — no `--fix`: the repair for each finding is
   the existing lifecycle (missing/stale → `writ deploy`; orphan → `writ decommission`), and the report names the
   repairing command per finding. The internal `reconcile` package (the surviving report/classify layer) renames
   to match in the rewrite slice.

3. **Readback fold semantics (question 1a): a time-ordered fold, best effort.** Per scope, over the writ graphs'
   traces in timestamp order: a successful link/copy/render node marks its target deployed (action chain +
   source); a successful unlink/remove marks it gone; failed nodes don't count (outcomes read from the trace).
   Best effort because the store is user territory — graphs or traces may have been deleted out from under us; the
   fold works over whatever documents survive, skips a trace whose graph is missing (and vice versa), and never
   refuses because history is incomplete. Zero surviving knowledge is still a refusal for destructive consumers
   (decommission keeps its "no deployment history" guard — that is ignorance, not incompleteness).

4. **The run index (accepted 2026-07-15).** An append-only NDJSON index at the store root
   (`DevloreStateHome()/index.ndjson`), appended by the store writers themselves (`internal/cli.WriteGraph` /
   `WriteTrace` — the same class of bookkeeping as the `latest.yaml` symlink), one writer for every tool. A graph
   event records `{at, event, tool, scope, graph_checksum}`; a trace event records
   `{at, event, graph_checksum, trace_file}` and joins to its graph event by checksum (fallback: load the graph
   document). The index gives the readback its tool/scope lookup without parsing every graph document, and turns
   silent degradation into a finding: an index entry with no surviving document reports as a missing piece; a
   document with no index entry (pre-index history, or a recreated index) folds in anyway. Torn last line reads
   as absent (NDJSON). **`writ status` errs when the index is missing** (ruled); the fold's best-effort semantics
   otherwise stand.

5. **What `writ status` reports (accepted 2026-07-15).** Writ-tool runs only (`Origin.Tool() == "writ"` — deploy,
   adopt, and migrate's registration all stamp it via their environments' application name; `plan.Provider.Origin`
   sets scope, `AssembleDefinition` stamps the tool). Standalone lore runs are excluded — lore's state is its own
   surface to report (the shared store + index make a future `lore status` or unified roll-up cheap). Sections:
   1. **Layers** (the "where from"): the registered layer tree under `WritLayersDir()` — which of
      base/team/personal exist, link-mode targets, broken layer symlinks; migrate's registration surfaces here.
   2. **Deployed inventory, per scope** (the fold over deploy/adopt traces): each target classified (Linked /
      Copied / Missing / Conflict / Orphan / Stale / Modified) with its source (layer, project) and the repairing
      command per finding (missing/stale → `writ deploy`; orphan → `writ decommission`). Adopt entries fold in
      deliberately — an adopted file is a link writ created and manages thereafter; migrate's restructure moves
      files inside the repo and contributes nothing to fold.
   3. **Packages via writ** (`pkg.*` nodes in writ traces): what writ's manifests installed, as fact-of-record;
      package-manager drift (still installed?) is lore's concern, not duplicated here.
   4. **Store health**: index-vs-document detection findings ("history records N runs, M documents missing;
      findings may be incomplete"); a missing index is a hard error (settled item 4).

6. **Slicing and sequencing (question 2, agreed 2026-07-15).** One step, four slices fed one at a time:
   slice 1 = store index + readback + deploy; slice 2 = decommission + upgrade; slice 3 = `writ status`;
   slice 4 = close (`internal/execution` deleted, docgen greens, steps 18/22 close, stubs confirmed). Decommission
   ships WITHOUT the signed/unsigned safety gate — nothing signs today, so the gate would only demand `--force` on
   every run; step 46 wires the gate when the signer exists. **Demo addition (ruled)**: Scenario 2 of the demo
   milestone runs through `writ deploy noblefactor thenobles` against the user's personal environment repository
   (`../Personal`, to be restructured by the user) — recorded in
   [demo-milestone.md](../demo-milestone.md); slice 1 is its critical path.
7. **Sops wiring (question 3): dissolved — nothing to wire.** Verified: `sops.Client` is a zero-size,
   configuration-free struct ("the encrypted file carries its own recipients and is unlocked with ambient
   identities" — pkg/sops/client.go); recipients/format for encryption come from the `.sops.yaml` governing the
   source path, discovered per path. The old `WithSops(client)` spec plumbing and the crater's
   `sops.NewClient(cfg.SourceRoot)` calls are obsolete by design and simply die with the rewrite. The deploy docs
   note the operational requirement instead: decrypting deploys need ambient age identities.

8. **Drift-matrix content identity (question 4, settled 2026-07-15): record both tiers in the trace's ledger
   snapshot.** `LedgerEntrySnapshot` gains `Etag string` + `Digest string` (both omitempty, canonical digest form),
   captured in `ResourceCatalog.Snapshot()` for Active entries only, best effort (error → empty), at
   trace-snapshot time; `Rehydrate` ignores both (reporting metadata, not rebuild inputs). The signals are the
   sealed `op.Resource` interface's own pair — Etag the cheap token, Digest the honest hash — implemented across
   file/git/json/yaml/mem/appnet/service/pkg; git's Digest already detects repo content change (sha256 over HEAD
   SHA + dirty-tree stash-create TREE SHA, timestamp-free). Status-side drift checks run the catalog's own economy
   (`verifyLocationFreshness`, resource_catalog.go:507 — whose comment explicitly defers drift surfacing to "a
   future reconciliation pass", i.e. this work): live Etag vs. recorded Etag screens the unchanged majority; only
   mismatches compute a live Digest; the recorded Digest attributes source-changed vs. target-modified.
   **Charters its own small framework step** (the family step makes no `pkg/op` changes): the two snapshot fields +
   capture, plus the known gap — file DIRECTORY digests are `ErrUnimplemented`
   (pkg/op/provider/file/resource.go:254). Until that step lands, the family ships the conservative interim:
   upgrade skips + warns on any differing target (`--force` overwrites); status reports differing copied entries
   as modified-or-stale (indeterminate). The signal vocabulary is exactly two concepts: Etag, the cheap check
   (file: one stat — sha256 over the packed (size, mtime_ns, inode) tuple, no content read; git: HEAD short-id +
   dirty-tree suffix), and Digest, the expensive honest one (file: full content read + sha256, the way git hashes
   content; git: sha256 over HEAD SHA + dirty stash-create tree SHA).

9. **Command-surface trims (question 5, agreed 2026-07-15): delete the stubs.** `writ inspect`, `writ list`, and
   `writ receipt show|list` — registrations and their builder functions — die in the rewrite. A shipped CLI
   advertising commands that only error is clutter; their eventual jobs are better-owned elsewhere (`writ status`
   for the what/where queries; the store's YAML documents for direct inspection; step 46's `writ verify` for
   authenticity). A per-project/per-file `inspect` view charters as its own step against the readback if it earns
   its keep. The roadmap lives in the plan docs, not in `--help`.

10. **Conflict policy (finding 1 of slice 1, ruled 2026-07-15).** Source-side collisions need no policy (tree
    precedence + specificity settles them; losers reported). The occupied-target dimension is `op.ConflictPolicy`
    with exactly three values — `stop` | `skip` | `replace` (Backup/Overwrite collapse: replace ALWAYS archives,
    compensation requires it) — enforced by the FILE PROVIDER at the write seam, reading the announced "runtime"
    config section (`RuntimeEnvironmentConfig`, floor `stop`) live from `Application.Config`; writ's `--conflict`
    feeds the cli layer of the rollup (wired in step 47 slice 4). Chartered as
    [steps/49-conflict-policy-enforcement.md](steps/49-conflict-policy-enforcement.md); LANDED 2026-07-16 with
    two implementation amendments — the seam floor is `replace` (in-place updates are not conflicts; the suite
    proved a stop floor breaks every in-place updater) and the cautious stop default is writ deploy's layered
    pre-flight (readback-classified occupants; redeploys flow).

## Store layout

```
$XDG_STATE_HOME/devlore/                       # DevloreStateHome() (default ~/.local/state/devlore)
├── index.ndjson                               # NEW — the append-only run index, one JSON object per line
├── graphs/                                    # GraphsDir() — immutable plans, checksum-keyed, written once
│   ├── sha256-1f8a…c202.yaml                  #   e.g. writ deploy, scope=system
│   ├── sha256-77b3…09de.yaml                  #   e.g. writ deploy, scope=home
│   ├── sha256-9e41…55aa.yaml                  #   e.g. writ adopt (Home batch)
│   └── sha256-d0c4…7b31.yaml                  #   e.g. lore deploy docker
└── receipts/                                  # ReceiptsDir() — traces, one subdirectory per graph
    ├── sha256-1f8a…c202/
    │   ├── 20260715T183021Z.yaml              #   one trace per run, timestamped UTC
    │   ├── 20260716T090412Z.yaml              #   re-runs of the same plan accumulate here
    │   └── latest.yaml -> 20260716T090412Z.yaml
    ├── sha256-77b3…09de/
    │   ├── 20260715T183025Z.yaml
    │   └── latest.yaml -> 20260715T183025Z.yaml
    └── sha256-9e41…55aa/
        ├── 20260714T121502Z.yaml
        └── latest.yaml -> 20260714T121502Z.yaml
```

`graphs/` and `receipts/` are exactly today's store (internal/cli/receipts.go); the index is the only addition.
All tools share the one store and the one index; entries carry `tool`/`scope`, so the writ readback filters
without opening unrelated documents.

## Open questions

None — all five settled 2026-07-15 (items 1–9 above). The design round is closed; the work charters as step 47
(the family rewrite) plus step 48 (the ledger content-identity companion).
