---
step: 49
title: "Conflict-policy enforcement — {stop, skip, replace} at the file provider's write seam"
status: chartered 2026-07-15 (out of step 47 slice 1, finding 1; ruling settled the design); amended 2026-07-16 — this step owns BOTH halves: the provider-side section read and the cli-layer flag feed (the rollup's cli source has no client surface yet)
parent: ../../phase-8.md
---

# Step 49 — conflict-policy enforcement

**Chartered 2026-07-15** from step 47 slice 1's finding: `op.ConflictPolicy` is declared but read by nothing —
the runtime environment hardcodes `ConflictStop` at construction, and the file provider consults only
`BackupSuffix`. The `--conflict` surface writ inherited is therefore unenforced; the sealed write-family actions
always archive-and-replace an occupied target.

## The ruling (2026-07-15)

Two conflict dimensions exist, and only one needs a policy:

1. **Source-side collisions** (two projects/layers claiming one target) are already resolved deterministically
   at tree-build time — layer precedence, then specificity in platform order — with losers reported as
   collisions. No policy; settled machinery.
2. **An occupied target location** (`~/some-file` exists and a deploy would replace it) is what
   `op.ConflictPolicy` governs. Three values: **`stop`**, **`skip`**, **`replace`**.

## Design (settled)

1. **The enum collapses to three.** `ConflictBackup` / `ConflictOverwrite` merge into `ConflictReplace`:
   replace must ALWAYS archive the occupant to the recovery site (the receipt's pre-archive digest is what
   compensation restores from — an unarchived overwrite would break the SAGA contract), so "backup vs.
   overwrite" was a false distinction. Flag/serialized names: `stop` | `skip` | `replace`.
2. **The file provider enforces, at the write seam.** Occupation is dispatch-time state only the provider
   knows — it already lstats targets (`file.Link`) and runs the archive machinery (`prepareWrite`). The branch:
   occupied + `replace` → archive-and-replace (today's behavior); occupied + `skip` → no-op success (the
   `file.Link` "already correct" short-circuit shape); occupied + `stop` → error → the node fails and the run
   unwinds. The executor is the wrong layer (no resource knowledge); plan-time is the wrong time (occupation is
   runtime state; per-file in-graph guards would duplicate provider knowledge).
3. **The policy's home already exists**: `RuntimeEnvironmentConfig` — the framework's announced "runtime"
   config section — carries `ConflictPolicy` with builtin floor `ConflictStop`, and its TODO already directs
   consumers onto live `Application.Config` reads (the step-41 `PoliciesConfig` precedent). Enforcement = the
   provider reading the section live. The unread `RuntimeEnvironment.ConflictPolicy` field retires.
4. **Default `stop`** (the section's existing floor; the old writ default). Consequence: an enforced first
   deploy over a pre-existing foreign file refuses unless `--conflict=replace` or `skip`; the refusal names the
   flag. Not-conflicts stay no-ops under every policy: a link already pointing correctly (and, if ever wanted, a
   digest-identical copy).
5. **Writ wiring rides THIS step (amended 2026-07-16)**: step 47 slice 4 found the config rollup's cli source
   has no client-side surface yet — wiring the flag there would have meant inventing a side channel. The flag
   stays parsed (`parseDeployConfig` → `cfg.ConflictPolicy`); this step builds both halves — the provider-side
   section read AND the cli-layer feed. Until it lands, deploy's real semantics are replace-always (recorded in
   step 47).

## Scope

1. The enum collapse ({stop, skip, replace}) + the flag/serialized name mapping + retire the dead field.
2. The provider-side branch at the write seam (Link, Copy, WriteText/WriteBytes, and the sops decrypt's write).
3. Tests: per policy × occupied/vacant/already-correct; the skip tally (skipped nodes surface in
   `Trace.Summarize`); the stop unwind.

## Test plan

1. Occupied target × {stop, skip, replace}: stop fails the node and unwinds; skip leaves the occupant and the
   run succeeds; replace archives (receipt carries the pre-archive digest) and the occupant restores on unwind.
2. Vacant target and already-correct link behave identically under all three.
3. Config layering: builtin floor stop; a cli-layer `replace` wins per the rollup.
