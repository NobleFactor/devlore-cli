---
title: "Golangci Bump"
status: complete
created: 2026-08-07
updated: 2026-08-07
---

# Plan: Golangci Bump

## Summary

Item 12: the local golangci-lint auto-updated to v2.12.2 while CI pinned v2.10.1, splitting
the board — v2.12's new `inline` govet analyzer surfaced 23 deprecated `reflect.Ptr` uses,
and its gosec pass flagged one new G115 (`hashString`'s rune→uint32). Approved 2026-08-07:
align the CI pin to v2.12.2 and fix the findings deliberately, restoring the single-board
contract (local and CI lint the same way, uncapped zero on both GOOS).

## Changes

1. **CI pin** — `.github/workflows/ci.yaml` installs v2.12.2 (both the install.sh URL and
   the version argument).
2. **`reflect.Ptr` → `reflect.Pointer`** — 23 sites across 9 files (star cli/config/goast,
   `pkg/op/helpers.go`, `pkg/op/receiver_type.go`); `reflect.Ptr` is the deprecated alias.
3. **G115 suppression with reason** — `hashString` (`pkg/op/starlarkbridge/go_receiver.go`)
   ranges a string, so the rune is a code point (0..0x10FFFF, never negative) and hash
   wraparound is the algorithm's intent.
4. **No config change** — the current `.golangci.yaml` runs unchanged under v2.12.2, so the
   three-copy lockstep (repo config, star seed template, ops canonical) is untouched and
   the drift guard stays green.

## Verification

Suite green; uncapped board zero on Darwin and `GOOS=linux` under v2.12.2.
