---
step: 60
issue: https://github.com/NobleFactor/devlore-cli/issues/438
title: "The store writes `latest.yaml` as a symlink an ordinary Windows user cannot create, and nothing reads it"
status: charter — chartered 2026-08-17 from the 3.3a review; three findings, one subject
proof_run: TBD — must include a Windows case with the symlink privilege DENIED, not merely absent
parent: ../../phase-8.md
---

# Step 60 — The execution store's cross-platform contract

**Status:** `charter` — chartered 2026-08-17, asking whether phase 3.3a's `op.SaveGraph` /
`op.SaveTrace` split needed per-platform save/load scenarios. It does not. But the question opened the
store's persistence path, and three things were found there that no tier proves.

## Finding 1 — `latest.yaml` is a symlink, and it is the one fatal step

[`cmd/internal/cli/store.go:141`](../../../../../cmd/internal/cli/store.go):

```go
latest := stateRoot.NewPath(directory, "latest.yaml")
//nolint:errcheck // diagnose-ignored-error: stale link; see docs/architecture/2.8-eventing-infrastructure.md
_ = stateRoot.Remove(latest) // best-effort: replace any prior link
if err := stateRoot.Symlink(filename, latest); err != nil {
    return "", fmt.Errorf("link latest trace %s: %w", latest.Abs(), err)
}
```

Creating a symlink on Windows requires `SeCreateSymbolicLinkPrivilege`, held by Administrators, or
Developer Mode enabled — Go passes `SYMBOLIC_LINK_FLAG_ALLOW_UNPRIVILEGED_CREATE`, and that flag is
honored only under Developer Mode. **An ordinary Windows user has neither.** For that user every
`WriteTrace` returns an error at this line.

**What actually happens is quieter than a failed run, and worse in one respect.** Enumerated across
all nine call sites (`deploy`, `upgrade`, `decommission`, `adopt/batch`, `migrate_cmd`,
`migrate/register`, `migrate/session`, `secret/encrypt`): none treat the error as fatal. Every one
warns, in the shape of [`deploy.go:234`](../../../../../cmd/writ/writ/deploy/deploy.go):

```go
if receiptPath, writeErr := cli.WriteTrace(trace); writeErr != nil {
    cli.Warn("failed to write receipt: %v", writeErr)
```

So on an unprivileged Windows machine:

1. The trace document **is** written — `op.SaveTrace` runs before the link.
2. `latest.yaml` does not exist.
3. `appendIndexEntry` sits **after** the symlink, so the run index never receives the event. The
   index silently loses every trace on that machine.
4. The command reports success with a warning, and `writ reconcile` / reconciliation later read an index
   that disagrees with the directory.

Step 3 is a defect on **every** platform, not just Windows: any failure of the link step discards the
index entry for a trace that is already durably on disk. The ordering is wrong independently of how
the link question is answered.

## Finding 2 — nothing reads the link

Enumerated 2026-08-17: `LoadLatestTrace` is the only reader of `latest.yaml`, through
`LatestTracePath`, and it has **no callers outside `cmd/internal/cli/store.go` itself** — no command,
no reconciliation path, no test outside the package. The store fails a write step for a convenience
entry point that no code currently uses.

That reframes the question. Before deciding how to spell "latest" on a platform without symlinks,
decide whether the entry point earns its place at all. The architecture describes it as "the
convenience entry point for drift detection, reconciliation, and pause/restart"
([5-graph-trace-integrity.md](../../../../architecture/5-graph-trace-integrity.md)) — all three are
planned, none landed.

### The options, and what each leaves broken

1. **Keep the symlink; make it best-effort and move `appendIndexEntry` ahead of it.** Smallest change,
   and it fixes finding 1's step 3 outright. Residual: on Windows the entry point silently does not
   exist, so every future reader needs an absence branch that fires on one platform only — the kind of
   conditional that rots because it is never exercised where anyone looks.
