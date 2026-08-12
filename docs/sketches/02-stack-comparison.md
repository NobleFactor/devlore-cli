# Stack comparison — variant 2: literal call stacks with file:line

```
═══════════════════════════════════════════════════════════════════════════════════════════════════════
 IMMEDIATE-MODE call stack                          │  PLAN-MODE call stack
 (e.g. file.read_text("/etc/foo"))                  │  (e.g. plan.file.write_text("/etc/foo", body))
═══════════════════════════════════════════════════════════════════════════════════════════════════════
 starlark.Call                                      │  starlark.Call
   value.go (go.starlark.net)                       │    value.go (go.starlark.net)
                                                    │
 → (*starlark.Builtin).CallInternal                 │  → (*starlark.Builtin).CallInternal
     value.go:851 (go.starlark.net)                 │      value.go:851 (go.starlark.net)
                                                    │
 → (*goReceiver).dispatch                           │  → (*NodeBuilder).dispatch
     go_receiver.go:669                             │      task_builder.go:164
                                                    │
   ├─ starlark.UnpackArgs(actionName, args, …)      │    ├─ extractOptionsKwarg(kwargs)        (:183)
   │    go_receiver.go:751                          │    ├─ classify slots/values/pairs        (:200-208)
   │                                                │    ├─ filter known vs extra kwargs       (:216-234)
   │  per named param:                              │    ├─ starlark.UnpackArgs(label, …)      (:236)
   │                                                │    ├─ op.NewNode(GenerateNodeID(name))   (:242)
 → toGoInto(sv, &val)                               │    ├─ node.Bind(method)                  (:243)
     go_receiver.go:765 → go_receiver.go:354        │
       (interface branch unpacks to natural Go)     │    per slot, sv = values[i]:
                                                    │
   │  per *variadic:                                │  → (*NodeBuilder).fillSlot
   │                                                │      task_builder.go:390
 → toGoInto(variadicList, &val)                     │
     go_receiver.go:795 → go_receiver.go:354        │      ├─ if *Invocation:                  (:405)
                                                    │      │     SetSlot or inv.FillSlot       (:407,409)
   │  per **kwargs entry:                           │      │     return
   │                                                │      ├─ if *starlark.List of Invs:       (:418)
 → toGoInto(kv[1], &val)                            │      │     fan-in sub-slots [i] + .len   (:430-440)
     go_receiver.go:809 → go_receiver.go:354        │      │     return
                                                    │      ├─ if NoneType: return              (:448)
   │  build slots map[string]any                    │      ├─ if Projector:
                                                    │      │     proj.Project(target)          (:458)
 → (*Method).Invoke(ctx, instance, slots)           │      │     SetSlot; return               (:462)
     method.go:383                                  │      │
                                                    │      └─ generic path:
   per param:                                       │
                                                    │      → starlarkToGoTyped(ctx, sv, target)
 → op.Convert(ctx, slotValue, p.Type)               │          task_builder.go:471
     method.go:401  →  convert.go:40                │             →  go_receiver.go:654
                                                    │
       Step 1.  AssignableTo / ConvertibleTo        │           ├─ NoneType → nil              (:656)
       Step 2.  slice element recursion             │           │
       Step 3.  map element recursion               │         → toGo(sv, anyType)
       Step 4.  SourceConverter.ConvertTo           │             go_receiver.go:660 → :573
       Step 5.  TargetConverter.ConvertFrom         │               →  toGoInto(sv, fresh rv)
       Step 6.  registered Resource constructor     │                  go_receiver.go:354
       Step 7.  error                               │
                                                    │         → op.Convert(ctx, intermediate, target)
 → m.Do(receiver, goArgs)                           │             go_receiver.go:666 → convert.go:40
     method.go (post-Invoke reflect-call)           │
       (reflect-call into Go method)                │               Step 1.  AssignableTo / ConvertibleTo
                                                    │               Step 2.  slice element recursion
   Go method runs, returns result                   │               Step 3.  map element recursion
                                                    │               Step 4.  SourceConverter
 ← Result/Complement                                │               Step 5.  TargetConverter
                                                    │               Step 6.  registered Resource ctor
 → (*goReceiver).toStarlark(result)                 │               Step 7.  error
     go_receiver.go:827 → go_receiver.go:203        │
       └ project Go → starlark for caller           │      ├─ if Resource: catalog.Link(resource) (:482-495)
                                                    │      └─ node.SetSlot(name, ImmediateValue) (:498)
 ← starlark.Value                                   │
                                                    │    after slot loop:
 returned to starlark.Call                          │    ├─ if kwargsSlot: build Dict, fillSlot (:267-274)
                                                    │    │     ── kwargs Dict re-enters fillSlot
                                                    │    │        which re-enters starlarkToGoTyped
                                                    │    │        which re-enters toGo + op.Convert
                                                    │    ├─ apply opts.RetryPolicy             (:280-281)
                                                    │    ├─ NewPromise(node, "")               (:293)
                                                    │    ├─ AutoLabel(label)                   (:299)
                                                    │    └─ Invocation{Target: node, …}        (:later)
                                                    │
                                                    │  ← *Invocation (a starlark.Value)
                                                    │
                                                    │  returned to starlark.Call
═══════════════════════════════════════════════════════════════════════════════════════════════════════
                            BOTH PATHS BOTTOM OUT IN op.Convert (convert.go:40)
                  with the same 7-step cascade. No duplicate type-matching anywhere.
═══════════════════════════════════════════════════════════════════════════════════════════════════════
```

**Convergence:** plan-mode's slot-fill enters `starlarkToGoTyped` at task_builder.go:471, which composes `toGo(sv, anyType)` (go_receiver.go:660) + `op.Convert(ctx, intermediate, target)` (go_receiver.go:666). Immediate-mode's per-param `toGoInto(sv, &val)` at go_receiver.go:765/795/809 followed by `Method.Invoke`'s `op.Convert(ctx, slotValue, p.Type)` at method.go:401 lands in the same convert.go:40 cascade. Same destination, same 7 steps.

**`op.Convert` step 1** absorbs both `AssignableTo` and `ConvertibleTo` (the previously-separate step 1b folded in during 982529d). Non-container-kinds widen via `reflect.Value.Convert` when the underlying types are convertible; `IsValid()` guard prevents reflection panics on nil interface values.

**Type discriminator nit:** plan-mode's list-of-Invocations short-circuit checks `*starlark.List` whose elements are all `*Invocation` (task_builder.go:418-440). The starlark list is the discriminator, not a Go `[]*Invocation` slice.
