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
[phase-8/platform-unification.md](../platform-unification.md). Its internal section headers were rewritten
to the current numbering in the 2026-07-03 audit (group 4).

Related status: [3.4-platform-package-managers.status.md](../../../architecture/3.4-platform-package-managers.status.md)
(rewritten to the current numbering in the same audit, group 4).

## Remaining to reach `complete`

1. Consumer migration in `cmd/` onto the routed `op.PackageManager` surface.

