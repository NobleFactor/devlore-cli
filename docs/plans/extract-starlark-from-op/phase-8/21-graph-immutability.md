---
title: "Step 21 — Graph immutability: consumer & test migration after the seal"
parent: "docs/plans/extract-starlark-from-op/phase-8.md"
issue: 275
status: in-progress
created: 2026-05-27
updated: 2026-05-30
---

## Context — what the seal landed

The framework half of step 21 (graph immutability + restartability) is **complete** in `pkg/op`
production code. `make vet` shows `pkg/op` proper compiling; the remaining red is entirely in
`pkg/op` tests, the gen test templates, the flow provider, and the two apps.

`Graph` is now **fully sealed**: every field is private (`kind`, `schemaVersion`, `checksum`,
`signature`, `timestamp`, `origin`, `resourceCatalog`, `root`, `unitsByID`), and access is only
through getters — `Root() *Subgraph`, `Origin() Origin`, `Edges() []Edge`,
`ResourceCatalog() *ResourceCatalog`, `Nodes()`, `Subgraphs()`, `UnitCount()`, `Summary` **removed**.
Construction is exclusively all-args and fallible:

```go
func NewGraph(origin Origin, units []ExecutableUnit, slots map[string]SlotValue,
    catalog *ResourceCatalog, rollback *Subgraph, retryPolicy *RetryPolicy,
    sopsClient *sops.Client) (*Graph, error)

func NewSubgraph(id string, action Action, children []ExecutableUnit,
    slots map[string]SlotValue, retryPolicy *RetryPolicy, parent *Subgraph) (*Subgraph, error)
```

**Where things actually moved — diverges from step 21's original prediction.** Step 21 predicted
`State` / `Rollback` / `summary` / `Catalog` would move onto `RuntimeEnvironment`. The actual landing:

| Removed from `*op.Graph` | Predicted home | **Actual home** |
|---|---|---|
| `State` (mutable run state) | `RuntimeEnvironment` | **executor** — `RunState` (`run_state.go`) + `Trace` (`trace.go`); read via `executor.State()` / `RunStateFailed` |
| `Summary()` / failed-count | — | folds into `executor.State() == RunStateFailed` (and `Trace` for per-unit detail) |
| `Catalog` | `RuntimeEnvironment` | **carried on the graph** (`graph.resourceCatalog`, getter `ResourceCatalog()`); `GraphExecutor.Run` clones it onto a fresh per-run `RuntimeEnvironment.ResourceCatalog` |
| `ctx` + `Rebind` / `Unbind` | removed | removed |
| `ExecuteWithStack` | — | removed; dispatch is the executor's job (children dispatch via `ActivationRecord.DispatchChild`) |
| `RuntimeEnvironment.Results map[string]any` | removed | removed; resolved values come from the receipt stack keyed on `Receipt.UnitID()` |
| `Action.Do(record, map)` second arg | — | dropped — signature is now `Do(record *ActivationRecord) (Result, Complement, error)` |

The step-21 row in `phase-8.md` is updated to record this landed state; this sub-plan covers the
remaining consumer/test/template migration.

## What's broken — 5 buckets

| # | Where | Errors | Nature |
|---|---|---|---|
| 1 | `pkg/op/provider/flow/helpers.go` | 2 | framework-adjacent |
| 2 | `pkg/op` tests — `dependencyview_test.go`, `preflight_test.go` | ~10 | framework's own tests |
| 3 | gen test **templates** — `action.gen_test.go.template`, `receiver_type.gen_test.go.template` | ~15 providers | stale templates |
| 4 | `cmd/lore/lore` — `builder.go`, `commands.go` | ~10 | broken app |
| 5 | `cmd/writ/writ` — `commands.go`, `graph_builder.go`, `migrate/plan_builder.go`, `migrate/session.go`, `migrate/format.go` | many | broken app (largest) |

## Approach per bucket

### Bucket 1 — flow provider helpers (`pkg/op/provider/flow/helpers.go`)

Both fixes route flow back onto the single executor-owned walk + the receipt stack. No new framework
API — the mechanisms already exist and a sibling helper already uses them (see resolved Q1/Q2 below).

- **`dispatchBodyChildren` (`:94`, called from `Provider.Gather` at `provider.go:242`)** loops
  `subgraph.Children()` calling the deleted `graph.ExecuteWithStack(...)`. Its sibling `dispatchWithRetry`
  (`:134`) already does it the right way. Change the signature from `(ctx, graph, subgraph, stack, frame)`
  to `(activation *op.ActivationRecord, ctx, subgraph, stack, frame)` and call
  `activation.DispatchChild(ctx, child, stack, frame)` per child. `Gather` drops the
  `graph := activation.Graph` guard (`provider.go:209-212`) — it existed only to feed the old doorway.
- **`resolveDispatchedValue` (`:245`)** is the last reader of the removed `RuntimeEnvironment.Results`.
  Replace both lookups with the stack the activation already carries: `*op.Invocation` →
  `activation.Stack.ResultByUnitID(v.Target.ID())`; `*op.Promise` →
  `activation.Stack.ResultByUnitID(v.Unit().ID())`. Guard on `activation.Stack != nil`.

