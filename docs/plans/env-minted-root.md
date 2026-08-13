---
title: "Environment-Minted Root"
issue: https://github.com/NobleFactor/devlore-cli/issues/393
status: in-progress
created: 2026-08-13
updated: 2026-08-13
---

# Plan: Environment-Minted Root

Implements the amended #393 ruling (option 4, banked at
[#393 issuecomment-5283660132](https://github.com/NobleFactor/devlore-cli/issues/393#issuecomment-5283660132)):
`RuntimeEnvironmentSpec` carries no live `fsroot.Root` — only the anchor path and access mode —
and every `RuntimeEnvironment` mints its own Root and closes it. Branch: `env-minted-root`.
Tracking issue: #393 (pre-existing; no new issue). Campaign: #373 / [platform-test-matrix.md](./platform-test-matrix.md).

## Summary

The confined Root's directory handle (`os.OpenRoot`) leaks on every path that mints a Root into a
spec without a Run to close it, and Windows turns each leak into a `RemoveAll` test failure — 18
of the standing 48. The verified fix: no handle ever lives in a spec. The spec carries two
serializable values (anchor path + mode), `NewRuntimeEnvironment` mints from them, and
`RuntimeEnvironment.Close` — already the single closer — releases what it minted. Nothing mints
before environment build, so pre-Run error paths cannot leak, structurally. The deciding fact was
verified exhaustively on 2026-08-13: nothing between spec construction and Run dereferences
`spec.Root` (sole production readers `runtime_environment.go:200,206`; all nine host mint→Run
gaps clean; no `.star` script or doc touches `spec.root`).

## Goals

1. **No live Root in any spec** — `RuntimeEnvironmentSpec` holds anchor path + access mode only.
2. **The environment mints and closes its own Root** — executor lifecycle ownership unchanged.
3. **`plan.spec` keeps its fail-fast contract** — an open-and-release probe at spec time; the
   authoritative gate stays the mint at Run's preflight (`PhasePreparing` →
   `ReasonPreflightFailed`).
4. **Clear the 18 Windows handle-leak failures** and structurally heal the lore closed-Root loop
   defect (`cmd/lore/lore/commands.go:259`).

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Happy-path close | ✅ Working | `GraphExecutor.Run` defer → `RuntimeEnvironment.Close` → `iox.Close(re.Root)` |
| Pre-Run error paths | ❌ Leak | `plan.Provider.Spec` (`provider.go:475`), devlore-test `buildSpec` (`test_context.go:829`) mint Roots no Run closes |
| lore command loop | ❌ Defect | one spec's Root reused by N executors (`commands.go:259`); iteration 2+ runs against a closed Root |
| devlore-test root aliasing | ⚠️ Latent | `runner.go:281` aliases the spec's root into `TestContext` for post-close reads; benign only because unconfined `Close` is a no-op |
| `cmd/lore/lore/integration_test.go` | 💀 Dead | `//go:build ignore`, references removed APIs (`op.contextBase`) — rider: delete |

## Requirements

### 1. `fsroot.Mode` and mode-dispatched opening

`pkg/fsroot` gains a `Mode` type naming its three implementations, and one constructor that
dispatches on it:

- `ModeConfined` (**zero value** — the production-correct default), `ModeUnconfined` (read-only),
  `ModeWritableUnconfined`.
- `Open(dir string, mode Mode) (Root, error)` — dispatches to `OpenConfined` /
  `OpenUnconfined` / `OpenWritableUnconfined`. Only the confined branch can fail; the unconfined
  constructors stay error-free and public (tests and `TestContext` use them directly).

### 2. Spec carries anchor + mode

In `pkg/op/runtime_environment.go`:

- `RuntimeEnvironmentSpec.Root fsroot.Root` → `RootPath string` + `RootMode fsroot.Mode`.
- `WithRoot(root fsroot.Root)` → `WithRoot(path string, mode fsroot.Mode)` — anchor and mode
  travel together; the name stays because the method still specifies the root.
- Empty `RootPath` = no root: `env.Root` stays nil and no `RecoverySite` is built — the existing
  nil-Root semantics, with the guard at `runtime_environment.go:206` re-keyed to the mint result.
- Starlark surface: the spec's goReceiver now projects `root_path` / `root_mode` instead of
  `root`. Verified breaking nothing — no script or doc dereferences `spec.root`.

### 3. `NewRuntimeEnvironment` mints; the signature gains an error

- `NewRuntimeEnvironment(ctx, spec) *RuntimeEnvironment` →
  `(ctx, spec) (*RuntimeEnvironment, error)`. When `RootPath` is non-empty, mint via
  `fsroot.Open(spec.RootPath, spec.RootMode)`; a mint failure is the error. `Close` is unchanged
  — it already owns the Root's release.
- Callers updated: `op.Plan` (`planner.go:49` — propagates), `GraphExecutor.Run`
  (`graph_executor.go:477`), `GraphExecutor.ResumeUnwind` (`graph_executor.go:296`), and the
  direct-environment hosts (`cmd/star/star/application.go:82`, `cmd/writ/writ/verify/verify.go:260`,
  `cmd/writ/writ/readback/readback.go:520`, `cmd/lore/lore/builder.go:107`).
