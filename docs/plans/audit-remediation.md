---
title: "Audit Remediation"
issue: https://github.com/NobleFactor/devlore-cli/issues/365
status: draft
created: 2026-08-11
updated: 2026-08-11
---

# Plan: Audit Remediation

## Summary

A mechanical audit on 2026-08-11 produced five findings. Verification against the source
invalidated one, narrowed another, and showed a third to be undercounted by an order of
magnitude. This plan carries the survivors, with every correction recorded. The work is four
phases, each landing as one or more branches, every branch retired before the next begins.

## Findings, after verification

| # | Finding as first reported | Verdict |
| --- | --- | --- |
| 3 | Two bare `nolint` directives | **Undercounted** — 18 bare, not 2 |
| 4 | A live backward-compatibility path in the goast production parser | **Wrong** — reduced to a naming defect |
| 1 | `status:` used as a prose dumping ground | **Confirmed** — 59 free-text values |
| 2 | Three incompatible status conventions | **Confirmed** — plus 41 docs with no status |
| 5 | 22 TODO/FIXME markers | **Narrowed** — ~11 are real; the rest are product output |

### Finding 3 was undercounted

Reported as two sites. The tree carries **315 `nolint` directives, of which 18 are bare** — no
reason after the linter list. The original count came from a grep truncated at `head -20` whose
cap was then read as the total. Enumerate fully or do not report a count.

The 18 fall into two classes:

**Nine `errcheck`** — terminal-output and flag-registration discards:
`cmd/lore/lore/commands.go:224`, `cmd/lore/lore/commands.go:628`, `internal/pwsh/pwsh.go:291`,
and six in `internal/console/console.go` (lines 118, 123, 128, 133, 138, 143).

**Nine complexity** (`gocognit` / `gocyclo`): `cmd/lore/lore/commands.go:633` (`runOnboard`),
`cmd/lore/lore/onboard/onboard.go:311` (`generateManifest`), `cmd/devlore-index/main.go:95`
(`main`), `cmd/writ/writ/tree/builder.go:232` (`buildMultiSource`),
`cmd/writ/writ/migrate/gather.go:64` (`buildTree`), `cmd/writ/writ/migrate/session.go:374`
(`applyGraphModifications`), `internal/model/config.go:224` (`promptForProvider`),
`prototype/bindgen/internal/cobra/extractor.go:103` (`findPackages`),
`prototype/bindgen/internal/cobra/extractor.go:214` (`extractFromFunction`).

None of those paths is excluded in `.golangci.yaml`. The thresholds are `gocyclo` ≤ 15 and
`gocognit` ≤ 20.

Four other complexity suppressions in the tree *are* argued and correctly placed
(`internal/cli/selfinstall.go:210`, `internal/cli/selfinstall.go:329`, `pkg/op/helpers.go:343`,
`pkg/platform/linux_managers_linux.go:335`). They are not in scope.

### Finding 3 implicates plan #312

