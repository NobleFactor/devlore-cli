# Phase 8: Generated Bridge Tests for Provider Actions

## Context

Every provider method signature change (Phases 3, 3b) requires hand-updating `gen/actions_test.go` — slot names, result type assertions, dry-run tests, undo-nil tests. These are mechanical tests that verify the reflection bridge, not provider behavior. They break predictably and are tedious to fix.

The star generator already produces `params.gen.go`, `immediate.gen.go`, `planned.gen.go`, and `actions.gen.go` from method signatures. This phase extends it to produce `actions_test.gen.go` — bridge verification tests that are regenerated on every `make build`.

Currently only `pkg/op/provider/file/gen/` has a hand-written `actions_test.go` (~950 lines). The pattern applies to all providers.

**Repo**: devlore-cli + noblefactor-ops (star generator)
**Branch**: TBD

## Goals

1. **Eliminate signature-change churn** — bridge tests regenerate automatically when method signatures change
2. **Ensure coverage across all providers** — every provider with generated actions gets bridge tests for free
3. **Separate bridge tests from behavior tests** — generated tests verify the bridge; hand-written tests verify provider semantics

## What Gets Generated

### Fully derivable from method signatures

| Test pattern | Derived from | Current example |
|---|---|---|
| **Registration** | Method list | `TestActionNames`, `TestRegister` |
| **Slot name mapping** | `params.gen.go` entries | `slots := map[string]any{"resource": path, ...}` |
| **Result type assertion** | Return type | `result.(provider.Resource)` vs `result.(string)` |
| **Dry-run** | Action name | Assert `result == nil`, output contains `[dry-run] file.action_name` |
| **Undo nil** | Compensable pair exists | `action.Undo(ctx, nil)` → no error |
| **Compensable interface** | `Compensate*` method exists | Cast to `op.CompensableAction` succeeds |

### Derivable with conventions

**Fixture setup** follows from parameter names and types:

| Parameter pattern | Fixture action |
|---|---|
| `Resource` named `source*` | Create temp file with test content |
| `Resource` named `destination*`, `path` | Temp path, no pre-existing file |
| `string` named `content` | Literal `"test-content"` |
| `os.FileMode` | `0o644` |
| `bool` named `prune` | `false` |
| `Resource` named `prune_boundary` | `Resource{}` (empty) |
| `string` named `backup_suffix` | `".test-backup"` |
| `bool` named `honor_gitignore` | `false` |

The generator already knows parameter names (params.gen.go), types (method reflection), and compensable pairs (Compensate* naming convention).

## What Stays Hand-Written

Behavior tests that verify provider semantics beyond the bridge:

- Content verification after Write/Copy (`os.ReadFile` + assert content)
- `os.Readlink` verification for Link
- Source-gone assertion for Move
- Checksum verification in compensation
- Error path tests (checksum mismatch, non-symlink Unlink, non-empty dir Remove)
- Round-trip tests (Do → Undo → verify original state)

These belong in `actions_test.go` (hand-written), not in the generated file. The file provider already has comprehensive behavior tests in `provider_test.go`.

## File Layout (per provider)

```
pkg/op/provider/<name>/gen/
  actions.gen.go          # existing — action registration
  actions_test.gen.go     # NEW — generated bridge tests
  actions_test.go         # hand-written behavior tests (shrinks significantly)
  params.gen.go           # existing — slot name mapping
  immediate.gen.go        # existing — immediate receiver
  planned.gen.go          # existing — planned receiver
```

## Changes

### 1. Star generator — new template (`noblefactor-ops`)

Add an `actions_test` template to the star generator that produces `actions_test.gen.go` for each provider. The template receives:

- Method list with parameter names, types, return types
- Compensable pairs (method → Compensate* mapping)
- Provider import path and type name
- Params map (from params.gen.go generation)

The template emits:

- Package declaration and imports
- `TestActionNames` — all action names are registered
- Per-method `Test<Method>Action_Do` — fixture setup, slot construction, Do call, result type assertion
- Per-compensable `Test<Method>Action_Undo_Nil` — Undo with nil state
- Per-method `Test<Method>Action_DryRun` — dry-run output assertion
- Test helpers: `newCtx`, `dryRunCtx`, `makeRegistry`, `getAction`, `getCompensable`

### 2. Fixture conventions

The generator uses parameter name conventions to construct test fixtures:

```go
// For a method: Move(source Resource, destination Resource) (Resource, map[string]any, error)
// Generator produces:
func TestMoveAction_Do(t *testing.T) {
    tmp := t.TempDir()
    source := filepath.Join(tmp, "test-source.txt")
    if err := os.WriteFile(source, []byte("test-content"), 0o644); err != nil {
        t.Fatal(err)
    }
    dest := filepath.Join(tmp, "test-destination.txt")
    // ...
    slots := map[string]any{
        "source":      source,
        "destination": dest,
    }
    // ...
}
```

Fixture rules are encoded as a small mapping in the generator, not as annotations on the provider methods.

### 3. Migrate file provider (`devlore-cli`)

- Run `make build` to generate `actions_test.gen.go`
- Remove bridge-only tests from `actions_test.go`
- Keep behavior tests that verify content, symlinks, checksums, error paths
- Rename remaining file to clarify its purpose (or keep as `actions_test.go` with a comment header)

### 4. Apply to other providers

Once the template is proven on the file provider, enable generation for all providers that have `actions.gen.go`. Each provider gets bridge tests automatically without writing any test code.

## Execution Order

1. Implement the `actions_test` template in the star generator (noblefactor-ops)
2. Test with the file provider — verify generated output matches current bridge tests
3. Migrate file provider: generated bridge tests + trimmed hand-written behavior tests
4. Enable for remaining providers
5. `make check` across all providers

## Verification

```bash
make build    # regenerates all gen files including new actions_test.gen.go
make vet      # no vet issues
make test     # all tests pass
make check    # full quality gate
```

## Design Decisions

### Why not annotations on provider methods?

The fixture conventions (source → create file, destination → path only) are derivable from parameter names and types. Adding `+devlore:test_fixture` annotations would be redundant with information the generator already has. If a method's fixture needs don't fit the convention, it gets a hand-written test instead.

### Why a separate generated test file?

Go allows multiple test files in the same package. `actions_test.gen.go` contains bridge tests (mechanical, regenerated). `actions_test.go` contains behavior tests (semantic, hand-written). The `// Code generated` header prevents editors and linters from suggesting modifications to generated code.

### Why not generate behavior tests?

Behavior tests encode domain expectations (file content matches, symlink targets are correct, checksums verify). These are the tests that catch real bugs. Generating them would either produce trivial tests (just call Do and check no error) or require encoding domain semantics in the generator — which is the provider author's job, not the generator's.
