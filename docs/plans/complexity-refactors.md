---
title: "Complexity refactors: 61 functions under the thresholds"
issue: "#312"
status: in-progress
created: 2026-08-02
updated: 2026-08-13
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
| 6 | `refactor/complexity-conversion` | The conversion surfaces: starlarkbridge `dispatch` 65, `toGoInto` 38, `toStarlarkReflect` 31, `toNaturalGo` 16; op `envValue.ConvertTo` 26, `Convert` 18, `IsTruthy` 19 | 7 | **complete** |
| 7 | `refactor/complexity-engine` | The op engine core, last and most carefully: `ActionPlanner.Plan` 69, `NewMethod` 59, `newReceiverType` 32, executor `dispatchWithPolicy` 32 + `Run` 26, subgraph `validateGuardedEdges` 32, `checkPromiseTypes` 31, `rearm` 23, `parseParameterToken` 21, `assembleGraph` 17; flow `GatherPlanner.Plan` 34, `Gather` 26, `WaitUntil` 25, `ChoosePlanner.Plan` 16, `WaitUntilPlanner.Plan` 16; **plus lint `Go` 31, a plan-table omission caught and folded in here** | 16 | **complete** |

| 8 | `refactor/complexity-remainder` | `git.guessDirName` 27 (`pkg/op/provider/git/helpers.go:182`), `cli.runSelfInstall` 22 (`cmd/internal/cli/selfinstall.go:213`) | 2 | **complete 2026-08-15** |

Total: 60 listed + `TestContext.Check` counted in phase 1 = 61, plus the 2 of phase 8 = 63.

### Phase 8 — the two the ladder did not clear (added 2026-08-13)

Surfaced by `make check` failing its `complexity` step during the #373 Windows campaign; both
functions predate that work and neither was touched by it. Measured directly with
`gocyclo -over 20`:

```
27 git guessDirName    pkg/op/provider/git/helpers.go:182:1
22 cli runSelfInstall  cmd/internal/cli/selfinstall.go:213:1
```

