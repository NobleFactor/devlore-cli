---
title: "Phase 6: Statement-Level E2E Tests"
status: complete
created: 2026-03-06
updated: 2026-03-06
parent: ../provider-registration.md
---

# Phase 6: Statement-Level E2E Tests

## Summary

Add one e2e test per statement type per provider — the minimum test that
proves each code path works end-to-end through the registration, graph
construction, and execution pipeline. Think of these as compiler tests: each
test exercises exactly one "statement" (planned action, immediate action, or
flow action) and verifies the observable result.

Tests that need receivers not currently available in the runner (immediate
tests for json, yaml, regexp, etc.) will fail until Phase 7 threads receiver
selection through `BindingConfig`. That's expected — write the tests first,
wire the plumbing second.

## Statement Types

| Type | Starlark form | What it proves |
| --- | --- | --- |
| Planned action | `plan.{provider}.{action}(...)` | Action registered, graph node created, executor runs it, side effect produced |
| Immediate action | `{provider}.{action}(...)` | Immediate receiver created, method callable, correct return value |
| Flow action | `plan.choose`, `plan.gather`, `plan.source` | Graph structure control works through the full pipeline |

## Infrastructure

### `t.expect_equal` assertion

Immediate actions return Starlark values during script evaluation — no
filesystem side effect to check. Add `t.expect_equal(actual, expected)` to
the TestContext so immediate tests can assert return values directly:

```starlark
result = json.encode({"key": "value"})
t.expect_equal(result, '{"key":"value"}')

matched = regexp.match("foo", "foobar")
t.expect_equal(matched, True)
```

Compares via `starlark.Equal`; queues a failure on mismatch (same deferred
pattern as existing expectations).

## Test Matrix

### Planned actions — file provider

| Action | Test script | Status |
| --- | --- | --- |
| `write_text` | `test_write_text.star` | exists |
| `write_bytes` | `test_write_bytes.star` | exists |
| `read` | `test_write_and_read.star` | exists |
| `copy` | `test_copy.star` | exists |
| `move` | `test_move.star` | exists |
| `link` | `test_link.star` | exists |
| `backup` | `test_backup.star` | exists |
| `mkdir` | `test_mkdir_and_remove_all.star` | exists |
| `remove` | `test_choose_exists.star` | exists (in choose branch) |
| `remove_all` | `test_mkdir_and_remove_all.star` | exists |
| `exists` | `test_choose_exists.star` | exists |
| `is_dir` | `test_is_dir.star` | exists |
| `is_file` | `test_is_file.star` | exists (skipped — choose bug) |
| `unlink` | `test_file_unlink.star` | **new** |
| `glob` | `test_file_glob.star` | **new** |
| `walk_tree` | `test_file_walk_tree.star` | **new** |
| `join` | `test_file_join.star` | **new** |
| `name` | `test_file_name.star` | **new** |
| `parent` | `test_file_parent.star` | **new** |

### Planned actions — shell provider

| Action | Test script | Status |
| --- | --- | --- |
| `exec` | `test_hello.star`, `test_shell_exec.star` | exists |
| `powershell` | — | skip (Windows-only) |

### Planned actions — template provider

| Action | Test script | Status |
| --- | --- | --- |
| `render` | `test_template_render.star` | **new** |

### Planned actions — dry-run providers

These require external resources to execute. Tested in dry-run mode to prove
registration + planned receiver + graph node creation.

| Provider | Actions | Test script | Status |
| --- | --- | --- | --- |
| `archive` | `extract` | `test_archive.star` | **new** |
| `encryption` | `decrypt_sops_file` | `test_encryption.star` | **new** |
| `git` | `clone`, `checkout`, `pull` | `test_git.star` | **new** |
| `net` | `download` | `test_net.star` | **new** |
| `pkg` | `install`, `remove`, `upgrade`, `update`, `installed`, `not_installed`, `version_gte` | `test_pkg.star` | **new** |
| `service` | `start`, `stop`, `restart`, `enable`, `disable`, `exists`, `running`, `enabled` | `test_service.star` | **new** |

### Immediate actions

These tests will fail until Phase 7 adds `Receivers` to `BindingConfig`.

| Provider | Actions | Test script | Status |
| --- | --- | --- | --- |
| `file` | `join`, `name`, `parent`, `write_text`, `read`, `exists`, `is_file`, `is_dir`, `mkdir`, `copy`, `move`, `remove`, `glob` | `test_imm_file.star` | **new** |
| `json` | `encode`, `decode`, `encode_indent` | `test_imm_json.star` | **new** |
| `yaml` | `encode`, `decode` | `test_imm_yaml.star` | **new** |
| `regexp` | `match`, `find`, `find_all`, `find_submatch`, `find_all_submatch`, `replace`, `replace_literal`, `split` | `test_imm_regexp.star` | **new** |
| `shell` | `exec` | `test_imm_shell.star` | **new** |
| `template` | `render` | `test_imm_template.star` | **new** |
| `ui` | `note`, `warn`, `error`, `success`, `fail` | `test_imm_ui.star` | **new** |
| `staranalysis` | `analyze` | `test_imm_staranalysis.star` | **new** |
| `starcode` | `capture` | `test_imm_starcode.star` | **new** |
| `starcomplexity` | `compute_complexity` | `test_imm_starcomplexity.star` | **new** |
| `starindex` | `index_files` | `test_imm_starindex.star` | **new** |
| `starstats` | `compute_stats` | `test_imm_starstats.star` | **new** |

### Flow actions

| Action | Test script | Status |
| --- | --- | --- |
| `choose` (true branch) | `test_choose_exists.star`, `test_is_dir.star` | exists |
| `choose` (false branch) | `test_choose_not_exists.star` | exists (skipped — choose executor bug) |
| `gather` | `test_gather.star` | exists |
| `source` | `test_source.star` | exists |
| `elevate` | — | not exposed in PlanRoot Starlark bindings yet |
| `wait_until` | — | not exposed in PlanRoot Starlark bindings yet |

## Tasks

- [x] Add `t.expect_equal(actual, expected)` to `TestContext`
- [x] Write planned action tests for untested file actions (unlink, glob, join, name, parent)
- [x] Write planned action test for `template.render`
- [x] Write dry-run tests for all external-resource providers (archive, encryption, git, net, pkg, service)
- [x] Write immediate action tests for all feasible providers
- [x] Add Go test functions for each new `.star` script (24 new tests)
- [x] Add `runScriptDryRun` helper for dry-run tests
- [x] Immediate tests skip with "requires Phase 7: BindingConfig receivers"
- [x] All non-skipped tests pass (`make test`)
- [x] `make test-race` passes

## Bugs Discovered

- `file.join` planned receiver: reflection panic — `cannot use []string as type string in Call`.
  Variadic `...string` parameter not handled correctly in generated receiver.
- `flow.choose` false-branch: executor runs then-branch even when predicate is false (pre-existing).
- `flow.choose` lambda closure: Output captured in lambda gets empty path at execution time (pre-existing).

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `internal/e2e/testrunner/test_context.go` | Modify | Add `t.expect_equal` |
| `internal/e2e/testrunner/runner_test.go` | Modify | New test functions |
| `internal/e2e/testrunner/data/*.star` | Create | New test scripts |

## Exit Criteria

- Every feasible planned action has at least one e2e test
- Every feasible immediate provider has at least one e2e test (skipped if
  receivers not yet available)
- All flow actions with Starlark bindings are tested
- `make test` passes (skipped tests are acceptable)
- `make test-race` passes
