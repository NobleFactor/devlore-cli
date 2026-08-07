---
title: "Golangci Template Sync"
status: complete
created: 2026-08-06
updated: 2026-08-07
---

# Plan: Golangci Template Sync

## Summary

Three divergent copies of the NobleFactor `.golangci` configuration exist: this repository's
root `.golangci.yaml` (the canonical — hardened by the lint ladder, 2,486 findings → 0), the
`defaultGolangciConfig` template embedded in star's lint provider (a stale pre-ladder
snapshot that seeds new repos), and `noblefactor-ops/.golangci.yaml` (older and looser — it
suppresses G204/G301/G302/G306 wholesale, the policy the ladder rejected in favor of
per-site suppressions with stated reasons). Approved 2026-08-06: sync all three from the
canonical.

## Steps

### 1. Upstream the canonical to noblefactor-ops — done

`noblefactor-ops/.golangci.yaml` replaced with the canonical template (repo config minus
the repo-specific exclusion below), merged as noblefactor-ops#126 with its own plan doc
(`docs/plans/golangci-canonical.md`). The original blocker — the ops tree sat on the parked
`chore/refine-coding-standards` branch — was resolved by ruling (2026-08-07): the parked
standards work was published and merged first (noblefactor-ops#125). The blanket
G204/G301/G302/G306 suppressions are gone from the org default.

### 2. Refresh star's embedded template — done

`defaultGolangciConfig` (cmd/star/provider/lint/provider.go) now carries the canonical:
the repository config minus the one repo-specific exclusion (revive var-naming for
`pkg/op/provider/(json|yaml)/`, whose package names deliberately mirror the stdlib).
107 → 148 lines; new repos seeded by `ensureGolangciConfig` get the current standard,
including `whitespace.multi-func`, the ladder's shared exclusions, and the v2-correct
`output` block.

### 3. Drift guard — done

`cmd/star/provider/lint/provider_test.go` — `TestDefaultGolangciConfigTracksRepoConfig`
parses both configs and requires semantic equality after removing the declared
repo-specific exclusion rules from the repository side. Any future ladder tweak must flow
into the template or be declared repo-specific in the test.
