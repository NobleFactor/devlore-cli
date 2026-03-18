---
title: "Star Provider Migration"
issue: TBD
status: draft
created: 2026-03-17
updated: 2026-03-17
---

# Plan: Star Provider Migration

## Summary

Migrate all hand-coded Starlark receivers in noblefactor-ops `internal/starlark/` to
framework providers backed by the devlore-cli `pkg/op` toolkit. Receivers that map to
existing devlore-cli providers (file, json, yaml, star*) reuse them directly — with
json and yaml gaining Resource types and schema validation. Receivers that are
noblefactor-ops-specific (shellcheck, lint, setup, config, commands) become new providers
under `internal/provider/`. All `.star` scripts are updated for the new API surface.
The hand-coded `Receiver` base type and all `receiver_*.go` files are deleted when
migration is complete.

## Goals

1. **Eliminate hand-coded receivers** — all Starlark bindings go through the framework's
   reflection-based dispatch + generated receiver factories
2. **Reuse devlore-cli providers** — file, json (with Resource), yaml (with Resource),
   and star* providers are shared across devlore-cli and noblefactor-ops
3. **Consistent API contract** — every receiver follows the same pattern: `ProviderBase`
   embed, `+devlore:` annotations, generated `gen/` package, `op.Announce` registration
4. **Unblock future extensibility** — framework providers get compensation, dry-run
   support, and graph-mode for free; hand-coded receivers cannot
5. **Structured document resources** — json.Resource and yaml.Resource hold parsed Go
   values, enabling schema validation and format conversion without Starlark↔Go roundtrips

## Current State

| Receiver | Status | Backing |
| --- | --- | --- |
| json | ✅ Framework | devlore-cli `pkg/op/provider/json` (codec only, no Resource) |
| yaml | ✅ Framework | devlore-cli `pkg/op/provider/yaml` (codec only, no Resource) |
| regexp | ✅ Framework | devlore-cli `pkg/op/provider/regexp` |
| ui | ✅ Framework | devlore-cli `pkg/op/provider/ui` |
| goast | ✅ Framework | noblefactor-ops `internal/provider/goast` |
| file | ❌ Hand-coded | `receiver_file.go` (14 methods) |
| schema | ❌ Hand-coded | `receiver_schema.go` (1 method) |
| shellcheck | ❌ Hand-coded | `receiver_shellcheck.go` (4 methods) |
| lint | ❌ Hand-coded | `receiver_lint.go` (4 methods) |
| setup | ❌ Hand-coded | `receiver_setup.go` (7 methods) |
| config | ❌ Hand-coded | `receiver_config.go` (3 methods) |
| starlark_parse | ❌ Hand-coded | `receiver_starlark.go` (3 methods) |
| commands | ❌ Hand-coded | `receiver_commands.go` (7 methods) |

**Receiver base type** (`receiver.go`): local copy of devlore-cli receiver base —
explicitly marked for deletion once migration completes.

**Singleton module** (`receivers.go`): declares package-level `Schema`, `Shellcheck`,
`Config`, `StarlarkParse` singletons — deleted when providers replace them.

## Requirements

### R1: file.Provider must support immediate mode

devlore-cli's `file.Provider` is currently `+devlore:access=planned` (graph-mode only).
noblefactor-ops star runs in immediate mode. The provider must be changed to
`+devlore:access=both` and regenerated so `NewExecuting` is emitted.

No new methods are needed:

- **`file.list` → `file.glob`**: The hand-coded `file.list(path)` is a single-level
  directory listing. Callers migrate to `file.glob(path + "/*")`, which already exists
  on `file.Provider`. No `.star` files currently call `file.list`.
- **`file.walk_tree` stays as-is**: `file.Provider` already has `WalkTree` with a Go
  callback signature. The code generator's function parameter bridge (see
  `star-consumes-pkg-op.md` Phase 4a) handles bridging `starlark.Callable` to Go
  closures. No `.star` files currently call `file.walk_tree`.

