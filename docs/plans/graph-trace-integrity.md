---
title: "Graph and Trace Integrity"
status: complete
created: 2026-08-07
updated: 2026-08-07
---

# Plan: Graph and Trace Integrity

## Summary

Item 8 of the post-ladder ledger: `docs/architecture/5-receipt-integrity.md` predated the
settled checksum trust boundary and the landed signing design, and its "receipt" framing
collided with the settled terminology (receipt = compensation datum; the persisted run
documents are the graph and its per-execution traces). Ruled 2026-08-07: rename the chapter
and rebase its terminology on what exists — a graph, a trace per execution, drift detection
and reconciliation over the per-graph trace stack — and rename the on-disk traces directory
to match.

## Changes

### 1. Chapter rename and rewrite

`5-receipt-integrity.md` → `5-graph-trace-integrity.md`, titled "Graph and Trace Integrity",
with the `.status.md` companion renamed alongside. The two settled sections (document
integrity table + format-leak rules; the checksum trust boundary) carry over intact. Stale
content replaced: tier 2 now describes the landed ssh-ed25519 model (`pkg/signing` —
namespace-prefixed canonical bytes, publisher key in OpenSSH wire format, verifier-owned
`allowed_signers`, best-effort signing at persist) instead of age/SOPS; storage now
describes the execution store (graphs keyed by checksum, per-graph trace stacks, run
index) instead of flat timestamped receipts; verification points at the real load funnel.
Deleted outright (greenfield): the v1–v4 version-history table, the age-claim signature
text, the SOPS key-type roadmap, and the pre-graph canonical sample. A terminology note
records the retirement of "receipt" for persisted documents.

### 2. On-disk rename: receipts → traces

Traces live at `${XDG_STATE_HOME:-~/.local/state}/devlore/traces` (ruled 2026-08-07;
`XDG_STATE_HOME` resolution confirmed over an initial `XDG_CACHE_DIR` slip).
`internal/cli/receipts.go` → `internal/cli/store.go` (the file owns the whole execution
store: graphs and traces); `ReceiptsDir` → `TracesDir`; callers in writ readback and the
three integration tests renamed; no store migration — greenfield.

### 3. Reference sweep

Every inbound reference updated: 12 markdown files (architecture chapters, index,
package-reference, historical plan docs) and 4 Go files whose comments cite the chapter
(`pkg/op/trace.go`, the file/service/pkg provider receipt files). Zero references to the
old name remain outside the deleted files.

## Verification

Suite green, dual-GOOS lint at zero, no `5-receipt-integrity` or `ReceiptsDir` references
outside the files the landing commit deletes.

## Chartered follow-on

`docs/guides/writ/receipts.md` (the user guide) likely carries the same stale framing —
separate item, not folded here.
