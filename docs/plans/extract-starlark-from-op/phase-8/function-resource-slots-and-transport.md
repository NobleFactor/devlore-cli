---
title: "Phase 8 · function-resource slots + content-resource transport"
parent: "docs/plans/extract-starlark-from-op/phase-8/21-lore-migration.md"
issue: TBD
status: draft
created: 2026-06-07
updated: 2026-06-07
---

# Function-resource slots + content-resource transport

## Goal

Let a planned action receive a Starlark function as an argument (e.g. `plan.file.walk_tree(fn=collector)`), execute it
by handing the provider method a real Go callable, and **save the graph and run it many times** — including on another
host — by carrying content-addressable resources (the function bytecode among them) in the graph document.

## Surfacing case / motivation

`TestWalkTreePlanned` fails: *`file.walk_tree: param fn: *starlark.Function value is neither assignable nor convertible
to file.Reducer`*. Root cause: a Starlark function passed as an argument never becomes anything the executor can hand to
a Go `file.Reducer` parameter — and even once it does, the function's content does not travel with a saved graph.

Three challenges, smallest first:

1. **Plan** — create the `function.Resource` from the function argument.
2. **Execute** — convert the `function.Resource` to the Go callable the method wants.
3. **Transport** — carry content-addressable resources (the function among them) with the saved graph.

## Background — how Resources serialize today

