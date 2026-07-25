---
title: "Resolve writ guide order collision"
issue: TBD
status: complete
created: 2026-07-25
updated: 2026-07-25
---

# Plan: Resolve writ guide order collision

Chartered follow-up 1 (devlore-cli half) of
[fix-windows-build-and-guide-category](fix-windows-build-and-guide-category.md).

## Summary

`writ/packages-manifest.md` and `writ/receipts.md` both carried `order: 4`, leaving their
relative position on the site's guides page unspecified (the page sorts each tool group by
`order`).

## Changes

1. `docs/guides/writ/receipts.md` — `order: 4` → `order: 5`.
2. `docs/guides/writ/repositories.md` — `order: 5` → `order: 6`.

Resulting writ sequence: Writ Overview 1, Manage Environments 2, Platform Awareness 3,
Packages Manifest 4, Deployment Receipts 5, Repositories 6 — the two reference guides sit
adjacent.

## Verification

Frontmatter-only change; the enumeration of all writ guide `order` values confirms the
sequence is collision-free. The docs-publish workflow syncs the new orders to the website
on merge.
