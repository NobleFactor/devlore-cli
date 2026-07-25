---
step: 19
former_step: 18.4
title: "Platform unification — op-free platform.Platform + Composite platform.PackageManager routing by purl; op duplicate deleted"
status: complete — consumer migration landed + op duplicate deleted 2026-07-04
proof_run: 2026-07-04
parent: ../../phase-8.md
---

# Step 19 — Platform unification (formerly 18.4)

**Status:** `complete`. Everyone is consolidated on the op-free `platform.Platform` per the resolved 2026-06-04
contract, and the `pkg/op` duplicate surface is deleted.

## Contract (per the resolved direction, 2026-06-04)

The settled contract lives in [phase-8/platform-unification.md](../platform-unification.md) §"Resolved direction &
contract": `pkg/platform` is a **standalone op-free capability** that `pkg/op` *imports* (the same shape as
`pkg/result` / `pkg/status`) — `platform.Platform` (interface) + `platform.PackageManager` (the Composite router,
purl-routed, `platform.Receipt`-returning) + named `*Spec` factories / `Detect()` / `New(spec)`. The duplicate
`op.Platform` struct and its string-based manager interfaces were chartered for **deletion** (resolved item 4).

**This step doc's previous frontmatter was wrong on both counts:** it described the `pkg/op/platform.go` struct as
"the contract" (it was the doomed duplicate), and named the remaining consumers as `cmd/` (those were already
migrated — the actual remainder was `internal/lorepackage`).

## Closed 2026-07-04

1. **Consumer migration** — the last old-surface consumers were exactly the plan's phase-4 sites, migrated this
   parcel:
   - `internal/lorepackage/package.go` (`VerifySyntheticPackage`) — `op.NewPlatform().PackageManager.Available(name)`
     → `platform.Detect()` + `platform.New(spec)` + `router.Available(platform.PURL{Type: host.DefaultPurlType(),
     Name: …})`, nil-router/detect-error handled (behavior-preserving: queries the default native manager).
   - `internal/lorepackage/search.go` (`searchNative`) — the router's `Search` fans out across leaves; each hit
     self-identifies via `SearchResult.Manager` (the contract's designed replacement for the dropped `Name()`), so
     the source mapping and the `Installed`/`Available` follow-ups are now per-hit purl-typed.
   - `internal/lorepackage/search.go` (`ResolveWithConfidence`) — same default-manager `Available` shape as site 1.
   - `pkg/op/provider/platform/provider.go` — code was already on the interface; its doc comment still said
     `[op.Platform]`; corrected.
2. **The op duplicate deleted** (resolved item 4's list, 11 files): `pkg/op/platform.go`, `pkg/op/purl.go`,
   `platform_{linux,darwin,windows}.go` + their `_panic.go` companions, `platform_new.go`, `platform_helpers.go`,
   `platform_test.go`. Verified consumer-less before deletion: qualified `op.<type>` sweep across `pkg`/`cmd`/
   `internal` and unqualified sweep inside `pkg/op` both return only the doomed files themselves (plus the two
   flagged items below).
3. **Verification** — `make test`: 87 ok / 20 no-tests / 7 red — the red set is byte-identical to the step-18 gate
   inventory (writ family + docgen + e2e → step 33; shell completion → step 28); `internal/lorepackage` passes.

## Flagged, not unilaterally fixed

1. **`internal/execution/provider_test.go`** — `//go:build ignore`, so it compiles nowhere, but its mocks reference
   the now-deleted `op.ServiceManager` / `op.PlatformResult`. Dead debris; owned by the lore-migration/step-33
   consumer arc. Delete or rewrite there.
2. **`pkg/platform/detect_linux.go:14` imports `pkg/iox`** — violated only the letter of the resolved contract's
   parenthetical ("imports no devlore package" / "imports only the stdlib" — settlement-time observations, not the
   property). **Resolved 2026-07-04:** the contract wording was corrected to the meaningful property — `pkg/platform`
   depends on `pkg/op` neither directly nor transitively; leaf devlore utilities like `pkg/iox` (stdlib-only:
   `errors`, `io`) are fine. Verified the same day: every package under `pkg/` outside the op subtree is op-free in
   this sense (the sole textual `pkg/op` match, `pkg/assert/assert.go:224`, is a doc-comment example, not an
   import). No code change.

## Related docs

Design + resolved contract: [phase-8/platform-unification.md](../platform-unification.md) (completion record
appended 2026-07-04). Status: [3.4-platform-package-managers.status.md](../../../architecture/3.4-platform-package-managers.status.md)
(stale "remaining broken consumers" line corrected in this parcel).
