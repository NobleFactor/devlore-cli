---
title: "Class b: the bug-smell review"
issue: TBD
status: complete
created: 2026-08-01
updated: 2026-08-01
---

# Plan: Class b — the bug-smell review

Second rung of the 4b-3 ladder. All thirty sites read before ruling (approved 2026-08-01:
19 mechanical, 2 restructures, 9 suppressions — one reclassified during implementation).

## The one real defect

`cmd/star/main.go` called `os.Exit(1)` below `defer iox.Close(&err, runtime)`: every
error exit skipped the runtime's Close. Fixed with the standard extraction — `main` calls
`run() (err error)` and owns the single `os.Exit`; the same shape applied to
`cli_test.go`'s `TestMain` (`testMain(m) int`).

## Mechanical fixes (19)

bodyclose ×3 (test responses now closed), staticcheck ×4 (S1008 boolean return, two
De Morgan applications, `WriteString(Sprintf)` → `Fprintf`), filepathJoin ×3 (test paths
as components), nilValReturn ×1, a dead no-op `strings.Replace(x, "run_status",
"run_status", 1)` deleted, `else{if}` → `else if`, if-else chain → switch, type-assert
chain → type switch, append combine, nesting reduce (invert + continue), and the
`file` parameter shadowing the file-provider import renamed to `backing`. The recurring
`archive/provider_test.go` goimports finding is autofixed and rides along.

## Suppressions with stated reasons (10)

- nilerr ×6 — every one deliberate error-to-verdict conversion: LookPath/install
  failures become the result message (setup ×2), an integrity-refused document IS the
  invalid verdict (verify), unreconstructable produced types are tolerated on resume
  (recovery stack), the inventory walker skips by design (×2). No bugs in the class.
- dupArg ×1 — `r.Equal(r)` tests reflexivity.
- ptrToRefParam ×2 — `iox.Close`'s pointer-to-error IS the defer idiom;
  `dataSectionIterator.Next` implements starlark.Iterator's signature.
- typeDefFirst ×1 — **reclassified from mechanical during implementation**:
  `filteredReceiver` lives in the house SUPPORTING TYPES region; moving it above its
  methods would break the mandated file layout, so house layout wins over the linter.

## Verification

- nilerr, staticcheck, bodyclose, goimports: all 0 uncapped; gocritic 48 → 31
  (only unnamedResult — class c — remains).
- `make vet` pass; full `make test` pass; `gofmt -l` clean. Total findings 247 → 216.
