---
title: "Lore command rewrites — ground-up against the sealed API"
parent: "docs/plans/extract-starlark-from-op/phase-8/21-graph-immutability.md"
status: in-progress
created: 2026-05-30
updated: 2026-05-30
---

## Problem statement

The graph-immutability seal (step 21) and its framework prereq (construction-time annotations, key-agnostic
receipt, `Assemble` `slots`/`Tool`, `plan.origin`) are in. lore is the first tool to migrate onto the sealed
API. Rather than mechanically port the old `cmd/lore/lore/builder.go`, we **rewrite each lore command from the
ground up, one at a time**, with a dual purpose:

1. **Refine / prove the sealed API.** Each command is a real consumer; building it surfaces gaps, awkward
   ergonomics, and missing pieces across the whole stack — the Go construction side (`NewGraph` / `Assemble` /
   `plan.Provider`) **and** the starlark side (the `plan.*` surface the bridge exposes).
2. **Improve the implementation of each command.** The old implementations are pre-seal and, for most
   commands, stubs. Ground-up gives each a correct, current implementation.

**Goal: a usable API for current *and* future work** — proven by a real package (docker) that builds, saves,
loads, and runs across operating systems.

This is expected to be a **lengthy, high-value journey.** Driving each command against a real package will
surface and fix many framework and bridge bugs along the way — that discovery is part of the point, not a
detour.

> **Construction model → architecture.** The durable design this work realizes — the pipeline model,
> phase-script→subgraph harvest, Origin/provenance, and the build/save/load/run portability promise — lives in
> [`docs/architecture/2.5-lifecycle-pipeline-construction.md`](../../../architecture/2.5-lifecycle-pipeline-construction.md).
> This plan tracks the **command-by-command work** that proves and refines that model; it does not restate it.

## Goals (the proving bar)

For the first command (`lore deploy`) against the docker package:

1. **Build** the execution graph for **each target OS** from the package's per-OS phase scripts.
2. **Save** each graph (wire form) and **load** it back faithfully — Origin (`Tool`/`Scope`/`Annotations`),
   unit annotations, slots, edges all round-trip.
3. **Run** each loaded graph on its **targeted OS**.
4. **Cross-build**: build *every* OS's graph **on macOS** (graph construction is host-independent), then run
   each on its target. This is the portability payoff of the immutable, serializable graph.

**Sequencing — Linux first.** The immediate target is **Ubuntu** (`Linux.Debian`): built on macOS, run on
Ubuntu. **Darwin (macOS) is deferred** — its delta from Linux is substantial (Docker Desktop DMG vs. apt
packages) and there is no macOS install procedure today; we cycle back to macOS once Linux is solid.
`Linux.Fedora` / `Windows` stay in the package but out of immediate scope.

## Command matrix (full)

Grouped by the API surface each exercises — which drives sequencing and dependencies.

| # | Command | Status | Group | API surface exercised |
|---|---|---|---|---|
| 1 | `deploy` | ⚠️ pre-seal (`builder.go`) | A — graph construction | `plan.*` script harvest, native-PM invocations, `NewSubgraph` nesting, `NewGraph` + `Origin`, save/load/run |
| 2 | `upgrade` | ❌ stub | A — graph construction | same as deploy, `Upgrade` pipeline |
| 3 | `decommission` | ❌ stub | A — graph construction | same, `Decommission` pipeline |
| 4 | `reconcile` | ❌ stub | A/B — construction + read-back | build repair graph from drift vs. `StateView` |
| 5 | `bundle` | ❌ stub | A — construction | assemble a manifest into a portable bundle |
| 6 | `list` | ❌ stub | B — read-back | trace-derived `StateView` over receipts |
| 7 | `inspect <package>` | ❌ stub | B — read-back | package detail (registry/manifest) + deploy state |
| 8 | `search` | ✅ implemented | C — registry | registry query (no graph) |
| 9 | `onboard` | ✅ implemented | C — registry | registry onboarding (no graph) |
| 10 | `resolve` | ❌ stub | C — registry | dependency resolution |
| 11 | `update` | ❌ stub | C — registry | registry refresh |
| 12 | `publish` | ❌ stub | C — registry | publish a manifest |
| 13 | `audit` | ❌ stub | C — registry | audit installed vs. available |
| 14 | `manifest create/validate/test/show/update` | ❌ all stub | C — manifest | manifest authoring/validation |

