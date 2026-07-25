---
title: "Whitespace lint: fix the config bug, then the 22 real findings"
issue: TBD
status: complete
created: 2026-07-25
updated: 2026-07-25
---

# Plan: Whitespace lint — fix the config bug, then the 22 real findings

Chartered follow-up 4, part a (whitespace), of
[fix-windows-build-and-guide-category](fix-windows-build-and-guide-category.md).

## Summary

The uncapped lint enumeration reported 2,027 `whitespace` findings. Classification showed
2,005 are **false positives caused by a config bug**: the linter's "unnecessary leading
newline" check flags the blank line after a function signature that NobleFactor style
*requires*. The config's `multi-func: true` comment claimed it protected that blank line;
it does not — it only requires a newline after multi-line signatures. The 22 real
findings: 16 "multi-line statement should be followed by a newline" (missing the mandated
blank after multi-line signatures) and 6 "unnecessary trailing newline".

An initial repo-wide autofix ran under the buggy config and stripped the 2,005 mandated
blank lines; that damage is reverted by the commit script (`git restore -- '*.go'`)
before the corrected fixer runs.

## Changes

1. `.golangci.yaml` — exclusion rule suppressing the whitespace linter's
   "unnecessary leading newline" check; corrected the `multi-func` comment to state what
   the option actually does.
2. `.golangci.yaml` — pre-existing v1-style `output` keys (`print-issued-lines`,
   `print-linter-name`, `sort-results`) migrated to the v2 schema
   (`golangci-lint config verify` failed on them; it now passes).
3. The 22 real whitespace findings fixed by `golangci-lint run --fix
   --enable-only=whitespace` under the corrected config (runs inside the commit script,
   after the revert).

## Verification

- `golangci-lint config verify` — pass.
- In-script, before commit: whitespace findings = 0 (uncapped), `gofmt -l` clean over
  the repo, `make test` passes.

## Chartered follow-ups (not in this change)

1. **Upstream template**: `.golangci.yaml` is copied from the shared NobleFactor
   template ("Copy this file to any Go repo root"); the template source (noblefactor-ops)
   needs the same exclusion, comment, and v2 output-key corrections.
