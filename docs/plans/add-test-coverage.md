---
title: "Provider and Flow Integration Tests"
issue: TBD
status: in-progress
created: 2026-03-17
updated: 2026-03-17
---

# Plan: Provider and Flow Integration Tests

## Summary

Add black-box integration tests for every provider and flow element. Only starcode has Starlark integration tests today — all other providers (19) and flow have zero. Every provider must be tested through the Starlark executing receiver and through graph action dispatch. Generated tests are out of scope — they are the code generator's responsibility.

## Goals

1. **Starlark integration tests** for every provider: execute a `.star` script that exercises the provider's full API surface via the executing receiver
2. **Graph action tests** for every provider: exercise `action.Do()` with a real context, verifying result values
3. **Flow integration tests**: exercise flow actions (choose, gather, elevate, wait_until, complete, degraded, fatal) through both the Plan receiver and action dispatch

## Current State

| Provider | Hand-written tests | Starlark `.star` | Integration test | Gap |
| --- | --- | --- | --- | --- |
| platform | 0 | 0 | 0 | All three paths missing |
| json | 1 | 0 | 0 | Starlark + action tests missing |
| yaml | 1 | 0 | 0 | Starlark + action tests missing |
| regexp | 1 | 0 | 0 | Starlark + action tests missing |
| ui | 1 | 0 | 0 | Starlark + action tests missing |
| shell | 1 | 0 | 0 | Starlark + action tests missing |
| template | 1 | 0 | 0 | Starlark + action tests missing |
| file | 1 | 0 | 0 | Starlark + action tests missing |
| archive | 1 | 0 | 0 | Starlark + action tests missing |
| encryption | 1 | 0 | 0 | Starlark + action tests missing |
| git | 2 | 0 | 0 | Starlark + action tests missing |
| appnet | 2 | 0 | 0 | Starlark + action tests missing |
| pkg | 2 | 0 | 0 | Starlark + action tests missing |
| service | 1 | 0 | 0 | Starlark + action tests missing |
| mem | 4 | 0 | 0 | Starlark + action tests missing |
| starcode | 3 | 6 | 1 | Reference implementation — complete |
| staranalysis | 1 | 0 | 0 | Starlark + action tests missing |
| starcomplexity | 2 | 0 | 0 | Starlark + action tests missing |
| starindex | 1 | 0 | 0 | Starlark + action tests missing |
| starstats | 1 | 0 | 0 | Starlark + action tests missing |
| **flow** | 1 (unit) | 0 | 0 | Starlark plan + action tests missing |

## Implementation Phases

### Phase 1: Pure-data providers (no I/O, no mocks needed)

These providers are self-contained — no filesystem, network, or platform dependencies. Ideal starting point.

- [x] **platform** — `integration_test.go` + `testdata/integration.star`; inject known `op.Platform` on context
- [x] **json** — encode/decode round-trips through Starlark
- [x] **yaml** — encode/decode round-trips through Starlark
- [x] **regexp** — all 8 methods: match, find, findAll, findSubmatch, findAllSubmatch, replace, replaceLiteral, split
- [x] **template** — DEFERRED to Phase 7 (refactor first)

Each provider gets:
- `testdata/integration.star` — exercises full API, sets `result_*` globals
- `integration_test.go` — builds context, wraps in `ExecutingReceiver`, executes script, asserts globals

**Files** (10 new):

| File | Action |
| --- | --- |
| `pkg/op/provider/platform/testdata/integration.star` | Create |
| `pkg/op/provider/platform/integration_test.go` | Create |
| `pkg/op/provider/json/testdata/integration.star` | Create |
| `pkg/op/provider/json/integration_test.go` | Create |
| `pkg/op/provider/yaml/testdata/integration.star` | Create |
| `pkg/op/provider/yaml/integration_test.go` | Create |
| `pkg/op/provider/regexp/testdata/integration.star` | Create |
| `pkg/op/provider/regexp/integration_test.go` | Create |
### Phase 2: UI and shell providers

These have side effects but are testable with buffer writers and controlled environments.

- [x] **ui** — error, note, success, warn, fail; capture writer output in Starlark test
- [x] **shell** — exec with a trivial command (e.g., `echo hello`)

**Files** (4 new):

| File | Action |
| --- | --- |
| `pkg/op/provider/ui/testdata/integration.star` | Create |
| `pkg/op/provider/ui/integration_test.go` | Create |
| `pkg/op/provider/shell/testdata/integration.star` | Create |
| `pkg/op/provider/shell/integration_test.go` | Create |

### Phase 3: Filesystem providers

Require temp directories and `op.Root` setup.

- [x] **file** — writeText, readText, copy, link, remove, exists, isFile, isDir, mkdir, join, name, parent, root
- [x] **archive** — extract tar.gz into temp dir, verify extraction + compensation tombstone

**Files** (4 new):

| File | Action |
| --- | --- |
| `pkg/op/provider/file/testdata/integration.star` | Create |
| `pkg/op/provider/file/integration_test.go` | Create |
| `pkg/op/provider/archive/testdata/integration.star` | Create |
| `pkg/op/provider/archive/integration_test.go` | Create |

