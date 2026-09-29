---
title: "LintStarlark: nothing checks Starlark, and dead API calls survive for months"
issue: https://github.com/NobleFactor/devlore-cli/issues/721
status: draft
created: 2026-08-27
updated: 2026-09-29
---

# Plan: `LintStarlark`

Written as a charter on 2026-08-27 out of the docker package rewrite
([docker.devlore-package.md](../docker.devlore-package.md)), where every phase script in the shipped
package called an API that no longer exists. Promoted to a plan on 2026-09-29, when the four things the
charter said had to be discovered were measured against the tree.

The charter lived at `docs/plans/lint-starlark.md`, named for its subject. A plan named that way is
reachable only by someone who already knows what it is called, so it now carries its issue and matches
its branch.

## Issue 721

## Summary

Nothing in either repository reads the contents of a `.star` file. 169 of them ship in `devlore-cli` and
28 shipped in `devlore-registry/packages/docker/` calling seven distinct APIs that do not exist. This
plan adds `LintStarlark`, a star extension that parses each file and resolves every `plan.*` call against
the **generated** provider tables, so the checker cannot drift from codegen.

## Goals

1. **Dead references are caught at author time.** Wrong namespace, absent method, wrong arity and wrong
   keyword argument all fail the lint rather than the deploy.
2. **The checker reads generated ground truth.** `action_names.gen.go` and the `ParameterNames` tables,
   never a hand-maintained list -- the discipline `Knowledge/extract.star` already applies to Go.
3. **Zero false positives** across the 169 `.star` files in this repository.

## The observed failure

`devlore-registry/packages/docker/` shipped 28 `.star` files. Read against the provider tree on
2026-08-26, the deploy path could not execute on any platform:

| Written in the package | Reality |
| --- | --- |
| `plan.package.install("docker-ce", "docker-ce-cli")` | No `package` namespace -- it is `pkg`, and it takes a list, not varargs |
| `plan.verify(name, check=..., optional=True)` | No `Verify` method on any provider. **All four `verify.star` files are entirely these calls** |
| `plan.file.write(path=..., content=...)` | It is `plan.file.write_text(destination_path=..., content=..., mode=...)` |
| `plan.user.add_to_group(user, "docker")` | No `user` provider |
| `plan.download(url, dest)` | `appnet.download(url)` returns bytes; there is no dest form |
| `plan.notify(...)` | Does not exist |
| `Upgrade/install.star` | `UpgradePhaseOrder` is `{prepare, upgrade, migrate, verify}` -- `install` is not an upgrade phase |

The registry's CI validates knowledge schemas and package structure. None of it reads a `.star` file's
contents, so all of the above passed continuously.

**A syntax checker would not have helped.** Every line is syntactically valid Starlark. The defects are
*resolution* errors, and a general-purpose Starlark formatter knows nothing about devlore's provider
surface.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `go.starlark.net/syntax` | Available | Direct dependency, imported by ten files |
| `starindex/provider.go` | Available | Already walks `DefStmt`, `LoadStmt`, `AssignStmt` |
| `action_names.gen.go` | Available | 20+ files enumerating every valid `<namespace>.<method>` |
| `ParameterNames` tables | Available | 29 `gen/provider.gen.go` files enumerating every valid keyword |
| `LintCopyright/Go/GoStyle/Markdown/Shell/Tools` | Present | Seven extensions; `LintAll` finds siblings itself |
| `LintStarlark` | **Missing** | The hole this plan fills |

## What the charter said to discover, and what it measures

The charter assumed no solution and listed four open questions. All four are now answered against the
tree rather than guessed. The corpus is **169** `.star` files, not the 162 the charter counted.

### 1. How much of `plan.<namespace>.<method>(...)` resolution is tractable statically?

**All of it.** Measured across the corpus:

| Shape | Sites |
| --- | ---: |
| `plan.<namespace>.<method>(` literal | 246 |
| `plan.<method>(` literal | 278 |
| `getattr(` | 2 |
| `plan[` subscript | 4 |

The six that look dynamic are not. `plan[namespace]` in
`star/extensions/com.noblefactor.devlore.Actions/commands/validate.star:142` and
`star/extensions/com.noblefactor.devlore.Knowledge/commands/extract.star:332` is a **local dictionary
that shadows the builtin**, and `getattr(comp.fields, "Operations", None)` reads a component's fields,
not the plan. Dynamic dispatch on the plan object does not occur anywhere in the corpus.

**This sets a design constraint, and it is the load-bearing one.** Because `plan` really is shadowed by
a local in two real files, the checker must be scope-aware. A checker that matches on the name alone
reports six false positives on the day it lands and fails Goal 3 immediately. Requirement 2 exists for
this reason.

