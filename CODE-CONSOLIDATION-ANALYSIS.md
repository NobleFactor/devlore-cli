# Code Consolidation Analysis

**Date**: 2026-03-10
**Scope**: devlore-cli + noblefactor-ops (develop branches)
**Previous analysis**: 2026-02-27

---

## Codebase Size

| Repo            | Language | Production |     Test |      Total |
|-----------------|----------|-----------:|---------:|-----------:|
| devlore-cli     | Go       |     38,756 |   39,536 |     78,292 |
| devlore-cli     | Starlark |      4,954 |        — |      4,954 |
| noblefactor-ops | Go       |     12,670 |   11,116 |     23,786 |
| noblefactor-ops | Starlark |      1,104 |        — |      1,104 |
| **Total**       |          | **57,484** | **50,652** | **108,136** |

Change from Feb 27: +1,652 production lines (+3%), +10,581 test lines (+26%).
Test-to-production ratio improved from 0.72:1 to 0.88:1.

### devlore-cli by package

| Package      | Production |
|--------------|----------:|
| `pkg/`       |    12,844 |
| `internal/`  |    23,862 |
| `cmd/`       |       387 |
| other        |     1,663 |
| **Total**    | **38,756** |

---

## Dead Code

No actionable dead code found. All exported types, functions, and constants are actively
referenced. The greenfield policy is working — there is no legacy cruft.

---

## Resolved Since February Analysis

These items from the Feb 27 analysis are **closed**:

| Item | Was | Resolution |
|------|-----|-----------|
| 2.1 Generated action wrappers (~460 lines) | Per-action struct generation | **Eliminated** by `RegisterReflectedActions`. Zero `*_gen.go` files contain action structs. |
| 2.2 Generated immediate dispatch (~245 lines) | Switch-based `Attr()` in `immediate_gen.go` | **Eliminated** by `WrapReceiver()` + map-based `methodBridge` lookup. |
| 2.3 Generated planned node creation (~215 lines) | Inline slot filling per method | **Eliminated** by `WrapPlanned()` + `buildPlannedBridge` at runtime. |
| 1.2 codegen.go monolith (~400 lines) | 2,528-line monolith | **File no longer exists.** Decomposed into modular receiver files. |
| 1.6 codegen_test.go expansion (~200 lines) | 3,411-line test file with inline fixtures | **File no longer exists.** Decomposed into receiver-specific test files with proper helpers. |
| 1.2 Parameter filtering pattern | 8+ repeated `for _, p := range m.Params` loops | **No matches found.** Pattern eliminated. |

**Total resolved: ~1,520 lines of the original ~2,472 estimate (61%).**

Generated code is now minimal factory functions. Each provider generates 4–5 files totaling
51–91 lines (params map, provider descriptor, `WrapPlanned()` call, `WrapReceiver()` call,
optional resource registration). All dispatch logic lives in three shared files:
`action_reflect.go`, `receiver_reflect.go`, `planned_reflect.go`.

---

## Part 1: noblefactor-ops — Open Items

### 1.1 Receiver Attr()/AttrNames() Boilerplate (16 receivers)

All 16 receiver files repeat:
- `Attr(name string)` switch → `op.MakeAttr()` per method
- `AttrNames() []string` hardcoded sorted slice

**Example** (`receiver_ui.go`):
```go
func (r *UiReceiver) Attr(name string) (starlark.Value, error) {
    switch name {
    case "note": return op.MakeAttr("ui.note", r.note), nil
    case "warn": return op.MakeAttr("ui.warn", r.warn), nil
    // ...
    default: return nil, op.NoSuchAttrError("ui", name)
    }
}
func (r *UiReceiver) AttrNames() []string {
    return []string{"error", "fail", "note", "success", "warn"}
}
```

devlore-cli solved this with `WrapReceiver()` + map-based dispatch. noblefactor-ops still
uses hand-coded switches.

**Consolidation**: Same pattern — map-based factory from a method table.

**Estimated savings: ~200 lines across 16 files**

### 1.2 starlarkToGo / goToStarlark Location

Defined in `receiver_yaml.go` (86 lines), used by 5 files (receiver_yaml, receiver_json,
receiver_schema, render, wasm_receiver).

**Consolidation**: Extract to `starlark_values.go`.

**Not a line savings — a coupling fix.**

### 1.3 Hook Command Duplication

`hook-pre-commit.star` (116 lines) and `hook-pre-push.star` (116 lines) are identical except
for the command name string. Both contain:
- `run()`, `run_linter()`, `run_go_check()`, `run_shell_check()`, `run_markdown_check()`

**Consolidation**: Extract to shared `hook_checks.star`, parameterized by hook name.

**Estimated savings: ~90 lines**

### 1.4 Lint Helper Duplication

`check_tool()` and `ensure_tool_installed()` copied in `lint-go.star`, `lint-shell.star`,
and both hook files.

**Consolidation**: Shared `lint_utils.star` module.

**Estimated savings: ~50 lines**

### noblefactor-ops Subtotal

