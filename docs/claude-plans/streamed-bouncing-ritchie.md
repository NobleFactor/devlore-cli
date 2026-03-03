# Plan: Phase 0 — `star devlore test` Extension

## Context

The resource management plan (`docs/plans/resource-management.md`) requires a
test harness as Phase 0. The harness must plan graphs in Starlark, execute them
via the real executor, and verify expectations. The user wants it as a star
extension invoked via `star devlore test run --script <path>`.

### Architectural Constraint

The `star` binary is built from noblefactor-ops. Builtin receivers are
**hardcoded** in `noblefactor-ops/internal/starlark/runtime.go:buildPredeclared()`.
The `type:` field in extension.yaml is documentation only — it does NOT drive
instantiation. noblefactor-ops imports `devlore-cli/pkg/op` (public) but
**cannot** import `internal/starlark/` (BindingSet) or `internal/execution/`
(GraphExecutor).

The TestReceiver needs both BindingSet and GraphExecutor. To bridge this gap:

1. Create `pkg/op/testharness/` in devlore-cli — a **public** package that
   wraps the internal BindingSet + GraphExecutor. Within the same Go module,
   `pkg/` can import `internal/` freely.
2. noblefactor-ops imports `devlore-cli/pkg/op/testharness` and wraps it in
   a TestHarnessReceiver (thin adapter, starlark.HasAttrs).
3. The star extension (extension.yaml + commands/run.star) lives in devlore-cli.

This splits Phase 0 across two repos:
- **devlore-cli**: Harness package, extension definition, test scripts, un-skip tests
- **noblefactor-ops**: TestHarnessReceiver, runtime wiring

## Implementation

### Step 1: `pkg/op/testharness/` (devlore-cli)

Create the public test harness package.

**`pkg/op/testharness/harness.go`**:

```go
package testharness

// Harness plans a graph from a Starlark script and executes it.
type Harness struct { /* BindingSet, GraphExecutor, expectations */ }

type Options struct {
    Providers []string       // Filter (default: all)
    DryRun    bool
    WorkDir   string         // Temp root for filesystem ops
    Trace     bool           // Enable step-by-step trace output
    Writer    io.Writer      // Trace/status output
}

type Result struct {
    Graph            *op.Graph
    NodeCount        int
    Err              error           // Execution error (nil on success)
    Passed           bool
    ExpectationCount int
    Failures         []Failure
}

type Failure struct {
    Expectation string
    Message     string
}

func New(opts Options) *Harness
func (h *Harness) Run(ctx context.Context, scriptPath string) (*Result, error)
```

`Run()` does:
1. Create `op.Graph`, `op.ActionRegistry`
2. Create `BindingSet` via `internal/starlark`, call `RegisterActions(reg)`,
   `BuildGlobals(graph, "test", reg)` → produces `plan` root
3. Create `TNamespace` (the `t` starlark.Value) with `t.tmp()`, `t.expect_*()`
4. Merge globals: `{"plan": planRoot, "t": tNamespace}`
5. Set up thread with `ConfigureThread()`, optional trace callback
6. Execute script via `starlark.ExecFileOptions`
7. Execute graph via `GraphExecutor` from `internal/execution`
8. Check queued expectations from TNamespace
9. Return Result

Key imports (all legal within the module):
- `internal/starlark` → BindingSet, NewBindingSet, PlanRoot (via BuildGlobals)
- `internal/execution` → GraphExecutor, NewGraphExecutor, ExecutorOptions

**`pkg/op/testharness/t_namespace.go`**:

```go
// TNamespace implements the "t" starlark.HasAttrs injected into test scripts.
type TNamespace struct {
    op.Receiver
    workDir      string
    expectations []expectation
}
```

Methods:
- `t.tmp(relative)` → returns absolute path under workDir
- `t.expect_file(path, content=None)` → queue FileExpectation
- `t.expect_no_file(path)` → queue NoFileExpectation
- `t.expect_node_count(n)` → queue NodeCountExpectation
- `t.expect_error(pattern)` → queue ErrorExpectation

Pattern: `op.MakeAttr` for each method (same as UiReceiver, FileReceiver).

**`pkg/op/testharness/trace.go`**:

Step callback for `--trace` mode. Installs on `starlark.Thread` via a custom
step-counting mechanism. Uses `thread.CallStack()` for position and
`thread.DebugFrame(0)` for local variables. Writes trace lines to the
harness Writer.

**`pkg/op/testharness/harness_test.go`**:

Go tests that prove the harness works:
- `TestHarness_WriteText` — plan write_text, execute, expect_file
- `TestHarness_Copy` — plan copy, execute, expect_file
- `TestHarness_WriteAndRead` — plan write then read, expect ordering
- `TestHarness_Compensation` — plan write + failing node, expect_no_file
- `TestHarness_Trace` — enable trace, verify output contains positions
- `TestHarness_DryRun` — dry-run mode, verify no filesystem changes

Test scripts live in `pkg/op/testharness/testdata/`:
- `test_write_text.star`
- `test_copy.star`
- `test_write_and_read.star`
- `test_compensation.star`

### Step 2: Star Extension Definition (devlore-cli)

**`star/extensions/com.noblefactor.devlore.Test/extension.yaml`**:

