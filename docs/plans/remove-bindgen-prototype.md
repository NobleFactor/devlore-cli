---
title: "Remove the bindgen prototype"
issue: https://github.com/NobleFactor/devlore-cli/issues/215
status: completed
created: 2026-08-12
updated: 2026-08-12
---

# Plan: Remove the bindgen prototype

## Summary

`prototype/bindgen/` is a January proof-of-concept for generating Starlark bindings from cobra
CLI source. Its extraction approach hit a documented semantic ceiling, the provider architecture
went a different direction (reflection-based codegen), and the strategic successor for
generate-from-vendor-docs is the AI authoring path (the licensing draft's Class 3b, and the
`lore onboard` pattern). The ruling (2026-08-12): it will not work well enough; remove it whole.
Git history keeps it — the same rationale this repository recorded when PR #311 deleted dead
code.

## Current State — measured, not assumed

| Piece | State |
| --- | --- |
| `internal/cobra/extractor.go` + its test | `//go:build ignore` — never compiled by anything |
| `cmd/main.go` | `//go:build ignore` — never compiled |
| `internal/` (schema, codegen, helpparser, loader, stubgen) | live, tests green — **imported by nothing** outside the prototype |
| `extractor.go`'s import `internal/bindgen` | does not exist; the file cannot compile even untagged |
| `golang.org/x/tools` | already absent from `go.mod` — `go mod tidy` dropped it long ago because its only consumer is ignore-tagged |
| References outside `prototype/` | exactly one prose line, `GITHUB-ISSUES.md:170` ("bindgen user guide") |
| Last touched in substance | created and last tested the same day, 2026-01-11 |
| Open charter | #215 "Reexamine bindgen prototype" (2026-03-13, unanswered) |

Two bare `//nolint` directives live in the ignore-tagged extractor and are **dead** — the linter
never loads the file. They are the whole of audit phase 1b-iii's scope (issue #365).

## Why removal, not decomposition

The README's own end-to-end analysis is the verdict: AST extraction yields syntax without
semantics — no positional arguments, no valid flag combinations, wrong invocation forms — so
144 generated commands each needed human refinement while 15 hand-written ones were production
ready. That is a ceiling, not a bug list. The successor is AI-driven authoring against vendor
documentation (`lore onboard` today; the licensing draft's authoring-CAG strategically), which
gets the semantics extraction structurally cannot.

## What is preserved

1. **The learnings.** The `internal/cobra/README.md` end-to-end analysis (what worked, the
   comparison table, why refinement was unavoidable) is copied into a closing comment on #215
   before the deletion merges, with the deleting commit's SHA for retrieval.
2. **The code**, via git history — `git show <sha>^:prototype/bindgen/...` recovers any of it.

## Implementation — one branch: `chore/remove-bindgen-prototype`

- [ ] Post the learnings comment on #215 (extractor results, the comparison table's verdict, the
      supersession rationale, the commit SHA once known).
- [ ] `git rm -r prototype/bindgen` — 12 files: the dead cobra extractor and cmd, and the live
      but unimported `internal` package with its tests and `examples/gh.yaml`.
- [ ] Update `docs/plans/audit-remediation.md`: 1b-iii re-scoped to **resolved by deletion**
      (bare directives 5 → 3), and two audit-statement corrections — the "none of those paths is
      excluded in `.golangci.yaml`" claim was false in effect for these two functions (the build
      tag excludes them from the linter entirely), and the repo-wide gocognit live-finding count
      was 9, not 11.
- [ ] PR body carries `Closes #215`, so the charter closes exactly when the deletion merges.
- [ ] `GITHUB-ISSUES.md:170` is left untouched — the file is root clutter already slated for
      removal by the licensing plan's §5.3 cleanup; editing one line of it here is churn.

## Verification

`make vet`, `make build`, full `make test` green (the suite loses one green package,
`prototype/bindgen/internal`, which nothing imports); `go list ./...` no longer reports any
`prototype/` package; bare `//nolint` directives measure exactly 3 (`buildMultiSource`,
`buildTree`, `applyGraphModifications` — 1b-iv/1b-v scope); zero `bindgen` references remain in
Go source.

**Corrected during execution:** the original bar said `go mod tidy` produces no diff. Wrong —
the commit script's guard caught a real change: `golang.org/x/text v0.34.0` reclassifies from a
direct requirement to `// indirect`, because a prototype file was its last direct importer.
Nothing leaves the graph and `go.sum` is unchanged. The measurement error was checking only the
dead extractor's dependencies (`x/tools`, already dropped) and never asking what the live
`internal` package still anchored. The tidied `go.mod` rides in the deletion commit.

## Effects on open charters

| Charter | Effect |
| --- | --- |
| #365 phase 1b-iii | Resolved by deletion; no decomposition to perform |
| #365 phase 1b-iv | Unchanged — now the last 1b branch |
| #215 | Closed with its answer and the preserved record |
| #379 | The two ignore-tagged `/dev/stdout` sites in `cmd/main.go` vanish; the "non-prototype" qualifier in its audits becomes unnecessary |
| #373 | `prototype/` stops needing special-casing in per-GOOS sweeps |