### Bucket 2 — `pkg/op` tests

- **`dependencyview_test.go:20,24`** — `&Graph{Root: root}` struct literals → build via `NewGraph(...)`
  all-args (mirror the step-22 `stubSubgraph` / `marshalers_test` migration), or a package-private test
  constructor if one exists.
- **`preflight_test.go`** — `g.Rebind` / `g.Unbind` removed. Binding direction reversed
  (RuntimeEnvironment → graph); rewrite these tests against the new binding entry point, or delete the
  cases that asserted the old mutable-rebind contract.

### Bucket 3 — gen test templates (edit template + regenerate; never the `.gen` files)

Templates live at `star/extensions/com.noblefactor.devlore.Actions/templates/`.

- **`action.gen_test.go.template`** — calls `action.Do(record, map[string]any{...})`. Drop the second
  arg to match `Do(record)`.
- **`receiver_type.gen_test.go.template`** — builds `op.RuntimeEnvironment{Registry: ...}`. The env
  field is `ReceiverRegistry` (the spec field stays `Registry`); update the literal accordingly.
- Regenerate via `make generate` (or `make build`) and confirm all `*/gen/*.gen_test.go` recompile.
  ~15 provider gen packages are affected.

### Bucket 4 — `cmd/lore/lore`

- **`builder.go`** — imperative `op.NewGraph()` / `op.NewSubgraph(id, action)` / `graph.AddSubgraph` /
  `node.SetSlot` / `target.AddChild` → gather-then-construct: assemble children + slots first, then one
  `NewSubgraph(...)` / `NewGraph(...)` all-args call (same shape step 22 applied to the in-package
  builders and flow planners). Handle the new `(_, error)` returns.
- **`commands.go:293`** — `buildResult.Graph.State == op.StateFailed` → `executor.State() == op.RunStateFailed`
  (the executor is already in scope at that call site).

### Bucket 5 — `cmd/writ/writ` (largest)

writ builds graphs imperatively across five files. Every site migrates to gather-then-construct:

- `commands.go:666` `op.NewGraph()` + `:668` `graph.AddNode(node)` loop + `:670` `graph.Root.SetEdges(edges)`
  + `:725–730` `node.SetSlot(...)` → assemble nodes/slots/edges, then `NewGraph(...)` all-args.
- `commands.go:691` `graph.Summary().Failed() > 0` → `eng.State() == op.RunStateFailed`.
- `graph_builder.go` — `g.AddNode`, `node.SetSlot`, `g.Root.AddEdge`, `g.Origin.TargetRoot = …`
  (Origin is now a getter; the target-root must be supplied at construction via the `Origin` arg).
- `migrate/plan_builder.go` — many `p.graph.AddNode` / `node.SetSlot` / `p.graph.Root.AddEdge` sites.
- `migrate/session.go` — `s.graph.AddNode` / `node.SetSlot`.
- `migrate/format.go:102` — `graph.Root.Edges()` → `graph.Root().Edges()` (read path; mechanical).

**Scope note.** phase-8.md deferred writ **migrate** cleanup (the nil-activation behavioral rewire) to
a follow-on PR. That deferral was about runtime behavior. The graph-**construction** breakage in
`migrate/*` is new (caused by the seal) and must be fixed for the branch to compile — so the
construction migration is in scope here; the deferred nil-activation behavioral rewire stays out.

**Progress (2026-05-29).** `migrate/*` graph-construction is migrated — `format.go` (getter),
`plan_builder.go` (env-sourced `plan.Provider`; accumulate invocations + ordering; topo-sort →
`Assemble`), `plan.go` (`buildGraphFromRegistry` inside an `op.Plan` closure; internal `buildMigration`
returns the op source so the interactive `Session` can re-derive its graph), `session.go` (holds the
editable op source; rebuilds the immutable graph on each add/remove). The package compiles except the lone
`cli.WriteReceipt` site (cleared by the trace store). The `cmd/writ/writ` package itself is **blocked on
lore** — it imports `lore.Planner` and the nuked `execution.StateView`, so it cannot compile until lore
does (see Bucket 4; lore is now sequenced first). Receipt→Trace model decided: graph and trace are distinct
(1 graph : many traces); the graph persists on first run to `GraphsDir`; traces write to `cli.ReceiptsDir`
keyed by graph checksum; tie-back is `Trace.GraphChecksum`.

## Sequencing

1. **Bucket 3 (templates) + Bucket 1 (flow)** — framework-adjacent; unblocks the gen packages and the
   flow provider so `pkg/op/...` is fully green before touching apps.
2. **Bucket 2 (pkg/op tests)** — closes the framework's own test surface.
3. **Bucket 4 (lore)** — smaller app migration; validates the gather-then-construct pattern end-to-end.
4. **Bucket 5 (writ)** — largest; apply the validated pattern across the five files.

## Progress (2026-05-27)

