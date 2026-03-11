# GitHub Issues Analysis

**Analyzed:** 2026-03-10 | **Open issues:** 25 | **Repo:** NobleFactor/devlore-cli

## Bugs (2 open)

### #168: CompensateBackup silently swallows bad compensation state

**Status:** Open — bug confirmed present.

`CompensateBackup`, `CompensateMove`, and `compensateWrite` in `pkg/op/provider/file/provider.go` silently return nil when the tombstone resource is nil, instead of returning an error. All three use a nil-check pattern that hides real problems. Other Compensate methods correctly return errors on bad state.

### #173: starcode.capture().count is method, not property

**Status:** Open — bug confirmed present. Test skipped.

The reflection bridge in `pkg/op/receiver_reflect.go` wraps all exported methods as callables. Zero-argument methods like `Count()` should be exposed as Starlark properties (returning `starlark.Int`) but are exposed as `builtin_function_or_method`. Test at `pkg/op/provider/starcode/integration_test.go:31` is skipped.

## Recently Closed Bugs (7)

| Issue | Title | Resolution |
|-------|-------|------------|
| #165 | compensateWrite nil guard on undo | Intentional — inline type assertion with error is correct |
| #166 | CompensateCopy file mode | Fixed by tombstone recovery rewrite (os.Rename preserves metadata) |
| #167 | TestBuild panics — unregistered action | Fixed import paths to provider/pkg/gen |
| #169 | shell.exec missing argument for output | Fixed — output param removed, test passes |
| #170 | WalkTree callback params unsupported | Fixed — buildCallableFunc in reflection bridge |
| #171 | Planned bindings for non-error methods | Fixed — reflectedPureAction handles them |
| #172 | TestLoadIntegration — undefined: ui | Fixed — ui provider properly announced and wired |

## CLI Stubs — FalseSuccess (5 issues, 21 commands)

These commands exist but return `fmt.Errorf("<command>: not yet implemented")`. Originally returned success (silent no-op), fixed by #65 to return errors.

### #70: writ adopt --from-receipt

`runAdoptFromReceipt()` at `internal/writ/commands.go:1363` — stub.

### #71: Lore lifecycle commands

| Command | Location |
|---------|----------|
| `lore upgrade` | `internal/lore/commands.go:318` |
| `lore decommission` | `internal/lore/commands.go:332` |
| `lore reconcile` | `internal/lore/commands.go:348` |

### #72: Lore manifest commands

| Command | Location |
|---------|----------|
| `lore manifest create` | `internal/lore/commands.go:389` |
| `lore manifest validate` | `internal/lore/commands.go:412` |
| `lore manifest test` | `internal/lore/commands.go:428` |
| `lore manifest show` | `internal/lore/commands.go:442` |
| `lore manifest update` | `internal/lore/commands.go:457` |

### #73: Lore registry commands

| Command | Location |
|---------|----------|
| `lore list` | `internal/lore/commands.go:566` |
| `lore resolve` | `internal/lore/commands.go:581` |
| `lore update` | `internal/lore/commands.go:592` |
| `lore inspect` | `internal/lore/commands.go:755` |
| `lore publish` | `internal/lore/commands.go:775` |
| `lore audit` | `internal/lore/commands.go:801` |
| `lore bundle` | `internal/lore/commands.go:359` |

### #74: Writ commands

| Command | Location |
|---------|----------|
| `writ inspect` | `internal/writ/commands.go:1480` |
| `writ list` | `internal/writ/commands.go:1494` |
| `writ receipt show` | `internal/writ/commands.go:1549` |
| `writ receipt list` | `internal/writ/commands.go:1559` |

## Code Quality (3 issues)

### #75: Fix ignored errors in security-sensitive operations

**Status:** Partially resolved.

| Location | Status |
|----------|--------|
| `identity.LoadIdentities()` in `internal/writ/commands.go:604` | Still ignored (`//nolint:errcheck`) |
| `config.Load()` in `internal/model/config.go:234,328` | Still ignored (`//nolint:errcheck`) |
| `cmd.Output()` in `internal/starlark/npm.go` | **Obsolete** — file no longer exists |

### #77: --verbosity flag never implemented

**Status:** Open — not implemented.

Both `internal/lore/root.go` and `internal/writ/root.go` still use `--verbose` (bool) + `--silent` (bool). No `--verbosity quiet|normal|detailed` flag exists. The config schema defines `verbosity` but no code reads it.

### #80: golangci-lint failures (236 issues)

**Status:** Open — configuration exists, tool not installed in dev environment.

`.golangci.yaml` is properly configured with comprehensive linter set (errcheck, govet, staticcheck, gosec, gocyclo max 15, gocognit max 20, etc.). `make lint` fails because `golangci-lint` is not installed. Issue count may have changed significantly since the 236-issue count was taken.

## Architecture / Design (5 issues)

### #65: Claude Code: Errors of Judgment

**Status:** Open — meta tracking issue. Not closeable.

Tracks errors in judgment with label taxonomy (Lie, FalseSuccess, IgnoredError, DesignDefect, Incomplete). Contains verification checklist and session log. References issues #70–#80.

### #78: Config precedence tests

**Status:** Partially resolved.

`internal/cli/config_test.go` exists with `TestSharedConfigPath` and `TestUnifiedConfig_BothToolsSeeSharedSettings`. Missing: `internal/cli/viper_test.go`, `internal/lore/root_test.go`, `internal/writ/root_test.go`. CLI flag → env var → config file full precedence chain is not tested.