**The two gates disagree, and that is its own finding.** `make complexity` (standalone
fzipp/gocyclo, threshold 20) fails on both, so `make check` is not a clean local signal today.
`make lint-all` reports **0 issues** on all three GOOS values even though `.golangci.yaml` enables
`gocyclo` at `min-complexity: 15`. For `runSelfInstall` the reason is explicit — it carries
`//nolint:gocognit,gocyclo // orchestration function with sequential install steps`, which
golangci honors and standalone gocyclo does not. **For `guessDirName` there is no suppression
anywhere in the file and no path exclusion covering it, yet golangci reports nothing**; running
`golangci-lint run --enable-only gocyclo ./pkg/op/provider/git/...` also returns 0 issues. Cause
undetermined — a plausible hypothesis is that the `min-complexity` setting is not reaching the
linter (golangci's own default is 30, which would explain silence at 27 and 22 alike), but that
was not confirmed, so it is recorded as an open question rather than a diagnosis.

Resolving the discrepancy is part of this phase: a threshold that silently does not apply is a
gate that has been passing on nothing. Decompose both functions per the discipline above, then
confirm the finding count reaches zero under **both** tools rather than one.

**Done 2026-08-15.** `gocyclo -over 20` reports nothing, and `make check` passes end to end.

`guessDirName` decomposed along the seven steps its own doc comment already names — `skipScheme`
(1), `skipAuthentication` (2), `trimTrailingGitSuffix` (3), `trimPortNumber` (4) and
`lastComponentStart` (5) became same-file helpers, leaving the parent as the sequence plus its three
error checks. Steps 6 and 7 were already one-liners.

`runSelfInstall` decomposed along its numbered install stages — `installManPagesUnderPrefix`,
`installShellCompletions`, `initConfigAndCache`, `runPostInstallHooks`, `initWritLayerDirectories`
and `printInstallSummary`. Each returns its display lines and manifest paths; the parent is now the
sequence that accumulates them. **Its `//nolint:gocognit,gocyclo` suppression is deleted** — the
function no longer needs one, which is the outcome the phase was after.

**The gate discrepancy is not diagnosed, only removed from the critical path.** Both tools now report
zero on these two functions, but that is because the functions are small, not because the
configuration was understood. Whether `.golangci.yaml`'s `gocyclo: min-complexity: 15` actually
reaches the linter remains untested — golangci reported nothing at 27, which its own default of 30
would explain. **Anything landing between 20 and 30 will still pass `make lint` and fail
`make complexity`.** That belongs to whoever next touches the lint configuration.

Note: issue #312 is CLOSED (2026-08-04). This phase needs either a reopen or its own tracking
issue before its PR — not yet decided.

## Phase notes

**Phase 1 (complete):** the three writ `Execute`s decomposed onto the same narrative shape
— fold/build → group (`buildScopeGraphs`) → dry-run (`emitGraphs`) → run-and-collect
(`runAll`) — as per-package unexported helpers. The dry-run emitter was verbatim-
identical in three packages; ruled 2026-08-04 (single-codec Requirement 1) and folded into
`op.SerializeGraphs` beside `op.LoadGraph` — see docs/plans/single-codec.md. `classifyEntry`
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

**Phase 6 (complete):** `dispatch` (65) decomposed into named dispatch phases over two new
carrier types — `parameterLayout` (classification) and `routedArgs` (argument routing) —
with `classifyParameters`, `routeArgs`, `unpackNamed`, `fillNamedSlots`,
`fillVariadicSlot`, and `fillKwargsSlot`; the activation/invoke tail untouched.
`toStarlarkReflect` extracted its struct arm (`structToStarlark`); `toGoInto` extracted
`toGoScalar` (behind an `isScalarKind` predicate) and `toGoSliceTarget`; `toNaturalGo`
its sequence/dict projections. `envValue.ConvertTo` extracted `parseScalarEnv`;
`Convert` its steps-1–2 direct paths (`convertDirect`, hot-path comments preserved);
`IsTruthy` its scalar switch. **Incident, disclosed:** a blind text-slice during
`toNaturalGo`'s extraction corrupted `converter.go` (self-referential replacement);
recovered by restoring the develop copy via the API and re-applying with
bounds-verified slices — the lesson (assert slice contents and lengths before
replacing) applied to every later edit.

**Phase 7 (complete):** the engine decomposed with exact order preservation throughout.
`ActionPlanner.Plan` (69) became `bindParameterSlots` → `bindPresentValue` →
`bindResourceValue` (the addressing rules' commentary carried into the helpers); `NewMethod`
(59) its four labeled validators (`validateParameterPositions`, `classifyMethodKind`,
`validatePlanCompanion`, `buildUndoInvoke`); `newReceiverType` (32) split announced/derived
method building. The executor's `dispatchWithPolicy` (32) extracted `waitRetryDelay` and
the three-outcome `gateRetry`; `Run` (26) extracted `prepareResume` and `failAndUnwind`
(the step-21 terminal selection stated once). The subgraph's guard validation split into
decision-node and tree-shape passes; `checkPromiseTypes` into a per-slot check; `rearm`
into per-entry re-arming; `parseParameterToken` into sink/named token parsers. flow gained
a shared `resolveCombinatorSubgraph` (Gather's nil-Graph compensator nuance preserved by an
inline pre-check), `classifyGatherRuns` with `gatherRun` promoted from an anonymous type,
`bindKwargFrame`, a shared `desugarLambdaBody`, and `layoutDecisionTree`. **A plan-table
omission surfaced and was corrected: lint's `Go` (31) was in the original 61 but assigned
to no phase; it folded in here** (`modTidyStatus`/`golangciArgs`/`runGolangciLint`/
`appendGoIssues`).

**Final board: zero.** The repository's full lint output — every linter, uncapped — is
empty. From 2,486 findings at the start of the lint charter to none, with every
suppression in the codebase carrying its stated reason.

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