- In `Run` / `ResumeUnwind`, a mint failure lands the preflight-failed terminal before any
  dispatch: `RunStatus{Phase: PhaseStopped, Condition: ConditionExecutionFailed, Reason:
  ReasonPreflightFailed}` — and the teardown defer is registered only after a successful build.

### 4. `plan.Provider.Spec` keeps fail-fast via open-and-release

`provider.go:475`'s retained mint becomes a probe: `fsroot.OpenConfined(rootPath)` +
immediate `Close` in the same statement group. The error contract and message shape
(`"plan.Provider.Spec: open root %s: %w"`) survive verbatim; the mint itself is the check, so
no second definition of "valid" exists. The returned spec is built with
`WithRoot(rootPath, fsroot.ModeConfined)`.

### 5. Host mint sites dissolve

Every host `OpenConfined` + error handling collapses to a `WithRoot(path, mode)` argument. Spec
factories whose only error source was the mint become infallible (error return dropped —
greenfield, no compatibility shims), and their callers simplify:

| Site | Change |
| --- | --- |
| `cmd/writ/writ/deploy/plan.go:387,412` | `deploySpec` / `runSpec` infallible; confined |
| `cmd/writ/writ/adopt/batch.go:274` | `buildSpec` infallible; confined |
| `cmd/writ/writ/migrate/execute.go:74`, `helpers.go:90` | inline mint removed; `migrateSpec` infallible; confined |
| `cmd/writ/writ/upgrade/upgrade.go:566` | `upgradeSpec` infallible; confined |
| `cmd/writ/writ/decommission/decommission.go:295` | `removalSpec` infallible; confined |
| `cmd/writ/writ/secret/encrypt.go:253` | `encryptSpec` infallible; confined |
| `cmd/writ/writ/verify/verify.go:255` | mint removed; spec `WithRoot(separator, confined)`; handles the env-build error |
| `cmd/writ/writ/readback/readback.go:515` | same as verify |
| `cmd/lore/lore/commands.go:245` | mint removed; the N-executor loop at `:259` heals — each Run mints fresh |
| `cmd/star/star/application.go:81` | `WithRoot(wd, ModeWritableUnconfined)`; handles the env-build error |
| `cmd/devlore-test/devloretest/test_context.go:829` | `buildSpec` mint removed; confined at `tc.tmpDir` |
| `cmd/devlore-test/devloretest/runner.go:241` | spec gets `WithRoot(tmpDir, ModeWritableUnconfined)`; `TestContext` keeps constructing its **own** unconfined root — separate object, separate lifecycle; the aliasing and double-close dissolve |

### 6. Tests

- `fsroot.Open` dispatch: one test per mode, plus the confined failure path.
- `NewRuntimeEnvironment` with a bad anchor returns an error; with an empty `RootPath` yields a
  nil-Root environment with no `RecoverySite`.
