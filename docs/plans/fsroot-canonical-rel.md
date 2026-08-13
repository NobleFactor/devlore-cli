---
title: "Canonical slash-form Rel"
issue: https://github.com/NobleFactor/devlore-cli/issues/377
status: completed
created: 2026-08-12
updated: 2026-08-12
---

# Plan: Canonical slash-form Rel

## Summary

`fsroot.Path.rel` is built with OS-native separators, and `{fsroot, rel}` is exactly what
serializes — `abs` is derived on decode and never stored. The same logical path therefore
produces different document bytes and different checksums per platform, violating the
repository's platform-stable identity rule (the Merkle-root digest is specified forward-slash
for precisely this reason). The fix: `rel` becomes canonical slash form at every construction
and decode site; `abs` stays OS-native and carries all direct filesystem I/O.

CI evidence (PR #388, the corrected Windows baseline): roughly 50 of 85 Windows failures trace
here — ~17 `fsroot`/`triad` path tests, much of the file provider's write/compensate suite, and
the graph-resume / receipt round-trips, including `TestReceipt_RestoreEncoded_JSONandYAML`,
which is the serialized-document divergence demonstrating itself.

## Design

**Invariant: `rel` is always slash-form; `abs` is always OS-native.** Conversion happens at the
type's boundaries, never in consumers.

1. Construction — `filepath.ToSlash` on the computed rel at `root.go:527` (`Path{root, rel,
   abs}` composite), `root.go:687` (`filepath.Rel` result), and `root.go:692`
   (`filepath.Clean` result).
2. Decode — `UnmarshalJSON` / `UnmarshalYAML` normalize `decoded.Rel` with `ToSlash` too, so a
   document written by a pre-fix Windows build still decodes to canonical form. This is a
   decode-time normalization, not a compatibility layer: the canonical form is the only form the
   type emits, and decode enforces the invariant on arrival. `abs` derivation via
   `filepath.Join` already accepts slash input on Windows; unchanged.
3. `abs` computation from slash-form rel uses `filepath.FromSlash` where needed for clarity,
   though `filepath.Join` normalizes regardless.

### Every non-test consumer of `Rel()`, audited

| Site | Feeds | Under slash form |
| --- | --- | --- |
| `pkg/op/recovery_site.go:171`, `:182` | recovery keying | **improved** — keys become platform-stable; today they diverge like the documents do |
| `pkg/op/provider/file/provider.go:2038` (`walkDir`) | `fs.WalkDir` | **fixed** — the `io/fs` contract requires slash paths on all platforms; native form on Windows violates it today |
| `pkg/op/provider/file/directory.go:333` (`merkleRoot`) | `fs.ReadDir` | **fixed** — same `io/fs` contract |
| `pkg/op/provider/function/resource.go:356` | `filepath.Dir` | safe — Windows `filepath` accepts `/` on input; result re-enters `NewPath` and re-normalizes |
| `pkg/op/provider/mem/helpers.go:81`, `:124`, `:150` | `filepath.Dir` → `MkdirAll` | safe — same reasoning |

### Rulings required before implementation

1. **Existing serialized documents are greenfield.** Traces/receipts already on disk from
   Windows builds carry native-form rel; the decode-time `ToSlash` reads them correctly anyway,
   and no migration is written. (Unix-written documents are already canonical.) Confirm.
2. **`Abs()`/`String()` test expectations become platform-correct**, not slash-form: the three
   failing assertions feeding `/project/src/main.go` and expecting it back verbatim get
   `filepath.FromSlash`-built expectations. `Abs` is a local handle; asserting Unix literals on
   it was the bucket-2 half of the original finding. Confirm.

## Implementation — one branch: `fix/fsroot-canonical-rel`

Tests-first applies and is cheap here: the failing CI assertions *are* the characterization —
`Rel() = "sub/file.txt"` is already asserted by `root_test.go` and `triad_test.go` on every
platform; they simply fail on Windows today.

- [ ] Phase 1: the three `ToSlash` construction sites + the two decode sites in
      `pkg/fsroot/root.go`.
- [ ] Phase 2: fix the three `Abs()`/`String()` bucket-2 test expectations
      (`root_test.go`, `triad_test.go`) with `filepath.FromSlash`.
- [ ] Phase 3: `registry.FilePath` expectations (`internal/lorepackage/registry_test.go`) —
      re-examine after phase 1; the `\tmp\test-cache\...` failures may be this same defect one
      layer up, or a separate native-path expectation to fix as bucket 2.
- [ ] Phase 4: local verification — `make test`, `make vet-all`, `make lint-all` green on
      Darwin; then CI's windows leg measures the drop from 85.

## Verification

The windows `test` leg is the gate that matters: expect the ~17 `fsroot`/`triad` failures and
the receipt/graph-resume round-trips to clear; the file provider's suite drops by whatever
share was separator-caused (its permission-semantics failures remain, tracked as #373 phase
3e). `make cover` confirms the decode-normalization path is exercised by the existing
round-trip tests. No test is skipped; no build tag is added.

## Outcome (2026-08-12)

**Landed inside PR #389** (the Apache-2.0 relicense), not as its own PR: the implementation was
in-flight in the same working tree when the licensing sweep was staged, and the three changed
files were entangled by whole-directory staging. The ruling was to merge as-is with the PR body
disclosing both changes and carrying the measurement, rather than splitting commits or rewriting
a shared branch. Issue #377 closed by that merge. This plan document lands separately, after.

**Measured effect** (#389's own `test (windows-latest)` leg vs the #388 baseline): **85 → 57**.
Every `fsroot`/`triad` path assertion and `TestRegistry_FilePaths` cleared; zero build failures.

**Attribution corrected:** the earlier "~50 of 85" estimate was wrong — the separator-direct
share was 28. The receipt/graph-resume round-trips (`TestReceipt_RestoreEncoded` 2,
`TestGraphResume*`/`TestGitCloneResume*` 5) survived the fix, so they have a **second cause
beyond separator form** — the first thing to read in the remaining-57 triage. The rest of the
remainder: `TestLintCopyright` (5, likely the deliberate SSPL/MIT fixture stragglers meeting
`license: auto` on Windows), file-provider permission/path semantics (~13), and known singles
including #376's Starlark escape.

## Related Documents

- Issue #377 — this defect
- [docs/plans/platform-test-matrix.md](./platform-test-matrix.md) — #373; supplies the CI
  evidence and consumes the failure-count drop
