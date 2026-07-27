---
title: "Sync PRs merge with --admin"
issue: TBD
status: complete
created: 2026-07-27
updated: 2026-07-27
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

## Verification

Workflow-only change (no Go); proof is the next docs-sync chain run.
