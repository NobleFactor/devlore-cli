---
title: "goast shared walker: parsedSources over an extended parsedFile"
issue: "#312 follow-on"
status: complete
created: 2026-08-04
updated: 2026-08-04
---

# Plan: goast shared walker

The structure ruling pre-declared in the complexity plan's phase-3 notes, ruled 2026-08-04:
option 1 (iterator) with the extended `parsedFile`.

## Changes

1. **`parsedFile` gains `path`** — one concept, one type: the parse-cache entry now carries
   the path it was parsed from. `parsedEntry` becomes the single cache-or-parse primitive;
   `parseFile` layers over it unchanged for its existing callers.
2. **`parsedSources(path)`** — an `iter.Seq[*parsedFile]` over the collected Go files:
   collection failures surface at the call (each client keeps its own `goast.<op>:` error
   wrap, text unchanged), parse failures skip mid-iteration (the existing semantics stated
   once), and breaking the range stops the walk — the early-exit shape `Callable` and
   `TypeDoc` use naturally.
3. **Four clients converted** — `Callable`, `ConstGroups`, `Structs`, `TypeDoc`. `Deps` and
   `Metrics` stay as they are: they share only the file collection, parsing per-file with
   different modes through their analyze helpers.

## Ownership rider: eleven findings from phase 7's final squeeze

The phase-7 squeeze batch skipped the full-config lint recount (complexity-only was
checked), shipping eleven style debts in my own helpers: seven unnamedResult, one
paramTypeCombine, one G204 (extraction moved the argv behind a parameter, changing gosec's
taint view — suppressed with provenance), one misspell, and one unparam (failAndUnwind's
always-nil result dropped; the caller supplies the nil). All fixed here; the lesson is the
standing one — every batch ends with the full-config recount, no exceptions.

## Verification

- Full-config lint: **0** findings, uncapped. `make vet` + full `make test` green; gofmt
  clean.
