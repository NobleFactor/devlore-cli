---
title: "Demo Milestone: prove the Go + Starlark APIs and the lore packaging model"
parent: "docs/plans/extract-starlark-from-op.md"
issue: TBD
status: in-progress
created: 2026-06-02
updated: 2026-06-02
---

# Demo Milestone Exit Criteria

## Summary

This milestone proves the **Go API**, the **Starlark API**, and the **lore
packaging model** end-to-end, in service of the key demo scenario: point lore at
a package set and watch `manifest → plan → prepare/install/provision/verify →
receipt` run to a verified, reproducible environment.

It is the engineering spine beneath the marketing demos — specifically
[Demo B "New Hire, Day One"](../../../../noblefactor/devlore/design/lore/03-lore-demos.md)
(deploy a manifest on a fresh machine, show the receipt, prove verification) and
[Demo C "The Archaeology Dig"](../../../../noblefactor/devlore/design/lore/03-lore-demos.md)
(a legacy script becomes `lifecycle.yaml` + `prepare.star` / `install.star` /
`provision.star` / `verify.star`). The narrative scenarios live in
[`../noblefactor/devlore/business/03-demo-script.md`](../../../../noblefactor/devlore/business/03-demo-script.md)
and [`../noblefactor/devlore/design/lore/03-lore-demos.md`](../../../../noblefactor/devlore/design/lore/03-lore-demos.md);
this document is the gate that says the spine they ride on is real.

## The gate

The milestone closes **only when every criterion below is ✅** — committed *and*
its tests green under `make test`. A criterion is ✅ only when fully closed:
partially-landed work is ⬜, never a half-credit. The full `make test` suite must
be green (criterion 16) — no sanctioned-red carve-outs survive into the demo.

