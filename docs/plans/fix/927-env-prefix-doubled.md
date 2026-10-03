---
title: "Lane 8: a setting's environment variable is the prefix plus the bare key, for every program"
issue: https://github.com/NobleFactor/devlore-cli/issues/927
status: draft
created: 2026-10-02
updated: 2026-10-02
---

# Plan: Lane 8 of the writ lifecycle schedule

## Summary

Lane 8 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), in PR C, which opens after it and lane 33.
Ruled on #927 (2026-09-23): **a setting's environment variable is the prefix plus the bare key, for every program**:
`WRIT_VERBOSE`, `LORE_VERBOSE`, `STAR_VERBOSE`. The key's program segment is dropped when the prefix already names
the program; `WRIT_WRIT_VERBOSE` is not honored. That is how `aws`, `gh` and `docker` name theirs. The lane changes
configuration naming only, and is independent of lane 33's named roots.

## Issue 927

Bug, epic WritDeployment (#451), feature #762; #916 lane 8, PR C. Lane 23
([#1011](https://github.com/NobleFactor/devlore-cli/issues/1011)) waits on it.

## Goals

1. A program's setting reads from the prefix plus the key without the program's own segment: `writ.verbose` from
   `WRIT_VERBOSE`, `writ.deploy.conflict` from `WRIT_DEPLOY_CONFLICT`.
2. The doubled spelling, `WRIT_WRIT_VERBOSE`, is not honored.
3. Every program on the shared root alike: writ, lore, star and devlore-test.
4. The CLI design document states the rule in one line, and the generated reference names each bound flag's
   variable.
5. Every Go file the lane touches passes `star lint go-style`.

## Current State

Read 2026-10-02 at `aa553b34`.

| Component | Status | Notes |
| --- | --- | --- |
| `cmd/internal/cli/viper.go` `InitViper` | ❌ | `SetEnvPrefix(prefix)`, `AutomaticEnv`, and a replacer from `.` to `_`. `BindFlags` binds each shared-root flag under `<program>.<flag>`, so `writ.verbose` reads `WRIT_WRIT_VERBOSE`. The function's doc comment already states the ruled form (`WRIT_REPO → writ.repo`), which the code does not do |
| prefixes | ⚠️ | `cmd/internal/cli/root.go`: the program's name upper-cased, `-` as `_`: `WRIT`, `LORE`, `STAR`, `DEVLORE_TEST`. devlore-test's verbose reads `DEVLORE_TEST_DEVLORE-TEST_VERBOSE` today |
| hyphenated flags | ⚠️ | `dry-run`, `model-api-key`, `model-endpoint`, `model-provider` map to names with a hyphen (`WRIT_WRIT_DRY-RUN`), which a POSIX shell cannot set: lane 23 (#1011) |
| suite-wide variables | out of scope | `cmd/internal/config/config.go` reads `DEVLORE_VERBOSITY`, `DEVLORE_DRY_RUN`, `DEVLORE_MODEL_*` and `DEVLORE_REGISTRY_*` by name, and `DEVLORE_PAGER`, `DEVLORE_VERSION` and `WRIT_SEGMENT_<NAME>` are read by name too. Lanes 24 and 25 own the model and registry ones |
| `10-command-line-interface.md` | ❌ | states the precedence (flags, then the environment, then configuration) but not how a variable is named |
| the generated reference (`cmd/devlore-docs`) | ❌ | names no variable |
| viper v1.21.0 | ready | upper-cases `PREFIX_key` before applying the key replacer (`mergeWithEnvPrefix`, then `getEnv`), so a replacer can drop the doubled segment at the start of the name and nowhere else |

## Requirements

### Requirement 1: the mapping

For a program whose prefix is `P`, a key `<program>.<rest>` reads `P_<REST>`: the rest upper-cased, dots as
underscores. A key outside the program's section, such as `pager`, reads `P_<KEY>`. The doubled `P_<PROGRAM>_<REST>`
is not honored. The rule lives in one place, `InitViper`'s key replacer, whose first pair turns the doubled prefix
`P_<PROGRAM>.` into `P_`. Viper prefixes the key before it replaces, so the pair matches at the start of the name
only. `InitViper`'s doc comment states the rule.

### Requirement 2: the tests

- **Unit:** the mapping for each program's prefix: a plain key, a nested key, a key outside the program's section,
  and the doubled spelling refused.
- **Subprocess, one per program on the shared root:** `WRIT_VERBOSE=1 writ version` behaves as `--verbose`, and
  `WRIT_WRIT_VERBOSE=1` does nothing; the same for lore, star and devlore-test. They run beside the self-install
  scenarios in `cmd/scenario`, which belong to no single program, under `make test-scenario`.

### Requirement 3: the pages

`10-command-line-interface.md` states the rule in one line, beside the precedence. The generated reference names,
beside each flag bound to a setting, the variable that sets it.

### Requirement 4: the style gate

Every Go file this lane touches passes `star lint go-style` in the lane's commit.

## Implementation Phases

### Phase 1: The plan

- [ ] This document, reviewed with the owner and chartered.

### Phase 2: The mapping and its tests

- [ ] Requirements 1 and 2.

### Phase 3: The pages and the gate

- [ ] Requirements 3 and 4; `make check` and `make test-scenario` green.

### Phase 4: The VM

On `danoble-ud24-1.local`, with the build installed: `WRIT_VERBOSE=1 writ version` narrates as `--verbose` does, and
`WRIT_WRIT_VERBOSE=1 writ version` does not; the same for `lore` and `star`. The Windows box is out of service; CI's
Windows jobs are the Windows proof.

- [ ] `danoble-ud24-1.local` (linux/arm64)

### Phase 5: Closure

- [ ] The lane's commits on this branch. PR C opens after lane 33, with `Closes #927`.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | the mapping, for each program's prefix | unit | a key reads the doubled name, or a nested or unsectioned key maps otherwise |
| 2 | each program honors the bare variable and ignores the doubled one | subprocess, `make test-scenario` | `WRIT_VERBOSE=1` does not narrate, or `WRIT_WRIT_VERBOSE=1` does |
| 3 | the reference names each bound flag's variable | unit, on `cmd/devlore-docs` | a bound flag shows no variable, or the wrong one |
| 4 | the behavior on a real install | VM | the machine behaves otherwise |

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/fix/927-env-prefix-doubled.md` | Create | this plan |
| `cmd/internal/cli/viper.go`, its test | Modify | the mapping and its unit tests |
| `cmd/scenario/` (a new test file) and the `Makefile`'s `test-scenario` target | Create, Modify | the subprocess tests, one per program |
| `cmd/devlore-docs/template.go`, its test | Modify | the variable beside each bound flag |
| `docs/architecture/10-command-line-interface.md` | Modify | the rule, in one line |

## Open Questions

1. **Hyphens (lane 23, #1011).** Of the shared root's flags, `dry-run`, `model-api-key`, `model-endpoint` and
   `model-provider` have hyphens. This lane alone leaves their variables unsettable (`WRIT_DRY-RUN`), and the
   reference would name variables a shell cannot set. Lane 23's ruling is already made: "we convert - to _ when
   mapping to environment variables and configuration setting names." Offered: (a) take lane 23 into this lane, as
   one more replacer pair, its tests, and a reference that names only settable variables; (b) keep lane 23 separate,
   and have this lane's reference name a hyphenated flag's variable only after lane 23 lands. Recommended: (a), since
   both change the same line and lane 23 waits on this one.
2. **The model settings.** Under this rule the shared root's `--model-*` flags read `WRIT_MODEL_PROVIDER` and its
   kin, while `cmd/internal/config` reads `DEVLORE_MODEL_PROVIDER`. Lane 24 (#1010) rules which governs; this lane
   changes neither.

## Related Documents

- [#927](https://github.com/NobleFactor/devlore-cli/issues/927) -- the issue and its ruling
- [#1011](https://github.com/NobleFactor/devlore-cli/issues/1011) -- lane 23, hyphens in variable names
- [#1010](https://github.com/NobleFactor/devlore-cli/issues/1010) -- lane 24, the model settings
- [10-command-line-interface.md](../../architecture/10-command-line-interface.md) -- the precedence, and the rule
- [926-scope-flag.md](../task/926-scope-flag.md) -- lane 7, the lane before this one
