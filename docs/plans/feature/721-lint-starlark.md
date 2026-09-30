---
title: "LintStarlark: nothing checks Starlark, and dead API calls survive for months"
issue: https://github.com/NobleFactor/devlore-cli/issues/721
status: active
created: 2026-08-27
updated: 2026-09-29
---

# Plan: `LintStarlark`

Opened `draft` on 2026-08-27 out of the docker package rewrite
([docker.devlore-package.md](../docker.devlore-package.md)), where every phase script in the shipped
package called an API that no longer exists. `approved` on 2026-09-29, when the four things it left open
were measured against the tree. `active` the same day, with Phase 1 complete.

It lived at `docs/plans/lint-starlark.md`, named for its subject. A plan named that way is reachable
only by someone who already knows what it is called, so it now carries its issue and matches its
branch.

## Issue 721

## Summary

Nothing in either repository reads the contents of a `.star` file. 169 of them ship in `devlore-cli` and
40 shipped in `devlore-registry/packages/docker/`, where 57 live call sites reach APIs that do not exist.
This plan adds `LintStarlark`, a star extension that parses each file and resolves every `plan.*` call
against the **generated** provider tables, so the checker cannot drift from codegen.

## Goals

1. **Dead references are caught at author time.** Wrong namespace, absent method, wrong arity and wrong
   keyword argument all fail the lint rather than the deploy.
2. **The checker reads generated ground truth.** `action_names.gen.go` and the `ParameterNames` tables,
   never a hand-maintained list -- the discipline `Knowledge/extract.star` already applies to Go.
3. **Zero false positives** across the 169 `.star` files in this repository.

## The observed failure

`devlore-registry/packages/docker/` shipped **40** `.star` files -- the draft said 28, and counted
short. Read against the provider tree on 2026-08-26, the deploy path could not execute on any platform.

