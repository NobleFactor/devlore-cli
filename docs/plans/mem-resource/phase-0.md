# Phase 0: Resource Identity

**Status**: Done
**PRs**: #192–#197

## Summary

Simplify the `Resource` interface from 6 methods to 3 (`URI()`,
`Resolve()`, `String()`). Correct URI schemes to match their proper
forms (opaque vs hierarchical). Rename `net` → `appnet`.

See [4.1-resource-identity.md](../../architecture/4.1-resource-identity.md)
for the full design.

## Changes

### Interface simplification — `pkg/op/resource.go`

- Removed `Scheme()`, `Host()`, `Path()` from `Resource` interface
- Kept them on `ResourceBase` as parsing helpers (not interface methods)
- Added `Opaque()` and `Fragment()` parsing helpers to `ResourceBase`
- Removed `NewURI(r Resource) string` method
- Renamed `SchemeNet` → `SchemeAppNet`, value `"net"` → `"appnet"`

### Per-provider URI corrections

| Provider | Before | After |
|----------|--------|-------|
| `file` | `file:///path` | `file:///path` (unchanged, already hierarchical) |
| `pkg` | Various | purl-compliant: `pkg:<type>/<name>[@<version>]` |
| `svc` | `svc:///<name>` | Opaque: `svc:<name>` |
| `net` → `appnet` | `net:<url>` | Opaque: `appnet:<escaped-url>` with `#`/`?` escaping |
| `git` | Various | Opaque: `git:<encoded-repo>[?path=...]#<commit>` |

### Package rename

- `pkg/op/provider/net/` → `pkg/op/provider/appnet/`
- All imports and references updated

## Files Modified

- `pkg/op/resource.go` — interface slimming, parsing helpers
- `pkg/op/provider/file/resource.go` — removed interface methods
- `pkg/op/provider/pkg/resource.go` — purl-compliant URI
- `pkg/op/provider/service/resource.go` — opaque URI
- `pkg/op/provider/net/` → `pkg/op/provider/appnet/` — rename + opaque URI
- `pkg/op/provider/git/resource.go` — opaque URI with query/fragment
- All associated test files
