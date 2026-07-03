---
step: 20
former_step: 18.5
title: "pkg.Provider — thin veneer over the Composite package-manager router"
status: complete — #6 closed 2026-06-07; the whole pkg/op tree green
proof_run: 2026-06-07
parent: ../../phase-8.md
---

# Step 20 — pkg.Provider veneer over the router (formerly 18.5)

**Status:** `complete`. `pkg.Provider` is a thin veneer over the Composite router
(`pkg/op/provider/pkg/provider.go:13`); `Install` / `Remove` / `Upgrade` / `Installed` / `Version` / `Update` all
delegate to `plat.PackageManager()` routing by purl — no `manager` argument survives on the surface. Issue #6 closed
2026-06-07 (provider/pkg + provider/service).

## Design and history

The migration plan and its phasing live in [phase-8/pkg-install-reconciler.md](../pkg-install-reconciler.md). That
document's title carries the pre-18.5 numbering lineage ("step 21.4 / #6") — a stale-lineage rewrite target tracked
by the 2026-07-03 consistency audit.