| #  | Exit criterion | Proven by | ✓ |
|----|----------------|-----------|---|
| 1  | **Starlark provider projection** — file, pkg, shell, git, service, encryption, net, template, json/yaml, regexp, ui surface through codegen + `goReceiver` | 60+ `cmd/devlore-test/.../*.star` fixtures | ✅ |
| 2  | **Eager-property / scalar / kwarg projection** — `+devlore:property`, `get_method` kwargs, named scalars, `NewGoReceiver` | phase-8/eager-property-projection.md rows 6/7/9/11 | ✅ |
| 3  | **Immediate-mode execution** — provider methods run inline | `test_imm_*.star` | ✅ |
| 4  | **Planned-mode graph assembly** — `plan.assemble`, orphan detection, plan-time type-check | phase-8 steps 17 / 18 / 19 | ✅ |
| 5  | **Sealed immutable Graph API** — re-executable plan; run-state off the graph; **lore/writ consumer migration complete** — **gates Scenario 1** | phase-8 step 21 + [21-graph-immutability.md](phase-8/21-graph-immutability.md) | ⬜ |
| 6  | **Receipts** — JSON/YAML marshaling + origin/layer enrichment; per-package verify status | phase-8 13.0(d) + receipt-enrichment commits | ⬜ |
| 7  | **Compensation / rollback (saga)** | `RecoveryStack`; `test_compensation.star` | ✅ |
| 8  | **Terminal flow control** — complete / degraded / fatal / recovery | `test_flow_*.star` | ✅ |
| 9  | **Resource model** — location vs. CAS addressing, digest/etag, lifecycle state machine | phase-8 13.0(k) (all 9 providers) | ✅ |
| 10 | **Variable binding** — override→flag→config→default, origin tracking | phase-8 13.0(n) + config lazy-resolver fix | ✅ |
| 11 | **`writ adopt`** — capture working config → packages-manifest + dotfiles | `test_writ_adopt*.star`; 13.0(n) integration | ✅ |
| 12 | **Orchestration combinators** — `plan.choose` / `gather` / `wait_until` redesign | phase-8 steps 13 / 14 / 15 | ⬜ |
| 13 | **`plan.run` / `plan.load` / `plan.save`** — explicit entry point + graph round-trip | phase-8 step 16 | ⬜ |
| 14 | **Cross-platform preflight** — `platform.Detect()` vs. target | phase-8 step 16 + [#282](https://github.com/NobleFactor/devlore-cli/issues/282) | ⬜ |
| 15 | **Function values through the bridge** — starlark callbacks → typed Go funcs | phase-8 step 24 (`TestWalkTreePlanned`) | ⬜ |
| 16 | **Full `make test` green** — phase-8 PR gate | phase-8 step 23 | ⬜ |

**Legend:** ✅ met (committed + tests green) · ⬜ not met (not started, in progress, or partially landed)

## Scenario 1 — deploy docker to Linux + macOS

**Goal:** `lore deploy docker` installs and verifies Docker on macOS (`Darwin`) and
Linux (`Linux.Debian` / `Linux.Fedora`) through the lore packaging model.

**How lore adapts (NOT `plan.choose`):** lore runs `detectPlatform()` and
`registry.Resolve(name, platform)` to pick the script set — flat root scripts for
cross-platform packages (e.g. terraform), `<Platform>/<Action>/` directories for
platform-partitioned packages (e.g. `docker/Darwin/Deploy/`,
`docker/Linux.Debian/Deploy/`; distro is part of the token, `Linux.Debian` vs
`Linux.Fedora`). Each phase script is `def <phase>(package, phase):` and registers
`plan.*` invocations; **lore assembles the `op.Graph` and executes it from Go**
(`cmd/lore/lore/builder.go` `Planner.buildPackageNodes` → executor → receipt). The
author never branches on platform; feature toggles (`package.has_feature("rootless")`)
are immediate package-metadata branches.

**NOT on this scenario's path:** `plan.choose` (criterion 12 / step 13) — adaptation
is directory resolution, not a Starlark conditional — and the Starlark `plan.run`
builtin (step 16) — execution is Go-driven. The combinator backlog that dominates
the ⬜ rows above is largely irrelevant to Scenario 1.

**Gate:** criterion 5 (step 21 — sealed graph + **lore consumer migration**) is the
true blocker. Per directive, **phase-8 step 21 does not exit until Scenario 1 works
end-to-end** (see phase-8 step 21). Enabling work layered on the seal:

- lore deploy end-to-end: resolve → `detectPlatform()` → `registry.Resolve` →
  `buildPackageNodes` → executor → receipt.
- Rewrite the stale registry scripts to the current API:
  `../devlore-registry/packages/docker/Darwin/Deploy/*` and `.../Linux.Debian/Deploy/*`.
- Close the planned primitives the docker scripts flag: `platform.arch`,
  `plan.download(url, dest)`, planned `plan.file.remove`, `phase.env(...)`.
- Receipt verify status: `docker --version` matches `lifecycle.yaml`'s
  `verification.pattern`.

## Current bottom line

The **Starlark/Go API surface and immediate-mode packaging model are proven**
(criteria 1–4, 7–11). The milestone is **not closeable** until the planned-mode
spine the richer demos lean on lands — `plan.run`, the `choose`/`gather`/`wait_until`
redesign, function-value callbacks, cross-platform preflight (criteria 5, 12–15) —
and the full suite is green (criterion 16). The step-21 graph-immutability seal is
green in `pkg/op` but its `lore`/`writ` consumer migration is in flight, which is
why criterion 5 is ⬜.

## Relationship to phase-8

Most criteria are phase-8 steps; criterion 16 *is* the phase-8 PR gate (step 23).
But this milestone outlives phase-8: it is satisfied only when the demo scenario
runs end-to-end, which depends on phase-8 closing **and** the criteria above all
landing green. Phase-8 closing is necessary, not sufficient.

## Tracking issue

A GitHub tracking issue for this milestone is not yet created (creating it
requires approval). Until then, `issue: TBD`; cross-reference is the demo scenario
docs above and phase-8 (issue 275) / parent (issue 264).
