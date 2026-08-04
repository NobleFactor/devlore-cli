---
title: "Complexity refactors: 61 functions under the thresholds"
issue: "#312"
status: in-progress
created: 2026-08-02
updated: 2026-08-02
---

# Plan: Complexity refactors

The 4b-3 ladder's chartered remainder: 52 gocognit findings (threshold 20) and 9 gocyclo
findings (threshold 15) — every function in the repository that exceeds the configured
complexity limits. This is the repository's only remaining lint debt.

## Discipline (applies to every phase)

1. **Behavior-preserving decomposition only.** Extract-method into unexported same-file
   helpers (or `helpers.go` per style §10 when multi-file), early-return flattening,
   and table-driven dispatch where a switch IS the complexity. No public signature
   changes, no semantic changes; the existing test suite must pass unmodified.
2. **The PR diff is the review surface.** Each extracted helper's name and single
   responsibility are visible in the diff; anything that smells like a design decision
   rather than a mechanical seam gets surfaced in the phase's plan-doc notes before the
   commit script is written.
3. **Verification per phase:** the phase's findings enumerate to zero (uncapped), no new
   findings introduced anywhere, `make vet` + full `make test` green, gofmt clean.
4. **This document is updated as each phase completes** — status flipped, actuals
   recorded — before the next phase begins.

## Phases (leaf → core; one PR each)

| # | Branch | Scope | Findings | Status |
|---|---|---|---|---|
| 1 | `refactor/complexity-writ` | writ commands: upgrade `Execute` 30 + `classifyEntry` 21, deploy `Execute` 22 + `buildScopeGraph` 29, decommission `Execute` 23; devlore-test `TestContext.Check` 32 | 6 | **complete** |
| 2 | `refactor/complexity-star-app` | star app + shellcheck: `registerStarlarkCommand` 37, `Extension.Validate` 30, `Command.Run` 30, `flagValue` 26 (gocyclo); `parseShellFile` 32, `calculateFunctionComplexity` 26, `isValidFunctionName` 17 | 7 | **complete** |
| 3 | `refactor/complexity-goast-provider` | goast provider methods: `ConstGroups` 70, `Structs` 49, `Callable` 49, `Methods` 38, `Composites` 35, `SortDeclarations` 24, `TypeDoc` 24, `Calls` 22, `spacingRulesFromConfig` 17 | 9 | **complete** |
| 4 | `refactor/complexity-goast-analysis` | goast analysis/serialization: `LoadSourceFile` 67, `analyzeFileMetrics` 55, `assignSlots` 44, `schemaFromConfigVal` 43, `typeToString` 37, `checkLineWidth` 32, `SaveAs` 29, `itemProduction.Execute` 23 | 8 | **complete** |
| 5 | `refactor/complexity-providers` | Concrete providers + satellites: archive `extractEntries` 48, plan `splitReservedKwargs` 39 (**also resolves the parked seven-results carrier-struct question**), file `compensateWrite` 25 + `Link` 23 + `Find` 22, lore `buildPackage` 26, devconfig `reflectToStarlark` 23, platform `compositeManager.dispatch` 21 | 8 | **complete** |
| 6 | `refactor/complexity-conversion` | The conversion surfaces: starlarkbridge `dispatch` 65, `toGoInto` 38, `toStarlarkReflect` 31, `toNaturalGo` 16; op `envValue.ConvertTo` 26, `Convert` 18, `IsTruthy` 19 | 7 | pending |
| 7 | `refactor/complexity-engine` | The op engine core, last and most carefully: `ActionPlanner.Plan` 69, `NewMethod` 59, `newReceiverType` 32, executor `dispatchWithPolicy` 32 + `Run` 26, subgraph `validateGuardedEdges` 32, `checkPromiseTypes` 31, `rearm` 23, `parseParameterToken` 21, `assembleGraph` 17; flow `GatherPlanner.Plan` 34, `Gather` 26, `WaitUntil` 25, `ChoosePlanner.Plan` 16, `WaitUntilPlanner.Plan` 16 | 15 | pending |

Total: 60 listed + `TestContext.Check` counted in phase 1 = 61.

## Phase notes

**Phase 1 (complete):** the three writ `Execute`s decomposed onto the same narrative shape
— fold/build → group (`buildScopeGraphs`) → dry-run (`emitGraphs`) → run-and-collect
(`runAll`) — as per-package unexported helpers. The dry-run emitter is now verbatim-
identical in three packages; deduplicating it into a shared location is a structure
decision available for a ruling, deliberately not taken unilaterally. `classifyEntry`
split its encrypted-chain arm (`classifyEncryptedChain`) and render arm (`renderedFresh`);
`TestContext.Check` became a uniform per-kind dispatch (`checkExpectation`), extracting
`checkUnitCount` and `checkEqual` to match the existing check-helper family;
deploy's plan closure extracted `splitManifests` / `planManifests` / `planChains`.

