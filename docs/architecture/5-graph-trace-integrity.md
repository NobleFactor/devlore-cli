# Graph and Trace Integrity

This document describes the two-tier integrity and authenticity model for the two persisted run documents:
the **graph** (the immutable plan) and the **trace** (one per execution of that graph).

> **Terminology.** A *receipt* is the compensation datum a compensable action returns — the evidence
> sufficient to undo that action ([2.2](2.2-phase-execution.md)). Receipts ride *inside* traces; they are
> not themselves persisted documents. Earlier revisions of this document used "receipt" for the persisted
> run record; that usage is retired (reconciled 2026-08-07).

## Overview

The execution store persists two document kinds, each carrying its own integrity and authenticity fields:

1. **Integrity** (tier 1): detect any modification to document contents — accidental or intentional.
2. **Authenticity** (tier 2): prove who published the document, for sharing and verification.

Drift detection and reconciliation read from these documents: one graph accumulates a stack of traces —
one per execution — and `writ status` classifies live state against the latest trace's recorded content
identity ([5.1](5.1-reconciliation.md)).

## Tier 1: Checksum (Integrity)

Settled 2026-07-26/27. Every persisted run document carries a tier-1 checksum computed the same way:

| Document | Checksum | Stamped | Verified |
|---|---|---|---|
| Graph | `GitStyleChecksum("graph", canonical)` | at build (`op.NewGraph`) | `op.LoadGraph` recomputes and compares |
| Trace | `GitStyleChecksum("trace", canonical)` | at persist (`op.SaveTrace`) | `op.LoadTrace` recomputes and compares |

**The stamp is unskippable, and that is the point (2026-08-17).** `op.SaveTrace` stamps rather than trusting
its caller to, because `op.LoadTrace` refuses a document carrying no checksum: a save that left stamping to the
caller could write a document nothing can read, and the failure would surface at the next load rather than at
the write. Stamping is idempotent — the canonical bytes exclude the checksum field — so `SaveTrace` stamps
unconditionally. Signing is not folded in for the same reason it is not skippable: the checksum belongs to the
artifact, the key belongs to whoever publishes it, so the caller signs and `SaveTrace` persists.

`GitStyleChecksum` is the git-object construction — `SHA256("<type> <len>\0" ‖ content)`, rendered
`"sha256:<hex>"` (`pkg/op/helpers.go`). The type word is fixed per document kind; the filename is never
part of the preimage. Each document's canonical form excludes both integrity fields (`checksum`,
`signature`); the tier-2 signature covers the same canonical bytes, so integrity and authenticity verify
independently. A document with a missing or mismatched checksum is refused at load — there is no
unverified read path.

**Format must never leak into identity** (settled 2026-07-27, after a leak was found and fixed):

1. Every struct in a persisted document graph carries matching snake_case `json` and `yaml` tags —
   an untagged field renders different keys per format (`name:` vs `Name`) and silently forks the
   document. `Variable`/`VariableSource` were the violation; the cross-format identity tests
   (`graph_format_identity_test.go`, `trace_format_identity_test.go`) are the regression net.
2. Canonicalization normalizes format-variant scalars: timestamps render as UTC RFC3339Nano strings
   (YAML parses unquoted RFC3339 into `time.Time`; JSON carries the same value as a string — see
   `normalizeCanonicalValue` in `pkg/op/trace.go`).
3. Still unlegislated (single-codec plan): non-integral float rendering, int64 beyond float64's 2^53
   through `encoding/json`, and null-versus-absent semantics.

### The Checksum Trust Boundary

Settled 2026-07-26: **the checksum is the trust boundary for documents read from disk.**

- **Up to and including checksum verification**, the bytes are untrusted. Unreadable file, malformed
  encoding, digest mismatch — expected external conditions (corruption, tamper, partial write) — are
  **errors**: returned and handled, never panics.
- **After verification**, the document is proven byte-identical to what was written, so any
  read-related failure — an absent-where-required field, a mistyped field, an unparseable embedded
  value, a dangling intra-document reference — can only be a bug in our own serialization or
  decoding, and **panics** via `pkg/assert` (strict extent, approved 2026-07-27). Interactions with
  external systems during restore (package-manager discovery, filesystem probes) are not document
  reads and stay on the error side.

