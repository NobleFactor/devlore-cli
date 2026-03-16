---
title: "Extract registry.Transport interface"
issue: pending
status: complete
created: 2026-03-15
updated: 2026-03-15
---

# Plan: Extract registry.Transport Interface

## Summary

Extract the registry transport abstraction from `internal/lorepackage` into a new `internal/registry` package. The
`lorepackage.Provider` interface becomes `registry.Transport`, `GitProvider` becomes an unexported `gitTransport`, and a
single `NewTransport(cfg)` constructor selects the implementation by configuration. Today that is always git; the design
accommodates OCI and HTTP transports in the future without changing consumers.

## Goals

1. **Eliminate name collision**: `lorepackage.Provider` collides with `model.Provider` (AI/LLM) — rename to
   `registry.Transport` for clarity
2. **Single constructor**: `registry.NewTransport(cfg)` replaces `lorepackage.NewGitProvider(url, branch)` — transport
   selection is an implementation detail
3. **Hide concrete types**: `gitTransport` is unexported — callers hold `registry.Transport`, never the concrete type
4. **Clean package boundary**: transport logic (git clone, pull, sync) lives in `internal/registry`, registry domain
   logic (knowledge domains, packages, lifecycle) stays in `internal/lorepackage`

## Non-Goals

- No new transport implementations (OCI, HTTP) in this plan — structure only
- No changes to sync behavior, caching, or version resolution logic
- No changes to the `SyncInfo` YAML format (the `provider: "git"` field stays as a string label)

## Current State

| Component                 | Location                                 | Notes                                        |
| ------------------------- | ---------------------------------------- | -------------------------------------------- |
| `Provider` interface      | `internal/lorepackage/registry.go:54-62` | `Sync()` + `Name()`                          |
| `GitProvider` struct      | `internal/lorepackage/git.go:30`         | 14 methods, ~360 lines                       |
| `NewGitProvider`          | `internal/lorepackage/git.go:36`         | Called from `NewRegistry()` and `New()`      |
| `SyncOptions`             | `internal/lorepackage/registry.go:65-67` | Passed to `Sync()`                           |
| `SyncResult`              | `internal/lorepackage/registry.go:70-76` | Returned from `Sync()`                       |
| `SyncInfo`                | `internal/lorepackage/registry.go:79-84` | Persisted YAML — has `Provider` string field |
| `Registry.provider` field | `internal/lorepackage/registry.go:26`    | Typed as `Provider`                          |

### Callers of GitProvider methods beyond the Transport interface

`Registry` type-asserts `r.provider.(*GitProvider)` in 5 places to access git-specific methods:
`ListVersions`, `ResolveVersion`, `CheckoutVersion`, `HasTag`, `CurrentRef`, `CurrentVersion`, `Branch`, `RepoURL`.

These methods are not on the `Provider` interface today. They need to either join the `Transport` interface or remain
accessible through a different mechanism.

## Design

### Package structure

```text
internal/registry/
  transport.go    — Transport interface, Config, NewTransport, SyncOptions, SyncResult
  git.go          — gitTransport (unexported), newGitTransport (unexported)
```

### Transport interface

```go
package registry

type Transport interface {
    Sync(ctx context.Context, cacheDir string, opts SyncOptions) (*SyncResult, error)
    Name() string
    ListVersions(ctx context.Context, cacheDir string) ([]string, error)
    ResolveVersion(ctx context.Context, cacheDir, version string) (string, error)
    CheckoutVersion(ctx context.Context, cacheDir, version string) error
    HasTag(ctx context.Context, cacheDir, tag string) (bool, error)
    CurrentRef(ctx context.Context, cacheDir string) (string, error)
    CurrentVersion(ctx context.Context, cacheDir string) (string, error)
}
```

The interface includes the version/tag methods that `Registry` currently type-asserts for. This eliminates the
`(*GitProvider)` type assertions entirely.

### Config and constructor

```go
type Config struct {
    URL    string
    Branch string
}

func NewTransport(cfg Config) Transport {
    return newGitTransport(cfg.URL, cfg.Branch)
}
```

Today `NewTransport` always returns `gitTransport`. When OCI or HTTP transports are added, the constructor inspects
`cfg.URL` scheme or adds a `Type` field to `Config`.

### Migration in lorepackage

`Registry` changes:

```go
// Before
provider  Provider  // lorepackage.Provider

// After
transport registry.Transport
```

`NewRegistry()` changes:

```go
// Before
provider: NewGitProvider(regCfg.URL, regCfg.Branch)

// After
transport: registry.NewTransport(registry.Config{URL: regCfg.URL, Branch: regCfg.Branch})
```

The 5 type assertions `r.provider.(*GitProvider)` become direct interface calls on `r.transport`.

## Implementation Phases

### Phase 1: Create `internal/registry` package — complete

- [x] Create `internal/registry/transport.go` with `Transport` interface, `Config`, `NewTransport`, `SyncOptions`,
      `SyncResult`
- [x] Create `internal/registry/git.go` — move `GitProvider` logic, rename to unexported `gitTransport`
- [x] Move `SyncOptions`, `SyncResult`, and `SyncInfo` types from `lorepackage/registry.go` to `registry/transport.go`
- [x] `go vet` passes

### Phase 2: Migrate `internal/lorepackage` — complete

- [x] Change `Registry.provider` field to `transport registry.Transport`
- [x] Update `NewRegistry()` to use `registry.NewTransport()`
- [x] Update `New()` to accept `registry.Transport` instead of `Provider`
- [x] Remove all `(*GitProvider)` type assertions — use interface methods
- [x] Delete `Provider` interface, `NewGitProvider`, `SyncOptions`, `SyncResult`, `SyncInfo` from `lorepackage`
- [x] Delete `lorepackage/git.go` — replaced by `registry/git.go`
- [x] Remove `Registry.ForceTags()` and `Registry.Branch()` — ForceTags moves to `registry.Config`, Branch removed (no callers)
- [x] `go vet` passes

### Phase 3: Update callers — complete

- [x] Update `internal/lore/commands.go` — `lorepackage.SyncOptions{}` → `registry.SyncOptions{}`
- [x] Update `internal/writ/migrate_cmd.go` — `lorepackage.SyncOptions{}` → `registry.SyncOptions{}`
- [x] Update `lorepackage/registry_test.go` — `NewGitProvider()` calls become `registry.NewTransport()`
- [x] E2e tests use `lorepackage.New("test", nil, ...)` — nil transport works, no changes needed
- [x] Unit tests pass

## Files to Create/Modify

| File                                    | Action | Purpose                                                                    |
| --------------------------------------- | ------ | -------------------------------------------------------------------------- |
| `internal/registry/transport.go`        | Create | Transport interface, Config, NewTransport, SyncOptions, SyncResult         |
| `internal/registry/git.go`              | Create | gitTransport implementation (moved from lorepackage/git.go)                |
| `internal/lorepackage/git.go`           | Delete | Replaced by registry/git.go                                                |
| `internal/lorepackage/registry.go`      | Modify | Remove Provider interface, SyncOptions, SyncResult; use registry.Transport |
| `internal/lorepackage/registry_test.go` | Modify | Update constructor calls                                                   |

## Resolved Questions

- [x] `Branch()` and `RepoURL()` stay off the `Transport` interface — they are git-specific implementation details on
      the unexported `gitTransport`. Callers that need this information consult the config.
