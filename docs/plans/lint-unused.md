---
title: "Class f: unused — nine orphans deleted, two tag-blind suppressions"
issue: TBD
status: complete
created: 2026-08-02
updated: 2026-08-02
---

# Plan: Class f — unused

Final rung of the 4b-3 ladder. Rulings 2026-08-02: delete all nine dead symbols —
including `parseEncryptionSystem` and `Subgraph.addEdge`, whose features had materialized
and lost their call paths in the framework refactor — and suppress the two
cross-platform false positives.

## Deletions (9 symbols, ~170 lines)

`extensionsConfig.wrapAsStarlark`; goast's `encodeScope` (scopes now arrive encoded) and
`resolveGoSource`; the `makeHeading` test helper; migrate's `parseEncryptionSystem` (its
detection path assigns enum constants from artifacts, and the string enum unmarshals
natively); the function provider's `goToStarlark`/`starlarkToGo` (superseded by the
starlarkbridge/op.Convert cascade — a duplicated-logic divergence risk retired); the pkg
provider's `testActivation`; and `Subgraph.addEdge` (edges materialize through the
promise/resource path).

## Suppressions (2)

`indexAgeOf` and `unknownIndexAge` are the platform staleness gate's measurement and
sentinel — called by the apt and pacman leaves in `linux_managers_linux.go`, which the
darwin analysis never compiles. Suppressed citing exactly that; not dead, just invisible.

## Verification

- unused 11 → **0** uncapped. The repository's full lint output is now exactly the 61
  chartered complexity findings (gocognit 52, gocyclo 9); every other linter is at zero.
- `make vet` and full `make test` pass; `gofmt -l` clean.
- Ladder totals: 2,486 findings at the start of follow-up 4 → 61 chartered.