2. **Make `latest` a regular file containing the target filename.** Works identically everywhere.
   Residual: two reads instead of one, and it is no longer resolvable by anything that follows links —
   `ls -l`, Explorer, or a person. We would be inventing a convention where the OS already has one.
3. **Copy the trace to `latest.yaml`.** No indirection at all. Residual: the store doubles per run, and
   one logical document acquires two paths — the "one document, one identity" property becomes "one
   identity, two locations", with a stale second copy possible whenever the copy fails after the write.
4. **Drop `latest` and resolve it by listing the directory.** The filenames are
   `20060102T150405.000000000Z`, so lexicographic order **is** chronological and the newest is a `max`
   over the listing. Deletes a feature nothing uses. Residual: the entry point becomes a scan whose
   cost grows with run count, the timestamp filename format becomes load-bearing rather than
   incidental, and there is no atomic pointer for a concurrent reader to observe.

**Do not pick from this list on tidiness.** Answer it the way step 54's layout question was answered —
by establishing what the three planned consumers actually need from "latest", since all three are
still to be written and none of them has yet been constrained.

## Finding 3 — the cross-platform contract is untested, and a scenario is the wrong instrument

The question that started this charter was whether save/load needs a scenario per platform. It does
not: the store's names are checksum-derived and contain no separators, the encoders are
`encoding/json` and `yaml.v3`, and the checksum covers canonical bytes rather than file bytes — there
is no filename-shaped assumption for a scenario to disagree with, which is the specific thing the
scenario tier exists to catch.

What is genuinely unproven is **document identity across platforms**: nothing shows that a graph
written on Windows loads on Linux with a matching checksum. A scenario cannot show it either, because
each runs on one platform and compares only against itself. The instrument is a **committed fixture** —
one graph and one trace in `testdata`, written once, loaded and checksum-verified by every platform's
test leg. That extends the existing `graph_format_identity_test.go` / `trace_format_identity_test.go`
from cross-*format* identity to cross-*platform* identity, using the same argument they already make.

Separately, the store the shipped binary produces is asserted nowhere. `cmd/writ`'s deploy scenario
already causes a real store to be written and does not look at it. That is a scenario-shaped gap, but
it is an assertion block on an existing scenario rather than a new one.

## Why CI cannot see finding 1

The GitHub Actions Windows runner account can create symlinks — which is exactly the problem. Both
tiers pass there, and would pass no matter how much coverage is added, because the privilege is
granted. **A test that runs where the privilege is held proves nothing about the machine where it is
not.** The proof run has to deny the privilege, not merely decline to grant it; this is the same
lesson the campaign already recorded when phase 2 enforcement was tested against the safest root
instead of the one production uses.

## Exit criteria

- [ ] `appendIndexEntry` precedes the link step, so a trace already on disk always reaches the index.
      This lands regardless of how the link question resolves.
- [ ] The `latest` question is answered against what drift detection, reconciliation and pause/restart
      need — with the chosen option's residual recorded, as above.
- [ ] `WriteTrace` succeeds for an unprivileged Windows user, proved by a test that **denies** the
      symlink privilege rather than one that runs where it happens to be granted.
- [ ] A committed graph + trace fixture pair loads with a matching checksum on all three platforms.
- [ ] `cmd/writ`'s deploy scenario asserts the store it produced: layout, the latest entry point
      resolving, and a second run appending a second trace.
- [ ] The nine `WriteTrace` call sites are re-examined once the above lands — a warn-and-continue is
      right for a link failure and wrong for an index failure, and today they cannot tell the two apart.

## Related

- [windows-native-permissions.md](../../../windows-native-permissions.md) — phase 3.3a is what opened
  this path; the campaign's exit gate should not close while an ordinary Windows user cannot write a
  complete trace record.
- [step 58](58-windows-system-target-root.md), [step 59](59-xdg-search-paths-on-windows.md) — the same
  pattern: a Windows defect surviving because exposure is currently zero and CI runs privileged.
- [5-graph-trace-integrity.md](../../../../architecture/5-graph-trace-integrity.md) — the store layout
  and the `latest.yaml` claim this step tests against reality.
