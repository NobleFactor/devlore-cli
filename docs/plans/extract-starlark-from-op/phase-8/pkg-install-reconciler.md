---
title: "Phase 8 · pkg.Provider Composite-router veneer migration (step 21.4 / #6)"
parent: "docs/plans/extract-starlark-from-op/phase-8/21-lore-migration.md"
issue: TBD
status: in-progress
created: 2026-06-04
updated: 2026-06-05
---

# Migrate `pkg.Provider` onto the platform Composite router

## Reframe (2026-06-05): the provider is a pure veneer

The `platform.PackageManager` Composite contract ([`platform-unification.md`](platform-unification.md)) is built and
committed. It moves **all convergence and verification into the leaf driver**: each leaf pre-queries the installed
version, runs the idempotent command, re-queries, and returns one `platform.Receipt`
(`Purl` / `PriorVersion` / `Version` / `Err`) per package — `Err` set from the observed post-state, not the exit
code. The router dispatches a `[]PURL` to leaves by `PURL.Type` and concatenates their receipts.

That makes `pkg.Provider` a **thin veneer** with no convergence policy. Its only job per verb is:

1. **Resource → PURL** — build `platform.PURL{Type: resolveType(plat, r.Type), Name: r.Name, Version: r.Version}`
   for each resource. `resolveType`: `r.Type` non-empty → `plat.ResolvePurlType(r.Type)`; empty →
   `plat.DefaultPurlType()`.
2. **call the router once** — `plat.PackageManager().Install(purls, kwargs) → ([]platform.Receipt, error)`.
3. **adapt** `[]platform.Receipt` → the provider's per-package `[]*pkg.Receipt` compensation state.

The reconciler mechanics in the prior draft are **superseded** — they describe the leaf's per-package behavior,
which already lives in `pkg/platform`.

## A `[]PURL`, not a lazy sequence

The provider materializes a `[]PURL` via a small `toPURLs(rs) []PURL` helper — *not* a Go 1.23 `iter.Seq[PURL]`
generator — because the router's contract is `Install(packages []PURL, …)`: it **batches by `PURL.Type`** (it needs
the whole set to group per-leaf) and returns **one receipt per package in input order** (the caller correlates by
index). The package list is tiny and bounded, so laziness buys nothing; a range-over-func iterator would just be
force-collected at the boundary. (`iter.Seq` is the right tool for large / unbounded / composed streams, not for a
handful of packages feeding a batch API that returns a parallel slice.)

## Contract deltas that drive the change

- **`Update` stays — automatic *and* manual.** Index refresh is **automatic, staleness-gated, per leaf** (rules
  below); there is **also** a manual force-refresh. `PackageManager` gains `Update() error` (the router fans out to
  every leaf, bypassing the gate), and the provider keeps `Update` / `pkg.update` → `plat.PackageManager().Update()`.
  **Neither is built yet** — no leaf carries refresh logic today (the `manager.go` "no public Update" comment was
  aspirational). See *Index update* below; it's the one net-new platform feature inside #6.
- **`Install` / `Remove` / `Upgrade`** adopt `(packages []*Resource, kwargs map[string]any) → (result []*Resource,
  receipts []*Receipt, error)`, mapping 1:1 onto `router.{Install,Remove,Upgrade}([]PURL, kwargs)`.
- **Queries by PURL.** `Installed` / `NotInstalled` / `VersionGTE` / `Observe` and `Resource.Etag` call
  `router.Installed(PURL)` / `router.Version(PURL)` (today they call the removed `PackageManagerByName`).
- **`cask` is an opaque kwarg** passed through to the brew leaf — deletes the `cask bool` positional and
  `runBrewCask`.
- **No `manager` override kwarg** — routing is purely by `PURL.Type`.

## Carried forward (still valid)

- **Version is state, one catalog entry.** `git` and `git@2.39.0` intern to the versionless purl URI; the requested
  version is mutable state on `Resource.Version`.