- **Bucket 1 — flow helpers: complete.** `dispatchBodyChildren` routes through
  `ActivationRecord.DispatchChild`; `resolveDispatchedValue` reads `activation.Stack.ResultByUnitID`;
  `Gather` dropped the `graph` guard.
- **Bucket 1b — Planner retry/errorAction threading: complete.** `errorAction, retryPolicy` appended to
  `Planner.Plan`; `ActionPlanner` applies them via the in-package `setErrorAction` / `setRetryPolicy`; the 4
  flow planners + `planSubgraphFromParams` pass them to `NewSubgraph`; `plan.Provider.invocation` passes them
  and drops the sealed setters; the two writ `Planner().Plan` callers pass `nil, nil`.
- **Bucket 3 — gen test templates: complete.** `action.gen_test.go.template` dropped `Do`'s map arg;
  `receiver_type.gen_test.go.template` renamed the `RuntimeEnvironment` field `Registry` → `ReceiverRegistry`;
  regenerated all providers. powershell/ui needed a forced regen — their Make rules key on `provider.go`, not
  the templates, and powershell is also omitted from the `generate` aggregate target (latent Makefile gap,
  noted, not fixed here).
- **Bucket 2 — pkg/op tests: complete.** `dependencyview_test` uses the private `root` field; `preflight_test`
  dropped 9 vestigial `g.Rebind` / `g.Unbind` pairs (binding is env → graph now, and `bindVariables` reads
  `e.environment`); `resource_catalog_test` passes `nil` (empty producer stamp) to `GetOrCreate` and the now-
  unusable `emptyActivation` helper was removed (`*ActivationRecord` no longer implements `ExecutableUnit`).
- **Revealed + fixed in production:** `validate.go:50` compared the `Root()` *method value* to nil (always
  false — a stale field reference from the seal) → `g.Root() == nil`. Masked until the pkg/op test files
  compiled, because vet skips its analyzers when a package's tests don't build.

**pkg/op is fully green** — production, generated tests, and hand-written tests compile, `make vet` is clean,
and `pkg/op` tests pass. Remaining: **Bucket 4 (lore)**, **Bucket 5 (writ)**.

**Bucket 4 (lore) — DEFERRED pending design (2026-05-27).** Not a mechanical gather-then-construct swap.
`cmd/lore/lore/builder.go` is entangled with the pre-Phase-8 mutable-graph model on three fronts:
1. `Build` does `graph := op.NewGraph()` then writes `graph.Origin` and threads the live graph through
   `Planner.PlanPackages(graph, …)` / `PlanByName(graph, …)`, which populate it in place.
2. `addNativePMNodes` builds nodes via `op.NewNode(...)` + `node.SetSlot(...)` + `target.AddChild(...)`.
   Node-with-slots has no public constructor post-seal — the sealed path is `method.Planner().Plan(...)`
   (as writ adopt does), so these become planner calls and the flow inverts to gather-then-construct
   (`addNativePMNodes` returns `[]ExecutableUnit`; `buildPackageNodes` gathers children → `NewSubgraph`;
   `Build` gathers subgraphs → `NewGraph`).
3. `executeScriptAction` runs Starlark phase scripts (`install` / `provision`) against the bridge runtime
   (`prepareScriptEnv`), and those scripts use `plan.*`. Under Phase 8 `plan.*` returns **detached
   invocations** assembled at the end — reconciling lore's "script mutates the live graph" flow with the
   detached-invocation + assemble-at-end model is the design question. Pick the approach before coding.

**Un-deferred 2026-05-29 — lore is sequenced next (writ depends on it).** Bucket 5 (writ) stalled because
`cmd/writ/writ` imports `cmd/lore/lore` for `lore.Planner` (commands.go, graph_builder.go) AND depends on
`internal/execution/stateview.go` (`StateView`/`StateViewBuilder`/`ViewOptions`/`FileEntry`, ~9 sites) — and
commit 37b900c nuked stateview.go. Go compiles whole packages, so writ cannot build until lore builds; the
original sub-plan sequenced lore→writ for exactly this reason. Agreed architecture (2026-05-29):
- **Resurrect `internal/execution/stateview.go` → move to `pkg/op` and upgrade.** It was graph-derived
  (`StateViewBuilder.Build` loaded serialized-graph "receipts" and walked `g.Nodes()`). The upgraded
  framework component is **trace-derived**: it builds the historical view purely from the traces in
  `ReceiptsDir` (the record of what executed) and never loads the graph. `HistoryRecord.Status` derives
  from the receipt's `Err()` (`op.Status` is gone). This requires the trace to be **self-describing** —
  the receipt must carry the producing unit's `Origin` (project) and `Layer`, stamped at `Commit` (which
  already receives the unit), and `op.Trace` must carry the graph's `Scope`. The old view sourced
  project/layer/scope from `node.Origin` / `node.Layer` / `graph.Origin.Scope`; the receipt carries none of
  these today, so enriching the trace is a prerequisite. The graph loads only when an operation needs the
  *plan* itself (restart).
