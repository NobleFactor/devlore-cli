# Phase 0: `devlore-test` — Graph Test Harness Binary

## Context

The resource-management plan (Phase 0) calls for a test harness that closes
the gap between planning and execution testing. Today no test does the full
loop: **Starlark plan -> graph -> execute -> verify results -> verify
compensation**. Before any resource management work begins, we need a tool
that proves the existing infrastructure works end-to-end.

**Design change from the original plan**: `devlore-test` is a **separate
binary** (`cmd/devlore-test/`) with process-level isolation, not an embedded
receiver. It runs under `star` command control via an extension that shells
out to the binary (CLI args + stdout + exit code). No noblefactor-ops spec
changes — the `.star` command resolves the binary path by convention.

## Architecture

```
star devlore test run --script path.star
        |
        v
   run.star (extension command)
        |  shells out to devlore-test binary
        v
   devlore-test --script path.star [--dry-run] [--trace]
        |
        |  1. Creates temp dir
        |  2. Sets up BindingSet -> plan namespace
        |  3. Injects `t` namespace (expectations)
        |  4. Executes .star script (plan.* builds graph)
        |  5. Executes graph via GraphExecutor
        |  6. Checks queued expectations
        |  7. Writes structured results to stdout
        |  8. Exit code: 0 = pass, 1 = fail, 2 = error
        v
   run.star reads stdout, reports via ui.*
```

## Binary: `cmd/devlore-test/main.go`

Follows the lore/writ pattern: minimal main, delegates to an internal
package.

### Dependencies

```go
import (
    "github.com/NobleFactor/devlore-cli/internal/e2e/testrunner" // testrunner.New(...)
    "github.com/NobleFactor/devlore-cli/internal/starlark"   // BindingSet
    "github.com/NobleFactor/devlore-cli/internal/execution"   // GraphExecutor
    "github.com/NobleFactor/devlore-cli/pkg/op"               // Graph, Node, Resource
    "github.com/NobleFactor/devlore-cli/pkg/op/provider/platform" // Platform detection
    _ "github.com/NobleFactor/devlore-cli/pkg/op/provider"    // Blank import registers providers
)
```

### CLI Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--script` | string | required | Path to test `.star` script |
| `--dry-run` | bool | false | Plan only, no side effects |
| `--trace` | bool | false | Starlark step-by-step trace logging |
| `--provider` | string | "" | Restrict to a specific provider |

### Initialization Order

1. Parse flags
2. Create temp directory (cleaned up on exit)
3. Create `BindingSet` with `op.BindingConfig{}` + `.With("plan")`
4. Create `ActionRegistry` via `bs.NewPopulatedRegistry()`
5. Create `Platform` via `platform.New()`
6. Create `op.Graph("devlore-test")`
7. Build Starlark globals from `bs.BuildGlobals(graph, "", registry)`
8. Add `t` namespace (TestContext) to globals
9. Execute `.star` script — `plan.*` calls build graph nodes,
   `t.expect_*` calls queue expectations
10. Create `GraphExecutor` with options
11. Execute graph via `executor.Run(ctx, graph)`
12. Check queued expectations against results
13. Write JSON result to stdout, exit with appropriate code

### Stdout Protocol

JSON object on stdout:

```json
{
  "passed": true,
  "node_count": 3,
  "expectation_count": 2,
  "failures": [],
  "trace": ["line 4: dest = /tmp/test-xxx/foo.txt", "..."]
}
```

On failure, `failures` contains objects:

```json
{
  "expectation": "file_exists(/tmp/test-xxx/foo.txt)",
  "message": "file not found"
}
```

Exit codes: `0` = all pass, `1` = expectation failure, `2` = harness error.

## Internal Package: `internal/e2e/testrunner/`

Sibling to the existing LLM accuracy harness in `internal/e2e/`. The LLM
harness tests migrate/onboard via provider configs and F1 metrics. This
package tests the graph pipeline: Starlark plan -> graph -> execute -> verify.

### `runner.go` — Test Runner

Core orchestration: temp dir, BindingSet setup, script execution, graph
execution, expectation checking. This is the bulk of the logic.

Key type:

```go
type Runner struct {
    script   string
    dryRun   bool
    trace    bool
    provider string
}

// testrunner.New(script, opts...)
func New(script string, opts ...Option) *Runner

type Result struct {
    Passed           bool       `json:"passed"`
    NodeCount        int        `json:"node_count"`
    ExpectationCount int        `json:"expectation_count"`
    Failures         []Failure  `json:"failures"`
    Trace            []string   `json:"trace,omitempty"`
}

func (r *Runner) Start(ctx context.Context) (*Result, error)
```

Usage:
```go
runner := testrunner.New("path/to/test.star", testrunner.WithDryRun(), testrunner.WithTrace())
result, err := runner.Start(ctx)
```

### `test_context.go` — The `t` Namespace

Starlark receiver injected as the `t` global. Queues expectations during
script execution, checks them after graph execution.

Methods:
- `t.tmp(relative)` — absolute path under test temp dir
- `t.expect_file(path, content=None)` — file exists after execution
- `t.expect_no_file(path)` — file does NOT exist (compensation)
- `t.expect_node_count(n)` — graph node count
- `t.expect_error(pattern)` — execution fails with matching error

