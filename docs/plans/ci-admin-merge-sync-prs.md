---
title: "Sync PRs merge with --admin"
issue: TBD
status: complete
created: 2026-07-27
updated: 2026-07-28
---

# Plan: Sync PRs merge with --admin

Item 1 (second half) of the 2026-07-27 queue; merge-strategy ruling: `--admin`, because the
site's branch policy prohibits a plain merge until required checks finish and policy
prevents `--auto`.

## Changes

1. `docs-publish.yaml` — the docs sync PR merge gains `--admin` (the plain merge failed on
   the site's branch policy on the #299 chain, leaving site PR #209 stranded).
2. `release.yaml` — the install-script sync only *created* its PR (the #289 direct-merge
   change covered docs-publish alone), so a future install.sh change would strand a PR.
   The sync now captures the PR URL and merges it `--admin --squash --delete-branch`,
   matching the #289 philosophy. **Behavior addition, flagged for veto.**

## Risk, stated

Both merges run under the NobleFactor automation app's token. `--admin` requires the app
to hold admin on devlore.noblefactor.com; if it does not, the next docs sync fails the
same way and the remediation is granting the app admin on the site repo (owner lever).
The next merge to develop is the live proof.

**Correction (2026-07-28, verified live):** repository admin rights are not the mechanism.
`--admin` only disarms gh's client-side merge-state veto (which reads the PR as BLOCKED
while required checks are pending — the race that stranded site PR #209); the merge API
then admits the app through its "always" bypass on the org ruleset, proven by site PR
#211 merging with checks pending and zero reviews. Workflow comments corrected in
[ci-sync-merge-retry](ci-sync-merge-retry.md).

## Verification

Workflow-only change (no Go); proof is the next docs-sync chain run.