### R2: json.Resource and yaml.Resource — structured document types

The hand-coded `schema.validate(data, schema)` receiver is eliminated. Schema validation
moves to the json and yaml providers via new Resource types that hold parsed Go values.

**Why resources, not string-based validation**: Without resources, validating a decoded
document requires a roundtrip: Starlark dict → Go → re-encode to string → parse again
for schema validation. Resources hold the parsed Go value (`map[string]any`, `[]any`)
internally, enabling validation and re-encoding without crossing the Starlark↔Go boundary.

**json.Resource and yaml.Resource are distinct types, not mem.Resource specializations.**
mem.Resource holds opaque bytes with a content-type label. json/yaml resources hold
*structured* data with a schema contract. Key differences from mem.Resource:

- **Lazy parsed representation**: A `parsed any` field caches the decoded Go value.
  mem.Resource has no such field — it stores `[]byte` and treats content as opaque.
  Adding `parsed` to mem.Resource would pollute it for all content types (callable,
  template, etc.) that don't need it.
- **Validation is a resource method**: `doc.validate(schema)` operates on the internal
  Go value directly. On mem.Resource this would require ContentType dispatch.
- **Format-aware encoding**: `json.encode(yaml_resource)` converts YAML→JSON by reading
  the resource's `parsed` field (Go→string). No Starlark hop, no re-parsing.
- **URI semantics**: `json:<qualifier>` and `yaml:<qualifier>` communicate format in the
  scheme. `mem:json/<qualifier>` buries format in the opaque path.

Both follow the mem.Resource architectural pattern: embed `op.ResourceBase`,
`AnnounceResource` in init, `ResourceFromValue` constructor, URI-based reconstruction.

```go
// pkg/op/provider/json/resource.go
type Resource struct {
    op.ResourceBase
    Data   []byte // raw JSON bytes
    Hash   string // SHA-256 of Data — metadata, NOT part of URI
    parsed any    // lazily decoded Go value — validates/encodes without roundtrip
}

func (r *Resource) Validate(schemaJSON string) (ValidationResult, error)
```

```go
// pkg/op/provider/yaml/resource.go
type Resource struct {
    op.ResourceBase
    Data   []byte // raw YAML bytes
    Hash   string // SHA-256 of Data — metadata, NOT part of URI
    parsed any    // lazily decoded Go value
}

func (r *Resource) Validate(schemaJSON string) (ValidationResult, error)
```

**Provider methods**:

| Method | Returns | Purpose |
| --- | --- | --- |
| `Parse(data string)` | `*Resource` | Decode string → Resource with cached Go value |
| `Validate(resource *Resource, schemaJSON string)` | `ValidationResult` | Validate resource against JSON Schema |
| `Decode(data string)` | `any` | Existing — returns Starlark value directly (unchanged) |
| `Encode(value any)` | `string` | Existing — accepts Starlark values or Resources (unchanged) |

`Parse` is the new entry point that returns a Resource. `Decode` remains for scripts that
just need a Starlark dict without validation.

**Starlark usage**:

```python
# Parse + validate (zero roundtrip):
doc = yaml.parse(file.read_text("config.yaml"))
result = doc.validate(schema_json)
if not result.valid:
    for err in result.errors:
        ui.error(err)
data = doc.value  # Starlark dict, converted lazily on demand

# Format conversion (Go→string, no Starlark hop):
json_text = json.encode(doc)  # yaml.Resource → JSON string via parsed field

# Simple decode (no resource, backwards compatible):
data = json.decode('{"key": "value"}')  # returns Starlark dict directly
```

### R3: Star analysis providers wired as immediate receivers

devlore-cli already has starindex, starcomplexity, starstats, and staranalysis providers.
These are already `+devlore:access=immediate`. Wire them into noblefactor-ops
`BindingConfig.WithReceivers(...)`. The `starlark_parse` hand-coded receiver is replaced
by these four providers, exposing separate namespaces:

