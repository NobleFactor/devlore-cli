---
title: "Rust Migration: Full Port of devlore-cli from Go to Idiomatic Rust"
status: draft
created: 2026-03-04
updated: 2026-03-04
---

# Plan: Rust Migration

## Summary

Port devlore-cli from Go to idiomatic Rust. The Go codebase (~34,500 LOC
source, ~30,200 LOC tests, 20 providers, 100+ actions) moves to a Rust
workspace using proc macros for provider bindings instead of runtime
reflection. The `star` codegen tool gains tree-sitter-based Rust source
parsing and emits annotations (~20 lines per provider) that Rust's compiler
expands into the equivalent of today's `.gen.go` files. The Starlark runtime
moves from `go.starlark.net` to `starlark-rust` (Meta's pure-Rust
implementation). No Go reflection code survives.

## Goals

1. **Eliminate runtime reflection**: The 1,400 lines of Go reflection code
   (`action_reflect.go`, `receiver_reflect.go`, `planned_reflect.go`,
   `starvalue_marshal.go`) become ~930 lines of compile-time infrastructure
   (proc macro crate + runtime trait library). Type errors surface at compile
   time, not at runtime.

2. **Simplify codegen**: `star` output per provider drops from ~90 lines of
   `.gen.go` files to ~20 lines of Rust annotations. The Rust compiler does
   the code expansion. No generated files to check in.

3. **Preserve architecture**: The provider/action/resource/graph model,
   the compensation (saga) pattern, the planned/immediate mode split, and
   the Starlark orchestration layer all carry over unchanged. This is a
   language port, not a redesign.

4. **Single-binary distribution**: Rust produces statically-linked binaries
   with no runtime dependencies. Cross-compilation to all six targets
   (darwin/linux/windows × amd64/arm64) becomes simpler.

5. **Performance**: Zero-cost abstractions replace reflection overhead.
   Trait-based dispatch is resolved at compile time. Starlark execution
   benefits from `starlark-rust`'s optimized interpreter.

6. **Concrete provider types**: The Go type aliases `Result = any` and
   `UndoState = any` erase provider-specific return types. A provider
   developer writes `Copy(...) (Resource, Tombstone, error)` — the return
   types are concrete, chosen by the provider. The reflection bridge throws
   this away via `reflect.Value.Interface().(any)`. The Rust port restores
   type safety: the proc macro reads the method signature, sees the concrete
   types, and generates code that preserves them. Type erasure happens only
   at the `ActionRegistry` boundary (heterogeneous storage), never inside
   the action implementation. `Result = any` and `UndoState = any` do not
   exist in the Rust codebase.

## Constraints

### No Parallel Maintenance

The Go and Rust codebases do not coexist in production. The port happens on
a long-lived branch. When the Rust port passes the full test suite and
matches Go feature parity, the Go code is retired. There is no period where
both languages ship.

### Starlark Script Compatibility

User-facing `.star` scripts must work identically on the Rust port. The
Starlark language surface — provider names, method names, parameter names,
return shapes — does not change. The graph JSON/YAML format does not change.
Existing graphs produced by the Go version must deserialize and execute on
the Rust version.

### `star` Tool Stays in Go

The `star` codegen tool in `noblefactor-ops` remains a Go program. It gains
tree-sitter-based Rust source parsing (`go-tree-sitter` with the Rust
grammar) to read provider method signatures from `.rs` files. It emits
Rust annotations instead of (or in addition to) Go `.gen.go` files.

### Test Parity

Every Go test has a Rust equivalent. The test macro crate generates the same
categories of tests that `star` generates today: `_Do`, `_DryRun`, `_Undo`,
`_UndoNil` for each action. Integration tests using `.star` scripts run
identically.

## Current State

| Component | Go Status | Notes |
| --- | --- | --- |
| Core types (`Resource`, `Action`, `Provider`, `Graph`, `Context`) | Implemented | `pkg/op/*.go` — sealed interfaces, embedding pattern |
| Reflection bridge | Implemented | `action_reflect.go` (301 LOC), `receiver_reflect.go` (228 LOC), `planned_reflect.go` (210 LOC) |
| Marshal/unmarshal | Implemented | `starvalue_marshal.go` (659 LOC) — Go↔Starlark via `reflect` |
| File provider | Implemented | `pkg/op/provider/file/` — 7 source files, full compensation |
| Other providers (19) | Implemented | `pkg/op/provider/*/` — archive, encryption, git, json, yaml, net, pkg, platform, regexp, service, shell, template, ui, star* (6) |
| Codegen (`star`) | Implemented | 58 `.gen.go` files across 20 providers |
| Test generation | Implemented | Generated action tests per provider |
| CLI (`lore`, `writ`) | Implemented | Cobra-based, Viper config, Bubbletea TUI |
| Execution engine | Implemented | `internal/execution/` — graph executor with flow control |
| LLM model providers | Implemented | `internal/model/` — Anthropic, OpenAI, Gemini, Groq, Ollama |
| Signing | Implemented | `internal/signing/` — GPG, AWS KMS, GCP KMS, Azure KV |
| Starlark extensions | Implemented | `star/extensions/` — 5 namespaces |