Every concrete Resource serializes to JSON/YAML as its **URI only**: none overrides `MarshalJSON`/`MarshalYAML` (all
inherit `ResourceBase`'s URI-string form), and all `Unmarshal*` rehydrate **from the URI**. Content bytes are explicitly
excluded (`function.Compiled` is `json:"-"`, `mem.Hash` is `json:"-"`, json/yaml `Data` is empty on rehydration). The
actual content lives in the local archive / `RecoverySite`, not the serialized resource — so today resources cache and
transport **identity, not content**. That is exactly the gap step 3 closes.

The full set (9 Resource types):

| Provider | Addressing | Reference / Content | JSON/YAML today |
|----------|------------|---------------------|-----------------|
| file | `AddressingLocation` | reference (filesystem) | URI only |
| git | `AddressingLocation` | reference (clone) | URI only (+ ref/HEAD) |
| appnet | `AddressingLocation` | reference (endpoint) | URI only |
| pkg | `AddressingLocation` | reference (package id) | URI only |
| service | `AddressingLocation` | reference (service id) | URI only |
| mem | `AddressingContent` | content (bytes) | URI only — content in archive |
| function | `AddressingContent` (embeds `mem`) | content (bytecode/source) | URI only — bytecode `json:"-"` |
| json | `AddressingContent` | content (JSON document) | URI only — `Data` empty on rehydrate |
| yaml | `AddressingContent` | content (YAML document) | URI only — `Data` empty on rehydrate |

Action-only providers (no Resource type): archive, encryption, flow, plan, platform, powershell, regexp, shell, stream,
template, ui.

**Classification hook already exists:** `Resource.Addressing()` → `AddressingContent` (must travel) vs
`AddressingLocation` (reference, stays).

## Step 1 — the planner converts at plan time, by parameter type + addressing (DONE)

The conversion lives in **`op.ActionPlanner.Plan`** (`planner.go`), in the immediate-value (`default`) branch — *not*
`projectToSlotValue` (that is only `Assemble`'s `**frame_bindings` path, which has no parameter type to drive it). Each
immediate arg is resolved against its parameter type at plan time:

- **A plain value** (string, int, …) → `Convert(env, value, param.Type)` now. A reference target (string →
  `*file.Resource`) is built via `TargetConverter.CanConvertFrom` and **cataloged at plan time**; otherwise it's
  identity / assignability.
- **A `Resource` value** (a `function.Resource` produced at the starlark→Go boundary) → switch on
  `Resource.Addressing()`:
  - `AddressingLocation` → convert now (location-based conversions — `file.Resource → path string` — are serializable).
  - `AddressingContent` → validate `SourceConverter.CanConvertTo(param.Type)` now and **defer the conversion to
    runtime** (content-based; its native product — a func pointer — is ephemeral and can't serialize into a saved graph).
  - anything else → `assert.Unreachable` (a `Resource` is content- or location-addressed; nothing else).

**The addressing contract.** An `AddressingLocation` resource converts along its *location* (`file.Resource ⇄ path`); an
`AddressingContent` resource converts along its *content* (`function.Resource ⇄ bytecode/func`). The switch makes that a
hard, enforced invariant.

**The platform comes for free — never call `WithPlatform`.** Plan-time resource construction needs a `Platform` (e.g.
`pkg.Resource → Platform.DefaultPurlType`), and `NewRuntimeEnvironment` now **defaults it to `platform.Detect()`** when
the spec sets none — so `env.Platform` is never nil and no caller (planner, test, or production) touches `WithPlatform`.
Execution always runs on the detected host; `WithPlatform` remains only as an explicit override for cross-platform
planning (e.g. build a Linux graph from a Darwin host). `PlanInvocator` gained `RuntimeEnvironment()` so the planner can
call `Convert`.

**Status.** `planner.go` updated; `NewRuntimeEnvironment` defaults `Platform` to `platform.Detect()`; the gating test
(`TestBuildPackage…`) passes with **no** `WithPlatform` — `pkg.Resource` and `file.Resource` build and catalog at plan
time. **Cleanup pending:** an earlier wrong-layer attempt in `projectToSlotValue` (+ a `plan → function` import +
`helpers_test.go`) is reverted in `helpers.go` / `provider.go`; `helpers_test.go` needs a `git rm`. Production needs
nothing — `builder.go` (planning) and `commands.go` (execution) get `Detect()` for free.

## Step 2 — produce the function.Resource without the cycle (typed source constructors + codegen source keys)

**The layering rule.** `starlarkbridge` wraps providers generically through `op` and must import **no concrete
provider**. The one violation was `toNaturalGo`'s `*starlark.Function` case calling `function.NewResource`
(`converter.go`) — a provider leak into the bridge. Removing it keeps the dependency graph one-directional
(`function → starlarkbridge`, never the reverse) and is what later lets the Step 3 `Invoker` live in
`starlarkbridge`.

**Where the resource is made — the planner, not the bridge.** The bridge knows only Starlark builtins, so `toNaturalGo`
reverts to passthrough for a `*starlark.Function` (it *is* a `go.starlark.net` builtin) and drops the `function` import.
Construction moves to **`op.ActionPlanner.Plan`**, which already owns resource recognition and addressing. In its
default branch, before the addressing switch, it asks the registry whether the value's Go type constructs a resource —
naming only `op` and `reflect`, never `function` or `*starlark.Function`:

```go
if _, isResource := value.(Resource); !isResource {
    if ctor, ok := env.ReceiverRegistry.ConstructorForSource(reflect.TypeOf(value)); ok {
        value, err = ctor(env, value)   // *starlark.Function → function.Resource; the addressing switch then runs
    }
}
```

**How the registry learns the source type — codegen, no registry, no directive.** A resource declares the Starlark value
it is born from in the one place that is both compiler-checked and codegen-readable: its constructor's **type-set
constraint**.

```go
func NewResource[T *starlark.Function | string](env *op.RuntimeEnvironment, unit op.ExecutableUnit, identity T) (*Resource, error)
```

Go type sets are real unions in constraint position, so `*starlark.Function | string` *is* the declaration. The generic
shell stays thin — it erases to `any` immediately into the non-generic `buildCandidate`, so monomorphization costs at
most one small stencil per instantiated GC shape (pointers share one shape; only used shapes are stamped). The
constructor is the single source of truth: its body `switch` and this constraint state the same union, the compiler
enforces it, and codegen reads it.

Codegen pieces (`star devlore actions generate` → `generate.star`):

- **`goast`** gains the ability to surface a function's **type-parameter constraint type-set** — today `Funcs` / `Methods`
  read only the value-parameter list, so the constraint behind `T` is invisible. A set `A | B | ~C` is an
  `ast.BinaryExpr` chain on `|`; walk it and expose the members on `FuncResult`. Structured, reuses the existing type
  renderer — not body parsing.
- **`_resource_return_type`** stops requiring the 2nd value param to be `any` and reads the constructor's type-set members
  as the source types. Only unambiguous Starlark-value members become keys: `*starlark.Function` does; `string` does
  **not** (it collides with `file` / `json` / … and stays target-driven).
- **`AnnounceResource`** gains a source-types argument and registers `byType[sourceType] = the receiver type` for each —
  reusing `byType`, **no new map**. `ReceiverRegistry.ConstructorForSource` is the read side the planner calls.

**Three constructors, three roles.** `NewResource[T *starlark.Function | string]` is the typed source declaration (its
caller passes a concrete `*starlark.Function`). `DiscoverResource(env, identity any)` stays `any` — the announced,
target-driven constructor the generated wrapper and the slot-coercion adapter both hand an `any`, so it *cannot* be
generic. `buildCandidate(env, identity any)` stays `any` — unexported (codegen never reads it), and `any` is what keeps
its type switch legal.

**Status.** Built and verified: `function.NewResource` typed `[T *starlark.Function | string]` (`DiscoverResource` /
`buildCandidate` stay `any`); `op.AnnounceResource` source types + `ReceiverRegistry.ConstructorForSource`; `goast`
surfaces type-parameter constraints; `generate.star` filters the type set to an allowlist (the `go.starlark.net/starlark`
builtins + `starlark.Value`; practically `*starlark.Function`) and emits the source arg + import — confirmed by
regenerating `function`'s gen byte-identical to the hand-edited target, with `gen/source_key_test.go` green. The
planner's `ConstructorForSource` construct (`ActionPlanner.Plan` default branch) and the `converter.go` passthrough
revert are now in — `starlarkbridge` no longer imports `function` (**the leak is closed**), and `go test ./pkg/op/...`
is green. **Step 2 complete.**

## Step 3 — execution converts the function.Resource to the Go callable (the starlarkbridge.Invoker)

**Where it happens.** At execution, `Method.Invoke` (`method.go:516`) converts each filled slot to its parameter type
via `Convert(env, slotValue, param.Type)`. For the `fn` slot, `value` is the `function.Resource` and `param.Type` is
`file.Reducer`; `Convert` reaches step 5 (`SourceConverter`, `convert.go:119`) and calls `function.Resource.ConvertTo`,
which **manufactures the Go func** with `reflect.MakeFunc(target, …)` (`resource.go:442`) — a value of the target type
whose closure converts the Go args → Starlark, calls the Starlark function, and converts the result back.

**The wrinkle (why `TestWalkTreePlanned` still fails).** That closure converts both ways, and `function` does it with
two hand-rolled, primitive-only helpers — `goToStarlark` (`resource.go:704`) and `starlarkToGo` (`:757`). They are
partial duplicates of the conversion the bridge already owns (`toStarlarkReflect` for Go→Starlark, the `converter` for
Starlark→Go), and they **cannot wrap a Resource**: the reducer's `resource *file.Resource` argument dies at
`goToStarlark`'s default (`:745` — "unsupported type file.Resource"). A planned `walk_tree` converts, runs, then fails
calling the reducer. The fake must go — there must be **one** Go↔Starlark converter.

**Why `function` can't just call the bridge.** `starlarkbridge` imports `function` (its boundary builds the
`function.Resource`), so `function → starlarkbridge` is a cycle. Resources are also deliberately Starlark-agnostic
(`op` / `op/provider/*` don't import Starlark), so they can't self-convert either.

**The design — `starlarkbridge.Invoker`, injected as an env service.**

- **`starlarkbridge.Invoker`** — defined in `starlarkbridge` itself. Step 2 removed the `starlarkbridge → function`
  edge, so `function` can now import `starlarkbridge`; the interface needs no neutral package and no import alias
  (`starlarkbridge` already imports `go.starlark.net/starlark` as `starlark`):

  ```go
  type Invoker interface {
      CallStarlark(callable starlark.Callable, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)
      ToStarlarkValue(value any) (starlark.Value, error)   // Go → Starlark
      ToGoValue(value starlark.Value) (any, error)         // Starlark → Go (the call's result)
  }
  ```

- **`starlarkbridge` implements it** over its real `converter`. The `toStarlark*` family lifts from `goReceiver` onto
  `converter`, so the receiver and the `Invoker` share one Go→Starlark converter; `ToStarlarkValue` wraps a
  `*file.Resource` as a `goReceiver` like every other Go→Starlark projection.

- **`CallStarlark` mints a fresh Starlark thread per call.** Starlark threads are not safe for concurrent reuse, so
  each call — hence each goroutine — gets its own. `RuntimeEnvironment` no longer carries a shared thread; the
  per-goroutine thread lives inside `CallStarlark`, not at the call sites.

- **`op.RuntimeEnvironment` carries it via a generic service locator** — `op` stays agnostic, never naming `Invoker`:

  ```go
  services map[reflect.Type]any
  func (re *RuntimeEnvironment) RegisterService(iface reflect.Type, service any)
  func (re *RuntimeEnvironment) ServiceFor(iface reflect.Type) any
  func ServiceFor[T any](runtimeEnvironment *RuntimeEnvironment) (T, bool)   // typed shortener over the method
  ```

- **Injected in both env-setup paths** under `reflect.TypeFor[starlarkbridge.Invoker]()`: the **Starlark runtime** registers it
  on the planning env; the **op-graph executor** setup registers it on the execution env — so a deferred `ConvertTo`
  finds it whether it fires at plan time or at graph runtime.

- **`function.Resource.ConvertTo` pulls it at call time** — `op.ServiceFor[starlarkbridge.Invoker](f.RuntimeEnvironment())` —
  and uses it inside the `reflect.MakeFunc` loop: `ToStarlarkValue` per arg, `CallStarlark`, then `funcReturn` via
  `ToGoValue`. The manufacture and reflect-glue (signature checks, `funcReturn` / `funcError`) stay in `function`; only
  the conversion and the call delegate. **`goToStarlark` and `starlarkToGo` are deleted.** `function.Resource.Init`
  keeps its own one-time thread — program initialization (run the `.star` to obtain the callable) is not a callable
  call.

- **`flow.Provider` is the second consumer** — its Case-lambda evaluation (`flow/helpers.go`) routes through
  `CallStarlark` rather than its own `starlark.Call`, and its `starlarkValueToGo` folds into `ToGoValue`, so the
  per-goroutine thread and the single converter are shared, not re-rolled.

Dependencies point one way — `function → starlark + starlarkbridge`, `starlarkbridge → starlark`, `op → neither` — so the
cycle is gone. Completing this un-skips `TestWalkTreePlanned`.

**Open — plan-time signature validation.** `CanConvertTo` only checks `target.Kind() == reflect.Func`, so a reducer
whose Starlark arity doesn't match the Go signature passes planning and fails at run (`ConvertTo`, `resource.go:420`).
Pulling the param-count check forward (`Init` + compare in `CanConvertTo`) would make it a build-time error; deferred
pending a decision.

## Step 4 — content-resource transport (the big one)

**Principle.** `AddressingContent` resources travel with the graph; reference resources (`AddressingLocation`) do not —
they are named by URI in slots and recreate on the target host.

**Invariant (decided).** `Addressing() == AddressingContent` ⟹ the type implements `op.Packer` **and** `op.Unpacker`. A
graph must be immutable and portable across machine boundaries, so a content resource that cannot pack its bytes cannot
cross the boundary and could not run there — "content-addressable but not packable" is an *illegal* resource, not a
degraded one. **Enforcement:** extend the existing Resource-enumeration test (the one asserting no type returns
`AddressingUnknown`) with a clause — every `AddressingContent` type must satisfy `op.Packer` / `op.Unpacker`.

**The out / in shape.**

- **Out (marshal).** The catalog accumulates resources as planning proceeds; its `AddressingContent` entries are the list
  that travels. Marshal walks that list → `Pack()` each → a digest-keyed **content section** in the document. Reference
  entries are *not* written there (slot URIs already carry them); the catalog's per-run state (lifecycle, producer
  stamps, observations — the `Clone`d-per-run part) does *not* travel either (rebuilt fresh per run). What crosses the
  boundary: graph structure + slot URIs (already serialized) **plus** the content blobs.
- **In (load).** `assembleGraph` reads the content section → `Unpack()` each → writes the bytes into the local sharded
  CAS (below) and repopulates the catalog with the resulting handles (today it builds an empty `NewResourceCatalog()`,
  `graph.go:371`). A run then resolves content from the rehydrated catalog and references on-demand from their URIs —
  indistinguishable from a freshly-planned run on the origin machine.
- **Dispatch (no new registry).** On the way in, the blob's concrete type comes from the `typeID` in its URI fragment
  (`PkgPath.Name`). Go cannot instantiate a type from a string id, but this needs no *new* registry — fold the
  `typeID → Unpacker` dispatch into the **existing provider announcement** (`AnnounceProvider`): a provider owns its
  resource types, so when announced it registers its content-resource `Unpacker` under that `typeID`, and `LoadGraph`
  already runs with the announced inventory. (Confirm the announcement has a clean hook when building.)

**Content store & lifetime (the run-time bytes).** The catalog owns *handles* (digest / URI), never raw bytes. The bytes
live in the **sharded content-addressed store** that `mem` defines and `function` already uses —
`<Root>/.devlore/<provider>/resource/sha256/<hex[0:2]>/<hex>` — read at execution via `mmap` + `io.SectionReader`, so a
multi-gigabyte blob is a digest plus a memory-mapped view, never RAM-resident. This is the established pattern, not new.
Two lifetimes flow through the same store:

- **Plan-time content** (`function`; `json` / `yaml` literals) — created during planning, travels in the document's
  content section, and is materialized into the local CAS on load.
- **Run-time content** (downloads / fetched bytes) — created during execution as a per-run product, written straight to
  the local CAS as produced, and **does not travel** (it is not part of the immutable graph). Its planned home is
  `stream.Resource` (`pkg/op/provider/stream` is empty today; `appnet` flags it as future step 13.0(k).10's
  `Download → *stream.Resource`).

`RecoverySite` (`.devlore/recovery`) is unrelated — it remains the saga file-backup / compensation store, never a content
store.

### Decisions (step 4)

- **A — RESOLVED.** `op.Packer` (`Pack() ([]byte, error)`) and `op.Unpacker` (`Unpack(uri string, b []byte) (Resource,
  error)`), implemented by the four content types (function/mem/json/yaml); `function` reuses `function/pack.go`.
  Per-resource `MarshalJSON`/`MarshalYAML` stays URI-only. The content-⟹-packable invariant (above) is enforced by the
  enumeration test; input is dispatched via the provider announcement — **no new registry**.
- **B — RESOLVED.** Serialize the content entries the catalog **accumulated as it built up** — no separate reachability
  pass over slot values.
- **C — RESOLVED.** A false dichotomy (a misread): the bytes never lived in `RecoverySite`. The **catalog owns handles;
  the sharded content-addressed store owns the bytes**, mmap'd at execution (`mem`'s formula, already used by `function`
  — see *Content store & lifetime*). Load materializes the document's content section into the local CAS; `RecoverySite`
  (`.devlore/recovery`) stays purely for compensation backups. Large run-time blobs (downloads) are per-run products
  realized straight to the CAS via the planned `stream.Resource` — they never bloat the document or RAM.
- **D — RESOLVED.** Content resources are included in `CanonicalContent`; their content-digest URIs keep it stable and
  integrity-covering, which the immutable-graph guarantee requires.

## Sequencing

1. Step 1 — the planner converts at plan time, by parameter type + addressing (done).
2. Step 2 — produce the `function.Resource` without the cycle: typed source constructor + codegen source key + planner
   registry-construct + converter passthrough (removes the `starlarkbridge → function` leak).
3. Step 3 — `starlarkbridge.Invoker` env service; `ConvertTo` pulls it and delegates conversion + the call; delete
   `goToStarlark` / `starlarkToGo` (un-skips `TestWalkTreePlanned`).
4. Step 4 — content-resource transport (all decisions A–D settled).

## Status

- 2026-06-09 — Step 3 foundation: `op.RuntimeEnvironment` gains a service-locator (`RegisterService` / `ServiceFor` /
  `ServiceFor[T]`) to carry the `Invoker` by interface type. **Per-goroutine Starlark threads** decided — the shared
  `RuntimeEnvironment.Thread` is removed (threads are not safe for concurrent reuse); `CallStarlark` mints one per
  call. Interim: `function.Resource` and `flow` mint inline threads until the `Invoker` lands and both route through
  it. Next: build the `Invoker` (interface + impl + the `toStarlark*` lift onto `converter`).
- 2026-06-09 — boundary-untangle (Step 2) **complete**: the planner's `ConstructorForSource` construct
  (`ActionPlanner.Plan` default branch) plus the `converter.go` passthrough revert — `*starlark.Function` flows
  passthrough through `toNaturalGo` → planner → `function.Resource`, and `starlarkbridge` no longer imports `function`
  (the leak is closed). `go test ./pkg/op/...` green. Next: Step 3 (`starlarkbridge.Invoker` runtime conversion)
  un-skips `TestWalkTreePlanned`.
- 2026-06-08 — boundary-untangle (Step 2) largely built: `function.NewResource` typed `[T *starlark.Function | string]`;
  `op.AnnounceResource` source types + `ConstructorForSource`; `goast` type-parameter constraints; `generate.star` +
  template emit the source arg, filtered to an allowlist (`go.starlark.net/starlark` builtins + `starlark.Value`) —
  verified by regenerating `function`'s gen byte-identical to the target. Dup handling deferred. Pending: the planner's
  `ConstructorForSource` construct and the `converter.go` passthrough revert (closes the `starlarkbridge → function`
  leak).
- 2026-06-07 — draft. Steps 1–3 approach settled. **All step-4 (transport) decisions (A–D) resolved:** `op.Packer` / `op.Unpacker`
  + the content-⟹-packable invariant (enforced by the enumeration test); serialize the catalog's accumulated content
  list; content in `CanonicalContent`; input dispatched via the provider announcement (no new registry); the catalog
  owns handles while the sharded, mmap'd content-addressed store (`mem`'s formula, already used by `function`) owns the
  bytes, with `RecoverySite` unrelated. Run-time blobs (downloads) realize straight to the CAS via the planned
  `stream.Resource` and do not travel. Resource-serialization enumeration captured (all 9 resources serialize URI-only
  today; four content types — function/mem/json/yaml — need content pack/unpack). Surfaced by `TestWalkTreePlanned`
  (tracked in [21-lore-migration.md](21-lore-migration.md)).