| Old API | New API |
| --- | --- |
| `starlark_parse.parse(path)` | `starindex.index_files(...)` |
| `starlark_parse.complexity(path)` | `starcomplexity.compute_complexity(...)` |
| `starlark_parse.metrics(path)` | `starstats.compute_stats(...)` |
| (none) | `staranalysis.analyze(...)` |

### R4: noblefactor-ops-specific providers

Four receivers have no devlore-cli equivalent and are project-specific. They become new
providers under `internal/provider/`:

| Provider | Package | Methods |
| --- | --- | --- |
| shellcheck | `internal/provider/shellcheck/` | Lint, Format, Parse, Complexity |
| lint | `internal/provider/lint/` | Go, Shell, Markdown, EnsureTools |
| setup | `internal/provider/setup/` | Tools, PrecommitInstall, PrecommitCheck, InitConfig, InstallHook, UninstallHook, CheckHook |
| config | `internal/provider/nfconfig/` | Get, Show, Sync |

Each follows the goast pattern: `provider.go` + `types.go` + `gen/` directory with
generated receiver and params files.

Package name `nfconfig` avoids collision with Go's `config` and the existing
`internal/config` package.

### R5: commands provider — Runtime dependency

`commands` is unique: it needs access to the Runtime's command tree, which is not part of
`op.Context`. Two options:

**Option A — Inject via Context extension**: Add a `CommandTree` field to the context
passed during `Initialize()`. The commands provider reads it from `p.Context()`.

**Option B — Keep hand-coded**: commands is the only receiver with this dependency. If
the cost of framework migration exceeds the benefit, keep it hand-coded as the sole
exception. It has no compensation or graph-mode use case.

**Recommendation**: Option A. The CommandTree is a read-only index — injecting it via
context is clean and avoids a permanent exception.

### R6: Starlark API name migration

The framework generates snake_case Starlark names from CamelCase Go methods. Several
names change:

| Old (hand-coded) | New (framework) | Receiver |
| --- | --- | --- |
| `file.read(path)` | `file.read_text(path)` | file |
| `file.write(path, content)` | `file.write_text(path, content)` | file |
| `file.is_directory(path)` | `file.is_dir(path)` | file |
| `file.list(path)` | `file.glob(path + "/*")` | file |
| `file.walk_tree(root, fn)` | `file.walk_tree(root, fn)` | file |
| `file.remove(path)` | `file.remove(path)` | file |
| `file.remove_all(path)` | `file.remove_all(path)` | file |
| `schema.validate(data, schema)` | `json.parse(data).validate(schema)` or `yaml.parse(data).validate(schema)` | json/yaml |
| `shellcheck.lint(path)` | `shellcheck.lint(path)` | shellcheck |
| `lint.go(path?)` | `lint.go(path?)` | lint |
| `setup.precommit_install()` | `setup.precommit_install()` | setup |
| `setup.precommit_check()` | `setup.precommit_check()` | setup |
| `setup.init_config()` | `setup.init_config()` | setup |
| `setup.install_hook(name)` | `setup.install_hook(name)` | setup |
| `setup.uninstall_hook(name)` | `setup.uninstall_hook(name)` | setup |
| `setup.check_hook(name)` | `setup.check_hook(name)` | setup |
| `config.get()` | `nfconfig.get()` | nfconfig |
| `config.show()` | `nfconfig.show()` | nfconfig |
| `config.sync()` | `nfconfig.sync()` | nfconfig |
| `starlark_parse.parse(path)` | `starindex.index_files(...)` | starindex |
| `starlark_parse.complexity(path)` | `starcomplexity.compute_complexity(...)` | starcomplexity |
| `starlark_parse.metrics(path)` | `starstats.compute_stats(...)` | starstats |

