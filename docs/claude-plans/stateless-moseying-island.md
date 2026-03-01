# Plan: Add `star lint` Commands

## Overview

Add `star lint shell` and `star lint go` commands to noblefactor-ops using a hybrid approach:
- **Go layer**: `lint` module with `lint.go()` builtin for golangci-lint
- **Starlark layer**: `ops/lint.star` with CLI commands that orchestrate linting

These commands will replace the current CI workflow's golangci-lint-action and shell-lint.sh script.

## Commands

```
star lint shell [--path=.]     # shellcheck + shfmt
star lint go [--path=./...]    # golangci-lint
```

## Files to Create

### 1. `internal/starlark/builtin_lint.go`

New Go module with:
- `lintModule()` factory function
- `lintGo()` builtin that:
  - Runs `golangci-lint run --out-format=json`
  - Parses JSON output into structured issues
  - Returns `{issues, error_count, warning_count, passed, total_count}`

### 2. `ops/lint.star`

Starlark commands:
- `lint.shell` command:
  - Calls existing `shell.lint(path, severity)` for shellcheck
  - Calls existing `shell.format_check(path, indent)` for shfmt
  - Reports results, fails if either tool fails
- `lint.go` command:
  - Calls new `lint.go(path, config?)` builtin
  - Reports issues, fails if errors/warnings found

## Files to Modify

### 3. `internal/starlark/runtime.go` (line ~84)

Add lint module registration:
```go
"lint": lintModule(),
```

### 4. `.github/workflows/ci.yaml`

Replace:
```yaml
- name: Lint Go
  uses: golangci/golangci-lint-action@v6

- name: Shell lint
  run: .github/scripts/shell-lint.sh
```

With:
```yaml
- name: Install tools
  run: |
    go install mvdan.cc/sh/v3/cmd/shfmt@latest
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

- name: Lint Go
  run: go run ./cmd/star lint go

- name: Lint Shell
  run: go run ./cmd/star lint shell
```

## Files to Delete

### 5. `.github/scripts/shell-lint.sh`

No longer needed after migration.

## Implementation Sequence

1. Create `internal/starlark/builtin_lint.go`
2. Register module in `runtime.go`
3. Create `ops/lint.star`
4. Test locally: `go run ./cmd/star lint shell` and `go run ./cmd/star lint go`
5. Update CI workflow
6. Delete `shell-lint.sh`

## Key Patterns

**Starlark builtin signature:**
```go
func lintGo(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)
```

**Return structured results:**
```go
return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
    "passed": starlark.Bool(passed),
    "issues": starlark.NewList(issues),
})
```

**Starlark command fail on error:**
```python
if not result.passed:
    fail("Lint checks failed")
```

## Notes

- Existing `shell.lint()` and `shell.format_check()` are reused (no changes needed)
- `lint.go()` uses `.golangci.yaml` by default (same as current CI)
- Exit codes work correctly: `fail()` propagates error → `os.Exit(1)`
