# Phase 8: Flow Doc Comments Through Code Generation

**Status**: Planning

## Summary

Provider methods already have doc comments. The generation pipeline already
extracts and carries them. Two of three templates silently drop them. Fix the
templates, regenerate, and the knowledge extract pipeline produces a real API
reference instead of an empty skeleton.

## Current State

The doc comment pipeline:

```
Provider method doc comment
  → go.methods() extracts doc field         ✓ works
  → generate.star passes m.doc             ✓ works
  → graph_actions template: {{.Doc}}       ✓ emits doc comment
  → plan_receiver template                 ✗ drops doc
  → realtime_receiver template             ✗ drops doc
  → knowledge extract reads Go source      ✓ works (reads what's there)
  → reference.yaml doc field               ✗ empty (because templates dropped it)
```

## Changes

### Step 1: Add `{{.Doc}}` to templates

Both local templates in `star/extensions/com.noblefactor.devlore.Ops/templates/`:

**plan_receiver.go.template** — add doc comment above each method:

```diff
 {{range .Methods}}
+// {{.SnakeName}} {{.Doc}}
 func (p *{{$.StructName}}Plan) {{.SnakeName}}(...)
```

**realtime_receiver.go.template** — same:

```diff
 {{range .Methods}}
+// {{.SnakeName}} {{.Doc}}
 func (r *{{$.StructName}}Receiver) {{.SnakeName}}(...)
```

Matches the existing pattern in `graph_actions.go.template`:
```go
// {{.GoName}} — {{.Doc}}
```

### Step 2: Regenerate all `_gen.go` files

Run `devlore ops.generate` for each provider. The regenerated files will now
carry doc comments from the Provider source.

| Provider | Plan binding | Receiver | Graph actions |
|----------|-------------|----------|---------------|
| file | plan_file_gen.go | receiver_file_gen.go | actions_gen.go (already has docs) |
| pkg | plan_package_gen.go | receiver_package_gen.go | actions_gen.go (already has docs) |
| service | plan_service_gen.go | receiver_service_gen.go | actions_gen.go (already has docs) |
| shell | plan_shell_gen.go | receiver_shell_gen.go | actions_gen.go (already has docs) |
| git | plan_git_gen.go | receiver_git_gen.go | actions_gen.go (already has docs) |
| archive | plan_archive_gen.go | receiver_archive_gen.go | actions_gen.go (already has docs) |
| net | plan_net_gen.go | receiver_net_gen.go | actions_gen.go (already has docs) |
| encryption | plan_encryption_gen.go | receiver_encryption_gen.go | actions_gen.go (already has docs) |
| template | plan_template_gen.go | receiver_template_gen.go | actions_gen.go (already has docs) |
| content | plan_content_gen.go | — | actions_gen.go (already has docs) |

### Step 3: Add `--format` flag to knowledge extract

**extension.yaml** — add format flag:

```yaml
- name: format
  type: string
  default: "all"
  help: "Output format: yaml, md, or all (default: all)"
```

**extract.star** — gate output on format:

```python
fmt = ctx.flags.get("format", "all")

if fmt in ("yaml", "all"):
    # write reference.yaml

if fmt in ("md", "all"):
    # write reference.md
```

Both renderings read from the same `api` dict. The dict is the single source
of truth; yaml and md are output formats.

### Step 4: Regenerate knowledge artifacts

Run knowledge extract against the updated devlore-cli source. With doc comments
now in the `_gen.go` files, reference.yaml's `doc` fields will be populated.

**Before** (current):
```yaml
- doc: ""
  full_name: plan.file.copy
  slots: [source, path]
```

**After**:
```yaml
- doc: "Copy writes content to path with the given mode."
  full_name: plan.file.copy
  slots: [source, path]
```

## Known Limitations

**slot_docs remain empty.** Provider doc comments describe the method, not
individual parameters. Populating `slot_docs` would require either structured
parameter annotations in Provider source or a parser that extracts per-parameter
descriptions from prose. This is a future enhancement — the method-level doc
is the important win for LLM consumers.

## Files

| Repo | File | Action |
|------|------|--------|
| devlore-cli | `star/.../templates/plan_receiver.go.template` | Modify: add `{{.Doc}}` |
| devlore-cli | `star/.../templates/realtime_receiver.go.template` | Modify: add `{{.Doc}}` |
| devlore-cli | `star/.../Knowledge/extension.yaml` | Modify: add `--format` flag |
| devlore-cli | `star/.../Knowledge/commands/extract.star` | Modify: gate output on format |
| devlore-cli | `internal/starlark/plan_*_gen.go` (10 files) | Regenerate |
| devlore-cli | `internal/starlark/receiver_*_gen.go` (9 files) | Regenerate |
| devlore-registry | `knowledge/.../reference.yaml` | Regenerate |
| devlore-registry | `knowledge/.../reference.md` | Regenerate |

## Verification

1. `go build ./...` — compiles
2. `go test ./internal/starlark/...` — bindings pass
3. `go test ./internal/execution/...` — compensation pass
4. Inspect a regenerated `plan_*_gen.go` — doc comment present above each method
5. Inspect a regenerated `receiver_*_gen.go` — doc comment present above each method
6. Inspect `reference.yaml` — `doc` fields populated, not empty strings
7. Inspect `reference.md` — descriptions appear in method sections
