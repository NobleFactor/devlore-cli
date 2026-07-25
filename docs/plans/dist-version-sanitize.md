---
title: "Keep non-release tags out of the dist version"
issue: TBD
status: complete
created: 2026-07-25
updated: 2026-07-25
---

# Plan: Keep non-release tags out of the dist version

Chartered follow-up 3 of
[fix-windows-build-and-guide-category](fix-windows-build-and-guide-category.md).

## Summary

`DEVLORE_VERSION` defaulted to `git describe --tags`, which can resolve to non-release
tags such as `develop/lkg-11`. The slash-bearing version breaks the dist archive path
(`tar: Failed to open 'dist/devlore-cli_develop/lkg-11-…_darwin_amd64.tar.gz'`), so a
local `make dist-all` required a manual `DEVLORE_VERSION` override. CI is unaffected
either way: the Release workflow determines and passes its version explicitly.

## Changes

1. `Makefile` — the `DEVLORE_VERSION` default gains `--match "v*"`, so describe only
   resolves release-style tags (`v0.1.0-dev.…` and future `vX.Y.Z`); `--always` still
   falls back to a bare (slash-free) commit hash when no such tag is reachable.

## Verification

- `make -n dist-all` derives `v0.1.0-dev.20260314201016-45-gcd7d73a0-dirty` — slash-free.
- `make dist-all` with no override completes all six platforms and writes correctly
  named archives.