- **Lift `lore.Planner` → `internal` as a shared component.** Used by both lore and writ; relocating it
  removes writ's app→app import. Seal-migrate it too: `PlanPackages` / `PlanByName` return units/invocations
  instead of mutating the graph.
- Then fix lore's `builder.go` (fronts 1-2 mechanical; front 3 — script-mutates-graph — still the open
  design question) and writ unblocks.

**Bucket 4 progress (2026-05-29).** Foundational pre-work landed:
- **Marshalers dissolved** — `pkg/op/marshalers.go` removed; `Graph` / `Node` marshalers moved into
  `graph.go` and `Subgraph`'s into `subgraph.go` (each marshaler with its type, in the proper regions).
  `pkg/op` compiles clean.
- **cli trace store created** — `internal/cli/receipts.go`: `GraphsDir` / `ReceiptsDir`, `WriteGraph`
  (idempotent first-run persist), `WriteTrace` (timestamped run file + `latest.yaml` symlink),
  `LoadLatestTrace` / `LatestTracePath` / `LoadTrace`; keyed by graph checksum, tie-back via
  `Trace.GraphChecksum`. Compiles clean.

**Origin model (2026-05-29).** Layer is writ tool metadata that predates Subgraphs and got stamped on
`Node` by historical accident. `Node.Origin` (project) is the same kind of leftover. Both retire onto a
generic per-unit Origin field; the framework stops carrying tool-specific vocabulary.

Two Origin roles, two structs, different shapes:
- **Graph.Origin** — the whole-graph setup the tool was operating in (writ example: `Tool`, `Scope`
  Home/System, `SourceRoot`, `TargetRoot`, repos covered + `CommitHashes` per repo, project list, segment
  list).
- **ExecutableUnit.Origin** — where this one unit lives within that setup (writ example: `Project`,
  `Layer`/repo).

Shape is tool-specific. Mechanism: `type op.Origin = any` (type alias). Tools define their own concrete
types (`writ.GraphOrigin{...}`, `writ.UnitOrigin{Project, Layer}`); framework treats them opaquely. Lines
up with how `Slots` / `Result` / `Complement` already work and with the `map[string]any` wire round-trip
the framework's `Convert` cascade can retype.

Origin on every `ExecutableUnit`. Promoted via the embedded `executableUnit` base — both `*Node` and
`*Subgraph` carry `origin any` plus an `Origin() any` accessor on the interface. No type assertions in
framework code.

All constructors accept Origin: `NewGraph(origin, units, ...)`, `NewSubgraph(id, action, ..., origin)`,
`NewNode(id, action, origin)`, `Method.Planner().Plan(provider, receiverType, method, args, slots,
errorAction, retryPolicy, origin)`, `plan.Provider.Assemble(invocations, frameBindings, errorAction,
retryPolicy, origin)`. Set at construction, immutable thereafter (matches the seal).

**Migration of today's `op.Origin` struct** (writ-flavored fields `Scope` / `SourceRoot` / `TargetRoot` /
`Projects` / `Segments` / `CommitHashes`): moves out of `pkg/op` to `cmd/writ/writ` as
`writ.GraphOrigin`. `op.Origin` becomes the type alias. writ also defines `writ.UnitOrigin{Project,
Layer}` and stamps it on each unit it builds. `Node.Origin` and `Node.Layer` public fields are removed.

**Receipt + trace fallout.** `ReceiptBase.Commit` stamps `unit.Origin()` opaquely (no `*Node` type
assertion). `ReceiptSnapshot` carries one opaque `Origin any` instead of the previously-proposed
`Origin` / `Layer` strings. `op.Trace` carries the graph's Origin as opaque `any` so the trace is
self-describing without loading the graph for `Scope` (writ casts to `writ.GraphOrigin` and reads
`Scope` from there). The previously-proposed `Trace.Scope` field is dropped; scope is part of
`writ.GraphOrigin`.

**Note on the in-flight step-1 commit.** It added `ReceiptBase.origin` / `layer` string fields and a
`*Node`-typed stamp; both are now superseded and will be backed out (or replaced inside the wire-form
commit) when the Origin migration step lands.

Step sequence (status):

> **Steps 2, 4, and 5 below are SUPERSEDED** — see "Origin redesign (2026-05-30)" at the end of this
> section. The opaque `type op.Origin = any` alias and the per-`ExecutableUnit` `Origin()` accessor were
> abandoned: unit-level origin/layer now ride the existing `AnnotationMap` (landed in commit fec3791), and
> graph-level `Origin` stays a single concrete struct with an `AnnotationMap` extension field — no interface,
> registry, or seal. Steps 1 and 3 landed.

1. cli trace store — **done**.
2. **Origin migration** — `type op.Origin = any`; `Origin` on every `ExecutableUnit` (alias + interface
   accessor + `executableUnit` field); constructors thread it; relocate today's `op.Origin` struct to
   `writ.GraphOrigin`; define `writ.UnitOrigin{Project, Layer}`; supersede the in-flight
   `ReceiptBase.origin` / `layer` from step-1 in favor of opaque `unit.Origin()` — **next**.
