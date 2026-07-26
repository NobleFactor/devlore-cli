---
title: "errcheck: assert the must-succeed classes"
issue: TBD
status: complete
created: 2026-07-26
updated: 2026-07-26
---

# Plan: errcheck — assert the must-succeed classes

Chartered follow-up 4, part b-1 (errcheck assert classes), of
[fix-windows-build-and-guide-category](fix-windows-build-and-guide-category.md).

## Summary

Of 117 uncapped errcheck findings, this change fixes the 56 whose failure indicates a bug
— converting silent discards into labeled panics via `pkg/assert`. Two functions are added
to `pkg/assert` (approved 2026-07-25):

- `Must[T any](value T, err error) T` — unwraps a (value, error) call; panics through
  `raise` on error. Go forbids mixing a context argument with a multi-value call, so Must
  carries no label; the AssertionError's captured call site supplies the location.
- `Type[T any](name string, value any) T` — comma-ok type assertion with a labeled panic,
  replacing unlabeled runtime panics of bare single-value assertions.

## Changes

1. `pkg/assert/assert.go` — `Must` and `Type` (alphabetical placement) + tests in
   `pkg/assert/assert_test.go`.
2. Flag getters (27 sites): the 25-case `flagValue` switch in `pkg/application/application.go`
   collapses to `return assert.Must(cmd.Flags().GetX(f.Name))`; the two
   `GetBool("silent")` sites in `internal/cli/root.go` and
   `cmd/devlore-test/devloretest/root.go` use `assert.Must`.
3. Single-value type assertions (19 sites): `assert.Type` with a named invariant, in
   `pkg/op/binding.go` (4), `pkg/op/node.go` (4), `pkg/op/receiver_type.go` (2),
   `pkg/op/method.go`, `pkg/op/runtime_environment.go`, `pkg/op/subgraph.go`,
   `pkg/op/starlarkbridge/converter.go`, `pkg/op/starlarkbridge/go_receiver.go`,
   `pkg/op/provider/regexp/provider.go`, `pkg/signing/signing.go`,
   `cmd/star/provider/commands/command_ref.go`, `cmd/star/provider/goast/helpers.go`.
4. Registration/transition discards (8 sites): `assert.NoError` in the four
   `NewProvider` RegisterParameter calls (`cmd/star/provider/{commands,config,goast,setup}`),
   `pkg/op/graph_executor.go` (frameworkFailure), `pkg/op/subgraph.go` (boundary
   transition), `pkg/op/provider/flow/provider.go` (degraded/failed transitions).
5. Must-succeed misc (2 sites): `uuid.Parse` of the archive-time recovery ID
   (`pkg/op/provider/file/receipt.go`) and `json.Marshal` of the event payload
   (`pkg/op/server/router.go`).

Deliberately NOT converted: `handlerStack.Unwind` in `pkg/op/graph_executor.go`'s deferred
error path (best-effort recovery cleanup — a panic there would mask the original failure;
class-5 review), and the fallback sites `NewReceiverType(reflectType, nil)` and
`platform.New(detected)` (intentional absent-tolerant flows).

## Remaining errcheck debt: 61 findings

### Class 2 — discarded-ok type assertions (24 sites, awaiting sign-off)

Provisional classification; "policy" marks the receipt-deserialization question (own
serialized documents, but read from disk — panic vs error on corruption):

| Site | Source | Provisional |
|---|---|---|
| `cmd/lore/lore/origin.go:56` | `token, _ := value.(string)` | review |
| `cmd/star/provider/commands/provider.go:44` | `t, _ := v.Value.(CommandTree)` | assert |
| `cmd/star/provider/commands/provider.go:52` | `Overrides["current_command"].(string)` | tolerant |
| `cmd/star/provider/setup/provider.go:51` | `c, _ := v.Value.(*cfg.Config)` | assert |
| `cmd/writ/writ/decommission/decommission.go:220` | `runRoot, _ := value.(string)` | policy |
| `cmd/writ/writ/deploy/deploy.go:337` | `target, _ := fields["target"].(string)` | tolerant |
| `cmd/writ/writ/deploy/plan.go:443` | `root, _ := value.(string)` | policy |
| `cmd/writ/writ/migrate/helpers.go:69` | `binding.Resolve(nil, nil).(string)` | review |
| `cmd/writ/writ/readback/readback.go:309` | `targetRoot, _ = value.(string)` | policy |
| `cmd/writ/writ/readback/readback.go:473` | `fields[key].(string)` | policy |
| `cmd/writ/writ/upgrade/upgrade.go:443` | `runRoot, _ := value.(string)` | policy |
| `pkg/application/application.go:73` | DryRun `Flags["dry_run"].(bool)` | tolerant (documented) |
| `pkg/op/action_types.go:300` | `v.Interface().(Compensator)` | tolerant (capability probe) |
| `pkg/op/graph.go:138` | `spec.Origin.(OriginBase)` | review |
| `pkg/op/provider/file/receipt.go:445` | `fields[key].(string)` | policy |
| `pkg/op/provider/pkg/receipt.go:223` | `fields[key].(string)` | policy |
| `pkg/op/provider/pkg/receipt.go:237` | `fields[key].(bool)` | policy |
| `pkg/op/provider/plan/helpers.go:71` | `kv[0].(starlark.String)` | assert |
| `pkg/op/provider/plan/helpers.go:343` | `kv[0].(starlark.String)` | assert |
| `pkg/op/provider/service/receipt.go:120` | `fields["was_running"].(bool)` | policy |
| `pkg/op/provider/service/receipt.go:121` | `fields["was_enabled"].(bool)` | policy |
| `pkg/op/recovery_stack.go:706` | `e.compensator.(Receipt)` | tolerant (variant probe) |
| `pkg/op/recovery_stack.go:716` | `e.compensator.(*RecoveryStack)` | tolerant (variant probe) |
| `pkg/platform/darwin_managers_darwin.go:49` | `kwargs["cask"].(bool)` | tolerant |

### Classes 5–7 — cleanup discards, Fprint family, fallbacks (37 sites)

Best-effort cleanup (needs the nolint-vs-logging ruling), stderr/SSE writes, and
parse-with-fallback sites, including the compensation-path discards
(`receipt.Commit`, `RestoreFile`, `catalog.VerifyExistence`) flagged for real scrutiny.

## Verification

- `make vet` — pass; `make test` — pass (full suite, twice: after the bulk edits and
  after the final missed-site fix).
- errcheck uncapped count: 117 → 61; zero remaining findings in converted files except
  the deliberately deferred `application.go` DryRun site.
- `gofmt -l` clean over `cmd`, `pkg`, `internal`.