```yaml
extension: com.noblefactor.devlore.Test
description: Test harness for Starlark graph planning and execution

receivers:
  - name: test
    builtin: true
    type: TestHarnessReceiver
    description: Graph planning, execution, and expectation verification
  - name: ui
    builtin: true
    type: UiReceiver
    description: Status output

commands:
  - name: devlore.test.run
    help: Run a Starlark test script that plans and executes a graph
    implementation: commands/run.star
    flags:
      - name: script
        type: string
        required: true
        help: Path to the test script (.star)
      - name: provider
        type: string
        default: ""
        help: "Restrict to a specific provider (default: all)"
      - name: dry-run
        type: bool
        default: "false"
        help: Execute in dry-run mode
      - name: trace
        type: bool
        default: "false"
        help: Enable step-by-step Starlark trace output
```

**`star/extensions/com.noblefactor.devlore.Test/commands/run.star`**:

```starlark
def run(ctx):
    result = test.run(
        script = ctx.args["script"],
        provider = ctx.args.get("provider", ""),
        dry_run = ctx.args.get("dry-run", False),
        trace = ctx.args.get("trace", False),
    )
    if result.passed:
        success(
            "All expectations met ("
            + str(result.expectation_count) + " expectations, "
            + str(result.node_count) + " nodes)"
        )
    else:
        for f in result.failures:
            error(f.expectation + ": " + f.message)
        fail(str(len(result.failures)) + " expectation(s) failed")
```

### Step 3: TestHarnessReceiver (noblefactor-ops)

**`internal/starlark/receiver_test_harness.go`**:

```go
type TestHarnessReceiver struct {
    op.Receiver
}

func NewTestHarnessReceiver() *TestHarnessReceiver {
    return &TestHarnessReceiver{Receiver: op.NewReceiver("test")}
}

func (r *TestHarnessReceiver) Attr(name string) (starlark.Value, error) {
    switch name {
    case "run":
        return op.MakeAttr("test.run", r.run), nil
    }
    return nil, nil
}

func (r *TestHarnessReceiver) run(thread *starlark.Thread, ...) (starlark.Value, error) {
    // Unpack script, provider, dry_run, trace kwargs
    h := testharness.New(testharness.Options{...})
    result, err := h.Run(ctx, script)
    // Convert Result to starlark.Value (struct-like)
}
```

**`internal/starlark/runtime.go`** changes:
- Add `testHarness *TestHarnessReceiver` to Runtime struct
- Init in `NewRuntime()`: `testHarness: NewTestHarnessReceiver()`
- Add `"test": r.testHarness` to `buildPredeclared()`

**`go.mod`** update: bump devlore-cli dependency to pick up `pkg/op/testharness`.

### Step 4: Un-skip Integration Tests (devlore-cli)

Fix `pkg/op/provider/file/gen/integration_test.go`:
- Remove `t.Skip(...)` for issues #170 and #171
- Run both `TestImmediateBindings` and `TestPlannedBindings`
- Investigate and fix any failures (likely minor issues in test scripts
  or receiver wiring that the skip has been hiding)

### Step 5: Debugging Support

Already built into the harness:

**Layer 1 — Go test entry point (free)**:
- `pkg/op/testharness/harness_test.go` tests run in GoLand debugger
- Set breakpoints at: `PlannedReceiver.CallInternal`, `FillSlot`,
  `GraphExecutor.runFlat`, `reflectedAction.Do`, `coerceSlotValue`,
  `Provider.WriteText`, etc.

**Layer 2 — Trace mode (`--trace`)**:
- `pkg/op/testharness/trace.go` installs step callback
- Logs file:line, locals, node/edge creation
- Conditional breakpoints on the step callback = "Starlark breakpoints"

## Files to Create/Modify

| File | Repo | Action |
| --- | --- | --- |
| `pkg/op/testharness/harness.go` | devlore-cli | Create |
| `pkg/op/testharness/t_namespace.go` | devlore-cli | Create |
| `pkg/op/testharness/trace.go` | devlore-cli | Create |
| `pkg/op/testharness/harness_test.go` | devlore-cli | Create |
| `pkg/op/testharness/testdata/test_*.star` | devlore-cli | Create (4 scripts) |
| `star/extensions/com.noblefactor.devlore.Test/extension.yaml` | devlore-cli | Create |
| `star/extensions/com.noblefactor.devlore.Test/commands/run.star` | devlore-cli | Create |
| `pkg/op/provider/file/gen/integration_test.go` | devlore-cli | Modify (un-skip) |
| `internal/starlark/receiver_test_harness.go` | noblefactor-ops | Create |
| `internal/starlark/runtime.go` | noblefactor-ops | Modify |
| `internal/starlark/receivers.go` | noblefactor-ops | Modify |
| `go.mod` | noblefactor-ops | Modify |

## Delivery Order

1. **PR to devlore-cli**: Steps 1, 2, 4 — harness package, extension def,
   test scripts, un-skip integration tests. Immediately usable via Go tests
   in GoLand. `make check` passes.

2. **PR to noblefactor-ops**: Step 3 — TestHarnessReceiver, runtime wiring,
   dependency update. After merge, `star devlore test run` works end-to-end.

## Verification

1. `make check` passes in devlore-cli (harness Go tests, un-skipped integration tests)
2. Go tests in GoLand: run `TestHarness_WriteText` with debugger, set breakpoint
   in `Provider.WriteText`, verify it hits
3. `star devlore test run --script testdata/test_write_text.star` passes
   (after noblefactor-ops PR merges)
4. `star devlore test run --trace --script testdata/test_write_text.star`
   shows file:line trace output
