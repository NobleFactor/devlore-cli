---
step: 49
title: "Conflict-policy enforcement — {stop, skip, replace} at the file provider's write seam"
status: COMPLETE 2026-07-16 — layered enforcement landed (writ pre-flight + write-seam backstop, interim flag channel per the dry-run precedent); TWO amendments settled during implementation: the seam floor is REPLACE (in-place updates are not conflicts), and writ's cautious stop default lives in the deploy pre-flight
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
4. **Default `stop` — REFINED to layered enforcement (ruled 2026-07-16).** A seam-level stop cannot tell a
   foreign occupant from writ's own previous output OR from a legitimate in-place update (a lint fix rewriting a
   file, an archive displacing, upgrade's re-render) — the suite proved seam-floor stop breaks all of them. The
   layered split: **the seam floor is `replace`** (archive-and-overwrite, today's semantics, compensable — what
   every in-place updater depends on), and **the cautious stop default is writ deploy's**, enforced by its
   pre-flight: every planned target that is occupied is classified through the readback — a symlink resolving to
   its recorded source, or a file whose digest equals the run's recorded as-deployed identity (step 48), is
   writ's own unmodified output and is cleared; anything foreign or locally modified refuses (listing the files,
   naming the flag) unless `--conflict=replace` or `skip`. A cleared default run executes under `replace` (the
   pre-flight vouched for every occupant); redeploys flow without the flag.
5. **Writ wiring rode THIS step (per the 2026-07-16 amendment)**: the interim channel is the dry-run precedent —
   the typed policy travels `Application.Flags["conflict"]`; the provider's `conflictPolicy()` reads the flag,
   falling to the announced section floor. The cli-config feed proper still arrives with the loader (config-plan
   items 3/5/7), retiring the flag channel alongside dry-run.

## Scope

1. The enum collapse ({stop, skip, replace}) + the flag/serialized name mapping + retire the dead field.
2. The provider-side branch at the write seam (Link, Copy, WriteText/WriteBytes, and the sops decrypt's write).
3. Tests: per policy × occupied/vacant/already-correct; the skip tally (skipped nodes surface in
   `Trace.Summarize`); the stop unwind.

## Landed (2026-07-16)

1. **Enum collapse**: `ConflictPolicy` = {`ConflictStop`, `ConflictSkip`, `ConflictReplace`} with
   `String`/`ParseConflictPolicy`; Backup/Overwrite and the never-read `RuntimeEnvironment.ConflictPolicy` field
   deleted; writ's `parseConflictPolicy` delegates; the deploy help text states the real semantics.
2. **Seam enforcement** (`file.Provider`): `prepareWrite`'s exists-branch and `Link`'s occupied branch switch on
   `conflictPolicy()` — stop errors (naming the mechanism), skip returns the Remove-style no-op success (a
   sentinel the three `prepareWrite` callers translate), replace keeps archive-and-overwrite. Covers Copy, Move,
   WriteBytes/WriteText/WriteFile, and Link. **Known gap**: `encryption.DecryptSopsFile` writes through its own
   path, not the file seam — its targets are still gated by writ's pre-flight; unifying the write path is
   follow-up.
3. **The floor amendment** (evidence-driven): seam-floor stop broke the star lint-fix writes, archive
   displacement, and the file provider's own overwrite pins — in-place updates are not conflicts, and the seam
   cannot distinguish them. Floor = `replace`; the stop default is writ deploy's layered pre-flight.
4. **Deploy pre-flight** (`preflightConflicts`): under default stop, occupied planned targets classify through
   the readback (`occupantIsOurs`: link-resolves-to-source, or digest equals the recorded identity); violations
   refuse listing the files and the flag; cleared runs execute under replace. Explicit skip/replace pass
   straight through to the seam. A missing run index reads as zero knowledge.
5. **Upgrade** runs its classification-cleared regeneration set under `replace` explicitly.

Tests: the deploy conflict matrix (foreign-occupant refusal with the file named; `--conflict=replace` archives
and lands; `--conflict=skip` leaves the occupant and deploys the rest; **redeploy-flows-under-default** — the
key regression), plus the flipped floor pin. `make test` zero failures repository-wide.

## Test plan (as executed)

1. Occupied target × {stop, skip, replace} at the writ level; vacant and already-correct-link unaffected.
2. The redeploy regression: change a source, redeploy under the default, the render refreshes.
3. Config layering (builtin floor vs cli `replace`) arrives with the loader; the interim flag channel is
   covered by the deploy matrix.
