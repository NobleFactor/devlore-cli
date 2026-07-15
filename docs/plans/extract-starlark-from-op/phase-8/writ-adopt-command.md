---
title: "writ adopt — command plan & design (step-33 slice A)"
status: draft — design settled on gather + field projection (2026-07-15 discussion); design COMPLETE (all three questions settled); step 45 executes first, then slice A implements
created: 2026-07-15
parent: steps/33-writ-migrate-rewrite.md
---

# writ adopt — plan & design

## Purpose

Bring existing configuration files under version control. For each adopted item, the file moves from its live
location into the layer tree at `<layer>/<scope>/<project>/<relpath>`, and a symlink is created at the original
location pointing at the moved file. Scope is inferred per item: paths under `$HOME` are `Home/`, everything else is
`System/`. Directories adopt recursively; existing symlinks are skipped.

## CLI surface (kept as-is)

```
writ adopt [--layer personal|team|base] --project <name> [--from-receipt] <item>...
```

- `--layer` (default `personal`), `--project` (required), `--from-receipt` (reads a lore receipt; **stays a
  not-implemented stub**).
- Root flags honored: `--dry-run` (narrate, change nothing), `--verbose`.
- Per-item UX kept: missing item → per-item error; already-a-symlink → warn + skip; adoption summary + commit
  reminder at the end.

## Current state (the ancient pattern — thrown away)

1. **One 3-node graph, two runtime specs, and a full plan/run cycle per file** (`adoptFile` →
   `adopt.BuildGraph` + `buildAdoptSpec` ×2 + `op.Plan` + `GraphExecutor.Run`).
2. **Fake variable plumbing**: `dest_dir` / `source_path` / `dest_path` are known at plan time but travel through
   `Application.Flags` + `plan.variable(...)` references, resolved back out at preflight.
3. **Legacy error mapping** (`adopt/execute.go`: `Run` / `mapAdoptError` / `firstJoinedError`) re-creates
   pre-rewrite error prefixes — dead weight under the greenfield rule.
4. **No SAGA semantics**: each file is its own graph, so a mid-batch failure leaves prior files moved with no
   rollback, and the framework's recovery model goes unused.

## Target design — gather over the inputs, with field projection (settled by discussion, 2026-07-15)

**The inputs are the locations of the files to adopt** (ruled 2026-07-15). Enumeration stays Go-side (expansion,
`Lstat` checks, symlink-skip, recursive walk, scope inference, dry-run narration); at plan time the tool derives each
location's destinations and builds one **item record** per file:

```python
items = [{"source": ..., "dest_dir": ..., "dest_path": ...}, ...]
```

The graph is a **`plan.gather` over those records**. (This supersedes this doc's first draft — N baked immediate
chains — after the design discussion: "cannot feed a gather" was wrong as a design statement; it held only for bare
items + a multi-slot body under the current no-projection constraint, which pre-slice A0 lifts.) One graph per scope
group (Home vs System confined roots); per group, the plan surface reads:

```python
adopt = plan.gather(
    items = items,          # the inputs, derived into records at plan time
    limit = 4,
    body  = [
        plan.choose(
            plan.case(
                # In-graph destination guard: per item, at dispatch time, policy-governed and journaled.
                when = plan.file.exists(resource = plan.item("dest_path")),
                then = plan.failed("destination already exists: {{ .dest }}",
                                   dest = plan.item("dest_path")),
            ),
            default = [
                plan.file.move(source = plan.item("source"),
                               destination_path = plan.item("dest_path")),
                plan.file.link(source = plan.item("dest_path"),
                               target_path = plan.item("source")),
            ],
        ),
    ],
)
```

**Graph shape per scope group** (writ builds this via `plan.Provider`, the Go mirror of the surface above):

```
graph  origin: writ adopt · <project>          one confined root per scope group
├── mkdir₁ … mkdir_k    file.mkdir per UNIQUE dest_dir (deduped, immediate-bound, ahead of the gather)
└── gather  items=<records>  limit=N
    └── body (per item):  choose( exists(dest_path) → failed | default: move → link )
```

Design points:

