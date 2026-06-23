---
title: "Phase 8: Plan-time scope and grouping combinators"
parent: "docs/plans/extract-starlark-from-op.md"
issue: 275
status: in-progress
created: 2026-04-17
updated: 2026-06-18
---

> **Post-refactor commitment — intensive testing.** When this refactor lands, the next dedicated effort is testing as
> **foreground work**, not threaded around features (the pattern that has kept "figure out testing" perpetually
> deferred). Sequenced: (1) a **coverage map** across `pkg/op` — executor, providers, catalog, compensation,
> signing — so the gaps are *visible* instead of found by accident; (2) a **testing-strategy doc** — what we test at
> which layer (provider unit / executor integration / end-to-end graph runs) and the conventions; (3) **close the
> load-bearing gaps** the map exposes, deliberately. Explicitly after the refactor; explicitly first-class. The
> `EncryptFile` work, the `pkg/root` extraction ([`phase-8/root-extraction.md`](phase-8/root-extraction.md)), the
> `pkg/signing` implementation (KMS data-layer signing of graphs + traces —
> [`phase-8/graph-signing.md`](phase-8/graph-signing.md)), and the cmd/ consumer migration are the remaining refactor
> items before this phase begins.

## Implementation status

Every step below is a commit unit — one step, one checkpoint commit on
`refactor/extract-starlark-from-op.phase-8`.

