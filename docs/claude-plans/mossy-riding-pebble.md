# Plan: General-Purpose Struct Parameter Mechanism

## Context

Issue #167: The code generator flags `file.Copy` as "unprojectable" because its `Blob` parameter type is not in `typeMappings`. The current approaches — adding types to `typeMappings` or using `+devlore:struct_param` — are both special-casing. The user's directive: build a general-purpose mechanism where struct types flow through as first-class Starlark values backed by their Go structs.

**All provider public methods MUST project. No exceptions.**

## Summary

Create a reflection-based `StructWrapper` type in `pkg/op` that wraps any Go struct for Starlark consumption. Fields are discovered at runtime via reflection — no per-struct code generation for the wrapper itself. The transitive closure (nested structs) is handled automatically by recursive wrapping. Struct values flow through the system as their underlying Go type: immediate receivers extract the Go struct from the wrapper, FillSlot stores it via a `GoValuer` interface, and graph actions read it from slots via type assertion.

## Goals

1. **`pkg/op.StructWrapper`**: Single generic Starlark type that wraps any Go struct using reflection, implementing `HasAttrs` + `GoValuer`
2. **Auto-detect struct params**: Code generator recognizes struct parameter types and handles them without directives or type map entries
3. **Struct constructors**: Each struct type used as a parameter gets a constructor attribute on the provider receiver (e.g., `file.blob(...)`)
4. **End-to-end**: Immediate receivers, planned receivers, and graph actions all handle struct parameters
5. **Remove special cases**: Delete `Blob` from `typeMappings`, deprecate `+devlore:struct_param`

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `typeMappings` in codegen.go | Special-cased | `Blob` mapped as string (line 164) — wrong approach |
| `+devlore:struct_param` directive | Working | Manual per-method annotation; kwargs expansion |
| `TToStarlark` converters | Working | Go→Starlark via `starlarkstruct.Struct` (generated per struct) |
| `Starlark→Go` converters | Missing | No reverse converters exist anywhere |
| `FillSlot` in output.go | Working | Handles primitives, Output, Gather, Dict, List — no struct support |
| `validateParamTypes` in codegen.go | Strict | Rejects any type not in `typeMappings` (line 320) |

## Implementation Phases

### Phase 1: StructWrapper in `pkg/op`

Create the generic runtime wrapper that makes any Go struct a first-class Starlark value.

#### Per-type cache

Field mappings are cached per `reflect.Type` using `sync.Map`. The O(n) field walk happens **once per struct type** for the process lifetime. After caching:
- `Attr(name)` → O(1) map lookup + O(1) `Field(idx)` — no reflection search
- `AttrNames()` → returns cached slice — O(1)

```go
var typeCache sync.Map // reflect.Type → *cachedTypeInfo

type cachedTypeInfo struct {
    fields map[string]int  // snake_name → field index
    names  []string        // sorted attr names
    types  []reflect.Type  // field types (parallel to struct fields)
}
```

#### API

- [ ] Create `pkg/op/struct_wrapper.go`:
  - `GoValuer` interface: `GoValue() any` — for extracting Go values from Starlark wrappers
  - `StructWrapper` struct: `value reflect.Value`, `typeName string`, `info *cachedTypeInfo`
  - `WrapStruct(v any, typeName string) *StructWrapper` — wraps Go struct, resolves or creates `cachedTypeInfo`
  - Implements `starlark.HasAttrs`:
    - `Attr(name)` — cached O(1) field index lookup, `goToStarlark` conversion
    - `AttrNames()` — returns cached names slice
  - `GoValue() any` — returns the underlying Go struct value (via `value.Interface()`)
  - `goToStarlark(v any) (starlark.Value, error)` — converts primitives (string, int, int64, bool, float64, []string) and recursively wraps struct/ptr-to-struct fields via `WrapStruct`
  - `starlarkToGo(sv starlark.Value, goType reflect.Type) (reflect.Value, error)` — reverse: Starlark→Go for constructor kwargs. For `*StructWrapper` values, extracts GoValue. For nested structs, recurses.
  - `StructConstructor[T any](name string) starlark.Value` — generic factory: creates Starlark callable that unpacks kwargs, populates struct fields via `starlarkToGo` + cached field indices, returns `*StructWrapper`
  - Helper: `snakeToCamel(string) string`
  - Helper: `camelToSnake(string) string` (reuse existing if available, or share with codegen)

