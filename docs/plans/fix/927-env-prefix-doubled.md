---
title: "Lane 8: a program setting's variable is the prefix plus the bare key; a variable naming nothing is warned of"
issue: https://github.com/NobleFactor/devlore-cli/issues/927
status: draft
created: 2026-10-02
updated: 2026-10-02
---

# Plan: Lane 8 of the writ lifecycle schedule

## Summary

Lane 8 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), in PR C, which opens after it and lane 33.
Ruled on #927 (2026-09-23): **a setting's environment variable is the prefix plus the bare key**; the key's program
segment is dropped when the prefix already names the program, and the doubled spelling is not honored. That is how
`aws`, `gh` and `docker` name theirs. Planning it on 2026-10-02, the owner ruled the shared root's flags (`--verbose`,
`--dry-run` and the rest) **global settings**, built in lane 24
([#1010](https://github.com/NobleFactor/devlore-cli/issues/1010)) with `DEVLORE_` variables, and #1011 goes with them.
This lane keeps the settings only one program has, adds a test that no two settings share a variable, and adds **a
warning for a variable that names nothing**. It changes configuration naming only, and is independent of lane 33's
named roots.

## Issue 927

Bug, epic WritDeployment (#451), feature #762; #916 lane 8, PR C. Its acceptance, written around `WRIT_VERBOSE`, is
rewritten to this plan, since `--verbose` is now a global setting.

## Rulings

All by the owner on 2026-10-02, while planning this lane.

1. **Lane 23 (#1011) goes with lane 24.** It first joined this lane ("Lane 23 should join lane 8"), then followed the
   shared root's flags to lane 24 when they went global: "split confirmed".
2. **The model settings' variables are `DEVLORE_MODEL_*` alone.** "WRIT_MODEL_PROVIDER should go away. we have a
   DEVLORE_MODEL_PROVIDER. I don't see that as limiting. leave it in lane 24."
3. **The shared root's flags are global settings**: "yes, global in lane 24". `--verbose`, `--dry-run`, `--interactive`,
   `--unattended`, `--silent`, `--config` and the `--model-*` flags bind to global settings read from
   `DEVLORE_<KEY>`. Lane 24 builds that, and its generated reference names their variables.
4. **A hyphen becomes `_`, not squeezed.** "keep _, add the collision test", reaffirmed after weighing Spring Boot's
   squeeze, which drops hyphens because Spring binds variables back into settings and viper never does: "keep _, add
   the warning."
5. **A variable that names nothing draws a warning**: "add the warning."

## Goals

1. A program setting reads from the program's prefix plus the key without the program's own segment: `writ.repo`
   from `WRIT_REPO`, and a nested `writ.deploy.conflict` would read `WRIT_DEPLOY_CONFLICT`.
2. The doubled spelling, `WRIT_WRIT_REPO`, is not honored.
3. A hyphen in a setting's name becomes `_` in its variable.
4. No two settings share a variable, by test.
5. A variable whose name begins with a program's prefix or `DEVLORE_`, and that nothing reads, draws a warning
   naming it.
6. The CLI design document states the rule.
7. Every Go file the lane touches passes `star lint go-style`.

## Current State

Read 2026-10-02 at `72a73e40`.

| Component | Status | Notes |
| --- | --- | --- |
| `cmd/internal/cli/viper.go` `InitViper` | ❌ | `SetEnvPrefix(prefix)`, `AutomaticEnv`, and a replacer from `.` to `_`; a key already namespaced under its program reads the doubled name, `WRIT_WRIT_REPO`. The function's doc comment states the ruled form, which the code does not do |
| program settings read through viper | ⚠️ | `writ.repo` and `writ.vars` (`cmd/writ/writ/config.go`), and `writ.scopes`, which `layer.go` reads as a map, so no variable reaches its entries. lore, star and devlore-test read none; lore's model settings go global in lane 24 |
| the shared root's flags | lane 24 | bound per program today (`writ.verbose`, `writ.dry-run`); lane 24 makes them global |
| graph parameters | ⚠️ | `pkg/op`'s `VariableResolver` reads a parameter from `<PREFIX>_<NAME>`, the namespace the settings use (open question 1) |
| variables read by name | ✓ | `DEVLORE_VERBOSITY`, `DEVLORE_DRY_RUN`, `DEVLORE_MODEL_*` and `DEVLORE_REGISTRY_*` (`cmd/internal/config`), `DEVLORE_PAGER`, `DEVLORE_VERSION`, `DEVLORE_REGISTRY` (devlore-index) and `WRIT_SEGMENT_<NAME>`; the installers read `DEVLORE_BASE`, `DEVLORE_TEAM`, `DEVLORE_PERSONAL`, `DEVLORE_TOOLS` and `DEVLORE_VERSION` |
| `config get` | n/a | reads the configuration file, not viper, so it shows no variable's value |
| `10-command-line-interface.md` | ❌ | states the precedence (flags, then the environment, then configuration) but not how a variable is named |
| viper v1.21.0 | ready | upper-cases `PREFIX_key` before applying the key replacer (`mergeWithEnvPrefix`, then `getEnv`), so a replacer can drop the doubled segment at the start of the name and nowhere else |

## Requirements

### Requirement 1: the mapping

For a program whose prefix is `P`, a key `<program>.<rest>` reads `P_<REST>`: the rest upper-cased, dots and hyphens
as underscores. The doubled `P_<PROGRAM>_<REST>` is not honored. The rule lives in one place, `InitViper`'s key
replacer, whose first pair turns the doubled prefix `P_<PROGRAM>.` into `P_`; viper prefixes the key before it
replaces, so the pair matches at the start of the name only. `InitViper`'s doc comment states the rule.

### Requirement 2: the collision test

A test maps every known setting (flags, defaults, documented settings, and the variables read by name) to its
variable, and fails if two settings share one.

### Requirement 3: the warning

At startup, a variable whose name begins with a program's prefix or `DEVLORE_`, and that names nothing a program
reads, draws a warning through the narrator, so `--silent` quiets it. The warning names the variable and, when one
is near, the known name it resembles: `WRIT_WRIT_REPO` suggests `WRIT_REPO`, and `DEVLORE_DRYRUN` suggests
`DEVLORE_DRY_RUN`. The known names are Requirement 2's table, the installers' variables, and the families that carry
a name, such as `WRIT_SEGMENT_<NAME>` for a declared segment; graph parameters are open question 1.

### Requirement 4: the tests

- **Unit:** the mapping, for each program's prefix: a plain key, a nested key, a hyphenated key, and the doubled
  spelling refused; the collision test; the warning's known names and its nearest-name suggestion.
- **Subprocess, one per program on the shared root:** an unknown variable with the program's prefix draws the
  warning on standard error, the doubled spelling of a known setting draws it naming the bare one, and under
  `--silent` neither prints. They run beside the self-install scenarios in `cmd/scenario`, under
  `make test-scenario`.

### Requirement 5: the pages

`10-command-line-interface.md` states the rule in one line, beside the precedence: a program setting's variable is
the program's prefix plus the bare key, and a global setting's is `DEVLORE_` plus the key, which lane 24 builds. It
states the warning too.

### Requirement 6: the style gate

Every Go file this lane touches passes `star lint go-style` in the lane's commit.

## Implementation Phases

### Phase 1: The plan

- [ ] This document, reviewed with the owner and chartered.

### Phase 2: The mapping and the collision test

- [ ] Requirements 1 and 2, with their unit tests.

### Phase 3: The warning

- [ ] Requirement 3, and Requirement 4's warning tests.

### Phase 4: The pages and the gate

- [ ] Requirements 5 and 6; `make check` and `make test-scenario` green.

### Phase 5: The VM

On `danoble-ud24-1.local`, with the build installed: `WRIT_WRIT_REPO=x writ version` warns and suggests `WRIT_REPO`,
and an unknown `LORE_` and `STAR_` variable each draws the warning. The Windows box is out of service; CI's Windows
jobs are the Windows proof.

- [ ] `danoble-ud24-1.local` (linux/arm64)

### Phase 6: Closure

- [ ] The lane's commits on this branch. PR C opens after lane 33, with `Closes #927`.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | the mapping, for each program's prefix | unit | a key reads the doubled name, or a nested or hyphenated key maps otherwise |
| 2 | no two settings share a variable | unit | two known settings map to one variable |
| 3 | the warning names what nothing reads, and suggests the near name | unit, subprocess | a known variable warns, an unknown one does not, or `--silent` lets it print |
| 4 | the behavior on a real install | VM | the machine behaves otherwise |

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/fix/927-env-prefix-doubled.md` | Create | this plan |
| `cmd/internal/cli/viper.go`, its test | Modify | the mapping, its unit tests, and the collision test |
| `cmd/internal/cli`, beside `viper.go` | Create | the known names and the warning |
| `cmd/scenario/` (a new test file) and the `Makefile`'s `test-scenario` target | Create, Modify | the subprocess tests, one per program |
| `docs/architecture/10-command-line-interface.md` | Modify | the rule and the warning |

## Open Questions

1. **Graph parameters share the namespace.** `pkg/op`'s `VariableResolver` reads a parameter from `<PREFIX>_<NAME>`,
   the names the settings use, so a warning at startup cannot tell a parameter's variable from a misspelled
   setting. Offered:
   - (a) parameters move to a namespace of their own, so each warning checks an exact set: settings at startup,
     a run's declared parameters when it resolves them. Recommended: the two are different things, and the split
     makes both warnings exact;
   - (b) the warning runs at the end of a graph run, knowing that run's parameters, and stays quiet on `<PREFIX>_`
     names a run without a graph cannot judge;
   - (c) the warning covers only `DEVLORE_` names and the doubled spellings.

## Related Documents

- [#927](https://github.com/NobleFactor/devlore-cli/issues/927) -- the issue and its ruling
- [#1010](https://github.com/NobleFactor/devlore-cli/issues/1010) -- lane 24: the shared root's flags as global
  settings, with #1011 ([#1011](https://github.com/NobleFactor/devlore-cli/issues/1011))
- [10-command-line-interface.md](../../architecture/10-command-line-interface.md) -- the precedence, and the rule
- [926-scope-flag.md](../task/926-scope-flag.md) -- lane 7, the lane before this one