All `.star` files and utility `.star` modules must be audited and updated.

### R7: DryRun integration

The hand-coded receivers use a global `starlark.DryRun` bool. Framework providers use
`op.Root` for confinement and dry-run gating. The `DryRun` global is removed; the
`op.Context` passed to `Initialize()` carries the dry-run flag via Root configuration.

## Implementation Phases

### Phase 1: devlore-cli — file.Provider immediate mode + json/yaml Resources

**Repo**: devlore-cli

**file.Provider**:

- [ ] Change `file.Provider` from `+devlore:access=planned` to `+devlore:access=both`
- [ ] Regenerate `gen/` files
- [ ] `make check` passes

**json.Resource**:

- [ ] Create `pkg/op/provider/json/resource.go` following mem.Resource pattern
- [ ] Define `Resource` struct: embed `op.ResourceBase`, fields `Data []byte`,
      `Hash string`, unexported `parsed any`
- [ ] Implement `ResourceFromValue` constructor (parses `json:<qualifier>` URIs)
- [ ] Implement `Validate(schemaJSON string) (ValidationResult, error)` on Resource
- [ ] Add `ValidationResult` type to `types.go` (Valid bool, Errors []string)
- [ ] Create `gen/resource.gen.go` — resource descriptor with `AnnounceResource`
- [ ] Add `Parse(data string) (*Resource, error)` method to `json.Provider`
- [ ] Ensure `Encode` accepts `*Resource` (reads `parsed` field, no Starlark hop)
- [ ] Add tests for Parse, Validate, Resource round-trip
- [ ] Regenerate provider `gen/` files

**yaml.Resource**:

- [ ] Create `pkg/op/provider/yaml/resource.go` following same pattern
- [ ] Define `Resource` struct: embed `op.ResourceBase`, fields `Data []byte`,
      `Hash string`, unexported `parsed any`
- [ ] Implement `ResourceFromValue` constructor (parses `yaml:<qualifier>` URIs)
- [ ] Implement `Validate(schemaJSON string) (ValidationResult, error)` on Resource
- [ ] Add `ValidationResult` type to `types.go` (Valid bool, Errors []string)
- [ ] Create `gen/resource.gen.go` — resource descriptor with `AnnounceResource`
- [ ] Add `Parse(data string) (*Resource, error)` method to `yaml.Provider`
- [ ] Ensure `Encode` accepts `*Resource` (reads `parsed` field, no Starlark hop)
- [ ] Add tests for Parse, Validate, Resource round-trip
- [ ] Regenerate provider `gen/` files

- [ ] `make check` passes (full suite)

**Files** (devlore-cli):

- `pkg/op/provider/file/provider.go` — Modify (access annotation)
- `pkg/op/provider/file/gen/*` — Regenerate
- `pkg/op/provider/json/resource.go` — Create
- `pkg/op/provider/json/types.go` — Modify (add ValidationResult)
- `pkg/op/provider/json/provider.go` — Modify (add Parse, update Encode)
- `pkg/op/provider/json/gen/resource.gen.go` — Create
- `pkg/op/provider/json/gen/receiver.gen.go` — Regenerate
- `pkg/op/provider/json/gen/params.gen.go` — Regenerate
- `pkg/op/provider/yaml/resource.go` — Create
- `pkg/op/provider/yaml/types.go` — Modify (add ValidationResult)
- `pkg/op/provider/yaml/provider.go` — Modify (add Parse, update Encode)
- `pkg/op/provider/yaml/gen/resource.gen.go` — Create
- `pkg/op/provider/yaml/gen/receiver.gen.go` — Regenerate
- `pkg/op/provider/yaml/gen/params.gen.go` — Regenerate

### Phase 2: noblefactor-ops — Wire reusable providers

**Repo**: noblefactor-ops

- [ ] Update `go.mod` to pull latest devlore-cli (with file immediate mode + json/yaml
      Resources)
