---
step: 18
title: "Resolve all test failures (phase-8 exit gate)"
former_step: 21
former_title: "Test triage — pre-existing failures"
status: complete — exit gate MET 2026-07-16; `make test` reports ZERO failures repository-wide. The reduction held: step 28 landed 2026-07-15; step 33 (writ migrate/adopt, slices A+B+D) landed 2026-07-15; the deploy-family crater (step 33's former slice C, chartered as step 47) landed slices 1–4 2026-07-15/16, greening cmd/writ, cmd/writ/writ, and docgen; the final sweep is green
proof_run: 2026-07-16 (make test — zero FAIL lines; the writ family, docgen, and e2e all green)
parent: ../../phase-8.md
---

# Step 18 — Resolve all test failures (phase-8 exit gate)

**Status:** `in-progress`. The exit gate is **not met**. The framework half of the graph-immutability /
RuntimeEnvironment seal is committed and green (`pkg/op` and every provider pass); the consumer / test / template
migration (the `writ` and `lore` apps, plus a handful of test helpers) is open. This is the same split the row already
records — this step doc replaces the stale 2026-05-27 red inventory with a clean-tree re-measurement and attributes
every remaining red.

## Exit criteria (relabeled 2026-06-15)

1. **100% `make test` pass** on existing code.
2. **All four apps compile and run.** The four apps are the Makefile `build:` target's binaries (`Makefile:79`):
   `lore`, `star`, `writ`, `devlore-test`.

## Proof run — clean tree, 2026-06-17

`make test` on `refactor/extract-starlark-from-op.phase-8` after `pkg/op/runtime_environment.go` was committed
(`8ea39b9f refactor(op): RuntimeEnvironment changes + configuration doc`):

- **83 packages `ok`**, **21 packages with no tests**, **10 packages red.**
- The 10 red packages = **7 build failures** + **3 packages carrying 4 test reds**.

Both halves are fully attributed below. Neither exit criterion is met: `make test` is not 100% green, and two of the
four apps (`writ`, `lore`) do not compile.

## Red inventory (10 packages)

### A. Build failures (7 packages) — tracked sealed-Graph / RuntimeEnvironment consumer-migration gap

Every build error names a **committed** framework API. The consumers have not been migrated onto it. This is the open
Buckets 4/5 work in [phase-8/graph-immutability.md](../graph-immutability.md) and
[phase-8/lore-migration.md](../lore-migration.md), not transient mid-edit WIP.

| Package | Compiler error(s) | Framework API it names (committed) |
|---|---|---|
| `cmd/writ/writ/adopt` | `plan.go:85` invalid composite literal type `op.Origin`; `plan.go:108,110` `env.ReceiverRegistry` undefined | `op.Origin` is now an **interface** (`pkg/op/origin.go:16`, with `OriginBase` struct at `:36`); `ReceiverRegistry` is a process-wide `sync.OnceValue` function (`pkg/op/receiver_registry.go:139`), not a `RuntimeEnvironment` member |
| `cmd/writ/writ/migrate` | `plan_builder.go:25` `op.ReceiverRegistry` used as a type (it is `func() *op.receiverRegistry`); `plan_builder.go:61` / `file_ops.go:150,152` `env.ReceiverRegistry` undefined; `plan_builder.go:149` not enough arguments to `method.Planner().Plan`; `plan_builder.go:156` `node.Origin` undefined; `file_ops.go:185` / `plan.go:430` invalid composite literal `op.Origin`; `session.go:569` undefined `cli.WriteReceipt` | process-wide `ReceiverRegistry()`; `ActionPlanner.Plan` (`pkg/op/planner.go:195`) signature change; `op.Node` no longer carries `Origin`; `op.Origin` interface; `cli.WriteReceipt` removed |
| `cmd/lore/lore` | `builder_test.go:26` too many arguments to `op.NewRuntimeEnvironmentSpec`; `builder_test.go:35` unknown field `ActionRegistry` in `Planner`; `builder_test.go:38` too many arguments to `planner.buildPackage` | `NewRuntimeEnvironmentSpec(programName string)` is single-arg (`pkg/op/runtime_environment.go:730`); `Planner` and `buildPackage` shapes changed |
| `cmd/writ` | build failed | transitive — imports `cmd/writ/writ/{adopt,migrate}` |
| `cmd/writ/writ` | build failed | transitive — imports `adopt`/`migrate` |
| `cmd/docgen` | build failed | transitive — imports the `writ` / `lore` consumer packages |
| `internal/e2e` | build failed | transitive — imports the `writ` / `lore` consumer packages |

**Confirmation requested at resumption ("confirm which"):** with `runtime_environment.go` committed, these failures
reflect the **tracked** consumer-migration gap, not live uncommitted WIP. Evidence: every error names a committed,
stable framework API (interface `op.Origin`, process-wide `ReceiverRegistry()`, single-arg
`NewRuntimeEnvironmentSpec`, the new `ActionPlanner.Plan` arity, `op.Node` without `Origin`), and the red set is
byte-identical to the prior mid-edit measurement (see Diff below).

### B. Test reds (3 packages, 4 tests)

| Package | Test | Symptom | Attribution |
|---|---|---|---|
| `pkg/op/provider/file` | `TestBackup_DefaultSuffix` | backup path is `myfile.txt.<timestamp>` (no `.devlore-backup.`); test wants prefix `myfile.txt.devlore-backup.` | **RuntimeEnvironment-refactor collateral (row 18 test-migration).** The `.devlore-backup` default moved onto `RuntimeEnvironment.BackupSuffix` — seeded by `RuntimeEnvironmentSpec.Build` defaulting (`runtime_environment.go:150`) and the spec floor (`:664`). `Provider.Backup` pulls `RuntimeEnvironment().BackupSuffix` when the arg is empty (`provider.go:93`), but the file test helper `testProvider` (`provider_test.go:31`) constructs a `RuntimeEnvironment` without that defaulting, so `BackupSuffix` is `""`. The empty-string fallback to `.devlore-backup` was lost in the relocation. |
| `cmd/devlore-test/devloretest` | `TestCompensation` | `compensated.txt` "exists but should not" — compensation did not unwind the prior `write_text` after the downstream `copy` to `/dev/null/...` failed | **Compensation-unwind red, needs diagnosis (row 18 scope).** Fixture `data/test_compensation.star` is on the **old** harness pattern (`t.tmp` / `t.expect_no_file` / `t.run(graph)` / top-level `graph =` magic). Open question: does the sealed-graph executor fail to unwind completed compensable actions on downstream failure, or does the old-harness `t.run` path not drive compensation? Sits inside this step's "resolve all test failures" + the 21.1 harness redesign. Not yet root-caused. |
| `cmd/devlore-test/devloretest` | `TestWalkTreePlanned` | deriving receiver type for the `*op.RecoveryStack` arg of `file.walk_tree` fails: `ResultByUnitID(string) (interface{}, bool)` is "not void, pure, fallible, or compensable" | **Row 21 (function values through the bridge) — known / allowed failure.** `file.walk_tree(root=…, fn=collector, …)` passes a starlark `def collector(initial, resource, path, stack)` whose Go type is `file.Reducer = func(any, *file.Resource, string, *op.RecoveryStack) (any, error)`. The proximate error is the bridge rejecting `*op.RecoveryStack`'s new `ResultByUnitID` method (the step-12 Receipt-broadening) during receiver-type derivation. Tracked by row 21. |
| `cmd/star/cli` | `TestShellCompletionPath/powershell` | `shellCompletionPath("powershell", "star")` returns `("", "")`; test wants `("share/powershell/completions", "star.ps1")` | **Standalone impl/test drift — NOT a refactor consequence, NOT captured by any phase-8 row.** The implementation (`selfinstall.go:329`) has a `"pwsh"` case returning `share/pwsh/completions` + `.ps1`; the test (`selfinstall_test.go:51`) feeds `"powershell"` and expects `share/powershell/completions`. The string `"powershell"` falls to the `default` arm (`:333`). Rows 21/22 cover `cmd/star/star`, not `cmd/star/cli`, so this red is currently orphaned. See Findings. |

## Diff vs prior (mid-edit) measurement

The prior measurement (mid-edit, before `runtime_environment.go` was committed) recorded the same substantive set:
the `writ` / `adopt` / `migrate` / `lore` / `docgen` / `e2e` build failures (traced to `env.ReceiverRegistry`
undefined + invalid `op.Origin` composite literals) and the four test reds `TestBackup_DefaultSuffix`,
`TestCompensation`, `TestShellCompletionPath/powershell`, `TestWalkTreePlanned`.

**The clean-tree red set is identical to the mid-edit set.** Committing `runtime_environment.go` changed nothing in the
failure inventory. The prior "14 FAIL packages" count vs. this run's 10 red packages is a counting artifact — `grep
'^FAIL'` also matches the 4 bare `FAIL` footer lines `go test` prints after a failing package's test output. The
substantive set — **7 build failures + 4 test reds** — is unchanged. This is the proof the build failures were never
transient mid-edit symptoms: they are the stable committed-framework-vs-unmigrated-consumer gap.

## Correction: the 2026-05-27 "22 reds" inventory is stale

The row's embedded "Refined inventory after a fresh `make test`: 22 reds" (`TestImm*` ×10, `TestLintCopyright_*` ×8,
`TestCLI_GraphOnly` + `TestCLI_RoutToFiles`, `TestSourceFile_StarlarkIntegration`) **no longer holds.** On the
2026-06-17 clean tree all of those are green (0 `FAIL:` occurrences for `TestImm`, `TestLintCopyright`,
`TestCLI_GraphOnly`, `TestCLI_RoutToFiles`, `TestSourceFile_StarlarkIntegration`; `cmd/star/star` passes). The live red
set is now dominated by build failures from the sealed-Graph / RuntimeEnvironment consumer migration — a different
shape than the 2026-05-27 script-drift hypothesis. The row's `make test` inventory paragraph should be replaced with
the 2026-06-17 measurement.

## Findings to surface (not unilaterally fixed)

1. **`TestShellCompletionPath/powershell` — DECIDED 2026-06-17 (refined standard).** Naming drift between
   `shellCompletionPath` (`cmd/star/cli/selfinstall.go:329`) and its test (`selfinstall_test.go:51`), unrelated to the
   refactor. The settled standard splits the word by role: executable = `pwsh` (PowerShell 7+, supported on every
   platform; Windows PowerShell unsupported), Go package = `powershell`, completions **directory** = `powershell`,
   product/prose = `PowerShell`. Under it, **both sides need work**: the impl's directory `share/pwsh/completions`
   (`cmd/star/cli/selfinstall.go:330`) is wrong → `share/powershell/completions`; the test's shell-selector **key**
   `"powershell"` is wrong → `"pwsh"` (the selector keys off exe names: bash/fish/pwsh/zsh), while its expected dir
   `share/powershell/completions` is correct. One occurrence of a wider drift (≈65 `powershell` occurrences / ≈20
   files) scoped as its own PowerShell-naming-standardization plan + branch, separate from phase-8.
2. **`TestCompensation` is not yet root-caused.** Diagnosis is owed: sealed-graph executor compensation-unwind vs.
   old-harness `t.run` wiring.

## Re-measurement — 2026-07-02

The red set shrank from 10 packages to 6, and every remaining red is attributed to an owning step:

**Fixed today (this step's bucket-1 work):**

1. `TestBackup_DefaultSuffix` — green. Diagnosis confirmed the 2026-06-17 attribution: `NewRuntimeEnvironment`
   defaults `BackupSuffix` to `.<ProgramName>-backup`, but the file test harness builds the environment as a bare
   struct literal, bypassing that defaulting. Fix: `testProvider` seeds `BackupSuffix: ".devlore-backup"` (mirroring
   the constructor), and `Provider.Backup`'s doc comment now states the environment-derived default instead of a
   hardcoded one. The environment keeps ownership of the default — no duplicate floor in the method.
2. `cmd/lore/lore` — compiles and passes. `builder_test.go` was stale against three committed signatures
   (`NewRuntimeEnvironmentSpec(programName)` single-arg, `Planner` without `ActionRegistry`, five-argument
   `buildPackage`); migrated to all three, registry now reaching the test via the process-global
   `op.ReceiverRegistry()`.

**Fixed earlier (2026-07-02, during step 10):** `TestWalkTreePlanned` — receiver-type derivation now skips
unclassifiable `(T, bool)` methods in derive-fresh mode, so `*op.RecoveryStack` projects across the bridge again.
`TestCompensation` is also green on this measurement (the step-31 recovery-stack arc resolved it; the 2026-06-17
diagnosis question is moot).

**Remaining (6 packages, 2 owners):**

| Red | Owner |
|---|---|
| `cmd/writ`, `cmd/writ/writ`, `cmd/writ/writ/adopt` (`planProvider.Assemble` undefined), `cmd/writ/writ/migrate` (`op.ImmediateOf` undefined et al.) | **Step 33** — `writ migrate` full rewrite onto the sealed-graph executor; patching these green here would accommodate what step 33 is chartered to replace |
| `cmd/docgen` (imports `cmd/writ/writ` at `main.go:17`), `internal/e2e` | **Step 33**, transitively |
| `TestShellCompletionPath_PerShell/powershell` (`cmd/star/cli`) | **Step 28** — PowerShell naming standardization (the settled pwsh/powershell split; both the impl directory and the test key change) |

The exit gate therefore reduces to: **step 33 lands + step 28 lands**, plus a final green sweep here.

## Re-measurement — 2026-07-03 (row-walk verification pass)

`make test` on the current tree: **87 packages `ok`, 20 with no tests, 7 red** — 6 build failures + 1 test red.
(The 2026-07-02 note said "6 packages red"; that count listed `cmd/star/cli` in its table but omitted it from the
total. The correct total is 7.)

Attribution re-verified against the actual compiler output — byte-identical owners, two details sharpened:

- `cmd/writ/writ/adopt` — `plan.go:73` `planProvider.Assemble` undefined (line drifted from the 2026-06-17
  measurement's `:85`). This and `migrate/file_ops.go:156` are **pre-rename callers**: the method is now
  `AssembleDefinition`; adopt/migrate still call `Assemble`. Step 33 owns the rewrite either way.
- `cmd/writ/writ/migrate` — `op.ImmediateOf` undefined ×10 (`execute.go:61/74/78/119/120`, `format.go:89/90/187/193`)
  plus the `Assemble` call above.
- `cmd/writ`, `cmd/writ/writ`, `cmd/docgen`, `internal/e2e` — transitive on the two above. All six → **step 33**.
- `cmd/star/cli` — `TestShellCompletionPath_PerShell/powershell`: `shellCompletionPath("powershell", "star")` returns
  `("", "")`, test wants `("share/powershell/completions", "star.ps1")`. Unchanged → **step 28**.

**Four-apps criterion (2026-07-03):** `lore` ✅ and `star` ✅ build clean via `make build`; `writ` ❌ fails on the
migrate errors above, which **aborts the `build:` target before its fourth line**, so `devlore-test`'s binary is
unreachable through make until step 33 lands — verified separately that `cmd/devlore-test` builds clean (and its
packages pass under `make test`). Three of four apps compile; `writ` is the sole blocker.

## Disposition / grade

`in-progress` — the exit gate is **unmet**, but fully attributed and freshly re-verified (2026-07-03):

- `make test` is **not** 100% green: 7 packages red, all owned — the `writ` family plus its transitive dependents
  (`docgen`, `internal/e2e`) by **step 33**, and the `cmd/star/cli` shell-completion drift by **step 28**.
- Three of the four apps compile (`lore`, `star`, `devlore-test`); `writ` does not, pending its step-33 rewrite, and
  its failure also blocks `make build` from reaching `devlore-test`'s line.

This step's own residual work is the final green sweep once steps 28 and 33 land; it holds no unattributed reds. The
step cannot flip to `complete` from within the numeric row walk — it closes last, after its two owners land.
