---
title: "Writ Secret Encrypt"
status: proposed
created: 2026-08-09
updated: 2026-08-09
---

# Plan: Writ Secret Encrypt

## Summary

`writ secret encrypt <file>...` — the user-facing front for the shipped
`encryption.encrypt_file` action, pulled forward (ruled 2026-08-09) so the personal-repo
SOPS migration runs through writ itself. The action already discovers the `.sops.yaml`
governing each source (walking up from the file, then the XDG fallback) and carries
generic key groups — age and Azure Key Vault both flow through untouched. Interactive
editing stays with `sops edit`; the everyday authoring path is sops, age, or — we hope —
writ. The provider is also surfaced in Starlark (agreed 2026-08-09) as a scripting
surface, not the everyday path.

## Design

1. **Command**: `secret` parent under writ with `encrypt` as its first subcommand
   (`cmd/writ/writ/secret_cmd.go`), `MinimumNArgs(1)` file arguments.
2. **Per file**: resolve the absolute path; the output is the `<file>.sops` sibling —
   matching the deploy convention (`foo.env.sops` deploys as `foo.env`). An existing
   output refuses loudly (no `--force` until a need is shown). The plaintext source is
   never deleted — writ performs no hidden destructive operations; removal belongs to
   the caller (`git rm` in the migration sweep).
3. **Execution**: spec-based graph construction through `plan.Provider` — one
   `encryption.encrypt_file` unit per file — executed and persisted through the standard
   writ pipeline: graph and trace in the execution store, receipts recorded
   (compensation removes the ciphertext).
4. **No-rule failure**: when no `.sops.yaml` creation rule governs a source, the
   encrypter's existing error surfaces verbatim
   ([encrypter.go:144](../../pkg/sops/encrypter.go)) — configuration is authored, never
   inferred.
5. **Starlark**: expose the encryption sub-namespace through the starlarkbridge adapter,
   same pattern as the existing planable providers.

## Verification

1. Unit tests: output naming, existing-output refusal, no-governing-rule error, multiple
   files in one invocation.
2. Round-trip: `writ secret encrypt` output decrypts byte-identically through the
   embedded client (age fixture, the pattern of the existing provider tests).
3. `make test`, dual-GOOS lint recount at zero.
4. Live proof: the personal-repo migration sweep (sops-migration plan, phase 3).

## Open questions

None — scope is deliberately minimal; `rekey` is chartered separately
([writ-secret-rekey](writ-secret-rekey.md)).