- [ ] Import file, starindex, starcomplexity, starstats, staranalysis gen packages
- [ ] Add to `BindingConfig.WithReceivers(...)` in `NewRuntime()`
- [ ] Remove hand-coded receivers: `file`, `schema`, `starlark_parse` from
      `buildPredeclared()`
- [ ] Delete `receiver_file.go`, `receiver_schema.go`, `receiver_starlark.go`
- [ ] Remove file/schema/starlark_parse from `receivers.go` singletons
- [ ] Update `.star` files for API name changes (R6 table):
  - `file.read()` → `file.read_text()`
  - `file.write()` → `file.write_text()`
  - `file.is_directory()` → `file.is_dir()`
  - `file.list()` → `file.glob()`
  - `schema.validate(data, schema)` → `json.parse(data).validate(schema)` or
    `yaml.parse(data).validate(schema)` depending on format
  - `starlark_parse.*` → `starindex.*` / `starcomplexity.*` / `starstats.*`
- [ ] `make check` passes

**Files** (noblefactor-ops):

- `internal/starlark/runtime.go` — Modify (BindingConfig, buildPredeclared)
- `internal/starlark/receiver_file.go` — Delete
- `internal/starlark/receiver_schema.go` — Delete
- `internal/starlark/receiver_starlark.go` — Delete
- `internal/starlark/receivers.go` — Modify (remove singletons)
- `star/extensions/*.star` — Modify (API name migration per R6)

### Phase 3: noblefactor-ops — shellcheck provider

**Repo**: noblefactor-ops

- [ ] Create `internal/provider/shellcheck/` following goast pattern
- [ ] Move shellcheck logic from `receiver_shellcheck.go` into `provider.go`
- [ ] Extract result types to `types.go` with `starlark` tags
- [ ] Mark `+devlore:access=immediate`
- [ ] Run code generation → `gen/receiver.gen.go`, `gen/params.gen.go`
- [ ] Wire into `BindingConfig.WithReceivers(...)` in `NewRuntime()`
- [ ] Remove `shellcheck` from `buildPredeclared()` hand-coded section
- [ ] Delete `receiver_shellcheck.go`, remove from `receivers.go`
- [ ] Update `.star` files if any method names changed
- [ ] `make check` passes

**Files** (noblefactor-ops):

- `internal/provider/shellcheck/provider.go` — Create
- `internal/provider/shellcheck/types.go` — Create
- `internal/provider/shellcheck/gen/` — Generate
- `internal/starlark/receiver_shellcheck.go` — Delete
- `internal/starlark/receivers.go` — Modify
- `internal/starlark/runtime.go` — Modify

### Phase 4: noblefactor-ops — lint provider

**Repo**: noblefactor-ops

- [ ] Create `internal/provider/lint/` following goast pattern
- [ ] Move lint logic from `receiver_lint.go` into `provider.go`
- [ ] Extract result types to `types.go` with `starlark` tags
- [ ] Mark `+devlore:access=immediate`
- [ ] Run code generation
- [ ] Wire into BindingConfig, remove hand-coded receiver
- [ ] Delete `receiver_lint.go`
- [ ] Update `.star` files if needed
- [ ] `make check` passes

**Files** (noblefactor-ops):

- `internal/provider/lint/provider.go` — Create
- `internal/provider/lint/types.go` — Create
- `internal/provider/lint/gen/` — Generate
- `internal/starlark/receiver_lint.go` — Delete
- `internal/starlark/runtime.go` — Modify

### Phase 5: noblefactor-ops — setup provider

**Repo**: noblefactor-ops

- [ ] Create `internal/provider/setup/` following goast pattern
- [ ] Move setup logic from `receiver_setup.go` into `provider.go`
- [ ] Extract result types to `types.go` with `starlark` tags
- [ ] Mark `+devlore:access=immediate`
- [ ] Run code generation
- [ ] Wire into BindingConfig, remove hand-coded receiver
- [ ] Delete `receiver_setup.go`
- [ ] Update `.star` files if needed
- [ ] `make check` passes