3. **Receipt wire-form expansion** — `op.ReceiptData` named type (renamed from the originally-proposed
   `ReceiptSnapshot`, per the `Payload → Data` wire-type convention from commit c14a242); expand
   `Snapshot` / `Restore` / `MarshalYAML` to round-trip all base fields including opaque `Origin`;
   update the concrete receipt types (`file` / `git` / `pkg` / `service` / `encryption`) to embed
   `ReceiptData`. **In progress** — named type + `Snapshot` / `Restore` / `MarshalYAML` landed; the
   five concrete receipts still pass the inline anonymous struct to `Restore` and remain red.
4. **`op.Trace`** carries the graph's Origin opaquely; `graph_executor.go` stamps it at capture.
5. Resurrect `stateview.go` → `pkg/op`, **trace-derived** (no graph load); casts `unit.Origin()` to
   `writ.UnitOrigin` for project / layer grouping and `trace.GraphOrigin` to `writ.GraphOrigin` for
   scope filtering.
6. Lift `lore.Planner` → `internal` shared component + seal-migrate (return units, not mutate the
   graph).
7. lore `builder.go` fronts 1-2 (mechanical gather-then-construct).
8. lore front 3 (script-mutates-graph) — open design.
9. writ finishes (receipt sites → trace store; `graph_builder.go` / `commands.go` construction) —
   unblocked once lore builds.

## Origin redesign (2026-05-30) — concrete struct + `AnnotationMap` extension model (SETTLED)

Supersedes the opaque-alias design in **steps 2, 4, 5** above *and* the interim sealed-interface + registry
sketch that briefly occupied this section. **Final decision: `op.Origin` is a single concrete struct. The
framework serializes/deserializes just this base; tools extend through `Annotations`. No interface, no seal,
no registry, no polymorphic deserialization.**

```go
// pkg/op — the only Origin that exists framework-side, and the only thing persisted.
type Origin struct {
    Tool        string
    Scope       string
    Annotations AnnotationMap   // extension door; map[string]any, native YAML/JSON round-trip
}
```

**Extension model (the deliberate good part).** Tool-specific data lives in `Annotations`. The framework stays
dumb — nothing dispatches on type. Persistence contract: **anything beyond `Tool`/`Scope` must live in
`Annotations` to survive a round-trip** (lossless — the data persists, only untyped).

**Rationale.** The organizing principle: *the framework carries no tool vocabulary — it persists the base and
leaves the door open to tools via `Annotations`.* Three things justify it.

1. **It matches the real coupling surface.** The framework's genuine need from Origin is two strings: `Scope`
   (the graph filename, `graph.go:206`) and `Tool` (trace identity). Everything else is a tool concern. The prior
   thirteen-field struct hard-coded writ/lore vocabulary — `CommitHashes`, `Features`, `Segments`, `Layers`, … —
   into `pkg/op`, fields whose own doc comment admitted the framework "never inspects" them. `{Tool, Scope,
   Annotations}` encodes exactly what the framework touches and pushes the rest to where it's owned.

2. **It's the simplest thing that round-trips.** One concrete type means `yaml.Unmarshal` just works — no
   discriminator in the wire form, no factory registry, no global registration or init-ordering, no dual
   raw-plus-typed representation. Because Go has no reflection-by-name, *any* interface-typed Origin forces you to
   build a registry yourself (as gob, Boost, and protobuf-`Any` all do); a single persisted type avoids the whole
   apparatus instead of working around the language — and skips the name-driven-deserialization class that drove
   Java/.NET to retreat from it for RCE reasons.

3. **Ergonomics are a tool-side luxury, paid on demand.** A tool that wants typed, cast-free access embeds the
   base and projects typed accessors over `Annotations`, caching the `any`→T conversion. That cost is borne only
   by the tool that wants it, only when it wants it — never by the framework, never by a tool content to read the
   map directly.

**Immutability vs. a mutable property bag (cf. Azure Cosmos DB).** The pattern — extension fields in a map, the
map is what serializes, derived types add typed accessors over it — mirrors the Cosmos DB SDK's `JObject`-backed
resources (`GetPropertyValue<T>` / `SetPropertyValue<T>`). The deliberate difference: `Origin` is sealed at
construction, so our accessors are **read-only** — there is no `Set`. A tool-side typed accessor is a pure
read-and-memoize, and because the backing `Annotations` map never changes, the cache can never go stale (no
write-through, no invalidation). A tool that mutates its own in-memory derived view does **not** alter the
graph's `Origin`, and the change is **not** persisted; post-construction updates are not reflected, by design.

The result is also reversible in spirit: persisting only the base discards no *information* (all tool data
survives in `Annotations`, merely untyped), so a future feature that genuinely needs typed reconstruction can
re-type from the map with no format migration.

**Landed this session:**
- **Unit-level origin/layer ride the existing `AnnotationMap`** (committed, fec3791). `ReceiptBase.Commit`
  stamps `origin`/`layer` from `unit.Annotations().Get(...)`; `Receipt.Origin()`/`Layer()` and the wire
  fields widened `string → any`; `Node.Origin`/`Node.Layer` struct fields removed.