- **One `Receipt` per package** — the compensable complement is a `[]*pkg.Receipt`, legal per
  `isLegalCompensableComplement` (pkg/op/method.go). No framework change.
- **`kwargs map[string]any`** trailing collector — idiomatic, bound by the bridge's `Parameter.Kwargs` path.

## Index update — automatic, with a manual override

`update` is two things: the **automatic, staleness-gated refresh** every leaf performs as a side effect, and a
**manual force-refresh** verb that bypasses the gate. **Status: both implemented and tested.** The manual
force-refresh is `PackageManager.Update()` + the router fan-out + the per-leaf `refresh` primitives
(apt/dnf/pacman/brew/port). The automatic gate is `driver.ensureFresh`, run before Install/Upgrade/Search/Available;
apt/pacman report index age via mtime (`indexAgeOf`), while brew/dnf self-manage and port defers.

### Automatic refresh — rules
1. **Implicit, never authored.** No plan step refreshes the index; the leaf does it as a side effect of ops that
   need a current catalog. `update` is a behavior, not something a plan author calls.
2. **Staleness-gated, per leaf.** A leaf refreshes only when its index is older than a per-manager TTL, owning both
   the staleness signal and the refresh command: apt → `/var/lib/apt/lists/` mtime → `apt-get update`; pacman →
   sync-db mtime → `pacman -Sy`; brew → defers to its own auto-update, force `brew update` past TTL; dnf → defers to
   `metadata_expire`, `dnf makecache` past TTL; winget / snap / flatpak → native refresh.
3. **Only before index-consulting ops.** Refresh precedes Install / Upgrade / Search / Available; never Remove /
   Installed / Version (local state, no index needed).
4. **At most once per leaf per run.** The first stale-index op triggers one refresh; the leaf marks itself refreshed
   for the run (20 `deb` installs ⇒ one `apt-get update`).
5. **Refresh failure is non-fatal but surfaced.** Offline / locked-db failure doesn't abort — the op proceeds
   against the existing index and the condition is recorded.
6. **TTL is a knob.** Per-manager default, overridable for always-fresh (CI) / never-refresh (air-gapped).

### Manual force-refresh — `PackageManager.Update()`
- Add `Update() error` to the `PackageManager` interface; amend the `manager.go` doc comment that currently says
  "there is no public Update."
- Leaf: refresh now, bypassing the staleness gate (the same refresh command, ungated).
- Composite router: fan out `Update()` to every leaf; aggregate per-leaf failures into the returned error.
- `pkg.Provider.Update` → `plat.PackageManager().Update()`.

## Design per file (all in `pkg/op/provider/pkg` unless noted)

### `resource.go`
- Add `Version string` — the requested version (from the purl `@version`; empty ⇒ latest).
- Migrate `Etag()` to `plat.PackageManager().Version(platform.PURL{Type: r.Type, Name: r.Name})`.
- Fix drift docs (they reference a non-existent `Observation` / `Observe`).

### `helpers.go`
- Delete `resolvePlatformManagerForInstall/Upgrade/Remove` and `runBrewCask`.
- Add `resolveType(plat, type) string`, `toPURL(plat, *Resource) platform.PURL`, `toPURLs(plat, []*Resource)
  []platform.PURL`.

### `provider.go`
- Rewrite `Install` / `Remove` / `Upgrade` and the three `Compensate*`: build `[]PURL`, call the router verb, adapt
  receipts, set `result[i].Type` from the receipt's resolved purl type.
- Rewrite `Installed` / `NotInstalled` / `VersionGTE` / `Observe` to query the router by PURL.
- Rewrite `Update` to call `plat.PackageManager().Update()` (manual force-refresh).

### `receipt.go`
- Per-package shape — `op.ReceiptBase` (affected Resource + TransactionID) + `Manager string` +
  `InstalledBefore bool` + `PreviousVersion string`, built from one `platform.Receipt`.
