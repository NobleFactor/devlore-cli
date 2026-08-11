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
invalidated one and narrowed another; this plan carries the survivors. The work is four
phases, each landing as one or more branches, every branch retired before the next begins.

## Findings, after verification

| # | Finding as first reported | Verdict |
| --- | --- | --- |
| 3 | Two bare `nolint` directives | **Confirmed** — real, two sites |
| 4 | A live backward-compatibility path in the goast production parser | **Wrong** — reduced to a naming defect |
| 1 | `status:` used as a prose dumping ground | **Confirmed** — 59 free-text values |
| 2 | Three incompatible status conventions | **Confirmed** — plus 41 docs with no status |
| 5 | 22 TODO/FIXME markers | **Narrowed** — ~11 are real; the rest are product output |

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
   status reported for each — not just the 41 that lack a status field.

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

These mappings are the starting hypothesis only. Ruling 3 governs: the reconciliation decides
each document's real status from evidence, and a document currently marked `complete` gets
`completed` only if the code and PR history bear that out.

## Goals

1. **Every suppression states its reason** — no bare `nolint` in the tree.
2. **Comments say what is true** — no "legacy" label on a live schema field.
3. **Plan status is trustworthy** — one convention, one legal token, and every value
   reconciled against what actually shipped.
4. **Every TODO is triaged** — fixed, cited to an issue, or deleted.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `nolint` reasons | ❌ 2 bare | Every other suppression in the tree carries a reason |
| goast `Type` comments | ❌ Inaccurate | 2 comments + 2 test names call a live field "legacy" |
| Plan status convention | ❌ 4 conventions | 186 YAML, 16 bold, 1 table, 41 none |
| Plan status values | ❌ 59 free-text | Paragraph-length values in a machine-readable field |
| Plan status accuracy | ❌ Unverified | No document's status has been checked against the code |
| TODO markers | ❌ Untriaged | ~11 real; 3 are documented stubs |

## Implementation Phases

### Phase 1: `nolint` reasons — branch `lint/nolint-reasons`

- [ ] `cmd/lore/lore/commands.go:224` — `fmt.Scanln` discards its error at an interactive
      confirmation prompt; a short read leaves `response` empty, which the following
      `EqualFold` treats as "not y" and cancels. State exactly that.
- [ ] `cmd/lore/lore/onboard/onboard.go:311` — `generateManifest` is suppressed for
      `gocognit,gocyclo` with no reason. Either state why the complexity is accepted or
      charter the decomposition. **Read the function first**; the disposition follows from
      what it is, and is not assumed here.

**Files**: `cmd/lore/lore/commands.go`, `cmd/lore/lore/onboard/onboard.go` — Modify.

### Phase 2: goast naming accuracy — branch `chore/goast-type-field-naming`

- [ ] `cmd/star/provider/goast/production.go:330` — state that `Consumes` is optional and
      `Type` supplies the default.
- [ ] `cmd/star/provider/goast/production.go:352` — same correction for `Production`.
- [ ] Rename `TestNewProduction_LegacyParagraph` and `TestNewProduction_LegacySection`
      (`cmd/star/provider/goast/production_test.go:385`, `:396`) and the section delineator at
      `:383` to name what they assert: defaulting from `Type` when the optional fields are
      absent.

No behavior changes. `make test` stays green.

**Files**: `cmd/star/provider/goast/production.go`,
`cmd/star/provider/goast/production_test.go` — Modify.

### Phase 3: plan status reconciliation

Ruling 3 makes this a reconciliation before it is a normalization. It runs in three stages.

#### 3.0 — TEMPLATE rewrite — branch `docs/plan-status-enum`

- [ ] Rewrite the frontmatter block in `docs/plans/TEMPLATE.md` to the seven-token enum.

One file. Lands first so every later PR has a fixed target to conform to.

#### 3.1 — Reconciliation report — branch `docs/plan-status-reconciliation`

- [ ] Read every plan document under `docs/plans/` (~244).
- [ ] For each, gather evidence: the PRs that touched it, whether the work it describes exists
      in the code, whether its phase checkboxes match reality.
- [ ] Produce `docs/plans/audit-remediation/status-reconciliation.md` — one row per document:
      path, current status and convention, evidence, **proposed status**, and confidence.
- [ ] Flag every document whose current status contradicts the evidence. Those are the finding.

This stage changes **no** plan document. It delivers the report for review; the proposed
statuses are applied only after they are approved.

#### 3.2..3.N — Application, per subtree

Sized after 3.1, because the report determines how many documents actually change. The
subtrees available to split on:

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

The two large subtrees are split further to keep each diff reviewable. Each PR converts its
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

- [ ] Ruling 3 is read as covering all ~244 documents rather than only the 41 without a status.

## Related Documents

- [docs/plans/TEMPLATE.md](./TEMPLATE.md) — the status enum, rewritten by phase 3.0
- Issue #65 — the standing ledger of judgment errors; no mechanical audit covers it
