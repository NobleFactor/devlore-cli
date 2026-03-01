# Plan: Star Project Directory Structure

## Summary

Standardize the directory structure for projects that consume star, and reorganize noblefactor-ops to follow the same pattern.

## Target Structure

### Consumer Project Layout

```
project/
├── star/
│   ├── config.yaml              # Project configuration
│   └── extensions/              # Project-specific extensions
│       └── com.example.MyExt/
│           ├── extension.yaml
│           └── commands/
└── ... (other project files)
```

### After `star self install`

```
~/.local/
├── bin/
│   └── star                     # Binary
└── share/
    ├── star/
    │   └── extensions/          # User-installed extensions
    ├── bash-completion/completions/star
    ├── fish/vendor_completions.d/star.fish
    ├── zsh/site-functions/_star
    └── man/man1/star*.1

~/.config/
└── star/
    └── config.yaml              # User default configuration
```

## Config Loading Hierarchy

Priority (highest to lowest):
1. `./star/config.yaml` - Project config
2. `~/.config/star/config.yaml` - User defaults
3. Built-in defaults

## Extension Discovery Paths

Priority (first match wins):
1. `./star/extensions/` - Project local
2. Walk up parent directories looking for `star/extensions/`
3. `~/.local/share/star/extensions/` - User extensions

## Changes to noblefactor-ops

### Files to Move

| From | To |
|------|-----|
| `star.yaml` | `star/config.yaml` |
| `extensions/` | `star/extensions/` |

### Files to Delete

| Path | Reason |
|------|--------|
| `ops/*.star` | Stale, covered by extensions |
| `ops/test/` | Stale |

### Files to Move to Other Repos

| File | Destination |
|------|-------------|
| `ops/devlore-cli/starlark-api.star` | devlore-cli repo |
| `ops/devlore-registry/*.star` | devlore-registry repo |

## Code Changes

### 1. Update Config Paths

**File:** `internal/config/config.go`

```go
// projectConfigPath returns path to project config
func projectConfigPath() string {
    return filepath.Join("star", "config.yaml")
}

// userConfigPath returns path to user config
func userConfigPath() string {
    configHome := os.Getenv("XDG_CONFIG_HOME")
    if configHome == "" {
        configHome = filepath.Join(os.UserHomeDir(), ".config")
    }
    return filepath.Join(configHome, "star", "config.yaml")
}
```

### 2. Update Extension Discovery

**File:** `internal/extension/discovery.go`

```go
func DefaultSearchPaths() []string {
    var paths []string

    // Walk up from CWD looking for star/extensions
    cwd, _ := os.Getwd()
    current := cwd
    for {
        extDir := filepath.Join(current, "star", "extensions")
        if dirExists(extDir) {
            paths = append(paths, extDir)
        }
        parent := filepath.Dir(current)
        if parent == current {
            break
        }
        current = parent
    }

    // User extensions (XDG_DATA_HOME)
    dataHome := os.Getenv("XDG_DATA_HOME")
    if dataHome == "" {
        dataHome = filepath.Join(os.UserHomeDir(), ".local", "share")
    }
    paths = append(paths, filepath.Join(dataHome, "star", "extensions"))

    return paths
}
```

### 3. Remove ops/ Loading

**File:** `cmd/star/main.go`

- Remove `findOpsDir()` function
- Remove `loadStarlarkCommands()` call that loads from ops/
- All commands now come from extensions only

### 4. Update Self-Install

**File:** `internal/cli/selfinstall.go`

- Default to `~/.local` when no path specified
- Install extensions to `<root>/share/star/extensions/`
- Install config template to `~/.config/star/config.yaml` (if not exists)

### 5. Extension Config Schema

Extensions declare config in `extension.yaml`:

```yaml
extension: com.noblefactor.star.LintGo

config:
  schema:
    lint.go:
      type: object
      properties:
        path:
          type: string
          default: "./..."
        skip_mod_tidy:
          type: boolean
          default: false
```

Values go in `star/config.yaml`:

```yaml
lint:
  go:
    path: "./..."
    skip_mod_tidy: false
```

## Files to Modify

| File | Changes |
|------|---------|
| `internal/config/config.go` | Update paths to `star/config.yaml` |
| `internal/extension/discovery.go` | Update paths to `star/extensions/` |
| `cmd/star/main.go` | Remove ops/ loading |
| `internal/cli/selfinstall.go` | Default to ~/.local, update paths |

## Migration Steps

1. Move `extensions/` → `star/extensions/`
2. Move `star.yaml` → `star/config.yaml`
3. Update code to use new paths
4. Delete `ops/` directory
5. Copy devlore scripts to their repos (separate PRs)
6. Update CI workflow

## Acceptance Criteria

1. `star lint go` works from noblefactor-ops with new structure
2. `star self install` defaults to `~/.local`
3. Config loads from `star/config.yaml`
4. Extensions discovered from `star/extensions/`
5. No references to `ops/` remain in codebase
