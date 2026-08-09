---
title: "Implicit Common Project"
status: complete
created: 2026-08-09
updated: 2026-08-09
---

# Plan: Implicit Common Project

## Summary

The reserved always-deployed project, ruled 2026-08-09. The spec existed
(platform-awareness guide: reserved `all`, always matched, implicit) but the code never
implemented it, and a second guide contradicted it with the every-project keyword reading
— the cutover surfaced both when `all`'s Tuckr links survived a deploy. Ruled:
platform-awareness wins; the name becomes **`common`** (Ansible's implicit-`all` pattern,
renamed so it cannot be misread as "every project"); the code must always match it.

## Delivered

1. `withCommonProject` injects `common` into deploy and upgrade selection (never
   decommission — destruction stays explicit; an empty selection stays empty where it
   already means "every project"). Unit-tested.
2. The scenario fixture renamed (`all*` → `common*`) and the deploy leg now asserts the
   implicit behavior: `common.conf` deploys without being named, `common.Unix` variants
   ride on unix platforms, microsoft stays explicit-only; platform floors raised (unix 6,
   windows 4).
3. All three guides reconciled: platform-awareness spec's `common` (with the Ansible
   provenance and the decommission boundary), manage-environments' every-project reading
   is dead, repositories shows `common/` in the structure and correct multi-layer
   phrasing.
4. The live repo: `devlore-cli/writ-layer` renamed its six `all*` directories to
   `common*`, and the post-deploy broken-link guard caught the rename's victims — three
   committed relative symlinks (`common.Unix` powershell profiles → the Windows tree)
   still naming `all-Windows`; rewritten onto `common.Windows`. The takeover deploy:
   282/282 healthy, zero new broken links. Tuckr's remaining domain: the `microsoft`
   group.