- `GraphExecutor.Run` with a bad anchor lands `ReasonPreflightFailed` and dispatches nothing.
- **Closed-Root-reuse regression (cross-platform, the lore bug's shape):** two executors built
  sequentially from one spec; both Runs succeed because each mints its own Root. This asserts the
  heal on every OS — it does not need Windows to observe.
- `plan.Provider.Spec` fail-fast: existing contract tests keep passing against the probe.
- Migration of the ~19 `WithRoot(` occurrences across 10 test files to the new signature.
- The Windows proof stays CI-side: the 18 leak failures (plan-package graph tests + resume
  family) are expected to clear on `test (windows-latest)` — Unix cannot observe the leak.

## Implementation Phases

### Phase 1: fsroot Mode — status: complete (2026-08-13)

- [x] `Mode` type, three values, `Open(dir, mode)` dispatch, doc comments per style guide.
- [x] Unit tests for the dispatch and the confined failure path — behavioral discriminators
      (post-Close failure = confined; `errors.ErrUnsupported` on write = read-only; post-Close
      success = writable unconfined), plus the missing-directory failure and the zero-value
      contract (`Mode(0) == ModeConfined`). `make vet` and full `make test` green.

**Files**: `pkg/fsroot/root.go` — Modify; `pkg/fsroot/root_test.go` — Modify.

### Phase 2: spec + environment + executor — status: complete (2026-08-13)

- [x] Spec fields + `WithRoot(path, mode)`; `NewRuntimeEnvironment` mints, returns error.
- [x] `Run` / `ResumeUnwind` preflight-failure terminals; defer registered only after a
      successful build.
- [x] `op.Plan` propagates the environment-build error.
- [x] `pkg/op` test migration + the new tests: mint-from-anchor, bad-anchor error, empty-anchor
      nil-Root semantics, `Run` bad-anchor → `ReasonPreflightFailed`, and the closed-Root-reuse
      regression (two executors, one spec, both green). Ride-along: `env` locals renamed
      `environment` in every touched test file (standing naming ban).

**Discovered constraint (2026-08-13):** phases 2–4 cannot be verified independently — `make
test` depends on `generate`, which builds the in-tree `star` binary, which consumes the changed
APIs through `cmd/star/star` and `pkg/op/provider/plan`. Phase 3 and the star slice of phase 4
were therefore pulled forward and verified jointly with phase 2.

### Phase 3: plan provider probe — status: complete (2026-08-13)

- [x] `plan.Provider.Spec` open-and-release probe; spec built from anchor + confined mode; error
      message shape preserved (`"plan.Provider.Spec: open root %s"`).
- [x] Plan-provider test migration (all nine test files); the defaults-contract test now asserts
      `RootPath`/`RootMode` instead of handle identity. Full `make test`: every `pkg/` package
      green; remaining reds are exactly the phase-4 host set below.

### Phase 4: hosts — status: complete (2026-08-13; rider executes at commit)

- [x] `cmd/star/star/application.go` — pulled forward (codegen gate; see phase 2 note).
- [x] All remaining host sites migrated. The six writ spec factories (`deploySpec`, `buildSpec`,
      `migrateSpec`, `upgradeSpec`, `removalSpec`, `encryptSpec`) became infallible and their
      eleven caller hunks simplified; adopt and migrate-register collapsed their two-spec
      planning/execute pairs into one shared spec (safe under #393 — each phase mints its own
      Root). lore's `commands.go` mint removed — the N-executor loop now heals structurally.
      verify/readback anchor at the separator path via the spec; both loading environments now
      CLOSE (a pre-existing handle leak in both, fixed en passant). devlore-test's spec carries
      the anchor; `TestContext` keeps its own writable-unconfined root (aliasing dissolved).
- [x] Host-side test migration (`migrate/receipt_integration_test.go`, `lore/builder_test.go`).
      Ride-along: `env`/`sharedEnv` identifiers renamed in every touched file.
- [ ] Rider: delete dead `cmd/lore/lore/integration_test.go` (`//go:build ignore`, removed
      APIs) — a `git rm` folded into the eventual commit script (git operations are script-side).

**Verification:** full `make test` — ZERO failures repository-wide; gofmt clean over the change
set.

**Flagged during phase 4 (user request, 2026-08-13): defer-lambda closes vs `iox.Close` in
writ** — findings reported; resolution pending a ruling: `verify.go` `loadGraph` and
`readback.go` `Fold` (both introduced this change), `identity.go:214` (pre-existing;
`identity.go:103` already uses the idiom), `snapshot_test.go:152` (test-side, different fix
shape — no error return to join).

**Files**: the `cmd/` files in the Requirement 5 table — Modify; `cmd/lore/lore/integration_test.go` — Delete.

### Phase 5: sweep and proof — status: complete except the PR (2026-08-13)

- [x] `gofmt -l` clean over the change set (40 files).
- [x] Go-style compliance sweep on every touched file: **77 multi-line doc summaries split**, **37
      over-120-column lines rewrapped** (10 of them multi-line signature reflows), and one
      blank-line-after-signature inserted. Both mechanical detectors — first-doc-line-followed-by-text,
      and raw >120 columns — now report **zero** findings across the touched set.
- [x] `make vet-all`, `make lint-all`, `make build-all`: green on linux, darwin, AND windows
      (lint reports `0 issues` per GOOS).
- [x] Full `make test`: **zero failures repository-wide**.
- [x] **`iox.Close` conversions** (user directive): `verify.go` `loadGraph`, `readback.go` `Fold`,
      and the pre-existing `identity.go` `loadRecipientsFile` converted from `defer func(){ _ =
      x.Close() }()` to named-return + `defer iox.Close(&err, x)`, so close failures join the
      result instead of being discarded under a nolint. No production defer-lambda close remains
      in `cmd/writ`. (`snapshot_test.go:152` is test-side with no error return — left as is.)
- [ ] PR; `test (windows-latest)` expected at ~30 failures (48 − 18). Count measured against the
      head commit's check-runs, uncapped.

**Known non-blocker:** `make check` fails its `complexity` step on two pre-existing functions this
branch never touched — `git guessDirName` (27) and `cli runSelfInstall` (22). The gate is
local-only (CI's `quality-gate` does not run it) and unrelated to this work; every other `check`
step passes.

## Migration Path

None — greenfield internal refactor. The Starlark-facing surface is unchanged where it is used
(`plan.spec(root_path=...)` arguments are already strings; no script reads `spec.root`).

## Files to Create/Modify

Consolidated in Requirements 1–5 and the phase lists above; no new files beyond tests.

## Related Documents

- Issue #393 — the ruling (amended 2026-08-13) and full diagnosis
- [platform-test-matrix.md](./platform-test-matrix.md) — the #373 campaign this clears 18 failures for
- Issue #373 — CI platform gap

## Open Questions

None — the amended ruling closes the design; naming and placement decisions above follow it and
the style guide.
