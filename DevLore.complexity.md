# DevLore Complexity Measurements

Generated: 2026-01-29 | Updated: 2026-02-17

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
| `execution` | Execution graph, actions, phases, recovery |
| `host` | Platform detection, package managers |
| `lore` | Lore command implementation |
| `model` | AI model providers |
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

- `Action` - Execution action interface (Do/Undo with typed slots)
- `PlanBindings` - Starlark plan API (package, file, service actions)
- `PackageManager` - Native PM abstraction (apt, dnf, brew, winget)
- `Host` - Platform abstraction

---

## devlore-registry (Starlark)

### Package Inventory

| Package | Platforms | Phase Scripts | Status |
|---------|-----------|---------------|--------|
| docker | 4 | 40 | Current API |
| astro | 1 | 4 | Current API |
| aws-cli | 1 | 4 | Current API |
| azure-cli | 1 | 4 | Current API |
| gcloud | 1 | 4 | Current API |
| kubectl | 1 | 4 | Current API |
| pandoc | 1 | 4 | Current API |
| terraform | 1 | 4 | Current API |
| xcode | 1 | 4 | Current API |
| xcode-clt | 1 | 4 | Current API |

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

### Phase Script API

All phase scripts use the three-argument signature:

```starlark
def phase(package, system, plan):
```

---

## Architecture Quality

### Strengths

1. **Clear separation of concerns** - `internal/` packages have single responsibilities
2. **Platform abstraction** - `host/` package isolates OS-specific code
3. **Unified execution model** - Both writ and lore use same action-based execution graph
4. **Namespaced API** - `plan.package.*`, `plan.file.*`, `plan.template.*` provides clear structure
5. **Saga-pattern compensation** - All resource providers support forward/undo with typed receipts

### Areas for Improvement

1. **Test coverage** - 20% of LOC is test code (target: 30%+)
2. **Documentation** - Knowledge base exists but needs more examples

### Complexity Indicators

| Metric | Value | Assessment |
|--------|-------|------------|
| Avg file size | 257 LOC | Good (under 300) |
| Interface count | 16 | Appropriate for project size |
| Package depth | 2 levels | Simple hierarchy |
| Circular deps | 0 | Clean dependency graph |

---

## Technical Debt

### Medium Priority

1. **Increase test coverage** - Add tests for starlark bindings
2. **Add integration tests** - End-to-end pipeline tests

---

## File Organization

```
devlore-cli/
├── cmd/                    # CLI commands (965 LOC)
│   ├── lore/              # lore command tree
│   └── writ/              # writ command tree
├── internal/              # Core packages (31,670 LOC)
│   ├── execution/         # Execution graph, actions, phases
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
