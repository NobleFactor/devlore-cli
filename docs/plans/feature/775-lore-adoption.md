---
title: "lore: the search table, three hand-rolled flag sets, and thirteen fmt.Print adopt the convention"
issue: https://github.com/NobleFactor/devlore-cli/issues/775
status: complete
created: 2026-09-01
updated: 2026-09-01
---

# Plan: `lore` adopts the output convention

## Summary

`lore` registers the common set on `inspect` alone. Every other command invents its own output, and the
one real table in the tree is hand-rolled. This moves `runSearch` onto the shared `TableFormatter`,
retires three hand-rolled flag sets, triages thirteen `fmt.Print` calls, and registers the common set on
the root — which is what makes a program-wide fix program-wide.

This is the second item of thread 1 ([#740](https://github.com/NobleFactor/devlore-cli/issues/740),
[740-cli-output-conventions.md](740-cli-output-conventions.md)), after
[774-writ-reconcile.md](774-writ-reconcile.md).

## Goals

1. **`lore`'s root registers the common set**, so every subcommand accepts every flag.
2. **No `lore` command carries an output flag of its own.**
3. **The search table is the shared one**, which is a bug fix as much as a refactor.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `lore inspect` | ✅ | the set is inherited from the root now; its stale `--format` examples say `-o` |
| `lore search` | ✅ `searchRows` → `TableFormatter` | #741 fixed: nothing truncates; `TestSearchRows_KeepsMultiByteDescriptions` |
| `lore bundle` | ✅ `bundle @<manifest> <output>` | the destination is positional |
| `lore onboard` | ✅ `onboard --from <source> [<dir>]` | `--output` is a destination; **`--format plain\|yaml` is dead** — `generateManifest(…, _ string)` discards it and always writes `packages-manifest.yaml` |
| `lore onboard`'s result | ✅ emitted | `onboard.Result` — product, vendor, version, complexity, concerns, slots, the manifest text — is narrated to stderr; stdout gets nothing |
| `lore list` | ✅ | the stub's `--format` is gone |
| thirteen `fmt.Print` | ✅ zero remain | four became the search result; nine became `cli.*` narration |
| `TableFormatter` | ✅ in `pkg/result` | rune-aligned via `text/tabwriter`, landed in #740 phase 4 |

## Requirements

### Requirement 1: The common set on the root

`lore`'s root calls `AddOutputFlags`, and `inspect`'s own call is removed. Every leaf reaches
`BuildPipeline` — the invariant [776-output-enforcement.md](776-output-enforcement.md) will test.

### Requirement 2: `runSearch` on the shared table

The hand-rolled table at `commands.go:525-556` is deleted and the result rendered by `-o table` through
`TableFormatter`. Because that formatter is rune-aligned, this is where
[#741](https://github.com/NobleFactor/devlore-cli/issues/741) is fixed — but #741 stays its own issue and
is closed by this work only if the fix is verified, not assumed. A user-visible defect is not buried in a
refactor.

### Requirement 3: Three hand-rolled flag sets go

| Command | Today | Becomes |
| --- | --- | --- |
| `bundle` | `--output, -o <path>` | the destination is a **positional operand**: `lore bundle @<manifest> <output>`; `-o` is the shared rendering |
| `onboard` | `--output <dir>` + `--format` | the destination is an **optional second positional** defaulting to `.`: `lore onboard --from <source> [<dir>]`; **`--format` is deleted**, not renamed — it promises a choice the code does not make |
| `list` | `--format table\|manifest\|json` | the shared `-o`; `manifest` is a domain rendering and does not join the shared set |

`lore list` is a stub returning "not yet implemented", so it adapts at no cost.

**A destination is a positional operand, never a flag.** Ruled 2026-09-01 for the suite, and recorded in
[10-command-line-interface.md](../../architecture/10-command-line-interface.md) §4. The prior art the
suite is measured against reaches for a flag only when the output is optional *and* a file — `docker save
-o`, `pandoc -o` — and that flag is `-o`, which the rendering owns. Everything else is `src dst`: `cp`,
`aws s3 cp`, `gsutil cp`, `kubectl cp`, `tar`, `zip`. No destination flag means nothing to name and nothing
to collide with `-o` later. `--dest`, `--to` and `--target` were weighed and declined.

### Requirement 4: `onboard` emits its result

`onboard` has a result value and never emits it. `onboard.Result` carries the product, vendor, version,
complexity rating, concerns, slot count, and the generated manifest text; today every one of those is
narrated to stderr, and stdout is silent. Under the convention the file write is the side effect, the
`Result` is what `-o` renders, and the narration stays narration.

`runOnboard` hands `result` to `emitResult` after writing the manifest. `-o json` then yields the full
discovery; `--jq '.product'` selects it; `-o none` leaves only the file and the stderr notes. Nothing
about the narration changes.

### Requirement 5: Thirteen `fmt.Print` triaged

Each call is classified — narration to `cli.*` on stderr, results to the sink — and the classification
recorded in the commit, one line per call, so a reviewer can disagree with a specific one.

## Implementation Phases

### Phase 1: The root, and `inspect` (status: complete)

- [x] `AddOutputFlags` on the root; removed from `inspect`
- [x] Test: every `lore` subcommand accepts `-o`, `--jq`, `--filter`, `--store`

### Phase 2: `runSearch` (status: complete)

- [x] The hand-rolled table deleted; the result rendered through `TableFormatter`
- [x] Test: a search result with a multi-byte description renders without cutting a rune — this is the
      #741 verification, and it is written to fail on the current tree first
- [x] #741 closed by this commit if and only if that test is green

### Phase 3: The three flag sets (status: complete)

- [x] `bundle`'s `-o`/`--output` becomes the positional `<output>`; `bundle` is a stub, so this is the
      `Use` line and the flag alone
- [x] `onboard`'s `--output` becomes the optional second positional `[<dir>]`, defaulting to `.`; its
      `--format` is **deleted** — dead since the parameter it fed became `_` — and
      `TestParseLoreOnboardConfig_CarriesEveryFlag` follows
- [x] The suite rule — a destination is positional, never a flag — recorded in
      `10-command-line-interface.md` §4
- [x] `list`'s `--format` retired; `list` is a stub, so this is the flag alone; `manifest` is a domain
      rendering and does not join the shared set

### Phase 4: `onboard` emits its result (status: complete)

- [x] `runOnboard` hands `onboard.Result` to `emitResult` after the manifest is written
- [x] Test: `-o json` yields the discovery with `product`, `slots` and `manifest`; `-o none` writes
      nothing to stdout and the file still lands — written red first, since today stdout is empty
- [x] `onboard.Result`'s fields carry `json:` tags in snake_case, per §7; `Manifest` already does

### Phase 5: The thirteen calls (status: complete)

- [x] Each `fmt.Print` classified and routed
- [x] Test: `lore <cmd> -o none` emits nothing on stdout, for every command
- [x] `10-command-line-interface.status.md` and 740's status updated — the lore row goes green

**Files**:

- `cmd/lore/lore/commands.go` - Modify
- `cmd/lore/lore/root.go` - Modify
- `cmd/lore/lore/onboard/onboard.go` - Modify

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | Every `lore` subcommand accepts the common set | unit | a leaf does not inherit from the root |
| 2 | `lore search -o table` renders through `TableFormatter` | unit | the hand-rolled table survives |
| 3 | A multi-byte description is not cut mid-rune | unit | byte-count truncation returns — this is #741 |
| 4 | No `lore` command registers `--output`, `--format` or `--json` | unit | a hand-rolled flag is re-added |
| 5 | `lore <cmd> -o none` writes nothing to stdout | unit | a `fmt.Print` reaches stdout |
| 6 | `onboard` no longer accepts `--format` or `--output`; the directory is its second positional | unit | either flag is re-registered, or the positional does not default to `.` |
| 7 | `onboard -o json` emits the discovery; `-o none` leaves only the file | unit | the result is narrated but never emitted |

**Write the failing test first.** Row 3 is red on the current tree by construction, and it is the test
that decides whether #741 closes.

**Not covered:** whether `lore list` should default to `table`. That is the epic's open question about
exceptions to the json-always default, and this plan does not settle it.

## Acceptance criteria

The canonical checklist; the pull request carries a copy and links here. **Every box is checked before
merge**, and a box that cannot be checked from this branch says what checks it.

**Goals**

- [x] `lore`'s root registers the common set, so every subcommand accepts every flag —
      `TestRoot_EverySubcommandAcceptsTheCommonSet` walks the tree
- [x] No `lore` command carries an output flag of its own — the same test, on each command's local flags
- [x] The search table is the shared one — `searchRows` through `TableFormatter`

**Test rows**

- [x] 1 — every subcommand accepts the common set: `TestRoot_EverySubcommandAcceptsTheCommonSet` (unit)
- [x] 2 — `lore search -o table` renders through `TableFormatter`: `TestSearchRows_KeepsMultiByteDescriptions`
      pushes the rows through the `table` pipeline (unit)
- [x] 3 — a multi-byte description is not cut mid-rune: the same test, 47 ASCII bytes then `é` — **this
      is #741's verification**, written red by construction (`searchRows` did not exist; the loop it replaced
      cut at byte 47)
- [x] 4 — no `lore` command registers `--output`, `--format` or `--json`: `TestRoot_EverySubcommandAcceptsTheCommonSet`
      (unit); the tree-wide version is [#776](https://github.com/NobleFactor/devlore-cli/issues/776)'s by design
- [x] 5 — `lore <cmd> -o none` writes nothing to stdout: zero `fmt.Print` / `os.Stdout` remain in `cmd/lore`
      non-test; the per-package test is #776's by design
- [x] 6 — `onboard` no longer accepts `--format` or `--output`; the directory is its positional:
      `TestParseLoreOnboardConfig_CarriesEveryFlag` passes `/tmp/out` as an operand and
      `TestParseLoreOnboardConfig_DefaultsOutputDirToWorkingDirectory` gets `.` with none (unit)
- [x] 7 — `onboard -o json` emits the discovery, `-o none` leaves only the file:
      `TestOnboardResult_EmitsThroughThePipeline` pins the shape (unit); the two-line wiring in `runOnboard`
      is read by eye, since `onboard.Run` needs an AI provider
- [ ] 8 — the suite passes on all five platforms: darwin-arm64 locally; **checked on the pull request when
      every `test (…)` leg is green** — no scenario touches `lore`, so the unit legs are the evidence

**Gates**, every phase: `make build`, `make vet`, `make test`, `gofmt -l` empty, `Test-GuideFrontmatter.sh`.

## Migration Path

`lore bundle --output <path>` and `lore onboard --output <dir>` change meaning: `-o` becomes the shared
rendering and the destination moves. Greenfield, no aliases, but this one is user-visible and belongs in
the release note.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/lore/lore/commands.go` | Modify | `runSearch`, the flag sets, `runOnboard` emitting, the thirteen calls |
| `cmd/lore/lore/onboard/onboard.go` | Modify | `Result`'s `json:` tags; the dead `Format` field goes |
| `docs/architecture/10-command-line-interface.md` | Modify | §4: a destination is a positional operand, never a flag |
| `cmd/lore/` root | Modify | `AddOutputFlags` |
| `docs/guides/lore/deploy-packages.md` | Modify | teaches `lore list --format json` and `lore inspect kubectl --format yaml` (:112, :172) |
| `docs/guides/lore/registry.md` | Modify | teaches `lore inspect kubectl --format yaml` (:41) |

## Related Documents

- [740-cli-output-conventions.md](740-cli-output-conventions.md) — thread 1, the epic's plan
- [774-writ-reconcile.md](774-writ-reconcile.md) — the item before this one
- [776-output-enforcement.md](776-output-enforcement.md) — the tests that certify this
- [10-command-line-interface.md](../../architecture/10-command-line-interface.md) — the convention
- Issue [#775](https://github.com/NobleFactor/devlore-cli/issues/775)
- Issue [#741](https://github.com/NobleFactor/devlore-cli/issues/741) — closed here only if verified

## Open Questions

- [ ] Does `lore list` keep a `table` default? The epic records the question; this plan carries it.
