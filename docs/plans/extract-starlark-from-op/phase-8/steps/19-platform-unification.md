---
step: 19
former_step: 18.4
title: "Platform unification — op.Platform struct + Composite op.PackageManager routing by purl"
status: in-progress — contract landed; consumer migration (cmd/) remaining
proof_run: 2026-07-03 (registration audit)
parent: ../../phase-8.md
---

# Step 19 — Platform unification (formerly 18.4)

**Status:** `in-progress`. The contract landed: `op.Platform` (`pkg/op/platform.go:9`, a concrete struct — the design
reversed the earlier interface flip) plus the `op.PackageManager` Composite router (`platform.go:65`) that routes by
purl and fans out to leaf drivers; `pkg/platform` is fully reshaped and style-compliant. Remaining: consumer
migration in `cmd/`.

## Design and history

The full design, contract resolution, and phasing live in
[phase-8/platform-unification.md](../platform-unification.md). That document's internal section headers still carry
the pre-18.4 numbering lineage ("step 21.4") — a stale-lineage rewrite target tracked by the 2026-07-03 consistency
audit, not a divergence in substance.

Related status: [3.4-platform-package-managers.status.md](../../../architecture/3.4-platform-package-managers.status.md)
(also on the stale-lineage rewrite list).

## Remaining to reach `complete`

1. Consumer migration in `cmd/` onto the routed `op.PackageManager` surface.
2. The stale-lineage rewrite of the two documents above (audit follow-up, not implementation work).