## Requirements

### Rust Workspace Structure

The Rust project is a Cargo workspace mirroring the Go package layout:

```
devlore-rs/
├── Cargo.toml                    # workspace root
├── crates/
│   ├── devlore-macros/           # proc macro crate (provider + resource + test macros)
│   │   ├── Cargo.toml
│   │   └── src/lib.rs
│   ├── devlore-runtime/          # runtime trait library (coerce, marshal, shadow)
│   │   ├── Cargo.toml
│   │   └── src/
│   │       ├── lib.rs
│   │       ├── action.rs         # Action, CompensableAction traits
│   │       ├── context.rs        # Context struct
│   │       ├── graph.rs          # Graph, Node, Edge, SlotValue
│   │       ├── marshal.rs        # ToStarlark, FromStarlark traits
│   │       ├── output.rs         # Output (promise), Gather
│   │       ├── provider.rs       # Provider trait, ProviderBase
│   │       ├── resource.rs       # Resource, Tombstone traits, ResourceBase
│   │       └── resource_catalog.rs
│   ├── devlore-providers/        # all 20 providers
│   │   ├── Cargo.toml
│   │   └── src/
│   │       ├── file/
│   │       │   ├── mod.rs
│   │       │   ├── provider.rs   # plain Rust methods (star adds annotations)
│   │       │   ├── resource.rs   # FileResource, FileTombstone
│   │       │   └── recovery.rs   # platform-specific recovery
│   │       ├── git/
│   │       ├── archive/
│   │       └── ...               # 17 more providers
│   ├── devlore-execution/        # graph executor
│   ├── devlore-starlark/         # Starlark integration layer
│   ├── devlore-signing/          # GPG, KMS signing backends
│   ├── devlore-model/            # LLM provider integrations
│   └── devlore-cli/              # CLI binaries (lore, writ)
│       ├── Cargo.toml
│       └── src/
│           ├── lore/
│           └── writ/
└── tests/                        # integration tests + .star scripts
```

### Proc Macro Crate (`devlore-macros`)

Three attribute macros:

- `#[devlore_provider(...)]` — applied to provider `impl` blocks. Reads
  `#[action(...)]` annotations on methods. Generates: Action/CompensableAction
  impls, immediate-mode Starlark binding functions, planned-mode graph node
  builders, `register_actions()`, `params()`, `new_receiver()`,
  `new_planned()`, `binding()`.

- `#[resource(...)]` — applied to resource structs. Generates: `Resource`
  trait impl, `StarlarkValue`/`HasAttrs` impls, `ToStarlark`/`FromStarlark`
  conversions.

- `#[devlore_provider_tests]` — applied to provider `impl` blocks (in test
  modules). Reads the same `#[action(...)]` annotations. Generates: `_do`,
  `_dry_run`, `_undo`, `_undo_nil` test functions per action.

### Runtime Trait Library (`devlore-runtime`)

Trait-based equivalents of Go's reflection functions:

| Rust Trait/Function | Replaces Go Function | Purpose |
| --- | --- | --- |
| `Coerce<T>` trait | `coerceSlotValue()` | SlotValue → concrete type |
| `shadow_result()` | `shadowResult()` | Register Resource in catalog |
| `FromStarlark` trait | `unmarshalValue()` | Starlark Value → Rust type |
| `ToStarlark` trait | `marshalReflect()` | Rust type → Starlark Value |
| `FillSlot()` | `FillSlot()` | Starlark Value → graph slot |
| `resolve_resource_param::<T>()` | `resolveResourceParam()` | Plan-time catalog resolution |

### `star` Tool Changes

`star` gains a Rust source backend:

1. **Parser**: `go-tree-sitter` with `tree-sitter-rust` grammar. Reads
   struct definitions and `impl` block method signatures from `.rs` files.
   Extracts: method name, parameter names/types, return type, existing
   annotations.

