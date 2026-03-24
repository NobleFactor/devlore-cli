---
title: "Move star to devlore-cli"
issue: TBD
status: draft
created: 2026-03-22
updated: 2026-03-23
---

# Plan: Move star to devlore-cli

## Summary

Move the `star` binary, its runtime, providers, extensions, and supporting
packages from noblefactor-ops to devlore-cli. Star is platform tooling that
all NobleFactor projects consume — it belongs with the framework, not with a
single project. This eliminates the cross-repo dependency coupling and the
go.mod pin problem.

## Goals

1. **Star lives in devlore-cli** — `cmd/star`, runtime, providers, extensions
2. **noblefactor-ops becomes a consumer** — carries only project-specific config (`star.yaml`) and any project-specific extensions
3. **Star-specific providers move out of `pkg/op/provider/`** — staranalysis, starcode, starcomplexity, starindex, starstats move to star's provider tree
4. **Clean framework boundary** — `pkg/op` is the framework, star owns its domain logic

## Current State

| Component | noblefactor-ops | devlore-cli |
| --- | --- | --- |
| `cmd/star/main.go` | source | `cmd/star/main.go` |
| `internal/starlark/` | source | `cmd/star/star/` (runtime + command types) |
| `internal/extension/` | source | `cmd/star/extension/` |
| `internal/config/` | source | `cmd/star/config/` |
| `internal/cli/` | source | `cmd/star/cli/` |
| `internal/wasm/` | source | TBD — not yet moved |
| `internal/provider/commands/` | source | `cmd/star/provider/commands/` |
| `internal/provider/config/` | source | `cmd/star/provider/config/` |
| `internal/provider/goast/` | source | `cmd/star/provider/goast/` |
| `internal/provider/lint/` | source | `cmd/star/provider/lint/` |
| `internal/provider/setup/` | source | `cmd/star/provider/setup/` |
| `internal/provider/shellcheck/` | source | `cmd/star/provider/shellcheck/` |
| `star/extensions/` (16 extensions) | source | `cmd/star/extensions/` (embedded via `//go:embed`) |
| `pkg/op/provider/staranalysis/` | — | `pkg/op/provider/staranalysis/` (needs move to `cmd/star/`) |
| `pkg/op/provider/starcode/` | — | `pkg/op/provider/starcode/` (needs move to `cmd/star/`) |
| `pkg/op/provider/starcomplexity/` | — | `pkg/op/provider/starcomplexity/` (needs move to `cmd/star/`) |
| `pkg/op/provider/starindex/` | — | `pkg/op/provider/starindex/` (needs move to `cmd/star/`) |
| `pkg/op/provider/starstats/` | — | `pkg/op/provider/starstats/` (needs move to `cmd/star/`) |

## Requirements

### Target structure in devlore-cli