**Sequence.** A first (graph-construction; proves the API and yields reusable phase-subgraph/harvest helpers),
starting with **`deploy`**. Then `upgrade`/`decommission`/`reconcile`/`bundle` reuse deploy's building blocks.
**B** (read-back) once steps 4–5 land the trace-derived `StateView`. **C** last (least API-exercising; two
already work).

## Per-command loop

For each command: **(1)** write an intent spec (what it does, what graph/state it builds or reads, the API
surface it should use — the CLI help + lore guides are the seed); **(2)** rewrite ground-up against the sealed
API; **(3)** record any API gap or refinement it surfaces (the prove/refine payoff, fed back here and into the
framework). Go style-guide compliance is non-negotiable per file touched.

## First target: `lore deploy` with the docker package

### The package — `../devlore-registry/packages/docker/`

- **`lifecycle.yaml`** — manifest: name/version, `platforms: [Darwin, Linux, Windows]`, `features` (`rootless`,
  `purge-data`), `settings` (`storage-driver`, `log-driver`), `verification` (`docker --version`),
  `hardware_provisions`, `conflicts`, `provides`.
- **Per-OS pipelines** under `Darwin/`, `Linux.Debian/`, `Linux.Fedora/`, `Windows/`, each with:
  - `Deploy/` — `prepare`, `install`, `provision`, `verify` (phase scripts, in `PhaseOrder`).
  - `Upgrade/` — `prepare`, `install`, `verify`.
  - `Decommission/` — `uninstall`, `unprovision`, `cleanup`.
- **Each phase script** is a `def <phase>(package, phase):` that calls `plan.*` — i.e. **each phase script
  defines a discrete subgraph** in the pipeline (the front-3 model).

### ⚠️ The phase scripts are STALE — rewriting them is in scope

The current `.star` scripts reference an **old / aspirational `plan.*` surface** (and carry TODOs for API that
never existed). They are **not** valid input as-is. Rewriting them against the **current sealed `plan.*`
surface** is a first-class part of this work, because it exercises and proves the **starlark→bridge→
plan-provider→graph** path end-to-end — the other half of "prove the API." Where a script *wants* API that
doesn't exist yet, that becomes an explicit decision (implement now vs. defer with a documented workaround).

**Phase-name reconciliation (non-Deploy pipelines).** Canonical phase orders live in
`internal/lorepackage/lifecycle.go` (`PhaseOrder`); see
[`docs/architecture/2.5`](../../../architecture/2.5-lifecycle-pipeline-construction.md). docker's **Deploy** dir
matches the code (`prepare, install, provision, verify`), but the stale `Upgrade/` dir (`prepare, install,
verify`) and `Decommission/` dir disagree (code: Upgrade `prepare, upgrade, migrate, verify`; Decommission
`unprovision, uninstall, cleanup`). Reconcile the package to the canonical orders when those pipelines' scripts
are rewritten — **Deploy (the first target) is unaffected.**

### Linux.Debian authoritative source — `Install-Docker`

The working Ubuntu/Debian procedure lives at
`~/Workspace/Personal/Home/Configs/noblefactor-Unix/.local/bin/Install-Docker`. The rewritten `Linux.Debian`
Deploy scripts are derived **from this procedure** against the current `plan.*` (the stale `.star` files are
replaced, not ported):

| Phase | Procedure (from `Install-Docker`) | `plan.*` surface |
|---|---|---|
| `prepare` | remove conflicts (`docker.io docker-doc docker-compose docker-compose-v2 podman-docker containerd runc`); `apt autoremove`; `apt update` | `plan.package.remove(...)` (variadic); index update |
| `install` | `docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin lshw` | `plan.package.install(...)` (variadic) |
| `provision` | detect product (`lshw -json \| jq .product`); ODROID-C4/C5 → append `systemd.unified_cgroup_hierarchy=0` to `boot.ini` bootargs; Pi 5 → no-op | `plan.shell.*`; runtime branch (`plan.choose`) on detected product; file-line edit |
| `verify` | `docker --version` (manifest `verification`) + `docker run hello-world` | `plan.shell.*` |

Gaps this surfaces (decide implement-now vs. defer): **`plan.package.remove`**, **runtime product/hardware
detection** feeding a `plan.choose`, and a **file-line-append/edit** op for `boot.ini`.

### `lore deploy` therefore has three parts

1. **Rewrite the docker phase scripts** against the current `plan.*` (shell / package / file provider methods as
   they actually exist) — proves the starlark surface; surfaces gaps.
