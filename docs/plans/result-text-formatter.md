---
title: "Result Text Formatter"
status: proposed
created: 2026-08-10
updated: 2026-08-10
---

# Plan: Result Text Formatter

## Summary

`text` joins `pkg/result` as the fourth designed format — chartered 2026-08-10,
**implementation deferred until it can be done fully** (ruled). The model, in the
owner's words: a production on an array of json documents, no normalization, top-level
elements only — which is exactly the model [CSVFormatter](../../pkg/result/csv.go)
already implements: header inference from struct fields (declaration order, `csv:` tag
overrides) or the sorted union of map keys, the `HasHeaders` opt-in for odd shapes,
cells via `fmt.Sprint`. `TextFormatter` is csv's sibling: the same inference, emitted
as tabwriter-aligned columns under a header row.

## Design

1. **Extract the shared inference** — the header/row derivation moves out of `csv.go`
   into a common helper both formatters call; csv's output stays byte-identical
   (regression-tested).
2. **`TextFormatter`** — `text/tabwriter` alignment, one header row, empty input
   renders nothing (csv parity), compile-time `Formatter` guard.
3. **Registration** — `FormatterByName` gains `"text"`; the `--format` help string in
   `cli.AddOutputFlags` updates to name five values.
4. **Per-command defaults are the consumer's choice** — `writ secret list` flips its
   default from json to text when this lands (recorded in its plan); nothing else
   changes defaults implicitly.

## Verification

1. Golden tables for struct slices, map slices, and a `HasHeaders` type; the empty and
   single-row edges; csv regression byte-identical.
2. `make test`; dual-GOOS lint recount at zero.

## Open questions

1. Nested-value cells: `fmt.Sprint` parity with csv, or compact JSON — resolve at
   implementation with real documents on screen.