1. **mkdirs are a deduped pre-gather stage.** Gather iterations run concurrently; per-item mkdir of a *shared*
   `dest_dir` inside the body would be same-resource production — a plan conflict under the gather contract (a
   correction from the discussion's first worked example, which drew mkdir inside the body). Deduped
   immediate-bound `file.mkdir` nodes run before the gather; LIFO unwind still orders compensation correctly
   (iteration substacks unwind before the mkdirs, which prune to their recorded creation boundaries).
2. **The destination guard is in-graph** — the `choose` above: per item, evaluated at dispatch (not against a
   pre-run snapshot of the world), routed structurally (Choose receives, the graph selects), failing through
   `plan.failed` → `execution_failed` → the policy floor. Supersedes the first draft's Go-side pre-run check.
3. **Failure policy (ruled): the policies as defined govern.** A failed adoption fails the run: `execution_failed`
   → stop at the saga boundary → LIFO unwind — completed iterations compensate (links removed, moves reversed),
   the pre-stage mkdirs prune — `stopped × execution_failed`, trace persisted. The old per-file continue-on-error
   loop is dead; operators who want different reactions configure the policy layer.
4. **What gather buys over baked chains**: bounded concurrency (`limit=`), per-iteration frames, per-iteration
   stamped substacks — pause/resume skips completed adoptions item-by-item — and failure handling at the same
   boundary shape as everywhere else.
5. **Execution**: one `op.NewGraphExecutor(graph, spec).Run(ctx, nil)` per scope group; no `Application.Flags`
   plumbing (items are immediates; in-body references are projections). The trace persists via `cli.WriteTrace`,
   success or failure (step-21 R4: a failed run's journal survives).
6. **Errors surface as the framework reports them** — no legacy prefix mapping.

## The framework extension: field projection (`plan.item`) — chartered as [step 45](steps/45-field-projection.md)

A gather body exposes the whole `item` with no in-plan derivation (the A4 fixture pins this), so record fields need a
projection surface. Small and additive (~100 lines + codegen + tests):

1. **`op.Variable` gains `Field`** (`variable.go:83`) — the plan-time reference shape.
2. **`plan.Provider.Item(field)`** → `&op.Variable{Name: "item", Field: field}`; announced → the adapter exposes
   `plan.item("source")`. (The general `plan.variable(name, field=...)` form shares all machinery; `plan.item` is
   the gather-body sugar.)
3. **`VariableBinding` grows a `field` member** (`binding.go:145`) — the sealed three-variant set is unchanged;
   `Resolve` looks up the frame value then projects the field (the record arrives as the converted natural form,
   `map[string]any`).
4. **The three stamp sites carry `Field` through** — `planner.go:294`, `plan/helpers.go:217`, and the document-load
   path `node.go:294`; the slot's document form gains `field` beside the variable name.
5. **Plan-time validation in `GatherPlanner.Plan`**: with immediate items, every `item`-projection field must exist
   in every record (a plan error, not a nil at dispatch); the step-16 type check reads the projected field's value
   type; the gather uniqueness contract is restated over the projected values feeding file ops; `plan.item`
   outside a gather body is a friendly plan error (the A2 frame-stripping rule makes it unresolvable anyway).
6. **Scope for free**: projections resolve wherever the frame binds the variable — nested subgraphs, choose
   when-predicates and branches, wait_until bodies (frame inheritance, pinned by A1). No per-combinator work.

## Deletions

1. `cmd/writ/writ/adopt/execute.go` — `Run`, `mapAdoptError`, `firstJoinedError`.
2. The per-file `BuildGraph` shape and the dual-spec `buildAdoptSpec` flag plumbing in `adopt_cmd.go`.
3. The `plan.variable` indirection for plan-time-known values.

## Test plan

**A0 (framework):** `VariableBinding` projection resolve + document round-trip; `GatherPlanner` validation (missing
field = plan error; `plan.item` outside gather = plan error); a `.star` fixture — gather over record items with a
multi-invocation body of projections (deliberately dissolving the A4 single-value limitation and restating the
uniqueness contract in field terms); a choose-inside-gather fixture (projection through inherited frames).

**A (adopt):**

1. Batch adopt of N files (mixed nesting, shared parent dirs) → deduped mkdir pre-stage + gather; files moved +
   linked; trace persisted.
2. Existing destination → the in-graph guard fires `plan.failed`; the run stops per policy; completed iterations
   compensated; failed trace persisted; non-zero exit.
3. Scope grouping: a `$HOME` item and a `/etc` item land in separate graphs with correct roots (the System run
   skipped/mocked where privileges are unavailable).
4. Pause/resume mid-batch: completed iterations replay from their stamped substacks; remaining items adopt on
   resume.
5. Symlink-skip, missing-item, dry-run narration, `--from-receipt` stub — behavior pinned.
6. Reshape `adopt_integration_test.go` onto the new shape.

## Settled

1. **Batch failure policy (2026-07-15): the policies as defined govern** — no adopt-specific mode, no flag; a
   failed adoption fails the run and unwinds per the `TransitionPolicy` floor.
2. **The inputs are the locations of the files to adopt (2026-07-15)** — derived into item records at plan time;
   fed to the graph as the gather's `items=`.
3. **Gather-shaped, not baked chains (2026-07-15 discussion)** — field projection (A0) is the enabler, chosen over
   the alternative (a single compensable `adopt_file` action hiding mkdir/move/link inside one dispatch) for plan
   transparency: visible, individually receipted, individually compensated nodes per item.
4. **The destination guard is in-graph** (the `choose` + `plan.failed` shape above) — per-item, dispatch-time,
   policy-governed, journaled.

## Formerly open questions (all settled)

1. ~~**Sequencing of A0**~~ — **settled 2026-07-15: chartered as [step 45](steps/45-field-projection.md), executed
   first; slice A is a pure consumer.**
2. ~~**Projection surface**~~ — **settled 2026-07-15: both; `plan.variable(name, field=...)` is the primitive,
   `plan.item(field)` is sugar over it** (recorded in [step 45](steps/45-field-projection.md)).
3. ~~**Per-file progress output**~~ — **settled 2026-07-15: post-run reporting.** After the run, the per-file
   `Adopted ...` lines derive from the trace/receipts; on failure, the report names what completed and what rolled
   back. Hooks can add live progress later without design change.