- [ ] Create `pkg/op/struct_wrapper_test.go`:
  - Test structs:
    ```go
    type Inner struct { Label string; Score int }
    type Outer struct { Name string; Count int; Active bool; Detail Inner }
    ```
  - `TestStructWrapper_Attr` — access fields by snake_case name
  - `TestStructWrapper_AttrNames` — lists all exported fields, sorted
  - `TestStructWrapper_GoValue` — round-trips: wrap → GoValue → type-assert → check fields
  - `TestStructWrapper_NestedStruct` — `Attr("detail")` returns another `*StructWrapper`; `.Attr("label")` works
  - `TestStructWrapper_Cache` — two wrappers of same type share `cachedTypeInfo`
  - `TestStructConstructor` — creates wrapper from kwargs, extracts Go struct
  - `TestStructConstructor_NestedKwarg` — constructor accepts `*StructWrapper` for struct fields
  - `TestStructConstructor_UnknownField` — errors on invalid field name
  - `TestGoToStarlark_Primitives` — string, int, int64, bool, float64, []string
  - `TestStarlarkToGo_Primitives` — reverse conversions for all primitives

**Files**:
- `pkg/op/struct_wrapper.go` — Create
- `pkg/op/struct_wrapper_test.go` — Create

### Phase 2: FillSlot GoValuer support

Add GoValuer handling to `FillSlot` so struct wrapper values store their underlying Go struct in slots.

- [ ] In `pkg/op/output.go`, add GoValuer check after Output/Gather and before primitive handling (after line 132):
  ```go
  if gv, ok := value.(GoValuer); ok {
      node.SetSlotImmediate(slotName, gv.GoValue())
      return nil
  }
  ```

**Files**:
- `pkg/op/output.go` — Modify FillSlot

### Phase 3: Code generator — struct param detection

Modify generate.star to auto-detect struct parameters and flag them for the Go-side codegen.

- [ ] Add `KNOWN_PARAM_TYPES` set (primitives + engine/context types that are NOT structs):
  ```python
  KNOWN_PARAM_TYPES = [
      "string", "bool", "int", "int64", "[]string",
      "os.FileMode", "map[string]any", "[]byte", "io.Writer",
  ]
  ```
- [ ] In `build_method_descriptors` (line 487): when a param's type is NOT in `KNOWN_PARAM_TYPES`, not a `func(` type, and IS in `structs_by_name` — flag it as `is_struct: True` on the param descriptor (don't expand to kwargs)
- [ ] Collect struct param types in the method descriptor: `"struct_params": ["Blob"]` — list of struct types this method uses as parameters
- [ ] Aggregate struct param types at the descriptor level for constructor registration
- [ ] Keep `+devlore:struct_param` for now (backward compat during transition) — can deprecate later

**Files**:
- `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star` — Modify

### Phase 4: Code generator — template function changes

Modify codegen.go to handle struct params in all three template contexts.

- [ ] Add `IsStruct bool` field to `paramInfo` struct (line 103)
- [ ] In `descriptorFromValue` (line 2290): read `is_struct` from descriptor dict, set `paramInfo.IsStruct`
- [ ] Remove `"Blob"` from `typeMappings` (line 164)
- [ ] `validateParamTypes` (line 320): skip validation when `p.IsStruct` is true (like StructType and Callable)
- [ ] `templateFuncImmediateUnpackArgs` (line 498): for struct params, unpack as `starlark.Value`
- [ ] New: `templateFuncStructExtract` — generates type assertion + GoValue extraction for struct params:
  ```go
  blobWrapper, ok := blobVal.(*op.StructWrapper)
  if !ok {
      return nil, fmt.Errorf("copy: blob: expected blob, got %s", blobVal.Type())
  }
  blob := blobWrapper.GoValue().(provider.Blob)
  ```
- [ ] `templateFuncImmediateProviderBody` (line 587): for struct params, use extracted Go variable in provider call (via `immediateArgExpr`)
- [ ] `templateFuncPlanUnpackArgs` (line 455): for struct params, include in UnpackArgs as `starlark.Value` (already the behavior for starlark-facing types)
- [ ] `templateFuncPlanFillSlots` (line 483): for struct params, call `op.FillSlot` normally — GoValuer handles storage
- [ ] `templateFuncGraphReaders` (line 880): for struct params, generate slot reader: `blob := slots["blob"].(provider.Blob)` (with correct provider prefix)
- [ ] `templateFuncImplArgs` (line 947): no change needed — struct param's GoName used directly

