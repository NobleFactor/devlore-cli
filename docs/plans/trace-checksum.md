---
title: "Trace checksums: symmetric integrity with graphs"
issue: TBD
status: complete
created: 2026-07-27
updated: 2026-07-27
---

# Plan: Trace checksums — symmetric integrity with graphs

Escalated from chartered follow-up 1 of
[lint-errcheck-remainder](lint-errcheck-remainder.md) (2026-07-26): "Traces and graphs
must each carry checksums that are computed the exact same way." Approved 2026-07-27 with
the **strict panic extent**.

## The gap

Graphs carry `Checksum = GitStyleChecksum("graph", canonical)` and `op.LoadGraph`
recomputes and compares on every load. Traces carry only `GraphChecksum` (the graph's
identity, not their own) and load through `cli.LoadTrace` = `document.ReadFile[op.Trace]`
— **no integrity verification at all**. Under the checksum trust boundary (settled
2026-07-26), every trace consumer therefore operates in a permanently pre-verification
zone.

## Changes

1. **`Trace.Checksum` field** — `"sha256:<hex>"`, computed as
   `GitStyleChecksum("trace", canonical)`: the identical helper and header construction
   the graph uses, domain string `"trace"`. `Trace.CanonicalContent` strips `checksum`
   alongside `signature`, mirroring the graph's canonical form (which excludes both
   integrity fields). Signature semantics unchanged: it covers
   content-sans-integrity-fields.
2. **Stamp at persist** — `cli.WriteTrace` computes and sets the checksum before signing
   and writing. (A trace's document form is fixed at persist; the graph seals at build.
   Mechanism and verification contract are identical.)
3. **Verifying loader** — `op.LoadTrace`, mirroring `op.LoadGraph`: decode, recompute
   over canonical, compare. Mismatch = error; **missing checksum = error**. Greenfield:
   traces already in the store predate the field and are refused; new runs regenerate
   them. No legacy tolerance.
4. **Funnel** — `cli.LoadTrace` routes through `op.LoadTrace`; readback and
   `LoadLatestTrace` inherit verification.
5. **Strict panic-side flip (approved)** — with traces verified at load, the decode path
   (recovery-stack decode, receipt `RestoreEncoded`, field decoders) sits post-checksum,
   so per the boundary ruling all its read failures become panics: the 4b-2
   `stringField`/`boolField` mistype errors flip to `assert.Type`, embedded
   `uuid.Parse`/`ParseDigest` become `assert.Must`, and dangling intra-document id
   lookups become asserts.
6. **Tests** — checksum-mismatch and missing-checksum error tests at the load boundary;
   decoder tests move from error to panic expectations; full `make test`.
7. **Design documentation** — the checksum trust boundary and the graph/trace integrity
   mechanism get a home in `docs/architecture` (location chosen during this work; this
   also discharges chartered follow-up 2 of lint-errcheck-remainder).

## Wrinkles found and fixed during implementation

1. **Typed round-trip instability.** Canonical bytes derived from a *decoded* trace do not
   re-marshal byte-identically (typed receipt results and catalog resources are not a
   marshal fixed point), so the first implementation's stamp and verify disagreed on
   every freshly written trace. Fix: `canonicalTraceBytes` canonicalizes the RAW document
   bytes (generic decode → strip integrity fields → re-marshal); `Trace.CanonicalContent`
   and `op.LoadTrace` both route through it.
2. **`signing.CanonicalDocument` stripped only `signature`.** With the new `checksum`
   field in trace documents, signature verification in `writ verify` failed until the
   integrity-field pair was stripped there too — aligning it with the canonical
   convention (5-receipt-integrity.md).

## Verification

- New `op.LoadTrace` boundary tests in `pkg/op/trace_test.go`: round-trip, stamp
  idempotence, missing-checksum refusal, tampered-document refusal.
- Full `make test` green (including `writ verify`'s store-document policy tests, which
  now exercise checksummed traces end to end); `make vet` pass; `gofmt -l` clean;
  errcheck stays at 0; no new lint findings in touched files.

## Design documentation

`docs/architecture/5-receipt-integrity.md` gains "§ Document Integrity: Graphs and
Traces" and "§ The Checksum Trust Boundary" — the architecture home for both rulings
(discharges chartered follow-up 2 of lint-errcheck-remainder). Noted there and here:
the doc's older receipt-checksum algorithm text predates `GitStyleChecksum` and needs
reconciliation (chartered).