```
devlore-cli/
├── cmd/
│   ├── lore/                        # existing
│   ├── star/                        # star application — all star-specific code lives here
│   │   ├── main.go                  # cobra root command, flag wiring, extension loading
│   │   ├── extensions.go            # //go:embed extensions (bundled into binary)
│   │   ├── cli/                     # CLI helpers (output, self-install)
│   │   ├── config/                  # star config system (unified, accessor, sync, starlark bindings)
│   │   ├── extension/               # extension discovery, spec parsing, registry
│   │   ├── extensions/              # bundled star extensions (16 com.noblefactor.star.*)
│   │   │   ├── com.noblefactor.star.ConfigShow/
│   │   │   ├── com.noblefactor.star.ConfigSync/
│   │   │   ├── com.noblefactor.star.HookPreCommit/
│   │   │   ├── com.noblefactor.star.HookPrePush/
│   │   │   ├── com.noblefactor.star.LintAll/
│   │   │   ├── com.noblefactor.star.LintCopyright/
│   │   │   ├── com.noblefactor.star.LintGo/
│   │   │   ├── com.noblefactor.star.LintGoStyle/
│   │   │   ├── com.noblefactor.star.LintMarkdown/
│   │   │   ├── com.noblefactor.star.LintShell/
│   │   │   ├── com.noblefactor.star.LintTools/
│   │   │   ├── com.noblefactor.star.Setup/
│   │   │   ├── com.noblefactor.star.SetupCheck/
│   │   │   ├── com.noblefactor.star.SetupConfig/
│   │   │   ├── com.noblefactor.star.SetupHooks/
│   │   │   └── com.noblefactor.star.SetupTools/
│   │   ├── inventory/               # generated imports for star-specific providers
│   │   │   └── inventory.gen.go
│   │   ├── provider/                # star-specific providers
│   │   │   ├── commands/            # command tree provider
│   │   │   ├── config/              # config provider
│   │   │   ├── goast/               # Go AST provider (with doctaxonomy/)
│   │   │   ├── lint/                # lint provider
│   │   │   ├── setup/               # setup provider
│   │   │   ├── shellcheck/          # shellcheck provider
│   │   │   ├── staranalysis/        # (to move from pkg/op/provider/)
│   │   │   ├── starcode/            # (to move from pkg/op/provider/)
│   │   │   ├── starcomplexity/      # (to move from pkg/op/provider/)
│   │   │   ├── starindex/           # (to move from pkg/op/provider/)
│   │   │   └── starstats/           # (to move from pkg/op/provider/)
│   │   └── star/                    # runtime (Application, Command, starlark execution)
│   └── writ/                        # existing
├── pkg/op/                          # framework (unchanged)
│   ├── inventory/                   # generated imports for framework providers
│   │   └── inventory.gen.go
│   └── provider/                    # framework providers only
│       ├── appnet/
│       ├── archive/
│       ├── encryption/
│       ├── file/
│       ├── git/
│       ├── json/
│       ├── mem/
│       ├── pkg/
│       ├── platform/
│       ├── regexp/
│       ├── service/
│       ├── shell/
│       ├── template/
│       ├── ui/
│       └── yaml/
├── star/
│   └── extensions/                  # devlore project-local extensions (filesystem, not embedded)
│       ├── com.noblefactor.devlore.Actions/
│       ├── com.noblefactor.devlore.Knowledge/
│       ├── com.noblefactor.devlore.Model/
│       ├── com.noblefactor.devlore.Package/
│       └── com.noblefactor.devlore.Test/
└── tools/
    └── New-OpInventory/             # code generator for inventory.gen.go files
        └── main.go
```

### What stays in noblefactor-ops

After the move, noblefactor-ops has no Go code. It becomes a config/scripts repo.

```
noblefactor-ops/
├── Makefile                        # shell/script targets only
├── scripts/                        # operational scripts
├── star.yaml                       # project config for star
└── star/
    └── extensions/                  # project-specific extensions only (if any)
```

### Inventory and code generation

The old `pkg/op/provider/register.go` (hand-maintained import list) is replaced by
two generated inventory files:

- `pkg/op/inventory/inventory.gen.go` — framework provider imports (15 providers)
- `cmd/star/inventory/inventory.gen.go` — star-specific provider imports (6 providers, more after star* move)

Both are generated by `tools/New-OpInventory/main.go`. The Makefile `generate` target
runs the generator for both inventories.

### Extension loading

Extensions load in two phases:

1. **Embedded** — `//go:embed extensions` in `cmd/star/extensions.go` compiles the 16
   bundled extensions into the binary. Loaded via `runtime.LoadEmbeddedExtensions()`.
2. **Filesystem** — `DefaultSearchPaths()` returns:
   - `${GIT_WORKSPACE_ROOT}/star/extensions/` (project-local)
   - `${XDG_DATA_HOME}/star/extensions/` (user, defaults to `~/.local/share`)
   - `/usr/local/share/star/extensions/` (system-wide)

**Known issue**: The current code loads extensions eagerly as discovered and silently
discards registry duplicate errors while still loading commands into the command map.
This needs to be fixed: collect all discovered extensions across all sources, resolve
which extension wins based on search-path priority, then load only the winners.

## Implementation Phases

### Phase 1: Move star internals to devlore-cli

Move all packages from noblefactor-ops `internal/` and `cmd/star/` to devlore-cli
`cmd/star/`. Rewrite import paths.

