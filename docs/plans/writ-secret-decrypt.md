---
title: "Writ Secret Decrypt"
status: proposed
created: 2026-08-10
updated: 2026-08-10
---

# Plan: Writ Secret Decrypt

## Summary

`writ secret decrypt <file>...` — the everyday authoring-side read, ruled 2026-08-10:
**stdout only**. No output flag, no sibling files — plaintext written beside a `.sops`
document inside the layer tree would sit one `git add` from the repository, the exact
hazard the migration removes. Decryption uses the embedded client's ambient keyservices
(config-free; under BYOK every read is a Key Vault unwrap and lands in the vault's audit
log), which is the boundary against [recover](writ-secret-recover.md) —
supplied-material-and-offline versus ambient-and-audited. Editing stays with `sops edit`;
decrypt is read-only inspection. Completes the family's everyday story: writ writes,
writ reads.

## Design

1. **Surface**: `writ secret decrypt <file>...` — layer-scoped like its authoring
   siblings (every argument inside a registered layer); multiple files stream to stdout
   in argument order.
2. **Pipeline-free**: an effectless read mints no graph, no trace, no receipt — the
   pipeline exists for effects. (The vault's own unwrap log is the audit record.)
3. **Implementation**: a thin front over the embedded client's existing decrypt path —
   the code deploy exercises daily; no provider or `pkg/sops` growth.
4. Replaces `sops -d` in the migration's verification steps (validation round-trips,
   the cutover byte-match), making the drills writ-native.

## Verification

1. Round-trip against `writ secret encrypt` fixtures (age pattern); containment
   refusal; multi-file ordering.
2. `make test`; dual-GOOS lint recount at zero.

## Open questions

None — scope is deliberately minimal; anything beyond a stdout read belongs to deploy,
recover, or `sops edit`.
