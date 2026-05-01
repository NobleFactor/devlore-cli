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
 → (*wrapper).dispatch                              │  → (*NodeBuilder).dispatch
     wrapper.go:974                                 │      task_builder.go:164
                                                    │
   ├─ starlark.UnpackArgs(actionName, args, …)      │    ├─ extractOptionsKwarg(kwargs)        (:183)
   │    wrapper.go:1063                             │    ├─ classify slots/values/pairs        (:200-208)
   │                                                │    ├─ filter known vs extra kwargs       (:216-234)
   │  per named param:                              │    ├─ starlark.UnpackArgs(label, …)      (:236)
   │                                                │    ├─ op.NewNode(GenerateNodeID(name))   (:242)
 → toGoInto(sv, &val)                               │    ├─ node.Bind(method)                  (:243)
     wrapper.go:1082  →  wrapper.go:467             │
       (interface branch unpacks to natural Go)     │    per slot, sv = values[i]:
                                                    │
   │  per *variadic:                                │  → (*NodeBuilder).fillSlot
   │                                                │      task_builder.go:388
 → toGoInto(variadicList, &val)                     │
     wrapper.go:1118 →  wrapper.go:467              │      ├─ if *Invocation:                  (:403)
                                                    │      │     SetSlot or inv.FillSlot       (:405,407)
   │  per **kwargs entry:                           │      │     return
   │                                                │      ├─ if *starlark.List of Invs:       (:416)
 → toGoInto(kv[1], &val)                            │      │     fan-in sub-slots [i] + .len   (:430-438)
     wrapper.go:1134 →  wrapper.go:467              │      │     return
                                                    │      ├─ if NoneType: return              (:446)
   │  build slots map[string]any                    │      ├─ if Projector:
                                                    │      │     proj.Project(target)          (:456)
 → (*Method).Invoke(ctx, instance, slots)           │      │     SetSlot; return               (:460)
     method.go:401                                  │      │
                                                    │      └─ generic path:
   per param:                                       │
                                                    │      → starlarkToGoTyped(ctx, sv, target)
 → op.Convert(ctx, slotValue, p.Type)               │          task_builder.go:469
     method.go:419  →  convert.go:40                │             →  wrapper.go:949
                                                    │
       Step 1.  AssignableTo                        │           ├─ NoneType → nil              (:951)
       Step 1b. ConvertibleTo (non-container)       │           │
       Step 2.  slice element recursion             │         → toGo(sv, anyType)
       Step 3.  map element recursion               │             wrapper.go:955 → wrapper.go:916
       Step 4.  SourceConverter.ConvertTo           │               →  toGoInto(sv, fresh rv)
       Step 5.  TargetConverter.ConvertFrom         │                  wrapper.go:467
       Step 6.  registered Resource constructor     │
       Step 7.  error                               │         → op.Convert(ctx, intermediate, target)
                                                    │             wrapper.go:961 → convert.go:40
 → m.Do(receiver, goArgs)                           │
     method.go:427                                  │               Step 1.  AssignableTo
       (reflect-call into Go method)                │               Step 1b. ConvertibleTo
                                                    │               Step 2.  slice element recursion
   Go method runs, returns result                   │               Step 3.  map element recursion
                                                    │               Step 4.  SourceConverter
 ← Result/Complement                                │               Step 5.  TargetConverter
                                                    │               Step 6.  registered Resource ctor
 → (*wrapper).toStarlark(result)                    │               Step 7.  error
     wrapper.go:1161                                │
       └ project Go → starlark for caller           │      ├─ if Resource: catalog.Link(resource) (:480-493)
                                                    │      └─ node.SetSlot(name, ImmediateValue) (:496)
 ← starlark.Value                                   │
                                                    │    after slot loop:
 returned to starlark.Call                          │    ├─ if kwargsSlot: build Dict, fillSlot (:267-274)
                                                    │    │     ── kwargs Dict re-enters fillSlot
                                                    │    │        which re-enters starlarkToGoTyped
                                                    │    │        which re-enters toGo + op.Convert
                                                    │    ├─ apply opts.RetryPolicy             (:280)
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

**The accountability part:** the asymmetry I caused chaos around earlier — plan-mode going through `assignTarget` with its own duplicated switch + `AssignableTo`/`ConvertibleTo` shortcuts — is gone. `assignTarget` is deleted. Plan-mode's slot-fill enters `starlarkToGoTyped` at task_builder.go:469, which composes `toGo(sv, anyType)` (wrapper.go:955) + `op.Convert(ctx, intermediate, target)` (wrapper.go:961). Immediate-mode's per-param `toGoInto(sv, &val)` at wrapper.go:1082/1118/1134 followed by `Method.Invoke`'s `op.Convert(ctx, slotValue, p.Type)` at method.go:419 lands in the same convert.go:40 cascade. Same destination, same 7 steps.

**Diagram nit corrected:** the plan-mode list-of-Invocations short-circuit was labelled `[]*Invocation`. The actual check is on `*starlark.List` whose elements are all `*Invocation` (task_builder.go:416-442). Same outcome (fan-in sub-slots), but the type discriminator is the starlark list, not a Go slice.