**Files** (noblefactor-ops):

- `internal/provider/setup/provider.go` — Create
- `internal/provider/setup/types.go` — Create
- `internal/provider/setup/gen/` — Generate
- `internal/starlark/receiver_setup.go` — Delete
- `internal/starlark/runtime.go` — Modify

### Phase 6: noblefactor-ops — nfconfig provider

**Repo**: noblefactor-ops

- [ ] Create `internal/provider/nfconfig/` following goast pattern
- [ ] Move config logic from `receiver_config.go` into `provider.go`
- [ ] Extract result types to `types.go` with `starlark` tags
- [ ] Mark `+devlore:access=immediate`
- [ ] Run code generation
- [ ] Wire into BindingConfig, remove hand-coded receiver
- [ ] Delete `receiver_config.go`, remove from `receivers.go`
- [ ] Update `.star` files: `config.get()` → `nfconfig.get()`, etc.
- [ ] `make check` passes

**Files** (noblefactor-ops):

- `internal/provider/nfconfig/provider.go` — Create
- `internal/provider/nfconfig/types.go` — Create
- `internal/provider/nfconfig/gen/` — Generate
- `internal/starlark/receiver_config.go` — Delete
- `internal/starlark/receivers.go` — Modify
- `star/extensions/*.star` — Modify (config → nfconfig)
- `internal/starlark/runtime.go` — Modify

### Phase 7: noblefactor-ops — commands provider

**Repo**: noblefactor-ops

- [ ] Resolve design decision (R5): inject CommandTree via Context or keep hand-coded
- [ ] If Option A: define `CommandTree` interface, add to context, create provider
- [ ] Create `internal/provider/commands/` following goast pattern
- [ ] Move command-tree logic from `receiver_commands.go` into `provider.go`
- [ ] Extract result types to `types.go` with `starlark` tags
- [ ] Mark `+devlore:access=immediate`
- [ ] Run code generation
- [ ] Wire into BindingConfig, remove hand-coded receiver
- [ ] Delete `receiver_commands.go`
- [ ] `make check` passes

**Files** (noblefactor-ops):

- `internal/provider/commands/provider.go` — Create
- `internal/provider/commands/types.go` — Create
- `internal/provider/commands/gen/` — Generate
- `internal/starlark/receiver_commands.go` — Delete
- `internal/starlark/runtime.go` — Modify

### Phase 8: Cleanup

**Repo**: noblefactor-ops

- [ ] Delete `internal/starlark/receiver.go` (Receiver base type)
- [ ] Delete `internal/starlark/receivers.go` (singleton module)
- [ ] Remove `DryRun` global variable; wire dry-run via `op.Root` / `op.Context`
- [ ] Remove UIProvider field from Runtime (UI is now framework-managed)
- [ ] Verify no remaining references to deleted types
- [ ] Full end-to-end test: run every `star` command
- [ ] `make check` passes

**Files** (noblefactor-ops):

- `internal/starlark/receiver.go` — Delete
- `internal/starlark/receivers.go` — Delete
- `internal/starlark/runtime.go` — Modify (final cleanup)

## Migration Path

Each phase is a standalone PR (one per repo). Phase 1 (devlore-cli) must merge before
Phase 2 (noblefactor-ops) can begin. Phases 3-7 are independent and can be done in any
order. Phase 8 depends on all prior phases completing.

```
devlore-cli:  Phase 1 ──┐
                         │
noblefactor-ops:         └──► Phase 2 ──► Phases 3-7 (parallel) ──► Phase 8
```

## Files to Create/Modify

