---
title: "Fix Narrator Wiring"
status: complete
created: 2026-08-05
updated: 2026-08-05
---

# Plan: Fix Narrator Wiring

## Summary

Every tool's runtime environment silently discards narration. `op.NewRuntimeEnvironmentSpec`
pre-fills `Status` with a narrator over `sink.Discard` (pkg/op/runtime_environment.go:788),
which makes `NewRuntimeEnvironment`'s documented nil→`sink.Stderr` default (lines 162–166)
unreachable — the spec constructor violates its own documented contract. No caller anywhere
passes `WithStatus`, so the starlark `ui.note` / `ui.warn` / `ui.error` path (a passthrough
to `RuntimeEnvironment().Status`) writes to `io.Discard` in every tool, everywhere — the
reason CI's failing lint gate reports "1 lint issues" without naming the finding, and why
`star lint go` runs mute even interactively. Meanwhile the `internal/cli` package-global
narrator (the second seam) is correctly wired in lore/writ/devlore-test bootstraps but the
comment claiming "the same instance flows into RuntimeEnvironmentSpec.Status"
(internal/cli/output.go:120) describes a flow that does not exist, and star's bootstrap
ignores its own `--silent` flag.

## Current State

| Piece | State |
| --- | --- |
| `NewRuntimeEnvironmentSpec` Status pre-fill | ❌ Discard narrator (runtime_environment.go:788) — dead-contract |
| `NewRuntimeEnvironment` nil→Stderr default | ❌ Unreachable for spec-built environments |
| `WithStatus` callers | ❌ Zero repo-wide |
| lore/writ bootstrap (`internal/cli/root.go:56`) | ✅ `--silent` fork → `SetUI` |
| devlore-test bootstrap (`root.go:55`) | ✅ `--silent` fork → `SetUI` |
| star bootstrap (`cmd/star/main.go:206`) | ❌ unconditional `sink.Stderr`; `--silent` (line 184) parsed but ignored |
| star environment ↔ cli narrator | ❌ two instances; starlark path discards |

## Changes

### 1. Framework: make the documented default real

Delete the `Status` AND `Result` Discard pre-fills from `NewRuntimeEnvironmentSpec` (both
ruled in, 2026-08-05).
`spec.Status` is nil until a caller sets it; `NewRuntimeEnvironment`'s existing defaulting
(nil → narrator over `sink.Stderr`) becomes live, matching both its doc comment (line 152)
and the field's (line 772: "pass a Narrator wrapping [sink.Discard] to suppress").
Suppression becomes an explicit caller choice, never a silent default.

### 2. star: honor --silent, one instance on both seams

In `cobra.OnInitialize` (cmd/star/main.go:199–207): fork on the parsed `silent` var
(Discard vs Stderr), construct ONE narrator, install it via `cli.SetUI` AND onto the star
application's runtime environment — making the "one instance, one silent gate" comment
true. Requires environment access on `star.Application` (field is unexported): add an
`Environment() *op.RuntimeEnvironment` accessor (recommended — smallest general API), then
`runtime.Environment().Status = narrator`. Fix the stale comments (main.go:201–205, 319).

### 3. Tools: flow the bootstrap narrator into every spec

Add `.WithStatus(cli.UI())` at each spec construction (all run at RunE time, after the
bootstrap has installed the forked narrator):

1. cmd/lore/lore/builder.go:106
2. cmd/lore/lore/commands.go:249
3. cmd/devlore-test/devloretest/runner.go:258
4. cmd/devlore-test/devloretest/test_context.go:845
5. cmd/writ/writ/upgrade/upgrade.go:599
6. cmd/writ/writ/decommission/decommission.go:328
7. cmd/writ/writ/adopt/batch.go:279
8. cmd/writ/writ/verify/verify.go:260
9. cmd/writ/writ/deploy/plan.go:390
10. cmd/writ/writ/migrate/execute.go:79
11. cmd/writ/writ/migrate/helpers.go:94

(Unit-test spec constructions that rely on today's silence get an explicit
`WithStatus(status.NewNarrator(name, sink.Discard()))` — suppression stated, never
inherited.)

### 4. Documentation corrections

- internal/cli/output.go:114–128 — re-verify every claim against the new wiring.
- docs/plans/lint-gate-verdict.md fallout addendum — correct the "narrator-suppressed in
  CI's non-TTY" misattribution to the real mechanism (Discard pre-fill + unwired seam);
  the narrator never gated on TTY (that selects color only).

## Verification

- `make vet`, `make build`, `make test`; dual-GOOS lint recount at zero.
- Empirical: `./build/star lint go ./pkg/sink` narrates (notes + per-issue lines);
  `./build/star --silent lint go ./pkg/sink` is mute; a planted finding's line is named in
  the failure output (the original item-6 motivation).

## Open questions

None — `Result` (the identical dead-contract pattern) was ruled into this change on
2026-08-05: same fix, its documented nil→stdout-JSON default becomes live.

## Outcome (2026-08-05)

All changes landed as planned, Result included. Verified: vet, build, full suite green (no
test relied on the old silence); dual-GOOS recount clean; `star lint go` narrates with the
`[star]` prefix and `--silent` is fully mute; a planted finding printed as
`[star] [△] pkg/lintprobe/probe.go:6:6 unused: func probeUnused is unused` before the
failing verdict — the failing CI gate now names its findings.