- [x] Create `cmd/star/` directory structure in devlore-cli
- [x] Copy noblefactor-ops packages to devlore-cli `cmd/star/` sub-packages
- [x] Rewrite all import paths from `noblefactor-ops/internal/...` to `devlore-cli/cmd/star/...`
- [x] Move `cmd/star/main.go` to devlore-cli
- [x] Update main.go imports
- [x] Build and fix compilation errors

**Files**: All of `internal/` and `cmd/star/` from noblefactor-ops

### Phase 2: Move star-specific providers from pkg/op/provider

Move staranalysis, starcode, starcomplexity, starindex, starstats from
`pkg/op/provider/` to `cmd/star/provider/`.

- [ ] `git mv` the 5 star* provider directories
- [ ] Rewrite import paths
- [ ] Update `cmd/star/inventory/inventory.gen.go` to include moved providers
- [ ] Update `pkg/op/inventory/inventory.gen.go` to exclude moved providers
- [ ] Move e2e test scripts for star* providers to star's test tree
- [ ] Build and fix

**Files**: 5 provider directories + both inventory files + e2e tests

### Phase 3: Move extensions

Move all 16 `com.noblefactor.star.*` extensions from noblefactor-ops to devlore-cli.
Extensions are now embedded into the binary via `//go:embed`.

- [x] Copy noblefactor-ops `star/extensions/com.noblefactor.star.*/` to `cmd/star/extensions/`
- [x] Add `//go:embed extensions` in `cmd/star/extensions.go`
- [x] Verify all extensions load and commands register
- [ ] Fix extension loading to respect search-path priority before loading

**Files**: 16 extension directories (YAML + .star files) + `extensions.go`

### Phase 4: Move Makefile targets and CI

- [ ] Move code generation targets from noblefactor-ops Makefile to devlore-cli
- [ ] Update devlore-cli CI to build star
- [ ] Remove star-related targets from noblefactor-ops Makefile
- [ ] Update noblefactor-ops CI (remove star build/test)
- [ ] Remove noblefactor-ops go.mod (no Go code remains)
- [ ] Build and test both repos

### Phase 5: Clean up noblefactor-ops

- [ ] Remove `cmd/`, `internal/`, `go.mod`, `go.sum`
- [ ] Keep `star.yaml` and `star/extensions/` for project-specific extensions
- [ ] Update README to reflect star has moved
- [ ] Verify noblefactor-ops works as a pure config/scripts repo

## Migration Path

After this change:

- `star` is built from devlore-cli: `cd devlore-cli && make star`
- Bundled extensions are embedded in the binary — no separate install step
- Projects install star via `star self install` (copies binary, man pages, completions)
- Project-specific config lives in each repo's `star.yaml`
- Project-specific extensions live in each repo's `star/extensions/` (filesystem search path)
- noblefactor-ops has no Go code and no dependency on devlore-cli at the module level

## Resolved Questions

1. **Namespace**: `cmd/star/` — star is a command, its code lives under its command directory.
2. **Extension search path**: Embedded extensions (compiled into binary) are the fallback.
   Filesystem extensions are discovered via `DefaultSearchPaths()` (project-local, user, system).
   Load order needs fixing — must deduplicate by priority before loading.
3. **noblefactor-ops scope**: No Go code remains. Becomes a pure config/scripts repo.
4. **Consumer model**: Projects consume star via `star self install` which copies the binary
   to `~/.local/bin/` (or specified prefix). `self upgrade` is tracked in devlore-cli#83.

## Related Documents

- [Star Application Restructure](https://github.com/NobleFactor/noblefactor-ops/blob/develop/docs/plans/star-application-restructure.md)
- [CLI Syntax Cleanup](https://github.com/NobleFactor/noblefactor-ops/blob/develop/docs/plans/star-cli-syntax-cleanup.md)
- devlore-cli#83 — Self install/upgrade for lore, writ, and star
- noblefactor-ops#39 — Closed, superseded by devlore-cli#83
- noblefactor-ops#121 — CLI syntax cleanup (prerequisite, just merged)
- noblefactor-ops#122 — Uniform file discovery and batched tool invocation (follow-up)
