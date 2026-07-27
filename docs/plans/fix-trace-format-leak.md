---
title: "Fix the trace format leak"
issue: TBD
status: complete
created: 2026-07-27
updated: 2026-07-27
---

# Plan: Fix the trace format leak

Escalated 2026-07-27 from the format-identity test evidence: a JSON-saved trace failed
checksum verification while graphs passed all cross-format tests — serialization format
was leaking into trace identity.

## Root cause

1. `Variable` and `VariableSource` serialized untagged, violating the snake_case-at-
   serialization mandate: YAML lowercased their field names while JSON emitted Go names
   (`name:` vs `"Name"`), forking the logical document per format.
2. Timestamps: YAML parses unquoted RFC3339 scalars into `time.Time` while JSON documents
   carry them as strings; the two re-marshal differently, so canonical bytes diverged
   wherever `at:` appears.

## Changes

1. `pkg/op/variable.go` — `Variable` (`name`, `field` omitempty, `value`, `source`) and
   `VariableSource` (`kind`, `name`) gain matching snake_case json+yaml tags.
   (`VariableSourceKind` serializes as a number in both formats — symmetric, not a leak;
   name-based serialization is a codec-plan candidate.)
2. `pkg/op/trace.go` — `canonicalTraceBytes` normalizes format-variant scalars via
   `normalizeCanonicalValue`: timestamps render as UTC RFC3339Nano strings.
3. `pkg/op/trace_format_identity_test.go` — the two skipped tests un-skip; the fixture
   gains a `Transitions` entry (proving the timestamp rule) and a populated
   `VariableSource`. With `graph_format_identity_test.go` (all four passing since
   creation), the eight tests are the permanent cross-format regression net.
4. `docs/architecture/5-receipt-integrity.md` — "Format must never leak into identity"
   rules recorded, including the still-unlegislated corners left to the single-codec plan
   (non-integral floats, 2^53 through encoding/json, null-versus-absent).

## Greenfield consequence

Traces stamped under the broken canonical are refused after this lands (the recomputed
canonical differs); new runs regenerate them. Same no-legacy stance as the checksum
introduction.

## Verification

- All eight format-identity tests pass (graph 4, trace 4 — including JSON verify and
  cross-format identity).
- Full `make test` green; `make vet` pass; `gofmt -l` clean.
