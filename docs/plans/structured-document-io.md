---
title: "Structured Document I/O Package"
issue: https://github.com/NobleFactor/devlore-cli/issues/218
pr: https://github.com/NobleFactor/devlore-cli/pull/221
status: complete
created: 2026-03-14
updated: 2026-03-15
---

# Plan: Structured Document I/O Package

## Summary

Extract a small `internal/document` package that encapsulates the read-file-deserialize and serialize-write-file
patterns used across 41 call sites in 17 files. The package handles YAML and JSON structured documents with consistent
error wrapping, permission modes, directory creation, and optional-file semantics.

## Goals

1. **Eliminate boilerplate**: Replace 5-10 line read/unmarshal and marshal/write sequences with single function calls
2. **Consistent error messages**: Uniform `"read <path>: %w"` and `"parse <path>: %w"` wrapping across the codebase
3. **Consistent permissions**: Default `0o600` for files, `0o750` for directories — overridable when needed
4. **Format inference**: Detect YAML vs JSON from file extension once, in one place

## Non-Goals

- Not a general-purpose file I/O library (no binary, streaming, archive, or SOPS support)
- Not a config framework (no env var overrides, no viper integration)
- No new dependencies — uses only `encoding/json` and `gopkg.in/yaml.v3`

## Current State

The codebase has 41 structured document I/O sites across 17 files:

| Pattern                                 | Count | Example locations                                            |
| --------------------------------------- | ----- | ------------------------------------------------------------ |
| `os.ReadFile` → `yaml.Unmarshal`        | 19    | config, receipts, manifest, registry, lifecycle, credentials |
| `os.ReadFile` → `json.Unmarshal`        | 4     | config schema validation, manifest                           |
| `os.ReadFile` → format-switch unmarshal | 4     | cli/config, manifest (auto-detect by extension)              |
| `yaml.Marshal` → `os.WriteFile`         | 10    | config, credentials, synthetic cache, markers                |
| `json.MarshalIndent` → `os.WriteFile`   | 2     | e2e harness                                                  |
| Format-switch marshal → `os.WriteFile`  | 2     | cli/config                                                   |

### Recurring sub-patterns

**Optional-file reads** (8 sites): `os.IsNotExist` check returns zero value instead of error.

**MkdirAll before write** (12 sites): `os.MkdirAll(dir, 0o750)` paired with the write, duplicated inline each time.

**Format detection** (4 sites): `filepath.Ext` → switch on `.json` vs default YAML, implemented independently in
`cli/config.go` and `manifest/manifest.go`.

## Design

### Complete interface

```go
package document

import "os"

// Read deserializes a structured document from disk into v. Format is inferred from the file extension: .json → JSON,
// .yaml/.yml/anything else → YAML.
func Read(path string, v any) error

// ReadIfExists is like Read but returns (false, nil) when the file does not exist, instead of an error.
func ReadIfExists(path string, v any) (bool, error)

// Write serializes v to disk as a structured document. Format is inferred from the file extension. Creates parent
// directories (0o750) if needed. Default file permission is 0o600; override with WithPerm.
func Write(path string, v any, opts ...Option) error

// Option configures Write behavior.
type Option func(*writeOpts)

// WithPerm overrides the default 0o600 file permission.
func WithPerm(mode os.FileMode) Option

// WithIndent controls JSON indentation (ignored for YAML). Default: 2-space indent for JSON.
func WithIndent(prefix, indent string) Option

// WithHeader prepends a literal string before the serialized content (e.g., a generated-file comment or disclaimer).
func WithHeader(header string) Option
```

That's it. Three functions, three options. No interfaces, no generics beyond `any`, no builders.

### Format detection (unexported)

```go
func formatFromExt(path string) string {
    switch strings.ToLower(filepath.Ext(path)) {
    case ".json":
        return "json"
    default:
        return "yaml"
    }
}
```

Replaces the `configFormat()` helper in `cli/config.go` and the extension switches in `manifest/manifest.go`.

### Error messages

Every error includes the file path so callers never need to re-wrap:

```
read /home/user/.config/devlore/config.yaml: no such file or directory
parse /home/user/.config/devlore/config.yaml: yaml: line 12: mapping values are not allowed in this context
create directory /home/user/.local/share/devlore: permission denied
marshal /home/user/.local/share/devlore/receipts/deploy.yaml: json: unsupported type: chan int
write /home/user/.local/share/devlore/receipts/deploy.yaml: permission denied
```

### Before and after

**Pattern 1 — Simple read** (19 call sites):

```go
// BEFORE (internal/cli/receipts.go:35-45)
data, err := os.ReadFile(path)
if err != nil {
    return nil, fmt.Errorf("read receipt: %w", err)
}
var g op.Graph
if err := yaml.Unmarshal(data, &g); err != nil {
    return nil, fmt.Errorf("parse receipt: %w", err)
}
return &g, nil

// AFTER
var g op.Graph
if err := document.Read(path, &g); err != nil {
    return nil, err
}
return &g, nil
```

**Pattern 2 — Optional-file read** (8 call sites):

