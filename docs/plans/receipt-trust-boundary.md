---
title: "Receipt Trust Boundary"
status: complete
created: 2026-08-07
updated: 2026-08-07
---

# Plan: Receipt Trust Boundary

## Summary

Item 9: scrutiny of the resume path's `receipt.Commit` handling and the restore chain's
compliance with the settled checksum trust boundary (verified-side decode asserts; the load
boundary errors). Three findings, ruled 2026-08-07; one positive finding needed no change
(pause/cancel receipts round-trip safely — a zero UUID serializes as the all-zeros string,
which parses back).

## Findings and dispositions

### 1. Provider Restore split — aligned to assert

`encryption` and `git` receipts error-returned on catalog-lookup, type, and base-restore
failures where `file`/`service`/`pkg` assert. All five run on verified-trace data. Both now
assert; the error return survives only for the missing-environment caller contract.

### 2. Recovery-stack reconstruction — moved to the assert side

`fromEntries` (bare-receipt base restore) and `reconstructReceipt` (companion-type
resolution, shape checks, encoded restore) asserted; both signatures dropped their
structurally-impossible error returns and callers simplified. `rearmEntry` keeps its error
(retyping and nested re-arms have live error sources).

### 3. The swallowed audit-stamp Commit — propagated

`pushAuditReceipt` ignored `receipt.Commit`'s error under a mislabeled annotation. The
analysis that settled the ruling: `Commit` mints the transaction id first and returns on
failure before recording the result and compensator, so a swallowed failure pushes an empty
receipt — a claimed success whose side effect has no undo information, a silent hole in the
recovery chain found only when compensation quietly skips it. The failure itself is
`uuid.NewV7`'s random read; on Go 1.24+ the default `crypto/rand` reader never returns an
error, so the path is effectively unreachable — but when it fires, failing honestly beats
hiding the same orphan. `pushAuditReceipt` now returns the error: failure exits join it
onto the exit error (`joinAuditStamp`, extracted after the inline joins pushed
`Subgraph.Execute` past the gocognit gate), success exits fail the unit.

## Verification

Suite green (no test exercised the converted error paths); uncapped board zero on Darwin
and `GOOS=linux`.