- **`Origin.Annotations AnnotationMap`** field added (`graph.go`), first field of the struct *(uncommitted)*.
- **`Receipt.receiptBase()` widened to `receiptBase() *ReceiptBase`** — unrelated to Origin; a consistency fix
  to the `Receipt` seal matching `Provider`/`Resource`. Kept *(uncommitted)*.

**Evidence trail (why the trace settled it against the registry):**
- Tree-wide, the only `op.LoadGraph` caller is `pkg/op/provider/plan/provider.go:245`, and it reads only `Scope`.
- writ never calls `LoadGraph`; its graphs are built fresh in-memory each run (`builder.Build()`).
- writ's entire read-back surface is `StateView`-derived from `cli.ReceiptsDir()` (`loadStateView` →
  `execution.StateViewBuilder`), shared by deploy/upgrade/reconcile.
- The unrealized writ commands (`inspect`, `list`, `receipt show/list`, `adopt --from-receipt`) ride the same
  receipt/`StateView` path — confirmed via CLI help + `docs/guides/writ/manage-environments.md`. `inspect` shows
  deployed-files / checksums / drift (receipts + live filesystem), not deserialized-graph Origin.
- **lore trace (2026-05-30) confirms the same, more strongly:** lore reads **zero** Origin fields, never calls
  `LoadGraph`, and has no receipt/`StateView` path in `cmd/lore`. Its read-back commands (`upgrade`,
  `decommission`, `reconcile`, `inspect`, `list`, `resolve`, …) are all `"not yet implemented"` stubs; only
  `deploy` is realized. `lore inspect <package>` is package-scoped (registry/manifest lookup), not graph-Origin.

**Needs analysis (the ground truth that justifies the shape):**

| Consumer | Sets at construction | Mutates post-construction (seal conflict) | Reads back |
|---|---|---|---|
| framework (`pkg/op`) | — | — | `Scope` only (filename, `graph.go:206`) |
| writ | `Tool, Scope, SourceRoot, TargetRoot, Projects, Segments` | `TargetRoot` (`graph_builder.go:344`), `Layers` (`:459`), `CommitHashes`+`DirtyLayers` (`commands.go:155-156`) | `Scope` (~8 sites), `TargetRoot` |
| lore | `Scope, Packages, TargetPlatform, Features, Settings`; **omits `Tool`** | — | none |

Two findings:
1. **The seal conflict is writ's.** The four `g.Origin.X = …` mutations violate immutable Origin and must be
   hoisted to compute-before-`NewGraph` (`CommitHashes`/`DirtyLayers` are derived *after* the build today).
   Real writ migration work, independent of the interface decision.
2. **Almost everything is write-only archival.** Only `Scope` (framework + writ) and `TargetRoot` (writ) are
   ever read back. The other eleven fields (`Projects, Segments, Layers, CommitHashes, DirtyLayers,
   SourceRoot, Packages, TargetPlatform, Features, Settings`) are set into Origin for the persisted record and
   never read typed — natural `AnnotationMap` residents.

**Implementation work this implies:**
1. Reshape `op.Origin` to `{Tool, Scope, Annotations}`; evict the eleven tool-specific fields
   (`CommitHashes, DirtyLayers, Layers, Projects, Segments, SourceRoot, Packages, TargetPlatform, Features,
   Settings`) — tools write them into `Annotations` at construction.
2. **writ seal conflict:** hoist the four `g.Origin.X = …` mutations to compute-before-`NewGraph`
   (`CommitHashes`/`DirtyLayers` are derived after the build today).
3. **`TargetRoot` placement:** writ reads it (3 sites). Per the extension model it lives in `Annotations`,
   fetched via a writ-side cached accessor; promotion to a base field is the only alternative and is not planned
   unless it proves common across tools.
4. **lore `Tool` gap:** lore must stamp `Tool = "lore"` (`builder.go:241`) — still required even without a
   registry; `Tool` is the trace-identity / `Scope`-filename key.
5. Step 4 (trace carries `Origin`) and step 5 (trace-derived `StateView`) read `Scope`/`Tool` off the concrete
   struct — no decode machinery.

**Open question — RESOLVED (2026-05-30).** Settled in favor of the single concrete struct. The trace shows no
tool ever reads typed tool-specific Origin off a disk-loaded graph (writ's read-back is entirely
receipt/`StateView`-derived; the lone `LoadGraph` caller reads only `Scope`). Registry / interface / seal
dropped; the `Receipt.receiptBase()` widening is independent and kept.

## Command surface — writ & lore (2026-05-30)

Captured during the Origin needs analysis. The point it establishes: every read-back / inspection command —
realized or stubbed — is receipt/`StateView`-derived, not graph-Origin-derived, which is what let the registry be
dropped. For both tools every command is *designed* (CLI help) and either implemented or cleanly stubbed;
documentation is near-complete (one gap, noted).

**writ — 6 implemented, 5 stubbed.**