### devlore-cli

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/provider/file/provider.go` | Modify | access=both |
| `pkg/op/provider/file/gen/*` | Regenerate | Updated receiver/params |
| `pkg/op/provider/json/resource.go` | Create | json.Resource type with Validate |
| `pkg/op/provider/json/types.go` | Modify | ValidationResult type |
| `pkg/op/provider/json/provider.go` | Modify | Add Parse, update Encode |
| `pkg/op/provider/json/gen/*` | Regenerate | Resource descriptor + receiver/params |
| `pkg/op/provider/yaml/resource.go` | Create | yaml.Resource type with Validate |
| `pkg/op/provider/yaml/types.go` | Modify | ValidationResult type |
| `pkg/op/provider/yaml/provider.go` | Modify | Add Parse, update Encode |
| `pkg/op/provider/yaml/gen/*` | Regenerate | Resource descriptor + receiver/params |

### noblefactor-ops

| File | Action | Purpose |
| --- | --- | --- |
| `internal/starlark/runtime.go` | Modify | Wire framework providers |
| `internal/starlark/receiver.go` | Delete | Base type no longer needed |
| `internal/starlark/receivers.go` | Delete | Singletons no longer needed |
| `internal/starlark/receiver_file.go` | Delete | Replaced by file.Provider |
| `internal/starlark/receiver_schema.go` | Delete | Replaced by json/yaml.Validate |
| `internal/starlark/receiver_starlark.go` | Delete | Replaced by star* providers |
| `internal/starlark/receiver_shellcheck.go` | Delete | Replaced by shellcheck provider |
| `internal/starlark/receiver_lint.go` | Delete | Replaced by lint provider |
| `internal/starlark/receiver_setup.go` | Delete | Replaced by setup provider |
| `internal/starlark/receiver_config.go` | Delete | Replaced by nfconfig provider |
| `internal/starlark/receiver_commands.go` | Delete | Replaced by commands provider |
| `internal/provider/shellcheck/` | Create | Shell analysis provider |
| `internal/provider/lint/` | Create | Lint orchestration provider |
| `internal/provider/setup/` | Create | Repo setup provider |
| `internal/provider/nfconfig/` | Create | Config management provider |
| `internal/provider/commands/` | Create | Command tree provider |
| `star/extensions/*.star` | Modify | API name migration |

## Related Documents

- [star-gen-receiver.md](./star-gen-receiver.md) — Generated receiver framework design
- [shared-provider-receivers.md](./shared-provider-receivers.md) — Cross-repo provider sharing
- [typed-access-receiver-factory.md](./typed-access-receiver-factory.md) — Typed access factory pattern
- noblefactor-ops `docs/plans/star-consumes-pkg-op.md` — Prior plan covering UI migration,
  code generator function parameter bridge, file provider wiring (Phases 2-6)

## Open Questions

- [ ] **R5 — commands provider injection**: Option A (CommandTree in Context) vs Option B
  (keep hand-coded). Current recommendation: Option A.
- [ ] **nfconfig naming**: The `config` namespace conflicts with `internal/config`. Using
  `nfconfig` changes the Starlark API from `config.get()` to `nfconfig.get()`. Is a
  wrapper acceptable to preserve the old name, or is the rename preferred?
- [ ] **DryRun scope**: Should dry-run be a Root property, a Context field, or a
  provider-level flag? Current recommendation: Root property, since file.Provider already
  gates writes through Root.
- [ ] **Compensation for noblefactor-ops providers**: Should shellcheck/lint/setup/config
  providers implement compensation (undo)? Current recommendation: No — these are
  read-mostly operations. Only setup.InstallHook/UninstallHook would benefit, and the
  cost/complexity doesn't justify it for a CLI tool.
- [ ] **ValidationResult sharing**: json and yaml providers both define `ValidationResult`.
  Should this be a shared type in `pkg/op` or duplicated per provider? Current
  recommendation: shared type in `pkg/op` — it's format-agnostic (JSON Schema validation
  produces the same result shape regardless of input format).
