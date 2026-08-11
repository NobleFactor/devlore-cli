---
title: "Segment grammar enforcement"
issue: https://github.com/NobleFactor/devlore-cli/issues/369
status: draft
created: 2026-08-11
updated: 2026-08-11
---

# Plan: Segment grammar enforcement

## Summary

The layer directory grammar is `[<project>|common][.<os>][.<distro>][.<arch>][.<custom>...]`.
`cmd/writ/writ/segment/segment.go:18` declares it; nothing enforces it. Suffixes are parsed into
a positionless list and matched against one flat set of every dimension's values, so order is
never checked, position never selects a dimension, and specificity degrades to a suffix count
that cannot rank DISTRO above OS. The resulting ties are then resolved by an unstable sort.

This plan makes the grammar the parser's contract. Specificity becomes a dimension tuple with a
total order, which dissolves the tie problem instead of patching it.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Grammar | ❌ Documented only | `segment.go:18` states it; no code asserts it |
| `ParseDirName` | ❌ Positionless | Returns `parts[1:]`; position discarded at `segment.go:79` |
| `Segments.Match` | ❌ Dimension-blind | Flat `AllValues()` membership at `segment.go:93` |
| `Specificity()` | ❌ A count | `len(m.Suffixes)` at `segment.go:120` |
| Tie ordering | ❌ Undefined | `sort.Slice` (unstable) at `matcher.go:61` and `matcher.go:108` |
| Tie-break branch | ❌ Untested | `builder.go:280` — `make cover` reports count 0 |

### Live failure cases

1. **`common.Unix` vs `common.Darwin` on macOS.** `AllValues` adds the OS family
   (`segment.go:64`), so both are specificity 1 and tie. `Darwin` should win. `noblefactor.Unix`
   is a form the codebase documents at `cmd/writ/writ/migrate/session.go:316`, so this is live.
2. **`common.Linux` vs `common.Debian` on Debian.** Both specificity 1. A distro-specific file
   ties with an OS-generic one.
3. **`common.arm64.Darwin`** matches identically to `common.Darwin.arm64`. Both specificity 2,
   both accepted, one ungrammatical.
4. **`common.desktop.Darwin`** with `ROLE=desktop` matches — a custom value in the OS slot.

## Goals

1. **The grammar is the contract** — a directory name that violates the documented order is
   rejected loudly, not silently matched.
2. **Position selects a dimension** — a suffix in the OS slot is checked against OS, not against
   every value in every dimension.
3. **Specificity ranks dimensions** — a total order in which `Darwin` beats `Unix` and a
   distro-specific directory beats an OS-generic one.
4. **No undefined ordering** — the sort is stable or the comparison is total. Ties reduce to
   identical names, which cannot occur within one directory.
5. **The `builder.go:280` branch gets defined behavior and a test**, or is removed because the
   total order makes it unreachable.

## Design

Parsing becomes dimension resolution rather than a split:

1. Split the directory name on `.`; `parts[0]` is the project.
2. Resolve each suffix to the dimension it belongs to, consulting that dimension's value
   (OS including its family, DISTRO, ARCH, then each custom segment in configured order).
3. A suffix resolving to no dimension means the directory targets a different machine —
   no match, as today.
4. Verify the resolved dimensions appear in canonical order. Out-of-order is a **grammar
   violation**, reported against the directory, not a silent non-match.
5. Specificity becomes a tuple over dimensions, compared by the precedence ruled below.

Within the OS dimension, an exact OS beats its family: `Darwin` > `Unix`.

## Implementation Phases

### Phase 1: Typed parse — branch `fix/segment-grammar-parse`

- [ ] Replace `ParseDirName`'s `[]string` return with a structured result carrying the resolved
      dimension of each suffix.
- [ ] Resolve suffixes against their dimension rather than the flat `AllValues()` set.
- [ ] Report out-of-order suffixes as a grammar violation.
- [ ] Keep `Match` semantics otherwise unchanged so the phase is independently reviewable.

