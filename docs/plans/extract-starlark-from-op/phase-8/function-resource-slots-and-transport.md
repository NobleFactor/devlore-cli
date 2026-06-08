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

## Step 2 — execution converts the function.Resource to the Go callable (the starlark.Bridge)

**Where it happens.** At execution, `Method.Invoke` (`method.go:516`) converts each filled slot to its parameter type
via `Convert(env, slotValue, param.Type)`. For the `fn` slot, `value` is the `function.Resource` and `param.Type` is
`file.Reducer`; `Convert` reaches step 5 (`SourceConverter`, `convert.go:119`) and calls `function.Resource.ConvertTo`,
which **manufactures the Go func** with `reflect.MakeFunc(target, …)` (`resource.go:442`) — a value of the target type
whose closure marshals the Go args → Starlark, calls the Starlark function, and marshals the result back.

**The wrinkle (why `TestWalkTreePlanned` still fails).** That closure marshals both ways, and `function` does it with
two hand-rolled, primitive-only helpers — `goToStarlark` (`resource.go:704`) and `starlarkToGo` (`:757`). They are
partial duplicates of the conversion the bridge already owns (`toStarlarkReflect` for Go→Starlark, the `converter` for
Starlark→Go), and they **cannot wrap a Resource**: the reducer's `resource *file.Resource` argument dies at
`goToStarlark`'s default (`:745` — "unsupported type file.Resource"). A planned `walk_tree` converts, runs, then fails
calling the reducer. The fake must go — there must be **one** Go↔Starlark marshaler.

**Why `function` can't just call the bridge.** `starlarkbridge` imports `function` (its boundary builds the
`function.Resource`), so `function → starlarkbridge` is a cycle. Resources are also deliberately Starlark-agnostic
(`op` / `op/provider/*` don't import Starlark), so they can't self-convert either.

**The design — `starlark.Bridge`, injected as an env service.**

- **`starlark.Bridge`** — a small neutral package (`pkg/op/starlark`, aliasing `gostarlark "go.starlark.net/starlark"`)
  owning the contract; no one owns it, and any provider can call into Starlark through it:

  ```go
  type Bridge interface {
      CallStarlark(callable gostarlark.Callable, args gostarlark.Tuple, kwargs []gostarlark.Tuple) (gostarlark.Value, error)
      ToStarlarkValue(value any) (gostarlark.Value, error)   // Go → Starlark
      ToGoValue(value gostarlark.Value) (any, error)         // Starlark → Go (the call's result)
  }
  ```

- **`starlarkbridge` implements it** over its real `converter` / `toStarlarkReflect`, so `ToStarlarkValue` wraps a
  `*file.Resource` as a `goReceiver` exactly like every other Go→Starlark projection.

- **`op.RuntimeEnvironment` carries it via a generic service locator** — `op` stays agnostic, never naming `Bridge`:

  ```go
  services map[reflect.Type]any
  func (e *RuntimeEnvironment) RegisterService(iface reflect.Type, svc any)
  func (e *RuntimeEnvironment) ServiceByType(iface reflect.Type) any
  func ServiceFor[T any](e *RuntimeEnvironment) (T, bool)   // typed accessor
  ```

- **Injected in both env-setup paths** under `reflect.TypeFor[starlark.Bridge]()`: the **Starlark runtime** registers it
  on the planning env; the **op-graph executor** setup registers it on the execution env — so a deferred `ConvertTo`
  finds it whether it fires at plan time or at graph runtime.

- **`function.Resource.ConvertTo` pulls it at call time** — `op.ServiceFor[starlark.Bridge](f.RuntimeEnvironment())` —
  and uses it inside the `reflect.MakeFunc` loop: `ToStarlarkValue` per arg, `CallStarlark`, then `funcReturn` via
  `ToGoValue`. The manufacture and reflect-glue (signature checks, `funcReturn` / `funcError`) stay in `function`; only
  the marshaling and the call delegate. **`goToStarlark` and `starlarkToGo` are deleted.**

Dependencies point one way — `function → starlark`, `starlarkbridge → starlark + function`, `op → neither` — so the
cycle is gone. Completing this un-skips `TestWalkTreePlanned`.

**Open — plan-time signature validation.** `CanConvertTo` only checks `target.Kind() == reflect.Func`, so a reducer
whose Starlark arity doesn't match the Go signature passes planning and fails at run (`ConvertTo`, `resource.go:420`).
Pulling the param-count check forward (`Init` + compare in `CanConvertTo`) would make it a build-time error; deferred
pending a decision.

## Step 3 — content-resource transport (the big one)

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

### Decisions (step 3)

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

1. Step 1 — planner creates the `function.Resource` (TDD red → implement).
2. Step 2 — `starlark.Bridge` env service; `ConvertTo` pulls it and delegates marshaling + the call; delete
   `goToStarlark` / `starlarkToGo` (un-skips `TestWalkTreePlanned`).
3. Step 3 — content-resource transport (all decisions A–D settled).

## Status

- 2026-06-07 — draft. Steps 1–2 approach settled. **All step-3 decisions (A–D) resolved:** `op.Packer` / `op.Unpacker`
  + the content-⟹-packable invariant (enforced by the enumeration test); serialize the catalog's accumulated content
  list; content in `CanonicalContent`; input dispatched via the provider announcement (no new registry); the catalog
  owns handles while the sharded, mmap'd content-addressed store (`mem`'s formula, already used by `function`) owns the
  bytes, with `RecoverySite` unrelated. Run-time blobs (downloads) realize straight to the CAS via the planned
  `stream.Resource` and do not travel. Resource-serialization enumeration captured (all 9 resources serialize URI-only
  today; four content types — function/mem/json/yaml — need content pack/unpack). Surfaced by `TestWalkTreePlanned`
  (tracked in [21-lore-migration.md](21-lore-migration.md)).
