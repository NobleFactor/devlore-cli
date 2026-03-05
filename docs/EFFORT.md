# Project Effort Report

Generated: 2026-03-04

## Code Quality Assessment

The architecture is a 9, the implementation is an 8, dragged down by type
erasure that the language forces on the codebase.

**Architecture discipline is high.** The provider/action/resource/graph
separation is clean. The three-mode split (action/immediate/planned) is
consistent across all 20 providers. The compensation pattern is
well-executed — `moveToRecovery` / `restoreFromRecovery` with
platform-specific recovery bases, same-device rename guarantees,
UUID-named recovery paths. This isn't accidental — someone thought about
failure modes.

**The reflection code is the best it can be given Go's constraints.**
`coerceSlotValue` has a clear coercion chain with well-ordered fallbacks.
`classifyActionReturn` and `classifyReturn` are the same logic split
correctly for two contexts (Go-native vs Starlark). `shadowResult` handles
value types, pointer types, and slices of resources — all the cases that
actually arise. The code comments explain *why*, not *what*.

**Test ratio is exceptional.** 32,791 test LOC against 34,533 source LOC —
nearly 1:1. The W08 refactoring (source dropped 3,400 lines, tests grew
6,000) shows willingness to delete code and backfill coverage. That's rare.

**The codegen boundary is respected.** Generated files are never hand-edited.
The `star` tool is the source of truth. `Compensate*` methods are required
to exist (panic at registration if missing). These are guardrails that
prevent drift.

**The "no legacy" principle is enforced.** No backward compatibility shims,
deprecated paths, or fallback behavior. The code does one thing.

**Where it's weaker:**

- **The `Result = any` / `UndoState = any` problem.** Type information is
  destroyed at the reflection boundary and recovered via runtime assertions
  that can fail silently or with unhelpful errors. This is the single
  biggest quality issue — and it's forced by Go's type system, not by
  design intent.

- **The `init()` registration pattern** is invisible coupling. Constructor
  registrations in `resource.go` `init()` functions are
  action-at-a-distance — if someone forgets to import a package, the
  constructor silently doesn't register, and the error surfaces later as a
  coercion failure. Go has no way to make this visible.

- **`sync.Map` for registries** is correct but obscures intent. The
  type-erased `func(any) (any, error)` stored in `constructorRegistry` is
  a consequence of Go's generics limitations — the `RegisterConstructor[T]`
  generic wrapper hides it, but the underlying storage is still untyped.

These three issues are all manifestations of the same root cause: Go's type
system forces runtime workarounds for things that should be compile-time
guarantees. The code quality is high *within* those constraints.

## External Evaluation

Independent architectural evaluation of devlore-cli's design and
implementation.

### Architecture

The core architectural strength is the **Provider/Operation Graph**. Instead
of hardcoding system calls, the project defines platform-agnostic provider
interfaces (Git, Service, Package Manager, Filesystem, Shell) that decompose
complex system tasks into discrete, compensable operations.

**Planner/Executor separation** is the structural backbone. The planner acts
as a graph builder — it performs dry runs, impact analysis, and dependency
resolution before a single side effect occurs. The executor is the state
enforcer — it traverses the graph, evaluates current state against desired
state, and skips resources already converged. This makes the process
idempotent by default.

**Generated abstraction** eliminates the "n+1" problem. By generating
boilerplate from Provider/Resource pairs, the system guarantees that every
resource type satisfies the same scaffolding. Contributors focus on
reconciliation logic without worrying about logging, auditing, or graph
traversal. The interface between user-defined "lore" and the execution engine
is strictly typed and consistent.

**Compensability** across all providers indicates a design focused on
transactional integrity — the ability to roll back state if an orchestration
task fails partway through. The `moveToRecovery` / `restoreFromRecovery`
pattern with platform-specific recovery bases and same-device rename
guarantees shows deliberate thought about failure modes.

### Design

**Starlark sandboxing** is a strong choice for registry logic. Unlike
executing arbitrary shell scripts, Starlark provides a restricted,
deterministic environment that prevents lore from spawning runaway processes
or accessing sensitive files without explicit permission. It is fast to load
and evaluate — critical for CLI responsiveness — and enables a central library
of versioned, shareable actions.

**Standardized configuration** via JSON schemas (`schema` package) provides a
predictable interface for users and developers, enforcing validation before
system-level changes are attempted.

**Registry-based knowledge** (`indexgen`, knowledge domains) positions devlore
as a framework for sharing codified tribal knowledge, not just a static set of
shell scripts.

**Implementation** follows standard Go best practices: clean `pkg/`/`cmd/`/
`schema/` structure, dependency injection via context and platform drivers for
testability and extensibility, and thoughtful details like `go-git`'s
`.gitignore` awareness in the `starcode` package.

### Summary

