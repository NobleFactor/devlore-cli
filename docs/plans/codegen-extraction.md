# Plan: Extract devlore-cli Codegen from noblefactor-ops

## Context

noblefactor-ops contains ~1550 lines of devlore-cli-specific code generation
logic in `internal/starlark/codegen.go`. This code generates Go source files
for devlore-cli's provider binding infrastructure — it hardcodes
`github.com/NobleFactor/devlore-cli/pkg/op` imports, references `op.RegisterBinding`,
`op.ProviderBinding`, `op.WrapReceiver`, etc., and validates devlore-cli-specific
patterns (compensable methods, return signatures, callable parameters).

noblefactor-ops is intended for general-purpose tooling. Project-specific
code belongs in the project. This plan moves all devlore-cli-specific codegen
to devlore-cli while keeping general Go AST introspection in noblefactor-ops.

## Current State

**noblefactor-ops** (`internal/starlark/`):
- `receiver_go.go` — `GoReceiver` with 16 methods:
  - 13 GENERAL (go.methods, go.structs, go.funcs, go.const_groups,
    go.callable, go.calls, go.composites, go.return_string,
    go.return_strings, go.raw_string, go.type_doc, go.metrics, go.deps)
  - 3 DEVLORE-SPECIFIC (go.generate, go.template, go.mapping)
- `codegen.go` — 1550 lines: template constants, template helper functions,
  descriptor parsing, return signature validation, `goGenerate()`/`goTemplate()`/`goMapping()`
- `codegen_test.go` — tests for codegen
- GoReceiver embeds `op.Receiver` from devlore-cli (Go module dependency)

**devlore-cli** (`star/extensions/com.noblefactor.devlore.Actions/`):
- `commands/generate.star` — orchestrator, calls `go.generate(template, descriptor)`
- `templates/` — 7 local `.go.template` files (all override builtins via `LOCAL_TEMPLATES`)
- `StructConverterTemplate` builtin is dead code (struct converters no longer generated; line 1047)
- The ONLY call into noblefactor-ops codegen is `go.generate(template_content, descriptor)` at line 1120

## What Will Be True When This Plan Is Complete

1. `codegen.go` and `codegen_test.go` are deleted from noblefactor-ops
2. `go.generate()`, `go.template()`, `go.mapping()` are removed from GoReceiver
3. noblefactor-ops gains a general-purpose `go.render(template, data)` method
4. All devlore-cli-specific validation and template logic lives in `generate.star`
5. `make build` / `make test` pass in both repos

## Approach: Replace `go.generate()` with General `go.render()`

`go.generate()` currently does 4 things:
1. Parses the Starlark descriptor dict into a Go `generateDescriptor` struct
2. Validates return signatures (devlore-cli patterns)
3. Executes a Go `text/template` against the descriptor
4. Formats output via `go/format`

Steps 1-2 are devlore-cli-specific. Steps 3-4 are general.

**Solution**: Add `go.render(template_string, data_dict)` — a general-purpose
method that takes a template string and a raw Starlark dict, executes the
template, and runs `go/format`. No descriptor parsing, no validation, no
devlore-cli types. The dict values are accessed in templates via `.Key` syntax
through a thin Go adapter.

Devlore-cli-specific logic (validation, template helper computations) moves
to Starlark in `generate.star`. Template helper functions like
`providerFieldInit` become Starlark functions that pre-compute Go code strings
and pass them as data fields. Templates use `{{.ProviderFieldInitCode}}`
instead of `{{providerFieldInit .}}`.

### `go.render()` Design

```
go.render(template_string, data_dict) → string
```

- `template_string`: Go `text/template` content
- `data_dict`: Starlark dict (or struct) — keys become template fields
- Returns: `go/format`-formatted Go source code
- Template accesses: `.Key` looks up `"Key"` in the dict
- Nested dicts and lists work recursively
- Built-in template functions only (`camelToSnake`, `lcFirst`)
- ~80 lines of Go (adapter + method)

## Implementation Phases

### Phase 0: Add `go.render()` to noblefactor-ops

Non-breaking. Additive only — `go.generate()` continues to work.

- [ ] Create `internal/starlark/render.go` — `goRender()` method + dict-to-template adapter
- [ ] Add `go.render` case to `Attr()` and `AttrNames()` in `receiver_go.go`
- [ ] Add `go.format(code_string)` for standalone formatting (useful for pre-computed code)
- [ ] Tests in `render_test.go`