2. **Emitter**: Instead of generating `.gen.go` files, emits `#[action(...)]`
   and `#[devlore_provider(...)]` annotations in-place on the `.rs` source.
   Idempotent — re-running `star` updates annotations without duplicating.

3. **Dual mode**: During the transition, `star` supports both Go and Rust
   output. A flag (`--lang=rust`) selects the target.

### Dependency Mapping

| Go Dependency | Rust Crate | Notes |
| --- | --- | --- |
| `go.starlark.net` | `starlark` (starlark-rust) | Pure Rust Starlark interpreter |
| `spf13/cobra` | `clap` | CLI framework |
| `spf13/viper` | `config` or `figment` | Configuration |
| `charmbracelet/bubbletea` | `ratatui` + `crossterm` | TUI (different paradigm) |
| `charmbracelet/lipgloss` | `ratatui` styling | Integrated in ratatui |
| `charmbracelet/glamour` | `termimad` or `comrak` | Markdown rendering |
| `go-git/go-git` | `git2` (libgit2 bindings) | Git operations |
| `gopkg.in/yaml.v3` | `serde_yaml` | YAML via serde |
| `encoding/json` | `serde_json` | JSON via serde |
| `filippo.io/age` | `rage` | Age encryption |
| `getsops/sops` | CLI subprocess | No Rust equivalent; shell out to `sops` |
| `aws-sdk-go-v2` | `aws-sdk-rust` | AWS KMS |
| `Azure/azure-sdk-for-go` | `azure_core` + `azure_security_keyvault` | Azure Key Vault |
| `cloud.google.com/go/kms` | `google-cloud-kms` or REST via `reqwest` | GCP KMS |
| `Masterminds/semver` | `semver` | Semantic versioning |
| `google/uuid` | `uuid` | UUID generation |
| `golang.org/x/text` | `unicode-*` crates | Unicode handling |
| `fzipp/gocyclo` | `cognitive_complexity` or custom | Complexity checking |
| `google.golang.org/protobuf` | `prost` | Protocol Buffers |

**Gap**: `sops` has no Rust library. Use `sops` as a CLI subprocess (same
pattern as GPG signing today).

## Implementation Phases

### Phase 0: Workspace Bootstrap

Set up the Rust workspace, Cargo configuration, CI, and cross-compilation.

- [ ] Create Cargo workspace with crate stubs
- [ ] Configure CI (`cargo build`, `cargo test`, `cargo clippy`, `cargo fmt`)
- [ ] Configure cross-compilation targets (6 platforms)
- [ ] Add Makefile targets mirroring Go (`make build`, `make test`, `make check`)
- [ ] Set up `cargo-deny` for dependency auditing

**Crates created**: All crate directories with stub `lib.rs`/`main.rs`.

### Phase 1: Runtime Trait Library (`devlore-runtime`)

Port the core types and trait-based runtime functions. This is the foundation
everything else depends on.

- [ ] `Resource` trait + `ResourceBase` (sealed via private module)
- [ ] `Tombstone` trait + `TombstoneBase`
- [ ] `Action` trait + `CompensableAction` trait
- [ ] `Provider` trait + `ProviderBase`
- [ ] `Context` struct (embeds catalog, graph, writer, dry-run flag)
- [ ] `Graph`, `Node`, `Edge`, `SlotValue` structs with serde
- [ ] `ResourceCatalog` (Mutex-protected append-only ledger)
- [ ] `Output` (promise) + `Gather` as Starlark values
- [ ] `NoResult` sentinel
- [ ] `ActionRegistry`
- [ ] `ProviderBinding`, `BindingConfig`, `Access`, `Lifetime`
- [ ] `ToStarlark` trait + impls for primitives, Vec, HashMap, structs
- [ ] `FromStarlark` trait + impls for primitives, Vec, HashMap, structs
- [ ] `Coerce` trait for slot value → concrete type
- [ ] `shadow_result()` function
- [ ] `FillSlot()` function
- [ ] `resolve_resource_param::<T>()` function
- [ ] `camel_to_snake()` utility
- [ ] Unit tests for all traits and functions
- [ ] Verify graph JSON/YAML round-trip with Go-produced fixtures

**Validation**: Deserialize a graph JSON file produced by the Go version,
re-serialize, diff. Must be identical.

### Phase 2: Proc Macro Crate (`devlore-macros`)

Build the proc macro that replaces the reflection bridge and codegen tool
output.

