---
title: "Phase 8 · pkg.Provider Install/Remove/Upgrade reconciler redesign"
parent: "docs/plans/extract-starlark-from-op/phase-8/21-lore-migration.md"
issue: TBD
status: draft
created: 2026-06-04
updated: 2026-06-04
---

# Redesign `pkg.Provider` install/remove/upgrade as a reconciler

## Reframed by the Composite model (2026-06-04)

The platform Composite model ([`platform-unification.md`](platform-unification.md)) relocates most of what this
plan originally placed in `pkg.Provider`:

- **`pkg.Provider` is a thin veneer**, not a reconciler. It adapts the starlark/graph call and hands the package
  list to the platform's `op.PackageManager` **composite router**; it holds no convergence policy.
- **Convergence + verification live in the leaf driver** (`apt`, `npm`, …): pre-query → run (idempotent) →
  post-query → one `Receipt` per package, erroring the receipt if the post-state did not reach the purl's request.
- **Routing is by purl; there is no `manager` kwarg** (dropped). The manager rides in each `Resource.Type`,
  normalized with the platform default at construction.
- **Failure handling is the framework's** — best-effort per package, `Degraded` via a consumer `flow.Degraded`
  error action, `Failed`/`Stranded` via unwind — specified in
  [`compensation-failure-contract.md`](compensation-failure-contract.md), not here.

**Carried forward** (still valid, now as inputs to the leaf-driver design): version-as-state with one versionless
catalog entry; **one `Receipt` per package**; `kwargs` as opaque native-installer flags; **keep `Upgrade`** (the
Install/Remove/Upgrade triad). The detailed reconciler-in-`pkg.Provider` mechanics below are **superseded** by the
leaf + composite split — read them as the leaf driver's per-package behavior.

## Summary

`pkg.Provider` is being exercised for the first time by the Part B `cmd/lore` migration, and the behavioral
test (`TestBuildPackage_NativePackageProducesParentedPhaseSubgraph`) surfaced that the surface is stale:
`Install(packages []*Resource, manager string, cask bool)` carries a required positional `cask` flag, a
positional `manager`, and a single batch `Receipt`. A lot has changed since that code was written — the
package model is now purl-based, the platform exposes a cross-platform `PackageManager`, and the framework's
compensable machinery accepts a slice of receipts.

This plan **guts and reimplements the compensable triplet** (`Install` / `Remove` / `Upgrade`) as a
**reconciler**: the consumer hands a list of package resources and the provider converges each package to its
requested state — install when absent, upgrade when older, downgrade when an earlier version is pinned, no-op
when already satisfied. The shape is fluid from the consumer's side: *give me a list, and `Install` ensures the
request is satisfied.*

New signature (all three mutators adopt it):

```go
func (p *Provider) Install(packages []*Resource, kwargs map[string]any) (result []*Resource, receipts []*Receipt, err error)
func (p *Provider) CompensateInstall(receipts []*Receipt) error
```

`kwargs` are **pass-through arguments to the native package installer** (apt/brew/dnf/winget flags). Details
vary enormously by manager; we assume consumers know what they're doing. Cross-platform option abstraction is
explicitly **out of scope** for now — `platform.PackageManager` is **not touched**.

## Settled decisions (2026-06-04)

1. **Version is state, not identity — one catalog resource.** Versionless `git` and pinned `git@2.39.0` are
   the **same** catalog entry (identity = the versionless purl URI). The requested version is mutable state
   the reconciler converges to, exactly like a file resource: path = identity, content = state.
2. **Keep `Upgrade`.** It is retained as a distinct verb (not collapsed into `Install`) and reshaped to the
   new signature for consistency. See *Open question 1* for its distinct semantics.
3. **Reshape `Remove` in lockstep** with `Install` (same signature, per-package receipts). See *Open
   question 2* for the kwargs-flag limit imposed by `PackageManager.Remove(name string)`.
4. **One `Receipt` per package** — each receipt records a single package's pre-action state.

## Gate findings (verified against the code, not assumed)