```go
// BEFORE (internal/config/config.go:66-74)
data, err := os.ReadFile(Path())
if err != nil && !os.IsNotExist(err) {
    return nil, fmt.Errorf("reading config: %w", err)
}
if err == nil {
    if err := yaml.Unmarshal(data, cfg); err != nil {
        return nil, fmt.Errorf("parsing config: %w", err)
    }
}

// AFTER
if _, err := document.ReadIfExists(Path(), cfg); err != nil {
    return nil, err
}
```

**Pattern 3 — Simple write** (10 call sites):

```go
// BEFORE (internal/config/config.go:100-112)
dir := filepath.Dir(path)
if err := os.MkdirAll(dir, 0o750); err != nil {
    return fmt.Errorf("creating config directory: %w", err)
}
data, err := yaml.Marshal(&fileCfg)
if err != nil {
    return fmt.Errorf("marshaling config: %w", err)
}
return os.WriteFile(path, data, 0o600)

// AFTER
return document.Write(path, &fileCfg)
```

**Pattern 4 — Format-switch read/write** (4 call sites):

```go
// BEFORE (internal/cli/config.go:260-279)
data, err := os.ReadFile(path)
if err != nil {
    if os.IsNotExist(err) {
        return make(map[string]interface{}), nil
    }
    return nil, err
}
var config map[string]interface{}
switch configFormat(path) {
case "json":
    if err := json.Unmarshal(data, &config); err != nil {
        return nil, fmt.Errorf("invalid JSON: %w", err)
    }
default:
    if err := yaml.Unmarshal(data, &config); err != nil {
        return nil, fmt.Errorf("invalid YAML: %w", err)
    }
}

// AFTER
config := make(map[string]interface{})
if _, err := document.ReadIfExists(path, &config); err != nil {
    return nil, err
}
```

**Pattern 5 — Write with header** (2 call sites):

```go
// BEFORE (cmd/indexgen/main.go:261-271)
data, err := yaml.Marshal(index)
if err != nil {
    return err
}
header := "# Auto-generated file list by: go run ./cmd/gen-index\n"
content := header + string(data)
return os.WriteFile(filepath.Join(domainPath, "index.yaml"), []byte(content), 0o600)

// AFTER
header := "# Auto-generated file list by: go run ./cmd/gen-index\n"
return document.Write(filepath.Join(domainPath, "index.yaml"), index, document.WithHeader(header))
```

**Pattern 6 — Write with custom permissions** (1 call site):

```go
// BEFORE (internal/lorepackage/git.go:213-219)
data, err := yaml.Marshal(&info)
if err != nil {
    return err
}
return os.WriteFile(filepath.Join(cacheDir, ".sync-info.yaml"), data, 0o644)

// AFTER
return document.Write(filepath.Join(cacheDir, ".sync-info.yaml"), &info, document.WithPerm(0o644))
```

## Implementation Phases

### Phase 1: Create `internal/document` package — complete

- [x] Create `internal/document/document.go` with Read, ReadIfExists, Write, Option types, and formatFromExt
- [x] Create `internal/document/document_test.go` with tests covering: Read YAML/JSON, ReadIfExists missing/present,
  malformed content parse error, Write YAML/JSON with 0o600, Write creates parent dirs, WithPerm overrides permission,
  WithHeader prepends text, format detection for .yaml/.yml/.json/unknown, error messages include file path,
  round-trip YAML/JSON
- [x] `make check` passes

### Phase 2: Migrate `internal/config` — complete

- [x] `Load()`: `document.ReadIfExists(Path(), cfg)` (line 69)
- [x] `Save()`: `document.Write(Path(), &fileCfg)` (line 104)
- [x] Remove unused imports
- [x] `make check` passes

### Phase 3: Migrate `internal/cli` — complete

- [x] `loadConfig()`: `document.ReadIfExists(path, &config)` (config.go:269)
- [x] `saveConfig()`: `document.Write(path, config)` (config.go:290)
- [x] `configEdit()`: excluded — writes raw `[]byte` defaults, not marshaled struct (same category as selfinstall.go)
- [x] `configFormat()` deleted — replaced by `document.formatFromExt`
- [x] `LoadReceipt()`: `document.Read(path, &g)` (receipts.go:45)
- [x] selfinstall.go left as-is (raw byte writes, not structured docs)
- [x] `make check` passes

### Phase 4: Migrate `internal/credentials` — complete

- [x] `fileGet()`: `document.ReadIfExists(path, &creds)` (file.go:42)
- [x] `fileSet()`: `document.ReadIfExists` + `document.Write` with `WithHeader` (file.go:66, 77)
- [x] `fileDelete()`: `document.ReadIfExists` + `document.Write` (file.go:95, 109)
- [x] `make check` passes

**Note**: Actual count was 5 call sites (2 reads + 2 writes + 1 WithHeader), exceeding the planned 3.

### Phase 5: Migrate `internal/manifest` — complete