- [ ] `#[devlore_provider(...)]` attribute macro — parse impl block, find
  `#[action]` methods, generate Action structs + trait impls
- [ ] Action code generation: `Do` with slot extraction, dry-run, method
  call, result shadowing
- [ ] CompensableAction code generation: `Undo` with state downcasting
- [ ] Immediate-mode code generation: Starlark arg unpacking, type
  conversion, method call, return marshaling
- [ ] Planned-mode code generation: Node creation, slot filling, resource
  param resolution, Output return
- [ ] `register_actions()`, `params()`, `new_receiver()`, `new_planned()`,
  `binding()` generation
- [ ] `#[resource(...)]` attribute macro — Resource trait impl, StarlarkValue
  impl, HasAttrs impl
- [ ] `#[devlore_provider_tests]` attribute macro — generate `_do`,
  `_dry_run`, `_undo`, `_undo_nil` tests per action
- [ ] Compile-time error messages for malformed annotations
- [ ] Unit tests: macro expansion verified via `trybuild` or `macrotest`

**Validation**: Write a minimal test provider by hand (2 methods, 1
compensable), apply macros, verify generated code compiles and passes
tests.

### Phase 3: File Provider (Reference Implementation)

Port the file provider as the first real provider. This validates the
entire stack: runtime traits, proc macros, and Starlark integration.

- [ ] `FileResource` struct with `#[resource(scheme = "file")]`
- [ ] `FileTombstone` struct
- [ ] `FileProvider` struct with all methods:
  - Compensable: `copy`, `link`, `move_file`, `backup`, `remove`,
    `remove_all`, `unlink`, `walk_tree`, `write_bytes`, `write_text`
  - Immediate-only: `exists`, `glob`, `is_dir`, `is_file`, `join`,
    `mkdir`, `name`, `parent`, `read`
- [ ] `recovery.rs` with `move_to_recovery()`, `restore_from_recovery()`
- [ ] Platform-specific recovery: `#[cfg(unix)]` / `#[cfg(windows)]`
- [ ] `checksumFile()`, `checksumBytes()` helpers
- [ ] Apply `#[action(...)]` annotations (manually first, `star` later)
- [ ] Apply `#[devlore_provider_tests]` annotation
- [ ] Verify all generated tests pass
- [ ] Starlark integration test: `.star` script → planned mode → graph →
  execute → verify file system state → compensate → verify restoration

**Validation**: The file provider's test suite matches or exceeds the Go
version's 974-line `actions_test.go` + 277-line `integration_test.go`.

### Phase 4: `star` Rust Backend

Add Rust source parsing and annotation emission to the `star` tool.

- [ ] Add `go-tree-sitter` + `tree-sitter-rust` dependencies
- [ ] Implement Rust method signature parser (struct fields, impl methods,
  return types)
- [ ] Implement annotation emitter (idempotent in-place insertion of
  `#[action(...)]`, `#[devlore_provider(...)]`, `#[resource(...)]`)
- [ ] `--lang=rust` flag for `star devlore actions generate`
- [ ] Test: run `star` on file provider `.rs` source, verify annotations
  match hand-written ones from Phase 3
- [ ] Test: round-trip — strip annotations, re-run `star`, verify identical

**Validation**: `star --lang=rust` on the file provider produces the same
annotations as the hand-written Phase 3 version.

### Phase 5: Remaining Providers (19)

Port each provider. Order by dependency and complexity:

**Tier 1 — No external dependencies, simple methods:**
- [ ] `json` provider (serde_json)
- [ ] `yaml` provider (serde_yaml)
- [ ] `regexp` provider (regex crate)
- [ ] `template` provider (tera or handlebars)
- [ ] `archive` provider (zip, tar, flate2 crates)

