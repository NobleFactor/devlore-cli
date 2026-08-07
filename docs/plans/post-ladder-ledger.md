---
title: "Post-Ladder Ledger"
status: in-progress
created: 2026-08-07
updated: 2026-08-07
---

# Plan: Post-Ladder Ledger

The durable ledger of the items chartered after the lint ladder (2,486 findings → 0) and
its follow-on arcs. Thirteen items chartered; eleven closed. **The two open items are the
whole of this plan's remaining work.**

## OPEN — Item 10: the 2.8 diagnose-sweep

**Blocked on the diagnostics API** (`docs/architecture/2.8-eventing-infrastructure.md`,
proposed/under refinement). The settled policy ([ignored errors are diagnostics]): every
deliberately ignored error is marked
`//nolint:errcheck // diagnose-ignored-error: <reason>` until the diagnostics stream
exists, then every marker converts to a real diagnostics emission and the suppression
dies. **Current marker count: 30** (`grep -rn "diagnose-ignored-error" --include="*.go"`
enumerates the work-list). The sweep is mechanical once the API lands; it unblocks the
moment 2.8's eventing infrastructure ships its diagnostics stream.

## OPEN — Item 11: the single codec

**Owner: project owner (design in authoring).** The requirements ledger is
[single-codec.md](single-codec.md): one canonical model; YAML/JSON/protobuf are
renderings; same checksum ⇔ same logical document; PR #298's raw-bytes canonical is
interim. Requirement 1 (the framework owns both directions of graph-document handling)
was ruled and closed 2026-08-04 via `op.SerializeGraphs` (#323). Known
still-unlegislated corners, recorded in
[5-graph-trace-integrity.md](../architecture/5-graph-trace-integrity.md): non-integral
float rendering, int64 beyond float64's 2^53 through `encoding/json`, and
null-versus-absent semantics. Standing offer: a serialization-surface inventory and
divergence-point enumeration on request.

## Closed items

| # | Item | Delivery |
|---|------|----------|
| 1 | Lint gate verdict — file-isolated JSON, hard error | #321, [lint-gate-verdict.md](lint-gate-verdict.md) |
| 2 | `make lint` in quality-gate (belt-and-suspenders) | #321 |
| 3 | `SilenceUsage` under a failing verdict | #321 |
| 4 | Linux-only `unconvert` platform-reasoned suppression | #322 |
| 5 | `emitGraphs` dedup → `op.SerializeGraphs` | #323, [single-codec.md](single-codec.md) Req. 1 |
| 6 | Narrator wiring — dead Discard defaults died, one instance on both seams, Result included | #324, [fix-narrator-wiring.md](fix-narrator-wiring.md) |
| 7 | `.golangci` three-copy lockstep — seed template + drift guard, ops canonical upstreamed | #325/#326, ops#125/#126, [golangci-template-sync.md](golangci-template-sync.md) |
| 8 | Graph and trace integrity — chapter rename, `TracesDir`, reference sweep | #327, [graph-trace-integrity.md](graph-trace-integrity.md) |
| 9 | Receipt trust boundary — verified-side asserts, propagated audit stamp | #329, [receipt-trust-boundary.md](receipt-trust-boundary.md) |
| 12 | golangci v2.12.2 — pin aligned, new analyzers satisfied | #328, [golangci-bump.md](golangci-bump.md) |
| 13 | writ graphs-and-traces guide — receipts fiction retired | #330, [writ-traces-guide.md](writ-traces-guide.md) |

This plan closes when items 10 and 11 do.

[ignored errors are diagnostics]: ../architecture/2.8-eventing-infrastructure.md