| Command | Handler | Status |
|---|---|---|
| `deploy` | `runDeployV2` | ✅ builds + executes per-scope graphs |
| `decommission` | `runDecommission` | ✅ |
| `upgrade` | `runUpgrade` | ✅ loads `StateView`, upgrades copied files |
| `reconcile` | `runReconcile` | ✅ builds reconcile report |
| `adopt` | `runAdopt` | ⚠️ implemented except `--from-receipt` |
| `migrate` | `runMigrate` | ✅ (`migrate/` package) |
| `inspect <project\|file>` | inline | ❌ `"not yet implemented"` (`commands.go:1215`) |
| `list` | inline | ❌ (`commands.go:1229`) |
| `receipt show [name]` | inline | ❌ (`commands.go:1284`) |
| `receipt list` | inline | ❌ (`commands.go:1294`) |
| `adopt --from-receipt` | flag path in `runAdopt` | ❌ (`adopt_cmd.go:233`) |

Partial within implemented commands: `TODO(step 15)` at `commands.go:693` (per-node error → recovery-stack
receipt) and `:906` (skipped-status filter dropped with `Node.Status`).

**lore — 3 implemented, 11 stubbed (incl. manifest's 5 subcommands).**

| Command | Implementation | Documented (guide) |
|---|---|---|
| `deploy` | ✅ `runDeploy` | pipeline, deploy-packages, index, registry |
| `search` | ✅ `runSearch` | registry |
| `onboard` | ✅ `runOnboard` (`onboard/` pkg) | registry |
| `upgrade` | ❌ stub (`commands.go:328`) | deploy-packages |
| `decommission` | ❌ (`commands.go:342`) | deploy-packages |
| `reconcile` | ❌ (`commands.go:358`) | deploy-packages |
| `bundle` | ❌ (`commands.go:369`) | registry |
| `manifest` (create/validate/test/show/update) | ❌ all 5 (`commands.go:399–467`) | pipeline, index, create-manifests, registry |
| `list` | ❌ (`commands.go:576`) | ⚠️ **none** (CLI-help only) |
| `resolve` | ❌ (`commands.go:591`) | registry |
| `update` | ❌ (`commands.go:602`) | registry |
| `publish` | ❌ (`commands.go:785`) | create-manifests, registry |
| `audit` | ❌ (`commands.go:811`) | pipeline, registry |
| `inspect <package>` | ❌ (`commands.go:765`) | deploy-packages, registry |

All lore commands carry a CLI-help spec (designed). Documentation gap: `list` (cobra `Short` only, no guide).
`lore inspect <package>` is package/registry-scoped, not graph-Origin.

## Status snapshot (2026-05-29)

`ReceiptSnapshot` was renamed to **`ReceiptData`** (full 12-field wire shape, relocated into a
`// region SUPPORTING TYPES` in `pkg/op/receipt.go`). No `ReceiptSnapshot` references remain in the
tree. Plan prose above this section predates the rename and still uses the old name.

Build matrix — every red traces to two root causes, nothing scattered:

| Area | Status | Cause |
|---|---|---|
| `pkg/op` core (compile + tests) | 🟢 | builds; `pkg/op` tests pass |
| `op.ReceiptData` named type + `Snapshot` / `Restore` / `MarshalYAML` | 🟢 | round-trips |
| Concrete receipts: file / git / pkg / service / encryption | 🔴 | **Root A** — still pass inline anon struct to `Restore(ReceiptData)` |
| Providers archive / encryption / flow (+ `*/gen`) | 🔴 | cascade A |
| Other providers (mem / json / yaml / function / shell / template / ui / regexp / appnet / platform / powershell + gen) | 🟢 | unaffected |
| `cmd/lore/lore` | 🔴 | **Root B** — pre-seal API (`NewGraph` / `NewSubgraph` / `AddSubgraph` / `SetSlot` / `.State`) |
| `cmd/writ/writ` (+ adopt, migrate) | 🔴 | cascade A + B |
| `cmd/star/star`, starcode, inventory | 🔴 | cascade A |
| `cmd/devlore-test`, docgen, internal/e2e, pkg/op/inventory | 🔴 | cascade A |
| `internal/*`, most `cmd/star` providers, `cmd/writ` sub-pkgs | 🟢 | pass |
| Binaries: lore / star / writ / devlore-test | 🔴 | blocked by A and/or B |

Two fixes clear everything: **A** — migrate the five `Restore` call sites to `ReceiptData` (mechanical,
identical shape); **B** — migrate `cmd/lore/lore/builder.go` + `commands.go` to the sealed API.

**Open ordering question (unresolved, user's call).** The wire-form currently carries `origin` / `layer`
as **strings**; step 2 (Origin → opaque `type op.Origin = any`) supersedes those. Either do step 2 first
and build the wire-form once on the opaque Origin, or finish step 3 on strings now and rework in step 2.

## Resolved decisions

- **Q1 (single subgraph walk) — resolved.** There is one walk: `ExecutableUnit.Execute` recursion, entered
  at `graph.Root().Execute(...)` (`graph_executor.go:264`). Structural subgraphs (`Action() == nil`) loop
  `child.Execute(...)` directly (`subgraph.go:335`); bound combinators (gather/choose/wait_until) re-enter the
  *same* walk via `ActivationRecord.DispatchChild`, whose closure the executor installs at
  `subgraph.go:360-367`. Traversal is implemented once; combinators feed children back in, they never
  re-traverse. `graph.ExecuteWithStack` was a duplicate doorway and is gone. flow's `dispatchBodyChildren`
  is the only caller that hadn't migrated — Bucket 1 routes it through `DispatchChild` (its sibling
  `dispatchWithRetry` already does).
- **Q2 (receipt walk replaces `RuntimeEnvironment.Results`) — resolved.** `Results` is redundant. Every
  dispatch exit pushes a receipt carrying `UnitID` + `Result` (`recovery_stack.go:60-66`);
  `RecoveryStack.ResultByUnitID(unitID) (any, bool)` (`recovery_stack.go:196`) walks LIFO for the latest
  result. `PromiseValue.Resolve` already resolves this way (`subgraph.go:298-299`,
  `ActivationRecord.Stack` at `activation_record.go:55-56`); only `resolveDispatchedValue` still reads
  `Results`. The executor owns the top-level stack (`graph_executor.go:262`); each combinator iteration
  mints its own (`provider.go:239`) and `PushNested`s it as a saga boundary. **One verify at
  implementation:** `ResultByUnitID` does not descend nested substacks (`recovery_stack.go:186-188`) —
  combinators push a parent-level receipt with their overall result, so cross-boundary lookups resolve
  there; for `plan.case(when=upstream, …)` the upstream and the case dispatch are siblings on the same
  stack (direct hit). Confirm the `when` upstream landed on `activation.Stack` when wiring.
- **Q3 (writ Origin) — resolved.** `Origin` (with `TargetRoot`) is assembled in full before `NewGraph`.
  `NewGraph` is the one and only construction path and the graph is immutable thereafter. writ's
  `g.Origin.TargetRoot = …` post-construction mutation is removed — the value is supplied via the `origin`
  constructor argument.

## Exit criteria

- `make vet` clean across all packages.
- `make build` clean — all four binaries (lore, star, writ, devlore-test).
- `make test` green. This rolls in step 21's original test-triage backlog (the "22 reds": `TestImm*`,
  `TestWalkTreePlanned`, `TestCLI_*`, `TestLintCopyright_*`, `TestSourceFile_StarlarkIntegration`) — once
  the apps and templates compile, re-run to see which reds were compile-driven vs. genuine behavioral
  failures still needing the 21.1–21.3 sub-step work.
