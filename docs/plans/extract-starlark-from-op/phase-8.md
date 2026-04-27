---
title: "Phase 8: Plan-time scope and grouping combinators"
parent: "docs/plans/extract-starlark-from-op.md"
issue: 275
status: in-progress
created: 2026-04-17
updated: 2026-04-22
---

## Implementation status

Every step below is a commit unit — one step, one checkpoint commit on
`refactor/extract-starlark-from-op.phase-8`.

| # | Step | Status | Notes |
|---|---|---|---|
| 1 | Invocation registry + options types + plan.options builder | complete | `starlarkbridge.Invocation`, `starlarkbridge.InvocationRegistry` (ordered + byLabel + per-provider.method counts), `starlarkbridge.Options{Label, RetryPolicy}` as pure data struct. `*plan.Provider.Options(label, retryPolicy) *starlarkbridge.Options` method; codegen picks it up to expose starlark-side as `plan.options(...)`. |
| 2 | `+devlore:root=true` directive & ProviderRole placement zone | complete | Per D12. `ProviderRole` is partitioned into dispatch zone (bits 0–7: `RoleModule`, `RoleAction`) and placement zone (bits 8–15: `RoleRoot`) with zone masks and `Dispatch()` / `Placement()` accessors. `AnnounceProvider` validates that at least one dispatch-zone bit is set. `ReceiverRegistry` gains `RootProviders() []ProviderReceiverType`. Codegen parses `+devlore:root=true` on the provider struct and threads it through to the generated `AnnounceProvider` call as `|op.RoleRoot`. `filter_ctx_param` added in `generate.star` to strip a leading `context.Context` from announced parameter lists. Test template `receiver_type.gen_test.go.template` updated from `rt.ReceiverName()` to `rt.Name()`. |
| 3 | Reserved-kwarg enforcement at method registration | complete | `newReceiverType` rejects any provider method parameter list declaring `options`, `args` (without `*` prefix), or `kwargs` (without `**` prefix) as plain names. The `*args` and `**kwargs` variadic markers remain valid. Errors name the provider, method, and offending parameter. `reservedNameError` helper + table-driven tests cover plain / optional / variadic-decorated forms, the variadic markers, and ordinary names. |
| 4 | flow.Provider declares `+devlore:root=true` | complete | Directive added to `pkg/op/provider/flow/provider.go` with an updated doc comment explaining the root semantics. Regenerated `pkg/op/provider/flow/gen/provider.gen.go`; roles expression is now `op.RoleAction\|op.RoleRoot`. Verified at runtime: `registry.RootProviders()` returns `flow` with `roles=0x102`, `dispatch=0x2` (RoleAction), `placement=0x100` (RoleRoot). No consumer wired yet — plumbing activation only. |
| 5 | Rename `starlarkbridge.NodeBuilder` → `starlarkbridge.NodeBuilder` | complete | Type, constructor (`NewNodeBuilder`), file (`bind/provider_node_builder.go`), codegen template (`node_builder.gen_test.go.template`), generated filenames (`*/gen/node_builder.gen_test.go`), generate.star dict keys, Makefile rule targets, and plan doc references all updated. Test function names `TestPlanner_*` → `TestProviderNodeBuilder_*`. Field in `plan/provider.go` renamed `planners` → `adapters` (holds `*starlarkbridge.NodeBuilder` values). No behavior change; rename only. Supersedes the original "absorb into plan.Provider" plan — the revisit concluded that `starlarkbridge.NodeBuilder` is a real abstraction (wrapper for a `ProviderReceiverType` + `Graph` pair that turns starlark attribute access into graph-node-creating builtins) and keeps its place in the `bind` package. |
| 6 | plan.Provider discovers root-planned peers; three-tier Attr with collision detection | complete | `plan.Provider` gains `peerBuiltins map[string]starlark.Value` (Tier 2, write-once) and `rootNames map[string]struct{}` (to exclude roots from Tier 3). `NewProvider` calls `buildPeerBuiltins` which iterates `ctx.Registry.RootProviders()` filtered to `RoleAction`, constructs a `*starlarkbridge.NodeBuilder` per peer, and stores each method as a `*starlark.Builtin` under its snake name. Collision detection panics at construction on: (a) peer method vs. plan.Provider's own method, (b) peer method vs. sub-namespace provider name, (c) peer method declared on multiple peers — each error identifies both offenders. `ResolveAttr` walks Tier 2 → Tier 3; root providers are excluded from Tier 3 so `plan.flow` returns nil. `starlarkbridge.NodeBuilder.Attr` now selects builtin label form by placement bit (bare for root, `<provider>.<method>` for non-root). `starlarkbridge.NodeBuilder.dispatch` writes `node.Receiver` as the always-dotted `<provider>.<method>` form for execute-time resolution independent of the builtin's display label. Smoke-verified: `plan.choose` / `plan.gather` / `plan.wait_until` / `plan.complete` / `plan.degraded` / `plan.fatal` / `plan.elevate` resolve to builtins; `plan.file` / `plan.git` resolve to `*starlarkbridge.NodeBuilder` adapters; `plan.flow` returns nil. |
| 7 | StarlarkRuntime access×root registration branches | complete | `NewStarlarkRuntime`'s module-iteration loop now explicitly branches on access × root per D12. `dispatch.&RoleModule == 0` (planned-only providers, root or non-root) → skip entirely; their methods surface via plan.* dispatch (Tier 2 for root, Tier 3 for non-root). `RoleModule + !root` → register as top-level global under `prt.Name()` (status quo for plan, ui, template, file/json/yaml/regexp/platform's module side). `RoleModule + root` → iterate the provider's methods and install each as its own top-level predeclared entry via `receiver.Attr(snake)`; collision against an existing predeclared panics. Reserved for future use; no Phase 8 provider claims this row. Smoke-verified: plan → "plan" global, flow → not registered, file/template → "file"/"template" globals for module side, git → not registered, ui → "ui" global. |
| 8 | NodeBuilder.dispatch intercepts options kwarg | complete | `NodeBuilder` gains a `registry *InvocationRegistry` field; `NewNodeBuilder(rt, graph, registry)` threads it in. `plan.Provider` gains `Invocations *starlarkbridge.InvocationRegistry` (instantiated in `NewProvider`) and passes it to every NodeBuilder it constructs (Tier 2 peers + Tier 3 child adapters). `dispatch` now extracts the reserved `options` kwarg via `extractOptionsKwarg` before `starlark.UnpackArgs` — unwraps a `*receiver` wrapping `*Options` (or accepts `starlark.None`), filters the kwargs, and returns the Options value. After node creation and slot filling, `dispatch` registers an `*Invocation{Target: node, Result: promise}` under the effective label (user-supplied via `Options.Label` or auto-labeled via `registry.AutoLabel(label)` where label is the builtin's display label — bare for root, dotted otherwise). `Options.RetryPolicy` applies to `node.Retry` before the graph add. Dispatch return stays `*Promise` at this step (step 10 changes it to `*Invocation`). Five unit tests cover `extractOptionsKwarg`: absent, *receiver unwrap, None, wrong type, wrong receiver instance. |
| 9 | NodeBuilder detaches from Graph | complete | Aligned dispatch with D5's detached-invocation model. `NodeBuilder` dropped its `graph *op.Graph` field and gained `ctx *op.ExecutionContext` + `catalog *op.ResourceCatalog`; new signature `NewNodeBuilder(rt, ctx, catalog, registry)`. `dispatch` no longer calls `graph.AddNode` — the node lives only on the returned `*Invocation` until plan.run (step 16) walks the reachable set and materializes a fresh `op.Graph`. `fillSlot` (list-of-promises branch and *receiver branch) stopped appending to `graph.Root.Edges`; the `PromiseValue{NodeRef, Slot}` in the consumer's slot already names the producer, and the Resource's `originID` (extractable via `op.ExtractResource`) names the resource-edge producer. `Promise` dropped its `graph` field, its `Graph()` accessor, and its `DependOn` method (unused); `NewPromise(node, slot)` has no graph argument. `Promise.FillSlot` now only sets the slot PromiseValue, no edge append. `shadowPendingOutput` uses `p.ctx` + `p.catalog` directly; `assignTarget` uses `p.ctx`; `linkResource` uses `p.catalog`. `plan.Provider` dropped `Graph *op.Graph` and gained `Catalog *op.ResourceCatalog`; `NewProvider` no longer calls `op.NewGraph`. Test template updated to construct `(ctx, catalog, registry)` instead of `(graph, registry)`; all 14 `*/gen/node_builder.gen_test.go` regenerated. |
| 10 | starlarkbridge.Invocation as starlark.Value; dispatch returns `*Invocation` | complete | `*Invocation` now implements `starlark.Value` (`Freeze`/`Hash`/`String`/`Truth`/`Type`) and `starlark.HasAttrs` (`Attr`/`AttrNames`) by delegating to the wrapped `Result *Promise`. Added `Label string` field to `Invocation` (the registered label, used by `String()` and set by dispatch). `Invocation.FillSlot` delegates to `Result.FillSlot` for slot-fill compatibility. `Invocation.Unmarshal` projects to `*Invocation` / `*Promise` / `op.PromiseValue` / `interface{}`. `NodeBuilder.dispatch` now returns the `*Invocation` (instead of `*Promise`) with the label stamped. `NodeBuilder.fillSlot` replaces its `*Promise` branch with a `*Invocation` branch (list-of-promises becomes list-of-invocations). Promise remains as an internal helper for slot-assignment mechanics. Seven unit tests cover Invocation's starlark.Value surface + Attr delegation + Unmarshal projections. |
| 11 | NodeBuilder.fillSlot dispatches by target type; catalog.Link extraction | complete | Per phase-8 D2. `fillSlot`'s `*Invocation` branch now reads the slot's target type and chooses: when `op.ExecutableUnit` is assignable to the target (slot wants the structural unit reference), set `ImmediateValue{Value: inv.Target}` — no PromiseValue, no edge implication; when not assignable, fall through to `inv.FillSlot(node, name)` which sets a PromiseValue (existing per-step-9 behavior). List-of-invocations branch follows the same rule per element: if the slot is `[]T` where `op.ExecutableUnit` is assignable to `T`, sub-slots hold ImmediateValue targets; otherwise PromiseValues. New package-level `executableUnitType = reflect.TypeFor[op.ExecutableUnit]()` cached at file scope for the AssignableTo check. **Refactor:** `op.ResourceCatalog` gains a `Link(resource Resource) Resource` convenience over `Resolve` for callers that only need the linked entry (slot-fill today, plan.load rehydration in step 16). `NodeBuilder.linkResource` deleted — its catalog-interning concern collapsed into the inline `catalog.Link(...)` call site, with the reflect-based pointer-vs-value reshape kept inline at the slot-fill site (it was always a slot-fill caller concern, not a Resource concern). Container methods landing in steps 12–15 take `op.ExecutableUnit` parameters and consume the unit references; value-typed parameters keep their PromiseValue behavior unchanged. |
| 12 | plan.subgraph primitive | complete | Added `Subgraph(children ...op.ExecutableUnit) []any` method to `pkg/op/provider/flow/provider.go`. Codegen picks it up; the regenerated announce map includes `"Subgraph": {"*children"}`. Surfaces in starlark as `plan.subgraph(...)` because flow is `RoleAction|RoleRoot`; action name `subgraph` (bare per D7). The variadic `op.ExecutableUnit` parameter triggers step 11's target-type dispatch — each child invocation's slot value is `ImmediateValue{inv.Target}` (structural reference, not a value-side promise). Return type `[]any` matches D3's container-output shape. The method body returns a length-`len(children)` slice of nils — the structural materialization (turning the Subgraph invocation into an `op.Subgraph` in the executable graph) is step 16's plan.run job, not this method's. Smoke-verified: `plan.Provider.ResolveAttr("subgraph")` now returns a `*starlark.Builtin` (previously nil). |
| 13.0 | Resource foundation cleanup: delete `<M>Planned` companions + roll out the 12 required interfaces | in-progress | Prerequisite for step 13 and everything downstream that touches Resources. **(a) Delete `<M>Planned` companions.** The mechanism is subsumed by Resource marshaling (see `project_resource_marshaling_subsumes_planned.md`): method decls across file/git/service/archive/encryption providers, the reflection lookup in `pkg/op/receiver_type.go`, `shadowPendingOutput` in `pkg/op/starlarkbridge/task_builder.go`, `Method.planned` / `HasPlanned` / `Plan` accessors, and the codegen filter in `generate.star`. Forward methods construct their own Resource inline at the top of the body (per `project_clone_pattern_inline_newresource.md`). **(b) Implement the 12 required Resource interfaces** (per `project_resource_required_interfaces.md`) across all eight Resource types. Shared implementations land on `op.ResourceBase` (CanConvert, Convert, Equal, MarshalJSON, MarshalStarlark, MarshalText, MarshalYAML); concrete types add `String`, a strict-type `Equal` override, and the four Unmarshal methods. **Progress:** companion decls deleted across provider files; most in-provider call-throughs rewritten; reflection lookup and `Method.planned` mechanism still in place pending step 13.0 completion. Interface rollout: `op.ResourceBase` complete; `file.Resource` complete (all 12 + Equal override); `git.Resource` **complete** — struct + 12 interfaces, Resolve body landed (reads `.git/` via exec: isGitRepo detection for Bare, rev-parse HEAD for HEAD, symbolic-ref for Ref, status --porcelain for Dirty, config --get-regexp for Remotes); `appnet.Resource` **complete** — strict-type Equal, region-marked structure, alias-trick MarshalJSON/MarshalYAML preserving SourceURL (transport-scheme preservation alongside the transport-independent URI), UnmarshalJSON/YAML/Text/Starlark reconstructing via NewResource, no-op Resolve (URL-identity has no on-disk state to reconcile). `git.Provider.Clone` accepts the full `git clone` surface: nine named kwargs (`bare`, `branch`, `depth`, `filter`, `no_checkout`, `no_tags`, `origin`, `recurse_submodules`, `single_branch`) plus a `**kwargs` catch-all, per the kwarg-to-flag rule (strip `--`, `-`→`_`, always expect a value). `directory == ""` defers to `guessDirName`, a byte-for-byte port of git's `git_url_basename`. Clone's undo slot is `*Resource` (not Tombstone) per the strict Tombstone rule — a tombstone exists only for an object moved to a RecoverySite, and Clone creates rather than displaces. `git.Tombstone` deleted entirely (no git action in the package today moves to a RecoverySite). `netprov`/`appnet` dependency purged. Checkout and Pull rewritten to call Resolve after `git checkout`/`git pull` — no direct `repo.Ref = ref` shortcut. The Bucket-B pattern (creation handles, not tombstones) is formalized as Phase 14 in the parent plan: file/archive/encryption/service/pkg providers adopt the same rule once Phase 8 closes. Four Resource types remaining for the 12-interfaces rollout: `json`, `pkg`, `service`, `yaml` (mem and appnet now complete per the principle below). **mem.Resource + appnet.Resource refactored to align with "identity ensures reachability"**: the URI carries everything a consumer needs to locate the Resource's content. mem.Resource's URI stays `mem:<content-type>/<namespace>/<name>`; the on-disk `SourcePath` is derived deterministically from the URI as `<Root>/.devlore/mem/<content-type>/<namespace>/<name>`. Content archival bypasses the UUID-based RecoverySite scheme (which stays focused on compensation-displacement backups) and writes directly to SourcePath via `ctx.Root.WriteFile` / `OpenFile`. Two resources with the same URI resolve to the same file — named-content dedup by construction. mem.Resource's NewResource accepts spec.Data in seven full-fidelity shapes (nil, io.Reader, []byte, string, `interface{ Bytes() []byte }`, encoding.BinaryMarshaler, encoding.TextMarshaler); types that don't round-trip losslessly (fmt.Stringer, op.Converter) are rejected to prevent silent data loss. Reader() returns an mmap-backed io.ReadCloser; Convert implements op.Converter for []byte and string (overrides ResourceBase's URI-as-string baseline). Unmarshal{JSON,YAML,Text,Starlark} reconstruct via the URI alone. appnet.Resource's URI is now the full URL (with transport scheme) — no `appnet:` wrapper, no alias-trick MarshalJSON override. SourceURL is derivable from URI (non-persisted). `http://x` and `https://x` are now distinct resources per the new principle (transport is reachability-critical). mem.Function retains its pack format but archives the pack at the URI-derived SourcePath (`<Root>/.devlore/mem/function/<namespace>/<name>`) rather than via `RecoverySite.ArchiveStream`; loadProgram mmaps from SourcePath. `op.RecoverySite.ArchiveStream` was added earlier but is now unused by mem; it stays as a general streaming entry point on RecoverySite for callers that need it. |
| 13.0(c) | Tag URI scheme as canonical `Resource.URI()` | not-started | Replace every `Resource.URI()` return value with an RFC 4151 tag URI of the form `tag:devlore.noblefactor.com,2026-01-01:<specific>#<go-type-id>`, where `<go-type-id>` is the canonical Go identity of the concrete Resource type — `PkgPath() + "." + TypeName()`. Locked decisions: D1(a) replace — `URI()` IS the tag URI (no dual accessor); D2(a) fixed date constant `2026-01-01` per RFC §2.1 "entitlement, not mint time"; D3(a) `<specific>` is each concrete Resource type's own identity-carrying payload (not a literal embedding of a reachability URI — per-type reach functions interpret `<specific>` to reach content); D4(a) `op.ResourceBase` owns minting; D5(a) strict RFC §2.4 octet-by-octet comparison, fragment included in identity. **Fragment is the Go type id** — canonical `PkgPath() + "." + TypeName()` form (e.g., `#github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Function`, `#github.com/NobleFactor/devlore-cli/pkg/op/provider/yaml.Resource`) — not a provider name, not a short package name. Long fragments are accepted; they disambiguate types across the entire import graph. Disambiguates types that share a `<specific>` shape (mem.Resource vs mem.Function vs mem.Stream; file.Resource vs git.Resource). **Reach function is per concrete type** — dispatched by fragment. For types where devlore owns storage, the reach path is `<Root>/.devlore/<last-pkg-segment>/<lowercase(TypeName)>/<specific>` — derived from the fragment by splitting on the final `.`, taking the final slash-segment of the left side as `<last-pkg-segment>`, and lowercasing the right side. On-disk layout stays short even when the fragment is long. For types where storage is external (file, git, appnet, pkg, service), `<specific>` is the reachability URI directly. **Per-type `<specific>` + fragment + storage** (all fragments are under `github.com/NobleFactor/devlore-cli/pkg/op/provider/`): `file.Resource` → `file://<absolute-path>#github.com/NobleFactor/devlore-cli/pkg/op/provider/file.Resource` → user FS; `git.Resource` → `file://<local-clone-path>#github.com/NobleFactor/devlore-cli/pkg/op/provider/git.Resource` → user FS; `mem.Resource` → `<ns>/<name>#github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Resource` → `<Root>/.devlore/mem/resource/<ns>/<name>`; `mem.Function` → `<ns>/<name>#github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Function` → `<Root>/.devlore/mem/function/<ns>/<name>`; `mem.Stream` → `<ns>/<name>#github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Stream` → `<Root>/.devlore/mem/stream/<ns>/<name>`; `json.Resource` → `<ns>/<name>#github.com/NobleFactor/devlore-cli/pkg/op/provider/json.Resource` → `<Root>/.devlore/json/resource/<ns>/<name>`; `yaml.Resource` → `<ns>/<name>#github.com/NobleFactor/devlore-cli/pkg/op/provider/yaml.Resource` → `<Root>/.devlore/yaml/resource/<ns>/<name>`; `appnet.Resource` → full canonicalized URL`#github.com/NobleFactor/devlore-cli/pkg/op/provider/appnet.Resource` → remote (scheme preserved); `pkg.Resource` → PURL`#github.com/NobleFactor/devlore-cli/pkg/op/provider/pkg.Resource` → package mgr; `service.Resource` → `<name>#github.com/NobleFactor/devlore-cli/pkg/op/provider/service.Resource` → OS. **Empty `<specific>` is the deferred ("known-at-execution") state** — `tag:devlore.noblefactor.com,2026-01-01:#yaml.Resource` means "a yaml.Resource whose identity is deferred to runtime." `IsKnownAtExecution(r)` becomes `r.ReachabilityURI() == ""`. The `op.KnownAtExecution` var and `op.knownAtExecution` unexported type at `pkg/op/resource.go:288-296` are **deleted** — no singleton. Invariants: `URI()` matches `^tag:devlore\.noblefactor\.com,2026-01-01:.*#[a-z][a-z0-9]*(?:[./][a-z][a-z0-9-]*)*\.[A-Z][A-Za-z0-9]*$` (fragment = full Go type id: slash-or-dot-separated lowercase segments for the package path, then a final `.<TypeName>` starting with an uppercase letter; `.*` allows empty `<specific>` for deferred state); `<specific>` cannot contain `#`; tag URIs are canonical by construction (all canonicalization happens before mint), so `Equal` stays a string compare. **Structural changes riding along** (necessarily coupled — URI shape depends on type shape): (1) `mem.Resource.ContentType` field **deleted** — mem.Resource is shapeless bytes; content-kind distinctions migrate to distinct Go types. (2) `mem.Stream` **introduced** as a new concrete Resource type embedding `mem.Resource` — generic byte stream with Reader/Bytes/Text access; storage parallel to `mem.Function`. (3) `json.Resource` and `yaml.Resource` **refactored to embed `mem.Resource`** — identity shifts from `<hash12>` to `<ns>/<name>`; hash becomes non-persisted metadata; storage shifts to `.devlore/<package>/resource/<ns>/<name>`. (4) Convenience parsers `Scheme`/`Opaque`/`Host`/`Path`/`Fragment` on `ResourceBase` **deleted** — not interface methods, only callers are tests, no production use. **Type-id / starlark-name separation.** Two namespaces, never conflated: (i) **Go type id** = `PkgPath() + "." + TypeName()` — registry cache key and tag URI fragment; (ii) **Starlark receiver name** = short (`file`, `mem.Function`, `plan`) — `rt.Name()`, first segment of an action name (`file.write_text`), what users type in `.star`. Starlark receiver name + starlark method name = **action name** (`file.write_text`), stored in `n.Receiver` and input to `ExecutionContext.ActionByName`. Receiver-name collisions across distinct Go types are possible in principle; deferred, not addressed in 13.0(c). **Registry rekey** (`pkg/op/receiver_registry.go`): `byName` renamed and rekeyed to `byID map[string]ReceiverType` keyed on `rt.TypeID()`; accessors rename `Registry.{Type,Action,Module,Planner,Resource}ByName` → `*ByID`. **`ReceiverType.TypeID() string`** (`pkg/op/receiver_type.go`) new method returning `PkgPath() + "." + TypeName()`, cached at `newReceiverType` construction and identical byte-for-byte to the tag URI fragment. `Name()` unchanged — still returns the short starlark receiver name. **Dispatch resolver (β)**: `n.Receiver` stays short (`file.write_text`); `ExecutionContext.ActionByName(actionName)` and `ModuleByName(starlarkReceiverName)` unchanged in name and input — internally each scans the sorted list (`Registry.Actions()` / `Modules()`) matching on `Name()` (O(N), N ≈ 10); no reverse short-name index in the registry. File-level work: `pkg/op/resource.go` — rewrite `NewResourceBase` to return `(ResourceBase, error)` with syntactic `#`-in-reachability validation (no I/O, no reachability verification); add `ReachabilityURI() string` returning `<specific>` (empty if deferred); add `ResourceType() string` returning the fragment; add package-level `op.ExtractTagSpecific(s string) (specific, typeID string, err error)`; add `op.Defer[T Resource](ctx *ExecutionContext) T` helper constructing a deferred instance of T; delete `KnownAtExecution` var, `knownAtExecution` type, and the convenience parsers; rewrite `IsKnownAtExecution` as `r.ReachabilityURI() == ""`. `pkg/op/resource_test.go` — add tag URI round-trip, empty-`<specific>` cases, rejection cases; delete convenience-parser tests. `pkg/op/provider/mem/resource.go` — delete `ContentType` field and `spec.ContentType` usage; `sourcePathFromURI` derives `.devlore/mem/resource/<ns>/<name>` from URI; all four Unmarshal methods call `ExtractTagSpecific` before reconstructing. `pkg/op/provider/mem/function.go` — drop `spec.ContentType = "function"` (type IS the distinction); storage shifts to `.devlore/mem/function/<ns>/<name>`. `pkg/op/provider/mem/stream.go` — **new file** — `mem.Stream` type embedding `mem.Resource`, NewStream constructor, storage at `.devlore/mem/stream/<ns>/<name>`, Reader/Bytes/Text access, pack format TBD (parallel to mem.Function but optimized for streaming). `pkg/op/provider/json/resource.go` — embed `mem.Resource` (not `op.ResourceBase`); identity shifts to `<ns>/<name>`; `NewResource` gains ns/name parameters; hash recomputed on read from storage; storage `.devlore/json/resource/<ns>/<name>`. `pkg/op/provider/yaml/resource.go` — same as json. `pkg/op/provider/{file,git,appnet,pkg,service}/resource.go` — Unmarshal methods call `ExtractTagSpecific`; concrete NewResource unchanged in signature, receives the stripped reachability URI. All per-provider Resource tests — URI-shape assertions updated to tag form. `ResourceSpec.URI()` method in `mem/resource.go:89` — replaced by `ResourceSpec.Specific()` returning `<ns>/<name>` (the `<specific>` payload; no scheme prefix). Ordering within 13.0 track: land before finishing 13.0(b)'s remaining 12-interfaces rollout on `pkg`/`service` (json and yaml are absorbed into this step's json+yaml refactor). Blast radius: every Resource type (10 total including new mem.Stream); every concrete Unmarshal method (40 methods — 4 × 10); every Resource test file. **Staged as one Resource per commit.** Commit sequence: **(0) Infrastructure** — `op.ResourceBase` gains `MintTagURI(specific, typeID) (string, error)`, `ExtractTagSpecific(s) (specific, typeID string, err error)`, `ReachabilityURI() string`, `ResourceType() string`, `Defer[T Resource](ctx) T`; `ReceiverType.TypeID() string` added (cached at construction, returns `PkgPath() + "." + TypeName()`); registry cache rekeyed from `byName` to `byID` on `rt.TypeID()`; accessors rename `Registry.{Type,Action,Module,Planner,Resource}ByName` → `*ByID`; `ExecutionContext.ActionByName`/`ModuleByName` internals updated to scan sorted receiver-type lists by `Name()` (β); convenience parsers and their tests deleted; `op.KnownAtExecution` var kept for now (unmigrated Resources still reference it). **(1–10) Per-Resource migrations**, one commit each, each leaving the build green: `file.Resource` → `git.Resource` → `appnet.Resource` → `pkg.Resource` → `service.Resource` → `mem.Resource` (structural: `ContentType` deleted) → `mem.Function` → `mem.Stream` (new type) → `json.Resource` (embed change) → `yaml.Resource` (embed change). Each migration commit: concrete `NewResource` calls `MintTagURI`; Unmarshal methods call `ExtractTagSpecific`; tests assert tag URI shape; any planned-path callsites returning `op.KnownAtExecution` for this type switch to `op.Defer[T]()`. **(11) Sentinel removal** — by this point no caller references `op.KnownAtExecution`; delete the var and `knownAtExecution` type; rewrite `IsKnownAtExecution` as `r.ReachabilityURI() == ""`. Transitional invariant during migration: `ReachabilityURI()` returns the `<specific>` for migrated types (tag URI on receiver) and the full stored URI for unmigrated types (no tag prefix to strip); `IsKnownAtExecution` stays pointer-based against the surviving singleton until the final commit when it flips to the empty-`<specific>` check. |
| 13.0(d) | Receipt JSON + YAML marshalers via `Snapshot` / `Restore` | not-started | Add `MarshalJSON` / `UnmarshalJSON` / `MarshalYAML` / `UnmarshalYAML` to every concrete `op.ReceiptBase`-embedding type so the recovery ledger persists across executor restarts. Five types in scope: `archive.Tombstone` (Dest, CreatedFiles), `encryption.Tombstone` (DestinationPath), `file.Tombstone` (base only; `RecoveryID()` is an alias over `TransactionID()`), `pkg.Tombstone` (Packages, Manager, Cask, AlreadyInstalled, PreviousVersions), `service.Tombstone` (Name, WasRunning, WasEnabled). Twenty methods total. **Wire shape**: flat object — base envelope `{action, resource_uri, transaction_id}` plus the type's own snake_cased fields; no nested `base:` object. **Marshaler pattern**: `Snapshot()` → transient struct combining the trio (Resource projected to URI string, TransactionID projected to canonical UUIDv7 string) with the type's exported fields; hand to `json.Marshal` / return for the YAML encoder. **Unmarshaler pattern**: decode into the same transient struct; resolve `resource_uri` through `ExecutionContext().Catalog` (per the round-trippable-URI invariant); parse `transaction_id` via `uuid.Parse`; pack the trio into the anonymous `Snapshot` shape and call `Restore(snapshot)`; assign the provider-specific fields directly; surface the `Restore`-already-set error when the receipt has been committed. **No new code on `op.ReceiptBase`** — `Snapshot` / `Restore` already form the encapsulation boundary; this work is purely derivative-side. **Open questions to close before code lands**: (1) how Tombstone unmarshalers reach a `ResourceCatalog` — recommend caller pre-seeds `ReceiptBase.resource` with a context-bearing zero resource (parallel to the Resource convention in `project_resource_unmarshal_context_wiring.md`); (2) Resource type discrimination — each Tombstone hard-codes its companion Resource type for the `NewResource(ctx, uri)` call (file.Tombstone ↔ file.Resource, etc.); (3) confirm YAML library handling of `pkg.Tombstone.PreviousVersions map[string]string` (gopkg.in/yaml.v3 native; goccy/go-yaml may need a tag); (4) confirm this work serves the persistence story (serialize the recovery ledger) rather than starlark slot-fill. **Out of scope**: `MarshalText` / `UnmarshalText` and `MarshalStarlark` / `UnmarshalStarlark` (Tombstones are persistence artifacts, not user-typed values); changes to `op.ReceiptBase`; changes to `op.RecoverySite` integration; updates to `file.Tombstone.RecoveryID()`. **Tests**: one `<type>_test.go` per package — JSON and YAML round-trip, assert `Snapshot` triples match plus provider-field equality; cover the `Restore`-on-committed error path once at the type level. **Step sequencing** (one commit each): close questions 1–4 → `archive.Tombstone` → `encryption.Tombstone` → `file.Tombstone` → `service.Tombstone` → `pkg.Tombstone`. Order grows surface gradually so encoder-library quirks surface and resolve before the largest provider-field set lands. |
K| 13.0(e) | Saga shape and stack-based recovery | not-started | Reshape the compensable-action contract and the recovery machinery so every saga is a first-class, persistable, transactional unit. Closes seven design questions in chat. **Action contract.** Compensable methods return one of exactly two shapes: `(Result, Receipt, error)` for single-output actions; `(Result, []Receipt, error)` for multi-output actions (e.g., `archive.Provider.Extract` returns one receipt per created file). The classifier in `Method.NewMethod` enforces this at registration — anything else fails program init. **`op.RecoveryStack` is anonymous; two entry kinds.** Constructor `op.NewRecoveryStack()` takes no parameters; stacks carry no name. `Push(receipt Receipt, actionName string) error` appends a single-receipt entry and auto-Commits the receipt with the supplied action name. `PushNested(sub *RecoveryStack)` appends a nested sub-stack as one transactional entry. `Unwind()` walks LIFO, dispatches per entry kind (receipt → bound Compensate companion; sub-stack → recursive Unwind), aggregates errors via `errors.Join`. The lower-level closure-bearing Push API (`Push(compensate, reconcile, undoState, reconcileState)`) and `Do(invoke, compensate, reconcile)` helper are **deleted** — receipt-bearing entries are the only legal entry shape. **Engine behavior — `Method.Invoke` is the single per-action stack-construction site.** Type-switch on the complement: nil → return nil (non-compensable); single Receipt → return as-is (executor pushes via `parent.Push`); `[]Receipt` (or any slice whose element implements Receipt) → engine builds an anonymous sub-stack, pushes each element via `sub.Push(r, m.actionName)`, returns `*RecoveryStack` (executor pushes via `parent.PushNested`). Prior `default` fallback is removed — unreachable by construction. **Executor push site (`pkg/op/executor.go:487` area).** Replaces today's `stack.PushAction(ec, action, complement)` with a type-switch: nil no-op; `Receipt` → `parent.Push(v, action.FullName())`; `*RecoveryStack` → `parent.PushNested(v)`. New accessor `Action.FullName() string` exposes the canonical action name (`<pkg-path>.<receiver>.<method>`); each concrete action delegates to `a.method.ActionName()`. **Subgraph behavior — Model B (subgraph-as-saga-unit).** Each subgraph creates its own anonymous local `*RecoveryStack` at execution start. Compensable child completions push into the local stack per the rules above. On subgraph **success**, the executor splices the local stack into the caller's parent via `parent.PushNested(local)` — the sub-saga becomes a single transactional entry on the parent. On subgraph **failure**, the executor calls `local.Unwind()` first (sub-saga unwinds as a unit) then propagates — sibling sagas in the parent are untouched. The root `RecoveryStack` is constructed identically in `Graph.Execute`; subgraph stacks splice into it as nested entries. Splice is **nested, not flat** — flat splicing was rejected because it erases the very saga boundary the type was declaring. **Wire form — anonymous structs, field-presence discrimination.** Two entry shapes with disjoint field sets, no `kind` tag: `{receipt}` for receipt-bearing entries; `{sub}` for nested entries. The receipt's own `action` field carries the action name (stamped by `Commit` at Push time); the entry envelope does **not** duplicate it. Recursion is automatic — each `sub` is a `*RecoveryStack` whose own `MarshalYAML` runs when the encoder walks the field. `MarshalYAML` is the source of truth (anonymous-struct literal with both `json:` and `yaml:` tags); `MarshalJSON` delegates via `json.Marshal(v)`. **Persistence (path (a) — receipt-as-data + closures-rebound-at-reload).** Marshal: each entry serializes its receipt (using the receipt's own marshaler per the `op.ReceiptBase` envelope landed in 13.0(d)). Sub-stacks recurse. Unmarshal uses **discriminator decode** on receipt-bearing entries — capture the receipt's bytes undecoded (`json.RawMessage` / `yaml.Node`), do a peek decode into a minimal `{Action string}` struct to read the action name, look up the action in the receiver registry, allocate a fresh instance of the concrete Receipt type from the Compensate companion's parameter type, then full-decode into it. Rebind the closure that `Unwind` will call. Reconcile state is **not** persisted on the wire; reconcile is re-derived at reload time by calling `Resolve()` on the receipt's Resource. Closure-bearing entries cannot exist in the new design (closure-only Push API is deleted), so persistence is universal by construction — no "this saga is durable, that one isn't" gradient. **Push API note:** `Push(receipt, actionName)` still takes `actionName` at runtime — it's needed to call `Commit(actionName)` on the receipt before storing. The action name flows into the receipt's own `action` field via Commit; from there it's serialized as part of the receipt and read back at unmarshal time. The Push signature is unaffected by dropping `action_name` from the wire — the wire change is purely a serialization concern. **`archive.Provider.Extract` concrete shape (the motivating example).** Signature `(source *file.Resource, prefixPath string) (*file.Resource, []op.Receipt, error)`. The affected resource is the archive (input/identity), not the destination directory. Product (first return) is the destination directory `*file.Resource`. Complement (second return) is `[]op.Receipt` — one receipt per created file; each receipt's affected Resource is the corresponding `*file.Resource` for the created file. `archive.Tombstone` is **deleted** — Bucket-A name on a Bucket-B (creation, not displacement) action; same rule that retired `git.Tombstone`. `extractTarGz` / `extractZip` continue to return `[]string`; `Extract` itself wraps each path via `file.NewResource(ctx, path)` to construct the receipt list. Resource construction stays at the provider boundary; helpers stay narrow. `CompensateExtract(undo *file.Receipt) error` is called N times by the unwind walker in reverse-creation order, removing one file per call. Errors aggregate via `errors.Join` at the sub-stack level. **Tombstone → Receipt rename across all surviving providers** (locked in chat). `file.Tombstone` → `file.Receipt`; `encryption.Tombstone` → `encryption.Receipt`; `pkg.Tombstone` → `pkg.Receipt`; `service.Tombstone` → `service.Receipt`. The "Tombstone" name carried displacement semantics that not all of these providers actually have (per Bucket-A/Bucket-B framing); Phase 14 will sort per-provider Bucket assignments. This step renames uniformly to remove the dual-vocabulary confusion (`op.Receipt` interface vs. `<provider>.Tombstone` concrete) while keeping the per-provider concrete types as distinct nominal types for type-safe Compensate signatures. `file.Tombstone.RecoveryID() string` (the one-line alias over `TransactionID()`) is **deleted** — RecoverySite interprets `TransactionID()` as the recovery key directly; the per-domain alias adds nothing once the discipline is "RecoverySite owns the recovery-key interpretation." No in-tree callers exist (existing call sites in `pkg/op/provider/file/provider.go` already use `TransactionID()` directly). The doc comment on `op.ReceiptBase` referencing `file.Tombstone.RecoveryID` is updated to drop the "Per-domain aliases" sentence and the parenthetical example. **Step 5 splits accordingly** — 5a does the rename across the four surviving providers (file, encryption, pkg, service) and deletes `RecoveryID()`; 5b does the archive.Extract refactor and deletes `archive.Tombstone`. **No migration of existing single-Receipt providers.** `file`, `encryption`, `service`, `pkg` keep their single-Receipt shape — they are genuinely 1:1 actions. The two contract shapes coexist via Method.Invoke's type-switch; no churn for unaffected providers. **Exception flagged for step 5b:** `file.Provider.WalkTree` currently returns `(result any, stack *op.RecoveryStack, err error)` and `CompensateWalkTree` takes `*op.RecoveryStack` directly. This violates the locked contract — `*op.RecoveryStack` is neither a `Receipt` nor a `[]Receipt`, so step 3's classifier will reject it. WalkTree is conceptually doing what sagas formalize (accumulating compensable operations as it traverses) but it exposes the stack rather than returning receipts. Open design fork for step 5b: (a) flatten to `(any, []op.Receipt, error)` — loses the WalkTree-built saga structure; (b) refactor WalkTree's body to push receipts onto a stack passed in via context — different contract, no return-stack; (c) extend Method.Invoke's complement classifier to accept `*op.RecoveryStack` as a third legal shape — explicitly for "action already built its own saga." (c) is the most honest given WalkTree's actual semantic but is a contract expansion. Decision deferred until step 5b lands. **Closed design questions:** (Q1) same `MethodCompensableFunction` kind, runtime type-switch — no new kind; (Q2) nested splice semantics — sub-stacks unwound as transactional units; (Q3) action returns plain data, engine builds anonymous stack, single Receipt is pushed as a single receipt (not wrapped in a one-entry stack); (Q4) persistence path (a), insist on Receipt or `[]Receipt`, delete closure-only API; (Q5) action-returned RecoveryStack concerns evaporate under the engine-builds-stack model; (Q6) no migration of single-Receipt providers; (Q7) no unification of single-Receipt and `[]Receipt` shapes — they stay distinct at every layer. **Open follow-ons (deliberately deferred):** whether `op.RecoveryStack` exposes `MarshalText` / `MarshalStarlark` (probably no — persistence/runtime artifact, not user-typed); concrete Receipt-type naming for Bucket-B actions (the `file.Tombstone` → `file.Receipt` rename you let sit); deeply-nested-subgraph performance characteristics (bounded by graph depth, not node count). **Ordering within 13.0 track:** depends on 13.0(d)'s receipt marshalers (the wire-form foundation) being complete. Lands after 13.0(d) and before any of the action-returning-saga-shaped providers (archive first; others as Bucket-B refactor proceeds in Phase 14). |
| 13.0(f) | Codegen extension for parameter defaults (pre-phase-9 prerequisite) | not-started | The `+devlore:defaults param=value` directive is parsed by `generate.star` and stored on every method parameter descriptor as a `default` string field, **but the value is never emitted in the generated bridge code** for method parameters (it IS emitted for `+devlore:bind` provider struct fields, in `compute_provider_init` and `compute_descriptor_init` — that path is unaffected). For method parameters today, the `?` suffix marks the parameter as optional in the announced parameter list, and `starlark.UnpackArgs` leaves the slot value at nil when the caller omits the kwarg; the nil becomes Go's zero value when reflect projects it into the Go arg. So `+devlore:defaults mode=0o755` on a method doc comment is informational only — directives across the codebase have been silently inert for method params since the directive was introduced. **Concrete consequence in `pkg/op/provider/file/provider.go`:** Copy/Mkdir/WriteText/WriteBytes/write all accept `mode os.FileMode` and currently take whatever the caller passes literally (`if mode == 0` checks were intentionally removed when this gap was recognized — Go has no way to distinguish "caller omitted" from "caller passed 0," and `0` is a valid file mode meaning "fully locked"). Until the codegen extension lands, starlark callers must pass `mode=` explicitly to every method that takes one; the convenient defaults `+devlore:defaults` was supposed to provide don't materialize. **End-to-end flow this step builds:** starlark caller `file.copy(src, dst)` (omits `mode`) → codegen-emitted bridge notices `mode` is unfilled, substitutes the default value (e.g., `file.ModeDefault`) into the slot → Method.Invoke projects the sentinel into the Go method's `mode` parameter → Go method body checks `if mode == file.ModeDefault { mode = source.Mode.Perm() }` (or the per-method default — see cp/mkdir semantics decision in chat) → caller gets the conventional Unix outcome with no manual kwarg threading. **Concrete work, in order:** (1) `pkg/op/provider/file/modes.go` (new file) defines `const ModeDefault os.FileMode = ^os.FileMode(0)` — the bit pattern is impossible as a real file mode (every type bit set, including mutually exclusive ones), so it never collides with a meaningful caller value. Per-method default semantics under cp/mkdir conventions: Copy → `source.Mode.Perm()` (default cp; setuid/setgid/sticky stripped); Mkdir → `0o777` (typical umask 022 yields 0o755); WriteText/WriteBytes/write → `0o666` (typical umask 022 yields 0o644). (2) Five Go method bodies updated to `if mode == ModeDefault { mode = <method's default> }`. (3) Five doc-comment directive updates: `+devlore:defaults mode=file.ModeDefault`. (4) `pkg/op/receiver_registry.go` extends `AnnounceProvider` to accept a 5th parameter `methodDefaults map[string]map[string]any` (method name → param name → typed default value); every existing announce call site (~18 generated provider.gen.go files) gets `nil` for this param when regenerated. (5) `pkg/op/method.go` / `receiver_type.go` — `Method` gains a `defaults map[string]any` field; `NewMethod` accepts it; new accessor `Method.Default(paramName) (any, bool)`. (6) `pkg/op/starlarkbridge/task_builder.go` — after `starlark.UnpackArgs`, walk optional parameters; for each slot still nil, look up via `method.Default(name)` and fill. Insertion point is between the UnpackArgs call (currently around line 236) and the slot-fill logic that follows. Compose cleanly with the `**kwargs` catch-all and variadic `*args` paths. (7) `star/extensions/com.noblefactor.devlore.Actions/templates/provider.gen.go.template` extends the AnnounceProvider call to emit a defaults map alongside method parameters. (8) `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star` computes the per-method defaults map in `build_method_descriptors` and propagates it to the template (descriptor already has the `default` field; just needs to surface in template output). (9) Regenerate all 18 `provider.gen.go` files via the `make inventory` / `make build` cycle; every file picks up the new AnnounceProvider arity. (10) Focused starlark integration test: `file.copy(src, dst)` without `mode` kwarg; assert destination's mode equals source's `.Perm()`. **Risk:** task_builder.go's slot-fill logic has its own quirks around the `**kwargs` catch-all and the variadic `*args` paths that the default-fill step needs to compose with cleanly — may surface mid-execution. **Phase-9 prerequisite:** without this step, every starlark caller of file/encryption/pkg/service/etc. methods that "should have a default" must pass the kwarg explicitly. Phase 9 (lore-package authoring) will be miserable without this. Land before phase 9 opens. |
| 13 | plan.choose initial redesign (superseded; successor open) | complete, superseded | Initial source landed: `flow.Provider.Case{When any, Then any}` pure data; `flow.Provider.Choose(defaultValue any, cases ...Case) (any, op.Complement, error)` compensable signature with `CompensateChoose` stub; `flow/helpers.go` `isTruthy`; `plan.Provider.Case(when, then) *flow.Case` constructor. Source never got a standalone commit — it rode in with the phase-8 WIP checkpoint (`f1ed104`). **Superseded in review**: (a) side-effecting Whens execute regardless of case selection because evaluating a When *is* running it; (b) per-method compensation doesn't model per-branch activation; (c) control-flow semantics (short-circuit, per-iteration scope, polling) belong in graph topology rather than as one-off method bodies. **Successor direction is open** — the previously-drafted 13b.1/13b.2/13c/13d recast (PlanM prefix + subgraph-kind executor + conditional-edge topology) has been abandoned. A fresh redesign for plan.choose is pending step 13.0 completion. |
| 14 | plan.gather redesign | not-started | Direction TBD pending successor redesign for step 13. Current `flow.Provider.Gather` goroutine orchestration remains in place and unchanged by phase 8 so far. |
| 15 | plan.wait_until redesign | not-started | Direction TBD pending successor redesign for step 13. Current `flow.Provider.WaitUntil` polling loop remains in place. |
| 16 | plan.run + plan.load + plan.save | not-started | Immediate methods on plan.Provider. `plan.run(...)` is the explicit entry point: variadic invocations, wrapped in a subgraph when more than one is passed; materializes the `*op.Graph` from the reachable invocation set (nodes from `invocation.Target`, edges from slot PromiseValues + resource originIDs); owns pre-flight with error aggregation (D5). `plan.load(path)` rehydrates a graph from a serialized form; `plan.save(path)` serializes the current graph. Both load/save are immediate — no graph node, no invocation — callable from starlark for tooling that wants to round-trip graphs without executing them. |
| 17 | Orphan detection at plan-end | not-started | Walk from `plan.run`'s root; mark reachable invocations; collect unreached registry entries as errors. Part of `plan.run`'s pre-flight pass per D4 + D5. |
| 18 | CanConvert on Converter + plan.Provider.CanConvertTypes | not-started | Type-only pre-flight conversion check. `op.Converter` interface gains required `CanConvert(target reflect.Type) bool` method with the nil-safety contract (D9). `plan.Provider` gains `CanConvertTypes(source, target reflect.Type) bool` implementing the type-level cascade (D8). |
| 19 | Topological sort + plan-time type-check pass | not-started | Order the graph producer-before-consumer; walk Promise→slot bindings in topological order; apply `plan.Provider.CanConvertTypes`; collect mismatches as errors joined with orphan errors per D5. |
| 20 | Migration of existing .star callers | not-started | `cmd/devlore-test/devloretest/data/test_is_*.star` files and doc snippets; switch from old `plan.choose(when=..., then=...)` kwargs form to invocation-passing form with `plan.case(...)` members. Any `.star` usage of `plan.flow.<method>` (sub-namespace form) becomes `plan.<method>` (flat form) per D12. Per D11. |
| 21 | Test triage | not-started | Run the full suite; fold residuals into follow-ups. Resolve the `starlarkbridge.NewProvider` / `ReceiverName` template staleness flagged during step 2 (module test template references APIs removed during Phase 7/8 refactoring). |
| 22 | Factor `file.Resource` into a taxonomic tree | not-started | Split the current catch-all `file.Resource` into a base type plus specialized variants: `file.Resource` retains shared identity + URI + SourcePath + cross-kind metadata; `file.Regular` holds regular-file fields (Checksum, Size, Mode-as-permissions); `file.Directory` holds directory-specific concerns; `file.Link` holds symlink target + follow behavior. Each variant implements the twelve required interfaces (per `project_resource_required_interfaces.md`). Migration: every provider method that currently accepts a generic `*file.Resource` is audited against the three variants and rewritten to accept the specific variant its semantics require (e.g., Copy/WriteText take `*file.Regular`; Mkdir returns `*file.Directory`; Link returns `*file.Link`). Gives `git.Resource` a cleaner "constrained directory" story (potential future embed of `*file.Directory` if that relationship becomes load-bearing). Exit item for phase-8 — must complete before phase closes. |

Plus unresolved design discussions that must close before phase-8 exits:

| # | Topic | Status |
|---|---|---|
| O1 | Marshaling design — argument-to-parameter-type matching via ReceiverType-hosted marshalers | open; direction stated, five questions pending |
| O2 | Toss the bind package — the 11 `unmarshal_*.go` files + `Unmarshaler` interface go; names survive | open; inventory captured, open questions tied to O1 |
| O3 | Rename `pkg/op` → `pkg/workflow` and revisit type names | open; blast-radius surveyed, strawman considered, counter-proposal recorded |

**Status:** in-progress. Steps 1–12 complete. Step 13's initial plan.choose redesign source rode into the
phase-8 WIP checkpoint (`f1ed104`) but was superseded in review; the previously-drafted 13b.1/13b.2/13c/13d
recast (PlanM prefix + subgraph-kind executor + conditional-edge topology) is abandoned. Current work splits
into two tracks: **(a)** Step 13.0 — Resource foundation cleanup — `<M>Planned` companion deletion (subsumed
by Resource marshaling) and the 12-required-interfaces rollout across all eight Resource types. Two of eight
complete (`op.ResourceBase` shared implementations + `file.Resource`); `git.Resource` structurally complete
pending test rewrites; six types remaining. **(b)** Successor designs for plan.choose (step 13), plan.gather
(step 14), and plan.wait_until (step 15) are open — no fresh direction has been locked; will be designed
after step 13.0 lands. Steps 16–21 unchanged. Step 22 (file.Resource taxonomic factoring into base + Regular
+ Directory + Link) is the phase-8 exit item. Open design items O1–O3 remain. The build is currently broken
at `pkg/op/method.go:379` (undefined `Convert`) and its downstream in `pkg/op/starlarkbridge/task_builder.go`
— resolution is part of step 13.0's deletion sweep.

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
- Architecture:
  - `docs/architecture/4-resource-management.md` §6 — catalog + reconciliation
  - Dependency-analysis prototype notes — (to add pointer when located)