**Files**: `cmd/writ/writ/segment/segment.go` — Modify.

### Phase 2: Total-order specificity — branch `fix/segment-specificity-order`

- [ ] Replace `Specificity() int` with the dimension tuple and its comparison.
- [ ] `sort.Slice` → `sort.SliceStable` at `matcher.go:61` and `matcher.go:108`, or make the
      comparison total so stability is moot. Prefer total.
- [ ] Re-examine `builder.go:280`. If the total order makes same-layer ties impossible, delete
      the branch rather than leave dead code with a suppression.

**Files**: `cmd/writ/writ/segment/segment.go`, `cmd/writ/writ/segment/matcher.go`,
`cmd/writ/writ/tree/builder.go` — Modify.

### Phase 3: Tests

- [ ] `common.Unix` vs `common.Darwin` on Darwin — `Darwin` wins, deterministically.
- [ ] `common.Linux` vs `common.Debian` on Debian — `Debian` wins.
- [ ] `common.arm64.Darwin` — grammar violation, reported.
- [ ] `common.desktop.Darwin` with `ROLE=desktop` — no longer matches via the OS slot.
- [ ] A custom segment whose value collides with an OS/distro/arch name resolves to its own
      dimension.
- [ ] Existing `TestBuildMultiSource*` tests pass unchanged — they encode intended behavior and
      must not be edited to accommodate the fix.

**Files**: `cmd/writ/writ/segment/segment_test.go`, `cmd/writ/writ/tree/tree_test.go` — Modify.

### Phase 4: The `all` → `common` sweep — branch `docs/common-project-rename`

Separate concern, same neighborhood; sequenced last so it does not muddy the semantic diffs.

- [ ] `cmd/writ/writ/commands.go:113` — user-facing help still teaches `writ decommission all`.
- [ ] `cmd/writ/writ/migrate/session.go:316` — user-facing guidance, `e.g., all.Darwin`.
- [ ] `cmd/writ/writ/tree/builder.go:127` — doc comment.
- [ ] `cmd/writ/writ/deploy/deploy.go:45` — doc comment.
- [ ] 30 `"all"` occurrences across the writ tests.
- [ ] `docs/architecture/7-registry-knowledge.md:158` — `Home/Configs/all/`.
- [ ] Verify `docs/plans/writ-deploy-scenario.md:39` — it lists `all`, `microsoft`,
      `noblefactor`, `thenobles`, which may correctly describe the real Tuckr-era layout rather
      than being stale.

## Open Questions

- [ ] **Dimension precedence.** The grammar orders OS, DISTRO, ARCH. Is that increasing
      specificity — does `common.Debian` (DISTRO) beat `common.amd64` (ARCH) when both are
      present at equal count? A tuple needs a defined precedence, and left-to-right is an
      assumption until ruled.
- [ ] **Custom segment ranking.** Custom segments append after ARCH. Do they outrank ARCH (they
      are more situational) or rank below it? With several custom segments, does configured
      order define their precedence?
- [ ] **Out-of-order handling.** Hard error naming the directory, or a warning that skips it?
      An error is the greenfield answer, but it turns an existing misnamed directory into a
      failed deploy rather than a quiet mismatch.
- [ ] **Ambiguous values.** If a custom segment's value equals an OS or arch name, does the
      earliest dimension in canonical order claim it, or is the configuration itself rejected as
      ambiguous?

## Verification

`make vet`, `make build`, full `make test` green; `gofmt` clean. `make cover` shows the
`builder.go` collision block fully covered, or the branch removed. No existing
`TestBuildMultiSource*` assertion is weakened to accommodate the change.

## Related Documents

- Issue #369 — this bug
- [docs/plans/audit-remediation.md](./audit-remediation.md) — issue #365; this blocks its phase
  1b `buildMultiSource` decomposition
