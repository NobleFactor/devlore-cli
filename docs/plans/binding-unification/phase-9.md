# Phase 9: Per-Slot Documentation

**Status**: Planning

## Summary

Populate `slot_docs` in the API reference by flowing per-parameter documentation
from Provider method doc comments through the generation pipeline. The knowledge
extract parser already handles `// Slots:` blocks — the templates just need to
emit them.

## Current State

```
Provider doc comment: "Link creates a symlink at path pointing to source."
  → go.methods() extracts: doc="Link creates a symlink..."  ✓
  → go.methods() extracts: params=[{name: source}, {name: path}]  ✓
  → go.methods() extracts: param docs                       ✗ not parsed
  → template emits: // Slots: block                         ✗ not emitted
  → extract.star parses: // Slots: block                    ✓ already implemented
  → reference.yaml: slot_docs: {}                           ✗ empty
```

The only populated `slot_docs` today come from hand-written methods (e.g.,
`plan.source` in plan_root.go, which has a manually written `// Slots:` block).

## Design

### Doc comment convention

Provider methods adopt a `Params:` section in their doc comments:

```go
// Link creates a symlink at path pointing to source. Idempotent: if the
// symlink already points correctly, it's a no-op (returns nil state).
//
// Params:
//   - source: path to the existing file to link to
//   - path: destination path for the symlink
func (p *Provider) Link(source, path string) (map[string]any, error) {
```

The convention mirrors the `Slots:` format that extract.star already parses, but
uses `Params:` in the Provider source (Go domain) and `Slots:` in the generated
bindings (Starlark domain).

### Pipeline flow

```
Provider doc comment "Params:" section
  → go.methods() parses param docs, adds doc field to each param
  → generate.star passes p.doc for each param in descriptor
  → template emits "// Slots:" block in generated method doc comment
  → extract.star parses "// Slots:" block (already implemented)
  → reference.yaml slot_docs populated
```

## Changes

### Step 1: Add `Doc` to paramInfo (noblefactor-ops)

**`internal/starlark/receiver_go_gen.go`** — extend paramInfo:

```diff
 type paramInfo struct {
     GoName    string
     SnakeName string
     GoType    string
     Variadic  bool
+    Doc       string
 }
```

Update `methodInfoFromValue()` to read `doc` from each param entry.

### Step 2: Parse param docs in go.methods() (noblefactor-ops)

**`internal/starlark/receiver_go.go`** — add `parseParamDocs(docText)` function:

- Finds `Params:` section in the doc comment text
- Parses `- name: description` lines
- Returns `map[string]string` keyed by param name

Wire into `go.methods()`: after calling `extractParams()`, call `parseParamDocs()`,
then merge doc strings into the param structs returned to Starlark:

```go
paramDocs := parseParamDocs(doc)
params := extractParams(fn.Type.Params)
// attach doc to each param in the Starlark list
```

The params returned to Starlark gain a `doc` field:

```python
m.params[0].name  # "source"
m.params[0].doc   # "path to the existing file to link to"
```

### Step 3: Pass param docs through generate.star (devlore-cli)

**`star/.../Ops/commands/generate.star`** — include `doc` in param descriptors:

```diff
 for p in m.params:
     params.append({
         "name": p.name,
         "type": p.type,
         "variadic": p.variadic,
+        "doc": p.doc,
     })
```

### Step 4: Emit `// Slots:` in templates (devlore-cli)

**`star/.../Ops/templates/plan_receiver.go.template`**:

```diff
 {{range .Methods}}
 // {{.SnakeName}} {{.Doc}}
+{{- if hasSlotDocs .}}
+//
+// Slots:
+{{- range .Params}}{{- if .Doc}}
+//   - {{.SnakeName}}: {{.Doc}}
+{{- end}}{{- end}}
+{{- end}}
 func (p *{{$.StructName}}Plan) {{.SnakeName}}(...)
```

**`star/.../Ops/templates/realtime_receiver.go.template`** — same pattern.

