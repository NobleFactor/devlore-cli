---
title: "The regex Provider's Name in the Design Record"
issue: https://github.com/NobleFactor/devlore-cli/issues/678
status: draft
created: 2026-08-25
updated: 2026-08-25
---

# Plan: The regex Provider's Name in the Design Record

**Epic:** [#454 — Design and documentation debt](https://github.com/NobleFactor/devlore-cli/issues/454)
**Design:** [3.5-provider-catalog.md](../architecture/3.5-provider-catalog.md)

## Summary

The regexp provider was renamed to `regex` in code — directory, package, and Starlark-facing name — under
[lint-revive.md](lint-revive.md). The design record never followed. `3.5-provider-catalog.md` still calls itself
"the index of record for every provider" while naming this one by a name that no longer exists anywhere in the
tree, and the provider's own design doc and status companion are still filed under the old name. This plan
finishes the rename in the documentation.

## Goals

1. **One name** — `regex` in code and in docs, with no surviving `regexp` provider reference
2. **Findable docs** — the design doc and its status companion filed under the name a reader will search for
3. **A true test matrix** — the status doc naming fixtures that exist

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Package, directory, action names | ✅ `regex` | `pkg/op/provider/regex/action_names.gen.go:16` announces `regex.find` |
| Design doc filename | ❌ `3.5.15-regexp-provider.md` | plus its `.status.md` companion |
| Catalog row | ❌ `regexp` / `regexp.*` | `3.5-provider-catalog.md:48` |
| Status doc test matrix | ❌ Cites absent fixtures | names `test_regexp.star` and `test_imm_regexp.star`; **neither exists** — the fixture is `cmd/devlore-test/devloretest/data/test_regex.star` |

**23 `regexp` references across 6 architecture files.** `7-registry-knowledge.md`'s 8 hits need per-line triage:
Go stdlib `*regexp.Regexp` in code samples is correct and stays.

## Steps

| # | Step | ✓ |
| --- | --- | --- |
| 1 | `git mv docs/architecture/3.5.15-regexp-provider.md docs/architecture/3.5.15-regex-provider.md` | ☐ |
| 2 | `git mv` the `.status.md` companion to match | ☐ |
| 3 | Correct the 12 in-file references — header line, 8 method comments, cross-links | ☐ |
| 4 | Triage `7-registry-knowledge.md`'s 8 hits; stdlib references stay, provider references change | ☐ |
| 5 | Correct `3.5-provider-catalog.md:48`, `index.md:52`, `star-extensions.md:308` | ☐ |
| 6 | Repair the status doc's test matrix against the real fixture `test_regex.star` | ☐ |

## Not in scope

Fifteen `regexp` references live in `docs/plans/`, describing work done before the rename. Those were the correct
name when written and are accurate history. Only stale **code paths** in plan documents are corrected, because a
path that no longer resolves breaks navigation.

## Verification

- Zero `regexp` references in `docs/architecture/` except Go stdlib `*regexp.Regexp` in code samples
- Both renamed files resolve from every document that links them
- Every fixture named in the status doc's test matrix exists on disk
- `make check` green