2. **Rewrite the `lore deploy` Go command** against the sealed API — per-phase plan-provider harvest →
   phase subgraphs → package subgraph → `NewGraph(Origin{Tool, Scope, Annotations})`; then save / load / run.
3. **Prove the build/run matrix** (below).

### `run` argument binding — command-side work

Realizing the `run(package, phase, …)` contract (model in
[`docs/architecture/2.5`](../../../architecture/2.5-lifecycle-pipeline-construction.md)) adds two command-side
tasks to Part 2:

1. **Enforce the reserved names.** Reject — with a clear error — any phase whose `run` declares an additional
   argument named `package` or `phase`; those are framework-controlled. No such enforcement exists today.
2. **Wire extra `run` args to the binding layer.** Declare each extra arg as a root variable resolved by name
   via `VariableResolver` — **CLI flags** (Flags source) and **env vars** (`envValue` → `json.Unmarshal`) — and
   resolve each phase's `run` args by name against **prior-phase outputs first**, then the root variables.
   Untyped: pass what's found.

**Open (settle while building harvest/assemble):** how a phase **exposes its named outputs** into the
inter-phase namespace so the next phase's `run` argument can match them — i.e. what a phase subgraph "returns,"
keyed by name.

### Phase `plan` surface — command-side work

A phase `run` gets a **restricted `plan`**: the lifecycle/meta operations (`plan.assemble`, `plan.run`,
`plan.save`, `plan.load`, `plan.origin`) are unavailable to scripts; lore drives those from Go. Mechanism and
rationale (the Go-path/starlark-path split, the per-runtime bridge deny set):
[`2.5` → "Phase authoring surface — and how it is enforced"](../../../architecture/2.5-lifecycle-pipeline-construction.md#phase-authoring-surface--and-how-it-is-enforced).
Two tasks:

1. **Bridge.** Add `DenyAttributes(global, names…)` + a `filteredReceiver` to `starlarkbridge.NewRuntime`
   (generic, tool-agnostic), with tests: a denied name errors on `Attr` and drops from `AttrNames`; every other
   name delegates unchanged.
2. **lore.** Build each phase runtime with
   `DenyAttributes("plan", "assemble", "run", "save", "load", "origin")` (wired with the `lore deploy`
   construction in Part 2).

### API surface the (rewritten) scripts will use — and gaps to decide

Observed in the stale scripts (subject to rewrite against the real surface):
- `plan.shell.*` — run shell commands (Darwin DMG mount/detach).
- `plan.package.install(name, ...)` — variadic package install (Debian).
- `package.has_feature("rootless")` — feature-gated conditional invocations.
- `phase` context (phase name, retry).

Gaps the scripts *want* (the "future work" surface — decide implement-now vs. defer):
- `platform.arch` (arm64 vs amd64 selection)
- `plan.download(url, dest)` (fetch remote artifact)
- `phase.env(name)` (read env var at run time)
- `plan.file.remove(path)` (delete a file)

### The build / run matrix (the proving bar, concretely)

All graphs are **built on macOS**; the "Run" column is the **target** OS.

| Priority | Target OS | Package variant | Source of truth | Build | Save | Load | Run |
|---|---|---|---|---|---|---|---|
| **1 — now** | Ubuntu (`Linux.Debian`) | `docker/Linux.Debian/Deploy` | `Install-Docker` (working script) | ☐ | ☐ | ☐ | ☐ |
| later | macOS (Darwin) | `docker/Darwin/Deploy` | docs.docker.com — Desktop for Mac (instructions → package definition) | ☐ | ☐ | ☐ | ☐ |
| later | Fedora (`Linux.Fedora`) | `docker/Linux.Fedora/Deploy` | docs.docker.com — Engine on Fedora | ☐ | ☐ | ☐ | ☐ |
| later | Windows | `docker/Windows/Deploy` | docs.docker.com — Desktop for Windows | ☐ | ☐ | ☐ | ☐ |

**Sources of truth per OS:** Linux/Ubuntu = the working `Install-Docker` script (above). macOS/Windows =
**Docker's official install instructions** (`docs.docker.com`), translated into the lore package definition
when we cycle back — there is no local working script for those.

"Build on macOS, run on target" proves the graph is a **host-independent, serializable plan** — the core
promise of the immutable graph.

## QA baseline — `pkg/**` & `cmd/star/**` (2026-05-30)

Health snapshot taken before the deploy journey begins (the "starting place"). Method: `make vet` /
`golangci-lint` / `make test` / `gocyclo`, scoped to the two trees.

| Metric | `pkg/**` | `cmd/star/**` |
|---|---:|---:|
| Packages | 52 | 28 |
| Production LOC (`.go`, non-gen, non-test) | 37,421 | 12,358 |
| Generated LOC (`*.gen.go`) | 6,932 | 2,699 |
| Test LOC | 20,946 | 8,765 |
| Test functions | 1,097 | 367 |
| Avg cyclomatic complexity | 3.1 | 4.55 |

**Build / vet / lint:** CLEAN across both trees — 0 issues (the only tree-wide lint issue is
`cli.WriteReceipt undefined` in `cmd/writ/writ/migrate`, out of scope / expected-red).

**Tests:** `pkg/**` 100% green (all 52 packages `ok`). `cmd/star/**` 27/28 green; `cmd/star/star` fails 9 tests
(`TestLintCopyright_*`, `TestSourceFile_StarlarkIntegration`) — the **pre-existing Row-4** failures already
tracked in `21-graph-immutability.md`.

**Actionable findings:**
- **F1 — `make complexity` is a false-green gate (fix first).** `gocyclo` isn't on the make shell's `PATH`
  (installed to `$GOPATH/bin`); the check hits `command not found`, prints "All functions below complexity
  threshold," and exits 0 — checking nothing. `make check`/CI never enforces complexity. Fix: call
  `$(go env GOPATH)/bin/gocyclo` (or put it on PATH).
- **F2 — 14 functions over the complexity-20 threshold** (unenforced due to F1). `pkg/**` (7): `op.NewMethod`
  (42), `starlarkbridge.(*goReceiver).dispatch` (37), `starlarkbridge.toGoInto` (27), `git.guessDirName` (27),
  `application.flagValue` (26), `starlarkbridge.(*goReceiver).toStarlarkReflect` (22), `op.(ActionPlanner).Plan`
  (21). `cmd/star/**` (7): `goast.LoadSourceFile` (35), `goast.schemaFromConfigVal` (27),
  `goast.(*Provider).ConstGroups` (25), `goast.analyzeFileMetrics` (24), `goast.checkLineWidth` (23),
  `goast.assignSlots` (22), `goast.typeToString` (22).
- **F3 — `cmd/star/star` Row-4 test failures** (9) — pre-existing, tracked.

**Net:** `pkg/**` is a strong baseline (clean, all tests green, low avg complexity). `cmd/star/**` clean except
the known Row-4 tests. The one new discovery is **F1**, which masks **F2**.

## Decisions & pre-work

**Confirmed (2026-05-31):**
- **Per-unit provenance is an *annotation*, not `Origin`.** `Origin` is **graph-level** (`{Tool, Scope,
  Annotations}`); an `ExecutableUnit` carries `Annotations` only — there is no unit `Origin`. lore records the
  package a unit belongs to as `annotations["package"] = <name>` (the receipt is key-agnostic, so lore owns the
  key; `"package"` is accurate and avoids echoing the graph concept). **Ambient annotations stay deferred** → in
  the first pass, script-produced leaf units won't carry `annotations["package"]`; **native-PM** units will, via
  the explicit `Plan(annotations={"package": pkg.Name})` feed. The provider ambient API (in `provider.go`) closes
  this later.
- **Rewritten docker scripts live in place** in `../devlore-registry/packages/docker/` — a **separate repo**, so
  those `.star` changes commit there, independent of devlore-cli.

**Pre-work before deploy starts:**
- **F3 — Row-4 test failures (`cmd/star/star`): DECIDED — fix the bridge (box 2).** `TestLintCopyright_*` (9
  cases) + `TestSourceFile_StarlarkIntegration`. Investigated 2026-05-31: not stale scripts, but the reflection
  projection surfacing zero-arg getters as callables where the scripts and the documented
  [3.3](../../../architecture/3.3-static-starlark-codegen.md) contract expect eager properties. Resolution is an
  opt-in `+devlore:property` signal (`MethodModifiers` / `ModifierProperty`) honored by the bridge — scripts pass
  unedited. Scoped in [eager-property-projection.md](./eager-property-projection.md). Must be green (phase-8 PR
  gate) before deploy work begins.

**Resolved (2026-05-31):** the durable construction model moved to
`docs/architecture/2.5-lifecycle-pipeline-construction.md`; this file stays in `docs/plans` as the
command-by-command work tracker, cross-linked to 2.5.
