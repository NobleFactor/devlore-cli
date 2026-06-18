---
step: 18
title: "Resolve all test failures (phase-8 exit gate)"
former_step: 21
former_title: "Test triage — pre-existing failures"
status: in-progress — exit gate UNMET (10 packages red; writ + lore do not compile)
proof_run: 2026-06-17
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
Buckets 4/5 work in [phase-8/21-graph-immutability.md](../21-graph-immutability.md) and
[phase-8/21-lore-migration.md](../21-lore-migration.md), not transient mid-edit WIP.

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

## Disposition / grade

`in-progress` — the exit gate is **unmet**:

- `make test` is **not** 100% green: 10/97 packages red.
- The four apps do **not** all compile: `writ` and `lore` (and, transitively, the `docgen` tool and the `internal/e2e`
  suite) fail to build against the committed sealed-Graph / RuntimeEnvironment / `op.Origin`-interface framework.

The framework half (the `pkg/op` seal) is complete and green. The remaining work is the consumer / test / template
migration scoped in [phase-8/21-graph-immutability.md](../21-graph-immutability.md) (Buckets 4/5 — the `writ` and `lore`
apps) and [phase-8/21-lore-migration.md](../21-lore-migration.md) (the `op.Origin` interface + `OriginBase`,
`plan.Provider.Plan(name, args, kwargs)`), plus three localized test-migration fixes
(`file` `testProvider` BackupSuffix seeding, the `TestCompensation` diagnosis, and the orphaned
`TestShellCompletionPath/powershell` drift).
