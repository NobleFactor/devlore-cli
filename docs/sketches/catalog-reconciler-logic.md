# Catalog Reconciler

At the end of 13.0(k), `ResourceCatalog.Resolve` runs a two-path reconciler.
Both paths share a common change-detection front end; they diverge only on
what to do when content actually changes.

## Front end: three-step compare

Every `op.Resource` exposes two change-detection signals:

- **`Etag() (string, error)`** — cheap opaque token. Stat-derived stamp for
  filesystem-backed Resources, the URI itself for content-addressed Resources,
  the HTTP `ETag` header for `appnet`. Suggestive of change.
- **`Digest() (Digest, error)`** — honest content hash. Full read for
  filesystem-backed types; projected from the URI for CAS types. Authoritative.

| URI match? | Etag match? | Digest match? | Action |
|---|---|---|---|
| no | — | — | **First sighting.** Intern. Return new. |
| yes | yes | _(skipped)_ | **Cache hit.** Return existing. Cheapest path. |
| yes | no | yes | **Touch drift.** Refresh Etag. Return existing. No shadow. |
| yes | no | no | **Real content change.** Branch on `Addressing()`. |

The Digest is computed only when an Etag mismatch demands a closer look.

## Path A — Location-based (`AddressingLocation`)

Types: `file.Resource`, `git.Resource`, `appnet.Resource`, `pkg.Resource`,
`service.Resource`.

Identity is location; bytes at the location are mutable. URI-match +
Digest-mismatch is a legitimate update event.

**Action: shadow.** Append `current` to `shadowed`; install the new Resource
as `current`; store new Etag and Digest; transition predecessor
`Active → Active'`.

State machine: `Created → Active → Active' → Gone`.

## Path B — Content-addressed (`AddressingContent`)

Types: `mem.Resource`, `stream.Resource`, `function.Resource`,
`json.Resource`, `yaml.Resource`.

Identity is the digest. URIs take the form
`tag:devlore.noblefactor.com,2026-01-01:sha256:<hex>#<typeID>`. URI-match +
Digest-mismatch is *logically impossible* by construction; new bytes mean a
new URI and a new catalog entry, never a shadow on the existing one.

**Action: error.** Treat the mismatch as corruption (corrupt store / hash
collision / minting bug). Transition to `Gone`.

State machine: `Created → Active → Gone`.

## Catalog entry

```go
type catalogEntry struct {
    current  *Resource
    etag     string
    digest   Digest
    shadowed []*Resource  // always empty for AddressingContent
}
```

`map[string]*catalogEntry` keyed on URI.

## Sentinel: `AddressingUnknown`

Every concrete Resource type self-declares `Addressing()`. The zero value is
`AddressingUnknown` — a tripwire, not a default. A boot test
(`pkg/op/addressing_test.go`) asserts no announced Resource returns it; the
catalog's branch panics if encountered. No implicit "location is the default"
bias.

## What this is not

- **Not a push model.** No file watchers, no polling. Divergence is detected
  at `Resolve` time when consumers ask, not asynchronously.
- **Not reference-counted.** Eviction policy for the `shadowed` chain (size
  cap, age, manual reclaim) is deferred until usage data informs it.
- **Not a unified shadow flow.** CAS Resources never accumulate a `shadowed`
  chain — the two paths are genuinely distinct.

---

> Companion explainer for phase-8 sub-step **13.0(k)**, sections D5–D9. The
> normative source is the 13.0(k) row in
> `docs/plans/extract-starlark-from-op/phase-8.md`.