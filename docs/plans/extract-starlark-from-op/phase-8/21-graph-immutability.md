---
title: "Step 21 — Graph immutability: consumer & test migration after the seal"
parent: "docs/plans/extract-starlark-from-op/phase-8.md"
issue: 275
status: in-progress
created: 2026-05-27
updated: 2026-05-27
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