- [x] `Load()`: `document.Read(path, &m)` (manifest.go:82)
- [x] `Validate()`: `document.Read(path, &doc)` (manifest.go:100) — additional site migrated beyond plan
- [x] Parse/ValidateBytes left as-is (byte-oriented, not file-oriented)
- [x] `make check` passes

**Note**: `Validate()` was also migrated (reads a file then validates), bringing this phase to 2 sites instead of 1.

### Phase 6: Migrate `internal/lorepackage` — complete

- [x] `SyncInfo()`: `document.ReadIfExists(infoPath, &info)` (registry.go:140)
- [x] `KnowledgeDomain.Index()` left as-is (git tree read)
- [x] `LoadLifecycle()`: `document.Read(path, &lifecycle)` (lifecycle.go:196)
- [x] `SyntheticCache.Get()`: `document.ReadIfExists(path, &info)` (synthetic.go:61)
- [x] `SyntheticCache.Put()`: `document.Write(...)` (synthetic.go:93)
- [x] `SyntheticCache.List()`: `document.ReadIfExists(path, &info)` in loop (synthetic.go:150)
- [x] `writeSyncInfo()`: `document.Write(..., document.WithPerm(0o644))` (git.go:223)
- [x] `make check` passes

### Phase 7: Migrate remaining packages — complete

- [x] `ParseSopsConfig()`: `document.Read(path, &config)` (signing/signer.go:132)
- [x] `loadReceipt()`: `document.Read(path, &g)` (execution/stateview.go:386)
- [x] `WriteMigratedMarker()`: `document.Write(markerPath, &marker)` (writ/migrate/execute.go:125)
- [x] `LoadTestConfig()`: `document.Read(path, &cfg)` (e2e/harness.go:100)
- [x] `WriteReport()` JSON: `document.Write(..., "results.json", r)` (e2e/harness.go:203)
- [x] `WriteReport()` YAML: `document.Write(..., "results.yaml", r)` (e2e/harness.go:208)
- [x] `loadExistingIndex()`: `document.ReadIfExists(indexPath, &index)` (cmd/indexgen/main.go:207)
- [x] `writeIndex()`: `document.Write(..., document.WithHeader(header))` (cmd/indexgen/main.go:281)
- [x] `make check` passes

## Exclusions

These call sites were evaluated and deliberately excluded:

| File                                            | Reason                                                    |
| ----------------------------------------------- | --------------------------------------------------------- |
| `cli/selfinstall.go` (lines 467, 475)           | Writes raw `[]byte` defaults, not marshaled structs       |
| `cli/config.go` (lines 369, 401, 528, 616)      | Unmarshal from embedded `[]byte` schema, not file reads   |
| `manifest/manifest.go` Parse/ValidateBytes      | Byte-oriented API, not file-oriented                      |
| `lorepackage/registry.go` KnowledgeDomain.Index | Reads from git tree via `r.ReadFile()`, not `os.ReadFile` |
| `lore/onboard/onboard.go` (lines 234, 297)      | Unmarshal from HTTP response body, not file               |
| `lore/onboard/onboard.go` fetchContent          | Reads plain text, not structured document                 |
| `lore/onboard/onboard.go` WriteManifest         | Writes pre-rendered `string`, not a Go struct             |
| `lore/commands.go` (line 723)                   | Writes pre-rendered `string`, not a Go struct             |
| `cli/receipts.go` WriteReceipt (line 93)        | Uses streaming YAML encoder, not marshal+write            |
| `lorepackage/registry.go` SignatureIndex        | Unmarshal from `r.ReadFile()` (git tree), not os.ReadFile |

## Files to Create/Modify

| File                                 | Action | Purpose                                   |
| ------------------------------------ | ------ | ----------------------------------------- |
| `internal/document/document.go`      | Create | Package implementation                    |
| `internal/document/document_test.go` | Create | Tests                                     |
| `internal/config/config.go`          | Modify | Replace 2 call sites                      |
| `internal/cli/config.go`             | Modify | Replace 3 call sites, delete configFormat |
| `internal/cli/receipts.go`           | Modify | Replace 1 call site                       |
| `internal/credentials/file.go`       | Modify | Replace 3 call sites                      |
| `internal/manifest/manifest.go`      | Modify | Replace 1 call site                       |
| `internal/lorepackage/registry.go`   | Modify | Replace 1 call site                       |
| `internal/lorepackage/lifecycle.go`  | Modify | Replace 1 call site                       |
| `internal/lorepackage/synthetic.go`  | Modify | Replace 3 call sites                      |
| `internal/lorepackage/git.go`        | Modify | Replace 1 call site                       |
| `internal/signing/signer.go`         | Modify | Replace 1 call site                       |
| `internal/execution/stateview.go`    | Modify | Replace 1 call site                       |
| `internal/writ/migrate/execute.go`   | Modify | Replace 1 call site                       |
| `internal/e2e/harness.go`            | Modify | Replace 3 call sites                      |
| `cmd/indexgen/main.go`               | Modify | Replace 2 call sites                      |

**Total**: 2 files created, 13 files modified, 25 call sites replaced.

## Resolved Questions

- [x] Package name: **`internal/document`** — chosen for clarity and directness
