---
title: "Deploy manifest error propagation"
issue: https://github.com/NobleFactor/devlore-cli/issues/368
status: draft
created: 2026-08-11
updated: 2026-08-11
---

# Plan: Deploy manifest error propagation

## Summary

`lore deploy` swallows a manifest it cannot load. `parseLoreDeployConfig` logs the failure and
continues, so the command deploys whatever else parsed and exits 0 — reporting success for a
request it only partly fulfilled. This plan propagates the failure and removes the dead error
return that the swallow created.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `manifest.Load` failure | ❌ Swallowed | Logged and `continue`d at `cmd/lore/lore/commands.go:111` |
| `parseLoreDeployConfig` error return | ❌ Dead | Always `nil`; masked by `//nolint:unparam` |
| Exit code on partial deploy | ❌ Wrong | 0, despite an unfulfilled request |
| Diagnosis on total failure | ❌ Misleading | "no packages to deploy", not the load error |
| Test coverage | ❌ None | No test exercises an unloadable manifest |

### The defect

`cmd/lore/lore/commands.go:108` loads each `@manifest` argument. On failure,
`cmd/lore/lore/commands.go:110` prints and `cmd/lore/lore/commands.go:111` skips it. Because
every failure is absorbed there, `cmd/lore/lore/commands.go:126` can return `nil`
unconditionally — and the signature at `cmd/lore/lore/commands.go:89` carries
`//nolint:unparam // error return reserved for future use`, which describes the consequence as
though it were the design.

`manifest.Load` is `document.ReadFile[PackagesManifest]` (`internal/manifest/manifest.go:81`),
so it fails on a missing file, an unreadable file, a malformed document, or a schema mismatch.
A filename typo is enough.

### Two observable failures

1. **`lore deploy @typo.yaml some-package`** — the manifest is skipped, `some-package` deploys,
   `len(resolved) != 0`, and `runDeploy` returns `nil`. **Exit 0 on a partial deploy.** CI
   wrapping this goes green.
2. **`lore deploy @typo.yaml`** — `cfg.Packages` is empty and `runDeploy` returns
   "no packages to deploy" from `cmd/lore/lore/commands.go:81`. Correct exit code, wrong cause;
   the real error has scrolled past.

## Goals

1. **A manifest the user named and that cannot be loaded fails the command** — no silent skip.
2. **The reported error names the manifest and the underlying cause** — never "no packages to
   deploy" standing in for "that file does not exist".
3. **`parseLoreDeployConfig`'s error return is live**, and the `//nolint:unparam` is deleted
   rather than re-argued.
4. **Regression tests pin both failure modes.**

## Design decision required

Parsing completes before any deployment executes, so aborting costs the user nothing already
done. That rules out any "best effort" argument grounded in partial work. Three candidates:

| Option | Behavior | Residual failure |
| --- | --- | --- |
| **A — fail fast** | The first unloadable manifest aborts with its error | A user with three broken manifests fixes them one run at a time |
| **B — collect, then abort** (recommended) | Attempt every manifest, join all failures via `errors.Join`, abort before deploying anything | Slightly more code; error output is multi-line |
| **C — skip, but exit non-zero** | Current behavior, with a correct exit code and a summary of what was skipped | Preserves partial deploys as a *feature*. A green-looking run still under-deploys, and the greenfield principle argues against keeping a behavior only because it exists |

**Recommendation: B.** Same safety as A, strictly better diagnostics, and it matches the fact
that nothing has been executed yet at parse time. Requires the user's ruling before
implementation.

## Implementation Phases

### Phase 1: Propagate — branch `fix/deploy-manifest-error-propagation`

- [ ] Replace the `cli.Error` + `continue` at `cmd/lore/lore/commands.go:110` with error
      collection per the approved option.
- [ ] Return the joined error from `parseLoreDeployConfig`; delete the `//nolint:unparam` at
      `cmd/lore/lore/commands.go:89`.
- [ ] Confirm `runDeploy`'s existing `if err != nil` at `cmd/lore/lore/commands.go:64` already
      surfaces it — it does; no change expected there, verify rather than assume.
- [ ] Re-read "no packages to deploy" at `cmd/lore/lore/commands.go:81`. With load failures now
      propagating, that message should only appear when the user genuinely named nothing
      deployable. Confirm it is still reachable and still correct.

**Files**: `cmd/lore/lore/commands.go` — Modify.

### Phase 2: Tests

- [ ] Unloadable manifest alone → the returned error names the manifest path and wraps the
      `document.ReadFile` cause.
- [ ] Unloadable manifest plus a valid direct package → the command **fails**; nothing deploys.
      This is the exit-0 regression and is the most important test in the plan.
- [ ] Two unloadable manifests → both are reported (option B only).
- [ ] Valid manifest plus valid package → unchanged behavior; guards against over-correction.
- [ ] A genuinely empty but well-formed manifest → still "no packages to deploy", not a load
      error.

**Files**: `cmd/lore/lore/commands_test.go` — Create or Modify, per what exists.

## Verification

`make vet`, `make build`, and full `make test` green; `gofmt` clean over the change set. The
exit code is asserted directly in the phase 2 tests, not inferred.

## Open Questions

- [ ] Which option — A, B, or C? B is recommended.
- [ ] Should a manifest that loads but contains zero packages be an error, or is that a valid
      no-op? Current behavior treats it as contributing nothing, which then trips "no packages
      to deploy" if it was the only argument. That seems right, but it is worth an explicit
      ruling since phase 2 pins it.

## Related Documents

- Issue #368 — this bug
- [docs/plans/audit-remediation.md](./audit-remediation.md) — issue #365; found during its phase
  1b review, deliberately kept out of that behavior-preserving decomposition