**Files**:
- `noblefactor-ops.binding-unification/internal/starlark/codegen.go` — Modify

### Phase 5: Template changes for constructor registration

Add struct constructor attributes to the immediate receiver template.

- [ ] In `immediate_receiver.go.template`: extend `Attr` switch to include struct constructor entries:
  ```go
  {{- range .StructParams}}
  case "{{.SnakeName}}":
      return op.StructConstructor[{{providerTypePrefix $}}{{.GoType}}]("{{.SnakeName}}"), nil
  {{- end}}
  ```
- [ ] In `AttrNames`: include struct constructor names
- [ ] In `graph_actions.go.template`: no struct-specific template changes needed — `graphReaders` and `implArgs` handle it via codegen.go template functions

**Files**:
- `star/extensions/com.noblefactor.devlore.Actions/templates/immediate_receiver.go.template` — Modify

### Phase 6: Regenerate and test

- [ ] Run `make` to regenerate all providers
- [ ] Verify `file.Copy` projects in all three artifacts (immediate, planned, graph actions)
- [ ] Update `pkg/op/provider/file/gen/testdata/immediate_test.star` — `file.copy` now takes a blob struct:
  ```python
  b = file.blob(source_path=fixture)
  copy_path = file.copy(path=tmp_dir + sep + "copied.txt", blob=b, mode=0o644)
  ```
- [ ] Update `pkg/op/provider/file/gen/testdata/planned_test.star` — same struct-based call
- [ ] Update test helpers in `integration_test.go` and `actions_test.go`
- [ ] Verify existing providers still work (staranalysis, starcode, starsources — still use `+devlore:struct_param`)
- [ ] `make check` — all tests pass

**Files**:
- `pkg/op/provider/file/gen/testdata/immediate_test.star` — Modify
- `pkg/op/provider/file/gen/testdata/planned_test.star` — Modify
- `pkg/op/provider/file/gen/integration_test.go` — Modify
- `pkg/op/provider/file/gen/actions_test.go` — Modify

## Starlark API Example

```python
# Construct a blob (struct wrapper backed by Go file.Blob)
b = file.blob(source_path="/src/data.bin", size=1024)

# Pass as a structured value — not individual kwargs
result = file.copy(destination="/dest/data.bin", blob=b, mode=0o644)

# Struct values have attributes
print(b.source_path)  # "/src/data.bin"
print(b.size)         # 1024
```

## Transitive Closure and Caching

When `goToStarlark` encounters a struct field, it wraps it in another `StructWrapper` backed by the same per-type cache. When `starlarkToGo` encounters a `*StructWrapper` kwarg, it extracts the Go struct. This handles nested structs automatically:

```
Blob{SourcePath string, Size int64}
  → StructWrapper{value: reflect.ValueOf(blob), info: cachedBlobInfo}
    → Attr("source_path") → info.fields["source_path"]=0 → Field(0) → starlark.String
    → Attr("size")        → info.fields["size"]=1 → Field(1) → starlark.MakeInt64

Outer{Name string, Detail Inner}
  → StructWrapper{value: reflect.ValueOf(outer), info: cachedOuterInfo}
    → Attr("detail") → Field(1).Interface() → goToStarlark → WrapStruct(inner, "inner")
      → StructWrapper{value: reflect.ValueOf(inner), info: cachedInnerInfo}  ← same cache
```

The type cache (`sync.Map`) ensures the O(n) field walk happens exactly once per struct type. All `StructWrapper` instances of the same type share the same `cachedTypeInfo`.

## Verification

1. `make check` in devlore-cli — all tests pass
2. `make check` in noblefactor-ops — all codegen tests pass
3. New `StructWrapper` unit tests pass (Phase 1)
4. `file.Copy` appears in all three generated files
5. `file.blob(source_path=..., size=...)` constructor works from Starlark
6. Struct values flow through planned graph as Go structs in slots
7. Graph actions reconstruct Go structs from slots
8. Existing `+devlore:struct_param` providers unchanged

## Open Questions

- [ ] Should `+devlore:struct_param` be removed in this PR or deprecated for a follow-up? (staranalysis, starcode, starsources use it)
- [ ] Constructor naming: `file.blob(...)` (on receiver) vs `blob(...)` (global builtin)?
- [ ] Should the `StructWrapper` constructor accept positional args for single-field structs, or kwargs only?

## Related Documents

- Issue #167 — Code generator flags Copy as unprojectable due to unmapped Blob type
- [Binding Unification Plan](./binding-unification.md) — Parent plan
