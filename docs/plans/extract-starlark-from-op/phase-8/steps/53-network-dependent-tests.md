---
step: 53
issue: https://github.com/NobleFactor/devlore-cli/issues/439
title: "Network-dependent tests make CI's red signal ambiguous"
status: charter — chartered 2026-08-14; investigation not started, solution deliberately not chosen
proof_run: TBD — defined by the step's own decision (see Exit criteria)
parent: ../../phase-8.md
---

# Step 53 — Network-dependent tests make CI's red signal ambiguous

**Status:** `charter` — chartered 2026-08-14 from the [#405](https://github.com/NobleFactor/devlore-cli/issues/405)
campaign, where `test (macos-latest)` failed on PR #414 for a reason that had nothing to do with the change under
review.

**No solution is assumed.** This charter states the problem, the evidence, and what has to be
*discovered* before anyone picks a fix. The options listed at the end are candidates to evaluate,
not a shortlist to choose from, and the enumeration task exists precisely because the shape of the
answer depends on facts nobody has gathered yet.

## The observed failure

PR #414, job `test (macos-latest)`:

```
--- FAIL: TestRegistry_SyncIntegration (30.11s)
    registry_test.go:198: first Sync() error: git clone: exit status 128:
    Cloning into '/var/folders/.../registry-test-3456362816/central'...
    fatal: unable to access 'https://github.com/NobleFactor/devlore-registry.git/':
    Could not resolve host: github.com
```

`internal/lorepackage`'s `TestRegistry_SyncIntegration` clones
`https://github.com/NobleFactor/devlore-registry.git` from a CI runner. The runner's DNS failed. The
same commit passed `test (ubuntu-latest)` and `pkg/op` on Windows; nothing in the PR touched
`internal/lorepackage`, networking, or git.

## Why this is worth a step

**A red check that sometimes means "the network was unwell" trains everyone to look past red.** This
repository is in the middle of a campaign whose central discipline is a *known-failure baseline* —
the Windows leg is expected to fail at exactly 28, verified name by name, and any 29th name is a
real regression. That discipline works only when a failure is attributable. One test whose outcome
depends on GitHub's reachability makes "is this red mine?" a judgment call on every PR, and judgment
calls at that frequency decay into reflex.

Note the failure was also **slow** — 30 seconds of clone timeout before the verdict, on a leg that
otherwise finishes in ~5 minutes.

## What must be discovered first — enumerate, do not sample

The one failure we saw is not the population. Before any fix is designed:

1. **Enumerate every test that reaches the network**, by what it *does* rather than by what it is
   named. A grep for `https://` in `_test.go` finds string-literal fixtures (`repo_cmd_test.go`
   parses URLs without fetching them) and misses anything that reaches the network through a
   helper, a fixture repository, a provider, or a `go:generate` step. The starting evidence: the
   grep surfaces `internal/lorepackage/registry_test.go` as the only *fetching* test, and
   `cmd/devlore-index/main_test.go` deliberately builds a **local** `devlore-registry` sibling — so
   a local-fixture precedent already exists in this repository and should be understood before
   anything new is invented.
2. **Establish how often this actually fires.** One occurrence is an anecdote. Query the recent CI
   history for this test's failures per leg — it decides whether this is a nuisance or a real tax,
   and a fix sized for the wrong frequency is waste in one direction or negligence in the other.
3. **Determine what the test is actually for.** `TestRegistry_SyncIntegration` asserts
   `FromClone`/`Updated` on first sync and the update path on the second. Whether that *needs* a
   real remote — versus any git remote, versus a fake transport — is the crux, and it is a question
   about the registry's contract, not about test plumbing.
4. **Check the other legs' exposure.** Windows and Ubuntu run the same test. If it has failed there
   too, the frequency question above changes shape; if it has not, that itself is evidence about
   runner networking rather than about the test.

## Candidate directions, each with the residual to weigh

Listed to be *evaluated*, not chosen. Every one of them has a cost that the discovery work above
should price:

- **A local fixture repository** (`git init` a tree in `t.TempDir()`, clone from a `file://` URL).
  *Residual:* stops exercising the real transport — TLS, redirects, auth, and the `https://` code
  path all go untested, and a transport bug then reaches users first.
- **A recorded/replayed remote.** *Residual:* the recording drifts from the real service silently,
  which is the failure mode that makes recorded tests quietly worthless.
- **Tag as an integration test, gated behind an environment variable or build tag.** *Residual:*
  gated tests stop running, and a test nobody runs is a test nobody maintains — it will rot and its
  failure, when it finally surfaces, will be someone else's afternoon.
- **Keep the network but make failures attributable** — detect the network-unreachable case
  specifically and skip with a message that names the cause, so a red check keeps meaning "the code
  is broken." *Residual:* a skip is invisible in aggregate reporting; a *permanent* outage looks
  like a pass forever, which is worse than a loud failure.
- **Retry with backoff.** *Residual:* hides genuine intermittent breakage in the thing being tested,
  and lengthens the slowest leg.
- **Move it out of `make test` into a scheduled job.** *Residual:* decouples it from the PR that
  breaks it, so the signal arrives after the merge rather than before.

**Do not assume the answer is one of these.** The right shape may be a combination (a fake transport
for the contract, one real-network test in a scheduled job), or something the enumeration surfaces
that this list does not anticipate.

## Exit criteria

- [ ] The enumeration exists: every network-reaching test named, with what it fetches and why.
- [ ] The frequency question is answered with data from CI history, not impression.
- [ ] A decision is recorded **with its residual stated**, in the shape this repository uses for
      rulings — including what the chosen approach stops covering.
- [ ] `make test` is deterministic with respect to network availability, or the exceptions are
      explicitly enumerated and justified.
- [ ] A CI failure in `make test` attributes to the change under review, not to the network.

## Related

- PR [#414](https://github.com/NobleFactor/devlore-cli/pull/414) — where this surfaced.
- [windows-native-permissions.md](../../../windows-native-permissions.md) — the campaign whose
  known-failure baseline discipline this ambiguity undermines.
- `cmd/devlore-index/main_test.go` — the existing local-fixture precedent worth studying first.
