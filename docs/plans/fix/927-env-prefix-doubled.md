---
title: "Lane 8: a program setting's variable is the prefix plus the bare key; a variable naming nothing is warned of"
issue: https://github.com/NobleFactor/devlore-cli/issues/927
status: chartered
created: 2026-10-02
updated: 2026-10-03
---

# Plan: Lane 8 of the writ lifecycle schedule

## Summary

Lane 8 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), in PR C, which opens after it and lane 33.
Ruled on #927 (2026-09-23): **a setting's environment variable is the prefix plus the bare key**; the key's program
segment is dropped when the prefix already names the program, and the doubled spelling is not honored. That is how
`aws`, `gh` and `docker` name theirs. Planning it on 2026-10-02, the owner ruled the shared root's flags (`--verbose`,
`--dry-run` and the rest) **global settings**, built in lane 24
([#1010](https://github.com/NobleFactor/devlore-cli/issues/1010)) with `DEVLORE_` variables, and #1011 goes with them.
This lane keeps the settings only one program has, adds a test that no two settings share a variable, has the
variable resolver read the `<program>.variable` section through the config system and nothing else, and adds **a
warning for a variable that names nothing**. It also takes
[#1022](https://github.com/NobleFactor/devlore-cli/issues/1022), lane 37: **dry-run belongs to the runtime**, and
today `lore deploy --dry-run` runs every provider method. The variables family's surfaces, `--variable` and
`<program>.variable`, are [#1023](https://github.com/NobleFactor/devlore-cli/issues/1023), lane 38, in PR C′, and the
name-keyed families take singular keys ([#1024](https://github.com/NobleFactor/devlore-cli/issues/1024)). It is
independent of lane 33's named roots.

## Issue 927

Bug, epic WritDeployment (#447), feature #762; #916 lane 8, PR C. Its acceptance, written around `WRIT_VERBOSE`, is
rewritten to this plan, since `--verbose` is now a global setting.

## Issue 1022

Bug, epic UnifiedConfiguration (#441), feature #456; #916 lane 37, done in this lane, PR C. Found 2026-10-02 while
enumerating graph variables for the owner; read in the code, not reproduced.

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
6. **Graph variables get a namespace of their own**: "(a), plan chartered." Reopened the same day by rulings 8 and 9,
   then settled by rulings 11 to 13: the namespace is the variables family's, `<PREFIX>_VARIABLE_<NAME>`.
7. **"Setting" names the whole chain.** "I would like to establish a shorthand for talking about flags => environment
   variables => configuration settings. To me they are all settings arranged in a hierarchy. configuration settings
   have defaults overridden by config which is overridden by environment variable values which are overridden by
   flags. So when I say setting, I'm talking about that chain." This plan uses the word that way.
8. **Settings and variables are orthogonal.** "settings are different than variables in the context of a
   RuntimeEnvironment. variables are declared by reference and bound at execution time. settings are orthogonal."
9. **Dry-run belongs to the runtime.** "I agree that dry_run belongs to the runtime and will add that other settings
   do too. verbosity comes to mind." The runtime applies it whether a provider method is called in a graph or outside
   one; no graph reads it; lore's `package.dry_run` is withdrawn; providers and commands stop reading it.
10. **#1022 joins this lane**: "log the defect and include it in lane 8." It is lane 37 on #916.
11. **Settings stay out of the runtime variable space.** "Until we have specific use cases with a syntax for
    introducing settings (as i've now defined settings) they will not be entered into the runtime variable space.
    Bindings are entered into the runtime variable space using a syntax similar to what make uses."
12. **The variables family.** "this seems like a big hole that was meant to be filled exclusively by the
    VariableResolver. It is the logical place. the VariableResolver needs access to settings. it's domain is the
    variable space. It should not need to do more than resolve the set of variables defined by the current
    application. ... I propose --variable NAME=value." Its names, ruled "yes", follow segments': `--variable
    NAME=value`, `<PREFIX>_VARIABLE_<NAME>` and `<program>.variables` (singular by ruling 15), which `writ.vars`
    becomes. A long value has no file form until a use case asks for one, ruled "(a)". "our shipping apps (lore,
    star, and writ) and devlore-test are the apps that should carry variables. none other."
13. **The work splits** (its first half superseded by ruling 17), ruled "(c)": this lane moves the resolver's
    environment lookup to `<PREFIX>_VARIABLE_<NAME>` and drops its lookups of settings; the surfaces are #1023,
    filed as a feature on the owner's "yes" and scheduled as lane 38 in PR C′, with #975 and #683 as lanes 39 and
    40.
14. **The collision test reads the code's viper keys**, ruled "(a)": it also gathers, from the source, the keys the
    code reads through viper, and names lore's three model pairs as lane 24's exception, which lane 24 deletes.
15. **The name-keyed families take singular keys**: "whatever we do, we should do it to scopes, segments, and
    variables", then "yes, singular for all three": `writ.scope` and `writ.segment`
    ([#1024](https://github.com/NobleFactor/devlore-cli/issues/1024)), and `<program>.variable` (#1023). Under this
    lane's rule, `writ.variable.<NAME>` reads `WRIT_VARIABLE_<NAME>`: the file key and the environment variable are
    one setting chain.
16. **The design documents, the plan and the issues follow the full scope**: "update the design docs, the plan, and
    the issues associated with the full scope of this work." Done 2026-10-03 (Requirement 7).
17. **Variables are a section, read through the config system** (2026-10-03). Offered a resolver that reads the
    environment itself and a warning fed by a hand-kept table, the owner: "we are using the config system to read
    variables "section". the sources of these settings as we've discussed MANY times is...i was going to tell you.
    you tell me. we are redesigning config, but the pattern will always be the same." Every value a program reads
    from the user is a setting in a section, resolved by one chain: default < configuration file < environment
    variable < flag. Variables are the `<program>.variable` section: for each name a graph declares, the resolver asks
    the config system (viper now, devconfig after the redesign) for `<program>.variable.<NAME>`, beneath values the
    run's caller supplies and above the declared default. It reads no environment variable, flag map or configuration
    map itself, and the environment layer, `<PREFIX>_VARIABLE_<NAME>`, comes from the section by this lane's naming
    rule. Known names come from the config system, never from a side table. This supersedes ruling 13's first half;
    #1023 binds `--variable` into the section, renames `writ.vars`, and points templates at it.

The owner also said, of how code reaches settings: "the real point of access for providers and starlark code is
config. That work is TBD." This lane does not build that access.

## Goals

1. A program setting reads from the program's prefix plus the key without the program's own segment: `writ.repo`
   from `WRIT_REPO`, and a nested `writ.deploy.conflict` would read `WRIT_DEPLOY_CONFLICT`.
2. The doubled spelling, `WRIT_WRIT_REPO`, is not honored.
3. A hyphen in a setting's name becomes `_` in its variable.
4. No two settings share a variable, by test.
5. A graph variable is read from the `<program>.variable` section through the config system, and the variable
   resolver reads nothing else.
6. A variable whose name begins with a program's prefix or `DEVLORE_`, and that nothing reads, draws a warning
   naming it.
7. Every dry run reaches the runtime's skip, whichever layer of the setting asked for it, and nothing but the runtime
   reads dry-run (#1022).
8. The design pages state the rule, the warning, and the line between settings and variables.
9. Every Go file the lane touches passes `star lint go-style`.

## Current State

Read 2026-10-02 at `72a73e40`; the rows on `viper.go`, settings read by code, graph variables, the runtime's dry-run,
and 2.1 and 2.5 at `d24bff32`.

| Component | Status | Notes |
| --- | --- | --- |
| `cmd/internal/cli/viper.go` `InitViper` | ✓ | Fixed by phase 2 (`50e03403`). Before it: `SetEnvPrefix(prefix)`, `AutomaticEnv`, and a replacer from `.` to `_`; a key already namespaced under its program reads the doubled name, `WRIT_WRIT_REPO`, and devlore-test's `verbose` reads `DEVLORE_TEST_DEVLORE-TEST_VERBOSE`, which a POSIX shell cannot export. The doc comment states the ruled form, which the code does not do, and its example `WRIT_VARS_USER_NAME` names a variable nothing reads, since `writ.vars` is read as one map. `InitViper`'s error names a field that does not exist (`ViperConfig.ReceiverName`), and `BindFlags`' reads `failed to starlarkbridge flag`, leftovers of a rename sweep. No test covers the file; `star lint go-style` finds 5 violations, all missing doc sections |
| settings read by code | ⚠️ | through viper: `writ.repo`, `writ.vars` and `writ.scopes` (maps, read whole, so no variable reaches their entries), `writ.targets` (`IsSet`, for its refusal), `writ.dry-run`, `writ.verbose`, `lore.dry-run`, `lore.verbose`, and `lore.model.provider`, `.endpoint`, `.api_key` and `.model`. Once the prefix is not doubled, `lore.model.provider` and the `--model-provider` flag's `lore.model-provider` both read `LORE_MODEL_PROVIDER`, as do the endpoint and API-key pairs; and `--model`'s key `lore.model` is also their parent. Lane 24 (#1010) removes both sides; ruling 14 |
| the shared root's flags | lane 24 | bound per program today (`writ.verbose`, `writ.dry-run`); lane 24 makes them global |
| graph variables | ⚠️ | declared by reference (`plan.variable(...)`), gathered from the nodes by `Graph.Parameters()`, and bound at the run's start by `pkg/op`'s `VariableResolver` from override, flag, environment (`<PREFIX>_<NAME>`, the names the settings use), configuration and default; the middle three are a setting's layers. No shipped graph declares one: writ builds its graphs in Go without variable bindings, lore's packages take only the reserved `package` and `phase`, and star's commands take their flags as `run` arguments. devlore-test's fixtures declare seven (`dest_dir`, `dest_path`, `source_path`, `layer`, `mode`, `greeting` and `items`; `item` is bound per gather iteration), and `pkg/op`'s tests five. star's providers register `config` and `command_tree`, which star supplies as overrides; their path skips the environment |
| the runtime's dry-run | ❌ | `Application.DryRun()` reads `dry_run` from the flag map (`pkg/application/application.go:106`); `action.Do` and its two siblings skip on it, and the process runner takes it. lore and devlore-test hand it `dry-run`, so `lore deploy --dry-run` runs every provider method; writ returns before running (#853). A call made outside a graph never checks (`pkg/op/starlarkbridge/go_receiver.go:933`): star's setup provider checks for itself, the file provider does not, and `shell.exec` bypasses the process runner (#800). lore hands scripts `package.dry_run`, which no package reads. `RuntimeEnvironmentConfig` (`pkg/op/runtime_environment.go:809`), the runtime's section, holds dry-run, the conflict policy and the backup suffix, with a TODO to move the dry-run readers onto it |
| variables read by name | ✓ | `DEVLORE_VERBOSITY`, `DEVLORE_DRY_RUN`, `DEVLORE_MODEL_*` and `DEVLORE_REGISTRY_*` (`cmd/internal/config`), `DEVLORE_PAGER`, `DEVLORE_VERSION`, `DEVLORE_REGISTRY` (devlore-index) and `WRIT_SEGMENT_<NAME>`; the installers read `DEVLORE_BASE`, `DEVLORE_TEAM`, `DEVLORE_PERSONAL`, `DEVLORE_TOOLS` and `DEVLORE_VERSION` |
| `config get` | n/a | reads the configuration file, not viper, so it shows no variable's value |
| `10-command-line-interface.md` | ❌ | states the precedence (flags, then the environment, then configuration) but not how a variable is named |
| `2.1-typed-slots.md`, `2.5-lifecycle-pipeline-construction.md` | ❌ | say a graph variable resolves from flags, configuration and the environment |
| viper v1.21.0 | ready | upper-cases `PREFIX_key` before applying the key replacer (`mergeWithEnvPrefix`, then `getEnv`, `viper.go:418`, `:442` and `:1231`), so the doubled segment a replacer must drop sits at the start of the name |

## Requirements

### Requirement 1: the mapping

For a program whose prefix is `P`, a key `<program>.<rest>` reads `P_<REST>`: the rest upper-cased, dots and hyphens
as underscores. The doubled `P_<PROGRAM>_<REST>` is not honored. The rule lives in one place, `InitViper`'s key
replacer, whose first pair turns the doubled prefix `P_<PROGRAM>.` into `P_`; viper prefixes the key before it
replaces, so the doubled prefix sits at the start of the name, where the pair removes it. `InitViper`'s doc comment
states the rule.

### Requirement 2: the collision test

A test maps every known setting (flags, defaults, documented settings, and the variables read by name) to its
variable, and fails if two settings share one, or if one maps into a reserved family: `<PREFIX>_VARIABLE_`, the
variables family's (#1023), and `WRIT_SEGMENT_`, the segments'. It gathers the keys the code reads through viper from
the source, and fails on one it cannot read (ruling 14). Lore's three model pairs share `LORE_MODEL_PROVIDER`,
`LORE_MODEL_ENDPOINT` and `LORE_MODEL_API_KEY` with the `--model-*` flags until lane 24 (#1010) removes both sides;
they are the test's one exception, and the test fails once the exception no longer holds, so lane 24 deletes it.

### Requirement 3: the resolver reads the variable section

For each variable a graph declares, `VariableResolver` binds the value the run's caller supplies, else the config
system's `<program>.variable.<NAME>`, resolved through the setting chain, else the declared default (ruling 17). A
value decodes as today (`envValue`, then `op.Convert`). Its own lookups go: the flag map, the configuration map, and
`<PREFIX>_<NAME>` in the environment. The application hands the resolver the config system's section reader instead
of those maps. The missing-variable message names the key and the variable it tried. `pkg/op`'s example and tests,
and devlore-test's builtins and fixtures that feed the old lookups, change with it. The family's flag, `--variable`,
and the rename of `writ.vars` are #1023's.

### Requirement 4: the warning

A variable whose name begins with a program's prefix or `DEVLORE_`, and that names no key the config system knows,
draws a warning through the narrator, so `--silent` quiets it. The known keys come from the config system (ruling
17): its bound flags, the keys it holds, and the schema's documented settings, mapped by Requirement 1's rule; beside
them, the variables read by name and the installers' variables, which Requirement 2's source scan keeps listed. A
name in `<PREFIX>_VARIABLE_` is an entry of the variable section, and is checked when a run binds its variables,
against the variables the run declares.

The warning names the variable and, when one is near, the known name it resembles: `WRIT_WRIT_REPO` suggests
`WRIT_REPO`, and `DEVLORE_DRYRUN` suggests `DEVLORE_DRY_RUN`.

### Requirement 5: dry-run belongs to the runtime (#1022)

- Every dry run reaches the runtime's skip, whether the setting was set by the file, the environment or the flag.
- A provider method called outside a graph is skipped under dry-run, as one in a graph is.
- Nothing but the runtime reads dry-run: `package.dry_run` is withdrawn, star's setup provider stops checking, and no
  program hands dry-run to the runtime through the variables' flag map. How the runtime receives it is open question
  1; the conflict policy, carried the same way today, goes with it.
- writ's commands still return before running under `--dry-run`; open question 2.

### Requirement 6: the tests

- **Unit:** the mapping, for each program's prefix: a plain key, a nested key, a hyphenated key, and the doubled
  spelling refused; the collision test; the resolver binding from the caller, `<PREFIX>_VARIABLE_<NAME>` and the
  default, and from no flag, configuration or `<PREFIX>_<NAME>`; the warning's known names and its nearest-name
  suggestion; a graph run and a call outside a graph, under dry-run, invoke no provider method.
- **devlore-test:** the fixtures that exercise the resolver, against `<PREFIX>_VARIABLE_<NAME>`.
- **Subprocess, one per program on the shared root:** an unknown variable with the program's prefix draws the
  warning on standard error, the doubled spelling of a known setting draws it naming the bare one, and under
  `--silent` neither prints; a dry run asked for by the flag and by the environment changes nothing. They run beside
  the self-install scenarios in `cmd/scenario`, under `make test-scenario`.

### Requirement 7: the pages

`10-command-line-interface.md` states the rule in one line, beside the precedence: a program setting's variable is
the program's prefix plus the bare key, and a global setting's is `DEVLORE_` plus the key, which lane 24 builds. It
states the warning too. 2.1 and 2.5 state the line between settings and variables and point to the variables family
(#1023), and `configuration.md` states that dry-run is the runtime's setting.

Written ahead of the code on 2026-10-03, at the owner's direction (ruling 16): `10-command-line-interface.md`
§ `--scope` and §11 (the setting chain, the naming rule and the warning, settings and variables, the families
singular), `configuration.md` § Resolution, § Variables and the runtime section, `2.1-typed-slots.md` § Variables,
`2.5-lifecycle-pipeline-construction.md` § Construction, and `3.5.7-template-provider.md`'s `Env`, each status page
recording what the code does not yet do. Phase 5 checks them against what this lane builds.

### Requirement 8: the style gate

Every Go file this lane touches passes `star lint go-style` in the lane's commit.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed with the owner and chartered: "(a), plan chartered. go on phase 2." (2026-10-02).
  Rulings 7 to 14, #1022, and the split that made #1023 were added the same day.

### Phase 2: The mapping and the collision test

- [x] Requirements 1 and 2, with their unit tests: the mapping in `50e03403`, and the collision test,
  `cmd/internal/cli/environment_test.go`, with this box. The mapping is exported as `cli.EnvironmentPrefix` and
  `cli.EnvironmentVariable`; the test's one exception is lore's three model pairs (ruling 14), which lane 24 deletes.

### Phase 3: The resolver and the warning

- [ ] Requirements 3 and 4, and Requirement 6's tests for them.

### Phase 4: Dry-run belongs to the runtime (#1022)

- [ ] Requirement 5, and Requirement 6's tests for it.

### Phase 5: The pages and the gate

- [ ] Requirements 7 and 8; `make check` and `make test-scenario` green.

### Phase 6: The VM

On `danoble-ud24-1.local`, with the build installed: `WRIT_WRIT_REPO=x writ version` warns and suggests `WRIT_REPO`,
an unknown `LORE_` and `STAR_` variable each draws the warning, and `lore deploy --dry-run` of a package not yet
installed changes nothing. The Windows box is out of service; CI's Windows jobs are the Windows proof.

- [ ] `danoble-ud24-1.local` (linux/arm64)

### Phase 7: Closure

- [ ] The lane's commits on this branch. PR C opens after lane 33, with `Closes #927` and `Closes #1022`.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | the mapping, for each program's prefix | unit | a key reads the doubled name, or a nested or hyphenated key maps otherwise |
| 2 | no two settings share a variable, and none falls in a reserved family | unit | two known settings map to one variable, or one maps into `<PREFIX>_VARIABLE_` or `WRIT_SEGMENT_` |
| 3 | the resolver reads the variable section | unit, devlore-test | a variable binds from the flag map, the configuration map or `<PREFIX>_<NAME>`, or a layer of `<program>.variable.<NAME>` goes unread |
| 4 | the warning names what nothing reads, and suggests the near name | unit, subprocess | a known variable warns, an unknown one does not, or `--silent` lets it print |
| 5 | a dry run invokes no provider method | unit, subprocess | a provider method runs under dry-run, in a graph or outside one, whichever layer set it |
| 6 | the behavior on a real install | VM | the machine behaves otherwise |

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/fix/927-env-prefix-doubled.md` | Create | this plan |
| `cmd/internal/cli/viper.go`, its test | Modify, Create | the mapping, exported as `EnvironmentPrefix` and `EnvironmentVariable`, its unit tests, and the two error messages |
| `cmd/internal/cli`, beside `viper.go` | Create | the collision test, in package `cli_test` because it builds the four programs' roots; then the known names and the warning |
| `pkg/op/variable_resolver.go`, its test, and `pkg/op/variable.go` | Modify | the resolver reads the `<program>.variable` section through the config system |
| `cmd/devlore-test/devloretest/test_context.go`, `runner.go`, and the fixtures `test_writ_adopt_origin_full.star` and `test_writ_adopt_precedence.star` | Modify | the builtins and fixtures that feed or exercise the resolver |
| `pkg/op/action_types.go`, `pkg/op/starlarkbridge/go_receiver.go` and `pkg/op/runtime_environment.go` | Modify | the runtime applies dry-run in both paths, from one place |
| `pkg/application/application.go`, its test | Modify | dry-run leaves the variables' flag map |
| `cmd/lore/lore/commands.go` and `cmd/lore/lore/package_context.go` | Modify | lore hands the runtime its setting; `package.dry_run` withdrawn |
| `cmd/writ/writ/deploy/plan.go`, `upgrade/upgrade.go`, `decommission/decommission.go` and `secret/encrypt.go` | Modify | writ hands the runtime its settings the same way |
| `cmd/star/provider/setup/provider.go` | Modify | its own dry-run checks go |
| `pkg/op/provider/plan/provider.go` and `pkg/op/provider/file/provider.go` | Modify | `plan.spec`'s dry-run for a sub-run, and the conflict policy, from the runtime's settings |
| `cmd/scenario/` (a new test file) and the `Makefile`'s `test-scenario` target | Create, Modify | the subprocess tests, one per program |
| `docs/architecture/10-command-line-interface.md` and `configuration.md`, and their status pages | Modify | the rule, the warning, the families singular, and dry-run as the runtime's setting |
| `docs/architecture/2.1-typed-slots.md` and `2.5-lifecycle-pipeline-construction.md`, and their status pages | Modify | the line between settings and variables |
| `docs/architecture/3.5.7-template-provider.md` and its status page | Modify | `Env` reads the variable space (#683) |

## Open Questions

1. **How the runtime receives dry-run** (#1022), with the settings that join it, such as verbosity and the conflict
   policy. Offered:
   - (a) the program resolves them through the chain and supplies them as the runtime's section,
     `RuntimeEnvironmentConfig`, as it supplies the platform (`WithPlatform`): #694's rule, that the framework reads
     nothing and the application supplies everything. Recommended: a typed field has no key to misspell, the value is
     the resolved setting whichever layer set it, and no setting rides in the variables' sources;
   - (b) the resolved value under one key in the variables' flag map: the smallest change, but a setting stays among
     the variables' sources, which ruling 8 separates.
2. **writ's early return under dry-run.** Ruling 9 has the runtime apply dry-run, so writ's commands would stop
   returning before they run. #853, in writ's thread, already asks for the dry run to run the pre-flight. Offered:
   - (a) #853 takes it, in its own lane. Recommended: it changes what writ's dry run prints, and #853's acceptance
     says how;
   - (b) this lane takes it.

## Related Documents

- [#927](https://github.com/NobleFactor/devlore-cli/issues/927) -- the issue and its ruling
- [#1022](https://github.com/NobleFactor/devlore-cli/issues/1022) -- dry-run belongs to the runtime; lane 37
- [#1023](https://github.com/NobleFactor/devlore-cli/issues/1023) -- the variables family; lane 38, PR C′
- [#1024](https://github.com/NobleFactor/devlore-cli/issues/1024) -- scopes and segments take singular keys
- [#1010](https://github.com/NobleFactor/devlore-cli/issues/1010) -- lane 24: the shared root's flags as global
  settings, with #1011 ([#1011](https://github.com/NobleFactor/devlore-cli/issues/1011))
- [#694](https://github.com/NobleFactor/devlore-cli/issues/694) -- how configuration reaches a run
- [#853](https://github.com/NobleFactor/devlore-cli/issues/853) -- writ deploy's dry run and the pre-flight
- [10-command-line-interface.md](../../architecture/10-command-line-interface.md) -- the precedence, and the rule
- [configuration.md](../../architecture/configuration.md) -- settings, their layers, and the runtime's section
- [2.1-typed-slots.md](../../architecture/2.1-typed-slots.md) and
  [2.5-lifecycle-pipeline-construction.md](../../architecture/2.5-lifecycle-pipeline-construction.md) -- graph
  variables and their sources
- [926-scope-flag.md](../task/926-scope-flag.md) -- lane 7, the lane before this one
