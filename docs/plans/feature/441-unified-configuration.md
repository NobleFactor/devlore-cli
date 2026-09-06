---
title: "Thread 4 — Unified configuration: the design is settled, the tree does not yet agree"
issue: https://github.com/NobleFactor/devlore-cli/issues/441
status: draft
created: 2026-09-01
updated: 2026-09-02
---

# Plan: Thread 4 — Unified configuration

## Summary

Configuration is the fourth thread of work, alongside the CLI output conventions (#740), resource management
(`Epic:ResourceModel`), and the writ lifecycle surface (#762). Its design is settled and its implementation has
barely begun, and that gap is not local: **two other threads depend on behavior this design promises and the tree
does not yet provide.** This plan carries the thread's status, the cross-thread dependencies, and the observations
that justify scheduling it — not the sequencing, which already exists at
[`phase-8/configuration.md`](../extract-starlark-from-op/phase-8/configuration.md) and remains the implementation
plan of record.

**The design is king.** [`configuration.md`](../../architecture/configuration.md) carries the full model. This
document does not restate it and must not drift from it.

## Goals

1. **Schedule the thread** — name what is landed, what is not, and what other threads are waiting on.
2. **Record the unknown-key finding** — the silence that #762's migration note depends on is designed away but
   not yet built, and the exemption that makes the answer conditional is written down here rather than
   rediscovered.
3. **Keep one implementation plan** — sequencing stays in `phase-8/configuration.md`; this plan cross-references
   it and does not fork it.

## Current State

Taken from [`configuration.status.md`](../../architecture/configuration.status.md), design draft 2026-06-12,
**implementation not started** beyond the foundation.

| Component | Status |
| --- | --- |
| Design — distributed participation, recursive tree, two announcement paths, per-key overlay with set-by sidecar, declared-type instantiation | Landed |
| `pkg/devconfig` foundation — `Config`, `Section` + `SectionBase`, `DataSection`, `SectionSpec`, `SectionConstructor`, `SettingSourceKind`, accessors, closed `toStarlark` | Landed (`config.go`) |
| Announcement — `AnnounceSection` / `AnnounceSectionSpec`, loader read API; first owner `op.RuntimeEnvironmentConfig` | Landed (`registry.go`) |
| **Section definition — Go-typed path (reflect over a struct) and data path (tagged `defaults:`)** | **Not started** |
| Modular loader (koanf) + staged overlay; `${…}` Converter for variable expansion | Not started |
| Owner-located sections — `signing`, model, registry | Not started (`op` runtime landed) |
| Unify `cmd/star/config` onto `devconfig` | Not started |
| Retire `cmd/internal/config` and the package-global `viper` reads | Not started |
| `Config`'s own starlark face (boundary / star-unification) | Not started |
| `config` command surface — one set on the four programs (ruled 2026-09-02) | Landed for `lore`, `writ`, `devlore-test` on the shared root; `star` joins in #743 with `show` and `sync` beneath |

## Observations

### Unknown keys are designed to be loud, and the mechanism is unbuilt

The design settles this three times over:

- [`configuration.md:489`](../../architecture/configuration.md) — "One key→field table per Go section type
  (reflect once over the struct, matching yaml tags) maps layer keys to fields; the kv variant needs none — its
  keys *are* its storage. **Unknown keys in a layer (a typo'd setting name) are detected here and reported.**"
- `configuration.md:137` — "a section's struct fields stay reflection-driven (**an unknown member key is a loud
  error**)"
- Restated at `:737`, `:744` and `:771` — "unknown key → loud error (a schema typo)".

That detection lives inside exactly one status box, and it is unticked: **Section definition — Go-typed path
(reflect over a struct)**. So the behavior is designed and sequenced, and today a typo'd setting name is ignored
in silence.

### The kv exemption makes the answer conditional, and correctly so

The same line exempts one shape: the kv variant "needs none — its keys *are* its storage". A section whose keys
are user-defined names cannot have unknown ones, because there is no schema to be unknown against.

This is the right rule, and it means the answer to "will my typo be caught?" depends on *where* the typo is:

| Where | Caught? | Why |
| --- | --- | --- |
| A member of a struct-shaped section (`writ.targets`) | Yes, once the Go-typed path lands | The struct's key→field table has no `targets` field |
| A key inside a kv container (`writ.segments.ROEL`) | No, by design | Segment names are user-defined; there is no schema to violate |

### What #762 is actually waiting for

[`762-lifecycle-scopes.md`](762-lifecycle-scopes.md) records a migration cost: `writ.targets` stops being read,
and "nothing validates unknown keys today, so a stale `writ.targets` would be ignored in silence."

`writ` is a struct-shaped section with named members — `segments`, `scopes`, `vars` — so a stale `writ.targets`
*is* an unknown key at the struct level and *will* be caught. The safety #762 wants therefore arrives with the
Go-typed section definition and not before. Until then the release note is the only protection, which is why
#762 states it.

This is a **cross-thread dependency, not a reordering argument**: #762 ships with a release note either way.

### The command surface was ruled before the fold

Chartering #743 on 2026-09-02 forced the question, because `star` moving onto the shared root would have carried two
commands named `config`: the shared one and the group its `ConfigShow` and `ConfigSync` extensions create. Ruled:
**one set on all four programs** — `edit`, `get`, `list`, `path`, `schema`, `set`, `unset`, `validate` — with
`star`'s `show` and `sync` attached beneath. The spec records it under
["The command surface"](../../architecture/configuration.md#the-command-surface--one-set-on-four-programs).

What the ruling does **not** do is make the subcommands agree on what they read. `star config get` reads the XDG
tree; `star config show` reads the `star.yaml` hierarchy. That is this thread's fold — "Unify `cmd/star/config` onto
`devconfig`" in the table above — and until it lands, `star`'s `config` is one command over two sources. The
command surface is uniform first; the model beneath it becomes uniform here.

The same day, [#780](https://github.com/NobleFactor/devlore-cli/issues/780) made `self install` uniform across the
four programs: each produces its own default config, man pages and completions, and its manifest claims only what
it wrote. The seeded default config is the file the "Builtin as runtime floor" question demotes to overrides once
the floor is constructed at runtime.

### The thread has no schedule

`Epic:UnifiedConfiguration` (#441) holds ten open issues and appears in none of the other three threads' work.
It also **blocks phase 3.3b of the Windows campaign** — `document.Write` taking a root was deferred by ruling to
land with this work and the codec. A thread that blocks other work and is scheduled nowhere is the finding this
plan exists to record.

### The concrete serialization target

[`441-unified-configuration.case-study.yaml`](441-unified-configuration.case-study.yaml) is an
illustrative full-hierarchy configuration, written 2026-06-17 to settle the devconfig model against
something concrete. Values are fabricated; the shape is the point.

It covers what a smaller example omits: all three reserved resolver-level keys, the app-by-profile
recursion in `applications.lore.profiles` — a layer inside a layer, and where an overlay implementation
goes wrong — typed leaves beside bare kv maps, a brokered provider beside an unbrokered one, and an
application overriding nothing so inheritance is exercised.

`configuration.md`'s own case study covers the elevation section alone. This is the whole tree.

**It has not been re-verified against design changes since June**, and `elevator` was renamed to
`elevation` when it entered the repository, per [#679](https://github.com/NobleFactor/devlore-cli/issues/679).
Re-verifying it is this thread's first task, because a stale target is worse than none.

## Implementation Phases

Sequencing is **not** duplicated here. The implementation plan of record is
[`phase-8/configuration.md`](../extract-starlark-from-op/phase-8/configuration.md), whose iteration loop is:
baseline `pkg/devconfig` → schema for first owners → importing a package registers its sections → test, debug,
refine, return to schema.

### Phase 1: Record the thread (status: in progress)

- [x] Status transcribed from `configuration.status.md` and the unbuilt boxes named
- [x] The unknown-key finding recorded, with the kv exemption that qualifies it
- [x] The #762 dependency stated as a cross-reference rather than a reordering
- [x] The `config` command-surface ruling of 2026-09-02 recorded in the spec and here, with the two-source
      state of `star`'s `config` named as this thread's fold
- [ ] `phase-8/configuration.md` reviewed for drift against the current design, and its `status: draft`
      reconciled with what has landed since 2026-07-16
- [ ] The case study re-verified against the current design — it predates every change since
      2026-06-17, and nothing has checked it since

### Phase 2: Not yet scheduled

The remaining work is the status table's unticked rows, sequenced by the implementation plan. It is deliberately
left unscheduled until threads 1 and 2 complete, per the agreed thread order.

## Test Plan

No code changes in Phase 1; the test plan belongs to the implementation plan and is stated there when Phase 2 is
scheduled. Recorded here so the omission is a decision rather than an oversight.

One row is worth pre-committing, because it is the finding above turned into evidence:

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | A key with no field in a struct-shaped section is reported, not ignored | unit | The key→field table accepts an unmatched key, or the loader drops it silently |
| 2 | A key inside a kv container is accepted as data | unit | The unknown-key check is applied to the kv variant, breaking user-defined names |

Row 2 exists because row 1's fix is the natural place to over-apply the rule.

## Related Documents

- [`configuration.md`](../../architecture/configuration.md) — the design of record
- [`configuration.status.md`](../../architecture/configuration.status.md) — landed versus designed
- [`phase-8/configuration.md`](../extract-starlark-from-op/phase-8/configuration.md) — the implementation plan
- [`phase-8/sops-config-discovery.md`](../extract-starlark-from-op/phase-8/sops-config-discovery.md)
- [`441-unified-configuration.case-study.yaml`](441-unified-configuration.case-study.yaml) — the
  full-tree serialization target
- [`762-lifecycle-scopes.md`](762-lifecycle-scopes.md) — Thread 3, whose migration note depends on this thread
- [`743-star-adoption.md`](743-star-adoption.md) — Thread 1; where the command-surface ruling was forced, and
  where `star`'s `config` gains the shared set beside `show` and `sync`
- Issue [#780](https://github.com/NobleFactor/devlore-cli/issues/780) — `self install` uniform across the four
  programs; produces the default config this thread's floor demotes
- Issue [#441](https://github.com/NobleFactor/devlore-cli/issues/441) — Epic: Unified configuration
- Issues #455, #456, #335, #336, #337, #280, #385, #694, #763 — the epic's open members

## Open Questions

- [ ] Scope-composition home — one shared assembly package, or per-app? Carried from
      `configuration.status.md`'s own open questions and unresolved.
- [ ] **Should we instantiate a config system with flags that it can pull from cobra/viper?** Asked
      2026-09-01 and recorded in the spec's Open questions. Source 5 of resolution is "the parsed pflag set,
      read once"; the question is whose flag set — devconfig's own binding, or a snapshot of what cobra has
      parsed and viper has bound, which is how `parseReconcileConfig` reads settings today.
- [ ] Does the unknown-key report fail the load, or accumulate and report at the end? The design says "detected
      here and reported" without saying whether one typo stops the process. A loud error at startup argues for
      failing; a user with three typos argues for accumulating.
- [ ] Do `show` and `sync` join the shared `config` set on all four programs, or stay `star`'s? Asked
      2026-09-02 and recorded in the spec's Open questions.
