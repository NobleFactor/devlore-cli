---
title: "Archive Root Confinement"
status: complete
created: 2026-08-07
updated: 2026-08-07
---

# Plan: Archive Root Confinement

## Summary

Issue #225: the archive provider opened archives with `os.Open`, bypassing the
`fsroot.Root` confinement boundary. The sweep found the reported site plus a second,
subtler one in the same flow: the zip branch closed the file and re-opened it by path via
`zip.OpenReader`, which performs its own unconfined `os.Open` internally.

## Fix

`openArchive` opens through the environment's root (`root.Open(root.NewPath(source))` —
the file provider's idiom), and `newZipArchiveReader` now takes the already-open handle:
`file.Stat` for size, `zip.NewReader` over the handle, the handle owned and closed by the
reader. The zip branch hands the same confined handle over instead of re-opening; the
spool path passes its temp-file handle directly, dropping a close-and-reopen. The
`zipArchiveReader` struct holds `*zip.Reader` + `io.Closer` in place of `*zip.ReadCloser`.

## Sweep: remaining os-level file I/O in providers

The broadened search (all `os.*` filesystem calls under `pkg/op/provider/`) found four
judgment groups, none a confinement hole of the #225 kind. Ruled 2026-08-07: all four are
deliberate keeps, each site now carrying a one-line confinement-reason comment:

1. **archive spool** — `os.CreateTemp`/`os.Remove` in the system temp dir. Process
   scratch mandated by §10 ruling 5 (stream-shaped zips spool to disk); not target-tree
   I/O.
2. **plan Save/LoadDefinition** — `os.Create`/`os.ReadFile` on caller-named plan-document
   paths. Plan documents are store/CLI documents, not confined-tree resources;
   Root-routing would confine them to the run root (a behavior change).
3. **function synthesize** — `os.ReadFile` of the Starlark source at the interpreter
   position. Script sources live outside the target root by definition.
4. **git provider** — `os.RemoveAll` in CompensateClone and three `os.Stat` repo probes.
   Git trees are managed by the external git subprocess, which confinement never binds;
   `fsroot.Root` has no `RemoveAll`.

## Verification

Suite green, board zero on both GOOS, zero `os.Open` remaining in the archive provider.