**Tier 2 — Platform or system interaction:**
- [ ] `shell` provider (std::process)
- [ ] `platform` provider (#[cfg] for linux/darwin/windows)
- [ ] `net` provider (reqwest)
- [ ] `pkg` provider (package manager interaction)
- [ ] `service` provider (systemd/launchd)

**Tier 3 — Git and encryption:**
- [ ] `git` provider (git2)
- [ ] `encryption` provider (aes-gcm, chacha20poly1305)

**Tier 4 — Starlark analysis providers (6):**
- [ ] `starcode`
- [ ] `staranalysis`
- [ ] `starcomplexity`
- [ ] `starindex`
- [ ] `starsources`
- [ ] `starstats`

**Tier 5 — UI:**
- [ ] `ui` provider (ratatui integration)

Each provider: write plain Rust methods → run `star --lang=rust` →
`cargo test` → verify.

### Phase 6: Execution Engine

Port the graph executor and flow control.

- [ ] Graph executor with node state machine (pending → executed/failed)
- [ ] Slot resolution (immediate values, promise delivery, gather fan-in)
- [ ] Compensation: LIFO undo on failure
- [ ] Flow control: `gather` (parallel iteration with concurrency limit)
- [ ] Retry policy support
- [ ] Dry-run mode
- [ ] Integration tests: multi-node graphs with dependencies, failure +
  compensation, parallel execution

### Phase 7: Internal Packages

Port supporting packages.

- [ ] `signing` — GPG (subprocess), AWS KMS, GCP KMS, Azure KV
- [ ] `credentials` — credential handling
- [ ] `config` — configuration management (figment or config crate)
- [ ] `manifest` — package manifest YAML/JSON loading
- [ ] `lorepackage` — package lifecycle
- [ ] `model` — LLM provider integrations (Anthropic, OpenAI, Gemini,
  Groq, Ollama) via reqwest + serde
- [ ] `console` — terminal UI components (ratatui)
- [ ] `starlark` integration layer — Starlark thread management, extension
  loading, script execution
- [ ] `e2e` — end-to-end test framework

### Phase 8: CLI Binaries

Port the command-line applications.

- [ ] `lore` binary — clap commands mirroring Cobra structure
- [ ] `writ` binary — clap commands + subcommands (identity, migrate,
  reconcile, secrets, segment, tree)
- [ ] Shared CLI infrastructure — version, man page, self-install, output
  formatting, XDG paths
- [ ] Shell completions — bash, zsh, fish (clap generates these)
- [ ] PowerShell integration (`pwsh` subprocess)
- [ ] `docgen` tool
- [ ] `indexgen` tool

### Phase 9: Starlark Extensions

Port the Starlark extension namespaces.

- [ ] Port all 5 extension namespaces from `star/extensions/`
- [ ] Verify `.star` script compatibility with existing scripts
- [ ] Port embedded assets (schemas, defaults)

### Phase 10: Validation and Cutover

Final validation before retiring Go.

- [ ] Full test suite passes (`cargo test`)
- [ ] Clippy clean (`cargo clippy -- -D warnings`)
- [ ] Cross-platform builds for all 6 targets
- [ ] Graph format compatibility: Go-produced graphs execute on Rust
- [ ] `.star` script compatibility: all existing scripts produce identical
  graphs
- [ ] Performance comparison: build time, binary size, execution speed
- [ ] `make dist-all` produces release binaries
- [ ] Update CI to build Rust instead of Go
- [ ] Archive Go source (tag, don't delete)

## Migration Path

There is no migration for end users. `.star` scripts are the user-facing
API. They don't change. Graphs are the serialization format. They don't
change. The binary names (`lore`, `writ`) don't change. The only visible
change is binary size and startup time.

For `star` contributors: the tool gains `--lang=rust` but continues to
support `--lang=go` (default) until the Go code is archived.

## Risk Assessment

| Risk | Impact | Mitigation |
| --- | --- | --- |
| `starlark-rust` API gaps | High | Evaluate early in Phase 1; file issues or fork if needed |
| Proc macro debugging difficulty | Medium | Use `cargo expand` to inspect; `trybuild` for regression tests |
| `sops` has no Rust library | Low | Shell out to `sops` binary (same as GPG pattern) |
| `git2` (libgit2) differs from `go-git` | Medium | Test against same repositories; accept minor behavior differences |
| Cloud KMS SDKs less mature in Rust | Medium | Use REST APIs via `reqwest` as fallback |
| TUI paradigm difference (Elm vs immediate) | Medium | Accept different implementation; match UX, not code structure |
| Long-lived branch diverges from Go development | High | Freeze Go feature development during port; only bug fixes on Go |

## Open Questions

- [ ] Should the Rust workspace live in the same repo or a new repo?
- [ ] Should the proc macro crate be published to crates.io or kept private?
- [ ] What is the minimum supported Rust version (MSRV)?
- [ ] Should we evaluate `starlark-rust` API surface before committing to Phase 1?
- [ ] Should Go feature development freeze during the port, or continue in parallel?

## Related Documents

- [Resource Management Plan](./resource-management.md) — current Go architecture
- [Reflection Marshaler Plan](./reflection-marshaler.md) — Go reflection bridge design
- [Star Gen Receiver Plan](./star-gen-receiver.md) — codegen tool design
- Issue #65 — Error tracking