| Area                          | Savings |
|-------------------------------|--------:|
| Receiver boilerplate          |     200 |
| Hook command dedup            |      90 |
| Lint helper dedup             |      50 |
| starlarkToGo relocation      |       — |
| **Subtotal**                  | **~340** |

---

## Part 2: devlore-cli — Open Items

### 2.1 Root Command Initialization (NEW)

`internal/lore/root.go` (154 lines) and `internal/writ/root.go` (145 lines) are nearly
identical. Differences: command name, env prefix, subcommand list, help text.

Both implement:
- `NewRootCmd()` with identical cobra setup boilerplate
- `initConfig()` with identical viper configuration
- Identical persistent flag binding

**Consolidation**: Extract `internal/cli/root.go` with a parameterized factory:
```go
func NewRootCmd(name, envPrefix string, subcommands ...func() *cobra.Command) *cobra.Command
```

**Estimated savings: ~130 lines**

### 2.2 countLines() Duplication (NEW)

Identical 26-line function in both `starstats/provider.go` and `starindex/provider.go`.
Counts LOC, SLOC, comments, blanks.

**Consolidation**: Extract to shared `pkg/op/provider/starutil/lines.go`.

**Estimated savings: ~26 lines**

### 2.3 File Read + Relative Path Pattern (NEW)

9-line pattern repeated verbatim in `starindex`, `starcomplexity`, `starstats`:
```go
data, err := os.ReadFile(absPath)
if err != nil { return nil, err }
relPath, err := filepath.Rel(p.Root, absPath)
if err != nil { relPath = absPath }
```

**Consolidation**: Shared helper alongside 2.2.

**Estimated savings: ~18 lines**

### 2.4 StateView / DependencyView Parallel Patterns

`stateview.go` (556 lines) and `dependencyview.go` (445 lines) share builder patterns,
graph traversal helpers, index construction, and sorting logic.

**Consolidation**: Shared `View` base if more views are added. Low priority — distinct
concerns that happen to share structure.

**Estimated savings: ~80 lines**

### 2.5 Star Extension Commands

`extract.star`, `index.star`, `sign.star`, `validate.star` across Knowledge and Package
extensions share argument parsing, validation flows, and output formatting.

**Consolidation**: Shared Starlark utility module for extension commands.

**Estimated savings: ~100 lines**

### 2.6 BindingSet Factory Map

`collectPlannedProviders()` pattern repeated in `BuildGlobals()` and `buildPlanModule()`.

**Consolidation**: Extract helper.

**Estimated savings: ~8 lines**

### devlore-cli Subtotal

| Area                          | Savings |
|-------------------------------|--------:|
| Root command initialization   |     130 |
| StateView/DependencyView      |      80 |
| Star extension commands       |     100 |
| countLines() dedup            |      26 |
| File read + relpath pattern   |      18 |
| BindingSet factory map        |       8 |
| **Subtotal**                  | **~362** |

---

## Combined Summary

| Category                      | noblefactor-ops | devlore-cli |   Total |
|-------------------------------|----------------:|------------:|--------:|
| Receiver/dispatch boilerplate |             200 |           — |     200 |
| Consumer duplication          |               — |         130 |     130 |
| Starlark command dedup        |             140 |         100 |     240 |
| View/state consolidation      |               — |          80 |      80 |
| Utility dedup                 |               — |          52 |      52 |
| Misc                          |               — |           8 |       8 |
| **Total**                     |         **340** |     **362** | **~702** |

**Remaining reduction**: ~702 lines from 57,484 production = **1.2%**

The February analysis identified ~2,472 lines. Of that, ~1,520 lines (61%) were resolved by
the reflection-based dispatch and codegen decomposition work. The remaining ~702 lines are
smaller, lower-ROI items — no single item exceeds 200 lines.

---

## Priority Ranking

| Priority | Target                              | Effort | Savings | ROI |
|----------|-------------------------------------|--------|--------:|-----|
| 1        | Root command factory                 | Low    |     130 | High — eliminates drift between lore/writ CLI setup |
| 2        | Receiver factory (noblefactor-ops)  | Low    |     200 | High — 16 files simplified, mirrors devlore-cli pattern |
| 3        | Hook/lint Starlark dedup            | Low    |     140 | Medium — quick win, 4 files → 2 |
| 4        | Star extension command utils        | Low    |     100 | Medium — reduces Starlark duplication |
| 5        | Star provider shared utils          | Low    |      52 | Medium — countLines + readFile helpers |
| 6        | View/state consolidation            | Medium |      80 | Low — distinct concerns, defer until more views exist |

---

## Comparison: Feb 27 → Mar 10

| Metric                     | Feb 27 | Mar 10 | Change |
|----------------------------|-------:|-------:|--------|
| Production lines           | 55,832 | 57,484 | +1,652 (+3%) |
| Test lines                 | 40,071 | 50,652 | +10,581 (+26%) |
| Identified consolidation   |  2,472 |    702 | -1,770 (72% reduction in tech debt) |
| Generated code boilerplate |   ~920 |      0 | Eliminated |
| codegen.go monolith        |  2,528 |      0 | Eliminated |