### Phase 4: Starlark analysis providers

These are already tested at the Go level but not through the Starlark runtime.

- [x] **staranalysis** — analyze with config dict
- [x] **starcomplexity** — computeComplexity on test .star files
- [x] **starindex** — indexFiles with docstrings/globals flags
- [x] **starstats** — computeStats with bytes/loc flags

**Files** (8 new):

| File | Action |
| --- | --- |
| `pkg/op/provider/staranalysis/testdata/integration.star` | Create |
| `pkg/op/provider/staranalysis/integration_test.go` | Create |
| `pkg/op/provider/starcomplexity/testdata/integration.star` | Create |
| `pkg/op/provider/starcomplexity/integration_test.go` | Create |
| `pkg/op/provider/starindex/testdata/integration.star` | Create |
| `pkg/op/provider/starindex/integration_test.go` | Create |
| `pkg/op/provider/starstats/testdata/integration.star` | Create |
| `pkg/op/provider/starstats/integration_test.go` | Create |

### Phase 5: External-dependency providers (mocked or skippable)

These need network/platform services. Use interface mocks or skip when unavailable.

- [ ] **appnet** — download with a test HTTP server
- [ ] **git** — clone/checkout/pull with a local bare repo
- [ ] **encryption** — decrypt with a test SOPS fixture
- [ ] **pkg** — install/remove/upgrade with mock PackageManager
- [ ] **service** — enable/disable/start/stop with mock ServiceManager
- [ ] **mem** — memory provider operations

**Files** (12 new):

| File | Action |
| --- | --- |
| `pkg/op/provider/appnet/testdata/integration.star` | Create |
| `pkg/op/provider/appnet/integration_test.go` | Create |
| `pkg/op/provider/git/testdata/integration.star` | Create |
| `pkg/op/provider/git/integration_test.go` | Create |
| `pkg/op/provider/encryption/testdata/integration.star` | Create |
| `pkg/op/provider/encryption/integration_test.go` | Create |
| `pkg/op/provider/pkg/testdata/integration.star` | Create |
| `pkg/op/provider/pkg/integration_test.go` | Create |
| `pkg/op/provider/service/testdata/integration.star` | Create |
| `pkg/op/provider/service/integration_test.go` | Create |
| `pkg/op/provider/mem/testdata/integration.star` | Create |
| `pkg/op/provider/mem/integration_test.go` | Create |

### Phase 6: Flow actions

Flow is handwritten (not generated). Test through the `Plan` receiver and action `Do()`.

- [ ] **complete** — create terminal node via Plan, execute action
- [ ] **degraded** — format string with args/kwargs, verify error message
- [ ] **fatal** — verify FatalError return
- [ ] **choose** — branch selection with condition evaluation
- [ ] **gather** — parallel fan-out collection
- [ ] **elevate** — privilege escalation marker
- [ ] **wait_until** — condition polling

**Files** (2 new):

| File | Action |
| --- | --- |
| `pkg/op/flow/testdata/integration.star` | Create |
| `pkg/op/flow/integration_test.go` | Create |

## Test Structure

Each `integration_test.go` contains two subtests:

1. `TestStarlark` — wraps provider in `ExecutingReceiver`, runs `testdata/integration.star`, asserts result globals
2. `TestActions` — registers actions via `RegisterActions`, calls `action.Do()` with a real context and params map, asserts result values

Same context setup, same provider instance, different entry points. One file per provider covers both the Starlark reflection bridge and the action dispatch bridge.

### Phase 7: Template provider refactor

The current `Render` method conflates template execution with metadata injection. `source`, `path`, and `project` are pipeline metadata — they describe where content came from and where it's going. They belong on the context or in the caller's data map, not as provider method parameters.

**Refactor:**

1. Replace `Render` with two entry points:
   - `RenderText(content string, data map[string]any) (string, error)`
   - `RenderBytes(content []byte, data map[string]any) ([]byte, error)`
2. Remove `source`, `path`, `project` parameters — callers inject these into `data` if needed
3. Update all callers in `internal/execution/` to populate `data["Source"]`, `data["Target"]`, `data["Project"]` before calling render
4. Update codegen params, regenerate
5. Add integration tests (Starlark + action dispatch)

**Files**:

| File | Action |
| --- | --- |
| `pkg/op/provider/template/provider.go` | Modify |
| `pkg/op/provider/template/provider_test.go` | Modify |
| `pkg/op/provider/template/gen/params.gen.go` | Regenerate |
| `internal/execution/plan.go` | Modify |
| `internal/execution/execution_test.go` | Modify |
| `internal/execution/lifecycle_test.go` | Modify |
| `pkg/op/provider/template/testdata/integration.star` | Create |
| `pkg/op/provider/template/integration_test.go` | Modify |

## Out of Scope

- Modifying or adding tests in `gen/` directories — those are the code generator's responsibility
- Unit tests on provider internals — focus is black-box through Starlark and graph actions
- starcode — already has complete integration test coverage
