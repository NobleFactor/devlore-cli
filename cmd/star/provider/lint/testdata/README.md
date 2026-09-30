---
title: "lint provider testdata"
description: "The docker package fixture for lint.starlark, and what it must report"
---

# `lint` provider testdata

## `docker-package/`

`devlore-registry/packages/docker` at commit **`cc87c4f0`** — *Binding unification Phases 6-9 —
knowledge artifacts, slot_docs, docker lifecycle (#21)*, 2026-02-22. Copied verbatim: 42 files, of
which 40 are `.star`.

This tree is the regression corpus for [#721](https://github.com/NobleFactor/devlore-cli/issues/721),
and it is here because **it exists nowhere else.** `packages/docker` on the registry's `develop` now
holds only `Darwin/`, `README.md` and `lifecycle.yaml`. `cc87c4f0` is the last commit to touch the path
before 2026-08-27, so this tree is the state that was read on 2026-08-26 when the defects were found.

The directory structure is load-bearing and must not be flattened. Requirement 3b resolves a script's
phase against the action directory that contains it — `Deploy/`, `Upgrade/`, `Decommission/` — so the
layout is part of the fixture, not packaging around it.

Its `Linux.Debian/` and `Linux.Fedora/` directories are the retired form of lore's platform selectors. Since
[#944](https://github.com/NobleFactor/devlore-cli/issues/944) they are `Debian/` and `Fedora/`, and lore refuses a
package that still carries the two-word form. They are kept verbatim here all the same, because this tree is #721's
regression corpus and the linter never reads the platform directory, only the action directory inside it; nothing
selects over this tree. That is #944's ruling, Q17 in
[its plan](../../../../../docs/plans/feature/944-one-selector-api-writ-and-lore.md#decisions).

## `docker-package.want.tsv`

The live defect sites, by `rule`, `path`, `line` and `call`. **57 rows**, generated from the fixture
rather than typed:

| Rule | Rows | What it is |
| --- | ---: | --- |
| `unknown-namespace` | 23 | `plan.package.*` — there is no `package` namespace; it is `pkg`, and it takes a list, not varargs |
| `unknown-method` | 30 | `plan.verify(` — no provider has a `Verify` method |
| `phase-not-in-order` | 4 | `Upgrade/install.star` — `install` is a `Deploy` phase; `UpgradePhaseOrder` is `{prepare, upgrade, migrate, verify}` |

The `rule` names are the contract this file holds; the finding **wording** is not, so a message may be
reworded without regenerating this file. Rows were counted twice by different means and agreed both
times.

## What this fixture cannot prove

The package also carries dead API calls that are **commented out**, and a parser sees a comment as
trivia. These are absent from the inventory on purpose, and their absence from a run's results is
correct behavior, not a gap in the checker:

| Commented call | Sites |
| --- | ---: |
| `plan.download(` | 12 |
| `plan.user.*` | 8 |
| `plan.file.write(` | 6 |
| `plan.notify(` | 4 |

Reporting these would need a comment scanner, which is a different tool with a different
false-positive profile. It is not in #721's scope.

## The lifecycles nothing builds

24 of the 40 scripts are under `Upgrade/` and `Decommission/`. `Planner.buildPackage`, in
`cmd/lore/lore/builder.go`, is the only caller of `Release.PhaseActions` and is hardcoded to
`lorepackage.Deploy`, so nothing in the tree builds those lifecycles today.

That does not reduce the fixture. Their phase orders are defined in
`cmd/internal/lorepackage/lifecycle.go` and packages are written against them, so a name that is wrong
now is wrong when a builder arrives — which is why Requirement 3b reads `PhaseOrder` for whichever
action directory a script sits in rather than assuming `Deploy`.
