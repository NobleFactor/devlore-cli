# Phase 9: Per-Slot Documentation

**Status**: Complete

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
  → go.methods() extracts: param docs                       ✓ parseParamDocs()
  → template emits: // Slots: block                         ✓ {{slotDocs .}}
  → extract.star parses: // Slots: block                    ✓ already implemented
  → reference.yaml: slot_docs: {source: "...", path: "..."}  ✓ regenerated
```

The only populated `slot_docs` today come from hand-written methods (e.g.,
`plan.source` in plan_root.go, which has a manually written `// Slots:` block).

## Design

### Doc comment convention

Provider methods adopt a `Parameters:` section in their doc comments:

```go
// Link creates a symlink at path pointing to source. Idempotent: if the
// symlink already points correctly, it's a no-op (returns nil state).
//
// Parameters:
//   - source: Absolute path to the symlink target
//   - path: Absolute path where the symlink will be created
func (p *Provider) Link(source, path string) (string, map[string]any, error) {
```

The convention uses `Parameters:` in the Provider source (Go domain, with
camelCase param names) and `Slots:` in the generated bindings (Starlark domain,
with snake_case slot names).

### Pipeline flow

```
Provider doc comment "Parameters:" section
  → go.methods() parses param docs, adds doc field to each param
  → generate.star passes p.doc for each param in descriptor
  → template emits "// Slots:" block in generated method doc comment
  → extract.star parses "// Slots:" block (already implemented)
  → reference.yaml slot_docs populated
```

## Changes

### Step 1: Add `Doc` to paramInfo (noblefactor-ops) ✅

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

### Step 2: Parse param docs in go.methods() (noblefactor-ops) ✅

**`internal/starlark/receiver_go.go`** — add `parseParamDocs(docText)` function:

- Finds `Parameters:` section in the doc comment text
- Parses `- name: description` lines
- Returns `(cleanDoc string, map[string]string)` — clean doc with section stripped, and param docs keyed by param name

Wire into `go.methods()`: call `parseParamDocs(rawDoc)` before `extractParams()`,
pass `paramDocs` map into `extractParams()` which populates the `doc` field on
each param struct:

```go
cleanDoc, paramDocs := parseParamDocs(rawDoc)
params := extractParams(fn.Type.Params, paramDocs)
```

The params returned to Starlark gain a `doc` field:

```python
m.params[0].name  # "source"
m.params[0].doc   # "Absolute path to the symlink target"
```

### Step 3: Pass param docs through generate.star (devlore-cli) ✅

**`star/.../Actions/commands/generate.star`** — include `doc` in param descriptors:

```diff
 for p in m.params:
     params.append({
         "name": p.name,
         "type": p.type,
         "variadic": p.variadic,
+        "doc": p.doc,
     })
```

### Step 4: Emit `// Slots:` in templates (devlore-cli) ✅

Both templates updated to use `{{docComment .SnakeName .Doc}}{{slotDocs .}}`:

- **`plan_receiver.go.template`** — `{{slotDocs .}}` appended after `{{docComment}}`
- **`realtime_receiver.go.template`** — same pattern

**`slotDocs`** and **`hasSlotDocs`** — template functions in noblefactor-ops
(`receiver_go_gen.go`): `slotDocs` emits a `// Slots:` block from structured
param data, filtering to starlark-facing non-content params with snake_case
names. `hasSlotDocs` returns true if any qualifying param has a doc.

### Step 5: Add param docs to all Provider methods (devlore-cli) ✅

Added `Parameters:` sections to every Provider method that has slotted parameters.
Param names use Go camelCase (e.g., `backupSuffix`, `pruneBoundary`, `templateData`).
The `slotDocs` template function converts to snake_case for the generated `Slots:` output.

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

Methods with only internal params (dryRun, logger, writer) need no `Parameters:`
section — those params are not slots. The `content` provider no longer exists
(deleted in Phase 2C).

### Step 6: Regenerate and verify

1. Regenerate all `_gen.go` files via `star devlore actions generate` ✅
2. Run `make build` and `make test` ✅
3. Run knowledge extract in devlore-registry
4. Verify `slot_docs` populated in reference.yaml

Steps 1-2 complete. Steps 3-4 require devlore-registry.

## Files

| Repo | File | Action |
|------|------|--------|
| noblefactor-ops | `internal/starlark/receiver_go.go` | Modify: add `parseParamDocs()`, wire into `go.methods()` |
| noblefactor-ops | `internal/starlark/receiver_go_gen.go` | Modify: add `Doc` to `paramInfo`, add `hasSlotDocs` helper |
| noblefactor-ops | `internal/starlark/receiver_go_test.go` | Add: test for `parseParamDocs()` |
| noblefactor-ops | `internal/starlark/receiver_go_gen_test.go` | Add: test for `hasSlotDocs` helper, update test templates |
| devlore-cli | `star/.../Actions/commands/generate.star` | Modify: pass `p.doc` |
| devlore-cli | `star/.../Actions/templates/plan_receiver.go.template` | Modify: emit `// Slots:` |
| devlore-cli | `star/.../Actions/templates/realtime_receiver.go.template` | Modify: emit `// Slots:` |
| devlore-cli | `internal/execution/provider/file/provider.go` | Modify: add `Parameters:` docs |
| devlore-cli | `internal/execution/provider/pkg/provider.go` | Modify: add `Parameters:` docs |
| devlore-cli | `internal/execution/provider/service/provider.go` | Modify: add `Parameters:` docs |
| devlore-cli | `internal/execution/provider/shell/provider.go` | Modify: add `Parameters:` docs |
| devlore-cli | `internal/execution/provider/git/provider.go` | Modify: add `Parameters:` docs |
| devlore-cli | `internal/execution/provider/archive/provider.go` | Modify: add `Parameters:` docs |
| devlore-cli | `internal/execution/provider/net/provider.go` | Modify: add `Parameters:` docs |
| devlore-cli | `internal/execution/provider/encryption/provider.go` | Modify: add `Parameters:` docs |
| devlore-cli | `internal/execution/provider/template/provider.go` | Modify: add `Parameters:` docs |
| devlore-cli | `internal/starlark/plan_*_gen.go` (9 files) | Regenerate |
| devlore-registry | `knowledge/.../reference.yaml` | Regenerate |
| devlore-registry | `knowledge/.../reference.md` | Regenerate |

## Bug Fixes (discovered during implementation)

- **`generate.star` trailing-slash vulnerability**: `provider = path.split("/")[-1]`
  produced empty string when `--source` had a trailing slash. Fixed with
  `rstrip("/")` on the source path. This was the root cause of the bogus
  `plan__gen.go` file (empty provider name, broken identifiers like
  `MustGet(".render")` and `registerPlan("")`).

## Verification

1. `make build` — compiles ✅
2. `make test` — all tests pass ✅
3. Inspect generated `plan_*_gen.go` — `Slots:` blocks with descriptions ✅
4. Inspect generated `actions_gen.go` — single-line docs, no `Parameters:` leak ✅
5. Regenerate reference.yaml in devlore-registry ✅
6. Verify no `slot_docs: {}` remains for methods that have slotted params ✅
   (14 remain — all are package/phase context attributes and flow actions, not provider methods)