`docs/plans/complexity-refactors.md` (issue #312, status `complete`) decomposed 61 functions
across seven PRs and states it covered "every function in the repository that exceeds the
configured complexity limits — the repository's only remaining lint debt." Nine functions sat
out that campaign behind suppressions, so that claim does not hold as written. Recorded here as
a **known input to phase 3.1**: #312's `complete` status is a candidate for revision.

### Finding 4 was wrong

The claim was that `NewProduction` carries a backward-compatibility path that the greenfield
principle makes a critical bug. It does not. `SchemaElement.Type` is declared `yaml:"type"`
with no `omitempty` at `cmd/star/provider/goast/doctaxonomy/schema.go:61` — it is the schema's
live, required classifier. `Production` and `Consumes` are the newer *optional* refinements
(`schema.go:69`, `schema.go:70`). The code at `cmd/star/provider/goast/production.go:330` and
`cmd/star/provider/goast/production.go:352` derives defaults from the still-live `Type` field
when those optional fields are absent. It is load-bearing: deleting it breaks every schema
element that does not spell out `production` and `consumes`.

The actual defect is the word "legacy" in those two comments and the two test names that
inherited it. A naming fix, not a deletion.

### Finding 5 was overstated

Roughly half the 22 markers are the goast doc-stub generator's own product output — the
`TODO(go-style): add summary` text it writes into undocumented code
(`cmd/star/provider/goast/production.go:287`, `cmd/star/provider/goast/production.go:316`) and
the tests asserting it. Real debt is ~11 sites. Three are stubs, but all three fail loudly
(`cmd/writ/writ/adopt_cmd.go:143`, `pkg/op/provider/elevator/broker.go:112`,
`pkg/op/provider/elevator/broker.go:137`). None returns success without implementing, so none
is the critical-bug class from the CLAUDE.md checklist. They are chartered debt.

## Rulings (2026-08-11)

1. **Phase 3 splits per subtree** — one PR per plan subtree, each fully normalizing its own
   files, rather than one 244-file pass or a split by finding.
2. **The status enum is replaced.** Seven tokens, in lifecycle order:

   ```
   draft | proposed | chartered | in-progress | completed | deferred | abandoned
   ```

   `chartered` means **approved**. `settled` is gone (it folds into `chartered`); `complete`
   becomes `completed`. `deferred` and `abandoned` are the off-ramps. TEMPLATE.md is rewritten
   to this set.
3. **Every plan document is reconciled against the code and the merged PRs**, with a proposed
   status reported for each — all ~244, not just the 41 that lack a status field.
4. **The nine complexity suppressions are decomposed, not annotated** — the #312 discipline
   applied verbatim, and the suppressions deleted rather than argued.
5. **Treatment is decided per function by whether it reads as steps** (2026-08-11). Extract only
   where named steps fall out; flatten where the problem is depth; argue the suppression where
   splitting would fragment a coherent unit. Measured verdicts are in phase 1b below. Ruling 4
   stands as the default; this is the criterion that decides how each one is met.

### Status mapping implied by ruling 2

| Current | Count | → |
| --- | --- | --- |
| `complete` + dated variants | 116 | `completed` |
| `implemented`, `shipped`, `done pending commit` | 4 | `completed` |
| `draft` | 44 | `draft` |
| `in-progress` | 19 | `in-progress` |
| `settled`, `design-solidified` | 3 | `chartered` |
| `active` | 1 | `in-progress` |
| `proposed` | 6 | `proposed` |
| `deferred` | 1 | `deferred` |
| `pending` | 1 | decided by reading it |

A starting hypothesis only. Ruling 3 governs: the reconciliation decides each document's real
status from evidence, and a document marked `complete` today gets `completed` only if the code
and PR history bear that out.

## Goals

1. **Every suppression states its reason, or does not exist** — no bare `nolint` in the tree,
   and no complexity suppression standing in for a decomposition.
2. **Comments say what is true** — no "legacy" label on a live schema field.
3. **Plan status is trustworthy** — one convention, one legal token, and every value
   reconciled against what actually shipped.
4. **Every TODO is triaged** — fixed, cited to an issue, or deleted.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `nolint` reasons | ⚠️ 9 bare | Was 18 of 315; the 9 `errcheck` fixed by 1a (PR #367) |
| Complexity gate | ❌ 8 escapees | Was 9; `applyGraphModifications` measured under both thresholds |
| goast `Type` comments | ❌ Inaccurate | 2 comments + 2 test names call a live field "legacy" |
| Plan status convention | ❌ 4 conventions | 186 YAML, 16 bold, 1 table, 41 none |
| Plan status values | ❌ 59 free-text | Paragraph-length values in a machine-readable field |
| Plan status accuracy | ❌ Unverified | No document's status checked against the code |
| TODO markers | ❌ Untriaged | ~11 real; 3 are documented stubs |

## Implementation Phases

### Phase 1: suppressions

Ruling 4 splits this into a mechanical stage and a decomposition campaign.

#### 1a — `errcheck` reasons — branch `lint/errcheck-reasons` — **COMPLETED**

Landed as PR #367, merged 2026-08-11 (develop `e19d35cd`).

- [x] `cmd/lore/lore/commands.go:224` — `fmt.Scanln` at an interactive confirmation prompt; a
      short read leaves `response` empty, which the following `EqualFold` treats as "not y" and
      cancels. Stated exactly that.
- [x] `cmd/lore/lore/commands.go:628` — `MarkFlagRequired` on a flag registered immediately
      above; it fails only if the flag name is absent, which the same function guarantees.
- [x] `internal/pwsh/pwsh.go:291` and the six `internal/console/console.go` sites (118, 123,
      128, 133, 138, 143) — terminal writes with no recovery path.

All nine follow the convention already used at 32 sites: the directive on its own line above the
statement, `diagnose-ignored-error: <reason>; see
docs/architecture/2.8-eventing-infrastructure.md`. **Bare directives 18 → 9**, re-enumerated
uncapped. Verified: `make vet`, `make build`, full `make test` green (zero FAIL/panic lines by
count); `gofmt -l` empty.

#### 1b — Complexity decomposition — four branches

Applying the #312 discipline verbatim: behavior-preserving extract-method into same-file
unexported helpers (or `helpers.go` per style §10 when multi-file), early-return flattening, and
table-driven dispatch where a switch *is* the complexity. No public signature changes, no
semantic changes; the existing test suite passes unmodified. Each suppression is **deleted**,
not rewritten. Per-branch verification: the target functions enumerate to zero findings, no new
findings anywhere, `make vet` and full `make test` green, gofmt clean.

All nine were read in full and measured with `gocyclo` and `gocognit` (2026-08-11). Ruling 5's
criterion produced five distinct treatments, not one.

| Function | gocyclo | gocognit | Verdict |
| --- | --- | --- | --- |
| `promptForProvider` (`config.go:224`) | 16 | 17 | **Extract** — steps; four switch arms are one table |
| `main` (`devlore-index/main.go:95`) | 15 | 28 | **Extract** — steps |
| `runOnboard` (`commands.go:635`) | 18 | 21 | **Extract** — steps |
| `generateManifest` (`onboard.go:311`) | 16 | 26 | **Extract** — steps |
| `extractFromFunction` (`extractor.go:214`) | 19 | 51 | **Extract** — a named operation, not steps; a six-deep AST ladder |
| `buildMultiSource` (`builder.go:232`) | 12 | 31 | **Extract** — a named domain rule, not steps. **BLOCKED on #369** |
| `findPackages` (`extractor.go:103`) | 11 | 23 | **Flatten** — depth, not sequence; invert one condition |
| `buildTree` (`gather.go:64`) | 13 | 23 | **Argue** — a `WalkDir` guard chain returning distinct sentinels; splitting fragments the filter |
| `applyGraphModifications` (`session.go:374`) | 7 | 8 | **Delete the directive** — under both thresholds; it suppresses nothing |

Branch split, revised:

| Branch | Work | Status |
| --- | --- | --- |
| `refactor/complexity-lore` | `runOnboard`, `generateManifest` — extract | **completed** |
| `refactor/complexity-internal` | `promptForProvider`, `main` — extract | chartered |
| `refactor/complexity-bindgen` | `extractFromFunction` — extract; `findPackages` — flatten | chartered |
| `refactor/complexity-writ-migrate` | `applyGraphModifications` — delete; `buildTree` — argue | chartered |

**1b-i landed 2026-08-11.** `runOnboard` decomposed into `parseLoreOnboardConfig`,
`newOnboardProvider`, `syncedRegistry`, `reportOnboardResult`, and `writeOnboardManifest`;
`generateManifest` into `writeProductHeader`, `writeComplexityWarning`, and
`writeInstallCommands`. Both suppressions deleted.

| Function | Before | After |
| --- | --- | --- |
| `runOnboard` | gocyclo 18 / gocognit 21 | gocyclo 4 / gocognit 3 |
| `generateManifest` | gocyclo 16 / gocognit 26 | gocyclo 3 / gocognit 2 |

No helper exceeds a threshold; the largest is `reportOnboardResult` at gocyclo 10 / gocognit 12.
Bare directives 9 → 7.

**Characterization tests landed with the refactor.** The first pass offered "no test was modified"
as behavior-preservation evidence, which was worthless: `cmd/lore/lore/onboard` had **0%**
coverage and both target functions measured zero covered blocks, so there were no tests to
modify. Tests were written before the commit, with expectations hand-derived from the
pre-decomposition implementations rather than captured from the new code, and both suites were
mutation-checked — a one-space change to the annotation indent and a `"."` → `"./"` change to the
output-directory default each failed exactly the tests that should have failed.

| Package | Before | After |
| --- | --- | --- |
| `cmd/lore/lore/onboard` | 0% | 25.8% |
| `cmd/lore/lore` | 12% | 19.8% |

Per-helper: `parseLoreOnboardConfig` 3/3 blocks, `newOnboardProvider` 5/5,
`reportOnboardResult` 13/13, `writeOnboardManifest` 3/3. `syncedRegistry` (0/6) and `runOnboard`
itself (0/7) stay uncovered — both are bound to the registry and the network. The decomposition
is what made the other four testable at all.

**Standing rule for the remaining 1b branches: tests land before the refactor commit**, since a
behavior-preserving claim is unverifiable without them.

**`buildMultiSource` is deferred to issue #369.** Its collision predicate at `builder.go:280` has
zero coverage (`make cover`: `builder.go:280.45,282.7 1 0`) and is reachable only through a
specificity tie whose winner is undefined — `sort.Slice` at `matcher.go:61` is unstable. A
behavior-preserving refactor of a predicate with no defined behavior is not possible. #369 gives
specificity a total order, which is expected to make that branch unreachable and deletable rather
than testable.

Thresholds for reference: `.golangci.yaml` sets `gocyclo` 15 and `gocognit` 20;
`Makefile:163` gates at `gocyclo -over 20`. None of the nine trips the Makefile gate — every one
is a golangci-only finding, which is how the suppressions were reachable.

### Phase 2: goast naming accuracy — branch `chore/goast-type-field-naming`

- [ ] `cmd/star/provider/goast/production.go:330` — state that `Consumes` is optional and
      `Type` supplies the default.
- [ ] `cmd/star/provider/goast/production.go:352` — same correction for `Production`.
- [ ] Rename `TestNewProduction_LegacyParagraph` and `TestNewProduction_LegacySection`
      (`cmd/star/provider/goast/production_test.go:385`, `:396`) and the section delineator at
      `:383` to name what they assert: defaulting from `Type` when the optional fields are
      absent.

No behavior changes. `make test` stays green.

### Phase 3: plan status reconciliation

Ruling 3 makes this a reconciliation before it is a normalization. Three stages.

#### 3.0 — TEMPLATE rewrite — branch `docs/plan-status-enum`

- [ ] Rewrite the frontmatter block in `docs/plans/TEMPLATE.md` to the seven-token enum.

One file. Lands first so every later PR has a fixed target to conform to.

#### 3.1 — Reconciliation report — branch `docs/plan-status-reconciliation`

- [ ] Read every plan document under `docs/plans/` (~244).
- [ ] For each, gather evidence: the PRs that touched it, whether the work it describes exists
      in the code, whether its phase checkboxes match reality.
- [ ] Produce `docs/plans/audit-remediation/status-reconciliation.md` — one row per document:
      path, current status and convention, evidence, **proposed status**, and confidence.
- [ ] Flag every document whose current status contradicts the evidence. `complexity-refactors.md`
      (#312) is already a known candidate.

This stage changes **no** plan document. It delivers the report for review; statuses are applied
only after approval.

#### 3.2..3.N — Application, per subtree

Sized after 3.1, because the report determines how many documents actually change.

| Subtree | Docs |
| --- | --- |
| (top-level) | 92 |
| `extract-starlark-from-op/phase-8` | 77 |
| `resource-management` | 13 |
| `binding-unification` | 10 |
| `resource-provider`, `provider-registration`, `mem-resource` | 8 each |
| `star-gen-receiver` | 7 |
| `reconciliation`, `compensation` | 6 each |
| `terminal-flow-control` | 4 |
| `extract-starlark-from-op` | 3 |
| `variadic-starlark-args` | 2 |
| `receiver-params-registration` | 1 |

The two large subtrees split further to keep each diff reviewable. Each PR converts its
documents to YAML frontmatter, applies the approved status, and moves any free-text prose
**verbatim** into a `## Status Notes` body section. Nothing is deleted.

### Phase 4: TODO triage — branch `chore/todo-triage`

- [ ] Confirm the ~11 real markers and separate them from goast's product output.
- [ ] For each: fix it, charter it against a GitHub issue and cite the issue in the comment, or
      delete it. No marker survives uncited.
- [ ] The three documented stubs get issue references, not deletion.

**Files**: `cmd/writ/writ/adopt_cmd.go`, `internal/lorepackage/action.go`,
`pkg/op/runtime_environment.go`, `pkg/op/provider/elevator/provider.go`,
`pkg/op/provider/elevator/broker.go`, `pkg/platform/helpers.go`,
`cmd/lore/lore/onboard/onboard.go` — Modify.

## Open Questions

None outstanding. Rulings 1–4 close every question this plan opened.

## Related Documents

- [docs/plans/TEMPLATE.md](./TEMPLATE.md) — the status enum, rewritten by phase 3.0
- [docs/plans/complexity-refactors.md](./complexity-refactors.md) — issue #312; its `complete`
  status is a phase 3.1 candidate
- [docs/plans/segment-grammar-enforcement.md](./segment-grammar-enforcement.md) — issue #369;
  **blocks** phase 1b's `buildMultiSource` decomposition
- [docs/plans/deploy-manifest-error-propagation.md](./deploy-manifest-error-propagation.md) —
  issue #368; found during the phase 1b review of the same file, kept out of the
  behavior-preserving work
- Issue #65 — the standing ledger of judgment errors; no mechanical audit covers it
