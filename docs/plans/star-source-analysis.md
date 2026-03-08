---
title: "Starlark Source Analysis Provider"
status: in-progress
created: 2026-02-24
updated: 2026-02-24
---

# Plan: Starlark Source Analysis Provider

## Summary

Implement a `starlarkcode` provider in `pkg/op/provider/starlarkcode/` that provides Starlark source analysis: file capture with gitignore awareness, AST-based indexing (functions, loads, globals), line statistics, cyclomatic/cognitive complexity scoring, and hotspot detection. The provider exposes both a Go API and a Starlark binding for use in `.star` scripts.

## Goals

1. **Source capture**: Glob-based file collection with gitignore filtering and `.bzl` inclusion
2. **AST indexing**: Extract functions, loads, globals with docstrings and line metadata
3. **Complexity analysis**: Cyclomatic, cognitive, and nesting depth per function with hotspot detection
4. **Starlark binding**: Object-oriented API — `starlarkcode.capture()` returns a handle with `.index()`, `.stats()`, `.analyze()` methods

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Starlark parser (`go.starlark.net/syntax`) | Available | Already in go.mod |
| Gitignore tracker (`file/ignore`) | Available | `NewTracker`, `WalkTree` |
| Binding infrastructure (`op.RegisterBinding`) | Available | Used by all existing providers |
| Source analysis provider | Missing | This plan |

## Requirements

### Package Structure

```
pkg/op/provider/starlarkcode/
    provider.go        Provider struct, Capture()
    sources.go         Sources handle, Paths(), Count()
    index.go           Index types and indexFile()
    stats.go           Stats types and countLines()
    complexity.go      complexityWalker, cyclomatic/cognitive/nesting
    analyze.go         Analyze(), AnalysisReport, Hotspot, AnalysisConfig
    sources_value.go   SourcesValue (starlark.HasAttrs wrapper for Sources)
    receiver.go        StarlarkCodeReceiver (top-level binding)
    *_test.go          Tests for each component
    testdata/          Fixture files
```

No `planned_gen.go` or `actions_gen.go` — immediate-only provider.

### Starlark API

```python
sources = starlarkcode.capture("**/*.star", gitignore=True, include_bzl=True)
sources.paths    # list[string]
sources.count    # int
index  = sources.index(with_docstrings=True, with_globals=True)
stats  = sources.stats(bytes=True, loc=True)
report = sources.analyze(hotspots=True, cyclomatic_threshold=10,
                         cognitive_threshold=15, with_index=False)
```

### Go API

```go
p := &starlarkcode.Provider{ProviderBase: op.NewProviderBase(op.Context{BaseDir: "."})}
sources, _ := p.Capture("**/*.star", true, true)
idx, _     := sources.Index(true, true)
stats, _   := sources.Stats(true, true)
report, _  := sources.Analyze(starlarkcode.AnalysisConfig{...})
```

### BindingConfig Change

Add `WorkDir string` field to `op.BindingConfig` so providers can access the working directory.

## Implementation Phases

### Phase 1: Foundation — Provider, Sources, Stats

- [ ] Add `WorkDir` to `op.BindingConfig`
- [ ] `provider.go`: `Provider` struct, `Capture()` with glob + `**` + gitignore filtering
- [ ] `sources.go`: `Sources` type, `Paths()`, `Count()`
- [ ] `stats.go`: `countLines()`, `Stats()` method, all Stats types
- [ ] `testdata/simple.star`, `testdata/complex.star`, `testdata/empty.star`, `testdata/with_globals.star`
- [ ] `provider_test.go`, `stats_test.go`

### Phase 2: Index — AST Parsing

- [ ] `index.go`: `indexFile()` using `syntax.Parse()`, extract functions/loads/globals
- [ ] `Index()` method on Sources with totals aggregation
- [ ] `index_test.go`

### Phase 3: Complexity

- [ ] `complexity.go`: `complexityWalker`, cyclomatic/cognitive/nesting scoring
- [ ] `complexity_test.go` with hand-calculated verification

### Phase 4: Analyze + Hotspots

- [ ] `analyze.go`: `Analyze()` with single-pass parse, hotspot detection
- [ ] `analyze_test.go`

### Phase 5: Starlark Binding

- [ ] `sources_value.go`: `SourcesValue` wrapping `*Sources`
- [ ] `receiver.go`: `StarlarkCodeReceiver`, `init()` + `op.RegisterBinding`
- [ ] Update `pkg/op/provider/register.go` blank import
- [ ] End-to-end verification

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/binding_config.go` | Modify | Add `WorkDir` field |
| `pkg/op/provider/starlarkcode/provider.go` | Create | Provider struct and Capture |
| `pkg/op/provider/starlarkcode/sources.go` | Create | Sources handle |
| `pkg/op/provider/starlarkcode/index.go` | Create | Index types and indexFile() |
| `pkg/op/provider/starlarkcode/stats.go` | Create | Stats types and countLines() |
| `pkg/op/provider/starlarkcode/complexity.go` | Create | Complexity walker |
| `pkg/op/provider/starlarkcode/analyze.go` | Create | AnalysisReport orchestration |
| `pkg/op/provider/starlarkcode/sources_value.go` | Create | Starlark HasAttrs wrapper |
| `pkg/op/provider/starlarkcode/receiver.go` | Create | Top-level binding + init() |
| `pkg/op/provider/starlarkcode/*_test.go` | Create | Tests for each component |
| `pkg/op/provider/starlarkcode/testdata/*.star` | Create | Fixture files |
| `pkg/op/provider/register.go` | Modify | Add blank import |

## Verification

1. `make test` passes
2. `make build` succeeds
3. `make vet` clean
4. Grep for `legacy|backward|compat|deprecated` — zero matches in new files
5. No stub functions returning success without implementation
6. All new types have test coverage
7. Complexity walker scores match hand-calculated values
8. Gitignore filtering excludes expected files