- Drop `Packages` / `Cask` / `AlreadyInstalled` / `PreviousVersions`; shrink the marshalers.

### `*.gen.go`
- **Regenerate** (never hand-edit): the announced mutator signatures change and `cask` drops; `Update` keeps its
  slot (now backed by the router). The kwargs marker matches the `plan.Plan` precedent.

### `cmd/lore/lore/builder.go`
- `addNativeSoftwarePackages` drops the `cask` positional and adopts the kwargs contract; the gating test
  `TestBuildPackage_NativePackageProducesParentedPhaseSubgraph` goes green.

## Receipt adaptation (`platform.Receipt` → `pkg.Receipt`)

Per package `i`:

```go
pkg.Receipt{
    Resource:        result[i],                       // via op.ReceiptBase
    Manager:         receipts[i].Purl.Type,
    InstalledBefore: receipts[i].PriorVersion != "",
    PreviousVersion: receipts[i].PriorVersion,
}
```

`Compensate*` replay it:
- `CompensateInstall` — `!InstalledBefore` ⇒ `Remove`; else if the installed version drifted ⇒ reinstall at
  `PreviousVersion`.
- `CompensateRemove` — reinstall every `InstalledBefore` package at its `PreviousVersion`.
- `CompensateUpgrade` — restore `PreviousVersion` (best-effort; the cross-manager downgrade caveat is unchanged).

## Implementation order

0. `pkg/platform` — add `PackageManager.Update() error` (interface + composite fan-out + per-leaf force-refresh) and
   the automatic staleness-gated refresh in each leaf (the rules above); amend the `manager.go` doc comment. This is
   the prerequisite for the provider's `Update` verb. (Tracked as task #9.)
1. `resource.go` — add `Version`, migrate `Etag`, fix docs (unblocks the rest).
2. `receipt.go` — per-package shape + marshalers.
3. `helpers.go` — `resolveType` + `toPURL[s]`; delete the old resolvers + `runBrewCask`.
4. `provider.go` — verbs + compensators + predicates + `Observe`; drop `Update`.
5. `make generate` — regenerate the gen file.
6. `provider_test` / `receipt_test` — new shapes.
7. `cmd/lore/lore/builder.go` consumer + the gating behavioral test.
8. `make check` green for `pkg/op/...` and `cmd/lore/...`.

## Decisions & open items

- **`Update()` routing — decided: fan out to every leaf.** A box's `update` freshens all indexes; the native
  package-manager set is small and fixed, so refresh-all is cheap. A manager/type filter can be added later if
  wanted — not now.
- **`cask`** — confirm during step 0 whether the brew leaf already reads `kwargs["cask"]`; if not, that's a small
  *leaf* addition folded in (the only extra `pkg/platform` touch).
- **`Upgrade` = `router.Upgrade`** (force-to-latest per package), distinct from `Install` of a versionless purl.

## Status

- 2026-06-05 — settled. Composite-veneer shape; `Update` retained (automatic gated refresh + manual fan-out
  force-refresh — task #9). Ready for implementation (#9 → #6); no code written yet.
- 2026-06-06 — #9 Part A landed: `PackageManager.Update()` (manual force-refresh) + router fan-out + per-leaf
  `refresh` (apt/dnf/pacman/brew/port) + a hang-proof `runShellCommand` (timeout, `sudo -n`, `yes`-stdin,
  `DEBIAN_FRONTEND`). `pkg/platform` tests rewritten to the new API and green, with `Update`/`refresh` coverage
  (router fan-out + per-leaf command/sudo). Remaining: #9 Part B (the automatic staleness gate), then the #6 veneer.
- 2026-06-06 — #9 Part B landed and committed (cd926310): the automatic staleness gate (`driver.ensureFresh` before
  index-consuming ops; apt/pacman report index age by mtime, brew/dnf/port defer). **#9 is complete.** Remaining: the
  #6 `pkg.Provider` veneer + consumer migration.