**Files (noblefactor-ops)**:
- `internal/starlark/render.go` — Create
- `internal/starlark/render_test.go` — Create
- `internal/starlark/receiver_go.go` — Modify (add to Attr switch)

### Phase 1: Move validation + template helpers to Starlark

Non-breaking. Internal to devlore-cli — `go.generate()` still called.

- [ ] Port `validateReturnSignature`, `validateCompensableReturn`,
      `validateImmediateReturn` to Starlark (pure string parsing)
- [ ] Port `inferContentModel` to Starlark
- [ ] Port template helper logic to Starlark functions that produce
      Go code strings:
  - `providerFieldInit` → `compute_provider_field_init(descriptor)`
  - `providerInit` → `compute_provider_init(descriptor)`
  - `providerTypePrefix` → `compute_provider_type_prefix(descriptor)`
  - `paramNamesList` → `compute_param_names_list(methods)`
  - `needsOpImport` → already deterministic from descriptor data
- [ ] Test: `make test` still passes (no behavioral change yet)

**Files (devlore-cli)**:
- `star/extensions/.../commands/generate.star` — Modify (add helper functions)

### Phase 2: Switch to `go.render()` + update templates

Non-breaking for noblefactor-ops. devlore-cli stops calling `go.generate()`.

- [ ] Update `gen_file()` to call `go.render()` instead of `go.generate()`
- [ ] Pre-compute all template function values in `gen_file()` before
      calling `go.render()` — add them to the data dict
- [ ] Update all 7 local templates to use `{{.PreComputedField}}`
      instead of `{{templateFunc .}}`
- [ ] Remove `go.template()` fallback from `load_template()` (dead path)
- [ ] Handle `result.flagged` — move flagging logic to Starlark
      (before the `go.render()` call)
- [ ] Test: `make build` regenerates all providers; `make test` passes

**Files (devlore-cli)**:
- `star/extensions/.../commands/generate.star` — Modify
- `star/extensions/.../templates/*.go.template` — Modify (all 7)

### Phase 3: Delete codegen from noblefactor-ops

Breaking change for anyone calling `go.generate()` / `go.template()` /
`go.mapping()`. Only devlore-cli calls them, and Phase 2 removes those calls.

- [ ] Remove `go.generate`, `go.template`, `go.mapping` from `Attr()` switch
- [ ] Delete `codegen.go` entirely (1550 lines)
- [ ] Delete `codegen_test.go`
- [ ] Remove `builtinTemplates` map
- [ ] Verify GoReceiver's `AttrNames()` reflects the removal
- [ ] Test: noblefactor-ops builds; devlore-cli `make test` passes

**Files (noblefactor-ops)**:
- `internal/starlark/codegen.go` — Delete
- `internal/starlark/codegen_test.go` — Delete
- `internal/starlark/receiver_go.go` — Modify (remove 3 cases from Attr)

## Template Function Migration Detail

Current Go template functions and their Starlark replacements:

| Go template func | What it produces | Starlark approach |
|---|---|---|
| `providerFieldInit` | Multi-line Go struct init from `+devlore:bind` fields | Pre-compute string in `compute_provider_field_init()` |
| `providerInit` | Full ImmediateFactory body | Pre-compute string in `compute_provider_init()` |
| `providerTypePrefix` | `"provider."` or `""` | Simple dict field: `descriptor["type_prefix"]` |
| `paramNamesList` | `"param1", "param2?"` | Pre-compute string from method params |
| `needsOpImport` | bool | Determined from descriptor data |
| `converterFunc` | Dead — struct converters no longer generated | N/A |
| `camelToSnake` | `CamelCase` → `snake_case` | Already exists in generate.star as `to_snake()` |
| `lcFirst` | Lowercase first char | Trivial Starlark: `s[0].lower() + s[1:]` |

## Verification

After each phase:

```bash
# noblefactor-ops
cd ../noblefactor-ops && go build -o bin/star ./cmd/star && go test ./internal/starlark/...

# devlore-cli
make build    # rebuilds star + regenerates all providers
make vet
make test
```

After Phase 2 specifically: diff every `gen/*.gen.go` file against its
pre-migration version. The generated output must be byte-identical.

## Open Questions

- [ ] Should `go.render()` support custom template functions via a Starlark
      dict of callables? Or is pre-computation sufficient? (Pre-computation
      is simpler; custom funcs add complexity to the bridge.)
- [ ] The `go.mapping()` method generates operation-mapping YAML. Is this
      still used? If so, it can move to Starlark using `yaml.encode()`.
