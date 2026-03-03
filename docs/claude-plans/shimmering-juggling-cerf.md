# Plan: Phase 6 Documentation (Worker 4)

## Summary

Create two documentation guides for the star extension system:
1. `docs/guides/writing-extensions.md` - Extension author guide
2. `docs/guides/config-migration.md` - User migration guide

## Context

- **Branch:** `feature/ext-commands-phase-3` (current)
- **Plan reference:** `docs/plans/star-agent-team-refactor.md`, Phase 6
- **Role:** Worker 4 - Extensions and Commands

## Deliverables

### 1. docs/guides/writing-extensions.md

Step-by-step guide for extension authors covering:

1. **Introduction** - What extensions are and the three types (binding-only, command-only, full)
2. **Quick Start** - Minimal extension in 5 minutes
3. **Extension YAML Specification**
   - Required fields (`extension`, `description`)
   - Optional sections (`receivers`, `command`, `flags`, `config`)
   - Naming conventions (extension name → command path → config path → env vars)
4. **Starlark Command Implementation**
   - The `run(ctx)` entry point
   - Accessing flags via `ctx.args.get()`
   - Loading config via `config.get()`
   - Calling binding functions (receivers)
   - Helper functions (`success()`, `fail()`, `note()`, `warn()`, `error()`)
5. **Configuration Schema**
   - Defining fields and types
   - Setting defaults
   - Complex types (maps, lists)
6. **Built-in Receivers** - Available built-ins (`config`, `fs`, `lint`, `copyright`, etc.)
7. **Testing Extensions** - How to test before publishing
8. **Examples** - Reference to existing extensions in `extensions/`

**Source material:**
- `docs/architecture/devlore-extension-model.md` (architecture concepts)
- `extensions/lint-copyright/extension.yaml` (full extension example)
- `extensions/lint-copyright/lint-copyright.star` (Starlark implementation)
- `extensions/lint-go/extension.yaml` (command-only example)

### 2. docs/guides/config-migration.md

Migration guide for users covering:

1. **Overview** - Extension-based configuration model
2. **Configuration Hierarchy**
   - Extension defaults (from extension.yaml)
   - User configuration (star.yaml)
   - Environment variable overrides (`STAR_<EXTENSION>_<FLAG>`)
   - CLI flag overrides
3. **star.yaml Format**
   - Structure mirrors extension names (e.g., `lint.copyright.enabled`)
   - Type coercion from YAML values
4. **Environment Variables**
   - Naming convention: `STAR_LINT_COPYRIGHT_FIX=true`
   - Override precedence
5. **Per-Project vs Per-User Configuration**
   - Project: `./star.yaml`
   - User: `~/.config/star/star.yaml`
6. **Troubleshooting**
   - Common issues and solutions
   - `star config show` to debug resolved values

**Source material:**
- `star.yaml` (example config)
- `docs/architecture/devlore-extension-model.md` (Flag Resolution section)
- Extension YAML specs (defaults)

## Implementation Steps

1. Create `docs/guides/` directory
2. Write `docs/guides/writing-extensions.md`
3. Write `docs/guides/config-migration.md`
4. Verify build passes: `go build ./...`
5. Show commit command for approval

## Files to Create

| File | Purpose |
|------|---------|
| `docs/guides/writing-extensions.md` | Extension author guide |
| `docs/guides/config-migration.md` | Config migration guide |

## Reference Files (read-only)

| File | Use |
|------|-----|
| `docs/architecture/devlore-extension-model.md` | Architecture concepts |
| `extensions/lint-copyright/extension.yaml` | Full extension example |
| `extensions/lint-copyright/lint-copyright.star` | Starlark example |
| `extensions/lint-go/extension.yaml` | Command-only example |
| `star.yaml` | Config example |
