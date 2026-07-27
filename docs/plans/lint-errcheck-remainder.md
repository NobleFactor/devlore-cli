---
title: "errcheck: close out the remaining 61 findings"
issue: TBD
status: complete
created: 2026-07-26
updated: 2026-07-26
---

# Plan: errcheck — close out the remaining 61 findings

Chartered follow-up 4, part b-2, of
[fix-windows-build-and-guide-category](fix-windows-build-and-guide-category.md). Applies the
two rulings settled 2026-07-26: the **checksum trust boundary** (disk-document corruption
before checksum verification is an error; any read failure after verification is a panic)
and **ignored errors are diagnostics** (spec section added to
`docs/architecture/2.8-eventing-infrastructure.md`, canonical search string
`diagnose-ignored-error`, which ships in this change).

## The audit that drove the split

- `op.LoadGraph` recomputes and compares the graph checksum on load (`pkg/op/graph.go`),
  so every loaded `*op.Graph` is post-verification: graph-annotation decodes panic on
  mistype (absence stays contractual).
- Trace documents are read by `cli.LoadTrace` = `document.ReadFile` — **no verification**
  (signing exists but is only checked by `writ verify`), so trace/receipt field decodes
  are pre-verification: mistype is a decode **error**, never a panic.
- `ReceiptSpec.WithRecovery` audit (from 4b-1): **passes** — all five callers pass a
  just-minted archive ID; the deserialization path parses `recovery_id` with error returns.

## Changes (61 findings → 0)

1. **assert.Type conversions (10)** — post-verification decodes: the four guaranteed-type
   rows (command_tree/config variables, two starlark kwarg keys), five graph-annotation
   sites (`origin.go` platform, decommission/upgrade/deploy-plan/readback run/target
   roots — absent-guarded, mistype panics), and readback's `stringField` helper.
2. **Explicit comma-ok (9)** — documented-tolerant sites: current-command override,
   Compensator capability probe, recovery-entry variant probes (×2), brew cask kwarg,
   migrate binding resolve, `Application.DryRun`, deploy targets filter, `NewGraph`'s
   OriginBase carrier.
3. **Trace-decoder error side (5 findings)** — `file`/`pkg` receipt `stringField`/
   `boolField` now return an error on present-but-mistyped fields (absence still yields
   the zero value per contract); both `RestoreEncoded` bodies propagate; `service`
   receipt's two bool fields check inline.
4. **`diagnose-ignored-error` markers (31)** — best-effort cleanup, error-path recovery,
   stream writes, and probes, each with a terse reason and the spec link.
5. **Restructures (6)** — Sscanf uses its error; viper probe via stdlib `errors.As`;
   devconfig `Items` asserts its just-enumerated names; `fsroot.makePath` and appnet's
   URL unescape assert construction invariants; `ParsePURL` propagates a version-unescape
   error (external input).

## Verification

- errcheck uncapped: 61 → **0**. Marker census: 31 `diagnose-ignored-error` sites.
- `make vet` pass; `make test` pass (full suite); `gofmt -l` clean.

## Chartered follow-ups (not in this change)

1. **Trace documents carry no integrity checksum** — the ruling's panic zone is empty for
   them until one exists; adding a trace checksum (mirroring the graph's) is a design
   decision to take.
2. **The checksum-trust-boundary ruling needs an architecture-doc home** — it currently
   lives in decoder doc comments and session memory only.
3. **`receipt.Commit` on the resume path** (`graph_executor.go`, marker "resume stamp")
   deserves design scrutiny: a failed commit silently loses the compensation stamping.
4. **Remaining lint debt for 4b-3+**: 281 non-complexity findings (gocritic 133, gosec
   55, revive 30, unparam 15, noctx 14, unused 11, nilerr 6, misspell 5, staticcheck 4,
   errorlint 4, bodyclose 3, goimports 1); the 61 complexity findings stay chartered.
