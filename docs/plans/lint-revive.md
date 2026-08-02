---
title: "Class e: revive — renames, docs, and the regex provider"
issue: TBD
status: complete
created: 2026-08-01
updated: 2026-08-01
---

# Plan: Class e — revive

Fifth rung of the 4b-3 ladder. Rulings 2026-08-01: rename the six stutter types; rename
the regexp provider to `regex` — directory, package, and Starlark-facing name.

## Renames

1. **Stutter types (37 files, ~490 references):** `config.{Accessor,Source,Element,Spec,
   Value}` and `platform.Result`. The reflection-by-name sites (`FieldByName("Element")`,
   generated `StructField{Name: "Element"}`) renamed consistently with the type.
2. **The regex provider:** `pkg/op/provider/regexp` → `pkg/op/provider/regex`; package
   `regex`; Starlark-facing provider name `regex` (regenerated, asserted by the gen
   tests); Makefile codegen paths, four .star scripts (including the LintCopyright
   extension), and the two test-data script filenames updated. Bootstrap note: the stale
   gen files deadlocked the generator build (no LKG snapshot), so their import paths were
   hand-bridged transiently and immediately overwritten by real regeneration.

## Docs and fixes

3. Eleven doc comments: five `NewProvider`s, four const blocks, `RuntimeEnvironment`
   (see 5), and `CommandInfo` — whose comment was a copy-paste bug ("HookCheckResult
   holds…"), now corrected.
4. Eight package docs written (`fsroot`, `doctaxonomy`, `op/provider`, `function`, `mem`,
   `starlarkbridge`); `internal/model`'s two "PkgPath model…" headers corrected to
   "Package model…".
5. **Stub deleted:** `RuntimeEnvironment.Config()` returned nil unconditionally with zero
   callers — removed per the stub rule and the greenfield mandate.
6. `context.Context` moved to first parameter in `walkSubgraphChildren` /
   `walkDecisionTree`; five callers and doc bullets updated.
7. Ownership stragglers from freshly landed parallel work: a new errcheck on the
   typed-mode path → `assert.Type` per the class-1 pattern; appnet's bodyclose is a false
   positive (`iox.Close` closes the body) → suppressed with that reason; the recurring
   archive goimports finding resolved for good (the comment inside the import group was
   the irritant — now an end-of-line comment).

## Verification

- revive 28 → **0** uncapped; errcheck/bodyclose/goimports 0.
- `make generate`, `make build`, full `make test` green; `gofmt -l` clean.
- Board after class e: gocognit 52 + gocyclo 9 (chartered complexity), noctx 20 (grew
  from 14 with newly landed code; next rung), unused 11 (class f).