- **`[]*Receipt` is a first-class compensable complement.** `isLegalCompensableComplement` (pkg/op/method.go:830)
  accepts a `Receipt`, **a slice of `Receipt`**, or a `*RecoveryStack`. So `(result []*Resource, receipts
  []*Receipt, err error)` is a legal `MethodCompensableFunction` (T, U, error) with U = the receipt slice, and
  the saga captures the slice and replays it into `CompensateInstall(receipts []*Receipt) error`. **No
  framework change.**
- **The `kwargs map[string]any` collector is supported and idiomatic.** `plan.Provider.Plan`,
  `flow.Provider.Failed/Degraded` already use a trailing `kwargs map[string]any`; the bridge binds it via the
  `Parameter.Kwargs` path (`go_receiver.go`). The pkg provider marks its kwargs parameter the same way.
- **Version-pinned installs and native flags ride the existing varargs.** Every manager's `Install(...string)`
  shells out to `<binary> install … <joined>` (e.g. `apt-get install -y …`), so formatted specs (`git=2.39.0`)
  and native flags (`--allow-downgrades`) pass through positionally — no `PackageManager` change.

## Design

### `pkg.Resource` — add the requested version (decision 1)

- Add `Version string` — the **requested** version (from the purl's `@version`; empty ⇒ latest). This is the
  desired state the reconciler converges to.
- `buildCandidate` interns the **versionless** purl as the URI (`Type` + `Name`, version stripped), so
  `git` and `git@2.39.0` resolve to one catalog entry; it then sets `Resource.Version = purl.Version`.
- The **installed/observed** version remains live via `Etag()` / `mgr.Version(name)` — unchanged. `Version`
  (requested) and `Etag()` (installed) are distinct: the reconciler compares them.
- **Fix the drift docs.** Today's comments describe a `Version` field, an `Observation`, and a
  `Provider.Observe` that don't exist, and claim `Resolve()` populates the *installed* version. Rewrite them to:
  `Version` = requested; installed version = live `Etag()`; drop the dead `Observation`/`Observe`/`Resolve`-
  populates-installed language.
- **Same-package-two-versions in one session is a plan conflict, by design** — consistent with the existing
  same-resource production rule. The versionless URI interns once; conflicting requested versions are a plan
  error to fix, not a framework concern.

### `pkg.Receipt` — per-package pre-action state (decision 4)

Replace the batch receipt with a per-package one:

```go
type Receipt struct {
    op.ReceiptBase          // affected Resource + TransactionID; identifies the one package
    Manager         string  // the manager that performed the action
    InstalledBefore bool    // was this package installed before the action
    PreviousVersion string  // installed version before the action ("" if absent)
}
```

Dropped: `Packages []string`, `Cask bool`, `AlreadyInstalled []string`, `PreviousVersions map[string]string`
— all subsumed by one-receipt-per-package + the embedded `Resource`. `MarshalJSON`/`MarshalYAML` and the
unmarshal hooks shrink to the new fields.

### Reconciler semantics — `Install`

For each package resource:

1. Resolve the manager: `kwargs["manager"]` override → else `Resource.Type` → else platform default.
2. Observe pre-state: `installed := mgr.Installed(name)`, `previous := mgr.Version(name)`.
3. Build the per-package `Receipt{Manager, InstalledBefore: installed, PreviousVersion: previous}`.
4. Converge:
   - **requested == "" (latest):** delegate to `mgr.Install(formatFlags(kwargs)…, name)` — install-or-upgrade-
     to-repo-latest, naturally idempotent. (We have no "latest-available" query, so we do **not** compare; we
     delegate.)
   - **requested pinned, installed == requested:** no-op.
   - **requested pinned, installed != requested (or absent):** `mgr.Install(formatFlags(kwargs)…,
     "name=version")` — upgrade or downgrade. (Downgrade is **not autonomous** — see *Open question 3*.)
5. Set `result[i].Type = mgr.Name()`; collect the receipt.

`CompensateInstall(receipts)`: per receipt — if `!InstalledBefore`, `mgr.Remove(name)`; else if
`PreviousVersion` differs from the now-installed version, reinstall `name=PreviousVersion`.

### `Remove` (reshaped, decision 3)

Same signature. Per package: record pre-state, `mgr.Remove(name)`, collect receipt.
`CompensateRemove(receipts)`: reinstall every package that was `InstalledBefore` (at `PreviousVersion` when
known). **Limit:** `PackageManager.Remove(name string)` takes a single name and no varargs, so native-flag
pass-through is **not** available on `Remove` — only the `manager` kwarg override is honored. Native uninstall
flags wait for a `PackageManager` change (deferred). *Open question 2.*

### `Upgrade` (kept, reshaped — decision 2)

Same signature. Distinct intent (proposed): upgrade named packages to the manager's latest **regardless of any
requested pin** — `mgr.Install(name)` to latest, recording `PreviousVersion`. `CompensateUpgrade` restores
`PreviousVersion`. *Open question 1* confirms the intended distinction from `Install(latest)`.

### kwargs → native flags

`kwargs` carries native installer arguments plus one reserved key:

- `manager` (reserved) — selects the manager; never emitted as a flag.
- `cask` and all other keys — formatted as flags and appended to the varargs ahead of the package specs:
  `true` ⇒ `--key`; `false` ⇒ omitted; scalar ⇒ `--key=value`.

This **deletes the `cask bool` positional and the `runBrewCask` special path** — `cask` is now just a brew
kwarg (`--cask`). *Open question 4* confirms the flag-formatting convention.

## Implementation steps

1. **`resource.go`** — add `Version`; versionless URI in `buildCandidate`; fix the drift docs.
2. **`receipt.go`** — per-package shape; update marshal/unmarshal.
3. **`helpers.go`** — `formatFlags(kwargs)` + manager resolution from kwargs; delete `runBrewCask`,
   `resolvePlatformManagerForInstall`'s cask coupling, and `packageNames` if unused.
4. **`provider.go`** — rewrite `Install`/`Remove`/`Upgrade` + the three `Compensate*` companions to the new
   signature and per-package receipts.
5. **`gen/provider.gen.go`** — **regenerate** (never hand-edit); the announced signatures + the kwargs
   parameter marker change. Confirm the kwargs marker matches the `plan.Plan` precedent.
6. **`provider_test.go` / `receipt_test.go`** — update for the new shapes.
7. **`cmd/lore/lore/builder.go`** — `addNativeSoftwarePackages` drops the `cask` argument it currently passes
   and adopts the new `pkg.install` contract; the Part B behavioral test goes green.

## Open questions (confirm before/while implementing)

1. **`Upgrade`'s distinct semantics.** Proposed: force-to-latest, ignoring the requested pin. It overlaps
   `Install` when the request is already versionless — is that the intended distinction, or something else?
2. **`Remove` kwargs limit.** `PackageManager.Remove(name)` blocks native-flag pass-through. Accept the limit
   (only `manager` honored on `Remove`) for this pass?
3. **Downgrade is consumer-driven.** The reconciler detects downgrade-needed and formats `name=version`, but
   success requires the consumer's kwargs flags (`--allow-downgrades` on apt). Document as-is under "consumers
   know what they're doing"?
4. **kwargs flag-formatting convention** — `true`⇒`--key`, `false`⇒omit, scalar⇒`--key=value`, `manager`
   reserved. Acceptable, or do you want raw pass-through?

## Testing

- `pkg.Provider`: install-absent, upgrade-older, downgrade-pinned (with flag), no-op-satisfied, latest-
  delegate; per-package receipts; `CompensateInstall`/`Remove`/`Upgrade` round-trips. Receipt marshal round-trip.
- `cmd/lore`: `TestBuildPackage_NativePackageProducesParentedPhaseSubgraph` green (the gating consumer).
- `make check` green for `pkg/op/...` and `cmd/lore/...`.

## Status

- 2026-06-04 — draft, awaiting review. No code written.