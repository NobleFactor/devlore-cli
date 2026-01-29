# DevLore Complexity Measurements

Generated: 2026-01-29

## Summary

| Repository | Files | Lines of Code | Packages/Modules |
|------------|-------|---------------|------------------|
| devlore-cli | 127 Go | 32,685 | 33 packages |
| devlore-registry | 76 Starlark | 7,191 | 10 lore packages |

---

## devlore-cli (Go)

### Code Distribution

| Directory | Lines of Code | Purpose |
|-----------|---------------|---------|
| `cmd/` | 965 | CLI entry points |
| `internal/` | 31,670 | Core implementation |
| Test files | 6,534 | Unit tests (19 files) |

**Source/Test Ratio:** 4.0:1 (26,101 source : 6,534 test)

### Internal Packages (13)

| Package | Description |
|---------|-------------|
| `bindgen` | Starlark binding generation |
| `cli` | Terminal UI, colors, prompts |
| `credentials` | Credential management |
| `engine` | Execution graph and operations |
| `host` | Platform detection, package managers |
| `lore` | Lore command implementation |
| `model` | Data models and DTOs |
| `pwsh` | PowerShell integration |
| `registry` | Registry client, package resolution |
| `shell` | Shell execution utilities |
| `starlark` | Starlark runtime and bindings |
| `tools` | Cross-cutting utilities |
| `writ` | Writ command implementation |

### Type Metrics

| Type | Count |
|------|-------|
| Interfaces | 16 |
| Structs | 177 |
| Go Packages | 33 |

### Key Interfaces

- `PlanBindings` - Starlark plan API (package, file, service operations)
- `PackageManager` - Native PM abstraction (apt, dnf, brew, winget)
- `Operation` - Engine operation interface
- `Host` - Platform abstraction

---

## devlore-registry (Starlark)

### Package Inventory

| Package | Platforms | Phase Scripts | Status |
|---------|-----------|---------------|--------|
| docker | 4 | 40 | Migrated to new API |
| astro | 1 | 4 | Legacy API |
| aws-cli | 1 | 4 | Legacy API |
| azure-cli | 1 | 4 | Legacy API |
| gcloud | 1 | 4 | Legacy API |
| kubectl | 1 | 4 | Legacy API |
| pandoc | 1 | 4 | Legacy API |
| terraform | 1 | 4 | Legacy API |
| xcode | 1 | 4 | Legacy API |
| xcode-clt | 1 | 4 | Legacy API |

### Platform Coverage

| Platform | Phase Scripts |
|----------|---------------|
| Darwin | 10 |
| Linux.Debian | 10 |
| Linux.Fedora | 10 |
| Windows | 10 |

### Lifecycle Coverage

| Lifecycle | Phase Scripts | Phases |
|-----------|---------------|--------|
| Deploy | 16 | provision, install, configure, verify |
| Upgrade | 12 | prepare, upgrade, verify |
| Decommission | 12 | unprovision, uninstall, cleanup |

### API Migration Status

| API Version | Files | Signature |
|-------------|-------|-----------|
| New (v2) | 40 | `def phase(package, system, plan):` |
| Legacy (v1) | 36 | `def phase():` |

**Migration Progress:** 53% complete (40/76 files)

---

## Architecture Quality

### Strengths

1. **Clear separation of concerns** - `internal/` packages have single responsibilities
2. **Platform abstraction** - `host/` package isolates OS-specific code
3. **Unified execution model** - Both writ and lore use same engine
4. **Namespaced API** - `plan.package.*`, `plan.file.*` provides clear structure

### Areas for Improvement

1. **Test coverage** - 20% of LOC is test code (target: 30%+)
2. **API consistency** - 47% of registry scripts still use legacy API
3. **Documentation** - Knowledge base exists but needs more examples

### Complexity Indicators

| Metric | Value | Assessment |
|--------|-------|------------|
| Avg file size | 257 LOC | Good (under 300) |
| Interface count | 16 | Appropriate for project size |
| Package depth | 2 levels | Simple hierarchy |
| Circular deps | 0 | Clean dependency graph |

---

## Technical Debt

### High Priority

1. **Migrate 9 legacy packages** to new API signature
   - astro, aws-cli, azure-cli, gcloud, kubectl, pandoc, terraform, xcode, xcode-clt

### Medium Priority

2. **Implement missing plan operations**
   - `plan.download()` - File downloads
   - `plan.verify()` - Verification checks
   - `plan.file.write()` - File creation
   - `plan.file.remove()` - File deletion
   - `plan.user.add_to_group()` - User management

### Low Priority

3. **Increase test coverage** - Add tests for starlark bindings
4. **Add integration tests** - End-to-end pipeline tests

---

## File Organization

```
devlore-cli/
├── cmd/                    # CLI commands (965 LOC)
│   ├── lore/              # lore command tree
│   └── writ/              # writ command tree
├── internal/              # Core packages (31,670 LOC)
│   ├── engine/            # Execution engine
│   ├── host/              # Platform abstraction
│   ├── registry/          # Package registry
│   ├── starlark/          # Starlark runtime
│   └── ...
└── docs/                  # Architecture docs

devlore-registry/
├── packages/              # Lore packages (7,191 LOC)
│   ├── docker/            # Multi-platform (40 scripts)
│   └── .../               # Single-platform (4 scripts each)
└── knowledge/             # AI authoring aids
```