The fixture is recoverable and its commit is named: **`cc87c4f0`**, *Binding unification Phases 6-9 --
knowledge artifacts, slot_docs, docker lifecycle (#21)*, 2026-02-22. It is the last commit to touch
`packages/docker` before 2026-08-27, so the tree at that commit **is** the state the draft read.
`packages/docker` on `develop` today holds only `Darwin/`, `README.md` and `lifecycle.yaml`, so the
corpus exists nowhere else.

Counted in that tree, the seven rows do not carry equal weight, and the difference decides what the
checker can be asked to prove:

| Written in the package | Live sites | Commented | Reality |
| --- | ---: | ---: | --- |
| `plan.package.install("docker-ce", "docker-ce-cli")` | **23** | 7 | No `package` namespace -- it is `pkg`, and it takes a list, not varargs |
| `plan.verify(name, check=..., optional=True)` | **30** | 0 | No `Verify` method on any provider |
| `Upgrade/install.star` | **4** | 0 | `UpgradePhaseOrder` is `{prepare, upgrade, migrate, verify}` -- `install` is not an upgrade phase, and all four platforms define one |
| `plan.file.write(path=..., content=...)` | 0 | 6 | It is `plan.file.write_text(destination_path=..., content=..., mode=...)` |
| `plan.user.add_to_group(user, "docker")` | 0 | 8 | No `user` provider |
| `plan.download(url, dest)` | 0 | 12 | `appnet.download(url)` returns bytes; there is no dest form |
| `plan.notify(...)` | 0 | 4 | Does not exist |

**Four of the seven rows are commented-out TODOs, not live code**, and a parser sees a comment as
trivia. No checker built on `go.starlark.net/syntax` can report them, so an exit criterion demanding
all seven would be unmeetable by construction. The criterion names the three live rows instead, and
this table is why.

That the dead API also appears in comments is worth noting and is not this tool's problem: a comment
scanner is a different tool with a different false-positive profile, and inventing one here would widen
the issue to chase 30 sites that never executed.

The draft also said *"all four `verify.star` files are entirely these calls."* There are **eight**
`verify.star` files -- one per platform under `Deploy` and under `Upgrade` -- carrying 30 live
`plan.verify(` calls between them.

The registry's CI validates knowledge schemas and package structure. None of it reads a `.star` file's
contents, so all of the above passed continuously.

**A syntax checker would not have helped.** Every live line is syntactically valid Starlark. The
defects are *resolution* errors, and a general-purpose Starlark formatter knows nothing about devlore's
provider surface.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `go.starlark.net/syntax` | Available | Direct dependency, imported by ten files |
| `starindex/provider.go` | Available | Already walks `DefStmt`, `LoadStmt`, `AssignStmt` |
| `action_names.gen.go` | Available | 20+ files enumerating every valid `<namespace>.<method>` |
| `ParameterNames` tables | Available | 29 `gen/provider.gen.go` files enumerating every valid keyword |
| `LintCopyright/Go/GoStyle/Markdown/Shell/Tools` | Present | Seven extensions; `LintAll` finds siblings itself |
| `LintStarlark` | **Missing** | The hole this plan fills |

## What the draft said to discover, and what it measures

The draft assumed no solution and listed four open questions. All four are now answered against the
tree rather than guessed. The corpus is **169** `.star` files, not the 162 the draft counted.

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

**Yes, and it costs one step.** The registry's CI already builds star from this repository's source:

```yaml
# devlore-registry/.github/workflows/validate.yaml, on develop
- name: Checkout devlore-cli        # devlore-cli first, or its checkout cleans away the nested one
  with: { repository: NobleFactor/devlore-cli, path: devlore-cli }
- name: Checkout                    # this repository, INSIDE devlore-cli
  with: { path: devlore-cli/devlore-registry }
- name: Build star
  working-directory: devlore-cli
  run: go build -o ../star ./cmd/star
- name: Validate
  working-directory: devlore-cli
  run: ../star devlore ${{ matrix.command }} validate --target=devlore-registry
```

So `star lint starlark` arrives there the moment it is in star. There is no artifact to publish and no
follow-on issue.

Two constraints that workflow states and the registry step must respect. **star resolves its extensions
relative to the working directory**, which is why the commands run from `devlore-cli` and the registry is
checked out inside it — a parent-relative target failed fsroot confinement. And its check name,
`json-schema-validate.${{ matrix.corpus }}`, is *"the check context a ruleset must match exactly"*, so the
lint must be a **step**, not a new job, for the same reason it is a step of `quality-gate` here.

#721 does not close until the registry's CI is green. That matters because the registry is where the
packages are: `devlore-cli` holds no package phase scripts at all.

### 4. Adopt `buildifier` alongside as a parse gate?

**No.** Ruled 2026-09-28: buildifier is being replaced, and it does not do nearly enough. The draft's
own evidence is that it would have caught none of the seven defects above, and its formatter rewrites
`kwarg=value` to `kwarg = value`, which no file in the corpus uses and which cannot be configured off.
Candidate approach 4 is struck rather than left standing as a baseline.

## Requirements

### Requirement 1: resolution against generated tables

The checker reads `action_names.gen.go` for the valid `<namespace>.<method>` set and the `ParameterNames`
tables for the valid keyword arguments of each. It holds no list of its own.

**The valid set is context-dependent.** `cmd/lore/lore/builder.go` calls
`starlarkbridge.DenyAttributes("plan", lifecycleVerbs...)` for phase-script runtimes, and two of those
verbs are real, resolvable attributes of the plan provider -- `Clear` and `Run`, snake-cased to `clear`
and `run` by `op.CamelToSnake`. A resolver reading the generated table alone accepts `plan.run()` and
`plan.clear()`, which a phase script may not call. So the valid set for a phase script is the generated
table **minus** the denied set, read from the same `lifecycleVerbs` slice so the two cannot drift.

This is not a rule about style. `plan.run` in a phase script is unresolvable for the same reason
`plan.nur` is, and it produces the same finding.

**Proof**: add a provider method, regenerate, and observe the checker accept a call to it with no edit to
the checker.

### Requirement 2: scope awareness

A `plan` bound by an assignment, a parameter, a comprehension variable or a loop variable in an enclosing
scope shadows the builtin, and calls through it are not resolved.

**Proof**: `validate.star` and `extract.star` produce zero findings. These two files are the regression
fixture for this requirement, and they exist already.

### Requirement 3: the phase entry point

Two checks. They matter for different reasons, and the second is the one nothing else can give you.

**3a -- the entry function matches the phase.** `cmd/lore/lore/builder.go:589` looks the function up by
the phase name and fails when it is absent:

```go
entry, ok := scriptGlobals[action.PhaseName]
if !ok {
    return nil, fmt.Errorf("function %q not found in script %s", action.PhaseName, action.Path)
}
```

Run time already names the function and the file, so the checker moves a good message earlier rather
than supplying a missing one.

**3b -- the filename is a phase that lifecycle has.** `cmd/lore/lore/builder.go:368` skips a phase with
no actions, without a word:

```go
actions := release.PhaseActions(targetPlatform, lorepackage.Deploy, phaseName)
if len(actions) == 0 {
    continue
}
```

A file whose name is outside its lifecycle's order is never opened. Nothing is logged and the step does
not happen, so the symptom is an absence -- which is the class of defect that cannot be debugged from
what it leaves behind. The checker is the only thing that can report it.

The orders are in `cmd/internal/lorepackage/lifecycle.go`:

| Action | Phases |
| --- | --- |
| `Deploy` | `prepare`, `install`, `provision`, `verify` |
| `Upgrade` | `prepare`, `upgrade`, `migrate`, `verify` |
| `Decommission` | `unprovision`, `uninstall`, `cleanup` |
| `Reconcile` | `scan`, `repair`, `verify` |

`Upgrade/install.star` in the fixture violates 3b on all four platforms: `install` is a `Deploy` phase,
not an `Upgrade` one.

**All four orders are checked, not only the one a builder currently walks.** `builder.go:367` is today's
only `PhaseActions` caller and it is hardcoded to `Deploy`, so `Upgrade`, `Decommission` and `Reconcile`
scripts are not built by anything yet. That does not make their phase orders less real: they are defined,
packages are written against them, and a name that is wrong now is wrong when a builder arrives. The
checker reads `PhaseOrder` for the directory it is in.

**Not a separate requirement: the lifecycle verbs.** The draft listed rejecting `plan.assemble`,
`clear`, `load`, `run` and `save` as a rule of its own. It is not one. Two of the five resolve on the
plan provider and the other three do not, so all five are answered by Requirement 1's context-dependent
valid set. A phase script calling `plan.run()` gets the finding that `plan.nur()` gets, for the reason
`plan.nur()` gets it.

**Not a requirement: lambdas.** A graph carrying one fails receipt writing. That is its own defect on its
own merits and stays with [function-resource-receipts.md](../function-resource-receipts.md).

## Implementation Phases

### Phase 1 -- the corpus is preserved as a fixture

The commit is known and the recovery is one command, so this phase is bounded:

```bash
gh api repos/NobleFactor/devlore-registry/tarball/cc87c4f0 | tar -xz --strip-components=1 \
    NobleFactor-devlore-registry-cc87c4f/packages/docker
```

- [x] The 40 `.star` files of `packages/docker` at `cc87c4f0` are committed as a testdata fixture, at
      `cmd/star/provider/lint/testdata/docker-package/`. All 42 files, copied verbatim with the directory
      layout intact -- Requirement 3b resolves a phase against its action directory, so flattening the
      tree would destroy that evidence while leaving every file present. The corpus exists nowhere else:
      the registry's `develop` holds only `Darwin/`, `README.md` and `lifecycle.yaml` there.
- [x] The three live rows are inventoried in `testdata/docker-package.want.tsv`: **57 rows**, generated
      from the fixture rather than typed, and counted twice by different means with the same answer.
      23 `unknown-namespace`, 30 `unknown-method`, 4 `phase-not-in-order`. The `rule` names are the
      contract; the finding wording is not, so a message can be reworded without regenerating the file.
- [x] The four commented rows are in `testdata/README.md` with their counts -- `plan.download(` 12,
      `plan.user.*` 8, `plan.file.write(` 6, `plan.notify(` 4 -- stated as out of a parser's reach, so
      their absence from a run's results reads as correct rather than as a gap.
- [x] The fixture proves itself. `TestStarlarkFixtureShape` asserts 40 scripts and the 16/12/12
      Deploy/Upgrade/Decommission layout; `TestStarlarkFixtureInventory` resolves every one of the 57
      rows to its file and line, requires the recorded call to be present, and **requires the line not to
      be a comment** -- so a call that becomes commented cannot sit in the inventory as a defect the
      checker could never report. Both pass; `go build ./...`, `go vet` and the package's other tests are
      clean.

The copyright gate is unaffected: `star/config.yaml` excludes `**/testdata`, which `develop` already
relies on -- `cmd/star/provider/starindex/testdata/sample.star` carries no SPDX header and the gate is
green. The fixture keeps the registry's `MIT` headers, as a verbatim copy must.

### Phase 2 -- the resolver

**It does not read the generated tables.** It asks `op.ReceiverRegistry()`, which the generated `gen`
packages populate in their `init` functions, so it holds exactly what a running binary offers. Reading the
text would have lost the two things that decide real calls: `op.Parameter.Kwargs` — a method declaring
`**kwargs` accepts any keyword — and `op.Method.Claims()`, which is how a hermetic runtime decides what it
admits. Neither survives as a list of names.

Modeling `prepareScriptEnv` turned up **three** things reaching a script through `plan`, filtered
differently, where the plan assumed one:

1. `plan.<method>` — the plan provider's own methods. Script-surface globals, so **hermetic-filtered**:
   only a method claiming `op.ClaimDeterministic` is admitted.
2. `plan.<namespace>.<method>` — the graph namespace. Workflow-surface providers by name, **not**
   hermetic-filtered, because a graph accepts anything with an action signature.
3. `plan.<method>` — a promoted provider's methods, surfacing at the namespace root. This is what the 278
   bare `plan.<method>(` sites in the corpus are: `note`, `warn`, `fail`, `gather` and the rest, from `ui`
   and `flow`.

- [x] `cmd/star/provider/lint/starlint` resolves all three shapes and answers keywords through
      `op.Method.ParameterByName`, with `Kwargs` short-circuiting to accept anything.
- [x] `lifecycleVerbs` moved from `cmd/lore/lore/builder.go` to `lorepackage.LifecycleVerbs`, so lore's
      run-time denial and the linter read **one slice**. `TestDeniedMatchesLore` fails if a second copy
      reappears.
- [x] Table-driven tests over the live registry, not a mock. Every distinct resolution is covered by a case
      drawn from the fixture's real defects, and `TestRegistryIsPopulated` guards the vacuous pass — an
      unlinked registry answers "unknown" to everything and would otherwise go green having checked nothing.

**Two filters, reported apart, because the reason is the useful half of a finding.** `plan.clear` claims
determinism, so hermetic admits it and the denial is what stops it. `plan.run` claims nothing, so it is
gone before the denial is consulted. Both are refused; a package author needs to know which.

`not-deterministic` is not a hypothetical branch: `load_definition`, `save_definition` and `spec` all reach
it. That also settles something raised while reading the denial — of the three `*_definition` methods the
denial's bare verbs fail to name, two are already unreachable from a phase script through hermeticity.
`assemble_definition` is the one that is not, and the resolver reports it as resolved because that is what
the runtime does: a linter that disagreed with the runtime would be a false positive.

### Phase 3 -- the checker

- [x] Parsed with `go.starlark.net/syntax`. **`starindex`'s walker does not fit**: `indexStmts` walks only
      top-level statements and never enters a function body, so it cannot see a call site. `syntax.Walk` is
      used instead — it is complete by construction, and it signals node exit by calling the visitor with
      `nil`, which is what makes a scope stack possible without hand-rolling a traversal over 30 node types
      and risking a missed call shape.
- [x] Scope tracking per Requirement 2, covered by nine cases. The one that matters in the other direction:
      **`plan[k] = v` is an `IndexExpr` on the left and does not bind** — it mutates. Treating it as a
      binding would silently exempt every file that mutates the real plan, which is most of the corpus.
- [x] Requirements 3a and 3b, reading `PhaseOrder` for whichever action directory a script sits in — all
      four orders, not only `Deploy`.
- [x] **Zero findings across the 169 `.star` files, and exactly the 57 inventoried rows on the fixture.**
      `TestRepositoryIsClean` keeps the first true permanently rather than measuring it once.

**Two things only running it could have shown.**

*The checker had 12 false positives, and they were an error of model, not of code.* It applied the
phase-script runtime — hermetic, lifecycle verbs denied — to every `.star` file. But a `.star` file is not one
kind of thing: `cmd/devlore-test/devloretest/data` runs under an **ambient** runtime where
`plan.save_definition` is entirely legal, and `star` is a scripting tool where effects are the point. There
are now two resolvers, `NewPhaseScriptResolver` and `NewAmbientResolver`, chosen by the same
action-directory test that drives Requirement 3b. Requirement 2 was the false positive the plan predicted;
this was one it did not.

*Reaching zero on the 169 meant fixing two real dead calls the checker found.* Neither was caught by
anything before:

| File | Was | Is |
| --- | --- | --- |
| `pkg/op/provider/flow/testdata/integration.star` | `plan.fatal(...)` | `plan.failed(...)` |
| `cmd/devlore-test/devloretest/data/test_pkg.star` | `plan.pkg.update(manager="")` | `plan.pkg.update()` |

No provider defines a `Fatal` action; `flow`'s three terminal actions are `Complete`, `Degraded` and
`Failed`, which that file's own comment says while calling something else. And `func (p *Provider) Update()`
takes no parameters at all, so `manager=""` was never bindable — its neighbors `remove` and `upgrade`
declare `**kwargs`, which is why the same keyword is legal there. `TestPkgActions` executes that file and
passed anyway, because it dry-runs and never binds the slots; that is precisely the gap the checker closes.

**3b anchors its finding at the entry point's `def`, not at line 1.** The defect is the file's *name*, which
has no line — but the phase name does appear in the source, and a reader needs somewhere to look.

### Phase 4 -- the extension, and both gates

- [x] `lint.starlark` is an action of the lint provider, beside `EnsureTools`, `Go`, `Markdown` and `Shell`.
      The Go method was added and `make generate` regenerated the tables — *"Found 5 methods for Provider"*,
      up from four — producing `Starlark op.ActionName = "lint.starlark"` and
      `ParameterNames: []string{"files?"}`. No generated file was edited by hand.
- [x] `com.noblefactor.star.LintStarlark` with its `lint-starlark.star` command. Verified end to end against
      the real CLI, not only the Go tests: **`star lint starlark` reports 57 issues on the fixture and exit
      1, and `Starlark lint passed (170 files)` with exit 0 over everything else.** The 57 are the same 57
      the inventory holds.
- [x] `LintAll` discovers it through `commands.siblings()` with **no edit** — `=== STARLARK ===` appears in
      `star lint all`'s output, which is the aggregator finding and invoking it.
- [x] `devlore-cli` CI runs it, as a step of `quality-gate`. A separate job could not block a merge: this
      repository's ruleset requires exactly one check by that name, the same constraint #964 hit.
- [ ] `devlore-registry`'s `validate.yaml` runs it and its CI is green. **This is a second pull request in a
      second repository**, and it cannot be written until this one merges, because the registry builds star
      from `devlore-cli`'s default branch. #721 does not close until this box is ticked.

**One exclusion, and it is not a blanket testdata exemption.** `star/config.yaml` excludes
`cmd/star/provider/lint/testdata/docker-package` from `lint.starlark` and nothing else: the other 170 files
are checked, including all 11 extension commands and the 112 devlore-test scripts. The fixture's entire
purpose is to hold the 57 defects the checker must report, so requiring it to be free of them would delete
the test. It is not unchecked, only not gated there — `TestCheckFixtureMatchesInventory` asserts the checker
reports exactly those 57 rows and no others, which is a stricter claim than "reports nothing".

### `star lint all` could never run a linter, and three defects were why

Ruled 2026-09-29: `star lint all` must pass. It reported all seven linters as failed with **no output** —
`copyright`, `go`, `go-style`, `markdown`, `shell`, `starlark` and `tools` alike — while standalone they
worked, `star lint copyright` passing 990 files and `star lint starlark` 170, both exit 0.

**[#829](https://github.com/NobleFactor/devlore-cli/issues/829) does not address this.** It governs extension
*loading* narration — the noisy success banner, and making load failures narrate. Nothing to do with sibling
invocation. Three separate defects were:

1. **`Application.RunCommand` did not normalize dots to spaces.** Its map is keyed by the space-separated
   form, `"lint go"`, while `commands.CommandRef` holds the dotted name `CommandNames` hands out and passes
   it to all three tree methods. `CommandFlags` and `CommandHelp` normalize; `RunCommand` did not. So
   `cmd.flags` and `cmd.help` worked and `cmd.run` answered `command "lint.go" not found` for **every**
   sibling. `star lint all` has never been able to invoke a linter.
2. **`lint-all.star` discarded `result.error`.** `commands.run` already returns the cause; the aggregator
   read only `result.passed`, so a linter that could not be *invoked* was indistinguishable from one that ran
   and found problems. It now reports the error, which is what made the first defect visible at all.
3. **`lint.all` imposed its own `path` default on every sibling.** Its `extension.yaml` declared
   `default: "."`, forwarded as `cmd.run(fix=fix, path=paths)`, overriding the default each linter declares
   for itself. `lint.go`'s is `./...` because golangci-lint reads `.` as the root package alone, which holds
   no `.go` files — so `lint all` failed with *"no go files to analyze"* while `lint go` passed. The default
   is gone: a linter knows its own corpus better than the aggregator does.

**After the three: `copyright`, `go` and `starlark` pass.** The remaining four are not this issue's, and none
is a defect in the aggregator:

| Linter | Why it fails | Whose |
| --- | --- | --- |
| `go-style` | **5,455 violations in 848 files** — 2,536 missing doc comment, 1,787 missing Parameters, 1,132 missing Returns. Repo-wide, and it flags every test function including `provider_test.go`'s | the Go style backlog |
| `markdown` | `markdownlint-cli2 failed: The command line is too long` — Windows only, 476 paths as arguments | lane 2, [#932](https://github.com/NobleFactor/devlore-cli/issues/932) |
| `shell` | `shellcheck is not installed` | this host |
| `tools` | `shellcheck` and `shfmt` not installed | this host |

In CI, `shell` and `tools` would pass (the workflow installs them) and `markdown` would not hit the Windows
limit. **`go-style` is the one real blocker to `lint all` passing**, and clearing 5,455 violations is its own
piece of work.

**My own code is clean under both Go gates.** `star lint go ./...` found four issues, all mine, all fixed:
`bindsPlan` at cognitive complexity 35 and `newResolver` at 23 are decomposed, `planCallShape`'s results are
named, and the test's `exec.Command` is `exec.CommandContext`. `go-style` found 14 in the non-test files —
missing `Parameters` sections — and those are added. The 11 that remain are in test files, where every test
in the repository stands, and they belong to the backlog above rather than to a convention invented here.

The `lint.starlark` command declares a `fix` flag it cannot honor, because `lint-all.star` calls every sibling
with one. Nothing here is auto-fixable — a dead `plan.*` call needs a human to decide what was meant — so it
warns and checks anyway.

## Exit criteria

- [x] The checker over the docker package at `cc87c4f0` reports **the three live rows**: 23
      `plan.package.*` calls (Requirement 1), 30 `plan.verify(` calls (Requirement 1), and four
      `Upgrade/install.star` files (Requirement 3b). The four commented rows are out of a parser's reach and
      are not required of it. Asserted by `TestCheckFixtureMatchesInventory` and confirmed against the real
      CLI: `star lint starlark` reports 57 issues and exits 1.
- [x] The checker reads generated truth, **proven by construction rather than asserted.** `lint.starlark`
      itself is the experiment: the Go method was added, `make generate` regenerated the tables, and
      `plan.lint.starlark(files = [...])` — a method that did not exist beforehand — is accepted with **no
      edit to the checker**, while `plan.lint.nonexistent_action(...)` is still rejected. It reads
      `op.ReceiverRegistry()`, which the generated `init` functions populate, so drift from codegen is not
      possible rather than merely unlikely.
- [x] Zero false positives across the `.star` files in `devlore-cli`: `Starlark lint passed (170 files)`,
      exit 0, kept honest permanently by `TestRepositoryIsClean`. Reaching it required fixing two real dead
      calls the checker found, in `flow/testdata/integration.star` and `devloretest/data/test_pkg.star`.
- [ ] `devlore-registry`'s CI runs the checker and is green. **A second pull request in a second
      repository**, which cannot be written until this one merges: the registry builds star from
      `devlore-cli`'s default branch.

## Related

- [Docker devlore package](../docker.devlore-package.md) -- the rewrite that surfaced this
- [function-resource-receipts.md](../function-resource-receipts.md) -- the lambda defect, out of scope here
- `cmd/star/provider/starindex/provider.go` -- the existing Starlark AST walker
- [#376](https://github.com/NobleFactor/devlore-cli/issues/376) -- extension loading on Windows