| Perspective | Rating | Notes |
| --- | --- | --- |
| **Architecture** | **High** | The operation graph / compensation model addresses the "partial failure" problem common in automation. Aligns with Terraform and Kubernetes patterns. |
| **Design** | **Strong** | Schema-validated config, clear separation of concerns, Starlark sandboxing, registry-as-framework. |
| **Implementation** | **Solid** | Idiomatic Go, good use of the type system for provider interfaces, exceptional test ratio. |

This system is technically superior to a traditional shell-scripting automation
suite. The architectural pivot to a reconciliation engine — where the planner
constructs an immutable graph and the executor enforces it — aligns devlore
with modern platform engineering patterns.

### Risks and Hardening

**Complexity vs. value.** The Provider layer must not become so complex that it
is harder to use than simple shell scripts. Preventing abstraction rot is the
primary adoption challenge.

**Journal atomicity.** The journal must describe exactly where the executor
failed and what the compensation path is. A checkpoint system that saves state
after every successful resource transition would strengthen reliability.

**Cycle detection.** As the registry grows and lore becomes more complex,
circular dependencies become likely. The planner needs robust static analysis
that validates the graph for cycles before execution begins.

**Orphaned resource cleanup.** A true reconciliation system (like Kubernetes)
includes garbage collection — removing resources no longer present in the
config. The current design should account for detecting and removing
unreferenced resources.

**Drift detection.** A natural extension of the reconciliation architecture:
compare actual system state against desired state without applying changes
(`devlore check`).

**State management.** While `Compensate` methods exist, ensuring these
operations are truly atomic and side-effect-free in edge cases is the true
test of the implementation.

---

## Repositories

- **noblefactor** — Product design documentation (ADRs, strategy, PRDs, architecture)
- **devlore-cli** — Go implementation (source, tests, codegen, CLI)

## Combined Weekly Effort

| Week | Dates | noblefactor (design docs) | devlore-cli (source) | devlore-cli (tests) | Total LOC delta | Commits |
|---|---|---|---|---|---|---|
| W02 | Jan 6–12 | 13,269 (new) | — | — | +13,269 | 5 |
| W03 | Jan 13–19 | +16,468 → 29,737 | 7,660 (new) | 403 (new) | +24,531 | 24 |
| W04 | Jan 20–26 | +5,297 → 35,034 | +13,332 → 20,992 | +5,286 → 5,689 | +23,915 | 70 |
| W05 | Jan 27–Feb 2 | +374 → 35,408 | +10,315 → 31,307 | +3,533 → 9,222 | +14,222 | 42 |
| W06 | Feb 3–9 | — | +48 → 31,355 | +1,480 → 10,702 | +1,528 | 13 |
| W07 | Feb 10–16 | −1,349 → 34,059 | +3,128 → 34,483 | +3,955 → 14,657 | +5,734 | 39 |
| W08 | Feb 17–23 | — | −3,387 → 31,096 | +5,942 → 20,599 | +2,555 | 36 |
| W09 | Feb 24–Mar 2 | — | +2,333 → 33,429 | +9,125 → 29,724 | +11,458 | 9 |
| W10 | Mar 3–5 | — | +1,104 → 34,533 | +3,067 → 32,791 | +4,171 | 7 |

## Summary

| Metric | noblefactor | devlore-cli | Combined |
|---|---|---|---|
| Period | Jan 11 – Feb 12 | Jan 14 – Mar 5 | Jan 11 – Mar 5 |
| Calendar days | 33 | 50 | 54 |
| Active days | 17 | 30 | ~35 |
| Total commits | 87 | 158 | 245 |
| Total PRs | 61 | 181 | 242 |
| Final LOC | 34,059 (docs) | 34,533 (src) + 32,791 (test) | 101,383 |
| Author | 1 | 1 | 1 |

## Phase Narrative

### W02–W03 (Jan 6–19): Product Design

Product design from scratch — 30K words of ADRs, strategy, PRDs,
architecture docs in `noblefactor`. First 7,600 LOC of Go code scaffolded
in `devlore-cli`.

### W04 (Jan 20–26): Peak Velocity

Peak velocity — 70 commits. Design docs finalized. Go codebase exploded
from 7,600 to 21,000 LOC source. Providers, Starlark integration, codegen
pipeline built.

### W05 (Jan 27–Feb 2): Core Build Sprint

Core build sprint — 31,300 LOC source. Execution engine, model providers,
signing, CLI infrastructure.

### W06–W07 (Feb 3–16): Refinement

Design docs consolidated to GitHub issues, source grew modestly, tests
ramped up (10K → 15K).

### W08 (Feb 17–23): Major Refactoring

Source LOC dropped by 3,400 (cleanup/coalescing) while tests grew by 6K.
Net quality improvement.

### W09–W10 (Feb 24–Mar 5): Resource Management

New resource management architecture layered in. Test LOC nearly doubled
(20K → 33K). 58 generated test files added.

## Tooling

All development performed with Claude Code (AI-assisted). Single human
author reviewing, steering, and approving all changes.
