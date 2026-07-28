---
title: "Sync merges: retry, registry --admin, corrected mechanism"
issue: TBD
status: complete
created: 2026-07-28
updated: 2026-07-28
---

# Plan: Sync merges — retry, registry --admin, corrected mechanism

Chartered 2026-07-28 from the #300 live proof, which cut both ways: the `--admin` merge
worked (site PR #211, merged by the app with checks pending and zero reviews), then the
very next run's merge died on a GitHub 502 and stranded site PR #212 open with green
checks. Separately, the registry sync still ran the pre-ruling plain merge.

## Changes

1. `docs-publish.yaml`, `release.yaml`, `knowledge-extract.yaml` — each sync merge runs
   in a bounded retry loop (5 attempts, 10/20/30/40s backoff). A failed attempt consults
   the PR state before retrying, because a 5xx can arrive after the merge landed
   server-side; a PR found merged ends the loop as success. Exhaustion fails the step.
2. `knowledge-extract.yaml` — the registry merge gains `--admin`. devlore-registry
   develop sits under the same org ruleset as the site repo, and the plain merge already
   failed live: registry PR #69 was app-created at 2026-07-25T06:59, its Knowledge
   Extract run (06:58:09) failed on the merge step, and the PR was merged by hand two
   minutes later.
3. Mechanism comments corrected in all three files: `--admin` disarms gh's client-side
   merge-state veto; the server-side entitlement is the automation app's "always" bypass
   on the org ruleset — not repository admin rights. A correction note is added to
   [ci-admin-merge-sync-prs](ci-admin-merge-sync-prs.md), which stated the admin-rights
   mechanism.

## Verification

Workflow-only change (no Go). The live proof is the next sync chain run. The retry loop
covers the observed stranding shape directly: merge reports failure, PR is actually
merged — the state consultation converts that to success instead of a retry storm or a
false failure.