| # | Step | Status | Notes |
|---|---|---|---|
| 1 | Invocation registry + options types + plan.options builder | complete (tests landed 2026-06-18) | Code present: `op.Invocation` (`pkg/op/invocation.go:18`) + `op.InvocationRegistry` (`pkg/op/invocation_registry.go:18` — `All` / `ByLabel` / `AutoLabel` / `Register` / `Reset`), in `pkg/op` (not `starlarkbridge` as drafted); the `Options` / `plan.options` builder was removed (no `Options` type; not announced). **Tests landed (2026-06-18)** — `pkg/op/invocation_registry_test.go` (9 tests: `New_IsEmpty`; `Register` creation-order / label-index / duplicate-reject-without-mutation; `ByLabel` unknown→nil; `AutoLabel` per-`provider.method` monotonic increment; `All` independent-copy; `Reset` clears entries + counters + frees labels; `Concurrent_IsRaceFree`) + `pkg/op/invocation_test.go` (`Binding_DelegatesToResultPromise` — delegates to `Result.Binding`, asserting the `PromiseBinding{UnitRef}`). `make test` and `make test-race` green for `pkg/op` (0 data races); registry ordering/labeling/dedup/reset and Binding delegation are now verified. |
| 2 | `+devlore:root=true` directive & ProviderRole placement zone | complete | Per D12. `ProviderRole` is partitioned into dispatch zone (bits 0–7: `RoleModule`, `RoleAction`) and placement zone (bits 8–15: `RoleRoot`) with zone masks and `Dispatch()` / `Placement()` accessors. `AnnounceProvider` validates that at least one dispatch-zone bit is set. `ReceiverRegistry` gains `RootProviders() []ProviderReceiverType`. Codegen parses `+devlore:root=true` on the provider struct and threads it through to the generated `AnnounceProvider` call as `|op.RoleRoot`. `filter_ctx_param` added in `generate.star` to strip a leading `context.Context` from announced parameter lists. Test template `receiver_type.gen_test.go.template` updated from `rt.ReceiverName()` to `rt.Name()`. |
| 3 | Reserved-kwarg enforcement at method registration | complete | `newReceiverType` rejects any provider method parameter list declaring `options`, `args` (without `*` prefix), or `kwargs` (without `**` prefix) as plain names. The `*args` and `**kwargs` variadic markers remain valid. Errors name the provider, method, and offending parameter — per-parameter message from `reservedParameterError` (`receiver_type.go:449`), wrapped with `provider <name> method <m>:` at the call site (`:357`); proven by `TestNewReceiverType_RejectsReservedParameterNames_NamesProviderMethodParam`. Table-driven tests cover plain / optional / variadic-decorated forms, the variadic markers, and ordinary names. |
| 4 | flow.Provider declares `+devlore:root=true` | complete | Directive added to `pkg/op/provider/flow/provider.go` with an updated doc comment explaining the root semantics. Regenerated `pkg/op/provider/flow/gen/provider.gen.go`; roles expression is now `op.RoleAction\|op.RoleRoot`. Verified at runtime: `registry.RootProviders()` returns `flow` with `roles=0x102`, `dispatch=0x2` (RoleAction), `placement=0x100` (RoleRoot). No consumer wired yet — plumbing activation only. |
| 5 | plan.Provider discovers fsroot-promoted methods; three-tier Attr with collision detection | complete | `plan.Provider` carries `promotedBuiltins map[string]starlark.Value` (Tier 2, write-once — `plan/provider.go:57`) and `rootNames map[string]struct{}` (excludes fsroot-placed providers from Tier 1 — `provider.go:54`); Tier 1 sub-namespace adapters are `*adapter` (`plan/adapter.go`), lazily minted. `NewProvider` calls `buildPromotedBuiltins` (`provider.go:86`), iterating `op.ReceiverRegistry.RootProviders()` (RoleAction+RoleRoot) and storing each promoted method as a builtin under its snake name. Collision detection panics at construction across the three tiers (promoted method vs. own method vs. sub-namespace name), naming both offenders. `ResolveAttr` walks Tier 2 (promoted) → Tier 1 (adapters) → Tier 3 (own); fsroot-placed providers are excluded from Tier 1 so `plan.flow` returns nil. Current resolution (verified 2026-06-15): `plan.choose` / `plan.gather` / `plan.wait_until` / `plan.complete` / `plan.degraded` / `plan.failed` / `plan.subgraph` → promoted builtins; `plan.file` / `plan.git` → `*adapter`; `plan.flow` → nil. (`starlarkbridge.NodeBuilder` and `peerBuiltins` / `buildPeerBuiltins` from the original landing were since removed/renamed; `plan.elevate` was removed with flow elevation.) |
| 6 | StarlarkRuntime access×root registration branches | complete | `NewStarlarkRuntime`'s module-iteration loop now explicitly branches on access × root per D12. `dispatch.&RoleModule == 0` (planned-only providers, root or non-root) → skip entirely; their methods surface via plan.* dispatch (Tier 2 for root, Tier 3 for non-root). `RoleModule + !root` → register as top-level global under `prt.Name()` (status quo for plan, ui, template, file/json/yaml/regexp/platform's module side). `RoleModule + root` → iterate the provider's methods and install each as its own top-level predeclared entry via `receiver.Attr(snake)`; collision against an existing predeclared panics. Reserved for future use; no Phase 8 provider claims this row. Smoke-verified: plan → "plan" global, flow → not registered, file/template → "file"/"template" globals for module side, git → not registered, ui → "ui" global. |
| 7 | NodeBuilder detaches from Graph | complete | Aligned dispatch with D5's detached-invocation model. `NodeBuilder` dropped its `graph *op.Graph` field and gained `ctx *op.ExecutionContext` + `catalog *op.ResourceCatalog`; new signature `NewNodeBuilder(rt, ctx, catalog, registry)`. `dispatch` no longer calls `graph.AddNode` — the node lives only on the returned `*Invocation` until plan.run (step 13) walks the reachable set and materializes a fresh `op.Graph`. `fillSlot` (list-of-promises branch and *receiver branch) stopped appending to `graph.Root.Edges`; the `PromiseValue{NodeRef, Slot}` in the consumer's slot already names the producer, and the Resource's `originID` (extractable via `op.ExtractResource`) names the resource-edge producer. `Promise` dropped its `graph` field, its `Graph()` accessor, and its `DependOn` method (unused); `NewPromise(node, slot)` has no graph argument. `Promise.FillSlot` now only sets the slot PromiseValue, no edge append. `shadowPendingOutput` uses `p.ctx` + `p.catalog` directly; `assignTarget` uses `p.ctx`; `linkResource` uses `p.catalog`. `plan.Provider` dropped `Graph *op.Graph` and gained `Catalog *op.ResourceCatalog`; `NewProvider` no longer calls `op.NewGraph`. Test template updated to construct `(ctx, catalog, registry)` instead of `(graph, registry)`; all 14 `*/gen/node_builder.gen_test.go` regenerated. |
| 8 | Target-type slot-fill dispatch (in the Planner machinery) + `catalog.Link` | complete | Per phase-8 D2. **The dispatch moved out of the now-removed `NodeBuilder` into the `op.Planner` machinery** (`pkg/op/planner.go`): `executableUnitType = reflect.TypeFor[op.ExecutableUnit]()` (`:17`) is the cached type, and `executableUnitType.AssignableTo(param.Type)` (`:277`) selects the structural-unit-reference branch — a param that accepts `op.ExecutableUnit` (e.g. `plan.subgraph`'s children) carries the unit itself, not a value-side `PromiseValue`. Value-typed params get a `PromiseValue`; the plan-side projection `projectToSlotValue` (`pkg/op/provider/plan/helpers.go:159`) maps `*op.Invocation`/`*op.Promise` → `PromiseValue`, `*op.Variable` → `VariableValue`, else `ImmediateValue`. **Refactor (survives):** `op.ResourceCatalog` gained `Link(resource Resource) Resource` (`pkg/op/resource_catalog.go:314`, used `:220`) — a convenience over `Resolve` returning the canonical linked entry; the deleted `NodeBuilder.linkResource` collapsed into that inline `catalog.Link(...)` call site. Container combinators take `op.ExecutableUnit` parameters and consume the unit references; value-typed parameters keep their `PromiseValue` behavior. (`NodeBuilder.fillSlot`, where this originally lived, is the dead vehicle.) |
| 9 | plan.subgraph primitive | complete | Added `Subgraph(children ...op.ExecutableUnit) []any` method to `pkg/op/provider/flow/provider.go`. Codegen picks it up; the regenerated announce map includes `"Subgraph": {"*children"}`. Surfaces in starlark as `plan.subgraph(...)` because flow is `RoleAction|RoleRoot`; action name `subgraph` (bare per D7). The variadic `op.ExecutableUnit` parameter triggers step 8's target-type dispatch — each child invocation's slot value is `ImmediateValue{inv.Target}` (structural reference, not a value-side promise). Return type `[]any` matches D3's container-output shape. The method body returns a length-`len(children)` slice of nils — the structural materialization (turning the Subgraph invocation into an `op.Subgraph` in the executable graph) is step 13's plan.run job, not this method's. Smoke-verified: `plan.Provider.ResolveAttr("subgraph")` now returns a `*starlark.Builtin` (previously nil). |
| 10 | plan.choose — value-picker superseded; subgraph-probe goal defined, unbuilt | superseded; goal defined | Initial source landed: `flow.Provider.Case{When any, Then any}` pure data; `flow.Provider.Choose(defaultValue any, cases ...Case) (any, op.Complement, error)` compensable signature with `CompensateChoose` stub; `flow/helpers.go` `isTruthy`; `plan.Provider.Case(when, then) *flow.Case` constructor. Source never got a standalone commit — it rode in with the phase-8 WIP checkpoint (`f1ed104`). **Superseded in review**: (a) side-effecting Whens execute regardless of case selection because evaluating a When *is* running it; (b) per-method compensation doesn't model per-branch activation; (c) control-flow semantics (short-circuit, per-iteration scope, polling) belong in graph topology rather than as one-off method bodies. **Successor goal now defined** (step doc [phase-8/steps/10-plan-choose.md](phase-8/steps/10-plan-choose.md)): `plan.choose` constructs a subgraph from `when`/`then` case clauses; at execution the `when`s are probed sequentially, the first truthy one stops the probe, its `then` executes and becomes the return value, and the choose ends — no later `when` evaluated, no non-matching `then` run; `flow.Provider.Choose` becomes a thin executor of that subgraph. **Current state ≠ goal:** the implementation is a value-picker — `ChoosePlanner` stores cases as inert `ImmediateValue` data (no case-branch children), so `when`/`then` producers run only when rooted separately (eager, no short-circuit) and `flow.Choose` iterate-and-picks (`provider.go:106`). 13 tests pass against the value-picker; `TestChoose_UnchosenInvocationBranchDoesNotRun` (the goal's proof) is unwritten and unpassable under the current design. The previously-drafted 13b.1/13b.2/13c/13d recast (PlanM prefix + subgraph-kind executor + conditional-edge topology) is abandoned. |
| 11 | plan.gather concurrent-iteration combinator | complete (failure-unwind proven via public-API test 2026-06-18) | `GatherPlanner` (`flow/planners.go:115`) materializes a `*op.Subgraph` bound to `flow.Gather`, adopting `body=` invocations as iteration-template children via `addBodyChildren` — the subgraph-child pattern `plan.choose` (step 10) still lacks. `flow.Provider.Gather` (`provider.go:181`) dispatches that one materialized body subgraph **once per item, concurrently**: per-dispatch frame minting (`buildIterationFrame(activation.Variables, item)` — fresh `item` binding, body not duplicated), bounded concurrency (`limit`-semaphore, `limit<=0`→`Platform.DefaultConcurrency()`), result collection by index, first-error `gatherCtx` cancel + LIFO unwind of completed iterations, per-iteration recovery nested into one returned stack (`CompensateGather`, `:299`). **The "redesign" landed** (Phase 5 of 13.0(n), ForEach-Object shape) and does not wait on choose. **Proven:** 7 tests — `test_gather_{basic,concurrency,advanced}.star` (~16 sub-scenarios: per-item dispatch, item+outer-flag frame visibility, frame hygiene, all limit modes, empty-items/empty-body, multi-node body, sequencing) + 2 Go compensate no-op guards + 2 gen (DryRun, CompensableInterface). **Failure-unwind proven (2026-06-18):** `TestGatherFailureUnwind_ViaPublicAPI` (`pkg/op/provider/plan/gather_api_test.go`) drives a gather through the **public consumer API** (`plan.Provider.Plan` → `Assemble` → `Spec` → `Run`, the path writ/lore use) — body `file.write_text` per item with the last item's parent a regular file so its write fails mid-flight; asserts the run errors at that write (dispatch proof) and the completed iterations' files are compensated away LIFO (unwind proof). `make test` green. Step doc: [phase-8/steps/11-plan-gather.md](phase-8/steps/11-plan-gather.md). |
| 12 | plan.wait_until — predicate-container subgraph re-evaluated each poll | incomplete; lambda-polling impl, 0 behavioral tests, container goal unmet | **Goal defined** (design §Goal lines 235/266/296/337/724): `plan.wait_until(predicate=<invocation>, timeout, interval)` — the predicate is a container subgraph re-evaluated (re-dispatched) each poll until truthy (return its final value) or timeout (error); missing predicate fails at plan time. **Current ≠ goal:** `flow.Provider.WaitUntil` (`provider.go:429`) is a correct polling loop but takes `predicate func(any)(bool,error)` — a lambda re-invoked each tick — plus a separate `target`; `WaitUntilPlanner` (`planners.go:364`) projects kwargs to **slots**, not adopting the predicate as a re-dispatchable child (no gather-style `addBodyChildren`), so an invocation predicate would project to a single `PromiseValue` that the slot model can't re-evaluate. **Unproven:** only `TestWaitUntilAction_DryRun` (gen) exists; no Go test exercises the loop's five branches (immediate-match / match-after-ticks / timeout / cancel / predicate-error) and no `.star` calls `wait_until` at all. Step doc: [phase-8/steps/12-plan-wait-until.md](phase-8/steps/12-plan-wait-until.md). |
| 13 | plan.assemble / plan.spec / plan.run / plan.save / plan.load — assemble-spec-run split | partial — Assemble + Save/Load proven (2026-06-18); Run/Spec still untested | Landed as the **assemble / spec / run split** — the design evolved from the original "`plan.run(...)` materializes from invocations and owns preflight" shape. `plan.Provider.Run(graph, spec)` (`pkg/op/provider/plan/provider.go:456`) executes via `op.NewGraphExecutor(graph, spec).Run(...)`; materialization of the graph from the reachable invocation set moved to `plan.assemble` (`provider.go:147`, step 17); the spec factory is `plan.spec` (`provider.go:403`). `plan.Provider.Load(path)` (`provider.go:295`) rehydrates via `op.LoadGraph` + `op.ValidateGraph`; `plan.Provider.Save(graph, path)` (`provider.go:335`) serializes JSON/YAML via `graph.Serialize` — both immediate, no graph node, as originally specified. All announced (`plan/gen/provider.gen.go:24,27,28`). **Pre-flight is not on `plan.run`:** plan-time orphan (step 17) + type-check (step 19) live in `Assemble`; runtime `(*ResourceCatalog).ResolvePending()` runs in `GraphExecutor.Run` (`executor.go:80`, per 13.0(k) k.15). Platform verification ([#282](https://github.com/NobleFactor/devlore-cli/issues/282)) is the one remaining preflight item from this step's original scope. **Proof state (2026-06-16):** only `Assemble` is exercised — `cmd/writ` adopt/migrate (`adopt/plan.go:85`, `migrate/plan_builder.go:126`, `migrate/file_ops.go:185`) + 53 `.star` files. `Run`, `Spec`, `Save`, `Load` are announced/callable (`gen/provider.gen.go:24/27/28/29`) but have **zero callers (Go or `.star`) and zero tests**: `t.run` reimplements execution via `op.NewGraphExecutor(...).Run` + `tc.buildSpec()` (`test_context.go:715`), bypassing `plan.Run`/`plan.Spec`; `plan.save`/`plan.load` appear only as commented-out placeholders in the unregistered `test_round_trip_writ_adopt.star`. **Save/Load proven + binding-serialization envelope (2026-06-18):** `TestGraphSaveLoadExecuteTrace_ViaPublicAPI` (`pkg/op/provider/plan/lifecycle_api_test.go`) drives the full lifecycle through the public Go API — `Plan` → `Assemble` → `Save`(JSON) → `Load` → execute the *loaded* graph via `op.NewGraphExecutor` → `executor.Trace()` → `document.Write` — asserting checksum identity across save↔load and a terminal `RunStateCompleted`. Landing it required fixing slot serialization: `op.Binding` is a sealed interface a JSON/YAML decoder cannot target, so `Load` previously failed with `cannot unmarshal object into op.Binding`. `nodeData.Slots` now serializes through the kind-discriminated `bindingData` envelope (`pkg/op/node.go` — one of `immediate`/`promise`/`variable`), wrapped by `marshalBindings` and restored by `assembleBindings`. **Concurrent taxonomy rename:** the sealed slot-value family `SlotValue`/`ImmediateValue`/`PromiseValue`/`VariableValue` → `Binding`/`ImmediateBinding`/`PromiseBinding`/`VariableBinding` (the interface head-noun names what a slot holds — a binding); and the vestigial single-output selector removed (`PromiseBinding.Slot`, `Promise.slot`, `Promise.Slot()`, `NewPromise`'s `slot` param) — a unit returns exactly one output. **This rename supersedes every earlier `SlotValue`/`ImmediateValue`/`PromiseValue`/`VariableValue` mention in this document; those rows are historical proof-state records, not retroactively renamed.** **Binding encapsulation + green core (2026-06-19):** the binding family was further encapsulated — the three variants share an embedded `binding{value any}` base whose `value` is unexported and built only through constructors (`op.NewImmediateBinding` / `op.NewPromiseBinding` / `op.NewVariableBinding`); `ProducerID() string` was promoted onto the `Binding` interface (replacing the `ProducerIDOf` free function), then — 2026-06-20 — superseded by `Edge(consumer string) *Edge`: a binding now returns the producer→consumer dependency edge it induces, or nil when it induces none. Only `PromiseBinding` yields an edge (its producer is a unit in the graph); an immediate value has no producing unit and a variable is injected from the `RuntimeEnvironment`, so both return nil — and neither peeks at a carried `Resource` (a resource's producer comes from its own stamp, discovered wherever it flows, not from the binding). The `*Edge`/nil return makes "no edge" first-class instead of the uninterpretable empty producer-id string. `Subgraph.materializeEdges` (`subgraph.go`) and the plan-time promise type-check (`validate.go`) consume it. **Consequence:** `materializeEdges` now derives promise edges only; resource→consumer edges (immediate resources + variable / activation-record resources) are no longer attempted there — the prior immediate-only peek is gone — and remain a pending follow-on, to be rebuilt from each resource's stamped producer wherever it flows. The `bindingData` envelope was reworked to carry each value through its own **exported** proxy fields (`immediateData.Value` / `promiseData.UnitID` / `variableData.Name`; document keys `value`/`unit_id`/`name`), filled from the unexported `value` inside package `op` — the prior envelope pointed at the binding structs, whose unexported field made `yaml`'s `reflect.Value.Interface` panic in `Graph.CanonicalContent` on *every* graph construction. Two dead types removed: the vestigial `Promise` (`Invocation.Binding()` now mints `NewPromiseBinding(i.Target.ID())` directly) and `preflight.go`/`preflight_test.go` (dead conflict-detection that hardcoded file-provider + writ-deploy semantics into op). The constructor migration (~70 sites via `gofmt -r`) + these fixes **green `pkg/op` and every op provider** (`make test`: 84 ok); `cmd/writ` (`op.ImmediateOf` + `execution.*` drift, step 30) and `cmd/lore`/`cmd/docgen` (lore migration) remain red on separate concerns. Still open under Save/Load: the Starlark `plan.save`/`plan.load` variant and the pause/resume (`op.ResumeExecutor`) variant. Step doc: [phase-8/steps/13-plan-run-load-save.md](phase-8/steps/13-plan-run-load-save.md). |
| 14 | Orphan detection at plan-end | incomplete — mechanism present; detection path untested | The "walk from root, mark reachable" intent lands as the inverse check at `plan.Provider.Assemble`:204–215 (`pkg/op/provider/plan/provider.go`): the assembled-graph walk iterates the session's invocation registry, flags any invocation whose `Target.ParentID()` is empty as an orphan, and aggregates the orphan errors via `errors.Join`. The equivalence holds because every `AddChild` call stamps a parent ID on the added unit — so "no parent ID" is exactly "no structural reachability from any `AddChild` root." Aggregation feeds into the same plan-time error envelope `Assemble` returns to the caller. Step 13's `plan.run` reuses that envelope verbatim by running against an already-assembled graph (`Assemble`'s output) rather than re-validating at execute time. Future refinement (transitive reachability through slot promises and resource origin IDs) is the spec's deeper aspiration; the parent-ID check covers the "forgot to compose" failure mode the spec was written to catch. **Proof state (2026-06-16):** the **no-orphan** path runs incidentally in all 53 `.star` fixtures, but the **detection path is untested** — `go test -run Orphan ./pkg/op/...` finds nothing (the only `Orphan` tests are the unrelated reconcile deleted-symlink `StateOrphan`), and no `.star` fixture constructs an unattached invocation to assert the `"orphan invocation … has no parent"` error (the `expect_error` fixtures cover type-mismatch / variadic / fatal / copy only). The harness already supports the missing test (`runner.go:337` plan-validation-only + `t.expect_error`). Step doc: [phase-8/steps/14-orphan-detection.md](phase-8/steps/14-orphan-detection.md). |
| 15 | Convertibility infrastructure — SourceConverter/TargetConverter + op.Convert + typesAreInterconvertible (D8/D9) | complete (2026-05-24); engine proven, symmetric probe transitive-only | The original D9 spec proposed a single `op.Converter` interface with `Convert(target)` + `CanConvert(target)`. Phase 6.0 refined the design into two opt-in interfaces — [op.SourceConverter] (value-side: `CanConvertTo(target)` + `ConvertTo(target)`) and [op.TargetConverter] (target-side: `CanConvertFrom(source)` + `ConvertFrom(value)`) — at `pkg/op/interfaces.go`. Both probes satisfy the original spec's cheap-probe contract (safe on zero-value receivers; pure function of the type pair). D8's `plan.Provider.CanConvertTypes(source, target)` lands as the framework-level [op.typesAreInterconvertible](../../../pkg/op/convert.go) helper, which probes identity, assignability, [op.SourceConverter] (in both directions), and [op.TargetConverter] (in both directions). The plan-doc D8 / D9 sections retain their original wording for traceability; the actual implementation supersedes them with a more compositional design — every Resource type opts into the conversions it supports, and the framework wires both plan-time and dispatch-time uniformly without each provider repeating the cascade. **Proof state (2026-06-16):** `op.Convert` is directly proven — `TestConvert_*` ×8 (identity, assignability, slice, map, source-converter, target-converter, resource-constructor ×2) all pass; 2 of 9 `CanConvert*` opt-ins (envValue, mem) have direct probe tests. **Title correction:** `plan.Provider.CanConvertTypes` **does not exist** — the D8 capability is the unexported `op.typesAreInterconvertible` (`convert.go:355`), which has **no direct test**: its symmetric `a↔b` relation is exercised only transitively at the **bubble-up** call site (`subgraph.go:685`: true branch via the 53 valid `.star` fixtures, false branch via `TestValidateGraph_TypeCollision`); its **other** call site `checkPromiseTypes` (`validate.go:234`) is untested (step 16). (`test_writ_adopt_type_mismatch.star`'s "not assignable to declared type" is `helpers.go:122`, not this probe.) Follow-up: a direct `TestTypesAreInterconvertible`. Step doc: [phase-8/steps/15-convertibility.md](phase-8/steps/15-convertibility.md). |
| 16 | Topological sort + plan-time type-check pass | partial — checkRequiredParams/checkBubbleUpConsistency tested; checkPromiseTypes untested | The plan-end type-check walk lives at [op.checkPromiseTypes](../../../pkg/op/validate.go); [op.ValidateGraph]'s `checkRequiredParams` + `checkBubbleUpConsistency` + `checkPromiseTypes` aggregate into the single envelope `plan.Provider.Assemble` returns. For every slot bound to a [op.PromiseValue], the walk looks up the producing unit by [op.PromiseValue.UnitRef], derives its declared output type via [op.Method.ResultType], compares to the consumer slot's [op.Parameter.Type] (looked up via [op.Method.ParameterByName]), and consults [op.typesAreInterconvertible] for the decision — the same convertibility relation [op.Convert] enforces at slot-fill time, so plan-time and dispatch-time agree on the contract. Topological sort isn't strictly necessary for static type-check: each Promise binding is independent (producer's type comes from the bound method's signature, not from execution state), so visit order doesn't affect the outcome. The plan-doc's "topological sort" language was a clarity hint, not a load-bearing constraint; the implementation walks units in map-iteration order without sacrificing correctness. Mismatches surface as joined errors alongside orphan-detection and bubble-up violations per D5. **Proof state (2026-06-16):** `checkRequiredParams` (6 `TestValidateGraph_*`) and `checkBubbleUpConsistency` (`TestValidateGraph_TypeCollision`, `MultipleViolations_AllJoined`) are tested. **`checkPromiseTypes` — the headline Promise→slot type-check — has zero tests:** no `validate_test.go` builds an incompatible `PromiseValue`→slot binding, and the `"cannot bind … output"` violation (`validate.go:235`) is asserted by nothing. `test_writ_adopt_type_mismatch.star` ("not assignable to declared type") hits `helpers.go:122` (value-side slot-fill), **not** `checkPromiseTypes`, so the `validate.go:234` `typesAreInterconvertible` call is unexercised — which is also where step 15's symmetric-vs-directional question goes untested. `topologicallySorted` (`helpers.go:250`) orders execution (`subgraph.go:602/789`), not the type-check; no direct test, transitively exercised by the 53 `.star` runs. Step doc: [phase-8/steps/16-toposort-typecheck.md](phase-8/steps/16-toposort-typecheck.md). |
| 17 | Migration of existing .star callers | complete (caller migration verified 2026-06-16; 3.2 phantom-API defect corrected 2026-06-17) | `.star` files: zero hits for old `plan.choose(when=..., then=...)` kwargs form or `plan.flow.<method>` sub-namespace form — the caller migration was carried in incrementally as part of Phase 5's root-promotion + plan.case introduction. User-facing doc snippets sweep: `docs/architecture/3-operation-namespaces.md` (line 362–363 example), `docs/architecture/3.2-projected-provider-api.md` (the "Gather — Parallel Fan-Out" subsection re-anchored to the iteration-combinator semantics with `body=[...]` + `plan.iter.item` per-frame binding), `docs/architecture/4-resource-management.md` (the planned `plan.choose(distro, {dict-of-cases})` example), and `docs/guides/lore/create-manifests.md` (the graph-primitives table row + the "Conditional logic at execution time" example) updated to current API. Out of scope: `docs/architecture/2.3-orchestration-primitives.md`'s "SlotProxy" subsection — that section describes the pre-13.0(n) struct-based `SlotValue` model with `GatherRef`/`FieldRef`/proxy-lambda semantics, which 13.0(n) replaced with the sealed-three interface model + per-dispatch frame minting; it's an architectural rewrite, not a caller-migration concern. Historical plan-docs (`extract-starlark-from-op.md`, `terminal-flow-control/*.md`, `binding-unification.md`, `mem-resource.md`, `phase-8.md` design-decision sections, `phase-8/13.0-n.md`) retain old patterns intentionally as historical records of what the design was migrating away from. Per D11. **Proof state (2026-06-16):** the caller migration is verifiably complete — old `plan.choose(when=,then=)` direct-kwargs and `plan.flow.<method>` forms are absent (0 `.star` hits; the 7 `plan.choose(.*when=` matches are the current nested `plan.case(...)` form). **Doc-sweep defect:** the `plan.iter.item` cited above for `3.2-projected-provider-api.md` (lines 91, 95) is a **phantom API** — `plan.iter` resolves to nothing; the real per-frame binding is `plan.variable("item")` (bound by `buildIterationFrame`, `flow/helpers.go:66`; documented at `flow/planners.go:80`), used by all 3 gather fixtures. **Resolved 2026-06-17 (user decision — "the code is king"):** `plan.variable("item")` is canonical; `3.2-projected-provider-api.md` corrected — phantom `plan.iter.item` → `plan.variable("item")` (lines 91/95), phantom "Proxy" slot row → "Variable" (line 75, the sealed-three), phantom `plan.depends_on` barrier → consume the gather's returned handle (Promise edge). Step doc: [phase-8/steps/17-star-caller-migration.md](phase-8/steps/17-star-caller-migration.md). |
| 18 | Resolve all test failures | in-progress — exit gate UNMET (re-measured 2026-06-17, step doc [phase-8/steps/18-resolve-test-failures.md](phase-8/steps/18-resolve-test-failures.md)); framework half green, consumer/test migration open, sub-plans [21-graph-immutability.md](phase-8/21-graph-immutability.md) + [21-lore-migration.md](phase-8/21-lore-migration.md) | **Exit criteria (relabeled 2026-06-15, was "Test triage — pre-existing failures"): 100% test pass on existing code; all four apps compile and run.** **Problem statement (graph immutability + restartability):** Graph is immutable — a re-executable plan. RuntimeEnvironment is mutable, scoped to one execution; owns every per-run mutation. Binding direction is env → graph; graph never references env. Graph must be restartable: graph + prior receipt suffices to hydrate a fresh RuntimeEnvironment and continue execution. Receipt (today `*op.RecoveryStack`) expands from compensation ledger into full execution-state envelope — per-dispatch entries already carry `Result()` (subsumes today's `RuntimeEnvironment.Results map[string]any`); resolved variable map joins (currently `executor.lastVariables`); pending units derivable (graph units minus successful-receipt UnitIDs); active activation frames need to surface (today goroutine-local in `ActivationRecord{Variables, Unit, Graph, Context, ...}`) for mid-combinator resume. **Mutable elements being removed from `*op.Graph`**: `State` (writes at `graph_executor.go:151,168,173,175`); `Rollback` (no writes found — likely already dead); `summary` (lazy populate in `Summary()` at `graph.go:419`); `Catalog` (Assemble-time transfer at `plan/provider.go:197`); `ctx` + `Rebind` + `Unbind` (writes at `graph.go:259,267`; method call sites at `graph_executor.go:147-148`, `planner.go:57`, `plan/provider.go:156`). **New homes**: `State` / `Rollback` / `summary` / `Catalog` → `RuntimeEnvironment` (the per-execution state-bearing type); `ctx` + `Rebind` / `Unbind` → removed; binding direction reverses so env holds a graph reference. Construction-time-only fields (`unitsByID`, `Timestamp`, `Checksum`, `Collisions`, `Provenance`, `Signature`, `Root`, `Version`) stay on `*op.Graph` — not per-execution mutations. **`RuntimeEnvironment.Results map[string]any` is also redundant**: every dispatch pushes a Receipt with `.Result()` per `recovery_stack.go`'s step-12 broadening; slot resolution walks the stack keyed on `Receipt.UnitID()` (O(1) with an index, O(n) without) instead of a separate map. These removals are the precondition for the 21.1 harness redesign — graph immutability is what makes `plan.run(graph, ctx.spec())` composable across re-runs without per-run contamination. **Re-measured 2026-06-17 (clean tree, after `runtime_environment.go` committed) — SUPERSEDES the 22-red inventory below.** Full attribution in step doc [phase-8/steps/18-resolve-test-failures.md](phase-8/steps/18-resolve-test-failures.md). `make test`: 83 `ok`, **10 red packages** = **7 build failures** + **3 packages / 4 test reds**. The 7 build failures (`cmd/writ`, `cmd/writ/writ`, `cmd/writ/writ/adopt`, `cmd/writ/writ/migrate`, `cmd/lore/lore`, `cmd/docgen`, `internal/e2e`) all name committed framework APIs — `op.Origin` is now an interface (`pkg/op/origin.go:16`); `ReceiverRegistry` is a process-wide `sync.OnceValue` function (`pkg/op/receiver_registry.go:139`), not a `RuntimeEnvironment` member; `NewRuntimeEnvironmentSpec(programName)` is single-arg; `ActionPlanner.Plan` arity changed; `op.Node` dropped `Origin` — i.e. the tracked sealed-Graph/RuntimeEnvironment consumer-migration gap (Buckets 4/5 of [21-graph-immutability.md](phase-8/21-graph-immutability.md) + [21-lore-migration.md](phase-8/21-lore-migration.md)), NOT live WIP. The 4 test reds: `TestBackup_DefaultSuffix` (`file` — `.devlore-backup` default relocated to `RuntimeEnvironment.BackupSuffix`; `testProvider` constructs the env without that defaulting); `TestCompensation` (`devloretest` — compensation did not unwind the prior write on downstream failure; old-harness fixture; needs diagnosis); `TestWalkTreePlanned` (`devloretest` — row 21 function-values gap, allowed); `TestShellCompletionPath/powershell` (`cmd/star/cli` — standalone `pwsh`-vs-`powershell` impl/test drift, untracked by any phase-8 row). **Two apps of four (`writ`, `lore`) do not compile**, so the exit gate is unmet. The clean-tree red set is identical to the prior mid-edit measurement, confirming committing `runtime_environment.go` changed nothing in the inventory. **The 2026-05-27 enumeration that follows is stale** — all of `TestImm*`, `TestLintCopyright_*`, `TestCLI_GraphOnly`/`TestCLI_RoutToFiles`, `TestSourceFile_StarlarkIntegration` are now green (0 `FAIL` occurrences; `cmd/star/star` passes), and sub-steps 21.2/21.3 below address reds that no longer exist; retained as a historical record of the 2026-05-27 state. **Refined inventory after a fresh `make test`:** 22 reds (not 19/20 as originally enumerated): `TestImm*` x 10 (plan-doc said 9 — `TestImmStarstats` was missed); `TestWalkTreePlanned` x 1; `TestCLI_GraphOnly` + `TestCLI_RoutToFiles` x 2; `TestLintCopyright_*` x 8 subtests; `TestSourceFile_StarlarkIntegration` x 1 (extra, not in original plan-doc enumeration). **Root-cause split** (only one of these is the originally-imagined "script-side drift"): (a) devloretest reds (11: TestImm* x 10 + TestWalkTreePlanned) trace to a deeper design issue in the test harness itself — the runner extracts the graph from a top-level starlark global `graph = plan.assemble([...])` (runner.go:307 / graphFromGlobals at runner.go:421), but Starlark doesn't have a natural "return value from a script" path; the convention forces scripts to hoist their graph into a magic global name. Imm-mode scripts that don't build a graph leave the global unset, the runner sees nil, and the harness fails `unit_count(0)` with `"no graph assembled (script did not assign \`graph = plan.assemble([...])\`)"` even though an empty graph would satisfy the assertion. (b) cmd/star/star reds (9: TestLintCopyright_* x 8 + TestSourceFile_StarlarkIntegration) are separate root causes — `builtin_function_or_method has no .lint field or method` (template-staleness for an API removed during Phase 7/8) and `package_name` returning a built-in function instead of a string. (c) cmd/devlore-test reds (2: TestCLI_GraphOnly + TestCLI_RoutToFiles) are subprocess CLI tests, diagnosis pending. **Sub-steps**: **21.1 — devloretest harness redesign.** Move from "script as top-level statement bag with `graph =` magic" to "script declares `def test(ctx): ...` entry point; harness calls it via [starlark.Call]; assertions are inline ctx-methods against the script's own local graph reference, not a queued-expectation drain". Eliminates the `globals["graph"]` lookup entirely — runner doesn't need the graph back at all; the script holds it and asserts against it directly. **Conventions settled with the user (2026-05-25 design session):** Entry-point function name `test`; signature `def test(ctx):` (parameter named `ctx`, function named `test` — chosen over the function/parameter shadow `def test(test):`). Bridge already exposes Graph's exported fields/methods via `goReceiver` (verified at `pkg/op/starlarkbridge/go_receiver.go:26-39, 800-836`); zero-arg methods still require parens at call site (verified at `goReceiver.Attr` provider.go:203-221 — no arity-based auto-invocation), so fixtures write `graph.unit_count()` not `graph.unit_count`. **TestContext surface delta** (existing logic preserved; rewiring + renaming): scaffolding `ctx.tmp` / `ctx.mkdir` / `ctx.write` keep current behavior. **Add `ctx.spec()`** — lift the existing internal `buildSpec` (test_context.go:606-634) to a published method returning `*op.RuntimeEnvironmentSpec` constructed from `tc.tmpDir` + `tc.sources`. **Drop `ctx.run(graph)`** — scripts call `plan.run(graph, ctx.spec())` instead. The execution surface stabilizes to three named primitives: `plan.assemble(invocations)` builds the graph, `ctx.spec()` (or `plan.spec(...)`) builds the spec, `plan.run(graph, spec)` executes. Two spec factories — `plan.spec(...)` clones the planning environment (production / general-purpose), `ctx.spec()` builds a test spec from tmpDir + BindingSources (test-only). **Assertion family renamed `expect_*` → `assert_*`** (Go testify convention; `expect_*` is Ruby/RSpec-flavored). **Initial assertion set (minimal, expand as fixtures demand)**: `ctx.assert_equal(actual, expected)` — arg order flipped from current `expect_equal(got, want)`; `ctx.assert_file(path, content=?)`; `ctx.assert_no_file(path)`; `ctx.assert_error(pattern)` — stays deferred because the script's eventual exception hasn't happened at call site. **Dropped from the surface entirely**: `expect_unit_count` (replaced inline by `ctx.assert_equal(graph.unit_count(), N)`); `expect_variable` and `expect_variable_namespace` (no dedicated assertion methods — the bridge already exposes `op.Variable`'s `Name` / `Value` / `Source.Kind` / `Source.Name` / `Source.String()` via `goReceiver`, so fixtures read the variable directly and use `ctx.assert_equal`, e.g. `v = <some path>.variable("dest_dir"); ctx.assert_equal(v.source.string(), "env:DEVLORE_TEST_DEST_DIR")`; `expect_variable_namespace` had zero non-comment call sites anyway). **Open wiring decision — variable retrieval path:** how the script gets the resolved variable map post-`plan.run` is not yet settled. Three candidates: (a) change `plan.run` (`pkg/op/provider/plan/provider.go:389`) to return `(value, variables, error)` and unpack tuple-style — affects one production caller (writ adopt rewire), all test fixtures; (b) wire via ctx — `ctx.spec()` returns a spec that captures variables into `tc.variables`, script reads with `ctx.variable("name")` — keeps `plan.run` signature, adds one ctx method; (c) expose on the graph post-execute — `graph.variables()` returns the resolved map — production-friendly, test does the same as production code. Pick before implementation lands. **Binding-source setters** `ctx.set_overrides` / `ctx.set_flags` / `ctx.set_env_prefix` / `ctx.set_env` / `ctx.set_config` keep current behavior (consumed by `ctx.spec()` internally). **Migration scope:** 69 .star fixtures rewrapped (`def test(ctx):` wrapper + assertion renames + parameter rename `t` → `ctx`); `runner.go` updated to call `globals["test"]` via [starlark.Call] instead of executing top-level + extracting `globals["graph"]` (drop `graphFromGlobals`); `test_context.go` rewires the four `assert_*` methods from queued-then-drained to inline-recorded (`assert_error` stays deferred). Migrating the harness fixes 11 of 22 reds. **21.2 — cmd/star/star template + integration reds.** Diagnose `lint-copyright.star:304` `.lint` resolution failure, and `TestSourceFile_StarlarkIntegration`'s `package_name` returning a built-in instead of evaluating. **21.3 — cmd/devlore-test CLI subprocess reds.** Diagnose `TestCLI_GraphOnly` and `TestCLI_RoutToFiles`. Also resolve the `starlarkbridge.NewProvider` / `ReceiverName` template staleness flagged during step 2 (module test template references APIs removed during Phase 7/8 refactoring). All 22 reds → green is non-negotiable for phase-8 PR per the step-23 gate. **Update (2026-05-27): framework half landed.** The graph-immutability seal is complete in `pkg/op` production code — `Graph` fully sealed (all-args `NewGraph` / `NewSubgraph`, getter-only access; `State` / `Summary` / `Catalog` / `ExecuteWithStack` / `Rebind` / `Unbind` removed; run state lives on the executor as `RunState` + `Trace`; the catalog is carried on the graph and cloned per-run; `Action.Do`'s second arg dropped). The predicted homes shifted — `State` went to the executor, not `RuntimeEnvironment`. Remaining work is the consumer / test / template migration — flow helpers, `pkg/op` tests, gen test templates, and the `lore` / `writ` apps — scoped and sequenced in the sub-plan: [phase-8/21-graph-immutability.md](phase-8/21-graph-immutability.md). **Progress (2026-05-27): the `pkg/op` layer is green** — flow helpers, Planner `errorAction`/`retryPolicy` threading, gen test templates, and the `pkg/op` tests are migrated (plus a revealed `validate.go` `Root()` nilfunc fix); `make vet` is clean and `pkg/op` tests pass. The `lore` / `writ` app migrations (Buckets 4 / 5) remain. **Scenario-1 exit gate (added 2026-06-02 per directive):** step 18 does **not** exit until `lore deploy docker` installs and verifies Docker on **both** macOS (`Darwin`) and Linux (`Linux.Debian`) end-to-end through the sealed-graph executor — see [demo-milestone.md](demo-milestone.md) Scenario 1 (criterion 5). This folds into step 18's exit, on top of the all-green `make test` gate: (a) the `lore` deploy path running on the sealed graph (Buckets 4/5) — `detectPlatform()` → `registry.Resolve(name, platform)` → `buildPackageNodes` → executor → receipt; (b) rewriting the stale docker registry scripts to the current API (`../devlore-registry/packages/docker/{Darwin,Linux.Debian}/Deploy/*`); (c) the planned primitives those scripts require — `platform.arch`, `plan.download(url, dest)`, planned `plan.file.remove`, `phase.env(...)`; (d) receipt verify status matching `lifecycle.yaml`'s `verification.pattern`. `plan.choose` (step 10) and the Starlark `plan.run` builtin (step 13) are explicitly **not** on this gate's path — platform adaptation is lore directory resolution and execution is Go-driven. |
| 19 | Resource foundation cleanup | in-progress | Prerequisite for step 13 and everything downstream that touches Resources. (a) Delete `<M>Planned` companions — code complete; doc closure folded into k.15. (b) Roll out 12 required Resource interfaces across all nine Resource-bearing providers — **complete:** `op.ResourceBase` shared impls plus per-type overrides on file/git/appnet/pkg/service/mem/function/json/yaml; k.12 boot-discipline test asserts no Resource type leaves Addressing at the default sentinel. (c) Catalog operations using the addressing/digest contract — **k.10 done** (Resolve cascade), **k.13 done** (lifecycle integration: Pending/Active/Gone state machine, catalog-owned transitions per Model A); **k.14 done** (audit-only — file-provider Compensate methods inspected method-by-method; no migration work remained), **k.15 done** (`(*ResourceCatalog).ResolvePending() []error` at `pkg/op/resource_catalog.go:437`; wired into `GraphExecutor.Run` preflight at `pkg/op/executor.go:80` with fail-fast `errors.Join`, skipped in dry-run; 8 tests at `pkg/op/resource_catalog_test.go:714–866`). **13.0(k) complete — all sub-items closed.** 13.0(n) writ graph executor is now the only not-started item under 13.0. Platform verification at preflight time, originally scoped into k.15, moved out of 13.0 — tracked as #282 under step 16's preflight scope. |
| 19(d) | Receipt JSON + YAML marshalers via `Snapshot` / `Restore` | complete | `MarshalJSON`/`UnmarshalJSON`/`MarshalYAML`/`UnmarshalYAML` on all five receipt types (archive, encryption, file, pkg, service). Wire shape: flat envelope `{action, resource_uri, transaction_id, ...provider-fields}`. Unmarshalers resolve `resource_uri` through catalog. |
| 19(e) | Saga shape and stack-based recovery | complete | Steps 1–5b complete (Action.FullName, RecoveryStack API, Method classifier + Invoke + pushComplement dispatcher, Tombstone→Receipt rename). Complement shape settled to `*op.RecoveryStack` as the singular legal form. Closure-only `Push`/`Do` APIs already deleted from `RecoveryStack` (ahead of plan). **File-provider bug list (k.14) closed by audit:** every call site in `pkg/op/provider/file` uses the current `*op.RecoveryStack` dispatcher form (`PushComplement`/`PushNested`/`Unwind`/`NewRecoveryStack`). All eleven Compensate methods (Backup, Copy, Link, Mkdir, Move, Remove, RemoveAll, Unlink, WalkTree, WriteBytes, WriteText) inspected against their paired forward method's receipt shape: shape-correct, no closure-API leftovers, no semantic regressions. `make build` / `make vet` clean; `make test` green for all `pkg/op/...` and provider packages. The original "orphaned during closure-only API deletion" concern was already addressed in earlier landings and never reflected in this row — k.14 was bookkeeping residue. Three pre-existing semantic items surfaced during the audit (not k.14 work, not k.13 regressions) and filed for separate triage; see the k.14 entry under 13.0(k). |
| 19(f) | Codegen extension for parameter defaults | complete | `name?=value` tokens in the existing parameter-name `[]string`. `parseDefaultExpression` handles bool/int/uint/float/string kinds + deferred `{{ }}` syntax. Resource-type defaults rejected upfront. No `AnnounceProvider` signature change. |
| 19(g) | Resource construction at the bridge boundary | complete | Landed: wrapper/Projector/`op.Convert` refactor; `op.SourceConverter`/`op.TargetConverter` split; producer rename (`originID`→`producerID`); catalog state model documented. Lifecycle integration items closed by 13.0(k) k.13 — Pending/Active/Gone state machine on `ResourceBase`, catalog-owned transitions via package-private `markActive`/`markGone`, Discover/GetOrCreate updated to apply transitions per the §6.2 behavior matrix. Provider deletion paths and executor Shadow gating subsumed by the "Gone is terminal; revive via shadow" rule — no separate executor changes needed. |
| 19(h) | Post-13.0(b) cleanup | complete | `wrapper`→`goReceiver` rename; `file.Receipt.recoveryID` field; stack-comparison docs refreshed; bare-UUID test sweep (9 sites); `ui.Provider` test pull. |
| 19(i) | Capability migration: `pkg/status` + `pkg/result` + `pkg/platform` | complete | `RuntimeEnvironment.Writer` removed; narrative output via `status.Narrator`; typed payloads via `result.Pipeline`; `pkg/process` bridges `os/exec`; `pkg/sink` byte-level abstraction; powershell split to own provider; codegen emits one gen file per Resource type. |
| 19(j) | Polymorphic `NewResource` for `mem` + `function` | complete | Both constructors accept `ResourceSpec` or `string` URI. `mem.SourcePath` field deleted; `SourcePath()` method derives path from typeID + `ReachabilityURI()`. `function.Resource` inherits via embedding. Unmarshalers rewritten to call `NewResource(env, uri)` directly. |
| 19(k) | Two-model resource design (location-based vs. CAS-based) + Digest/Etag/Addressing | complete | Supersedes 13.0(c). Location-based URI: `tag:..:<reach>#<go-type-id>` (file/git/appnet/pkg/service). CAS-based URI: `tag:..:<algo>:<hex>#<go-type-id>` (mem/json/yaml/function). New interfaces: `Digest()`, `Etag()`, `Addressing()` on `op.Resource`; defaults on `op.ResourceBase`; `AddressingMode` enum. **Done — all nine Resource-bearing providers have full Addressing/Digest coverage:** file/git/appnet/pkg overrides (k.1–k.5 plus pkg, ahead of plan); mem (`ResourceSpec` collapsed to `[]byte`/`io.Reader`/`string` dispatch, `SourcePath` sharded as `<algo>/<hex[0:2]>/<hex>`, full Addressing/Digest overrides — Etag inherits `ResourceBase`'s URI default which is correct for `AddressingContent`); function (CAS over synthesized-source digest via the embedded `mem.Resource`; identity migration + URI rewrite landed alongside the mem work; Addressing/Digest inherited via embed); service (full override surface — Digest hashes URI, Addressing=Location, Equal/Resolve/Etag + resource-side JSON/Text/YAML marshalers; `buildCandidate` now accepts bare names *or* canonical tag URIs so marshalers round-trip); json/yaml (k.8/k.9 — content canonicalization at construction via parse-and-remarshal through `encoding/json`, full Addressing/Digest overrides, URI hash widened from 12-char prefix to full 64-char to match `op.ParseDigest`'s strict sha256 contract; yaml.Resource is "alternative input rendering of json.Resource" — YAML inputs canonicalize through the same JSON path, so semantically-equal YAML and JSON content share the same Hash; YAML-native canonicalization through `*yaml.Node` deferred until typed-tag preservation becomes load-bearing; both types also accept `io.Reader` inputs, drained to bytes before canonicalization). **Done (k.10):** `ResourceCatalog.Resolve` now branches on `Addressing()`. AddressingContent cache hits skip Etag/Digest entirely (URI carries the digest; same URI ⟹ same content). AddressingLocation cache hits run the Etag-mismatch-then-Digest cascade outside the catalog mutex: input.Etag vs canonical.Etag; on mismatch, input.Digest vs canonical.Digest; on Digest mismatch, the canonical is still returned (Resolve preserves cached identity), with the drift to be surfaced by a future reconciliation pass (k.15). Cascade extracted into `verifyLocationFreshness`; lookup extracted into `lookupOrCatalog` so the I/O calls happen outside the lock. Four targeted tests covering content fast-path (no Etag/Digest calls), location Etag-match fast-path, location Etag-mismatch triggering Digest, and genuine drift preserving canonical. **Done (k.12):** boot-discipline test landed at `pkg/op/inventory/discipline_test.go`. Walks every announced Resource type via the receiver registry (exposed by new exported `op.SnapshotReceiverTypes()`); asserts `Addressing() != AddressingUnknown`. Test lives in the inventory package because it needs to blank-import every provider's gen package to populate the registry — pkg/op itself can't, providers depend on op. All nine Resource types pass. **Done (k.13):** Pending/Active/Gone state machine landed. New file `pkg/op/resource_state.go` (enum + `String()`); `state State` field on `ResourceBase` (unexported); `State() State` method on the `Resource` interface (read-only accessor — only catalog code writes via package-private `markActive`/`markGone` helpers). `Catalog.Discover` updated to call `r.Resolve()` and branch on cache state per the §6.2 matrix; `Catalog.GetOrCreate` updated to branch on `Addressing()` × state — content-addressable Pending/Active hits return existing (singleton); location-based hits shadow; Gone hits revive via shadow on both addressing types (Gone is terminal for the entry). Catalog-owned transitions enforced at the language level: the `state` field is package-private to `pkg/op` and providers have no setter. 14 new tests cover the matrix. **Done (k.14):** file-provider Compensate audit. Every call site in `pkg/op/provider/file` uses the current `*op.RecoveryStack` dispatcher form (`PushComplement`/`PushNested`/`Unwind`/`NewRecoveryStack`). All eleven Compensate methods (Backup, Copy, Link, Mkdir, Move, Remove, RemoveAll, Unlink, WalkTree, WriteBytes, WriteText) inspected line-by-line against their paired forward method's receipt shape: shape-correct, no closure-API leftovers. `make build` clean, `make vet` clean, `make test` green for all `pkg/op/...` and provider packages (only failures are in `cmd/devlore-test` / `cmd/star/star` — graph-executor harness, 13.0(n) scope). The migration work k.14 was nominally chartered for was absorbed silently into earlier 13.0(e) and 13.0(m) landings; this row was bookkeeping residue. Three pre-existing semantic items surfaced during the deep-dive audit and filed for separate triage (not k.14 work, not k.13 regressions): **(i)** asymmetry in recovery-archive digest verification — `CompensateMove` verifies the archive's bytes against the captured digest before restoring (`provider.go:362-385`); `CompensateUnlink` and `compensateWrite` do not. The asymmetry may be intentional (Move's archive holds displaced content with stakes; Unlink/Remove's archive holds content being discarded) but deserves a deliberate decision; **(ii)** dead re-self-assignment in `Copy` at `provider.go:107-109` — `_ = receipt.SetRecoveryID(receipt.RecoveryID())` sets the recovery ID to its current value (refactoring artifact); **(iii)** silent error swallow in `Move`'s rename-failure path at `provider.go:326-328` — the failure-recovery `RestoreFile` call uses `_ =` so a failed restore during a failed move is unobservable to the caller. **Done (k.15):** `(*ResourceCatalog).ResolvePending() []error` landed at `pkg/op/resource_catalog.go:437`. Catalog-owned sweep that drives every Pending entry to Active or Gone in a single call. Walks Pending entries in URI order (deterministic error output); for each, releases the catalog mutex and calls `r.Resolve()` (matching the I/O-outside-lock discipline of `lookupOrCatalog`/`verifyLocationFreshness`); on success applies `markActive`, on failure applies `markGone` and captures the error wrapping the URI. Returns an empty slice on full success, a per-failure `[]error` otherwise. Wired into `GraphExecutor.Run` preflight at `pkg/op/executor.go:80`: non-empty result transitions the graph to `StateFailed` and the executor aborts with `errors.Join(errs...)`; skipped under `RuntimeEnvironment.DryRun`. 8 tests at `pkg/op/resource_catalog_test.go:714–866` cover empty/all-active/all-gone no-ops, success/failure transitions, mixed catalog (only Pending touched), and deterministic URI ordering. Integration questions resolved: (i) wire-in site is `executor.go` directly — the `ResolveResources` free function and its `DiscoveryURIs()` feeder are gone; `pkg/op/preflight.go` retains only the file-conflict detection it already had; (ii) caller behavior on non-empty result is fail-fast aggregation as predicted. Reconciliation-as-side-effect on `Active` entries (the original k.15 framing) remains OUT OF SCOPE for 13.0 — tracked by #156. Platform verification at preflight time also remains out — tracked as #282 under step 16. **13.0(k) complete; 13.0(a–j) closed.** 13.0(a) doc closure (`<M>Planned` companion deletion narrative) rides independently as a doc-only tidy-up. |
| 19(l) | Remove `op.KnownAtExecution` sentinel | complete | Deleted `var KnownAtExecution`, `type knownAtExecution`, `func IsKnownAtExecution`. Sole call site in executor post-dispatch block removed alongside 13.0(m) m.1. |
| 19(m) | Move catalog lifecycle from executor to providers + catalog | complete | Providers self-intern via `Catalog.GetOrCreate` at create time (two-constructor pattern: `NewResource` for production, `DiscoverResource` for discovery). Executor post-dispatch block (`executor.go:333-379`) deleted. m.1–m.5 landed across file/git/appnet/json/yaml/mem/function/pkg/service. |
| 19(n) | Variable binding infrastructure | complete (2026-05-24) | Reframed from "create writ graph executor" to the broader binding-model overhaul that the original scope implied. **Slot model collapses to sealed three:** `ImmediateValue`, `PromiseValue`, `Variable` — `Variable` replaces both `EnvironmentValue` and the originally-proposed `ParameterValue` per the user's mental model (slot accepts Immediate, Variable, or Promise values). `Properties` interface and `EnvironmentValue` deleted outright in Phase 1; no migration window. `SlotValue.Resolve` signature changes to `(variables map[string]binding.Variable, results map[string]any) any`. `op.Parameter.Default *any` for required-vs-optional discrimination. `ExecutableUnit.Parameters()` bubble-up surface (node = method params with `Variable` slots; subgraph = deduplicated union of topological-root parameters). The binding model landed in `pkg/op` (not a separate `pkg/binding`): the `VariableSourceKind` enum (`variable.go:10` — Unknown < Default < Config < Env < Flag < Override, ascending by precedence), `Variable` (`variable.go:83`), and `VariableResolver` (`variable_resolver.go:35`) — functional-options construction, layered source precedence, origin tracking via the single internal variable map, no exported fields. Preflight pass `bindVariables` aggregating missing-required/type-mismatch/default-type-mismatch errors into the D5 envelope alongside `ResolvePending`. `RuntimeEnvironment.Data` and `RuntimeEnvironment.Property` retire in Phase 6. **Integration target: `writ adopt`** — three nil-activation call sites in `cmd/writ/writ/commands.go:1400/1410/1422` rewire to graph construction + `VariableResolver` + `GraphExecutor.Run`. **Test-first:** starlark integration test under `cmd/devlore-test/testdata/binding/` modeling adopt's mkdir → move → link sequence with `plan.var(...)` declarations is written and intentionally fails before later phases land. **Exit criterion:** full `make test` green (including the new integration test) and `writ adopt` runs through the new infrastructure with identical observable behavior. **Out of scope for 13.0(n):** the 5 nil-activation sites in `cmd/writ/writ/migrate_cmd.go:292/302/310/320` and `migrate/execute.go:81`, plus the file provider's defensive nil-activation paths — deferred to a follow-on PR with the normal small-PR cadence. **One monster PR to develop at 13.0(n) close** (one-time exception driven by structural coupling of the slot-model change). Sub-plan: [phase-8/13.0-n.md](phase-8/13.0-n.md). |
| 20 | Factor `file.Resource` into a taxonomic tree | not-started (audit-confirmed 2026-06-17, step doc [phase-8/steps/20-file-resource-taxonomy.md](phase-8/steps/20-file-resource-taxonomy.md)) | **Audit 2026-06-17:** confirmed not-started — only `type Resource struct` exists (`pkg/op/provider/file/resource.go:31`); no `file.Regular`/`file.Directory`/`file.Link` variant types anywhere (the `file.Link` grep hits are the Link *method*, not a type); zero taxonomy tests. Split the current catch-all `file.Resource` into a base type plus specialized variants: `file.Resource` retains shared identity + URI + SourcePath + cross-kind metadata; `file.Regular` holds regular-file fields (Checksum, Size, Mode-as-permissions); `file.Directory` holds directory-specific concerns; `file.Link` holds symlink target + follow behavior. Each variant implements the twelve required interfaces (per `project_resource_required_interfaces.md`). Migration: every provider method that currently accepts a generic `*file.Resource` is audited against the three variants and rewritten to accept the specific variant its semantics require (e.g., Copy/WriteText take `*file.Regular`; Mkdir returns `*file.Directory`; Link returns `*file.Link`). Gives `git.Resource` a cleaner "constrained directory" story (potential future embed of `*file.Directory` if that relationship becomes load-bearing). |
| 21 | Framework helper direct-test backfill + phase-8 PR gate | not-started (audit-confirmed 2026-06-17, step doc [phase-8/steps/21-helper-test-backfill-pr-gate.md](phase-8/steps/21-helper-test-backfill-pr-gate.md)); PR gate unmet | **Audit 2026-06-17:** confirmed not-started — none of `TestValidateGraph_CheckPromiseTypes_*` / `TestTypesAreInterconvertible_*` / `TestSubgraph_MergeBubbled_*` / `TestMethod_ResultType_*` exist; `makeMethod` (`pkg/op/validate_test.go:27`) is still synthetic (`&Method{parameters: params}`, no real `reflect.Method`), so the substep (i) extension hasn't happened. This is the same gap steps 15/16 flagged (untested `op.typesAreInterconvertible` + `checkPromiseTypes`). PR gate unmet — 2026-06-17 `make test` is 10 packages red (step 18). **The phase-8 exit gate.** Two paired outcomes: (a) close the test-coverage gap surfaced by step 19's landing — Phase 6.0's convertibility-layer helpers + step 19's `checkPromiseTypes` + the pre-existing `Method.ResultType` all landed with zero direct unit tests, relying entirely on indirect coverage via .star integration tests; (b) extend the test-helper surface in `pkg/op/validate_test.go` to enable those direct tests. Substeps: (i) extend `makeMethod` to construct a real `do reflect.Method` via reflection over mock Go provider types declared in the test file (so `Method.ResultType` is exercisable without the receiver-registry plumbing); (ii) `TestValidateGraph_CheckPromiseTypes_{Match, Mismatch, MissingProducer, NoMethod, NoParameter}` cases — 4–5 tests covering positive + negative paths of the new step-19 pass; (iii) `TestTypesAreInterconvertible_{Identity, Assignability, SourceConverter, TargetConverter, Incompatible, NilSafeProbe}` — 6 tests directly exercising Phase 6.0's pkg/op/convert.go helpers including the embedded-`ResourceBase` nil-pointer regression I fixed at landing; (iv) `TestSubgraph_MergeBubbled_{Convertible, PreferSourceSide, IrreconcilableTypes}` — 3 tests on the convertibility-aware bubble-up; (v) `TestMethod_ResultType_{FirstReturn, ErrorOnly, NoOutput, Compensable}` — 4 tests on the pre-existing helper. ~15–20 new test functions across `convert_test.go`, `subgraph_test.go` (if it exists), `method_test.go`, and `validate_test.go`. **PR gate:** phase-8 is not PR-eligible to develop until step 21 closes — which requires the full `make test` suite green: every test in `pkg/op/...`, `cmd/writ/...`, `cmd/devlore-test/...`, plus any other suite under the repo. Exit item for phase-8. This gate feeds the cross-phase [demo-milestone.md](demo-milestone.md) (criterion 16: full `make test` green). |
| 22 | Function values through the bridge → typed Go callbacks | not-started (audit-confirmed 2026-06-17, step doc [phase-8/steps/22-function-values-bridge.md](phase-8/steps/22-function-values-bridge.md)) | **Audit 2026-06-17:** confirmed not-started for the deliverable — both required conversions are absent: (b) `Convert` has no `reflect.Func`-target branch (`grep reflect.Func pkg/op/convert.go` empty); (a) the bridge passes `*starlark.Function` through as-is (`starlarkbridge/converter.go:305`), never minting a `function.Resource`. `TestWalkTreePlanned` red (the proof). Nuance: the prereq IS met — `function.Resource` carries the synthesized **source text** (the archived pack is the persistent source of truth, `function/resource.go:28-43`), and the session-service `Invoker.CallStarlark` exists (`starlarkbridge/invoker.go:34`); remaining work is the two conversions + the home-of-record decision. **Longstanding gap, not a regression** — surfaced during step-21 row-2 triage by `TestWalkTreePlanned` (now an allowed failure). Trigger: `file.walk_tree(root=…, fn=collector, …)` passes a starlark `def collector(initial, resource, path, stack)` as `fn`, whose Go parameter type is `file.Reducer = func(initial any, *file.Resource, relativePath string, *op.RecoveryStack) (any, error)`. At dispatch, `Convert` (`convert.go:137`) hits its generic "neither assignable nor convertible" fallback because **two conversions are missing**: **(a) plan-time (bridge)** — a starlark callable kwarg is never wrapped into a `function.Resource`; it stays a raw `*starlark.Function`, so the slot value the executor sees is a starlark object, not a resource. That change lives in `pkg/op/starlarkbridge/` and is therefore **staged at `pkg/op/<file>.go` for inspection** per the standing no-starlarkbridge-edit rule. **(b) dispatch-time (`Convert`)** — `Convert` has no `reflect.Func`-target branch; it needs one that, when the target is a func type and the value is a `function.Resource`, synthesizes a Go closure of the target signature which marshals the Go args → starlark, `starlark.Call`s the wrapped function, and unmarshals the result back to Go. **Portability is the load-bearing constraint** (a graph must save / load / run-many-times, and every literal must serialize into the graph): the runtime `ResourceCatalog` is **not serialized** (it re-materializes per run or from execution telemetry), so it **cannot be the home of record** — a loaded graph would have a dangling `fn`. The serialized form can be neither a live `*starlark.Function` (thread-bound, unserializable, and each run needs a fresh callable) nor the CAS **digest alone** (a hash can't be re-compiled); it must be the function's **source text** (re-compilable per run on a fresh thread), which matches `function.Resource`'s source-vs-bytecode-digest identity. **Prereq:** confirm `function.Resource` actually marshals its *source*, not just its `tag:…:sha256:…` URI — if digest-only, that gap closes first. **Home (decision pending):** lean is an **inline `ImmediateValue` slot literal** (a function literal serializes with its node like any other literal — simplest and self-contained); alternative is a **serialized function table on the graph** keyed by digest (CAS dedup if the same function is reused across slots). Either way the runtime catalog is a per-run intern cache, not the home of record. **Run-time flow:** load → source → compile a fresh `starlark.Function` on the run's thread → `Convert` wraps it as the typed Go callback. **Closures deferred:** v1 restricted to pure / top-level functions (`collector` qualifies); a closure additionally needs its captured free-variable bindings serialized, and those must themselves be serializable — a follow-on. |
| 23 | Row-4 eager-getter projection (`cmd/star/star`) | **complete** (regraded 2026-06-17 from not-started → scoped; deliverable landed + directly proven, step doc [phase-8/steps/23-eager-property-projection.md](phase-8/steps/23-eager-property-projection.md)) | **Audit 2026-06-17 — REGRADE not-started → complete.** The `+devlore:property` / `op.ModifierProperty` eager-property-projection landed end-to-end: signal (`pkg/op/method.go:780-789`) → bridge projection (`starlarkbridge/go_receiver.go:209-232`) → codegen (`config/gen/provider.gen.go:20`, `goast/gen/func_decl.gen.go:19`) → provider declarations (`config/provider.go:51`, `goast/source_file.go:660/827/907`). **Direct bridge test** at `starlarkbridge/go_receiver_test.go:20-106` (property method projects as eager value vs. plain method as callable). The gated reds are **green** on the 2026-06-17 clean tree (`cmd/star/star` ok; `TestSourceFile_StarlarkIntegration` asserts `ast.package_name` eager-getter form). Sub-plan [eager-property-projection.md](phase-8/eager-property-projection.md) still marked `in-progress` — stale for box 2 (the bridge fix), done. Original scope follows. All 9 `TestLintCopyright_*` cases + `TestSourceFile_StarlarkIntegration`. **Diagnosed 2026-05-31 (supersedes the earlier "config/namespace-injection, isolated from the framework" guess):** this is a *framework* projection gap, not a script bug — the reflection `goReceiver` surfaces zero-arg getters as callables, while the `.star` consumers and the documented [3.3](architecture/3.3-static-starlark-codegen.md) contract read them as eager properties (`config.get`, `ast.package_name`). **Decision: fix the bridge** via an opt-in per-method `MethodModifiers` / `ModifierProperty` signal (`+devlore:property`) honored by the projection; scripts pass unedited. Scoped as box 2 of the lore rewrites in [phase-8/eager-property-projection.md](phase-8/eager-property-projection.md). **Must be green before phase-8 closes** — do not move on from phase 8 with this red. |
| 24 | ActivationRecord-first invariant — codegen-enforced (hard exit gate) | not-started (audit-confirmed 2026-06-17, step doc [phase-8/steps/24-activation-record-first-invariant.md](phase-8/steps/24-activation-record-first-invariant.md)) | **Audit 2026-06-17:** confirmed not-started — the optional/detected model the invariant replaces is still in place. Discrimination present (`pkg/op/method.go:64-65` fields; conditional inject at `:508`/`:469`; live `TODO(david-noble)` at `:50-51`); no codegen/registration rejection (`receiver_type.go:400-404` detects-and-skips, tolerating both shapes); getters/pure-utils carry no leading activation param (`file.Root`/`Exists`/`IsDir`/`Join`/`Name`/`Parent`). None of codegen-reject + always-inject + discrimination-removal exists. **Phase-8 cannot close until this holds.** Every announced provider method MUST declare `*op.ActivationRecord` as its first parameter (after the receiver). Codegen **rejects with a compile-time error** any provider method whose first parameter is not `*op.ActivationRecord`. A provider developer never decides whether a method "needs" an activation record — it is always present, uniformly. **Core values:** *simplicity* (one signature shape, no per-method judgement), *discoverability* (the activation is always the first parameter, never conditional), *predictability* (every method is dispatched identically). **Consequence in `pkg/op`:** the `firstParamIsActivation` / `undoFirstParamIsActivation` discrimination (`method.go:54,61`; computed at `:108`; consumed at `Invoke:502` / `Undo:463`) is deleted — once codegen enforces the invariant, activation is *always* injected, so both flags and their conditional branches collapse away (closes the `TODO(david-noble)` at `method.go:49`). **Scope boundaries to settle at implementation (the directive is uniform; these are the edges):** (a) pure utilities (`file.Join`/`Name`/`Parent`), getters (`Exists`/`IsDir`/`Root`), encoders (`json.Encode`) gain a leading `*op.ActivationRecord` they ignore; (b) compensation companions (`CompensateX`) — their two-shape handling collapses to the single activation-first shape; (c) resource constructors (`NewResource`/`DiscoverResource`) — confirm whether they count as "provider methods" or stay exempt as package constructors. **Reconcile with D16** (`phase-8.md:1031-1034`): that note flagged a *nil* `*ActivationRecord` as a leaking-abstraction smell — under this invariant the activation is always a real, required value at dispatch, so mandating-and-populating it resolves the smell (the nil) rather than contradicting D16. Codegen validation + the bridge always-inject change + the `method.go` field/branch removal land together. |
| 25 | PowerShell naming standardization | not-started (added 2026-06-17, step doc [phase-8/steps/25-powershell-naming-standardization.md](phase-8/steps/25-powershell-naming-standardization.md)) | **Terminology standardization (added 2026-06-17).** Standardize the PowerShell vocabulary by usage role: devlore supports **PowerShell 7+** (`pwsh`), **NOT** **Windows PowerShell** (`powershell.exe`). The standard — **executable** = `pwsh` (hard-require on every platform; **drop all Windows-PowerShell fallbacks** — a capability change, not a rename); **Go package** = `powershell` (the `pkg/op/provider/powershell` provider package is kept); **completions directory** = `powershell` (`share/powershell/completions`); **product/prose** = `PowerShell`; **arbitrary literals** (e.g. `.star` gather fixtures) left. Blast radius ≈65 `powershell` occurrences across ≈20 files. Change-set: **(A) exe + drop fallbacks** — `internal/pwsh/pwsh.go:181` (remove the `powershell` LookPath fallback), `internal/credentials/helper.go:33/47/61/75/155/174/187`, `pkg/platform/windows_managers_windows.go:262/279`; **(C) completions dir** — `cmd/star/cli/selfinstall.go:330` + the `internal/cli/selfinstall.go` twin + their `*_test.go` key fix (`"powershell"`→`"pwsh"`, expected dir stays `share/powershell/completions`); **(D) prose** → `PowerShell` (`pkg/op/provider/powershell/provider.go` header, `pkg/op/provider/shell/provider.go:7`, docs). Closes the `TestShellCompletionPath/powershell` red (step 18). **To settle at implementation:** (1) group A removes Windows-PowerShell support — capability decision + ownership (platform/credentials code is outside the tidier lane); (2) whether the package-name rule renames `internal/pwsh` (package `pwsh`) → `powershell`; (3) shell-selector key = `pwsh` (keys off exe names bash/fish/pwsh/zsh). Own branch, separate from the phase-8 audit. |
| 26 | Relocate RuntimeEnvironment from the Provider surface to the Resource surface | not-started (added 2026-06-18, step doc [phase-8/steps/26-relocate-env-provider-to-resource.md](phase-8/steps/26-relocate-env-provider-to-resource.md)); gated on step 24 | **Added 2026-06-18.** Providers use the env only at dispatch (→ `activation.RuntimeEnvironment`); resources use it off-dispatch (catalog/preflight/compensation/serialization) where no activation is in scope, including the **fixed-signature marshalers** (`UnmarshalJSON` can't take an env param). So: **remove** `RuntimeEnvironment()` from the `op.Provider` interface (`provider.go:12-14`) + the field/accessor from `op.ProviderBase` (`provider.go:24`,`:34`); **add** `RuntimeEnvironment()` to the `op.Resource` interface (`resource.go:42`) + an own field/accessor on `op.ResourceBase` (today it embeds `ProviderBase` solely for the env, `resource.go:79`). Providers become stateless dispatch targets; resources keep the env they need. Cost: ≈87 provider-method rewrites `p.RuntimeEnvironment()`→`activation.RuntimeEnvironment` (rides step 24) + a small struct/interface split; the Tier-2 marshaler-rehydration cost is **avoided** by keeping the env on resources. Open: whether `Resource` should stop embedding `Provider` once `ProviderBase` is fieldless. |
| 27 | Caller id on the activation — Starlark call-site via Thread.CallFrame | not-started (added 2026-06-18, step doc [phase-8/steps/27-starlark-callsite-unit-id.md](phase-8/steps/27-starlark-callsite-unit-id.md)) | **Added 2026-06-18.** A provider method is the callee; both a graph `ExecutableUnit` (a graph-encoded call) and a `.star` line (a script-encoded call) are **callers**. The activation should carry a `callerID string` identifying that caller, not a `unit ExecutableUnit`. **Design (settled):** `NewActivationRecord(graph, unit ExecutableUnit, env)` → `NewActivationRecord(graph, callerID string, env)`; activation stores `CallerID string`. Graph dispatch → the unit's `ID()`; Starlark → a `file:line:col` from `thread.CallStack()[last].Pos.String()` (or `""`). Stamped onto `resource.producerID` (the caller seen from the resource's side), so a debugger shows `ProducerID() → "mkfile.star:42:8"` — strict improvement over today's empty stamp. Verified practical: producer stamping (~12 sites) takes a `producerID string` (already only reads `Unit.ID()`); the 4 typed-unit consumers (flow `Gather`/`Subgraph` `flow/provider.go:204`/`:364`, `method.go:545`/`:557`) resolve via existing `Graph.ResolveExecutable(callerID)` (`graph.go:575`); the Graph/Unit pairing invariant dissolves. Thread already passed to `g.dispatch` (discarded as `_`, `go_receiver.go:561`); pattern at `trace.go:47-51`. Name chosen over `unitID` (graph-only), `siteID`/`originID` (collide with `RecoverySite`/`op.Origin`). Caveats: call-site ≠ per-invocation (loops collapse); Starlark-only (nil thread on CLI/test/Go + eager-property path → empty). |
| 28 | Save / load / restart scenario coverage — Go + Starlark variants, then pause/resume | partial (added 2026-06-19; updated 2026-06-20) — Go + **Starlark** variants landed (the `*_definition` rename executed 2026-06-20); pause/resume **blocked** on a prerequisite execution-core refactor (per-subgraph executors must own their recovery stacks; combinators currently mint them) — **prerequisite design approved 2026-06-20, implementation of (a) in progress 2026-06-21** ([step doc](phase-8/steps/28-subgraph-executor-ownership.md)): `newChildExecutor` + shared pause flag + `Run` Paused-stamp landed (`graph_executor.go`); `subgraph.Execute` creates the child executor and `flow.Subgraph` binds kwargs→parameters. **Open regression + fork:** routing children through the child executor via `activation.Stack` overloaded that field — combinators read `activation.Stack` for *input* promise resolution (`resolveDispatchedValue` → upstream siblings = the parent stack), so repointing it at the child stack broke `TestChoose_NotExists`/`TestChoose_Predicates` and `TestSubgraph_ReturnsRecoveryStack`. **Resolution chosen — option (C), chained recovery stacks:** `activation.Stack` stays the executor's own stack; `ResultByUnitID` walks the **parent chain up** for input resolution (stacks chain up for resolution, and down for unwind via the subgraph receipt's `*op.RecoveryStack` complement — `PushNested` now only for `Gather`'s internal per-item grouping). Fix: add a parent pointer to `RecoveryStack`, walk it in `ResultByUnitID`, and re-derive the chain on `Trace` load (complement nesting durable / parent pointer transient). Re-greening of Choose + the Subgraph integration test follows. See the [Chained recovery stacks section](phase-8/steps/28-subgraph-executor-ownership.md#chained-recovery-stacks--up-for-resolution-down-for-unwind) of the step doc. **Compensation decision (closing the open issue):** the named `Compensate` companion is the live undo path. This requires fixing a base-`op` latent bug — `RecoveryStack.Push`'s compensable gate keys on `receipt.Resource() != nil` (and `invokeCompensateForReceipt` reads the env off that resource), so any complement that is not a single resource's receipt is silently demoted to audit-only. `file.Provider.WalkTree` (`file/provider.go:710`) proves it outside flow: it returns a `*op.RecoveryStack` complement and declares `CompensateWalkTree` (`:786`), yet that companion is dead code today (no resource → audit-only → compensation rides the nested auto-unwind), and `Gather`'s `[]*op.RecoveryStack` slice would be dropped outright. Fix: gate on `Complement() != nil`, supply the env from the executor, and route the compensate closure through the action's `Undo` companion (registry-resolved, so it survives `Trace` load). See the [Compensation section](phase-8/steps/28-subgraph-executor-ownership.md#compensation-gates-on-the-complement-not-a-resource) of the step doc. **Implemented 2026-06-22:** `RecoveryStack.Push` gates on `Complement() != nil` and takes the executor env; `invokeCompensateForReceipt` is env-parameterized and hands `Receipt.Complement()` to `Method.Undo`; `pushAuditReceipt` drops the nested-stack branch (a `*op.RecoveryStack` complement rides the audit receipt via `Commit`); `RecoveryStack.Receipts` descends into a recovery-stack complement (preserving `Trace.Summarize` coverage); `buildSubStackFromReceiptSlice` threads `activation.RuntimeEnvironment`. Confirmed equivalent on the resource path — `receipt.Complement() == receipt` for a resource action (`method.go:545`). `make test`: `pkg/op` + `flow` + `file` compensation green, zero new failures. **Failure→unwind wiring implemented 2026-06-22** (`TestCompensation` now passes — was a standing baseline failure): the cascade was masked by two execution-core defects the compensation gate above first exposed. (1) `Method.Invoke` (`method.go`) discarded the complement on any dispatch error (`if err != nil { return nil, nil, err }`), so a failed subgraph's recovery stack never reached its audit receipt — now the complement is committed and returned **alongside** `dispatchErr`, so the receipt is compensable and `Run`'s top-level `e.stack.Unwind()` cascades through `CompensateSubgraph` into the children. (2) `invokeCompensateForReceipt` resolved the companion via `ReceiverRegistry().ActionByPath`, which keys on the Go-qualified `ActionName`, but a name-bound unit (the graph root, every combinator) records the **dotted** action name (`flow.subgraph`) as its action path — `ActionByPath` missed, so the lookup now falls back to `RuntimeEnvironment.ActionByName` (the resolver dispatch uses). Both proven via a live stack dump at the undo seam. Remaining standing baseline failures: `TestBackup_DefaultSuffix`, `TestWalkTree_Planned`, `TestShellCompletionPath_PerShell` (unrelated). **(b) resume design settled 2026-06-22 — pseudo replay:** resume is a side-effect-free re-descent — `Run` re-walks the graph from the root and the restored trace's receipts dictate the descent: skip any unit with a successful receipt (return its cached result, prune the completed subtree) and descend only the unfinished spine to the frontier. "Do nothing" is not literally passive: the descent must **adopt each subgraph's restored child stack** (option (C) mints a fresh one otherwise → would re-run completed children) and **re-resolve the per-subgraph variable frames** (the one bit of state the `Trace` does not carry; pure recomputation against the restored stacks). Re-walk rather than a literal jump because `ErrPaused` unwound the Go call stack back to `Run` — only the recovery stack is restorable data. Scope (increment X): `Run` preamble state-driven (accept `RunStatePaused`, keep `trace.Stack`, use `trace.Variables`); per-unit skip/adopt/fresh guard in `node.Execute`/`subgraph.Execute`; pause-receipt supersession. No replay-map, no work-list rewrite, no frame persistence; the larger resumable-dispatch-loop rewrite (Y) deferred. See the [Resume re-entry section](phase-8/steps/28-subgraph-executor-ownership.md#resume-re-entry--pseudo-replay-settled-2026-06-22) of the step doc. Control plane (commands-in + events-out) scoped as **step 33** | **Added 2026-06-19.** End-to-end scenario over the public API: plan a graph → save the graph → load the graph → execute the *loaded* graph → save the [op.Trace]. Requested in **two variants — a Go API variant and a Starlark API variant** — followed by the same scenario with a **pause injected mid-execution**, verifying the run resumes. **Status:** the Go variant landed as `TestGraphSaveLoadExecuteTrace_ViaPublicAPI` (`pkg/op/provider/plan/lifecycle_api_test.go`); it is what exposed the `SlotValue`-not-deserializable defect that drove the `bindingData` serialization envelope (see step 13). **Outstanding — both remaining variants surfaced real gaps (the point of this step):** (1) the **Starlark** variant **landed 2026-06-20**. It was blocked because `plan.load(...)` is a Starlark parse error — `load` is a reserved keyword and cannot be an attribute name (proven differentially: `plan.save` parsed; `plan.load` errored `not an identifier`). **Fix:** renamed the plan provider's `Assemble`/`Save`/`Load` → `AssembleDefinition`/`SaveDefinition`/`LoadDefinition` (Starlark `plan.assemble_definition` / `plan.save_definition` / `plan.load_definition` via `op.CamelToSnake`); the `Definition` noun is the [workflow-rename](../workflow-rename.md) taxonomy for `Graph`. Chose `load_definition` over the proposed `bind_definition` because binding — `PromiseBinding.Resolve` against the recovery stack — happens at **run**, not load (the stack exists only during a run and fills incrementally), so no `Definition` is ever "bound" before `plan.run`; `bind` would over-promise. **Executed 2026-06-20:** methods renamed (`provider.go`/`helpers.go`), gen regenerated (`provider.gen.go`/`receiver_type.gen_test.go`), 53 `.star` fixtures + the embedded-script test files (`devloretest/commands_test.go`, `flow/result_flow_starlark_test.go`) + plan-package callers updated, and `TestGraphSaveLoadExecute_ViaStarlark` un-skipped and **passing**. `Run`/`Spec` stay unsuffixed. The 4 `cmd/writ` consumer call sites are now build-broken on the rename → step 30 (alongside their pre-existing `ImmediateOf`/`execution.*` drift). `make test`: 84 ok, zero new failures. (2) the **pause/resume** variant is *blocked* on an **unimplemented resume path** — not a missing test. `op.ResumeExecutor` restores `state`/`stack`/`variables` from the [op.Trace] (`graph_executor.go:139-142`), but `Run` cannot consume them: it rejects a resumed executor (`e.state != RunStatePending`, `:244`; a resumed executor is `RunStatePaused`), resets the stack (`e.stack = NewRecoveryStack()`, `:261`, discarding `trace.Stack`), and has no skip-already-completed guard in dispatch (`node.go`/`subgraph.go` query `ResultByUnitID` only for slot resolution). Greening it requires implementing resume re-entry (accept `RunStatePaused`, preserve `trace.Stack`, skip already-receipted units). **Prerequisite uncovered 2026-06-20 — a deeper execution-core refactor blocks the resume re-entry itself:** skip-by-receipt cannot work while flow combinators mint their own recovery stacks, because a re-entered subgraph re-runs `Do()` → a fresh empty `op.NewRecoveryStack()` (`flow/provider.go`: `Subgraph` `:369`, `Gather` per-iteration `:234`, `Choose` `:115`), so the trace's saved children are invisible to the re-dispatch. The correct model (recorded in [2.3-orchestration-primitives.md](../../architecture/2.3-orchestration-primitives.md#subgraph-execution--recovery-stack-ownership-current-model--2026-06-20)) is that **every subgraph executes via its own executor that owns its recovery stack** (+ variable scope, pause, trace, catalog) — `Gather`/`Choose`/`Subgraph` are not special, all are subgraphs with their own executors, one recursive rule; a combinator's `Do()` minting a stack is the deviation. Today there is a single shared `op.GraphExecutor` handed to children via the `dispatchChild` closure while combinators hand-roll stacks. **Sequence to close step 28:** (a) make subgraph dispatch go through a per-subgraph executor that owns its stack (the framework supplies/restores it; combinators stop calling `NewRecoveryStack()`); (b) resume re-entry + skip-completed, falling out recursively; (c) capture/restore the catalog in `op.Trace` (today `Trace` carries `GraphChecksum`/`State`/`Stack`/`Variables` but no catalog, so resources produced pre-pause are not restored — promise/slot flow survives via the stack, catalog-mediated URI sharing does not). Step 28 does **not** close until (c) lands. (a) is an execution-core architecture change — surface for an owner, not tidier work. **Prerequisite (a) design drafted 2026-06-20** (the symmetric ownership change: **every combinator keeps both its action and its compensate companion** — the action returns its compensation state as the complement and the companion undoes it; what changes is the *source* of the stack — the per-subgraph executor owns and creates it, so `Do()` no longer mints `op.NewRecoveryStack()`. `Choose`, `Subgraph`, and `WaitUntil` carry a single `*op.RecoveryStack`; `Gather` carries a `[]*op.RecoveryStack` slice and calls `Subgraph` once per item; `Subgraph` drops its vestigial `items`; `WaitUntil` becomes a combinator (`Subgraph` + poll-until-true/timeout); child executors share env/var-frame/pause and own their stack. The full combinator design — foundational principles (every combinator IS a subgraph; every combinator except `Subgraph` delegates to `flow.Provider.Subgraph` to run its subgraph one-or-more times; **`Choose` does NOT select — `ChoosePlanner` builds the branches into the graph, the graph selects, `Choose` only receives the result**), the sorted action/compensation signature table, and per-combinator behavior — is in the [step doc's Combinator-signatures section](phase-8/steps/28-subgraph-executor-ownership.md#combinator-signatures-confirmed-in-review--2026-06-20)). Saga-boundary semantics **settled 2026-06-20**: the boundary is maintained and retry-gated — each subgraph executor exhausts its retry policy before rollback continues up the stack (no retries → propagate immediately; retry count N → all N first), each executor unwinding its own stack outward rather than one root-level sweep. Each failed attempt first unwinds the boundary to its entry precondition before retrying — forced by atomicity (re-running completed children without undo double-applies non-idempotent work), not a choice. `DispatchChild`'s stack parameter is **dropped** (settled): once the dispatching executor owns its stack, the param only carries the stack that executor already holds. Design fully pinned — no open forks before code. Step doc: [phase-8/steps/28-subgraph-executor-ownership.md](phase-8/steps/28-subgraph-executor-ownership.md). |
| 29 | Compiler-checked action names | not-started — design settled 2026-06-19, implementation pending | **Added 2026-06-19.** Stop hand-formulating stringly-typed action names like `"file.write_text"` across tests and production code. Provide **compiler-checked** names — e.g. `file.WriteText` — hung on the **provider package**, explicitly *not* requiring an import of `pkg/op/provider/<package>/gen` and a `<package>.<function>` reference. **Design settled 2026-06-19:** add `type ActionName string` to `pkg/op/action.go` (beside the `Action` interface); codegen emits `const WriteText op.ActionName = "file.write_text"` (etc.) into the **provider package root** (not the `gen` subpackage), so callers write `plan.Plan(file.WriteText, …)`. `op.ActionName` covers **only the short starlark action name** — the `Action.Name()` / `plan.Provider.Plan(name)` / const surfaces that carry `"file.write_text"`. The **fully-qualified type identity** — `Action.FullName()` / `Method.ActionName()` = `<pkg-path>.<receiverName>.<methodName>` (`method.go:328`), the receipt-stamp form — **stays `string`**: it is a different concept (reflect's `(PkgPath, Name)` identity, for which the Go spec has no term), not the short name. Loose end for implementation: `BuildAction` (`receiver_registry.go:538`) keys lookups on the **full** form while `plan.Plan` takes the **short** form — trace the short→identity resolution before retyping signatures. |
| 30 | `writ migrate` — full rewrite onto the sealed-graph executor | not-started (added 2026-06-19) — **full rewrite, not an edit** | **Added 2026-06-19. Full rewrite, not an incremental fix.** `cmd/writ/writ/migrate` reimplements graph execution by hand instead of running the assembled graph. `Execute` (`migrate/execute.go`, called from `migrate_cmd.go:213`) filters the graph's `file.move` nodes into a `renameNodes` worklist, strip-mines each node's `path`/`source` literals via `op.ImmediateOf(node.Slots()[…])`, and re-dispatches every rename through `Move()` (`migrate/file_ops.go`), which builds its *own* single-node graph and runs that — so the assembled graph is never executed as a graph; instead N one-node graphs are constructed and run. The conflict precheck ("target exists") is hand-rolled the same way. **The correct pattern already exists in the same package:** `migrate/session.go:556-572` runs `s.graph` via `op.NewGraphExecutor(...).Run(...)` and writes `executor.Trace()` as the receipt. **Rewrite target:** collapse `Execute` onto `GraphExecutor.Run(graph, spec)` (the `session.go` path), deleting the `renameNodes` loop and its slot-peeking; the target-exists check becomes a real preflight pass rather than literal reads; the only legitimate remaining graph-slot inspection is the human-facing `.writ-migrated` marker (reporting). Folds in the migrate-package `op.ImmediateOf` callers (`execute.go` / `format.go` / `session.go`) as part of the broader `ImmediateOf` decision. Note: the `cmd/writ` parent is independently build-broken on the separate `execution.StateView`/`FileEntry` drift, and (2026-06-20) the `adopt`/`migrate` callers now also fail to compile on the `plan.Provider.Assemble` → `AssembleDefinition` rename (step 28); the rewrite adopts the renamed `*Definition` methods. |
| 31 | Architecture docs — rewrite `2.3`, `2`, `2.2` onto the `pkg/op` model in full | not-started (added 2026-06-20) | **Added 2026-06-20.** The execution-model architecture docs describe the superseded pre-`pkg/op` design and must be rewritten onto the sealed-graph model, not patched. **Stale core:** [`2-execution-graph.md`](../../architecture/2-execution-graph.md) (`internal/graph/builder.go`, `ExecutionGraph`, `graph.Run()`, the Command-Layer→GraphBuilder pipeline), [`2.2-phase-execution.md`](../../architecture/2.2-phase-execution.md) (the saga/phase model), and the body of [`2.3-orchestration-primitives.md`](../../architecture/2.3-orchestration-primitives.md) from `## Vocabulary` down (`Phase` type, `Graph.Phases`, `RunPhased`, `ExecutePhaseInner`, `ActivationState`, `SlotValue`, the `internal/execution/*` + `pkg/op/recovery.go` file map). **Rewrite target:** the sealed `op.Graph` of `op.Subgraph`/`op.Node` units in `pkg/op`; `op.Binding` slots (`Immediate`/`Promise`/`Variable`) replacing `SlotValue`; dispatch via each unit's bound action (no `Phase`/`RunPhased`); `op.GraphExecutor` + `op.RecoveryStack` as the runtime; and the **per-subgraph-executor recovery-stack-ownership model** now recorded in the dated [`2.3` section](../../architecture/2.3-orchestration-primitives.md#subgraph-execution--recovery-stack-ownership-current-model--2026-06-20) as the authoritative principle. The `2.3` dated section + its staleness blockquote (landed 2026-06-20) are the seed; this step finishes the job by replacing the historical body rather than leaving it fenced off. **Scope note:** scattered `SlotValue`→`Binding` and `internal/execution`→`pkg/op` references also appear in `2.1`, `3.2`, `8`, and others (grep 2026-06-20); whether they fold into this step or a follow-on documentation-debt sweep is open. Documentation work, not a code deliverable; sequence after the execution-core changes (step 28's prerequisite) so the docs describe landed reality. |
| 32 | Retry-policy tri-state + per-type defaults | not-started (added 2026-06-20) — design settled | **Added 2026-06-20.** Realize `RetryPolicy` as a true tri-state and give nodes and subgraphs distinct defaults. **Today it is not tri-state:** the policy is carried as `*RetryPolicy` (nil when unset, `executable_unit.go:240`); `DispatchChild` treats nil as one attempt (`activation_record.go:172-177`) and the field contract says `MaxAttempts:0` = "no retry, fail immediately" (`retry_policy.go:13`) — so **nil ≡ `MaxAttempts:0` ≡ no-retry** are conflated, and both a node (`node.go:59`) and a subgraph (`subgraph.go:719`) default to no-retry identically. **Tri-state to realize:** (1) *no retry* — explicit `MaxAttempts:0`, fail immediately; (2) *default retry* — unset/nil, defer to a framework default; (3) *non-default retry* — explicit `MaxAttempts:N>0`. The change is that **nil stops meaning no-retry and starts meaning *default*** (an explicit `MaxAttempts:0` carries "no retry"). **Per-type defaults (settled 2026-06-20):** a **node** carries an explicit `MaxAttempts:0` (no retry) by default — a leaf is one provider call, so fail-fast and let the enclosing boundary decide; a **subgraph** carries **nil** by default, which resolves to the **graph's default retry policy** — the subgraph *is* the saga boundary and step 28's rollback rule is gated on its retry policy, so defaulting it to retry is what makes "exhaust retries, then roll back up" the actual default. The two unset-defaults are deliberately different representations: a node stamps an explicit no-retry policy; a subgraph stays nil and inherits the graph default at resolution time. **The graph default retry policy is `MaxAttempts:3` with exponential backoff and jitter.** **New work this implies:** (a) **nil-resolution** — `DispatchChild` must resolve nil to the graph default (today nil → 1 attempt, `activation_record.go:172-177`), so nil stops meaning no-retry; (b) **jitter** — `RetryPolicy` (`retry_policy.go`) today has `Backoff` (none/linear/exponential), `InitialDelay`, `MaxDelay` but **no jitter**, so a jitter component must be added and applied in `ComputeDelay`; (c) **node default** — node construction must stamp `MaxAttempts:0` (today `node.go:59` leaves it nil); (d) a **graph-level default policy** must exist to resolve nil against. **Touches:** `DispatchChild`, node/subgraph construction (`node.go:59`, `subgraph.go:719`), `RetryPolicy` (jitter) + its doc contract, the graph default-policy carrier, and the step-28 saga rollback gating. Pairs with step 28. |
| 33 | Control plane — the executor's bidirectional command / event surface | not-started (added 2026-06-21) — design recorded | **Added 2026-06-21.** Realize the executor's **control plane**: one surface, two directions a listener bridges a connection to (subscribe to events, issue commands). **Commands in:** replace the `*atomic.Bool` pause flag (`graph_executor.go`) with a shared `*ExecutionControl` carrying a `ControlCommand` (`ControlNone`/`ControlPause`/`ControlStop`/`ControlStep`/…), polled at each control-point via a `switch` (was `pausePointObserved`); add **`Stop`** (`ErrStopped` → unwind + `RunStateStopped`, **not** resumable — distinct from pause's preserve-and-resume), then `step`. `GraphExecutor.Pause()`/`Stop()` delegate to `control.Request(...)`; the listener does the same for inbound connection commands. **Events out:** unify the lifecycle `HookRegistry` (`graph_executor.go:44`) with the `Status *status.Narrator` and `Result *result.Pipeline` channels — and **move `Status` and `Result` off `RuntimeEnvironment` onto the control plane** (they are events-out channels, not the world; a real refactor that re-threads the *do*-layer emission path from `activation.RuntimeEnvironment.Status`/`.Result`). **Ownership:** the plane lives on the `GraphExecutor` — the run's driver and command target — shared to child executors via `newChildExecutor` like the runtime environment, **not** on `RuntimeEnvironment`. **Relationship to step 28:** step 28 implements **only pause**, but through the `*ExecutionControl` primitive (carrying `ControlPause`), so the plane is forward-compatible from the start; this step realizes the rest — `Stop`/`step`, the listener/connection, and the `Status`/`Result` migration. Full design: the [Control plane section](phase-8/steps/28-subgraph-executor-ownership.md#control-plane--the-executors-bidirectional-command--event-surface) of the step-28 doc. |

### Milestone prerequisites — platform & pkg deploy (added 2026-06-04)

**These three sub-plans must land before step 18's Scenario-1 exit** (`lore deploy docker` on Ubuntu, then macOS).
The reason is direct: lore installs Docker via `pkg.install` (`apt` on Ubuntu, `brew` on macOS), and the current
`pkg.Provider` cannot — its surface is stale, surfaced by the failing `cmd/lore` behavioral test. The
Composite-router design fixes that and pulls the platform contract and the SAGA failure semantics along with it.

| # | Sub-plan | Status | Notes |
|---|---|---|---|
| 18.4 | Platform unification — `op.Platform` struct + Composite `op.PackageManager` router | in-progress | Contract landed: `op.Platform` (`pkg/op/platform.go:9`, a concrete struct — the design reversed the earlier interface flip) + the `op.PackageManager` router (`platform.go:65`) that routes by purl and fans out to leaf drivers; `pkg/platform` fully reshaped and style-compliant. Remaining: consumer migration (cmd/). [phase-8/platform-unification.md](phase-8/platform-unification.md) |
| 18.5 | `pkg.Provider` thin veneer over the router | complete | `pkg.Provider` is a thin veneer over the Composite router (`pkg/op/provider/pkg/provider.go:13`); `Install`/`Remove`/`Upgrade`/`Installed`/`Version`/`Update` all delegate to `plat.PackageManager()` routing by purl (no `manager` arg). #6 closed 2026-06-07 (provider/pkg + provider/service); the whole `pkg/op` tree green. [phase-8/pkg-install-reconciler.md](phase-8/pkg-install-reconciler.md) |
| 18.6 | SAGA failure-handling & compensation-failure contract | draft | Four run terminals `Completed`/`Degraded`/`Failed`/`Stranded`; error actions MUST run; a failed `Compensate` → `Stranded` (fail loud + `Trace` journal + restart). Cross-cutting; governs the deploy's failure semantics. [phase-8/compensation-failure-contract.md](phase-8/compensation-failure-contract.md) |

**Sequence:** 18.4 (platform contract + router) → 18.5 (pkg veneer) → step 18 Scenario-1 deploy. 18.6 is
cross-cutting and lands alongside — the deploy's failure terminals depend on it.

Plus unresolved design discussions that must close before phase-8 exits:

| # | Topic | Status |
|---|---|---|
| O1 | Marshaling design — argument-to-parameter-type matching via ReceiverType-hosted marshalers | open; direction stated, five questions pending |
| O2 | Toss the bind package — the 11 `unmarshal_*.go` files + `Unmarshaler` interface go; names survive | **code done** (verified 2026-06-15: no `bind/` dir, no `unmarshal_*.go` in tree, `Unmarshaler` interface gone); the marshaling-design questions tied to O1 remain open |
| O3 | Rename `pkg/op` → `pkg/workflow` and revisit type names | open; blast-radius surveyed, strawman considered, counter-proposal recorded |

**Status:** in-progress (Phase 6 of 19(n) closed 2026-05-24; phase-8 exit pending steps 11, 12, 18–27 + O1/O2/O3).
Steps 1–9 complete. **10.0 (a) through (n) all closed.** The step-18 graph-immutability seal landed in
`pkg/op` production code; `make build` / `make vet` are **currently red** in the consumers and test
surface the seal broke (flow helpers, `pkg/op` tests, gen test templates, `lore` / `writ`) — the
remaining migration is tracked in [phase-8/21-graph-immutability.md](phase-8/21-graph-immutability.md).

**13.0 closure summary.** Steps 13.0(a)–(m) landed in prior commits per the inventory table above. 13.0(n)
— variable binding infrastructure — closed on 2026-05-24 with the writ adopt migration integration
(Phase 6) landing the binding model end-to-end:

- **Phase 5** (planner-dispatch model + Tier-3 plan methods; step 19 inventory of 21 substeps) landed
  earlier. `flow.Gather` rewritten to PowerShell ForEach-Object shape; `ChoosePlanner` / `GatherPlanner` /
  `SubgraphPlanner` / `WaitUntilPlanner` registered in `MethodMetadata`; framework variables-map flip
  (overrides → variables) across the dispatch chain; `ActivationRecord.Variables` per dispatch;
  `Graph.ExecuteWithStack` propagates `g.ctx` into the fresh executor.
- **Phase 6.0** — Convertibility-aware bubble-up + provider `TargetConverter` contract. `pkg/op/
  typesAreInterconvertible` + `Subgraph.mergeBubbled` consult convertibility before declaring slot-type
  collisions; `preferSourceSide` picks source-side primitives over Resource-typed slots. `TargetConverter`
  opt-in on `file` / `git` / `appnet` / `pkg` / `service` Resources; CAS providers (`mem` / `function` /
  `json` / `yaml`) intentionally opt out (natural sources are content bytes, not CLI strings). `op.Convert`
  step 6 / step 7 reorder so the registered-Resource constructor preempts `TargetConverter` at dispatch.
- **Phase 6.A** — Baseline writ_adopt tests + 3 surface fixes. All 7 `test_writ_adopt*.star` tests wired
  into `runner_test.go` and green. `SubgraphPlanner.Plan` defaults `items=[]any{}` when not supplied;
  `VariableResolver.EnvPrefix` converts hyphens to underscores; `t.set_env(dict)` builtin +
  `GraphExecutor.LastVariables()` accessor.
- **Phase 6.B** — Adopt surface extraction. `cmd/writ/writ/adopt_cmd.go` (295 lines) +
  `cmd/writ/writ/adopt/{adopt,plan,execute}.go` stubs.
- **Phase 6.C** — Rewire `adoptFile` through the binding model. `adopt.BuildGraph` constructs the
  three-node mkdir → move → link graph via `plan.Provider.Variable` references; `adopt.Run` wraps
  `executor.Run` with `mapAdoptError`. Dual-spec pattern (planning + execution share nothing but the
  resolved graph, since `env.Close` closes the spec's Root). EXDEV fallback dropped per Q2.
- **Phase 6.D** — Behavioral coverage. 5 in-process integration tests covering happy path, dry-run,
  destination-exists, directory walk, symlink skip. Surfaced and fixed a framework bug: `complementOrNil`
  didn't detect typed-nil pointers, panicking when provider methods returned `(result, nil, nil)` for
  no-compensation cases (e.g., `file.Mkdir` on an existing directory).

Sub-plan: [phase-8/13.0-n-phase-6.md](phase-8/13.0-n-phase-6.md).

**Open items toward phase-8 close:**

- **Per-step status is in the table above** (the authoritative source). Step 21 is the explicit phase-8
  PR gate — phase-8 is not PR-eligible to develop until step 21 closes AND the full `make test` suite is
  green.
- **Step 21 — Framework helper direct-test backfill + PR gate.** Phase 6.0's convertibility-layer
  helpers (`typesAreInterconvertible` / `sourceSideAdvertises` / `targetSideAdvertises` / `mergeBubbled`
  / `preferSourceSide`) and step 16's `checkPromiseTypes` plus the pre-existing `Method.ResultType` all
  landed with zero direct unit tests, relying on indirect integration coverage. Step 21 closes that gap
  with ~15–20 new test functions, plus extends `validate_test.go`'s `makeMethod` to construct real
  `do reflect.Method` values so `Method.ResultType` is exercisable without the receiver-registry
  plumbing.
- **Step 18 — graph immutability + test triage.** Framework half **landed** (2026-05-27): `Graph`
  sealed, run state moved to the executor (`RunState` + `Trace`), `Action.Do` arity dropped. Remaining:
  the consumer / test / template migration the seal broke — flow helpers, `pkg/op` tests, gen test
  templates, `lore`, `writ` — plus the original test-triage backlog (`TestImm*`, `TestWalkTreePlanned`,
  `TestCLI_*`, `TestLintCopyright_*`, `TestSourceFile_StarlarkIntegration`). Sub-plan:
  [phase-8/21-graph-immutability.md](phase-8/21-graph-immutability.md).
- **Step 20 — `file.Resource` taxonomic split** (`file.Regular` / `file.Directory` / `file.Link`).
- **Step 25 (table row) — PowerShell naming standardization.** Added 2026-06-17.
  Standardize the PowerShell vocabulary by usage role — executable `pwsh` (hard-require on every platform;
  drop Windows-PowerShell fallbacks), Go package `powershell`, completions directory `powershell`,
  product/prose `PowerShell`. Own branch. Step doc:
  [phase-8/steps/25-powershell-naming-standardization.md](phase-8/steps/25-powershell-naming-standardization.md).
- **Step 26 (table row) — Relocate RuntimeEnvironment from providers to resources.** Added 2026-06-18.
  Remove the env from the `op.Provider` interface + `op.ProviderBase`; add it to the `op.Resource`
  interface + `op.ResourceBase`. Providers become stateless dispatch targets (read
  `activation.RuntimeEnvironment`); resources keep the env for off-dispatch I/O and the fixed-signature
  marshalers. Gated on step 24. Step doc:
  [phase-8/steps/26-relocate-env-provider-to-resource.md](phase-8/steps/26-relocate-env-provider-to-resource.md).
- **Step 27 (table row) — Caller id on the activation.** Added 2026-06-18. The activation carries a
  `callerID string` (the caller of the dispatched provider method), not a `unit ExecutableUnit`: a graph
  unit's `ID()` in graph dispatch, a `file:line:col` from `thread.CallStack().Pos` in Starlark. Stamped onto
  `resource.producerID` so a debugger shows the originating call. Typed-unit consumers resolve via existing
  `Graph.ResolveExecutable`. Step doc:
  [phase-8/steps/27-starlark-callsite-unit-id.md](phase-8/steps/27-starlark-callsite-unit-id.md).
- **Open design items O1–O3.** O1 (marshaling redesign) + O2 (toss the `bind` package, tied to O1) are
  exit blockers. O3 (`pkg/op` → `pkg/workflow` rename) is surveyed at ~5K LOC of mechanical churn; defer
  decision pending phase-8 closure.
- **Phase 7 (writ migrate cleanup + file provider defensive paths)** lands as the next follow-on PR after
  Phase 6. Out of scope for 13.0(n) per the original sub-plan.

Successor designs for plan.choose (step 10), plan.gather (step 11), and plan.wait_until (step 12) were
folded into 13.0(n) Phase 5; the rows above retain their original wording for traceability but the work
itself is done.

# Phase 8: Plan-time scope and grouping combinators

## Summary

Every `plan.*` call returns an invocation (`*starlarkbridge.Invocation`) — it does
not attach anything to any graph. Invocations are detached by default.
Explicit combinator calls (`plan.subgraph`, `plan.choose`,
`plan.gather`, `plan.wait_until`) bundle invocations into
containers. A `plan.run(...)` call at the end of each `.star` file names
the root — anything not in the root's transitive closure is an orphan
and errors at plan time.

An invocation carries both representations needed at every binding site:
the `op.ExecutableUnit` (for slots that want an executable reference —
combinator bodies, branches, iteration targets) and a `Promise` (for
slots that want a value — consumes the invocation's output via an edge).
The binding layer (`plan.Provider.FillSlot` after step 5; formerly
`starlarkbridge.NodeBuilder.FillSlot`) picks which field to use based on the target
slot's type. Starlark authors don't distinguish — invocations are
polymorphic at the call site. The binding layer handles the dispatch
transparently.

Phase 8 absorbs what was formerly Phase 11 ("Implement `plan.subgraph` as a
Flow Provider Method"). `plan.subgraph` is the general form; the old
single-case Phase 11 proposal is one usage of it.

## Problem

Strict-eval starlark evaluates inner expressions before outer ones. Under
the current model:

```python
plan.choose(
    defaultValue=plan.file.write_text(path, "default"),
    case(when=..., then=plan.file.remove(path)),
)
```

Both `plan.file.write_text(...)` and `plan.file.remove(...)` evaluate
before `plan.choose` runs. They attach to the enclosing subgraph as
children — and run unconditionally at execution time. The "choose one
branch" semantic is broken before it starts.

The problem generalizes across every grouping combinator. Without an
explicit deferral mechanism, any nested `plan.*` call attaches to the
wrong scope.

**Two alternatives considered and rejected:**

1. **Plan-time lambdas + scope stack.** The planner maintains a scope stack,
   combinators accept `lambda: …` expressions, evaluating them pushes a
   scope, and nested `plan.*` calls attach to the pushed scope. Rejected —
   the scope stack is ambient mutable state at plan time, violating
   invariant I2. Lambdas also add syntax cost at every combinator arg.
2. **Explicit `plan.detach(plan.file.write_text(...))` wrappers.** Forces
   every arg to be wrapped. Rejected on ergonomics and failure mode
   (forgetting the wrapper silently attaches to the wrong scope).

The adopted approach — invocations detached by default, explicit
attachment via `plan.subgraph` / combinators — eliminates both the ambient
scope stack and the wrapper burden. Every `plan.*` call is a pure function
that produces an invocation; nothing attaches until the caller says so.

Prior-art lesson: `op.ExecutionContext` embeds `context.Context` as a single
shared value, which broke scoped cancellation when gather needed its own
cancel scope (see Phase 7 step 10). The fix threaded `context.Context` as a
parameter through the dispatch chain so each scope could derive its own
child. The same principle applies to plan-time scope: centralizing "the
current enclosing subgraph" in ambient state (the rejected scope stack)
invites the same class of bug. Every scope has to be a value that callers
pass explicitly — for cancellation, a `context.Context`; for planning, an
invocation.

## Goal

- Authors write combinator calls with invocation-passing syntax; no
  lambdas required for attachment.
- Containers (subgraph, choose branches, gather body, wait_until predicate)
  explicitly own their members, receiving invocations as args.
- Anything the author constructs but doesn't attach fails at plan time as
  an orphan — silent dead code is not tolerated.
- Type mismatches on Promise→slot bindings fail at plan time — runtime
  coercion errors are caught by a pre-flight pass.

Representative shapes:

```python
# Subgraph: bundle N invocations into one executable unit.
setup = plan.subgraph(
    plan.file.mkdir(path=dir),
    plan.file.write_text(destination=dir + "/hello", content="hi"),
)

# Choose: branches are invocations; detached until the matching case fires.
plan.choose(
    defaultValue=plan.complete(),
    plan.case(when=plan.service.is_healthy(svc="db"),
                   then=plan.complete(output="ok")),
    plan.case(when=plan.service.is_down(svc="db"),
                   then=plan.degraded("{{.svc}} unhealthy", svc="db")),
)

# Gather: body is an invocation parameterized by an iteration input.
paths = ["/tmp/log/a.txt", "/tmp/log/b.txt", "/tmp/log/c.txt"]
body = plan.subgraph(plan.file.write_text(destination=_item, content="hello"))
plan.gather(items=paths, body=body)

# WaitUntil: predicate is an invocation.
plan.wait_until(
    predicate=plan.service.is_healthy(svc="db"),
    timeout="5m",
    interval="10s",
)

# Entry point: explicit root.
plan.run(plan.subgraph(setup, ...))
```

## Design decisions

### D1 — Invocation shape

```go
package starlarkbridge

// Invocation is the value returned by every plan.* call. It represents
// a planned provider-method invocation that has not yet executed. Target
// is the op-level unit the invocation will dispatch; Result is the Promise
// to its output. FillSlot picks which field to use based on the target
// parameter's type at the binding site.
type Invocation struct {
    Target op.ExecutableUnit // the Node or Subgraph this invocation will dispatch
    Result *Promise          // value-side accessor: edge source for the invocation's output
}
```

For node invocations, `Target` is a `*op.Node` and `Result` points at
that node's output. For container invocations (subgraph, choose, gather,
wait_until), `Target` is the container's subgraph (or the combinator node
itself, per D3) and `Result` points at the container's defined output.

Invocations are created by `plan.*` dispatch methods, registered in the
session's `InvocationRegistry` (D6), and returned as the starlark value the
caller sees.

### D2 — Argument binding: target-type dispatch

`NodeBuilder.FillSlot` gains a case for `*starlarkbridge.Invocation`:

```
When slot.Parameter.Type implements op.ExecutableUnit (or is assignable to it):
    slot.Value = ImmediateValue{invocation.Target}
    No edge — the caller wanted a unit reference.

Else (target expects a value):
    edge from invocation.Result.node → consumer node
    slot.Value = PromiseValue{NodeRef: invocation.Result.node.ID(), Slot: invocation.Result.slot}
    Same behavior as today's *Promise case, but sourced from invocation.Result.
```

Starlark callers never distinguish "pass a unit" from "pass a value" — the
receiving method's Go parameter type determines the semantic.

In full detail, this replaces the existing `*Promise` case in `FillSlot` —
a Promise is now always carried inside an `Invocation`, so the old case
disappears.

### D3 — Container output conventions

Every container has a defined output. The container invocation's `Result`
points at whatever produces that output at execute time. Output type is
inferred from member types when the members are homogeneous; falls back
to `any` when heterogeneous.

| Container | Output value | Output type |
|---|---|---|
| `plan.subgraph(a, b, c)` | list of terminal values in topological order | `[]T` when all terminals return `T`; `[]any` otherwise |
| `plan.gather(items, body)` | list of per-iteration results in item order | `[]T` when body returns `T` (every iteration produces the same type by construction); `[]any` when body's return is `any` |
| `plan.choose(default, cases...)` | value of the chosen branch | `T` when default and every case's Then return `T`; `any` otherwise |
| `plan.wait_until(predicate, ...)` | predicate's final value | the predicate's return type; timeout surfaces as error through Action.Do's error channel |

**Rationale.** Binding a container invocation's `Result` to a consumer's
slot requires type compatibility. Inferring the narrowest accurate output
type maximizes what can be bound cleanly and what plan-time type
verification (D8) can catch. A heterogeneous subgraph — e.g., terminals
returning `string` and `int` — is legal but its output is `[]any`; the
consumer must either accept `[]any` or the plan-time type check rejects
the binding.

**Subgraph + gather are always list-typed.** Even with one terminal or
one iteration, the output is a one-element list. Authors destructure or
index when they want the scalar. Keeps the rule predictable and the
type-inference logic uniform.

**Choose's inferred type.** Homogeneous cases produce a narrow type;
heterogeneous (including the default) fall back to `any`. The narrowing
happens at the planner by inspecting every branch's return type.

**Type-check implications.** D8's type verification uses these inferred
types as the SOURCE side of each binding that consumes a container's
`Result`. A subgraph of `[]string` bound to a slot expecting `[]string`
passes; bound to `[]int` fails; bound to `[]any` passes via
assignability.

### D4 — Orphan detection

At plan-end (after all starlark evaluation completes, before execution
begins), walk the graph from the invocation passed to `plan.run(...)`.
Mark every reachable invocation by applying these rules until fixed-point:

- The root invocation is reached.
- If a container invocation is reached, every invocation that appears as
  a child of its container is reached.
- If an invocation is reached, every edge incident on its Target has both
  endpoints reached — specifically, any invocation whose `Result` is
  consumed by a value-typed slot on a reached invocation is itself
  reached (the source must run to produce the value the consumer needs).

Any invocation in the session's `InvocationRegistry` that is not reached
is an **orphan**. Each orphan is collected; after the full walk completes,
the collected orphan errors are joined with type-verification errors and
presented together at the end of `plan.run`'s pre-flight (see D5).

Rationale: silent dead code is the worst failure mode — the author
believes their invocation is in the graph but it isn't. There is no
discard escape hatch at present. Starlark's `_` is not a blank identifier
like Go's — `_ = plan.file.write_text(...)` is a regular variable binding
to a variable named `_`, indistinguishable from any other binding at the
planner's level. Authors who don't want an invocation in the graph
simply don't construct it. If a "build but don't run" use case emerges
(inspection, testing), a future API like `plan.discard(invocation)` can
add it explicitly — not speculatively.

### D5 — Explicit root via `plan.run(root)`

`plan` is a starlark namespace, not an object. Two categories of
attribute access route through it:

- **Domain providers** — `plan.file.*`, `plan.service.*`, `plan.archive.*`,
  etc. — `plan.<provider>.<method>(...)` dispatches a domain operation.
- **Planner primitives** — `plan.subgraph`, `plan.choose`, `plan.case`,
  `plan.gather`, `plan.wait_until`, `plan.complete`, `plan.degraded`,
  `plan.fatal`, `plan.elevate`, `plan.options`, `plan.run` — direct on
  the `plan` namespace, not nested under any provider. These names are
  reserved planner-side; domain providers cannot declare methods with
  these names.

There is no "plan object," no ambient root, no accessor for a default
graph. Every `plan.*` call is a pure function from args to an invocation
(with the sole exception of `plan.run`, which terminates planning).

`plan.run(...)` is the terminal primitive. It accepts variadic
invocations and creates the graph from them:

```python
plan.run(a, b, c)                 # variadic form; common case
plan.run(plan.subgraph(a, b, c))  # single-invocation form; the one big subgraph case
```

The variadic form is shorthand for `plan.run(plan.subgraph(a, b, c))` —
the runner wraps the variadic invocations in a subgraph when more than
one is passed. Passing a single already-subgraph invocation uses it
directly.

**Graph creation happens here, not before.** Until `plan.run` is called,
authors are dealing only with invocations (which reference nodes and
subgraphs that exist conceptually but have no graph instance to belong
to). `plan.run` materializes the `op.Graph`, installs its single
`*op.Subgraph` root populated from the passed invocations, runs the
plan-end pre-flight, and hands the graph to the tool-level runner.

**Pre-flight error aggregation.** The pre-flight pass does not fail
fast. It runs every check (orphan detection D4, topological sort,
type verification D8) and collects every violation it finds.
`plan.run` joins the collected errors via `errors.Join` and returns
one report at the end. Users see the complete picture — every orphan,
every type mismatch — on a single run, not a one-at-a-time
fix-rerun-fix loop. A pre-flight with any violations aborts execution;
a clean pre-flight hands the graph off to the runner.

`plan.run` is single-call per `.star` file; a second call is a plan-time
error. Multi-graph scenarios (running multiple graphs in sequence or
parallel from one file) are composed at the tool level, not inside one
starlark script.

**Storage.** The top-level `plan.Provider` gains a `root *Invocation`
field (actually a slice when the variadic form is used) set by the first
`plan.run` call and consumed by the tool runner after starlark evaluation
completes. Orphan detection and type-checking walk from the invocations
stored there.

### D6 — Invocation registry

```go
package starlarkbridge

type InvocationRegistry struct {
    mu      sync.Mutex
    ordered []*Invocation          // creation order; used for deterministic iteration
    byLabel map[string]*Invocation // label → invocation; used for lookup and orphan reporting
    counts  map[string]int         // <provider>.<method> → next ordinal for auto-labeling
}

// Register appends inv to ordered and inserts it into byLabel under the
// given label. Duplicate labels (user-supplied collisions) are plan-time
// errors.
func (r *InvocationRegistry) Register(label string, inv *Invocation) error

// AutoLabel returns "<providerMethod>#<N>" where N is the next 1-based
// ordinal for providerMethod, incrementing the per-providerMethod counter.
// Callers use this when Options.Label is empty.
func (r *InvocationRegistry) AutoLabel(providerMethod string) string

// All returns every registered invocation in creation order. Used by the
// plan-end orphan pass and the type-check pass.
func (r *InvocationRegistry) All() []*Invocation

// ByLabel returns the invocation registered under label, or nil if no
// such invocation was registered.
func (r *InvocationRegistry) ByLabel(label string) *Invocation
```

Owned by the top-level `plan.Provider` (the unified planner; see step 5).
Every `plan.Provider.dispatch` call registers the invocation it constructed
before returning it to the starlark caller. Child `plan.Provider` instances
for sub-namespaces share the registry with the top-level via pointer.

Writes happen only during planning. Reads happen during planning (orphan
walk, type-check walk) and at execute time (if lookup by label is ever
needed — probably not, but the data is available).

### D7 — Invocation options (label, retry policy)

Cross-cutting invocation concerns — currently the label and the retry
policy — are supplied via a single reserved kwarg `options` that accepts
a value built by `plan.options(...)`. A single reserved name keeps the
planner's kwarg surface tight; fields on the options value are
free to grow without claiming more kwargs.

```python
plan.file.write_text(
    destination=path,
    content=text,
    options=plan.options(label="write-config", retry_policy=plan.retry.exponential(max_attempts=3)),
)

plan.subgraph(a, b, c, options=plan.options(label="setup"))

plan.gather(items=xs, body=body, options=plan.options(retry_policy=linear))
```

**Go-side representation.**

```go
package starlarkbridge

// Options collects plan-time-settable, cross-cutting concerns that apply
// uniformly to every invocation. Zero values mean "use the default":
// auto-generated label, no retry policy.
type Options struct {
    Label       string           // empty → auto-generated default label
    RetryPolicy *op.RetryPolicy  // nil → no retry
}
```

**Reserved kwarg: `options`.** Provider methods cannot declare a
parameter named `options`. Enforced at method registration (where
`parameters []string` is built in `receiver_type.go`) — any provider
that declares it fails program init with a clear message. Same treatment
applied to `*args` and `**kwargs`.

**Dispatch flow.** The planner's generic dispatch path (the code that
routes every `plan.*` call) intercepts the `options` kwarg before
passing the remaining kwargs to the method. Effective options are
applied to the constructed `Invocation`:

- `options.Label` supplied → registered under that label; auto-label
  skipped.
- `options.Label` empty → auto-labelled `<provider>.<method>#<N>` where
  N is the creation-order ordinal for that provider.method combination.
- `options.RetryPolicy` supplied → applied to the underlying Node or
  Subgraph (same hook as today's `Promise.retry` builtin).
- `options.RetryPolicy` nil → no retry.

Label collisions (user-supplied vs. user-supplied, or user-supplied vs.
auto-generated) are plan-time errors with a message naming both call
sites.

**Auto-labeling.** Format depends on the source provider's `root` flag
(D12). Non-root providers — file, git, service, archive, …, and every
sub-namespace under `plan` — use the qualified form
`<provider>.<method>#<N>`. Root-planned providers — flow.Provider in
this phase — drop the provider segment and use `<method>#<N>` because
their starlark surface already omits the sub-namespace and their
method names are reserved planner-side:

```
file.write_text#1
file.write_text#2
file.mkdir#1
choose#1
subgraph#1
service.is_healthy#1
```

Derivation: the dispatch site knows the source receiver type and
method name. It queries `receiverType.IsRoot()` to pick the label
form. A per-method counter in the `InvocationRegistry` yields the
ordinal. Monotonic within a `.star` evaluation; deterministic across
runs of the same script.

**Rejected alternatives** for the overall mechanism:
- **Individual reserved kwargs** (`label="…"`, `retry_policy=…`):
  every cross-cutting concern claims another reserved name; grows the
  planner's kwarg surface over time.
- **Fluent API** (`.label().retry_policy()`): if the initial dispatch
  registered under auto-label, fluent chains either mutate in place
  (violates I2) or create new `Invocation` copies that re-register
  under new labels (registry contains duplicates pointing at the same
  Target/Result — confusing for orphan detection and collision
  checking).
- **Decorator function** (`plan.create(inv, label=..., retry_policy=...)`):
  two-step construction; adds ceremony for the common case where users
  accept the default label.
- **Construction + mutation** (`inv.label = "name"`): explicit mutation
  of an Invocation after construction; violates I2 and I3.
- **Context-manager scope** (`with plan.retry(policy): …`): starlark
  has no `with` construct.

**Rejected alternatives** for the label format specifically:
- **Monotonic global** (`unit-1`, `unit-2`): opaque; gives no hint about
  what the invocation is.
- **Source-position-based** (`file.write_text@manifest.star:42`):
  fragile under refactors; labels shift whenever lines move.
- **Content-hash labels**: deterministic-by-args, enables caching, but
  unreadable and overkill for the current scope.

### D8 — Plan-time type checking

Every Promise→slot binding carries a type relationship: the slot's
parameter type (target) must accept the Promise's source-node output type
(source). `op.Convert` performs the runtime cascade; plan-time checking
answers "could Convert succeed?" without a value.

The per-type "can I convert to this target?" answer lives on the
`Converter` interface (D9). The Planner orchestrates the overall
cascade — it owns the walk over slot bindings, delegates the per-type
decision to `Converter.CanConvert` where applicable, and enforces the
fail-at-plan-time contract.

```go
package starlarkbridge

// CanConvertTypes answers whether a source type can be converted to a
// target type under the current registry. Mirrors op.Convert's runtime
// cascade at the type level. The per-type decision for Converter-
// implementing source types delegates to Converter.CanConvert; other
// steps are answered via reflect.Type alone.
func (p *Planner) CanConvertTypes(source, target reflect.Type) bool {
    if source == target {
        return true
    }
    if source.AssignableTo(target) {
        return true
    }
    if source.Implements(converterType) {
        zero := reflect.Zero(source).Interface().(op.Converter)
        return zero.CanConvert(target)
    }
    if rt, ok := p.graph.ExecutionContext().Registry.TypeByReflection(target); ok {
        if _, isResource := rt.(op.ResourceReceiverType); isResource {
            return true
        }
    }
    if target.Kind() == reflect.Ptr {
        if rt, ok := p.graph.ExecutionContext().Registry.TypeByReflection(target.Elem()); ok {
            if _, isResource := rt.(op.ResourceReceiverType); isResource {
                return true
            }
        }
    }
    if source.Kind() == reflect.Slice && target.Kind() == reflect.Slice {
        return p.CanConvertTypes(source.Elem(), target.Elem())
    }
    return false
}
```

**`reflect.Zero(source).Interface().(op.Converter)`.** Plan-time type
check calls `CanConvert` on a zero value of the source type. Converter
implementations must be callable on zero receivers — no dereferencing,
no field access, pure type logic. This is a documented contract of the
`Converter` interface (D9).

**Plan-end pass ordering.** Runs after starlark evaluation completes, in
this order:

1. **Orphan detection** (D4). Walk from `plan.run`'s root; mark
   reachable invocations; error if any registered invocation is
   unreached.
2. **Topological sort.** Order the graph so type verification can walk
   edges in producer-before-consumer order.
3. **Type verification.** Walk every slot that holds a `PromiseValue`
   in topological order. For each:

```
source = slot's Promise source node's output type (inferred per D3 for
         container sources).
target = slot.Parameter.Type.
If !p.CanConvertTypes(source, target):
    error: "cannot bind <source-label> output to <consumer-label> slot %s
           (have %s, want %s)", slot.Name, source, target
```

Every type-mismatch is collected during the walk and joined with
orphan-detection errors at the end of pre-flight (see D5). No ill-typed
edges reach execution; users see every mismatch in a single report.

### D9 — `CanConvert` method on `op.Converter`

The `Converter` interface (Phase 7 step 8) gains a required type-level
predicate:

```go
package op

type Converter interface {
    Convert(target reflect.Type) (any, error)
    CanConvert(target reflect.Type) bool
}
```

Every type that implements `Converter` must implement `CanConvert`. The
method answers "can I, as a source value of my type, convert to this
target type?" without performing the conversion or any I/O.

**Nil-safety contract.** `CanConvert` is invoked by the Planner at
plan-time on a zero value of the source type
(`reflect.Zero(source).Interface().(Converter)`). Implementations must
not dereference the receiver or access fields. The method answers on
TYPE information alone — the receiver is present only to satisfy the
interface-method-call mechanism.

**Runtime use.** `op.Convert` calls `c.CanConvert(target)` before
`c.Convert(target)` as a lookahead. If `CanConvert` returns false,
`Convert` is skipped (no cost, no side effects). If it returns true,
`Convert` runs and may still error for a specific reason (e.g., an
actual I/O failure that the type-level check couldn't predict) — but
type-mismatch errors are ruled out by construction.

**Plan-time use.** The Planner's `CanConvertTypes` method (D8)
delegates the Converter-branch of its cascade to `CanConvert`. The
decision at plan time is final — there's no "optimistic trust" gap —
because `CanConvert` is required to be accurate on type information.

### D10 — Empty containers

A container without any operations is a plan-time error at the call
site. The rule applies uniformly to every grouping combinator — there is
no meaningful container that does nothing.

| Container | Empty-when | Error |
|---|---|---|
| `plan.subgraph(...)` | no invocations passed | "subgraph must contain at least one invocation" |
| `plan.choose(default, ...)` | no cases passed | "choose must declare at least one case" |
| `plan.gather(items, body, ...)` | no `body` | "gather requires a body invocation" |
| `plan.wait_until(predicate, ...)` | no `predicate` | "wait_until requires a predicate invocation" |

Items-empty gather is **not** an error — a gather over zero items is a
valid no-op iteration (the body never runs) and returns `[]any{}`. The
rule targets missing WORK, not missing ITEMS.

Rationale:
- An empty container has no work and no output; downstream consumers of
  its invocation have nothing meaningful to bind.
- Authors who want conditional contents build the arg list in starlark:
  `plan.subgraph(*([a, b] + ([c] if cond else [])))`.
- A mutable builder pattern (`plan.subgraph_builder()` → `b.add(...)` →
  `b.done()`) is not adopted; it conflicts with the functional,
  pure-plan-time model (invariant I2).

Empty-container errors are collected and joined with the rest of
pre-flight via D5's aggregation — users see every violation on a single
plan.run attempt, not one at a time.

### D11 — Migration of existing `.star` callers

Existing callers of the old Choose/Gather APIs migrate to the
invocation-passing form:

- `cmd/devlore-test/devloretest/data/test_is_*.star` — rewrite from
  `plan.choose(when=..., then=...)` kwargs form to the invocation-
  passing form with `plan.case(...)` members.
- `pkg/op/provider/plan/gen/*` and `pkg/op/provider/flow/gen/*` —
  regenerate against the plan/flow split (D12) as each combinator
  redesign lands. flow.Provider's generated files come from the
  resurrected `pkg/op/provider/flow/` package with `+devlore:root=true`.
- Any `.star` doc snippets showing Choose/Gather call sites — update in
  place.

Each step that lands a combinator redesign includes its migration as
part of that step's PR.

**Deferred for now:**

- **Codegen template changes.** The current codegen templates emit the
  planner bridge under the old model. Instead of predicting what
  templates need to look like under the new model, we address template
  updates as each combinator redesign surfaces them — reactive rather
  than speculative.
- **`devlore-registry` and lore packages.** The `devlore-registry` repo
  and every lore package consuming this API will need a rewrite against
  the new planner surface (invocations, options kwarg, plan.run entry
  point, new Choose/Gather/Subgraph/WaitUntil shapes). That migration
  is a separate cross-repo effort tracked outside this phase. Phase 8
  lands the new API in this repo; downstream repos migrate in their
  own time.

### D12 — Root providers

The plan namespace hosts two categories of methods that behave
differently: cross-cutting metadata builders and lifecycle operations
run immediately as ordinary starlark calls (`plan.options`,
`plan.case`, `plan.run`, `plan.load`, `plan.save`), and planner
primitives that construct graph nodes for deferred execution
(`plan.choose`, `plan.gather`, `plan.subgraph`, `plan.wait_until`,
`plan.complete`, `plan.degraded`, `plan.fatal`, `plan.elevate`). These
two categories want the same starlark surface (flat under `plan`) but
different Go-side dispatch models. A single provider struct cannot
carry both cleanly without introducing per-method access annotations
that complicate every downstream consumer.

The split: the two categories live on two separate provider structs.

- `pkg/op/provider/plan/` — `plan.Provider`, tagged
  `+devlore:access=immediate` (no `root` directive; defaults false).
  Methods: `Options`, `Case`, `Run`, `Load`, `Save`. Registered as
  the top-level starlark global keyed `"plan"`.
- `pkg/op/provider/flow/` — `flow.Provider`, tagged
  `+devlore:access=planned` and `+devlore:root=true`. Methods:
  `Choose`, `Gather`, `Subgraph`, `WaitUntil`, `Complete`, `Degraded`,
  `Fatal`, `Elevate`. Not registered as a top-level starlark global;
  its methods surface flat under `plan` via the peer dispatch
  mechanism described below.

**`+devlore:root=true` directive.** A new struct-level directive
parsed by `generate.star` and threaded through codegen. Orthogonal to
`+devlore:access=`; composes with either value. The access × root
semantic table:

| `access` | `root` | Starlark surface | Dispatch | Action name | Auto-label |
|---|---|---|---|---|---|
| `immediate` | false (default) | `<provider>.<method>(...)` | immediate execution | N/A | N/A |
| `immediate` | true | `<method>(...)` — top-level global | immediate execution | N/A | N/A |
| `planned` | false (default) | `plan.<provider>.<method>(...)` | graph-node-creating | `<provider>.<method>` | `<provider>.<method>#<N>` |
| `planned` | true | `plan.<method>(...)` — flat on plan root | graph-node-creating | `<method>` | `<method>#<N>` |

Only the `planned + root=true` row is exercised in Phase 8 (by
flow.Provider). The `immediate + root=true` row is defined for
symmetry; no Phase 8 provider uses it.

**Root flag folded into `ProviderRole` as a placement-zone bit.**
Rather than adding a separate `IsRoot() bool` method to
`ProviderReceiverType`, the root directive is represented by a new
bit on the existing `ProviderRole` bitflag. The bit grammar is
partitioned into two zones:

- **Dispatch zone** (bits 0–7) — declares how the provider's methods
  are invoked. At least one bit must be set. Current bits:
  `RoleModule` (immediate), `RoleAction` (planned). Bits 2–7
  reserved for future dispatch modes.
- **Placement zone** (bits 8–15) — modifies where the provider's
  methods surface. Orthogonal to the dispatch zone; optional. First
  bit: `RoleRoot`. Bits 9–15 reserved for future placement modifiers.

```go
type ProviderRole uint

// Dispatch zone — bits 0–7.
const (
    RoleModule ProviderRole = 1 << iota
    RoleAction
    // bits 2–7 reserved
)

// Placement zone — bits 8–15.
const (
    RoleRoot ProviderRole = 1 << (iota + 8)
    // bits 9–15 reserved
)

// Zone masks.
const (
    roleDispatchMask  ProviderRole = 0x00FF
    rolePlacementMask ProviderRole = 0xFF00
)

func (r ProviderRole) Dispatch() ProviderRole  { return r & roleDispatchMask }
func (r ProviderRole) Placement() ProviderRole { return r & rolePlacementMask }
```

`AnnounceProvider` validates that `roles.Dispatch() != 0` at
announcement time — a placement bit without a dispatch bit is a
panic-level misconfiguration. The 27 existing generated
`AnnounceProvider` call sites are untouched; only flow.Provider's
future call site composes `RoleAction|RoleRoot`.

**`ReceiverRegistry.RootProviders()`.** `op.ReceiverRegistry` gains a
general `RootProviders() []ProviderReceiverType` method that returns
every registered provider whose `Roles().Placement()&RoleRoot != 0`.
Callers filter by dispatch zone as needed; `plan.Provider` filters
to `RoleAction` at construction to discover its peers. No new
interface method on `ProviderReceiverType` — the existing `Roles()`
method already carries the info.

**`StarlarkRuntime` registration (`pkg/op/starlarkbridge/runtime.go`
`NewStarlarkRuntime`).** The module-iteration loop branches on the
access × root combination:

- `access=immediate, root=false` → register the provider as a
  top-level predeclared global under `prt.Name()`. Status quo for
  pkg, archive, template, plan (plan is immediate-non-root — it
  registers as the `"plan"` global).
- `access=immediate, root=true` → iterate the provider's methods;
  install each as its own top-level predeclared entry. The provider
  instance is not itself exposed to starlark. Reserved for future use.
- `access=planned, root=false` → do NOT register as a top-level
  global. The provider is reached via `plan.<name>.<method>` through
  plan.Provider's sub-namespace dispatch. Status quo for file, git,
  service, pkg, archive, encryption.
- `access=planned, root=true` → do NOT register as a top-level
  global and do NOT register as a plan sub-namespace. plan.Provider
  discovers the provider via `registry.RootProviders()` and hosts
  its methods flat under its own `Attr` resolution.

**`plan.Provider` three-tier `Attr` resolution.** Construction-time
`plan.Provider` builds a merged dispatch table:

1. Tier 1 — `plan.Provider`'s own methods (`options`, `case`, `run`,
   `load`, `save`). Immediate dispatch.
2. Tier 2 — every `access=planned, root=true` provider's methods,
   queried from `registry.RootProviders()` filtered to planned. In
   Phase 8 this is exactly flow.Provider (`choose`, `gather`,
   `subgraph`, `wait_until`, `complete`, `degraded`, `fatal`,
   `elevate`). Planned dispatch routed to the peer provider instance.
3. Tier 3 — sub-namespace children for every non-root planned
   provider, keyed by the provider's Go name (`file`, `git`,
   `service`, …). Returned as child `*plan.Provider` values so
   nested starlark lookups `plan.file.write_text` resolve to the
   child's planned dispatch.

`Attr(name)` walks Tier 1, then Tier 2, then Tier 3, returning the
first match. Misses return `nil, nil`.

**Collision detection at construction.** When `plan.Provider` builds
the Tier 1+2 merged map, any method name appearing more than once
across (plan.Provider, flow.Provider, any future root-planned
provider) fails construction with an error of the form:

```
plan namespace: method "choose" declared on both
  flow.Provider (access=planned, root=true) and
  plan.Provider (access=immediate)
```

The same treatment applies when a Tier 3 child provider's Go name
collides with a Tier 1 or Tier 2 method name. Example: a future
non-root planned provider named `choose` would collide with
flow.Provider's `Choose` method; the plan.Provider constructor would
refuse to start. The error includes both offenders.

**Why a new directive rather than per-method access?** An earlier
sketch proposed per-method `+devlore:access=` to let plan.Provider
host both immediate and planned methods on one struct. The split
here trades one new struct-level directive for a clean separation of
concerns: each provider holds a single axis. Codegen stays uniform
(struct-level directive drives every generated method); flow.Provider
is a regular provider with a regular receiver type. The peer
relationship is discoverable from metadata (the `root` flag), so no
ad-hoc knowledge of "plan's peers" lives in either provider's code.

**Why a single `plan` namespace root?** Phase 8 has exactly one
flattening root. The directive does not take a target argument
(e.g., `+devlore:root=plan`) because no second root is planned. If a
second root emerges later, the directive extends to name its target
then — not speculatively now.

### D14 — Execution-state snapshot type named `Trace`

The serializable projection of a [*GraphExecutor]'s per-run mutable state is named `Trace`. The
in-flight scaffolding from step 18's graph-immutability work added a `Snapshot` type in
`pkg/op/snapshot.go` with `GraphExecutor.Snapshot()` and a `ResumeExecutor(graph, spec, snapshot)`
constructor; the rename lands as part of step 18:

- `pkg/op/snapshot.go` → `pkg/op/trace.go`
- `type Snapshot struct` → `type Trace struct`
- `(e *GraphExecutor) Snapshot() *Snapshot` → `(e *GraphExecutor) Trace() *Trace`
- `ResumeExecutor(graph, spec, snapshot *Snapshot)` → `ResumeExecutor(graph, spec, trace *Trace)`

**Role.** A `Trace` is the captured-state record of one execution of a Graph. It pairs with the
immutable [*Graph] (which carries the plan) to fully describe an execution: the Graph is *what was
meant to happen*; the Trace is *what did happen* — plus enough state to continue when execution
was paused. The two artifacts serialize independently and round-trip together.

The two top-level serializable artifacts of the framework, then, are:

- **`*Graph`** — immutable, run-many workflow definitions.
- **`*Trace`** — `GraphExecutor` snapshots, used for restarts, dependency analysis, drift
  detection, audit / forensics, and other post-execution analyses.

**Use cases.**

- **Restart.** A Trace in [RunStatePaused] carries the [*RecoveryStack], resolved variables, and
  active activation frames needed for a fresh executor to continue from where the prior run
  stopped via [ResumeExecutor].
- **Dependency analysis.** Each receipt's `UnitID`, slots snapshot, and complement form a
  per-dispatch span; walking the Trace reconstructs the execution-time dependency graph (which
  units depended on which, in what order).
- **Drift detection.** A Trace records what was observed at execution time; comparing it to fresh
  observations of the same resources reports drift (configuration changes, missing files, modified
  content).
- **Audit / forensics.** Every dispatch's outcome, inputs, and recovery state is permanently
  captured; the Trace is the audit log.

**Rationale for the name.** `Trace` matches the dominant modern usage in distributed tracing,
observability, replay, and dependency-analysis contexts. It covers the multi-use-case reality
without committing to any single role — `Snapshot` suggests only restart; `Journal` leans
persistence-flavored; `Witness` is poetic but unfamiliar. It also reads naturally in code:
`executor.Trace()`, `trace.MarshalJSON()`, `op.LoadTrace(...)`.

**Consequence for `internal/cli/receipts.go`.** The whole concept of "receipts" (per-tool YAML
files containing a stamped-and-signed Graph) is obsolete under this model — what gets serialized
is a `*Graph` and a `*Trace`, independently, not a tool-flavored receipt wrapping both. The
package is removed; tool-side wrappers (filename formatting, scope-aware paths, SOPS signing)
move to `cmd/writ` and `cmd/lore` directly. See D13's framework / tool separation discussion.

### D15 — Origin is plan-time-written, tool-read graph identity and context

Renames the tool-stamped graph metadata type (formerly `Provenance`, briefly considered `Imprint`)
to `Origin`. Pragmatic, cloud-native term; modern infrastructure teams reach for "origin" when they
mean "who, what, when, where this artifact came from." "Provenance" read academic; "Imprint" leaned
literary. `Origin` is direct.

Origin captures the tool's stamp, publisher context, and creation environment (e.g., tool, scope,
source root, target root, commit hashes, dirty flag, layers, packages, features) directly on the
graph.

Clarified architectural boundaries: this sits strictly above the graph's internal structural
content (nodes, subgraphs, edges). It is explicitly not a manifest or inventory, but rather an
immutable record of who produced the graph and under what conditions, accompanying the artifact
throughout its lifecycle. Plan-time-written by tools, tool-read at runtime and beyond, never
inspected by the framework.

### D16 — Catalog: single creator (`NewRuntimeEnvironment`), spec-supplied seed

One env, one catalog. `NewRuntimeEnvironment` is the single creator. To inject a pre-built catalog
(the `GraphExecutor.Run` clone case), callers pass it via `RuntimeEnvironmentSpec.WithCatalog`.

**Creation rules:**
- `NewRuntimeEnvironment(ctx, spec)` always assigns `env.Catalog`. If `spec.Catalog` is non-nil,
  it is used as-is; otherwise a fresh empty `*ResourceCatalog` is created.
- No other call site creates a `*ResourceCatalog`. Not `plan.Provider.NewProvider`, not the tool
  bootstraps (`cmd/lore`, `cmd/star`, `cmd/writ`), not anywhere else.

**Lifecycle stewardship (separate from creation):**
- `plan.Provider` operates on `env.Catalog` during planning — interning resources via provider
  method dispatch through the activation chain. It does not create the catalog; it mutates the
  one env was born with.
- `GraphExecutor.Run` constructs the per-run env with the spec extended via
  `WithCatalog(graph.ResourceCatalog().Clone())`. The per-run env is born with the cloned
  catalog; the executor then operates on it during dispatch.

**Why a single creator matters:** if multiple call sites created catalogs, a flow that constructs
both an orchestrator-side catalog and (later) a `plan.Provider`-side catalog in the same env
would clobber the first with an empty second, losing every resource interned in between. Single
creator, in `NewRuntimeEnvironment`, eliminates the risk by construction.

**Why this works without structural changes:** the analysis traced every direct access to
`env.Catalog`. The starlark bridge does not touch `env.Catalog` (only `env.Registry`). Providers
do not touch `env.Catalog` via the receiver path. Resources only touch `env.Catalog` indirectly,
through package-level constructors (`NewResource`, `DiscoverResource`) that go through the
activation's env reference. Receipt-hydrate paths reach it the same way. Every reader already has
env in scope; nothing needs replumbing.

**Type definitions unchanged.** `RuntimeEnvironment.Catalog *ResourceCatalog` stays as a field.
`ActivationRecord` is structurally unchanged — no `Catalog` field added.

**Net surface changes:**
- `RuntimeEnvironmentSpec` gains a `Catalog *ResourceCatalog` field and a `WithCatalog(catalog)`
  builder method.
- `NewRuntimeEnvironment` consults `spec.Catalog` first; falls back to a fresh
  `NewResourceCatalog()` when nil.
- `GraphExecutor.Run` uses `spec.WithCatalog(graph.ResourceCatalog().Clone())` instead of the
  post-construction assignment that existed before.

#### D16(b) — Option B: provider resource constructors take `runtimeEnvironment` (and `unit`) directly

The catalog-lifecycle landing surfaced a downstream issue: every provider's `NewResource` and
`DiscoverResource` accepted `*ActivationRecord` as their context-bearing first argument, even though
the only fields they read off it were `RuntimeEnvironment` (for the Catalog) and `Unit` (for the
producer stamp). Call sites that had no real activation were synthesizing one inline as
`op.NewActivationRecord(nil, nil, env)` — a strong smell that the abstraction was leaking.

**New signatures:**
- `provider.NewResource(runtimeEnvironment *op.RuntimeEnvironment, unit op.ExecutableUnit, value any) (*Resource, error)`
- `provider.DiscoverResource(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error)`
- `ResourceCatalog.GetOrCreate(unit op.ExecutableUnit, uri string, factory func() (Resource, error)) (Resource, error)`

`NewResource` stamps `unit.ID()` as the catalog entry's `producerID`; `DiscoverResource` records
no production claim. Tests that need an empty stamp pass `nil` for `unit`.

**Sweep landed:**
- All 9 provider `resource.go` files (file, function, json, yaml, service, git, mem, appnet, pkg).
- `pkg/op/provider/file/provider.go` — every `NewResource(activationRecord, X)` site rewritten to
  `NewResource(p.RuntimeEnvironment(), activationRecord.Unit, X)`; every synthetic-activation
  `DiscoverResource(...)` call simplified to pass the `runtimeEnvironment` local directly.
- `pkg/op/provider/{json,yaml,git}/provider.go` — one `NewResource` call site each.
- Receipt hydrate paths in `encryption`, `file`, `service`, `git`, `pkg` — the synthetic
  `op.NewActivationRecord(nil, nil, ctx)` wrapper inside `Receipt.hydrate` removed.
- `pkg/op/provider/archive/provider.go` (`Extract`) and `pkg/op/provider/encryption/provider.go`
  (`DecryptSopsFile`) — call-site updates for the new signatures.

**Generator changes:**
- `star/extensions/com.noblefactor.devlore.Actions/templates/resource.gen.go.template` — the
  `AnnounceResource` registration closure now calls `DiscoverResource(runtimeEnvironment, identity)`
  directly (no synthetic activation), and renames the closure parameter `ctx` →
  `runtimeEnvironment`.
- `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star` — `_resource_return_type`
  now requires the candidate function name to be exported; this rejects the package-private
  `buildCandidate` helper that shares `DiscoverResource`'s signature shape, eliminating the
  "multiple constructors found" ambiguity the Option B refactor introduced.
- `make generate` regenerates all 10 `provider/<X>/gen/resource.gen.go` cleanly under the new
  template + detector.

**Naming:**
- `ResourceConstructor` and `ProviderConstructor` (in `pkg/op/receiver_type.go`) now spell out
  `runtimeEnvironment` in their formal parameter — `ctx` was a magnet for the `context.Context`
  Go idiom even when it was actually carrying a `*RuntimeEnvironment`.
- Test files sweep-renamed `ctx` → `runtimeEnvironment` in 16 `provider/<X>/{provider,resource}_test.go`
  files. `newTestCtx` test-helper function names retained (lowercase-c `ctx` substring not matched
  by word-boundary rule); they can be renamed in a follow-up if desired.

**Status of `ActivationRecord.RuntimeEnvironment`:** the field is **retained**. See D16(c) below for
the reasoning chain that withdrew Option A.

#### D16(c) — Option A (remove `ActivationRecord.RuntimeEnvironment`) withdrawn

Option B narrowed the field's role on the constructor path but didn't justify keeping it
elsewhere. We worked through "should the field be removed entirely?" and the answer is **no**, for
a reason that's stronger than convenience: it's a GC-amortization invariant baked into the
layering, not a coincidence of how the code is currently written.

**The four layers and their reasons to exist.**

| Layer | Per-instance state | Required lifetime |
|---|---|---|
| `RuntimeEnvironment` | Yes — `Catalog`, `RecoverySite`, `Hooks`, `Status`, cancellation `Context`, `cachedProvider` map | Session |
| `Provider` | No (just a handle on env via `ProviderBase.runtimeEnvironment`) | Could be transient, but isn't (see GC argument) |
| `Action` | No (process-singleton in the receiver-type registry, holds `receiverType`/`method`/`name` only) | Process |
| `ActivationRecord` | No (per-call bundle of `Graph`, `Unit`, `Stack`, `Variables`, `Context`, and `RuntimeEnvironment`) | Dispatch |

**Why Action can't carry env.** Actions are registered at init time via the receiver-type
registry — one instance per `(receiverType, method)` pair, shared across every session in the
process. Putting env on Action would force per-session Action instances and a per-session
registry, eliminating the singleton property and rebuilding the dispatch table on every run. Big
restructure, no benefit.

**Why ActivationRecord *is* where env belongs at dispatch time.** Action is env-agnostic but
needs to find "the Provider for *this* session" at every `Do` / `Undo` call. The current chain is
`activationRecord.RuntimeEnvironment.cachedProvider(a.receiverType)`. ActivationRecord is the
per-dispatch carrier of every other dispatch-scoped concern (Stack, Variables, Context, Unit) —
making env an exception by threading it as a separate parameter on `Action.Do` / `Action.Undo`
would widen the signatures across every Action implementer, every test fixture, and every
executor call site, while gaining nothing the field doesn't already give us.

**Why the provider cache earns its keep (the GC argument).** A naive read of `NewProvider` —
`return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}` — suggests caching is
over-engineering: one allocation, one assignment. Per-call cost is sub-microsecond. The argument
breaks at scale: a non-trivial plan dispatches hundreds-to-thousands of actions per session, and
Go's GC cost is dominated by mark/scan work whose frequency scales linearly with allocation rate.
The canonical formula from the [Go GC guide][gc-guide] is:

> `GC frequency = (Allocation rate) / ((Live heap + GC roots) * GOGC / 100)`

…and "most of the CPU cost of the GC is marking and scanning, which is captured by the marginal
cost. […] more pointers means more GC work, because at minimum the GC needs to visit all the
pointers in the program." Per-dispatch Provider construction would multiply allocation rate by
the dispatch count and inflate the marginal mark/scan cost for every concurrent collection cycle,
producing CPU churn and latency spikes that don't correlate with any single Provider being
expensive. Go 1.26's [Green Tea GC][green-tea] explicitly targets "marking and scanning small
objects" — confirming this WAS a recognized pain point even after a decade of GC tuning —
and lands a 10–40% reduction in GC overhead on programs that pressure it. The *pattern* (cache
to avoid sustained small-object allocation pressure) is still the right move; Green Tea makes it
cheaper to violate, not unnecessary.

The cache turns N allocations per session into 1 per `(env, type)`. That's not micro-optimization
— it's the difference between "GC is invisible" and "GC is your bottleneck under load."

**The forced chain.**

1. Provider cache must live at **session lifetime** (GC amortization argument). ActivationRecord
   is per-dispatch — wrong scope. RuntimeEnvironment is the natural home.
2. Action must look up its Provider by current env at every dispatch (Action is process-singleton,
   Providers are per-env).
3. ActivationRecord is constructed fresh per dispatch by the executor — exactly the boundary
   where the live env gets stamped onto the call context.
4. Therefore: **`ActivationRecord.RuntimeEnvironment` is the bridge from the env-agnostic
   registry to the env-specific cache, and it's structurally load-bearing.**

This also reinforces Graph immutability: the saved Graph never embeds env. Fixup happens at the
dispatch boundary by stamping the live RuntimeEnvironment onto a fresh ActivationRecord. The
field is the carrier that makes "load a Graph on another machine, bind to local env, dispatch"
work without mutating the Graph.

**Sources (verified 2026-05):**

- [gc-guide]: <https://go.dev/doc/gc-guide> — official Go GC guide. Cites the GC-frequency formula
  and the mark/scan cost model.
- [green-tea]: <https://www.infoworld.com/article/4131097/go-1-26-unleashes-performance-boosting-green-tea-gc.html>
  — Go 1.26 release coverage; 10–40% GC overhead reduction targeted at small-object marking.
- Supporting context on small-object allocation pressure and reuse patterns:
  <https://goperf.dev/01-common-patterns/gc/>, <https://medium.com/@jedwaltondev/deep-dive-into-gos-garbage-collector-tuning-memory-reducing-gc-pauses-e00c409f1d39>.

## Open discussions blocking phase-8 closure

### O1 — Marshaling design: argument-to-parameter-type matching

**Direction (stated by user):** marshaling is driven by the
ReceiverType of the Go method argument. Every Go type that can
appear as a method argument has a registered ReceiverType; that
ReceiverType owns the marshaler for its type. Given a provider
method whose parameter is typed `T`, the pipeline looks up the
ReceiverType for `T` and asks it to produce a `T` from whatever
starlark source the caller supplied. One lookup, one registry, no
source-first dispatch, no fallback stage.

**What this replaces.** The current two-stage pipeline — source-
first `starlarkbridge.Unmarshaler` dispatch (`pkg/op/starlarkbridge/unmarshaler.go:30`)
followed by `op.Convert` fallback (`pkg/op/starlarkbridge/node_builder.go:418`) —
is the wrong shape. It matches on source first and reaches the
target through a fallback path; the target type authority is
secondary. Under the stated direction that whole pipeline is
replaced by a single target-driven lookup hosted on ReceiverType.

**What this means for `pkg/op/starlarkbridge/unmarshal_*.go`.** Those files
(`unmarshal_bool.go`, `unmarshal_int.go`, `unmarshal_string.go`,
`unmarshal_function.go`, …) each handle one starlark source type.
Under the new direction they disappear as a source-first registry.
Their per-source projection logic migrates into the ReceiverType
that owns each target Go type (or its factory). The `ToUnmarshaler`
dispatcher goes away; `starlarkbridge.Unmarshaler` as an interface goes away
or re-appears reshaped.

**Open questions to close before D13.**

1. **Marshal method shape.** Does ReceiverType gain a method like
   `Marshal(ctx *ExecutionContext, source any) (any, error)`,
   taking a generic `any` source? Or a different signature? The
   method cannot take `starlark.Value` directly because
   ReceiverType lives in `pkg/op` and `pkg/op` does not import
   starlark — that boundary stays.

2. **Ctx flow.** Several projections need ExecutionContext: resource
   construction (file.Resource from a string path requires Root),
   mem.Function construction (requires Thread for compile and
   program Init). Ctx threads through `Marshal`. Confirm.

3. **Compound target types.** A method parameter typed
   `func(any) (bool, error)` is not announced — there's no
   `AnnounceProvider`/`AnnounceResource` entry for function types.
   `TypeByReflectionOrDerive` handles unregistered struct types
   today; the equivalent for function types needs to exist and
   needs to know to route through `*mem.Function` (i.e., the
   starlark→mem.Function projection, then mem.Function.Convert
   to the target func type). Similarly for slices, maps, pointers
   to structs, etc. — the derivation rule per compound kind.

4. **Source type admission.** The ReceiverType for `string` needs
   to accept `starlark.String` as a source. The ReceiverType for
   `*file.Resource` needs to accept a starlark string (representing
   the path). The ReceiverType for `*mem.Function` needs to accept
   a `*starlark.Function`. How does each ReceiverType express
   which source shapes it handles? Is there a per-source-type
   adapter registered separately, or does the ReceiverType type-
   switch on the source internally?

5. **Migration order for existing code.** `starlarkbridge/unmarshal_*.go`
   cannot be deleted until every consumer is ported. Which sites
   currently call `ToUnmarshaler` / `Unmarshal` / `assignTarget`
   need to migrate, and in what order, so that the old pipeline
   and the new one do not have to coexist long?

D13 gets written once the five questions above are answered. Until
then, steps 4–7 (flow directive, plan.Provider restructure, peer
dispatch, StarlarkRuntime registration) proceed without touching
marshaling — plan.Provider's structural restructure and peer
dispatch are orthogonal to this.

### O2 — The bind package is mostly garbage

**User position (verbatim context):** "the bind directory is
mostly garbage that needs to be completely tossed. we'll save the
names and that's about it."

Phase 8 cannot exit while `pkg/op/starlarkbridge/` carries the current
contents. The inventory below enumerates every file and records
an initial read on whether it stays, gets reshaped, or goes. Final
decisions defer to you.

**Current contents of `pkg/op/starlarkbridge/` (19 files):**

| File | Role today | Initial read |
|---|---|---|
| `invocation.go` | `Invocation{Target, Result}` data type (D1). | Stays — names land; it's a data struct. |
| `invocation_registry.go` | `InvocationRegistry` ledger (D6). | Stays — load-bearing for orphan detection, labels. |
| `options.go` | `Options{Label, RetryPolicy}` data type (D7). | Stays — pure data. |
| `promise.go` | `Promise` — plan-mode output handle + `starlark.Value` + `.retry()` builtin. | Uncertain. Under step 9 it folds into `Invocation`. Under O1's target-driven marshaling, its role may shrink further or move. |
| `provider_node_builder.go` | `NodeBuilder` — adapts a `(ProviderReceiverType, Graph)` pair for plan-mode starlark dispatch. | Stays — real abstraction, named in step 5. The dispatch-internal helpers (`assignTarget`, `linkResource`, `shadowPendingOutput`, `FillSlot`) are candidates for relocation if target-driven marshaling (O1) moves slot-fill logic elsewhere. |
| `receiver.go` | `starlarkbridge.receiver` (unexported) — adapts a `(ReceiverType, instance)` pair for immediate-mode starlark dispatch. | Stays at the architectural level — the immediate-mode counterpart of `NodeBuilder`. Internal details (marshal / unmarshal / dispatch) want the same O1 rework as `NodeBuilder`. Possible rename to match the pair (e.g., `InstanceMethodBuilder`?). |
| `starlark_runtime.go` | `StarlarkRuntime` — module registration, predeclared globals, script invocation. | Stays — the entry point for every starlark session. Step 7 updates its registration branches. |
| `unmarshaler.go` | `Unmarshaler` interface + `ToUnmarshaler(starlark.Value)` source-first dispatcher. | **Goes.** Source-first dispatch is the wrong shape (O1). Target-driven marshaling via ReceiverType-hosted marshalers replaces it. |
| `unmarshal_bool.go` | `boolUnmarshaler` projecting `starlark.Bool` onto bool targets. | **Goes.** Subsumed by target-driven marshaling. |
| `unmarshal_bytes.go` | `bytesUnmarshaler`. | **Goes.** |
| `unmarshal_dict.go` | `dictUnmarshaler`. | **Goes.** |
| `unmarshal_float.go` | `floatUnmarshaler`. | **Goes.** |
| `unmarshal_function.go` | `functionUnmarshaler` passing `*starlark.Function` through. | **Goes.** Target for `*mem.Function` or `func(...)(...)` reached via its own ReceiverType's marshaler. |
| `unmarshal_int.go` | `intUnmarshaler`. | **Goes.** |
| `unmarshal_list.go` | `listUnmarshaler`. | **Goes.** |
| `unmarshal_none.go` | `noneUnmarshaler`. | **Goes.** |
| `unmarshal_set.go` | `setUnmarshaler`. | **Goes.** |
| `unmarshal_string.go` | `stringUnmarshaler`. | **Goes.** |
| `unmarshal_tuple.go` | `tupleUnmarshaler`. | **Goes.** |

**What "saving the names" means.** The decisions that survive the
tossing are the type names and the package layout, not the
implementations. Specifically:

- `starlarkbridge.Invocation`, `starlarkbridge.InvocationRegistry`, `starlarkbridge.Options` — the
  data-type names.
- `starlarkbridge.NodeBuilder` — the plan-mode adapter name (step 5).
- `starlarkbridge.Runtime` — the runtime entry-point name.
- `starlarkbridge.Promise` — the name, even if the shape compresses per step 9.
- `starlarkbridge.receiver` — the unexported immediate-mode adapter name (or a
  new name decided alongside the rework).

**What "tossing" means in terms of line count.** The 11
`unmarshal_*.go` files plus `unmarshaler.go` add up to roughly
900 lines of source-first dispatch plumbing. Under the
target-driven marshaling model (O1's stated direction), all of
that disappears. The surviving bind package is ~5 files of data
types, adapters, and the runtime — the load-bearing pieces.

**Exit criterion.** Phase 8 does not close while the `unmarshal_*`
files and the `Unmarshaler` interface still exist. The concrete
replacement is the target-driven marshaler on ReceiverType (O1).
Write D13 → implement the replacement → delete the garbage.

**Open questions that tie to O1.**

- Which step actually performs the tossing? A dedicated step near
  the end of phase 8 (before step 20, test triage), or in-line
  with step 17 (CanConvert) / step 18 (plan-time type-check pass)
  since those already touch the conversion cascade?
- Does `receiver.go`'s unmarshal-into-struct logic
  (`receiver.go:unmarshalValue`, `unmarshalMap`, `unmarshalSlice`,
  `unmarshalStruct`, etc.) migrate alongside or does it get a
  separate commit? The file is ~1200 lines today; much of it is
  tangled with marshaling concerns that O1 addresses.
- Does `Promise` survive or compress into `Invocation` per step 9?
  Step 9's status in the table says Promise becomes an internal
  helper; this O2 inventory treats that as an open question
  because the answer affects what "saving the Promise name" means.

### O3 — Rename `pkg/op` → `pkg/workflow` and revisit type names

**Motivation.** `op` is a terse package identifier that doesn't
signal domain. Every consumer writes `op.Graph`, `op.Node`,
`op.AnnounceProvider`, `op.RoleModule`, … — functional but opaque
to a newcomer. "Workflow" is the accurate general term for "a
graph of tasks with saga semantics" and aligns with the
vocabulary used across orchestration systems (Temporal, Airflow,
Conductor, Step Functions). Rename `pkg/op` → `pkg/workflow` and
decide which type names travel along.

**Blast radius.** Much larger than the `bind` → `starlarkbridge`
rename. Estimated 400–600 files modified:

- Every `.go` file under `pkg/op/...` changes its package
  declaration or is moved.
- Every consumer package (`cmd/*`, `internal/*`, every provider,
  every gen file) updates imports and identifier references.
- All 27 generated `provider.gen.go` files regenerate
  (`op.AnnounceProvider` → `workflow.AnnounceProvider`, roles
  constants, etc.).
- Codegen templates (~20 `op.X` occurrences across
  `provider.gen.go.template`, `receiver_type.gen_test.go.template`,
  `module.gen_test.go.template`, `node_builder.gen_test.go.template`,
  `action.gen_test.go.template`, `resource.gen.go.template`,
  `dependent_type.gen.go.template`).
- `generate.star` constants and comments.
- Makefile — `$(P)` variable, every rule target path, the
  `NEW_OP_INVENTORY` variable name.
- `tools/New-OpInventory` — the tool name contains "Op"; decide
  whether to rename to `New-WorkflowInventory` or leave as a
  tooling artifact.
- `pkg/op/inventory` subpackage → `pkg/workflow/inventory`; the
  `inventory.gen.go` blank-import block regenerates.
- Plan docs, architecture docs, guides.
- **Cross-repo:** `devlore-registry` and every lore package
  depend on `pkg/op/...` and will break until they also migrate.
  Same pattern as the `bind` → `starlarkbridge` cross-repo cost.

**Strawman proposal (from Gemini, paraphrased).**

| Old | Proposed | Proposal rationale |
|---|---|---|
| `op` | `workflow` | Domain-accurate; aligns with industry vocab. |
| `Graph` | `Plan` / `Definition` | Business concept over data structure. |
| `Node` | `Task` / `Step` | Industry term for an executable unit. |
| `Subgraph` | `Stage` / `Group` | Logical collection of tasks. |
| `Executor` | `Engine` / `Runner` | The component that makes the workflow move. |
| `ExecutableUnit` | `Activity` / `Unit` | Industry term (Temporal, Airflow). |

Gemini's specific recommendation: Plan / Task / Engine.

**Counter-proposal (rejecting most of the renames):**

- **`op` → `workflow`** — **accept.** Best general term for this
  package's domain; renames the outermost scope only.
- **Keep `Graph`.** `Plan` collides hard with the starlark `plan`
  namespace (`plan.run`, `plan.options`, `plan.choose`). Renaming
  to `Plan` produces recursive prose: "plan.run executes the
  Plan"; docs and code read as if `plan` and `Plan` are the same
  thing. `Definition` is too vague. `Graph` is the DAG-vocabulary
  term everyone uses and carries no ambiguity.
- **Keep `Node`.** `Task` is industry-correct but the churn is
  high — "node" is embedded in every log line, error message,
  attempt history, serialized payload (`Node.Receiver`,
  `Node.Status`, `Node.Retry`, `Node.Action`, `NodeResult`,
  `nodeJSON`, `NodeBuilder`). Churn-to-benefit is poor.
- **Keep `Subgraph`.** Per project memory, `Subgraph` is
  recursive (it contains nodes AND other subgraphs, forming a
  tree). `Stage` implies linear ordering — wrong shape. `Group`
  is too weak for a type that owns saga semantics (retry,
  compensation, attempt history).
- **Optionally rename `Executor` → `Engine`.** Low-priority
  taste change. `Engine` fits a workflow-themed package; decide
  when the rest settles.
- **Keep `ExecutableUnit`.** `Activity` is Temporal-specific
  jargon that doesn't map cleanly (Temporal's Activity is
  atomic; `ExecutableUnit` covers both atomic Nodes and composite
  Subgraphs). `Unit` is vague. Current name is descriptive and
  precise.
- **Keep `Slot`, `Parameter`, `ReceiverType`, `Method`,
  `Resource`, `Converter`, `RetryPolicy`.** Accurate names
  already; no workflow-theme pressure on them.

**Net effect under the counter-proposal:** package name changes;
most type names stay. The consumer-facing diff is almost entirely
`op.X` → `workflow.X` — mechanical and safe. Optional
`Executor` → `Engine` is additive and can land separately.

**Alternative package names considered (rejected):**

- `core` — too vague; says nothing about the domain.
- `engine` — conflicts with the optional `Executor` → `Engine`
  type rename.
- `orchestration` — accurate but long and marketing-flavored.
- `graph` — elevates one type's name to the package.
- `saga` — the pattern is central but not the whole package.
- `exec` / `execution` — misses the planning side; the package
  holds both planning and execution primitives.

**Exit criterion.** Phase 8 exit defers the rename decision until
the implementation steps (8–20) are done. Landing the rename
before combinator redesigns would churn every step's diff
unnecessarily; landing it after gives one clean rename-only
commit with every downstream site updated in lockstep. The
decision itself — accept package rename, keep type names —
should be recorded as D14 when finalized, and the actual work
scheduled as a follow-up task outside phase 8 if the cross-repo
coordination cost justifies it.

**Questions that tie into this decision.**

- Does the `tools/New-OpInventory` tool name rename to
  `New-WorkflowInventory`, or stay as a tooling artifact? If it
  stays, the rename is not 100% grep-clean.
- Does `ExecutionContext` shorten to `Context`? I lean no —
  `workflow.Context` stutters conceptually against
  `context.Context` (Go stdlib) and creates signature-level
  ambiguity at every call site.
- Is `Executor` → `Engine` in or out?
- Do historical plan docs get updated for consistency, or stay
  as frozen records of past state?

## Invariants

### I1 — Plan-time type checking

Every Promise→slot binding is validated at plan-end via the Planner's
`CanConvertTypes`. Ill-typed bindings fail at plan time with a message
naming the source label, the consumer label, and the expected vs. actual
types. Because `Converter.CanConvert` is required to answer accurately
on type information alone (D9), plan-time decisions are final — no
trust gap between plan-time and runtime, no type-mismatch surprises
during execution.

### I2 — No hidden mutable planning state

Every `plan.*` call is a pure function from its starlark arguments to a
`*starlarkbridge.Invocation`. The only mutable state during planning is the
`InvocationRegistry`, which is append-only until planning completes. Authors
can reorder, refactor, or extract helper functions without changing graph
semantics (beyond what the refactoring itself expresses).

### I3 — Invocation registry is write-once

After `plan.run(...)` is called, the registry is frozen. Orphan detection
and type-checking read from the frozen registry. Execution operates on
the graph reachable from the root invocation(s); the registry's presence
is incidental at execute time (available if needed for label lookup, but
no longer written).

### I4 — Every starlark-visible name is owned by exactly one provider

Within the plan namespace, each reachable attribute name resolves to
exactly one source: either plan.Provider itself (immediate methods),
a single root-planned peer (e.g., flow.Provider), or a single
sub-namespace child. plan.Provider's construction enforces this at
program-init time (D12) — any collision across Tier 1 (own methods),
Tier 2 (root-planned peers), or Tier 3 (sub-namespace children) fails
startup with a message identifying both offenders. Starlark authors
never see ambiguous resolution; the error arrives before any script
runs.

## Updated step outline

The step numbers below match the Implementation status table at the
top of this document. Each step is a commit unit.

1. **Invocation registry + options types + `plan.options(...)` builder.**
   Landed. `starlarkbridge.Invocation{Target, Result}` per D1;
   `starlarkbridge.InvocationRegistry` with `ordered` + `byLabel` + per-provider.method
   `counts` and the methods `Register`/`AutoLabel`/`All`/`ByLabel` per D6;
   `starlarkbridge.Options{Label, RetryPolicy}` as a pure data struct;
   `(*plan.Provider).Options(label, retryPolicy) *starlarkbridge.Options`. Codegen
   picks up the new method and surfaces it starlark-side as
   `plan.options(...)`.
2. **`+devlore:root=true` directive & ProviderRole placement zone.**
   Landed. Per D12. `ProviderRole` partitioned into dispatch zone
   (bits 0–7) and placement zone (bits 8–15); `RoleRoot` is the
   first placement-zone bit. `AnnounceProvider` validates
   `roles.Dispatch() != 0`. `ReceiverRegistry` gains
   `RootProviders() []ProviderReceiverType`. Codegen parses
   `+devlore:root=true` and threads it through to the
   `AnnounceProvider` call as `|op.RoleRoot`. `filter_ctx_param`
   helper in `generate.star` strips a leading `context.Context`.
   Test template updated from `rt.ReceiverName()` to `rt.Name()`.
3. **Reserved-kwarg enforcement at method registration.** Landed.
   `newReceiverType` rejects any provider's method parameter list
   that declares `options`, `args` (without `*` prefix), or
   `kwargs` (without `**` prefix) as plain names. Program init
   fails with a clear message naming provider and method.
4. **flow.Provider declares `+devlore:root=true`.** Single
   directive addition on `pkg/op/provider/flow/provider.go`.
   Regenerate `pkg/op/provider/flow/gen/provider.gen.go`; roles
   expression picks up `|op.RoleRoot`. Activates the RoleRoot
   plumbing from step 2. No consumer wired yet — this is a
   plumbing activation.
5. **Rename `starlarkbridge.NodeBuilder` → `starlarkbridge.NodeBuilder`.** Landed.
   Rename-only commit: type, constructor (`NewNodeBuilder`),
   file (`bind/provider_node_builder.go`), codegen template
   (`node_builder.gen_test.go.template`), generated
   filenames (`*/gen/node_builder.gen_test.go`),
   `generate.star` dict keys, Makefile rule targets, test function
   names (`TestProviderNodeBuilder_*`), and plan doc references all
   updated. The `planners` field on `plan.Provider` was renamed
   `adapters` mid-rename and retains that name. The original plan to
   absorb the type into `plan.Provider` was superseded — it is a
   genuine abstraction (wrapper for a `ProviderReceiverType` + `Graph`
   pair that turns starlark attribute access into graph-node-creating
   builtins) and stays in the `bind` package as a named type. Step 6
   now layers peer dispatch on top of this abstraction rather than
   replacing it.
6. **plan.Provider discovers root-planned peers; three-tier Attr
   with collision detection.** plan.Provider scans
   `registry.RootProviders()` filtered to `RoleAction` at
   construction and builds a `peerBuiltins` map keyed by snake
   method name. Each entry is a `*starlark.Builtin` whose dispatch
   routes to the peer provider's planned-dispatch logic; the
   builtin's label uses the bare form because the source receiver
   is root. `Attr(name)` walks Tier 1 (plan.Provider's own
   methods) → Tier 2 (peer builtins) → Tier 3 (child
   sub-namespaces). Any collision fails plan.Provider construction
   with a message naming both providers and the offending method.
7. **StarlarkRuntime access × root registration branches.**
   `NewStarlarkRuntime`'s module-iteration loop branches per D12's
   access × root table. Root-planned providers are not registered
   as top-level globals and not as plan sub-namespaces — they are
   discovered by plan.Provider via `registry.RootProviders()`.
   Non-root planned providers stay reachable only via
   `plan.<name>.<method>`. Immediate-non-root stays top-level.
   Immediate-root installs methods as top-level predeclared
   entries (reserved for future use).
8. **plan.Provider.dispatch intercepts options kwarg.** Dispatch
   extracts the `options` kwarg before `starlark.UnpackArgs`,
   unwraps to `*starlarkbridge.Options`, and removes it from the kwargs
   list. A `*starlarkbridge.Invocation` is constructed around the new
   `*op.Node` and registered with the InvocationRegistry under
   the effective label (user-supplied via `Options.Label` or
   auto-labeled via `InvocationRegistry.AutoLabel`).
   `Options.RetryPolicy` applies to the node. Dispatch return
   stays `*starlarkbridge.Promise` at this step.
9. **`starlarkbridge.Invocation` as `starlark.Value`; dispatch returns
   `*Invocation`.** Add `Freeze`/`Hash`/`String`/`Truth`/`Type`
   and Promise-compatible `Attr`/`AttrNames` to `*starlarkbridge.Invocation`
   so every callsite that consumed `*starlarkbridge.Promise` continues to
   work. `plan.Provider.dispatch` return type changes from
   `*starlarkbridge.Promise` to `*starlarkbridge.Invocation`; Promise becomes an
   internal helper.
10. **`plan.Provider.FillSlot` dispatches by target type.** Slot
    expects `op.ExecutableUnit` → pull `invocation.Target`; else
    pull `invocation.Result` and use the existing Promise/edge
    logic from Phase 7. Replaces the current `*Promise` case in
    `FillSlot`.
11. **`plan.subgraph` primitive.** New method on flow.Provider;
    takes variadic invocations, builds a subgraph. Owns
    container-output-type inference for subgraph per D3: `[]T`
    when terminals are homogeneous, `[]any` otherwise. Empty
    subgraph errors. Absorbs old Phase 11. Starlark surface
    `plan.subgraph(...)`; action name `subgraph`.
12. **`plan.choose` redesign.** On flow.Provider. `Case{When any,
    Then any}`; compensable method; `CompensateChoose` companion;
    lazy dispatch of branches via `Graph.ExecuteWithStack`. Owns
    container-output-type inference for choose per D3.
    `plan.case(...)` lands on plan.Provider (not flow.Provider)
    as an immediate data builder producing the `*Case` values
    `plan.choose` consumes. Starlark surface `plan.choose(...)`;
    action name `choose`.
13. **`plan.gather` redesign.** On flow.Provider.
    `body=invocation`; existing Go-side Gather from Phase 7
    step 10 stays; starlark-facing builder changes. Owns
    container-output-type inference for gather per D3. Starlark
    surface `plan.gather(...)`; action name `gather`.
14. **`plan.wait_until` redesign.** On flow.Provider.
    `predicate=invocation`; timeout surfaces as Action.Do error.
    Owns container-output-type inference for wait_until per D3.
    Starlark surface `plan.wait_until(...)`; action name
    `wait_until`.
15. **`plan.run` + `plan.load` + `plan.save`.** Immediate methods
    on plan.Provider. `plan.run(...)` is the explicit entry
    point: variadic invocations, wrapped in a subgraph when more
    than one is passed; owns pre-flight with error aggregation
    (steps 16 + topological sort + 18). `plan.load(path)`
    rehydrates a graph from a serialized form; `plan.save(path)`
    serializes the current graph. Both load/save are immediate —
    no graph node, no invocation.
16. **Orphan detection at plan-end.** Walk from `plan.run`'s
    root; mark reachable; collect unreached registry entries as
    errors. Part of `plan.run`'s pre-flight pass per D4.
17. **`CanConvert` method on `op.Converter` +
    `plan.Provider.CanConvertTypes`.** Interface addition to
    `op.Converter` (D9); corresponding method on `plan.Provider`
    implementing the type-level cascade (D8).
18. **Topological sort + plan-time type-check pass.** Order the
    graph producer-before-consumer; walk Promise→slot bindings in
    topological order; apply `plan.Provider.CanConvertTypes`;
    collect mismatches as errors joined with orphan errors per
    D5.
19. **Migration of existing `.star` callers.** Per D11.
    `cmd/devlore-test/devloretest/data/test_is_*.star` files; any
    usage of `plan.flow.<method>` becomes `plan.<method>`.
20. **Test triage.** Run the full suite; fold residuals into
    follow-ups. Resolve `starlarkbridge.NewProvider` / `ReceiverName`
    template staleness flagged during step 2.

## Blast radius

- `pkg/op/action.go` — `CanConvert` interface method on `Converter`
  (D9) with the nil-safety contract documented.
- `pkg/op/receiver_type.go` — `ProviderRole` gains the `RoleRoot`
  placement-zone bit (bit 8) per D12; zone masks plus `Dispatch()` /
  `Placement()` accessors on the role value. No new interface
  method; existing `Roles()` carries placement info.
- `pkg/op/receiver_registry.go` — `AnnounceProvider` validates that
  `roles.Dispatch() != 0`; gains `RootProviders()
  []ProviderReceiverType` returning providers with the `RoleRoot` bit
  set (general filter callable from any provider that needs to
  discover peers).
- `pkg/op/starlarkbridge/node_builder.go` — **deleted** in step 5. Its behaviors
  (`dispatch`, `FillSlot`, `shadowPendingOutput`, `assignTarget`,
  `linkResource`) move onto `plan.Provider`. The type-level cascade
  `CanConvertTypes` (D8) lands on `plan.Provider` too.
- `pkg/op/starlarkbridge/promise.go` — `Promise` may stay as an internal helper
  or fold into `Invocation`; decide at end of Phase 8 (noted in
  Invariants discussion).
- `pkg/op/starlarkbridge/runtime.go` — `NewStarlarkRuntime`'s
  module-registration loop branches on access × root per D12.
  Non-root planned providers are no longer promoted to top-level
  globals; root-planned peers are skipped entirely (discovered by
  plan.Provider via `RootProviders()`). `plan.run` wiring with
  pre-flight pass and error aggregation (D5).
- `pkg/op/provider/plan/` — holds only immediate methods (`Options`,
  `Case`, `Run`, `Load`, `Save`) plus the planner-side dispatch
  machinery collapsed from `starlarkbridge.NodeBuilder`. Three-tier `Attr`
  dispatch, collision detection at construction.
- `pkg/op/provider/flow/` — **resurrected** (not removed) as the
  root-planned peer provider for `plan.*` primitives. Tagged
  `+devlore:access=planned, +devlore:root=true`. Methods: `Choose`,
  `Gather`, `Subgraph`, `WaitUntil`, `Complete`, `Degraded`, `Fatal`,
  `Elevate`.
- `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star`
  — adds parser for `+devlore:root=true`; threads value through the
  provider descriptor into the provider template.
- `star/extensions/com.noblefactor.devlore.Actions/templates/` —
  updates as each combinator step lands; not a speculative upfront
  rewrite.
- `cmd/devlore-test/devloretest/data/test_is_*.star` — migration from
  the old kwargs form to the invocation-passing form.
- Any starlark test fixtures using the old Choose/Gather forms — same.

**Cross-repo follow-up (not blast-radius for this phase):**

- `devlore-registry` and every lore package. They consume this API and
  will rewrite against the new planner surface in their own time. The
  phase-8 plan lands the new shape here; downstream repos migrate
  separately.

## Dependencies

- **Follows Phase 7.** Gather's compensation pattern (Phase 7 step 10) and
  ctx threading (Phase 7 step 10) are the templates the new Choose design
  mirrors.
- **Precedes Phase 12.** Phase 12 addresses defects on what used to
  be the flow provider — now flow.Provider reconstituted as a
  root-planned peer of plan.Provider per D12. Some of those defects
  may only surface or become addressable after the invocation-based
  APIs land.
- **Precedes `devlore-registry` + lore-package rewrite.** Downstream
  consumers (the `devlore-registry` repo and every lore package that
  consumes this API) rewrite against the new planner surface —
  invocations, `options` kwarg, `plan.run` entry point, flat
  `plan.subgraph / choose / gather / wait_until / complete / degraded
  / fatal / elevate / options / run` namespace, old Choose/Gather
  forms replaced. Tracked as a cross-repo follow-up outside this
  phase; Phase 8 lands the new shape here, downstream migrates in
  its own time.

## Post-refactoring discussion topics

These are deferred until the current refactoring completes (Phase 7 through
the end of the planned phases). Raise them then.

### F1 — Multi-output providers (Bazel-style Providers)

Bazel rules return lists of typed `Provider` objects; consumers pattern-
match to pull named fields. Our invocation currently exposes one
`Promise` (one output). If combinators grow multi-field outputs (e.g.,
a subgraph returning "primary value" + "diagnostic trace"), a typed
provider system scales better than single-Promise invocations. Not
needed until a concrete use case arises.

### F2 — Hermeticity tightening

Bazel's action sandbox enforces that executions see only declared inputs.
Our execution already confines filesystem access via `Root`, but ambient
context access (via `ExecutionContext`) is broader. Tightening would
require every provider method to declare its inputs/outputs explicitly,
with the executor enforcing the boundary. Aligns with the existing
design goal of full plan-time hermeticity; extension to execute-time
remains an aspiration.

## Related documents

- Parent plan: [extract-starlark-from-op.md](../extract-starlark-from-op.md)
- Phase 7 plan: [phase-7.md](phase-7.md)
- Phase-8 sub-plans (milestone prerequisites, 2026-06-04):
  - [phase-8/platform-unification.md](phase-8/platform-unification.md) — `op.Platform` + Composite `op.PackageManager` router
  - [phase-8/pkg-install-reconciler.md](phase-8/pkg-install-reconciler.md) — `pkg.Provider` thin veneer
  - [phase-8/compensation-failure-contract.md](phase-8/compensation-failure-contract.md) — SAGA failure terminals + restart
- Architecture:
  - `docs/architecture/4-resource-management.md` §6 — catalog + reconciliation
  - Dependency-analysis prototype notes — (to add pointer when located)

## Session Accounting (2026-05-01)

### Work Completed
- **13.0(c) Tag URI Parsing:** `mem.Resource` and `sourcePathFromURI` now use `op.ExtractTagSpecific`
  to handle RFC 4151 Tag URIs.
- **13.0(d) Receipt Marshalers:** Added `UnmarshalJSON`, `UnmarshalYAML`, and `hydrate` to
  `git.Receipt`. All state-bearing providers now support rehydration.
- **13.0(f) Parameter Defaults:** Implemented `name?=value` convention in `op.NewMethod`.
  `op.Parameter` gains a `Default any` field. The starlark bridge injects these values when
  arguments are missing.
- **File Compensation Refactor:** Transitioned `file.Receipt` to a "Transformation + Creation" model
  with an explicit `recoveryID` field. `Move` and `Backup` now use atomic `rename` operations.

### Technical Debt / Slop Introduced
- **Build Blocker:** The build currently fails on a missing dependency:
  `make: *** No rule to make target 'pkg/op/provider/archive/resource.go'`. This is likely a stale
  entry in `inventory.gen.go` or a result of a recent rename. Requires inspection of
  `pkg/op/inventory/inventory.gen.go`.
- **UI Hot-patches:** `cmd/star/cli/output.go`, `cmd/star/star/application.go`, and
  `cmd/star/main.go` were patched with `ui.NewProvider()` and `SetSilent()` to fix unexported field
  access. This is a functional stop-gap but violates the long-term design of moving UI to a
  first-class `RuntimeEnvironment` property. Task `13.0(i)` must be completed to clean this up.
- **Refactoring Regressions:** During a signature cleanup, the `op.Parameter` struct in
  `pkg/op/action.go` was over-reverted, deleting the `Default` field. It has since been restored,
  but downstream generated files may still have incorrect argument counts for `AnnounceProvider`.
- **Generated File Accuracy:** A recursive perl regex was used to update `AnnounceProvider` calls in
  `*.gen.go` files. While verified for `shellcheck`, other generated files may require manual
  verification of the `nil` (defaults) argument placement.
- **Repository Hygiene:** `commit_msg.txt` was accidentally committed to the root and then removed.
  The history may require an `amend` to fully clean up.