### #79: Unify configuration

**Status:** Open — design decision pending.

Current state: tool-specific env prefixes (`LORE_`, `WRIT_`) + shared config file (`~/.config/devlore/config.yaml`). The issue proposes unified `DEVLORE_` prefix. This is a design decision about whether per-tool or unified prefixes are correct.

### #105: Shell operation compensation via structured return contract

**Status:** Open — design topic, deferred.

Shell provider's `Exec()` at `pkg/op/provider/shell/provider.go` returns `(string, error)` — not compensable. The issue proposes a JSON stdout contract for opt-in compensation. Explicitly deferred until the core compensation model is exercised.

### #156: Audit, Reconciliation, and Recovery

**Status:** Open — design only, no implementation.

Proposes `ExecutionEvent` envelope, `Reconcilable` interface, audit ledger, and reconciliation engine. None of these types exist in code. The `RecoveryStack` at `pkg/op/recovery.go` has a `reconcileState any` field but no `Reconcilable` interface is implemented. This is a significant future feature.

## Features (8 issues)

### #64: Milestone — Automated team onboarding via lore

**Status:** Open — vision/milestone issue.

End-to-end goal: `lore onboard <url>` parses documentation, identifies tools, creates packages, installs everything. Four phases defined. No implementation.

### #66: Generate JSON schemas from Go structs

**Status:** Open — blocked by #67 (config consolidation, now closed).

Proposes using `invopop/jsonschema` to generate schemas from Go structs, replacing manually-maintained schemas in `schema/`. Blocker may be cleared — #67 is not in the open issues list.

### #68: Refactor config struct design

**Status:** Open — blocked by #67.

Three items: shared Preferences, Sources naming clarity, SOPS-based secrets (not age). Same blocker as #66.

### #82: Versioning with timestamps and commit hashes

**Status:** Open — not implemented.

No `VERSION` file, no `scripts/bump-version.sh`, no `bump-*` Makefile targets. Version is currently derived dynamically from git tags in the release workflow. The semver scheme described (rc + build metadata) is not in place.

### #83: self command (install + upgrade)

**Status:** Partially implemented.

`lore self-install` exists at `internal/cli/selfinstall.go` (generates completions, man pages, config). Added to both `lore` and `writ` root commands. `lore self upgrade` (check for and install latest LKG release) does not exist.

### #89: Product documentation

**Status:** Open — eight documentation gaps identified.

Missing guides: troubleshooting, writ+lore integration workflow, migration from other tools, receipt format/schema, audit log API, state file management, bundle format, bindgen user guide.

### #90: Starlark API documentation

**Status:** Partially implemented.

Auto-generation via `star devlore knowledge api` is complete and functional. Missing: hand-written guidance for package authors (error handling patterns, elevation, chaining, worked examples, builtins), manifest validation criteria documentation.

### #91: Windows support audit

**Status:** Open — audit not performed.

CI runs on `ubuntu-latest` only. Windows binaries are cross-compiled and distributed (.zip), but no runtime Windows testing exists. No clear support statement.

## Epics / Tracking (1 issue)

### #92: Execution graph engine

**Status:** Open — epic tracker, substantially complete but stale. Updated 2026-03-10.

The issue's "Implemented and functional" table uses old package paths (`internal/execution/` → now `pkg/op/`). Many "not yet implemented" items have been superseded:
- **Choose, Gather, Elevate**: Implemented in `internal/execution/flow/`
- **WaitUntil, Complete, Degraded, Fatal**: Implemented in `internal/execution/flow/`
- **Distro detection**: Implemented in `internal/writ/segment/detect.go`
- **Probe, Guard, Retry**: Not implemented
- **Batching optimizer**: Not implemented
- **Lore receipt format**: Not implemented
- **Registry resolver (discovery)**: Not implemented

May be closed in favor of `ARCHITECTURE-STATUS.md`.

## Recently Closed Epics

| Issue | Title | Resolution |
|-------|-------|------------|
| #76 | Logging strategy | Unified through cli.Note/Warn/Error/Success; remaining work (log levels) blocked by #77 |
| #93 | Bindgen: CLI binding generator | Code removed from codebase |

## Feature Tracking (3 issues)

### #131: packages-manifest.yaml graph builder

**Status:** Open — partially implemented.

`internal/manifest/manifest.go` parses manifests. `lore deploy @packages-manifest.yaml` parses and delegates per-package. Missing: unified graph composition that wires all package subgraphs together, `manifest.Provider.Resolve` returning `[]PackageEntry` (with features) instead of `[]string`.

### #132: RecoveryStack serialization for deferred undo

**Status:** Open — not implemented.

`RecoveryStack` exists in memory only at `pkg/op/recovery.go`. No receipt serialization of `UndoState`. No receipt chain model. No decommission-via-receipt capability.

### #148: Devlore extensions — binding functions and lifecycle phase handlers

**Status:** Open — design topic, no implementation.

No extension binding mechanism or pluggable lifecycle phase handlers exist. Open questions listed in the issue remain unanswered.

## Summary

| Category | Open | Notes |
|----------|------|-------|
| Bugs | 2 | #168, #173 |
| CLI stubs | 5 | #70–#74 (21 commands) |
| Code quality | 3 | #75, #77, #80 |
| Architecture/design | 5 | #65, #78, #79, #105, #156 |
| Features | 8 | #64, #66, #68, #82, #83, #89, #90, #91 |
| Epics/tracking | 1 | #92 |
| Feature tracking | 3 | #131, #132, #148 |
