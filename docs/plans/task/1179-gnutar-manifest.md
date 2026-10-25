---
title: "The team layer's Darwin project installs GNU tar from MacPorts, and deploy always has a registry"
issue: https://github.com/NobleFactor/devlore-cli/issues/1179
status: approved
created: 2026-10-25
updated: 2026-10-25
---

# Plan: GNU tar on Darwin, and always a registry

## Summary

**The problem.** `make dist` requires GNU tar since #1163, and nothing in the team layer installs it: the team layer's only manifest, `Home/common/packages-manifest.yaml`, is `packages: []`. GNU tar is on Media-Server because it was installed by hand from MacPorts. The same gap leaves #1151's Phase 4 without a run to show: no layer manifest makes a package manager work, so no deploy shows a package manager's progress. And deploy carries a branch for a state that does not exist. `planManifests` skips every manifest with the note "Skipping N packages-manifest file(s): no registry configured" when `deploy.Config.Registry` is nil, but `writ deploy` always builds a registry from the configuration's defaults, the devlore-registry on `develop`. Only tests reach the branch. A missing registry hidden as a skipped manifest is a fallback, and a fallback is a bug here.

**The deliverable.** A new Darwin variant of the team layer's common project, `Home/common.Darwin/`, whose manifest claims `pkg:port/gnutar`. The nil-registry branch and its note are deleted, `deploy.BuildGraphs` refuses a nil registry with an error, and every test that builds a deploy passes one. The configuration spec carries the design topic and the TODO the owner raised while this issue was filed: one and only one way to set a default, an operator's config file generated from it, and one global command to get and set a value. After the merge, a deploy with GNU tar uninstalled installs it with the port command's progress live, and shows nothing under `--silent`: #1151's Phase 4.

## Issue 1179

[The team layer's Darwin project installs GNU tar from MacPorts, and deploy always has a registry](https://github.com/NobleFactor/devlore-cli/issues/1179). A task under feature [#1128](https://github.com/NobleFactor/devlore-cli/issues/1128) of Epic [#1118](https://github.com/NobleFactor/devlore-cli/issues/1118), The DevLore Broker, beside [#1151](https://github.com/NobleFactor/devlore-cli/issues/1151), whose last phase it closes.

## Goals

1. A deploy on Darwin installs GNU tar from MacPorts when it is missing, and converges with nothing to do when it is present.
2. Deploy never plans without a registry, and never says it skipped one.
3. The configuration spec states that a default is set one way only, and records today's two ways as a TODO.

## Current State

| Where | Does today |
| --- | --- |
| `Home/common/packages-manifest.yaml` | `packages: []`; the team layer has no Darwin variant of `common` |
| `cmd/writ/writ/commands.go:78` | builds the registry with `lorepackage.NewRegistry()`, which returns a client or an error, never nil |
| `cmd/writ/writ/deploy/manifest.go:85` | a nil `Registry` skips every manifest with a note |
| `cmd/writ/writ/deploy/plan.go` `BuildGraphs` | accepts a nil `Registry`; `deploy.go`'s `Config.Registry` comment says nil skips manifest planning |
| integration tests of upgrade, reconcile, decommission, adopt, readback, deploy, sops, selection and workflow verify | build a `deploy.Config` with no `Registry`; `manifest_matrix_test.go` passes `lorepackage.New("test", nil, dir)` |
| `docs/architecture/configuration.md` | rules a constructed floor, says nothing of today's two default mechanisms or of a generated config file |

## Requirements

### Requirement 1: the manifest

`Home/common.Darwin/packages-manifest.yaml` holds one claim, in the mapping form the loader accepts (#785 keeps bare strings unloadable):

```yaml
packages:
  - name: pkg:port/gnutar
```

The `port` scheme pins MacPorts and keeps the claim off the registry lookup. The file's comment names `make dist` and #1163 as the reason.

### Requirement 2: always a registry

The nil-registry branch and its note are deleted from `planManifests`, and its doc comment no longer names the nil case. `deploy.BuildGraphs` returns an error when `cfg.Registry` is nil, before it plans anything. Every test that builds a deploy passes a registry over a temporary directory, as `manifest_matrix_test.go` does, and a test pins the refusal.

### Requirement 3: the configuration spec

`docs/architecture/configuration.md` gains, already written on this branch: the open question "Where a default lives, and a generated default config file", and the subsection "TODO — one way to set a default, and one global command to get and set a value" under § Relationship to today's code. `configuration.status.md` lists both.

### Requirement 4: the golangci-lint pin

Found in Phase 2: Homebrew had moved Media-Server's golangci-lint to v2.15.0, so `make lint-all` refused to run against the v2.14.0 pin. The rule is the same version everywhere, not any particular version (the owner, 2026-10-25), so the pin moves to v2.15.0, the latest release, as part of #1020's monthly chore, and its comment states the rule. v2.15.0 finds nothing on Linux, Darwin or Windows. CI reads the pin, so CI moves with it; the other developer machines #1020 lists move by hand.

## Implementation Phases

### Phase 1: the plan lands

- [x] Reviewed with the owner, status `approved`, committed with Requirement 3's spec edits on `feature/1179-gnutar-manifest` -- 2026-10-25
- [x] #1179's documents table carries this plan's link -- 2026-10-25

### Phase 2: the manifest, the registry and the pin (Requirements 1, 2 and 4)

- [x] The manifest, the code and the tests, and the pin (Requirement 4); `make lint-all` and `make test-race` -- 2026-10-25

### Phase 3: the pull request

- [ ] `make check`, `make lint-all` and `make test-race` pass; the description says AI assisted and ends with `Signed-off-by:` and `Assisted-by: Claude Code`; the script is `~/Workspace/NobleFactor/devlore-cli.1179-gnutar-manifest.bash`
- [ ] Status `active` in the last commit, with Phase 4 named as the reason

### Phase 4: after the merge (rule 5)

The layer clone deploy reads is on `develop`, so the manifest reaches a deploy only once it merges.

- [ ] `make install`; `sudo port uninstall gnutar`; `writ deploy` installs it, with the port command's progress on the terminal as it happens; a second run with GNU tar uninstalled again shows nothing under `--silent`; `tar --version` names GNU tar
- [ ] This plan and `1151-narration-bytes.md` set `complete`, by a follow-up pull request that closes #1179 and #1151

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/task/1179-gnutar-manifest.md` | Create | This plan |
| `Home/common.Darwin/packages-manifest.yaml` | Create | Requirement 1 |
| `cmd/writ/writ/deploy/manifest.go`, `plan.go`, `deploy.go`, `manifest_matrix_test.go` | Modify | Requirement 2 |
| the integration tests named under Current State | Modify | Requirement 2 |
| `docs/architecture/configuration.md`, `configuration.status.md` | Modify | Requirement 3 |
| `Makefile` | Modify | Requirement 4 |

## Related Documents

- [1151-narration-bytes.md](1151-narration-bytes.md), whose Phase 4 this closes
- [configuration.md](../../architecture/configuration.md) § The factory and its floor, § Relationship to today's code
- [packages-manifest.md](../../guides/writ/packages-manifest.md), platform variants and the MacPorts preference

## Open Questions

1. **The demonstration uninstalls GNU tar.** `make dist` fails between `sudo port uninstall gnutar` and the deploy that reinstalls it. Nothing else on Media-Server needs it in that window.
2. **The registry's default stays in code for now.** The default is still `RegistryConfig.WithDefaults()` at the point of use. Moving it to a constructed floor is item 5 of the configuration spec's intake ledger, recorded by Requirement 3's TODO, and not this task.
