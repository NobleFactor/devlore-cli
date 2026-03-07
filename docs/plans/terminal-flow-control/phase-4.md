---
title: "Phase 4: E2E Tests"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../terminal-flow-control.md
---

# Phase 4: E2E Tests

## Summary

Add statement-level Starlark e2e tests for all three terminal flow
control nodes. Each test exercises the Starlark binding through the
test runner, verifying the full path from script to graph execution.

## Deliverables

### 1. Complete test

```starlark
# internal/e2e/testrunner/data/test_flow_complete.star

# plan.flow.complete with output value
plan.flow.complete(output=42)

# plan.flow.complete with no output (nil)
plan.flow.complete()
```

Dry-run test: verify the graph contains `flow.complete` nodes with
correct slot values.

### 2. Degraded test

```starlark
# internal/e2e/testrunner/data/test_flow_degraded.star

# Plain string message
plan.flow.degraded('disk space low')

# Template with kwargs from upstream promise
written = plan.file.write_text(destination=t.tmp("d.txt"), content="ok", mode=0o644)
plan.flow.degraded('wrote {{ .path }}', path=written)
```

Dry-run test: verify the graph contains `flow.degraded` nodes.
Full execution test: verify the graph completes (not failed) and the
node output contains the formatted warning.

### 3. Fatal test

```starlark
# internal/e2e/testrunner/data/test_flow_fatal.star

# Plain string — halts execution
plan.flow.fatal('database unreachable')
```

Full execution test: verify the graph state is `StateFailed`.

### 4. Fatal with template test

```starlark
# internal/e2e/testrunner/data/test_flow_fatal_template.star

# Template with kwargs — promise resolves before formatting
svc = plan.file.write_text(destination=t.tmp("svc.txt"), content="myapp", mode=0o644)
plan.flow.fatal('{{ .service }} startup failed.', service=svc)
```

Full execution test: verify the fatal error message contains the
resolved value.

### 5. Fatal with recovery test

```starlark
# internal/e2e/testrunner/data/test_flow_fatal_exec.star

# Verify compensable actions before fatal are unwound
plan.file.copy(source="a.txt", destination="b.txt")
plan.flow.fatal('abort after copy')
```

Full execution test: verify the copy action's compensation runs during
unwind.

### 6. Test runner support

The test runner (`internal/e2e/testrunner`) needs `WithReceivers("plan",
"flow")` or similar to include the flow receiver. Verify that the flow
planned receiver is available in test scripts via `plan.flow.*`.

### 7. Test functions

```go
// internal/e2e/testrunner/runner_test.go

func TestFlowComplete(t *testing.T)       { runScriptDryRun(t, "test_flow_complete") }
func TestFlowDegraded(t *testing.T)       { runScriptDryRun(t, "test_flow_degraded") }
func TestFlowFatal(t *testing.T)          { runScript(t, "test_flow_fatal") }
func TestFlowFatalTemplate(t *testing.T)  { runScript(t, "test_flow_fatal_template") }
func TestFlowFatalExec(t *testing.T)      { runScript(t, "test_flow_fatal_exec") }
```

Exact function signatures depend on what test helpers exist after
Phases 1-3.

## Tasks

- [ ] Create `internal/e2e/testrunner/data/test_flow_complete.star`
- [ ] Create `internal/e2e/testrunner/data/test_flow_degraded.star`
- [ ] Create `internal/e2e/testrunner/data/test_flow_fatal.star`
- [ ] Create `internal/e2e/testrunner/data/test_flow_fatal_template.star`
- [ ] Create `internal/e2e/testrunner/data/test_flow_fatal_exec.star`
- [ ] Add test functions in `internal/e2e/testrunner/runner_test.go`
- [ ] Verify `plan.flow.*` methods accessible in test scripts
- [ ] `make check` passes
- [ ] `make test-race` passes

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `internal/e2e/testrunner/data/test_flow_complete.star` | Create | Complete e2e test |
| `internal/e2e/testrunner/data/test_flow_degraded.star` | Create | Degraded e2e test |
| `internal/e2e/testrunner/data/test_flow_fatal.star` | Create | Fatal e2e test |
| `internal/e2e/testrunner/data/test_flow_fatal_template.star` | Create | Fatal template test |
| `internal/e2e/testrunner/data/test_flow_fatal_exec.star` | Create | Fatal + recovery test |
| `internal/e2e/testrunner/runner_test.go` | Modify | Test functions |

## Exit Criteria

- All five test scripts pass in the test runner
- Complete: graph contains `flow.complete` node with correct output
- Degraded: graph continues, node output contains formatted warning
- Fatal: graph halts, state is `StateFailed`, message formatted correctly
- Fatal with template: promise-based kwargs resolve and format
- `make check` and `make test-race` pass
