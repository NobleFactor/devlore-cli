# Config Consolidation Plan

## Goal

Centralize config so both `writ` and `lore` consume it from the same place.

## Important: No Legacy

**We have no legacy. We do not need aliases or any concern for backwards compatibility. We are in a period of rapid change. It is dangerous to leave anything that smacks of preserving legacy.**

When migrating code:
- Delete old types completely
- Update all callers to use new types directly
- No type aliases
- No deprecation warnings
- No shims or adapters

## Non-Goals

- Refactoring struct design (#68)
- Generating schema from code (#66)

## Solution

Create `internal/config/` package with all config types in one place.

## Config Package Structure

```
internal/config/
```

### `config.go`
- `Config` - Root config struct
- `Load()` - Load from file + env + keystore
- `Save()` - Save to file (API key to keystore)

### `lore.go`
- `LoreConfig`
- `Preferences`
- `Sources`

### `model.go`
- `ModelConfig`

### `registry.go`
- `RegistryConfig`

### `writ.go`
- `WritConfig`

## Files to Modify

### Phase 1: Create config package
- `internal/config/config.go`
- `internal/config/lore.go`
- `internal/config/model.go`
- `internal/config/registry.go`
- `internal/config/writ.go`

### Phase 2: Migrate consumers
- `internal/lore/commands.go`
- `internal/lorepackage/registry.go`
- `internal/model/config.go`
- `internal/writ/config.go`

### Phase 3: Update schema
- `schema/devlore-config.json`
- `schema/defaults/*.yaml`

## Verification

1. `go test ./...`
2. `lore config list` works
3. `writ config list` works
4. Both show same registry/model values

## GitHub Issues

- #67 - Consolidate config types (this work)
- #68 - Refactor config struct design (follow-up)
- #66 - Generate JSON schemas from Go structs (follow-up)
