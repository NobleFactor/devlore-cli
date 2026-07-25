---
title: "Fix Windows build and guide category"
issue: TBD
status: complete
created: 2026-07-25
updated: 2026-07-25
---

# Plan: Fix Windows build and guide category

## Summary

PR #290's merge surfaced two pipeline failures:

1. **Release failed** on the windows/amd64 cross-build: `pkg/op/default_funcs.go` called
   `syscall.Umask`, which does not exist in the Windows `syscall` package. A second, masked
   break sat behind it: `pkg/op/provider/file` type-asserts `*syscall.Stat_t` (also absent
   on Windows) at two sites; CI never reached it because packages depending on `pkg/op`
   were not compiled after `pkg/op` failed. The failed Release also skipped the
   `sync-install-script` job, so no install-script sync PR was created.
2. **The website deploy failed** after the docs sync merged: `provider-development.md`
   carried `category: "development"`, which the site's Astro guides schema rejects
   (allowed: `overview | tutorial | concept | reference`).

## Changes

1. `pkg/op/default_funcs_unix.go` (new, `//go:build unix`) — `processUmask()` reads the
   process umask via the `syscall.Umask` round-trip (set zero, restore).
2. `pkg/op/default_funcs_windows.go` (new, `//go:build windows`) — `processUmask()`
   returns zero: Windows has no umask, so `{{ umask base }}` resolves to the base mode.
3. `pkg/op/default_funcs.go` — `defaultUmask` calls `processUmask()`; `syscall` import
   removed; banned `env:` parameter-bullet keys renamed to `runtimeEnvironment:`.
4. `pkg/op/provider/file/helpers_unix.go` (new, `//go:build unix`) — `statIdentity()`
   returns the inode and device numbers from `*syscall.Stat_t`.
5. `pkg/op/provider/file/helpers_windows.go` (new, `//go:build windows`) —
   `statIdentity()` returns zeros: change detection falls back to size and mtime.
6. `pkg/op/provider/file/helpers.go` (`statTupleEtag`) and
   `pkg/op/provider/file/provider.go` (`Observe`) — call `statIdentity()`; the
   `syscall` import leaves `provider.go` (`helpers.go` keeps it for `ENOTEMPTY`,
   which exists on Windows).
7. `docs/guides/provider-development.md` — `title` and body H1 become
   "How to create and modify providers" (per review); `category` becomes `"tutorial"`.
   All 12 other guides were enumerated and already carry valid categories.

## Verification

- `gofmt -l` clean over the change set.
- `make vet` — pass.
- `make test` — pass (after `make star`; see follow-up 2).
- `make dist-all DEVLORE_VERSION=v0.0.0-local` — all six platforms compile, including
  windows/amd64 and windows/arm64 (see follow-up 3 for why the version override).

## Chartered follow-ups (not in this change)

1. **Guides metadata**: writ guides have an `order: 4` collision (Packages Manifest,
   Deployment Receipts); the site's `categoryLabel` has no case for `reference`, so that
   badge renders as raw lowercase text.
2. **Stale generator bootstrap**: the `build/star:` rule has no prerequisites, so an
   existing stale binary is silently used; a June-27 `build/star` rejected the `chmod`
   keyword that `generate.star` now passes. `make star` fixed it locally; the rule
   deserves real prerequisites or a staleness check.
3. **Local `dist-all` version**: `git describe` can yield slash-bearing versions
   (`develop/lkg-11-…`), which break the tar path. CI passes an explicit version;
   locally an override is required.
4. **Lint debt**: `make lint` reports 283 pre-existing findings repo-wide (this change
   adds none); CI's quality-gate does not gate them.
5. **Doc-comment sweep**: pre-existing multi-line doc summaries remain in
   `default_funcs.go`, `helpers.go`, and `provider.go` (≈9 sites enumerated by the
   mechanical sweep), out of scope for this fix.
