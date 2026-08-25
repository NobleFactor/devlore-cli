---
title: "Platform test matrix"
issue: https://github.com/NobleFactor/devlore-cli/issues/373
status: draft
created: 2026-08-12
updated: 2026-08-12
---

# Plan: Platform test matrix

## Summary

The unit suite runs on ubuntu-latest only. macOS and Windows execute exactly one test. The
ruling is unconditional — **all tests must run on every supported platform** — so this plan moves
`make test-race` onto the platform matrix, triages what that reveals, fixes it, and then makes
the legs required rather than advisory.

The matrix this plan built was three legs — ubuntu, macOS, and Windows, all x64 except macOS.
[arm64-build-and-test-matrix.md](./arm64-build-and-test-matrix.md) has since taken it to five by
adding linux/arm64 and windows/arm64. Phase 4 below is the only part affected: it must require
five contexts per job, not three.

## Current State

| Job | Runner(s) | Test command | Scope |
| --- | --- | --- | --- |
| `quality-gate` | ubuntu-latest | `make test-race` | all **121** packages |
| `scenario` | ubuntu, macos, windows | `make test-scenario` | **1** test, 1 package |
| `knowledge-extract` | ubuntu-latest | `make star` | — |

`make test-scenario` is `go test -run TestWritDeployScenario ./cmd/writ`. A green
`scenario (windows-latest)` attests to one writ deploy scenario on Windows and nothing else.

Ten platform-gated files are unit-tested nowhere, and the `_windows.go` / `_darwin.go` members
cannot even be compiled by `quality-gate` — build constraints exclude them on ubuntu. Their only
compilation in CI is `make dist`'s cross-compile at `Makefile:195`, on release tags only.

## What is already proven cross-platform

Worth stating, because it removes the largest unknown: `test-scenario: build` and
`build: generate` mean the existing scenario job **already runs the full code-generation chain**
— `generate` → `inventory` → the `star` generator — on all three platforms, and it is green.
`test-race: generate` therefore inherits a prerequisite chain that demonstrably works on macOS
and Windows today.

Likewise, `TestWritDeployScenario` is symlink-based deployment and passes on windows-latest, so
symlink creation works on that runner. The nine symlink-using test files are lower risk than
they first appear.

## Measured risk

Surveyed across the test tree:

| Signal | Count | Why it matters on Windows |
| --- | --- | --- |
| Hardcoded `/tmp`, `/usr`, `/etc`, `/bin` | 66 | No such paths |
| Permission-bit assertions (`Perm()`, `Mode()&`, `0o644`) | 66 | Windows does not model Unix permission bits |
| Expected values containing `/` separators | 14 | `filepath` yields `\` |
| Test files calling `os.Symlink` | 9 | Privileged on Windows — but demonstrably works on the runner |
| Test files calling `exec.Command` | 6 | Shell and binary names differ |
| Tests already skipping on Windows | 2 | The current extent of platform awareness |
| `runtime.GOOS == "windows"` branches in production code | **0** | Failures will be genuine behavior, not absent branches |

The zero is the most informative number here. The codebase carries no Windows runtime branching
at all; platform variation lives entirely in build-tagged files. So whatever the matrix surfaces
is real behavior on that platform, not a missing conditional.

## Rulings (2026-08-12)

1. **`-race` on every leg.** Not Linux-only. Windows needs `CGO_ENABLED=1` and the runner's
   mingw-w64 toolchain, and the run is slower; that cost is accepted.
2. **Non-blocking first**, so phase 2 gets real triage data without halting branches in flight.
   Phase 4 makes the legs required — mandatory, not optional, and not to be deferred past the next
   merge after phase 3.

   **Mechanism corrected during phase 1.** The original wording specified
   `continue-on-error: true`. That is both unnecessary and harmful here. The `develop` ruleset
   (`12426847`) requires exactly one status check — `quality-gate` — so any job absent from that
   list cannot block a merge whatever it reports. `continue-on-error` would add nothing except
   suppressing the red, and **the red is the phase-2 triage data**. The job therefore lands
   without it: failing legs report as failures, visibly, and still block nothing. Phase 4 reduces
   to a ruleset change with no further edit to `ci.yaml`.

   A side finding from the same query, recorded for its own sake: the existing
   `scenario (macos-latest)` and `scenario (windows-latest)` legs are **not** required checks
   either. A red Windows scenario does not block a merge today.
3. **`quality-gate` stays on ubuntu-latest.** macOS runners bill at 10× and the shell-lint step
   installs `shellcheck` via `apt`. Darwin was considered and rejected on cost and tooling.

### What ruling 3 exposes

Choosing a platform for `quality-gate` does not make it analyze the whole tree. `go vet` and
`golangci-lint` both honor build constraints, so on ubuntu the `_darwin.go` and `_windows.go`
files are invisible to both. Running on Darwin instead would merely change which files are
skipped.

So the ten platform-gated files are not only untested — they are **unvetted and unlinted on every
pull request**, and their sole compilation anywhere in CI is `make dist`'s cross-compile on
release tags. This repository has already been bitten by this: the lint recount had to add an
explicit `GOOS=linux` pass because CI is ubuntu and Darwin-only code was escaping the gate.

The remedy costs no additional runners — the same ubuntu box repeats the invocation per `GOOS`.
Phase 1b covers it.

## Design

Split the concerns rather than matrixing `quality-gate` wholesale — lint, `go mod tidy`,
frontmatter validation, and code metrics are platform-invariant and should not run three times.

```yaml
jobs:
  quality-gate:        # ubuntu-latest — build, tidy, frontmatter, vet, lint, shell-lint, metrics
  test:                # NEW — matrix [ubuntu, macos, windows] — make test-race
  scenario:            # unchanged — matrix [ubuntu, macos, windows] — make test-scenario
