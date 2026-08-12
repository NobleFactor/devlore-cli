# Stack comparison — variant 1: high-level flow

```
╔════════════════════════════════════════════════╦════════════════════════════════════════════════════╗
║ IMMEDIATE-MODE                                 ║ PLAN-MODE                                          ║
║ (goReceiver.dispatch — go_receiver.go:669)     ║ (NodeBuilder.dispatch — task_builder.go:164)       ║
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
║      toGoInto(sv, &val)      ── go_receiver.go ║    node.Bind(method)                               ║
║    for *variadic:                              ║                                                    ║
║      build list, toGoInto(list, &val)          ║ 5. for each slot, sv := values[i]:                 ║
║    for **kwargs:                               ║      fillSlot(node, slot, sv)    ── task_builder   ║
║      toGoInto(kv[1], &val) per extra kwarg     ║                                                    ║
║                                                ║       │ short-circuits:                            ║
║    ── all slots become natural Go (any-typed)  ║       │   *Invocation       → SetSlot/FillSlot     ║
║                                                ║       │   *starlark.List of *Invocation             ║
║ 5. slots := map[string]any{ … }                ║       │                       → fan-in sub-slots   ║
║                                                ║       │   NoneType          → skip                 ║
║ 6. method.Invoke(ctx, instance, slots)         ║       │   Projector         → Project(target)      ║
║         │                  ── pkg/op/method.go ║       └─ generic path:                             ║
║         │ for each param:                      ║                                                    ║
║         │   op.Convert(ctx, slotV, param.Type) ║ 6. starlarkToGoTyped(ctx, sv, param.Type)          ║
║         │       │              ── op/convert.go║         │                ── go_receiver.go         ║
║         │       │ 1. AssignableTo /            ║         │ NoneType → nil                           ║
║         │       │    ConvertibleTo             ║         │ toGo(sv, anyType)  ── natural Go         ║
║         │       │ 2. slice element recursion   ║         │   (toGoInto into a fresh reflect.Value)  ║
║         │       │ 3. map element recursion     ║         │                                          ║
║         │       │ 4. SourceConverter opt-in    ║         └ op.Convert(ctx, intermediate, target)    ║
║         │       │ 5. TargetConverter opt-in    ║                 │           ── op/convert.go       ║
║         │       │ 6. registered Resource ctor  ║                 │ 1. AssignableTo /                ║
║         │       └ 7. error                     ║                 │    ConvertibleTo                 ║
║         │                                      ║                 │ 2. slice element recursion       ║
║         └ reflect-call Go method               ║                 │ 3. map element recursion         ║
║                                                ║                 │ 4. SourceConverter opt-in        ║
║ 7. result, complement, err := …                ║                 │ 5. TargetConverter opt-in        ║
║                                                ║                 │ 6. registered Resource ctor      ║
║ 8. w.toStarlark(result)                        ║                 └ 7. error                         ║
║    └ project Go → starlark for caller          ║                                                    ║
║                                                ║ 7. if Resource: catalog.Link → linked              ║
║ 9. return starlark.Value to starlark eval      ║    node.SetSlot(name, ImmediateValue{Value:final}) ║
║                                                ║                                                    ║
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

**Step 1 of `op.Convert`** absorbs both `AssignableTo` and `ConvertibleTo` — primitive numeric widening (int → int64, int → float64) lands here without forcing callers to widen first.

**Asymmetry that remains, by design:** plan-mode runs the `op.Convert` step at slot-fill time so the catalog can intern Resources before the Node is sealed; immediate-mode runs it at dispatch time inside `Method.Invoke`. Same cascade, different timing.