Decode helpers on the verified side assert; the load boundary itself errors. Delivery:
[trace-checksum](../plans/trace-checksum.md).

## Tier 2: Signature (Authenticity)

The signing model is publisher identity with verifier-side trust (`pkg/signing`, phase-8 step 46):

- A raw **ssh-ed25519** signature over the document's namespace-prefixed canonical bytes.
  `NamespaceGraph` / `NamespaceTrace` give domain separation; no envelope, no hash options — the
  algorithm names the whole ciphersuite.
- The publisher's public key rides the document in OpenSSH wire format.
- Trust is resolved by the verifier against an OpenSSH `allowed_signers` file the verifier owns.
- The default custody tier is the developer's SSH key (`~/.ssh/id_ed25519`), with a generated local
  Ed25519 keyfile as the fallback. The ssh-agent, cloud-KMS, and keyless tiers are chartered follow-ons.

Signing at persist time is **best effort** (`signArtifact`, `cmd/internal/cli/store.go`): when no signer
resolves, the document writes unsigned and verification reports the fact — persistence never fails on
signing. `writ verify` performs signature verification against the verifier's trust file.

## The Execution Store

Graphs and traces are distinct artifacts with one-graph-to-many-traces cardinality
(`cmd/internal/cli/store.go`):

```
${XDG_STATE_HOME:-~/.local/state}/devlore/
├── graphs/
│   └── sha256-<hex>.yaml              # the immutable plan, written once, keyed by checksum
├── traces/
│   └── sha256-<hex>/                  # per-graph subdirectory, keyed by the graph's checksum
│       ├── 20260807T041500Z.yaml      # one trace per execution, UTC-timestamped
│       ├── 20260807T093012Z.yaml
│       └── latest.yaml -> 20260807T093012Z.yaml
└── index.ndjson                        # the run index: one line per graph/trace event
```

- A graph persists once (`cli.WriteGraph` is idempotent by checksum); distinct runs of the same plan
  share one persisted graph.
- Each run appends a timestamped trace to the graph's subdirectory (`cli.WriteTrace`) and repoints the
  `latest.yaml` symlink — the convenience entry point for drift detection, reconciliation, and
  pause/restart.
- A trace ties back to its graph through `op.Trace.GraphChecksum` (== the graph's checksum); the shared
  checksum is also the subdirectory name, so trace→graph lookup is direct.
- Both writes append to the run index, carrying tool and scope so index readers can filter without
  opening documents.

## Verification

Every read path funnels through the trust boundary:

- **Graphs**: `op.LoadGraph` recomputes the canonical checksum and refuses a mismatch.
- **Traces**: `cli.LoadTrace` → `op.LoadTrace`, same refusal; `cli.LoadLatestTrace` resolves the
  per-graph `latest.yaml` first.
- **Signatures**: `pkg/signing` verifies the ssh-ed25519 signature over the namespace-prefixed canonical
  bytes and resolves the publisher's key against `allowed_signers`; surfaced through `writ verify`.

## Security Considerations

1. **The checksum is not authentication**: it detects modification but proves no origin — anyone can
   compute a valid checksum. Authenticity is tier 2's job.
2. **Signature trust is verifier-owned**: a valid signature proves possession of the publishing key;
   whether that key is *trusted* is decided solely by the verifier's `allowed_signers` file.
3. **Unsigned documents are visible, not fatal**: best-effort signing means a document may persist
   unsigned; verification reports the absence rather than silently passing.

## Related Documents

- [Emergent System Model](1-system-model.md) — the system model, dependency taxonomy
- [Audit, Reconciliation, and Recovery](5.1-reconciliation.md) — drift detection over the trace stack
- [Recovery Serialization](5.2-recovery-serialization.md) — what a trace carries
- [Recovery Site](5.3-recovery-site.md) — pause/restart on the persisted documents
- [Action Namespaces](3-operation-namespaces.md) — engine actions