```

The `Test` step is removed from `quality-gate`; the matrix's ubuntu leg covers it.

## Implementation Phases

### Phase 1: Add the matrix job — branch `ci/platform-test-matrix`

- [ ] Add a `test` job with `strategy.matrix.os: [ubuntu-latest, macos-latest, windows-latest]`
      and `fail-fast: false`, so every platform reports independently — matching the reasoning
      already recorded on the `scenario` job.
- [ ] Run `make test-race` in it, on every leg, per ruling 1. Set `CGO_ENABLED=1` on the Windows
      leg so the race detector finds the runner's mingw-w64 toolchain.
- [ ] Remove the `Test` step from `quality-gate`; the matrix's ubuntu leg covers it.
- [ ] **Land it non-blocking without `continue-on-error`** — see ruling 2's correction. The job is
      not in the ruleset's required list, so it blocks nothing; suppressing its red would only
      destroy the triage signal.

**Files**: `.github/workflows/ci.yaml` — Modify.

### Phase 1b: Static analysis across every GOOS — branch `ci/goos-analysis-sweep`

Independent of the test matrix and cheap — one ubuntu runner, three invocations. Closes the
compile-and-lint half of the same blind spot.

- [ ] `GOOS=linux`, `GOOS=darwin`, `GOOS=windows` each get a `go vet ./...` pass in
      `quality-gate`.
- [ ] The same three for `golangci-lint run`, so `_darwin.go` and `_windows.go` are linted rather
      than skipped.
- [ ] `GOOS=windows go build ./...` and `GOOS=darwin go build ./...`, so a compile error in a
      platform-gated file surfaces on the pull request rather than at release.
- [ ] Expect findings: these files have never faced the linter. Suppressions added here must
      carry a platform-reasoned justification, per the standing convention — never a bare
      directive.

**Files**: `.github/workflows/ci.yaml`, possibly `Makefile` (a `vet-all` / `lint-all` target so
the sweep is runnable locally, not only in CI) — Modify.

#### Phase 2 baseline corrected — PR #388 / commit `e34ba2c7`

With the three `[build failed]` packages compiling on Windows for the first time, the honest
count is **85 failures, not 34** — the `--- FAIL`-line counting had silently excluded three
entire suites. The new failures cluster where the known causes predict: ~17 in `fsroot`/`triad`
path tests (**#377**, separator form), ~25 in the file provider's write/compensate suite
(separators plus permission semantics), and ~8 in graph-resume and receipt round-trips —
including `TestReceipt_RestoreEncoded_JSONandYAML`, which is **#377's serialized-document
consequence demonstrating itself in CI** rather than by analysis. Zero packages fail to build.
The 34→85 jump is measurement honesty, not regression: every one of these failures existed on
every prior run, invisible.

#### Phase 3e state — the handle-leak cluster is CLOSED, 48 → 28 (PR #402, 2026-08-13)

The burn-down is now **85 → 57 → 48 → 50 → 48 → 28**. PR #402 (issue #393, plan
[env-minted-root.md](./env-minted-root.md)) cleared the entire handle-leak cluster: the leak
signature (`TempDir RemoveAll cleanup: … being used by another process`) went **18 → 0**, with
zero panics and zero `[build failed]` packages, measured uncapped against head `27d23e8e`'s
check-runs. Seventeen `--- FAIL` lines cleared and none appeared.

**The attribution the earlier estimate got wrong.** This plan predicted −18 from the #393 ruling
(a spec no longer carrying a live Root). The framework change alone delivered **−3**; the
remaining **−17** came from a second commit closing fifteen *test* environments that were never
closed. The production leaks #393 diagnosed were real and are fixed — lore's deploy loop handing
one spec to N executors (so every iteration after the first ran against a closed Root), writ
verify and readback never closing their loading environments, devlore-test's root aliasing — but
they were not what those eighteen tests were failing on. The proof was a natural experiment
inside one package: the three plan-package helpers that close their environment had no failing
tests; the five that did not accounted for exactly fifteen. A right number reached by wrong
reasoning is still a miss; inherited estimates get their own enumeration.

**The 28 that remain** are the phase-3e grind, one failing package each across twelve packages:
file-provider write/permission semantics (`TestWrite_*`, `TestCopy_WritesNewFile`,
`TestWriteBytes_*`), path semantics (`TestName_*`, `TestParent_*`, `TestCommonAncestor`,
`TestSourcePath_ShardedLayout`), ownership (`TestApplyOwnership_*`, `TestResolveOwnership_*` — renamed from `TestApplyChown_*` /
`TestParseChown_*` in #514; same tests, same failures), git argv
(`TestCheckout_BuildsArgv`, `TestPull_BuildsArgv`), 2 CLI error-text expectations
(`TestCLI_ConfigPath`, `TestCLI_RunMissingFile`), and singles including
`TestSourceFile_StarlarkIntegration` (#376). Two survivors — `TestGatherFailureUnwind_ViaPublicAPI`
and the git clone resume test — were failing for a second reason behind the leak, now their only
reason.

#### Phase 3e state — first fully honest count (2026-08-13, superseded above)

The burn-down: **85 → 57 → 48 → 50 → 48**, with the two rises being unmaskings (panics abort a
package's whole test binary; clearing them lets more tests run). As of PR #400's leg there are
**zero panics** on Windows — every binary runs to completion, so 48 is the campaign's first
fully honest count. Landed along the way: the canonical-`Rel` fix (#377, in #389), the
slash-native `Find` matcher (#395), the ten fixture-root anchors (#398), the generator header
(#399, closing #396), and the file-package trio (#400) — including `resolveFindRoot`'s
rooted-pattern scope defect, a product fix.

The 48 decompose to: **18** handle-leak failures (#393 — CLOSED by PR #402; see the section
above, including the correction to this row's diagnosis), ~25 write/path/chown semantics (3e
grind), 1 Starlark escape (#376), 2 CLI error-text expectations, ~2 singles.

#### Phase 3e progress — the `TestLintCopyright` cluster (2026-08-12)

The five `TestLintCopyright` failures were a **product defect in `file.Find`**, not fixture
problems: `matchDoubleStar`/`splitFindPattern` split on `filepath.Separator` and matched via
`filepath.Match`, so on Windows every `**` glob returned nothing — the copyright linter (and any
Starlark `file.find` caller) silently found zero source files there. Fix: the matcher is
slash-native (`path.Match`; glob patterns are a slash-form language on every platform, the same
contract as `io/fs` and canonical `rel`), patterns normalize at `splitFindPattern`, and the walk
converts OS-native paths at the single boundary in `findWalkFunc`. Mutation-verified against the
lint-copyright suite. Expected windows-leg effect: the 5-cluster clears; `Find`-dependent
failures elsewhere in the remainder may clear with it.

#### Phase 1b results (2026-08-12)

`make vet-all`, `make lint-all`, and `make build-all` exist and are wired into `quality-gate` in
place of the single-GOOS `vet` and direct-lint steps, with a cross-compile step added. Findings
from the first local run, all fixed rather than suppressed:

- **`GOOS=windows go vet` failed to compile three test files** — `syscall.Umask` in
  `pkg/op/default_funcs_test.go` and `pkg/op/provider/plan/deferred_default_test.go`,
  `syscall.Stat_t` in `pkg/op/provider/file/provider_test.go`. This is the cause of a **triage
  undercount**: those three packages reported `[build failed]` on the windows test leg, which the
  `--- FAIL`-line counting never included — the standing "34 failures" excludes whatever those
  packages' suites will reveal once they run. Fixes: the umask tests now read through production's
  portable `processUmask` seam (a build-tagged `testProcessUmask` pair for the external test
  package), making them *run* on Windows rather than be skipped; the chown test moved to
  `provider_unix_test.go`, scoped because its subject (uid/gid) exists only on Unix.
- **First-ever windows lint pass: 5 findings, linux and darwin zero.** `statIdentity`
  (`helpers_windows.go`) gained its unix twin's named results; `runShellCommand` and its two
  knobs moved to `pkg/platform/helpers_unix.go` (every consumer is unix-gated — under a windows
  analysis they were dead code); `captureRefresh` moved to `update_unix_test.go` for the same
  reason.
- **Compliance sweep ridden along** (user directive, 2026-08-12): all 11 touched Go files brought
  to the full go-style standard — 17 multi-line doc summaries split, ~105 missing
  blank-after-signature lines inserted, 7 over-120-column lines rewrapped or shortened, fixture
  struct members documented. Mechanical detectors report zero findings; the one >120 survivor is
  `bracket`'s signature, exempt under §8.

### Phase 2: Triage — no branch; produces a report

- [ ] Enumerate every failure, per platform, from the phase 1 run. Uncapped — a count read off a
      truncated log is not a count.
- [ ] Classify each into exactly one of three buckets:
  1. **A genuine defect on that platform.** The product is wrong there. Fix the product.
  2. **A Unix-assuming test.** The product is fine; the test hardcodes `/tmp`, a permission bit,
     or a `/` separator. Fix the test.
  3. **Correctly platform-scoped.** The subject only exists on one platform — an `apt` manager
     test has no meaning on Windows. This is **not** a skip; the test moves into a
     `_linux_test.go` / `_darwin_test.go` / `_windows_test.go` file so the build constraint
     expresses it. A `t.Skip` added to dodge a red result is bucket 1 or 2 in disguise.
- [ ] Record the classification in this document before any fix lands.

#### Phase 2 results — first run, PR #375 / commit `f8b28c82`

`test (macos-latest)` and `test (ubuntu-latest)` **passed** — the full race-enabled suite over 121
packages, the first time it had ever run on Darwin. `test (windows-latest)` failed with **63**
`--- FAIL` lines, which collapse into far fewer causes:

| Cause | Failures | Bucket | Disposition |
| --- | --- | --- | --- |
| `binary` lacks `.exe`, so every exec returns -1 | est. ~37 | 2 | Fixed — `cli_test.go` |
| `Rel()` returns `\` separators | 6 | **1** | **Issue #377** |
| Permission-bit assertions (`document_test.go`, `sops_integration_test.go`) | 5 | 2 | Pending |
| `open /dev/null` | 2 | 1 or 2 | Pending — depends whether the product hardcodes it |
| Windows path breaks Starlark escape parsing | 1 | **1** | **Issue #376** |
| `openat source_input.txt`, compensation leftover | 2 | ? | Pending — needs reading |

The dominant cause was a single missing suffix: `go build -o` appends `.exe` on Windows, so the
binary landed at `devlore-test.exe` while the exec path still said `devlore-test`. Every
invocation failed to start, reporting `exit code = -1` with empty output, which cascaded into all
30 `TestCLI_*` failures plus the 5 "no JSON summary" and 2 "missing Hello World!" assertions.

**Two genuine product defects were found**, both invisible to the previous ubuntu-only gate and
both filed rather than patched inline:

- **#376** — Starlark extension loading breaks on any Windows path: `C:\Users\…` interpolated into
  Starlark source fails on `\U`, an invalid escape.
- **#377** — `fsroot.Path.Rel()` yields OS-native separators, and `Path` is a serialized type
  (`root.go:624`, `root.go:659`), so documents and their checksums differ between platforms. The
  six failing assertions are correct; the product is wrong. This contradicts the repository's own
  platform-stable digest rule.

Clearing the `.exe` cause first is deliberate: ~37 masked failures may be concealing others, and
the next run reveals what was behind them. The remaining counts above should be treated as a
lower bound until that run reports.

#### Phase 2 results, corrected — second run, PR #378 / commit `762c61ec`

**63 → 47.** The `.exe` estimate was wrong: it cleared **16**, not ~37. The fix itself worked —
exit codes became real numbers instead of `-1`, so the binary starts — but fourteen `TestCLI_*`
tests were failing for a *second* reason sitting behind the first. The masking risk named above
was real, which is why the table's counts were labelled a lower bound.

| Cause | Failures | Bucket | Disposition |
| --- | --- | --- | --- |
| Output streams default to `/dev/stdout`; `/dev/null` in examples | ~16 | **1** | **Issue #379** |
| Separator form — `Rel`, `Abs`, `String`, `registry.FilePath` | 11 | mixed | **Issue #377**, needs splitting |
| Permission-bit assertions | 5 | 2 | Pending |
| `runner_test` file and compensation failures | 4 | ? | Pending — needs reading |
| Windows path breaks Starlark escape parsing | 1 | **1** | **Issue #376**, correctly sized |

**#379 is the dominant remaining cause and a genuine product defect.**
`cmd/devlore-test/devloretest/commands.go:53-55` defaults every stream to `/dev/stdout`, and
`openDest` (`commands.go:148`) is a plain `os.OpenFile`. On Windows the open fails and
`devlore-test run` aborts before emitting anything — the CLI cannot run there at all. The
documented examples route to `/dev/null`, so the shipped guidance is Unix-only too.

**#376 was correctly sized** — `invalid escape sequence` appears exactly once in the log.

**#377 needs its classification split.** `Rel()` returning `\` is bucket 1: `Path` is serialized,
so document bytes and checksums differ per platform. But `Abs()` and `String()` returning
`\project\src\main.go` is *correct* on Windows — those three assertions feed Unix literals like
`/project/src/main.go` and expect them back, making them bucket 2. The two groups must not be
fixed the same way.

Three product defects have now been surfaced by one CI change — #376, #377, #379 — none of them
reachable by the previous ubuntu-only gate.

### Phase 3: Fix — one branch per cluster

- [ ] Bucket 1 defects: fixed in the product, each with a regression test.
- [ ] Bucket 2 tests: `filepath.Join` over string concatenation, `t.TempDir()` over `/tmp`,
      permission assertions guarded by the platform that has them.
- [ ] Bucket 3 tests: relocated into build-tagged files. Nothing is deleted and nothing is
      skipped to make a leg green.

Branch count and grouping are sized after phase 2, since the failure count is unknown today.

### Phase 4: Enforce — no branch; a ruleset change

Nothing in `ci.yaml` changes, per ruling 2's correction.

**Both matrices are now five legs, not three** — see
[arm64-build-and-test-matrix.md](./arm64-build-and-test-matrix.md), which added linux/arm64 and
windows/arm64. This phase must name all five per job, or the two arm64 legs stay advisory while
the rest are enforced, which is the worst of both arrangements: a gate that looks complete and
is not.

- [ ] Add the five `test (…)` legs to `required_status_checks` on ruleset `12426847`, which
      today lists only `quality-gate`:
      `test (macos-latest)`, `test (ubuntu-latest)`, `test (ubuntu-24.04-arm)`,
      `test (windows-latest)`, `test (windows-11-arm)`
- [ ] Consider adding the five `scenario (…)` legs at the same time — they are not required
      either, so a red Windows scenario does not currently block a merge.
- [ ] Confirm the context strings against a real run before writing them into the ruleset. The
      `test` job now uses `matrix.include`, and GitHub composes a leg's check name from the
      matrix values; a name that does not match exactly is a required check that never arrives,
      which blocks every merge rather than none.
- [ ] Confirm a deliberately failing test on Windows blocks a merge. The gate is not proven until
      it has refused something.

## Verification

`make test-race` green on every leg that supports it, and `make test` green on windows/arm64,
which does not — the full suite runs there, only the race detector is absent. Check-runs must be
attached to the pull request's real
head SHA — verified via `gh api repos/<repo>/commits/<sha>/check-runs`, not via `gh pr checks`,
which reports whatever the pull request head happens to be. PR #371 merged without its tests
precisely because that distinction was not made.

## Open Questions

None outstanding. The rulings below close every question this plan opened.

## Related Documents

- Issue #373 — this gap
- [docs/plans/audit-remediation.md](./audit-remediation.md) — issue #365; every refactor it lands
  is currently verified on one platform out of five
- [arm64-build-and-test-matrix.md](./arm64-build-and-test-matrix.md) — took both matrices from
  three legs to five, and the push-path build from host-only to all six GOOS/GOARCH pairs
