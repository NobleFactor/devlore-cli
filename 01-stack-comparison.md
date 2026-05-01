# Stack comparison — variant 1: high-level flow

```
╔════════════════════════════════════════════════╦════════════════════════════════════════════════════╗
║ IMMEDIATE-MODE                                 ║ PLAN-MODE                                          ║
║ (wrapper.dispatch — wrapper.go:974)            ║ (NodeBuilder.dispatch — task_builder.go:164)       ║
╠════════════════════════════════════════════════╬════════════════════════════════════════════════════╣
║ 1. starlark.Call(builtin)                      ║ 1. starlark.Call(builtin)                          ║
║                                                ║                                                    ║
║ 2. classify params (named / *args / **kwargs)  ║ 2. classify params (named / *args / **kwargs)      ║
║    filter kwargs against known-set             ║    filter kwargs against known-set                 ║
║                                                ║                                                    ║
║ 3. starlark.UnpackArgs(actionName, …, pairs)   ║ 3. starlark.UnpackArgs(label, …, pairs)            ║
║    populates  vals []starlark.Value            ║    populates  values []starlark.Value              ║
║                                                ║                                                    ║
║ 4. for each named param:                       ║ 4. node := op.NewNode(GenerateNodeID(actionName))  ║
║      toGoInto(sv, &val)         ── wrapper.go  ║    node.Bind(method)                               ║
║    for *variadic:                              ║                                                    ║
║      build list, toGoInto(list, &val)          ║ 5. for each slot, sv := values[i]:                 ║
║    for **kwargs:                               ║      fillSlot(node, slot, sv)    ── task_builder   ║
║      toGoInto(kv[1], &val) per extra kwarg     ║                                                    ║
║                                                ║       │ short-circuits:                            ║
║    ── all slots become natural Go (any-typed)  ║       │   *Invocation       → SetSlot/FillSlot     ║
║                                                ║       │   *starlark.List of *Invocation             ║
║       │                       → fan-in sub-slots   ║
║ 5. slots := map[string]any{ … }                ║       │   NoneType          → skip                 ║
║                                                ║       │   Projector         → Project(target)      ║
║ 6. method.Invoke(ctx, instance, slots)         ║       └─ generic path:                             ║
║         │                  ── pkg/op/method.go ║                                                    ║
║         │ for each param:                      ║ 6. starlarkToGoTyped(ctx, sv, param.Type)          ║
║         │   op.Convert(ctx, slotV, param.Type) ║         │                       ── wrapper.go      ║
║         │       │              ── op/convert.go║         │ NoneType → nil                           ║
║         │       │ 1.  AssignableTo             ║         │ toGo(sv, anyType)  ── natural Go         ║
║         │       │ 1b. ConvertibleTo            ║         │   (toGoInto into a fresh reflect.Value)  ║
║         │       │ 2.  slice element recursion  ║         │                                          ║
║         │       │ 3.  map element recursion    ║         └ op.Convert(ctx, intermediate, target)    ║
║         │       │ 4.  SourceConverter opt-in   ║                 │           ── op/convert.go       ║
║         │       │ 5.  TargetConverter opt-in   ║                 │ 1.  AssignableTo                 ║
║         │       │ 6.  registered Resource ctor ║                 │ 1b. ConvertibleTo                ║
║         │       └ 7.  error                    ║                 │ 2.  slice element recursion      ║
║         │                                      ║                 │ 3.  map element recursion        ║
║         └ reflect-call Go method               ║                 │ 4.  SourceConverter opt-in       ║
║                                                ║                 │ 5.  TargetConverter opt-in       ║
║ 7. result, complement, err := …                ║                 │ 6.  registered Resource ctor     ║
║                                                ║                 └ 7.  error                        ║
║ 8. w.toStarlark(result)                        ║                                                    ║
║    └ project Go → starlark for caller          ║ 7. if Resource: catalog.Link → linked              ║
║                                                ║    node.SetSlot(name, ImmediateValue{Value:final}) ║
║ 9. return starlark.Value to starlark eval      ║                                                    ║
║                                                ║ 8. for **kwargs slot:                              ║
║                                                ║    build starlark.Dict, fillSlot(node, kwSlot, d)  ║
║                                                ║                                                    ║
║                                                ║ 9. return *Invocation{Target: node, …}             ║
║                                                ║                                                    ║
║                                                ║   ── later, at plan.run materialization time:      ║
║                                                ║   resolve PromiseValues → method.Invoke(ctx, …)    ║
║                                                ║       which runs the same op.Convert per param     ║
╚════════════════════════════════════════════════╩════════════════════════════════════════════════════╝
```

**Convergence point:** both paths terminate in `op.Convert(ctx, naturalGoValue, targetType)`. The starlark-unpack stage is `toGoInto` in both (immediate-mode calls it directly; plan-mode reaches it via `starlarkToGoTyped` → `toGo` → `toGoInto`). The type-matching cascade (steps 1–7 of `op.Convert`) is one shared function, no duplication.

**Asymmetry that remains, by design:** plan-mode runs the `op.Convert` step at slot-fill time so the catalog can intern Resources before the Node is sealed; immediate-mode runs it at dispatch time inside `Method.Invoke`. Same cascade, different timing.
