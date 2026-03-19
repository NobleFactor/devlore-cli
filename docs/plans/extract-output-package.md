---
title: "Extract output package from internal/cli"
issue: pending
status: complete
created: 2026-03-18
updated: 2026-03-19
---

# Plan: Extract output package from internal/cli

## Summary

Extract the structured-data rendering pipeline from `internal/cli/output.go` into a new `internal/output` package with a single public function: `Render(w io.Writer, data any, options Options)`. Remove dead convenience wrappers (`RenderTo`, `RenderMutationTo`). Keep cobra flag bindings in `internal/cli`.

## Context

During dead code analysis (issue #238), we identified that `RenderTo` and `RenderMutationTo` are unused convenience wrappers that hardcode `os.Stdout`. Reviewing the rendering pipeline with the user revealed:

- The dotted-path viper wrappers (10 functions) add no value — remove them
- `RenderTo` and `RenderMutationTo` remove caller choice of `io.Writer` for no benefit — remove them
- The render pipeline is a distinct concern from CLI flag binding and status output — extract it
- The type `OutputFlags` should be renamed `Options` in the new package (it's not tied to flags)
- Parameter order: `Render(w io.Writer, data any, options Options)` — writer first per Go convention

## Goals

1. **Single render function**: `output.Render(writer, data, options)` — one way to render structured data
2. **Clean separation**: rendering logic decoupled from cobra, exit codes, and status output
3. **Dead code removal**: eliminate `RenderTo`, `RenderMutationTo`, and the 10 viper wrappers

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `Render(w, data, flags)` | Exists in `internal/cli/output.go` | Zero production callers, tested |
| `RenderTo(data, flags)` | Dead | Hardcodes stdout, zero callers |
| `RenderMutation(w, data, flags)` | Exists | Zero production callers, tested |
| `RenderMutationTo(data, flags)` | Dead | Hardcodes stdout, zero callers |
| `OutputFlags` | Used | Flag setup in lore and writ commands |
| `MutationFlags` | Unused | Only referenced in tests |
| `AddOutputFlags` | Used | 2 call sites (lore inspect, writ snapshot) |
| `AddMutationFlags` | Unused | Zero call sites |
| Viper wrappers (10 functions) | Dead | Zero callers, code uses viper directly |

## Implementation Phases

### Phase 1: Create `internal/output` package

- [ ] Create `internal/output/render.go` with:
  - `Options` struct (`Format string`, `Filter []string`)
  - `DefaultFormat` constant (`"json"`)
  - `func Render(w io.Writer, data any, options Options) error`
  - All supporting functions: `renderJSON`, `renderYAML`, `renderTable`, `renderTemplate`, `applyFilter`, reflection helpers
- [ ] Create `internal/output/render_test.go` — migrate relevant tests from `internal/cli/output_test.go`

**Files**:
- `internal/output/render.go` — Create
- `internal/output/render_test.go` — Create

### Phase 2: Update callers and remove dead code from `internal/cli`

- [ ] Update `AddOutputFlags` in `internal/cli/output.go` to use `output.Options` instead of `OutputFlags`
- [ ] Update `internal/lore/commands.go` — `newInspectCmd()` flag setup (line ~758)
- [ ] Update `internal/writ/commands.go` — `newSnapshotCmd()` flag setup (line ~1488)
- [ ] Remove from `internal/cli/output.go`:
  - `OutputFlags` struct (replaced by `output.Options`)
  - `DefaultFormat` constant (moved to `output`)
  - `Render` function (moved to `output`)
  - `RenderTo` function (dead)
  - `MutationFlags` struct (unused)
  - `RenderMutation` function (unused — passthru gate becomes caller's `if`)
  - `RenderMutationTo` function (dead)
  - `AddMutationFlags` function (unused)
  - All private rendering helpers (`renderJSON`, `renderYAML`, `renderTable`, `renderTemplate`, `applyFilter`, `matchesAllFilters`, `matchesFilter`, `toSlice`, `getFieldNames`, `getFieldValue`, `formatFieldValue`)
- [ ] Remove `internal/cli/output_test.go` tests for removed functions

### Phase 3: Remove viper wrappers

- [ ] Remove from `internal/cli/viper.go`:
  - `BindFlagsWithPrefix`
  - `Get`, `GetBool`, `GetInt`, `GetStringSlice`, `GetStringMap`
  - `ToolConfigPath`, `ConfigFileUsed`, `AllSettings`, `Debug`
- [ ] Verify `InitViper`, `BindFlags`, `SharedConfigPath` remain (they have callers)

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `internal/output/render.go` | Create | Render pipeline: `Render(w, data, options)` |
| `internal/output/render_test.go` | Create | Tests for render pipeline |
| `internal/cli/output.go` | Modify | Remove render pipeline, keep exit codes + status output + `AddOutputFlags` |
| `internal/cli/output_test.go` | Modify | Remove migrated tests, keep exit code + status output tests |
| `internal/cli/viper.go` | Modify | Remove 10 dead wrapper functions |
| `internal/lore/commands.go` | Modify | Update `OutputFlags` → `output.Options` |
| `internal/writ/commands.go` | Modify | Update `OutputFlags` → `output.Options` |

## Verification

1. `make check` passes (vet, lint, test)
2. `grep -r 'RenderTo\|RenderMutationTo\|OutputFlags' internal/` returns zero hits
3. `grep -r 'BindFlagsWithPrefix\|cli\.Get\b\|cli\.GetBool\|cli\.GetInt\|cli\.GetStringSlice\|cli\.GetStringMap\|cli\.ToolConfigPath\|cli\.ConfigFileUsed\|cli\.AllSettings\|cli\.Debug' internal/` returns zero hits
4. New package compiles independently: `go build ./internal/output/...`

## Related Documents

- Issue #238 — Dead code removal (GoLand inspection Phase 5)
