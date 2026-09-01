---
title: "Enforcement: two tests that fail when a command leaves the output convention"
issue: https://github.com/NobleFactor/devlore-cli/issues/776
status: draft
created: 2026-09-01
updated: 2026-09-01
---

# Plan: Enforce the output convention

## Summary

Adoption of the output convention went backwards, not merely slowly. `extract-output-package.md` recorded
two call sites in March — `lore inspect` and `writ snapshot`. `writ snapshot` was removed, taking half the
convention's adopters with it, and nothing recorded that it had. Invariants that are not tested decay
silently. These tests are what "done" means for the epic.

This is the last item of thread 1 ([#740](https://github.com/NobleFactor/devlore-cli/issues/740),
[740-cli-output-conventions.md](740-cli-output-conventions.md)), after
[774-writ-reconcile.md](774-writ-reconcile.md), [775-lore-adoption.md](775-lore-adoption.md), and `star`
([#743](https://github.com/NobleFactor/devlore-cli/issues/743)). It is last because it is red until they
land, and it certifies them rather than preceding them.

## Goals

1. **Three invariants have a test each**, and each test has been shown to fail.
2. **The generated CLI reference documents one flag set** across every command of every program.
3. **The epic closes** on these tests being green, not on a belief that the work is done.

## Current State

| Invariant | Test | Status |
| --- | --- | --- |
| No command package writes to `os.Stdout` directly | none | red — writ has 30 sites until #774 |
| No command registers its own output flag | none | red — lore has three until #775 |
| Every root calling `AddOutputFlags` has every leaf reaching `BuildPipeline` | none | red — `star` until #743 |

The third invariant exists because the first two miss a case: #753 and #754 were a root that registered
the flags and leaves that ignored them. `writ` registered nothing of its own, and the `os.Stdout` check
finds a command that prints but not one that silently discards the flags it inherited. A flag registered
on a root that no leaf consumes is worse than an absent one, because it looks like compliance.

## Requirements

### Requirement 1: No direct `os.Stdout` from a command package

A test that walks every `cmd/*` package and fails on `fmt.Print*`, `os.Stdout.Write`, or
`fmt.Fprint*(os.Stdout, …)`. The allowlist is empty; a legitimate exception is a bug in the convention,
not in the test.

### Requirement 2: No command registers its own output flag

A test that walks every registered `cobra.Command` and fails when a command defines `--output`, `-o`,
`--format`, `--json`, `--jq`, `--filter`, or `--store` on itself rather than inheriting it from the root.

### Requirement 3: Every leaf consumes what its root registers

A test that, for every root calling `AddOutputFlags`, walks its leaves and fails when a leaf's `RunE`
never reaches `BuildPipeline`. Greppable as "every root calling `AddOutputFlags` has every leaf reaching
`BuildPipeline`". This is the one that would have caught #753 and #754.

### Requirement 4: Each test is shown to fail

Each test is authored against the current tree, where it is red by construction, **or** — for a test
written after its defect is gone — is shown red by re-introducing the defect once before it is trusted.
A test that has only ever passed has not been shown to be a test.

### Requirement 5: The generated docs agree

`docs/cli` is regenerated and a test confirms every command documents the same flags. `docs/cli` is
gitignored here but published — `docs-publish.yaml` regenerates it on every push to `develop` — so the
generated surface is what users actually read.

## Implementation Phases

### Phase 1: The three tests, red (status: not started)

- [ ] `no_direct_stdout_test.go` — walks `cmd/*`, fails on any direct write
- [ ] `no_own_output_flag_test.go` — walks every command, fails on a self-registered output flag
- [ ] `every_leaf_consumes_test.go` — walks every root with `AddOutputFlags`, fails on a leaf that
      never reaches `BuildPipeline`
- [ ] All three committed **red**, with the failing output in the commit message, so the record shows
      they can fail

### Phase 2: Green as the work lands (status: not started)

- [ ] Test 1 goes green when #774 lands
- [ ] Test 2 goes green when #775 lands
- [ ] Test 3 goes green when #743 lands
- [ ] Each transition recorded in this plan with the commit that caused it

### Phase 3: The generated docs (status: not started)

- [ ] `docs/cli` regenerated
- [ ] A test that every command's generated page lists the same flag set
- [ ] `10-command-line-interface.status.md` — the enforcement box ticked; the epic's last box
- [ ] `740-cli-output-conventions.md` — `status: complete`

**Files**:

- `cmd/internal/cli/no_direct_stdout_test.go` - Create
- `cmd/internal/cli/no_own_output_flag_test.go` - Create
- `cmd/internal/cli/every_leaf_consumes_test.go` - Create
- `cmd/internal/cli/docs_agree_test.go` - Create

## Test Plan

This plan **is** a test plan. The rows are the requirements restated with their failure conditions.

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | No command package writes to `os.Stdout` directly | unit | a `fmt.Print` is added to any `cmd/*` package |
| 2 | No command registers its own output flag | unit | a command calls `Flags().String("output", …)` |
| 3 | Every leaf under an `AddOutputFlags` root reaches `BuildPipeline` | unit | a root gains the flags and a leaf's `RunE` never uses them |
| 4 | Every generated CLI page lists the same flag set | unit | a program's root skips the set |

**Every row is written red first.** That is the point of this plan, and it is the reason it is sequenced
last: on the current tree all three are red, and they turn green one at a time as #774, #775 and #743
land — which makes each of those PRs' "done" observable rather than asserted.

**Not covered:** whether the convention itself is right. These tests enforce the convention as written
in `10-command-line-interface.md`; a change to the convention changes the tests, not the other way
around.

## Migration Path

None. Tests only, plus regenerated documentation.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/internal/cli/no_direct_stdout_test.go` | Create | invariant 1 |
| `cmd/internal/cli/no_own_output_flag_test.go` | Create | invariant 2 |
| `cmd/internal/cli/every_leaf_consumes_test.go` | Create | invariant 3 |
| `cmd/internal/cli/docs_agree_test.go` | Create | the generated docs |
| `docs/architecture/10-command-line-interface.status.md` | Modify | the last box |
| `docs/plans/feature/740-cli-output-conventions.md` | Modify | `status: complete` |

## Related Documents

- [740-cli-output-conventions.md](740-cli-output-conventions.md) — thread 1, the epic's plan; this closes it
- [774-writ-reconcile.md](774-writ-reconcile.md) — turns test 1 green
- [775-lore-adoption.md](775-lore-adoption.md) — turns test 2 green
- [10-command-line-interface.md](../../architecture/10-command-line-interface.md) — the convention enforced
- Issue [#776](https://github.com/NobleFactor/devlore-cli/issues/776)
- Issue [#743](https://github.com/NobleFactor/devlore-cli/issues/743) — turns test 3 green

## Open Questions

- [ ] Are the three tests one file or three? Three keeps each invariant's failure legible on its own;
      one keeps the walk over `cmd/*` in one place. Decided when the first is written.