**Phase 2 (complete):** `registerStarlarkCommand` extracted its four stages
(`findOrCreateParent`, `useLineFor`, `collectFlagValues`, `defineFlags`);
`Extension.Validate` became a per-command dispatch (`validateCommand` →
`validateCommandArgs`/`validateCommandFlags`); `Command.Run` extracted `buildArgsDict`
(itself split once more when the extraction still scored 23 — `applyPositionalArgs`
carries the positional switch) and `setCurrentCommand`; `flagValue`'s 24-case switch
split into three type-family extractors (integer/scalar/collection) with a stated
fallback chain. shellcheck's scanner state moved into a `functionTracker` with an
`observe` method, match recording into `recordLineMatches`, per-line cyclomatic
accounting into `branchDelta` + `countParameterRefs` (preserving the quirk that `case`
opens no nesting level while `esac` closes one, floored at zero), and
`isValidFunctionName` now reads through positive rune predicates.

**Phase 3 (complete):** the repo's worst function, `ConstGroups` (70), decomposed into a
named state machine — `constGroupsFromDecl` with an explicit `flush` closure, entries via
`constEntriesFromSpec`, mapping via `constGroupResults`, the anonymous local types
promoted to file-level `constEntry`/`constGroup`. `Structs` extracted
`structFields`/`fieldDetailsFrom`; `Callable` and `TypeDoc` now share `typeSpecDoc` (the
Doc-preference rule stated once) via per-file finders; `Methods` split filter
(`methodMatches`) from construction (`methodResultFor`); `SortDeclarations` split
collection (`declBlocksInScope`) from splicing (`spliceSortedBlocks`); `Calls` and
`Composites` extracted their closure bodies; `spacingRulesFromConfig` went table-driven
(order-independent fields, so map iteration is safe). **The pre-declared structure
question is now concrete:** the `collectGoFiles` + `parseFile` iteration remains
repeated in five methods (`Callable`, `ConstGroups`, `Structs`, `TypeDoc`, `Deps`) — a
shared per-file walker (e.g. `forEachParsedFile`) would retire the repetition; available
for a ruling, not adopted unilaterally.

**Phase 4 (complete):** `LoadSourceFile` (67) decomposed along its own commented phases —
`docCommentGroups` (itself split once more via `markDeclDocs`/`markSpecDoc` when the
extraction scored 27), `bodyCommentGroups`, `loadFuncDecl`/`loadGenDecl`,
`floatingCommentDecls`, `associateMethods` — with its anonymous types promoted to
`positionedDecl`/`pendingMethod`. `SaveAs` extracted the preamble rule
(`preambleCommentDecl` + `writePackageClause`, the package-doc-adjacency comment kept) and
`emitDeclNode`. `analyzeFileMetrics` split into line counters and per-declaration
counters. `assignSlots` became the algorithm's named parts — `buildScoreMatrix`,
`bestFreePair`, `forcedMatch`, `freeStrings` — retiring the two inline G602 proofs
naturally. `schemaFromConfigVal` went table-driven over a shared `reflectStringFields`.
`typeToString` extracted its func/chan arms; `checkLineWidth` its under-fill analysis
(`underFilledViolation` over a single `commentPairExempt` predicate);
`itemProduction.Execute` its consume predicate and in-place sentence split.

**Phase 5 (complete):** archive's `extractEntries` split per-entry materialization
(`extractEntry` with the hard-link arm in `copyHardlinkEntry`) from the loop's
guard/commit/push bookkeeping, plus `resolveExtractPrefix`. **The parked seven-results
question resolved:** `splitReservedKwargs` now returns `(filtered, reservedKwargs, error)`
— the carrier struct holds label, retry/transition policies, and error/retry handlers;
per-key parsing lives in `reservedKwargs.apply` over a generic `policyKwarg[T]` (the two
policy arms were identical modulo type), and the tooManyResultsChecker suppression is
gone. file's `compensateWrite` extracted `pruneTowardBoundary`; `Link` extracted
`existingLinkMatches`/`archiveOccupant` around the untouched conflict-policy switch;
`Find` extracted `resolveFindRoot` and its walk closure. lore's `buildPackage` split
per-action application from phase-subgraph assembly (and a banned-vocabulary comment fixed
in passing); devconfig's slice/map conversion arms and platform's composite grouping
(`leafGroup`, promoted from an anonymous type) extracted.

## Ordering rationale

Leaf-to-core: the writ/star/goast phases build the decomposition pattern on
self-contained, well-tested surfaces before the engine phase touches dispatch, planning,
and recovery — the code where a behavioral slip costs the most. The conversion phase
(6) immediately precedes the engine because the engine's planners lean on the conversion
cascade; refactoring the cascade first means phase 7 reads its final shape.

## Risks, stated

1. **The engine phase.** `ActionPlanner.Plan`, `dispatchWithPolicy`, and `rearm` sit on
   the saga/recovery path; their tests are strong (lifecycle e2e, resume, rollback), but
   any seam that changes evaluation order is a defect even if tests stay green. Extracted
   helpers must preserve exact ordering and short-circuit behavior; anything ambiguous
   gets surfaced before committing.
2. **goast's build-tag-free bulk** is safe ground, but its provider methods share walk
   patterns — phase 3 may reveal a shared-walk helper wanted by five methods. That is a
   structure decision: proposed in the phase notes, ruled before adoption.
3. **splitReservedKwargs** carries a pre-existing ruling request (the carrier struct);
   phase 5's notes present the struct shape for sign-off as part of the phase, not as a
   silent side effect.

## Not in scope

Raising thresholds, suppressing findings, or semantic redesigns. Every function ends at
or under gocognit 20 / gocyclo 15 by decomposition alone; if any function genuinely
cannot be decomposed without semantic change, that finding comes back here with the
evidence for a ruling.
