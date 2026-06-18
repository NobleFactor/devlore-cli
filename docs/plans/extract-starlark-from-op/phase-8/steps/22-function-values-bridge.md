---
step: 22
title: "Function values through the bridge → typed Go callbacks"
status: not-started — deliverable absent (both conversions missing); adjacent infra + prereq present
proof_run: 2026-06-17
parent: ../../phase-8.md
---

# Step 22 — Function values through the bridge → typed Go callbacks

**Status:** `not-started` for the deliverable. The two conversions the row names are both absent, and the proof test
(`TestWalkTreePlanned`) is red. Some adjacent infrastructure exists, and the named prereq is already satisfied.

## What this step delivers

`file.walk_tree(root=…, fn=collector, …)` — a starlark `def collector(initial, resource, path, stack)` passed as `fn`,
whose Go parameter type is `file.Reducer = func(any, *file.Resource, string, *op.RecoveryStack) (any, error)` — must
dispatch as a typed Go callback. Two conversions are required:

- **(a) plan-time (bridge):** wrap a starlark callable kwarg into a `function.Resource` so the slot value the executor
  sees is a resource, not a raw `*starlark.Function`.
- **(b) dispatch-time (`Convert`):** add a `reflect.Func`-target branch that, when the target is a func type and the
  value is a `function.Resource`, synthesizes a Go closure of the target signature marshaling Go args → starlark,
  `starlark.Call`-ing the wrapped function, and unmarshaling the result.

## Evidence

| Item | State |
|---|---|
| (b) `Convert` `reflect.Func`-target branch | **Absent.** `grep reflect.Func pkg/op/convert.go` is empty — `Convert` still hits its generic "neither assignable nor convertible" fallback for a func target. |
| (a) bridge wraps a starlark callable → `function.Resource` at plan time | **Absent.** The converter passes `*starlark.Function` through **as-is** (`pkg/op/starlarkbridge/converter.go:305`: "returns any remaining starlark type as-is — notably a `*starlark.Function`, which the planner resolves"); it does not mint a `function.Resource`. |
| Proof test `TestWalkTreePlanned` | **Red** (`cmd/devlore-test/devloretest`) — receiver-type derivation for the `*op.RecoveryStack` arg rejects `ResultByUnitID(string) (interface{}, bool)`. This is the allowed/known failure the row points to. |
| Adjacent infra | **Present.** `pkg/op/starlarkbridge/invoker.go` carries an `Invoker.CallStarlark(callable starlark.Callable, args ...any)` session-service (the per-session Invoker), and the converter comment scaffolding names the `function.Resource`-holding-a-`*starlark.Function`-reducer case. The wiring that would route `fn` through it is not built. |
| Prereq ("`function.Resource` marshals source, not just the digest URI") | **Satisfied.** `pkg/op/provider/function/resource.go:28-43`: the synthesized **source text** + bytecode are archived in a RecoverySite pack; identity is `sha256` over the source bytes; "the archived pack is the persistent source of truth" and JSON/YAML persistence relies on the pack, not the URI alone. The source is re-compilable per run — the portability the row requires. |

## Disposition / grade

`not-started` — accurate for the deliverable. Both required conversions are missing and `TestWalkTreePlanned` is red.
Nuance for the row: the prereq is already met (source-carrying `function.Resource`) and the session-service
`Invoker.CallStarlark` exists, so the remaining work is the two conversions + the home-of-record decision (inline
`ImmediateValue` slot literal vs. a serialized function table), not the prereq. Design scoped in
[phase-8/function-resource-slots-and-transport.md](../function-resource-slots-and-transport.md).
