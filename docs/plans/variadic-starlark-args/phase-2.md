---
title: "Phase 2: Codegen + Tests"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../variadic-starlark-args.md
---

# Phase 2: Codegen + Tests

## Summary

Update `star`'s `compute_param_names_list` to emit the `*` prefix for
variadic Go parameters. Regenerate affected providers. Add e2e tests
verifying both positional and keyword calling conventions.

## Deliverables

### 1. Template change

In `compute_param_names_list`, check the `variadic` flag on each parameter
object. If `p.variadic` is true, emit `*name` instead of `name`:

```python
# star/extensions/com.noblefactor.devlore.Actions/commands/generate.star
# ~line 1115, compute_param_names_list

def compute_param_names_list(method):
    names = []
    for p in method.params:
        if p.callable:
            continue
        name = p.name
        if p.variadic:
            name = "*" + name
        elif p.optional:
            name = name + "?"
        names.append(name)
    return names
```

### 2. Regenerate affected providers

Run `make build`. Only `file` currently has a variadic method (`Join`),
so only `file/gen/params.gen.go` changes:

```go
// pkg/op/provider/file/gen/params.gen.go

// Before:
"Join": {"parts"},

// After:
"Join": {"*parts"},
```

### 3. E2E tests

**Immediate mode:**

```starlark
# internal/e2e/testrunner/data/test_imm_file.star

# Positional args (new)
t.expect_equal(file.join("a", "b", "c.txt"), "a/b/c.txt")

# Keyword list (existing — must still work)
t.expect_equal(file.join(parts=["a", "b", "c.txt"]), "a/b/c.txt")

# Empty
t.expect_equal(file.join(), "")
```

**Planned mode:**

```starlark
# internal/e2e/testrunner/data/test_file_join.star

# Planned mode uses named slots — calling convention unchanged
plan.file.join(parts=["a", "b", "c.txt"])
```

Verify this still works after regeneration.

### 4. Verify no regressions

All 18 providers regenerated (even though only `file` changes). Verify
that providers without variadic methods are unaffected — their `Params`
entries have no `*` prefixes.

## Tasks

- [ ] Update `compute_param_names_list` in `star/.../commands/generate.star` — emit `*` for variadic
- [ ] Regenerate all providers (`make build`)
- [ ] Verify only `pkg/op/provider/file/gen/params.gen.go` changes
- [ ] Update `internal/e2e/testrunner/data/test_imm_file.star` — add positional arg test case
- [ ] Verify `internal/e2e/testrunner/data/test_file_join.star` planned mode still works
- [ ] `make check` passes
- [ ] `make test-race` passes

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star` | Modify | Emit `*` prefix for variadic params |
| `pkg/op/provider/file/gen/params.gen.go` | Regenerate | `"Join": {"*parts"}` |
| `internal/e2e/testrunner/data/test_imm_file.star` | Modify | Positional arg test |
| `internal/e2e/testrunner/data/test_file_join.star` | Verify | Planned mode unchanged |

## Exit Criteria

- `file.join("a", "b", "c.txt")` works in immediate mode (positional args)
- `file.join(parts=["a", "b", "c.txt"])` still works (keyword list)
- Planned mode `plan.file.join(parts=...)` unchanged
- No other provider's `Params` affected
- `make check` and `make test-race` pass