**`hasSlotDocs`** — new template helper in noblefactor-ops (`receiver_go_gen.go`):
returns true if any param in the method has a non-empty Doc field.

### Step 5: Add param docs to all Provider methods (devlore-cli)

Add `Params:` sections to every Provider method that has slotted parameters:

| Provider | Methods needing param docs |
|----------|--------------------------|
| file | Link(source, path), Copy(path, mode, content), Write(path, content), Remove(path), Move(source, path), Mkdir(path), Source(path) |
| pkg | Install(packages, manager, cask), Upgrade(packages, manager), Remove(pkg, manager) |
| service | Start(name), Stop(name), Restart(name), Enable(name), Disable(name) |
| shell | Shell(command), PowerShell(command) |
| git | Clone(url, path), Checkout(repo, ref), Pull(repo) |
| archive | Extract(archive, prefix) |
| net | Download(url) |
| encryption | Decrypt(source) |
| template | Render(source) |
| content | Literal(content) |

Methods with only internal params (dryRun, logger, writer) need no `Params:`
section — those params are not slots.

### Step 6: Regenerate and verify

1. Regenerate all `_gen.go` files via `devlore ops.generate`
2. Run `go build ./...` and `go test ./...`
3. Run knowledge extract
4. Verify `slot_docs` populated in reference.yaml

## Files

| Repo | File | Action |
|------|------|--------|
| noblefactor-ops | `internal/starlark/receiver_go.go` | Modify: add `parseParamDocs()`, wire into `go.methods()` |
| noblefactor-ops | `internal/starlark/receiver_go_gen.go` | Modify: add `Doc` to `paramInfo`, add `hasSlotDocs` helper |
| noblefactor-ops | `internal/starlark/receiver_go_test.go` | Add: test for `parseParamDocs()` |
| noblefactor-ops | `internal/starlark/receiver_go_gen_test.go` | Add: test for `hasSlotDocs` helper, update test templates |
| devlore-cli | `star/.../Ops/commands/generate.star` | Modify: pass `p.doc` |
| devlore-cli | `star/.../Ops/templates/plan_receiver.go.template` | Modify: emit `// Slots:` |
| devlore-cli | `star/.../Ops/templates/realtime_receiver.go.template` | Modify: emit `// Slots:` |
| devlore-cli | `internal/execution/provider/file/provider.go` | Modify: add `Params:` docs |
| devlore-cli | `internal/execution/provider/pkg/provider.go` | Modify: add `Params:` docs |
| devlore-cli | `internal/execution/provider/service/provider.go` | Modify: add `Params:` docs |
| devlore-cli | `internal/execution/provider/shell/provider.go` | Modify: add `Params:` docs |
| devlore-cli | `internal/execution/provider/git/provider.go` | Modify: add `Params:` docs |
| devlore-cli | `internal/execution/provider/archive/provider.go` | Modify: add `Params:` docs |
| devlore-cli | `internal/execution/provider/net/provider.go` | Modify: add `Params:` docs |
| devlore-cli | `internal/execution/provider/encryption/provider.go` | Modify: add `Params:` docs |
| devlore-cli | `internal/execution/provider/template/provider.go` | Modify: add `Params:` docs |
| devlore-cli | `internal/execution/provider/content/provider.go` | Modify: add `Params:` docs |
| devlore-cli | `internal/starlark/plan_*_gen.go` (10 files) | Regenerate |
| devlore-cli | `internal/starlark/receiver_*_gen.go` (9 files) | Regenerate |
| devlore-registry | `knowledge/.../reference.yaml` | Regenerate |
| devlore-registry | `knowledge/.../reference.md` | Regenerate |

## Verification

1. `go build ./...` — compiles
2. `go test ./internal/starlark/...` — bindings pass
3. `go test ./internal/execution/...` — compensation pass
4. Inspect reference.yaml — `slot_docs` populated for all slotted methods
5. Inspect reference.md — slot descriptions appear in method tables
6. Verify no `slot_docs: {}` remains for methods that have slotted params