- **Test-triage progress (2026-05-28):**
  - **Row 1 (archive)** green — nil-registry panic fixed (test runtime environment + `ProviderByType` guard).
  - **Row 2 (`cmd/devlore-test` immediate suite)** green:
    - A: `file` slot-fill reds (Copy / Move / Link / Backup / Unlink / FileLifecycle) — a receipt-work
      `reflect.Value` regression in `Method.Invoke` (the raw reflected result was stored on the receipt,
      so promise consumers got a `reflect.Value`).
    - B: `TestWalkTreePlanned` — **deferred to step 24** (function values through the bridge; a
      longstanding feature gap, allowed-failing).
    - C: `TestImmediateJSON` (renamed from `TestImmJSON`) — `toNaturalGo` now projects starlark dicts to
      `map[string]any` (string keys via `starlark.AsString`); `encoding/json` rejects any `interface{}`-keyed
      map, so even `{"k":"v"}` failed before.
    - D: `TestImmUI` — fixture called `ui.success`; the method is `Succeed` → `ui.succeed`.
    - E: all other `Imm*` — `TestContext.Check` treated a nil graph as a `unit_count` failure; a nil graph
      (immediate-mode, no `plan.assemble`) now means zero units, so `expect_unit_count(0)` passes.
  - **Row 3 (`cmd/devlore-test` CLI: `TestCLI_GraphOnly` / `TestCLI_RoutToFiles`)** green — the "graph"
    output channel (the execution result = the final unit's return value) had no producer: structural
    `Subgraph.Execute` returned `nil` instead of its last child's result, and `t.run` discarded the result.
    Fixed both — `subgraph.go` propagates the last child's result; `starRun` emits the result to the
    runner's writer (threaded into `TestContext`).
  - **`TestGatherAdvanced` flake** fixed — A4 had two parallel iterations modifying the same constant path;
    rewritten to obey the gather concurrency contract (unique items; one file producer per unique `item`,
    a second non-file child for multi-child coverage). Verified green and race-free under `make test-race`.
  - **Remaining:** Row 4 (`cmd/star/star`: `TestLintCopyright_*`, `TestSourceFile_StarlarkIntegration`);
    Bucket 4 (lore) and Bucket 5 (writ) still `[build failed]`; `TestWalkTreePlanned` (step 24, allowed).
- Feeds the step-23 phase-8 PR gate (full `make test` green is non-negotiable there).