### 2. Star extension, Go analyzer, or `star` subcommand?

**A star extension**, `com.noblefactor.star.LintStarlark`, beside the seven that exist. `LintAll`
discovers its siblings through `commands.siblings()`, so it joins `make check` with no edit to the
aggregator.

Caveat, recorded rather than worked around:
[#376](https://github.com/NobleFactor/devlore-cli/issues/376) has Starlark extension loading failing on
Windows, where paths interpolated into source break escape parsing. That blocks proving the extension
**on this machine**; it does not block the approach, and CI is the authority.

### 3. Does it run against `devlore-registry` too?

**Not in this issue.** The registry needs the checker as a built, published artifact, which is a
different shape of problem from writing it. Phase 5 records what that would take; it should become its
own issue rather than widening this one.

### 4. Adopt `buildifier` alongside as a parse gate?

**No.** Ruled 2026-09-28: buildifier is being replaced, and it does not do nearly enough. The charter's
own evidence is that it would have caught none of the seven defects above, and its formatter rewrites
`kwarg=value` to `kwarg = value`, which no file in the corpus uses and which cannot be configured off.
Candidate approach 4 is struck rather than left standing as a baseline.

## Requirements

### Requirement 1: resolution against generated tables

The checker reads `action_names.gen.go` for the valid `<namespace>.<method>` set and the `ParameterNames`
tables for the valid keyword arguments of each. It holds no list of its own.

**Proof**: add a provider method, regenerate, and observe the checker accept a call to it with no edit to
the checker.

### Requirement 2: scope awareness

A `plan` bound by an assignment, a parameter, a comprehension variable or a loop variable in an enclosing
scope shadows the builtin, and calls through it are not resolved.

**Proof**: `validate.star` and `extract.star` produce zero findings. These two files are the regression
fixture for this requirement, and they exist already.

### Requirement 3: the phase entry point

Every phase script defines `def <phase>(package, phase)` matching its filename and its action directory's
phase order. `Upgrade/install.star` is the motivating failure: `install` is not an upgrade phase.

### Requirement 4: lifecycle verbs

`cmd/lore/lore/builder.go:31` denies `plan.assemble`, `clear`, `load`, `run` and `save` to phase scripts
at runtime. The checker rejects them at author time.

**Not a requirement:** lambdas. A graph carrying one fails receipt writing, which is its own defect on its
own merits; whether the linter should also flag it depends on whether that fix lands first. It is out of
scope here and stays with
[function-resource-receipts.md](../function-resource-receipts.md).

## Implementation Phases

### Phase 1 -- the corpus is preserved as a fixture

- [ ] `devlore-registry/packages/docker/` as it stood on 2026-08-26 is recovered and committed as a
      testdata fixture. It is the regression corpus and the exit criterion names it; it must not be lost
      to a cleanup before the checker exists to run against it.
- [ ] Each of the seven rows in the failure table maps to a named test case.

### Phase 2 -- the resolver

- [ ] A package that loads the generated tables and answers: does `<namespace>.<method>` exist, and does
      it accept keyword `<k>`?
- [ ] Table-driven tests over the real generated data, not a mock.

### Phase 3 -- the checker

- [ ] Parse with `go.starlark.net/syntax`, reusing `starindex`'s walker where it fits.
- [ ] Scope tracking per Requirement 2.
- [ ] Requirements 3 and 4.
- [ ] Zero findings across all 169 `.star` files; all seven fixture rows reported.

### Phase 4 -- the extension and the gate

- [ ] `com.noblefactor.star.LintStarlark` with its `lint-starlark.star` command.
- [ ] `LintAll` picks it up through `commands.siblings()` with no edit -- verified, not assumed.
- [ ] CI runs it. Proof is a CI run reporting the count over 169 files, the same way the PowerShell gate
      proved itself in #973.

### Phase 5 -- the registry

- [ ] Record what publishing the checker as an artifact for `devlore-registry` would take, and file it as
      its own issue. **This phase closes by filing, not by building.**

## Exit criteria

- [ ] The checker over the 2026-08-26 docker package reports every row in the failure table.
- [ ] The checker reads generated tables, proven by adding a provider method and observing acceptance
      with no edit.
- [ ] Zero false positives across the 169 `.star` files in `devlore-cli`.

## Related

- [Docker devlore package](../docker.devlore-package.md) -- the rewrite that surfaced this
- [function-resource-receipts.md](../function-resource-receipts.md) -- the lambda defect, out of scope here
- `cmd/star/provider/starindex/provider.go` -- the existing Starlark AST walker
- `cmd/lore/lore/builder.go:31` -- the runtime denial Requirement 4 moves to author time
- [#376](https://github.com/NobleFactor/devlore-cli/issues/376) -- extension loading on Windows