### `runner_test.go` — Go-Level Tests

Go test entry points that call `Runner.Start()` directly. These are the
GoLand-debuggable entry points — set breakpoints on `CallInternal`,
`FillSlot`, `runFlat`, `coerceSlotValue`, provider methods.

### `trace.go` — Starlark Trace Mode

Step callback on Starlark `Thread` using `thread.CallStack()` for position
and `thread.DebugFrame(0)` for local variable inspection. Enabled by
`--trace` flag.

## Star Extension: `com.noblefactor.devlore.Test`

```
star/extensions/com.noblefactor.devlore.Test/
  extension.yaml
  commands/
    run.star
```

### `extension.yaml`

```yaml
extension: com.noblefactor.devlore.Test
description: Test harness for Starlark graph planning and execution

receivers:
  - name: ui
    builtin: true
    type: UiReceiver
    description: Status output (note, warn, success, fail)

commands:
  - name: devlore.test.run
    help: Run a Starlark test script that plans and executes a graph
    implementation: commands/run.star
    flags:
      - name: script
        type: string
        required: true
        help: Path to the test script (.star)
      - name: dry-run
        type: bool
        default: "false"
        help: "Plan only, no side effects"
      - name: trace
        type: bool
        default: "false"
        help: "Enable Starlark step trace"
      - name: tool-path
        type: string
        default: ""
        help: "Path to devlore-test binary (overrides config and env)"
```

### `commands/run.star`

Resolves the `devlore-test` binary via a 3-tier lookup:

1. `--tool-path` flag (explicit override on the star command)
2. `DEVLORE_TEST_TOOL_PATH` environment variable
3. Extension config `test.tool_path` (default: `build/devlore-test`
   relative to the worktree root — the same `build/` directory where
   `make build` puts `lore` and `writ`)

Shells out with flags, parses JSON stdout, reports via `ui.*`.

### Extension Config

```yaml
config:
  path: "test"
  fields:
    tool_path: string
  defaults:
    tool_path: "build/devlore-test"  # relative to worktree root
```

The `run.star` command resolves `build/devlore-test` relative to the git
worktree root (the same root that `star` uses for extension discovery via
`${GIT_WORKSPACE_ROOT}/star/extensions/`). This ensures `make build` +
`star devlore test run` works without any configuration.

## Test Scripts

Live in `internal/e2e/testrunner/data/`. Naming: `test_<scenario>.star`.

### Baseline Scripts (before any resource management work)

| Script | Plans | Verifies |
|--------|-------|----------|
| `test_write_text.star` | `plan.file.write_text` | File exists with correct content |
| `test_copy.star` | `plan.file.copy` | Destination matches source |
| `test_write_and_read.star` | Write then read same path | Correct ordering + content |
| `test_compensation.star` | Write + failing node | Compensation restores state |

### Example Script

```starlark
# test_write_text.star
dest = t.tmp("hello.txt")
plan.file.write_text(destination=dest, content="hello world", mode=0o644)
t.expect_file(dest, content="hello world")
t.expect_node_count(1)
```

## Makefile Integration

Add `devlore-test` to the build:

```make
build: generate
	go build $(LDFLAGS) -o build/lore ./cmd/lore
	go build $(LDFLAGS) -o build/writ ./cmd/writ
	go build $(LDFLAGS) -o build/devlore-test ./cmd/devlore-test
```

## Un-skip Planned Binding Tests

Fix skipped tests at `pkg/op/provider/file/gen/integration_test.go`
(issues #170, #171). These become exercised by the harness through test
scripts that use the file provider's planned receivers.

## Files

| File | Action | Purpose |
|------|--------|---------|
| `cmd/devlore-test/main.go` | Create | Binary entry point |
| `internal/e2e/testrunner/runner.go` | Create | Test runner orchestration |
| `internal/e2e/testrunner/runner_test.go` | Create | Go-level test entry points |
| `internal/e2e/testrunner/test_context.go` | Create | `t` namespace (Starlark receiver) |
| `internal/e2e/testrunner/trace.go` | Create | Starlark trace mode |
| `internal/e2e/testrunner/data/test_write_text.star` | Create | Baseline test |
| `internal/e2e/testrunner/data/test_copy.star` | Create | Baseline test |
| `internal/e2e/testrunner/data/test_write_and_read.star` | Create | Baseline test |
| `internal/e2e/testrunner/data/test_compensation.star` | Create | Baseline test |
| `star/extensions/com.noblefactor.devlore.Test/extension.yaml` | Create | Extension spec |
| `star/extensions/com.noblefactor.devlore.Test/commands/run.star` | Create | Star command wrapper |
| `Makefile` | Modify | Add devlore-test build target |
| `pkg/op/provider/file/gen/integration_test.go` | Modify | Un-skip #170, #171 |

## Verification

1. `make build` — compiles lore, writ, and devlore-test
2. `make test` — Go-level tests in `internal/e2e/testrunner/` pass
3. `build/devlore-test --script internal/e2e/testrunner/data/test_write_text.star` — runs, exits 0
4. `build/devlore-test --script ... --trace` — trace output visible
5. `star devlore test run --script ...` — extension invokes binary, reports results
6. All 4 baseline scripts pass (write, copy, write+read, compensation)
