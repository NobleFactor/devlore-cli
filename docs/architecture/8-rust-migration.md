# Rust Migration: Architecture and Design Decisions

This document captures the full design context for porting devlore-cli from
Go to idiomatic Rust. It records the reasoning behind every architectural
decision so that any future session can pick up the work with complete
understanding.

See also: [Rust Migration Plan](../plans/8-rust-migration.md) — phased
implementation plan with tasks, timelines, and file listings.

## 1. Why Rust

The Go codebase works. The architecture is proven. The motivation for Rust
is not dissatisfaction with Go's runtime or performance, but with specific
language limitations that force design compromises in this codebase:

### 1.1 The Type Erasure Problem

Go's `action.go` defines:

```go
type Result = any
type Complement = any
```

These type aliases are **a bug, not a design choice**. Provider developers
write methods with concrete return types:

```go
func (p *Provider) Copy(source Resource, dest string, mode os.FileMode) (Resource, Tombstone, error)
func (p *Provider) Exists(resource Resource) (bool, error)
```

The return types (`Resource`, `Tombstone`, `bool`) are concrete and
provider-specific. But the reflection bridge (`action_reflect.go`) calls
`reflect.Method.Func.Call()`, which returns `[]reflect.Value`. Calling
`.Interface()` on those values produces `any`. The type information that
the method signature declares is thrown away at the dispatch boundary.

The `ActionRegistry` compounds this: it stores heterogeneous actions in a
single `map[string]Action`. Go's generics cannot express "any Action
regardless of type parameters" — there is no wildcard generic (`Action[?, ?]`)
and no way to store `Action[FileResource, FileTombstone]` alongside
`Action[bool, NoResult]` in the same map without erasing to `any`.

This is a fundamental limitation of Go's type system. Pointers don't help.
Generics don't help. The only escape hatch is `any`, and the only place
to recover the concrete type is via runtime type assertions that can fail.

### 1.2 The Reflection Tax

The reflection bridge spans three files (~740 LOC):

- `action_reflect.go` (301 LOC) — runtime method dispatch for graph execution
- `receiver_reflect.go` (228 LOC) — immediate-mode Starlark binding
- `planned_reflect.go` (210 LOC) — planned-mode graph node building

Plus `starvalue_marshal.go` (659 LOC) for Go↔Starlark marshaling via
`reflect`. Total: ~1,400 lines of runtime reflection code.

This code exists because Go cannot generate dispatch code at compile time.
Every method call goes through `reflect.Method.Func.Call()`. Every parameter
is coerced via `reflect.ValueOf()`. Every return value is classified by
inspecting `reflect.Type` at runtime.

### 1.3 What Rust Fixes

Rust's proc macros run at compile time and see the full method signature AST.
A macro applied to a provider impl block reads the concrete parameter types
and return types, then generates fully-typed dispatch code. The generated
code calls provider methods directly — no reflection, no type erasure within
the action implementation.

Type erasure still happens at the `ActionRegistry` boundary (heterogeneous
storage requires `Box<dyn Any>`), but the generated `Do` and `Undo`
implementations work with concrete types internally. The downcast from
`Box<dyn Any>` to the concrete type happens exactly once, in generated code
that knows the target type at compile time.

## 2. The Annotation Architecture

### 2.1 Three Approaches Explored

During the design conversation, three approaches were evaluated:

**Approach A: Fat codegen (rejected)**

`star` generates a full `bindings.rs` per provider containing concrete
Action structs, immediate-mode functions, and planned-mode functions — all
fully expanded with slot extraction, method calls, and return handling.

- Output: ~700 lines per provider, ~14,000 lines across 20 providers
- Problem: massive generated code volume, duplicates logic across providers
- No shared machinery — every binding is self-contained

**Approach B: Annotations via `#[starlark_module]` (rejected)**

Annotate hand-written provider methods with `starlark-rust`'s own macros.
Mix provider logic with Starlark binding concerns in the same file.

- Problem: `#[starlark_module]` expects standalone functions, not annotations
  on existing struct methods
- Problem: conflates provider implementation with binding generation
- Problem: doesn't support the three-mode split (action/immediate/planned)

**Approach C: `star` emits annotations, proc macros expand them (chosen)**

`star` reads plain Rust provider source, emits minimal annotations
(`#[devlore_provider(...)]`, `#[action(...)]`, `#[resource(...)]`). The Rust
compiler's proc macro expansion generates all binding code.

- Output: ~20 lines of annotations per provider
- Shared machinery: ~930 lines (proc macro crate + runtime trait library)
- `star` stays thin — it maps method signatures to annotation syntax
- The compiler does the code expansion

### 2.2 How It Works

**Step 1: Write plain Rust**

Provider developers write standard Rust structs and methods with no Starlark
awareness:

```rust
pub struct FileProvider {
    pub root: FileResource,
}

impl FileProvider {
    pub fn copy(&self, source: &FileResource, destination: &str, mode: Option<u32>)
        -> anyhow::Result<(FileResource, FileTombstone)> {
        // ... implementation
    }

    pub fn compensate_copy(&self, tombstone: FileTombstone) -> anyhow::Result<()> {
        restore_from_recovery(&tombstone)
    }

    pub fn exists(&self, resource: &FileResource) -> anyhow::Result<bool> {
        Ok(resource.exists())
    }
}
```

**Step 2: Run `star` — it adds annotations only**

`star` reads the method signatures via tree-sitter and emits annotations:

```rust
#[devlore_provider(name = "file", access = "both", lifetime = "stateless")]
impl FileProvider {
    #[action(compensate = "compensate_copy", params(
        source = "source_file",
        destination = "destination_filename",
        mode = "destination_file_mode?",
    ))]
    pub fn copy(&self, source: &FileResource, destination: &str, mode: Option<u32>)
        -> anyhow::Result<(FileResource, FileTombstone)> {
        // ... unchanged
    }

    pub fn compensate_copy(&self, tombstone: FileTombstone) -> anyhow::Result<()> {
        restore_from_recovery(&tombstone)
    }

    #[action(params(resource = "resource"))]
    pub fn exists(&self, resource: &FileResource) -> anyhow::Result<bool> {
        Ok(resource.exists())
    }
}
```

What `star` added: 1 `#[devlore_provider]` line, 2 `#[action]` blocks,
1 import. About 15 lines total. The method bodies are untouched.

**Step 3: `cargo build` — the compiler expands annotations**

The `#[devlore_provider]` proc macro reads the entire impl block, finds
`#[action]` annotations, and generates:

For each annotated method:
- An Action struct (e.g., `CopyAction`) with `Do` implementation
- If `compensate` is specified: a CompensableAction impl with `Undo`
- An immediate-mode Starlark binding function
- A planned-mode graph node builder function

For the provider as a whole:
- `register_actions()` — registers all Action structs in an ActionRegistry
- `params()` — the Starlark parameter name map
- `new_receiver()` — builds the immediate-mode Starlark object
- `new_planned()` — builds the planned-mode Starlark object
- `binding()` — ties all three modes together in a ProviderBinding

### 2.3 What the Proc Macro Replaces

| Go (today) | Rust (ported) | Where the logic lives |
| --- | --- | --- |
| `star devlore actions generate` → `actions.gen.go` | `#[devlore_provider]` expansion | proc macro crate |
| `star` → `planned.gen.go` | `#[devlore_provider]` expansion | proc macro crate |
| `star` → `immediate.gen.go` | `#[devlore_provider]` expansion | proc macro crate |
| `star` → `params.gen.go` | `#[devlore_provider]` expansion | proc macro crate |
| `action_reflect.go` (runtime dispatch) | Generated Action::Do impls | proc macro output |
| `receiver_reflect.go` (immediate bridge) | Generated Starlark functions | proc macro output |
| `planned_reflect.go` (planned bridge) | Generated node builders | proc macro output |
| `starvalue_marshal.go` (marshal/unmarshal) | `ToStarlark`/`FromStarlark` traits | runtime trait library |
| `//go:build unix` / `windows` | `#[cfg(unix)]` / `#[cfg(windows)]` | language feature |
| `reflect.ValueOf().Method().Call()` | Direct function calls | proc macro output |

### 2.4 Code Volume Comparison

| Component | Go | Rust |
| --- | --- | --- |
| `star` output per provider | ~90 lines (.gen.go files) | ~20 lines (annotations) |
| `star` output across 20 providers | ~1,800 lines | ~400 lines |
| Shared reflection / macro code | ~1,400 lines (runtime) | ~930 lines (compile-time) |
| Total infrastructure | ~3,200 lines | ~1,330 lines |

## 3. The `star` Tool

### 3.1 `star` Stays in Go

The `star` codegen tool in `noblefactor-ops` remains a Go program. It gains
Rust source parsing via `go-tree-sitter` with the `tree-sitter-rust` grammar.

Options evaluated for Go reading Rust source:

1. **tree-sitter-rust (chosen)** — proven parser, Go bindings exist
   (`github.com/smacker/go-tree-sitter`). Gives concrete syntax tree.
   Sufficient for extracting struct fields, method signatures, return types.

2. **syn via FFI** — wrap Rust's `syn` parser in a C-compatible shared
   library, call from Go via cgo. Full AST but complex build setup.

3. **rust-analyzer LSP** — query type information via LSP protocol. Overkill.

4. **Roll-your-own parser** — Rust method signatures are regular enough
   for a targeted parser. Fragile for edge cases.

tree-sitter is the pragmatic choice. `star` only needs method names,
parameter names/types, and return types. It doesn't need type resolution
or trait analysis.

### 3.2 Dual-Mode Operation

During the transition, `star` supports both Go and Rust output. A flag
(`--lang=rust`) selects the target:

- `--lang=go` (default): generates `.gen.go` files (current behavior)
- `--lang=rust`: emits annotations on `.rs` source files (new behavior)

### 3.3 Test Macro Generation

`star` also generates test annotations. A `#[devlore_provider_tests]` macro
reads the same `#[action]` annotations and emits test functions:

- `test_<method>_do` — call with valid slots, assert result type
- `test_<method>_dry_run` — assert no side effects, check output
- `test_<method>_undo` — do then undo, assert original state restored
- `test_<method>_undo_nil` — undo with None, assert no panic

This replaces the generated Go test files (e.g., the 974-line
`actions_test.go` for the file provider).

## 4. Type System Design

### 4.1 Action Trait Hierarchy

The Go codebase uses a single `Action` interface with `Result = any` and
`Complement = any`. This erases provider-specific types. The Rust port uses
two separate traits with associated types:

```rust
trait Action {
    type Result;
    fn name(&self) -> &str;
    fn do_action(&self, ctx: &Context, slots: &Slots)
        -> anyhow::Result<Self::Result>;
}

trait CompensableAction: Action {
    type Undo;
    fn undo(&self, ctx: &Context, state: Self::Undo) -> anyhow::Result<()>;
}
```

Key design rules:

- `NoResult` is a sentinel for the **result** position only. A provider
  method that produces no meaningful output uses `type Result = NoResult`.
- `NoResult` never appears in the undo position. If an action has nothing to
  undo, it implements `Action` only — it does not implement
  `CompensableAction`.
- The valid combinations are:

| Signature | Meaning |
| --- | --- |
| `Action<Result = bool>` | Returns a result, no undo |
| `Action<Result = NoResult>` | No meaningful result, no undo |
| `CompensableAction<Result = FileResource, Undo = FileTombstone>` | Result + undo state |
| `CompensableAction<Result = NoResult, Undo = FileTombstone>` | No meaningful result, but undoable |

- `Action<Result = bool, Undo = NoResult>` **does not exist**. It conflates
  two separate concepts.

### 4.2 ActionRegistry and Type Erasure

The `ActionRegistry` stores heterogeneous actions. This is the one place
where type erasure is unavoidable:

```rust
pub struct ActionRegistry {
    actions: HashMap<String, Box<dyn AnyAction>>,
}
```

`AnyAction` is an object-safe trait that erases the associated types. The
proc macro generates an `AnyAction` wrapper for each concrete action that
internally knows the concrete types and performs the downcast.

Type erasure happens at the registry boundary. Inside the generated `Do`
and `Undo` implementations, all types are concrete.

### 4.3 Resource Trait

Go seals the `Resource` interface via an unexported method
(`resourceBase()`). Rust achieves the same via the sealed trait pattern:

```rust
mod sealed {
    pub trait Sealed {}
}

pub trait Resource: sealed::Sealed {
    fn uri(&self) -> String;
    fn scheme(&self) -> &str;
    fn host(&self) -> &str;
    fn path(&self) -> &str;
    fn base(&self) -> &ResourceBase;
}
```

The `#[resource]` proc macro generates `impl sealed::Sealed for T` alongside
the `Resource` impl, ensuring only annotated types can satisfy the trait.

### 4.4 Embedding → Delegation

Go's struct embedding (e.g., `FileResource` embeds `ResourceBase`) promotes
methods automatically. Rust has no embedding. The `#[resource]` macro
generates explicit delegation methods that forward to the `base` field.

## 5. Go Features That Don't Map Cleanly

### 5.1 Painful in Rust

| Go Feature | Issue in Rust | Solution |
| --- | --- | --- |
| `any` as type-erased value | `Box<dyn Any>` with explicit downcast | Proc macro generates typed wrappers |
| Struct embedding with promotion | No equivalent | `#[resource]` macro generates delegation |
| Implicit interface satisfaction | Must write `impl Trait for Type` | Proc macros generate impls |
| `init()` for registration | No equivalent | `inventory` crate or explicit `register_all()` |
| Universal `nil` | Each nullable becomes `Option<T>` | More verbose but safer |
| Sealed interfaces (unexported methods) | Private module + supertrait | `sealed` trait pattern |
| Struct tags (`json`, `yaml`, `starlark`) | `#[serde]` handles json/yaml; `starlark` needs custom derive | `#[resource]` macro |

### 5.2 Better in Rust

| Go Feature | Rust Equivalent | Improvement |
| --- | --- | --- |
| `if err != nil { return }` | `?` operator | Less boilerplate |
| `//go:build unix` | `#[cfg(unix)]` | Better integrated |
| No sum types (interface + type assertion) | `enum` with variants | Exhaustive checking |
| Limited generics | Full generics with trait bounds | More expressive |
| Type switches | `match` on enums | Compile-time exhaustiveness |
| `reflect` for dispatch | Proc macros for codegen | Compile-time, zero-cost |

## 6. Dependency Mapping

| Go Dependency | Rust Crate | Maturity | Notes |
| --- | --- | --- | --- |
| `go.starlark.net` | `starlark` (starlark-rust, Meta) | Good | Pure Rust Starlark interpreter |
| `spf13/cobra` | `clap` | Excellent | CLI framework |
| `spf13/viper` | `config` or `figment` | Good | Configuration |
| `charmbracelet/bubbletea` | `ratatui` + `crossterm` | Good | Different paradigm (Elm → immediate) |
| `charmbracelet/lipgloss` | `ratatui` styling | Good | Integrated |
| `charmbracelet/glamour` | `termimad` or `comrak` | Good | Markdown rendering |
| `go-git/go-git` | `git2` (libgit2) | Excellent | Git operations |
| `gopkg.in/yaml.v3` | `serde_yaml` | Excellent | YAML via serde |
| `encoding/json` | `serde_json` | Excellent | JSON via serde |
| `filippo.io/age` | `rage` | Good | Age encryption |
| `getsops/sops` | CLI subprocess | Gap | No Rust library — shell out to `sops` binary |
| `aws-sdk-go-v2` | `aws-sdk-rust` | Maturing | AWS KMS |
| `Azure/azure-sdk-for-go` | `azure_core` | Maturing | Azure Key Vault |
| `cloud.google.com/go/kms` | `google-cloud-kms` or `reqwest` | Maturing | GCP KMS; REST fallback |
| `Masterminds/semver` | `semver` | Excellent | Semantic versioning |
| `google/uuid` | `uuid` | Excellent | UUID generation |
| `golang.org/x/text` | `unicode-*` crates | Excellent | Unicode handling |
| `google.golang.org/protobuf` | `prost` | Excellent | Protocol Buffers |

### 6.1 Risk: `starlark-rust`

The `starlark-rust` crate (maintained by Meta) is the biggest dependency
risk. It must support:

- Custom native types (`StarlarkValue` trait)
- Attribute access (`HasAttrs` trait)
- Function binding (`#[starlark_module]`)
- Thread-local state for context passing

If its API surface doesn't cover devlore's needs, the options are: file
upstream issues, fork, or implement a thin adapter layer. This should be
evaluated early in Phase 1 of the implementation plan.

## 7. Runtime Trait Library

The proc macro generates calls to runtime functions. These functions use
traits instead of reflection:

| Runtime Function | Replaces Go Function | Purpose |
| --- | --- | --- |
| `Coerce<T>` trait | `coerceSlotValue()` | SlotValue → concrete Rust type |
| `shadow_result()` | `shadowResult()` | Register Resource in catalog |
| `FromStarlark` trait | `unmarshalValue()` / `unmarshalToAny()` | Starlark Value → Rust type |
| `ToStarlark` trait | `marshalReflect()` | Rust type → Starlark Value |
| `FillSlot()` | `FillSlot()` | Starlark Value → graph slot |
| `resolve_resource_param::<T>()` | `resolveResourceParam()` | Plan-time catalog resolution |
| `new_uri()` | `NewURI()` | Build canonical URI |

The runtime library is ~400 lines. Combined with the proc macro crate
(~530 lines), the total shared infrastructure is ~930 lines — replacing
~1,400 lines of Go reflection code.

## 8. Proc Macro Crate

### 8.1 Structure

The `devlore-macros` crate exports three attribute macros:

- `#[devlore_provider(...)]` — applied to `impl` blocks. Reads `#[action]`
  annotations. Generates all binding code.
- `#[resource(...)]` — applied to structs. Generates `Resource` trait impl,
  `StarlarkValue` impl, `HasAttrs` impl, `ToStarlark`/`FromStarlark`.
- `#[devlore_provider_tests]` — applied to provider `impl` blocks in test
  modules. Generates test functions.

`#[action(...)]` is not a separate proc macro. It's a marker attribute
consumed by `#[devlore_provider]` during expansion. The Rust compiler never
sees it independently — the outer macro strips it before emitting the
processed impl block.

### 8.2 Dependencies

- `syn` — Rust source parsing (reading the impl block AST)
- `quote` — code generation (emitting Rust token streams)
- `proc-macro2` — token stream utilities

### 8.3 What the Macro Sees

For each `#[action]`-annotated method, the macro extracts from the AST:

- **Method name**: `copy` → action name `file.copy`
- **Parameters** (after `&self`): names, types, whether `Option<T>` (optional)
- **Starlark parameter mapping**: from the `params(...)` annotation
- **Return type**: determines action kind:
  - `Result<T>` → `Action` (non-compensable)
  - `Result<(T, U)>` → `CompensableAction` (T = result, U = undo state)
- **Compensate method**: from the `compensate = "..."` annotation

### 8.4 What the Macro Generates

For `copy` with signature `-> Result<(FileResource, FileTombstone)>` and
`compensate = "compensate_copy"`:

**Action struct + Do:**
- Extract slots by Starlark name, coerce to Rust types
- Handle dry-run mode (print slot values, return early)
- Call `provider.copy(source, destination, mode)` with concrete types
- Shadow the `FileResource` result in the catalog
- Return `(Box<FileResource>, Some(Box<FileTombstone>))` — type erasure
  happens here, at the registry boundary only

**CompensableAction Undo:**
- Downcast `Box<dyn Any>` to `FileTombstone` (the macro knows the type)
- Call `provider.compensate_copy(tombstone)`

**Immediate-mode function:**
- Unpack Starlark args by name
- Convert Starlark values to Rust via `FromStarlark` trait
- Call `provider.copy(...)` directly
- Shadow result in catalog if present
- Convert return to Starlark via `ToStarlark` trait

**Planned-mode function:**
- Unpack Starlark args (keep as Starlark Values, don't convert)
- Create a `Node` with action name `file.copy`
- Fill slots from Starlark values
- Resolve resource parameters in catalog at plan time
- Append node to graph
- Return `Output` (promise)

**Registration functions:**
- `register_actions()` — registers `CopyAction` etc. in an `ActionRegistry`
- `params()` — returns the Starlark parameter name map
- `new_receiver()` — builds immediate-mode Starlark object
- `new_planned()` — builds planned-mode Starlark object
- `binding()` — ties all three modes together

### 8.5 Equivalence to Go Templates

The proc macro crate is functionally equivalent to the `star` templates that
currently emit `.gen.go` files. Same logic, different output language,
different execution host:

| Aspect | Go (today) | Rust (ported) |
| --- | --- | --- |
| Template logic | `star` templates | proc macro crate |
| Template runner | `star` binary | `rustc` |
| Output | `.gen.go` files on disk | expanded code in memory |
| When | before build (`make generate`) | during build (`cargo build`) |

## 9. Codebase Metrics

| Metric | Value |
| --- | --- |
| Go source files | 232 |
| Go test files | 88 |
| Go source LOC | ~34,500 |
| Go test LOC | ~30,200 |
| Generated .gen.go files | 58 |
| Provider implementations | 20 |
| Action types | 100+ (mostly generated) |
| External dependencies | 29 direct, 156+ indirect |
| Core `pkg/op` LOC | ~9,600 |
| Reflection bridge LOC | ~1,400 |
| Development time | 50 calendar days, 30 active days, 181 PRs, 1 author |
| Development period | 2026-01-14 to 2026-03-04 |

## 10. Risk Assessment

| Risk | Impact | Mitigation |
| --- | --- | --- |
| `starlark-rust` API gaps | High | Evaluate early in Phase 1 |
| Proc macro debugging | Medium | `cargo expand` to inspect; `trybuild` for tests |
| `sops` has no Rust library | Low | Shell out to `sops` binary |
| `git2` (libgit2) behavior differences from `go-git` | Medium | Test against same repositories |
| Cloud KMS SDKs less mature in Rust | Medium | REST APIs via `reqwest` as fallback |
| TUI paradigm difference (Elm → immediate) | Medium | Match UX, not code structure |
| Long-lived branch diverges from Go | High | Freeze Go feature development during port |

## 11. Effort Estimate

### 11.1 Codebase Scope

| Component | Source LOC | Test LOC | Gen LOC |
| --- | --- | --- | --- |
| `pkg/op/` core (non-provider) | 3,716 | — | — |
| `pkg/op/provider/` (20 providers) | ~5,292 | ~9,000 | 1,150 |
| `internal/execution/` | 2,914 | 6,891 | — |
| `internal/writ/` | 6,676 | 3,154 | — |
| `internal/cli/` | 2,346 | 1,320 | — |
| `internal/lorepackage/` | 2,114 | 994 | — |
| `internal/lore/` | 1,781 | 882 | — |
| `internal/model/` | 1,245 | 97 | — |
| `internal/signing/` | 1,146 | 329 | — |
| Other internal packages | 4,791 | 3,159 | — |
| `cmd/` binaries | ~2,100 | — | — |
| **Total** | **33,404** | **32,791** | **1,150** |

### 11.2 Per-Phase Estimates (Active Working Days with Claude Code)

| Phase | Description | Source to Port | Key Challenge | Days |
| --- | --- | --- | --- | --- |
| **0** | Workspace bootstrap | Config only | CI, cross-compilation | 0.5–1 |
| **1** | Runtime trait library | 3,716 (minus ~1,400 reflection) | Foundation design; graph round-trip validation | 3–5 |
| **2** | Proc macro crate | ~530 new (dense `syn`/`quote`) | Hardest phase — proc macro dev is intricate, debugging is slow (`cargo expand`, `trybuild`) | 5–8 |
| **3** | File provider (reference) | 1,745 src + 3,094 test | First real validation; may expose Phase 2 bugs | 2–3 |
| **4** | `star` Rust backend | In noblefactor-ops | tree-sitter-rust in Go; annotation emitter | 2–3 |
| **5** | 19 remaining providers | ~3,547 src + ~5,906 test | Tier breadth; platform/service OS-specific code | 8–12 |
| **6** | Execution engine | 2,914 src + 6,891 test | Complex state machine; massive test suite | 4–6 |
| **7** | Internal packages | ~14,577 src + ~8,515 test | writ (6.7K LOC), TUI paradigm shift (bubbletea→ratatui), 5 LLM providers | 8–12 |
| **8** | CLI binaries | ~2,100 + cli shared (2,346) | clap command trees, man pages, self-install | 3–5 |
| **9** | Starlark extensions | Embedded in provider/starlark layers | starlark-rust API surface fit | 1–2 |
| **10** | Validation & cutover | — | Cross-platform, graph compat, CI | 2–3 |

### 11.3 Scenario Summary

| Scenario | Active Days | Calendar Weeks |
| --- | --- | --- |
| **Optimistic** — experienced Rustacean, starlark-rust just works | 35–40 | 7–8 |
| **Realistic** — moderate Rust experience, some starlark-rust workarounds | 50–60 | 10–12 |
| **Pessimistic** — starlark-rust API gaps require forking, cloud KMS issues, TUI rework | 65–80 | 13–16 |

Best estimate: **~55 active working days (~11 weeks)**.

### 11.4 Acceleration Factors

What makes Claude Code fast here:

- Architecture doc and migration plan already exist — no exploration phase.
- Go source is readable as a spec — Claude reads Go, writes Rust directly.
- 1,150 LOC of `.gen.go` disappear entirely (replaced by ~400 lines of
  annotations).
- 1,400 LOC of reflection code disappear (replaced by ~930 lines of
  compile-time infrastructure).
- Bulk provider porting (19 providers, most under 200 LOC) is highly
  parallelizable across sessions.
- serde replaces Go struct tags and manual marshal/unmarshal — less code.
- Test generation via macro means less hand-written test code.

### 11.5 Drag Factors

What prevents it from being faster:

1. **Proc macro crate (5–8 days)**: The hardest single piece. `syn` AST
   manipulation, `quote` code generation, and `trybuild` testing require
   tight iteration cycles. Claude Code can write the macro, but verifying
   expansion correctness requires `cargo expand` → read → fix → repeat.

2. **starlark-rust is the biggest risk**: The Go codebase relies heavily on
   `go.starlark.net`'s specific APIs (`StarlarkValue`, `HasAttrs`,
   thread-local state). If `starlark-rust`'s API surface doesn't cover
   these, the options are upstream issues, forks, or adapter layers. This
   could add 0 days (it just works) or 10+ days (it doesn't).

3. **The `writ` CLI is enormous** (6,676 LOC source + 3,154 test). It is
   the single largest component and will require multiple focused sessions.

4. **The execution engine test suite** (6,891 LOC) is massive relative to
   its source (2,914 LOC). Porting the tests faithfully is time-consuming.

5. **TUI paradigm shift**: bubbletea (Elm architecture) → ratatui
   (immediate mode) is not a line-for-line port. The rendering model is
   fundamentally different. The console package (838 LOC) and UI provider
   (103 LOC) need rethinking.

6. **Human remains the bottleneck**: Claude Code accelerates writing but
   the review-compile-test-debug cycle is still sequential and human-gated.

### 11.6 Comparison to Original Development

The Go codebase was built in 30 active days (~2,157 LOC/day total). The
Rust port benefits from a proven architecture to follow, but Rust's compile
times, borrow checker, and the proc macro innovation add friction. Expect
roughly 1.5–2× the original development time, which aligns with ~50–60
days.

### 11.7 Critical Path

```
Phase 0 (1d) → Phase 1 (4d) → Phase 2 (7d) → Phase 3 (3d) → Phase 5 (10d) → Phase 8 (5d) → Phase 10 (3d)
                                                    ↘ Phase 4 (3d) ↗
                                               Phase 6 (5d) ──────────↗
                                               Phase 7 (10d) → Phase 8 ↗
```

Critical path minimum: ~33 days. The remainder is parallel work that a
single developer handles sequentially, hence the ~55 day total.

### 11.8 Recommended Pre-Commitment Spike

Before committing to the full port, invest 2–3 days on a Phase 1 spike
evaluating `starlark-rust`. Write a minimal provider with `StarlarkValue`,
`HasAttrs`, and thread-local context passing. If that works cleanly, the
estimate holds. If not, add 5–10 days or reconsider the migration.


---

## 12. Migration Deferral Rationale (March 2026)

This section records the analysis and decision to defer the Rust migration
until the Go feature set is complete. It documents the dependency risks,
ecosystem health, alternative language evaluation, and the reasoning behind
staying on Go+Starlark for now.

### 12.1 starlark-rust Dependency Risk

The highest-risk dependency in the migration is
[`starlark-rust`](https://github.com/facebook/starlark-rust) (crate:
`starlark`). Analysis as of March 2026:

| Metric                  | Value                                    |
|-------------------------|------------------------------------------|
| Last crate release      | v0.13.0, December 2024 (15 months ago)   |
| Release cadence         | Irregular — Dec 2022, Dec 2024           |
| crates.io dependents    | 13                                       |
| GitHub stars            | ~700                                     |
| Primary consumer        | Buck2 (Meta's internal build system)     |
| Maintenance posture     | Internal-first; external use incidental  |

**Key concerns:**

1. **15-month release gap.** The crate has not published a release since
   December 2024. Commits continue on `main` for Buck2's needs, but these
   are not cut into releases on any predictable schedule. Consumers must
   either pin to stale releases or track `main` via git dependency.

2. **13 dependents.** For comparison, `go.starlark.net` has 1,158
   importers. The Rust Starlark ecosystem is two orders of magnitude
   smaller. Community pressure for stability, documentation, and API
   compatibility is negligible.

3. **Internal-first maintenance.** Meta maintains `starlark-rust` for
   Buck2. Breaking changes that serve Buck2 ship without regard for
   external consumers. There is no external stability commitment and no
   published deprecation policy.

4. **Context passing difference.** Go Starlark uses `thread.SetLocal(key,
   value)` — a keyed map that allows multiple independent subsystems to
   store context. Rust Starlark uses `Evaluator.extra` — a single
   `Option<&dyn AnyLifetime>`, requiring all context to be bundled into one
   struct. This is workable but requires a different design pattern for
   provider context injection.

5. **No external governance.** Unlike projects with independent foundations
   or diverse maintainer pools, `starlark-rust` lives under
   `facebook/` on GitHub with Meta employees as sole maintainers.

**Risk assessment:** The dependency is usable but fragile for production
use outside Meta's ecosystem. A pre-commitment spike (Section 11.8) is
essential before committing engineering time.

### 12.2 Go Starlark Ecosystem Health

By contrast, `go.starlark.net` is a healthy dependency:

| Metric              | Value                                         |
|---------------------|-----------------------------------------------|
| Importers           | 1,158 (pkg.go.dev)                            |
| Release model       | Zero tagged releases; pseudo-version pinning   |
| Maintenance         | Active commits from Google engineers           |
| Governance          | `google/` GitHub org, multiple contributors    |
| Notable consumers   | Bazel, Skycfg, Tilt, CUE, many others         |

The lack of tagged releases is a Go ecosystem convention — not a warning
sign. The module is imported by over a thousand projects, ensuring that
breaking changes are caught quickly by the community. devlore pins to a
specific commit hash via Go's pseudo-version mechanism
(`v0.0.0-20260210143700-b62fd896b91b`), which is deterministic and
reproducible.

### 12.3 Embeddable Language Ecosystem Comparison

To evaluate alternatives, we surveyed embeddable scripting languages across
both Go and Rust:

**Go ecosystem (by GitHub importers):**

| Language    | Library         | Importers |
|-------------|-----------------|-----------|
| JavaScript  | goja            | 3,915     |
| Lua         | gopher-lua      | 2,345     |
| Starlark    | go.starlark.net | 1,158     |
| Lua (cgo)   | golua           | ~400      |
| Tengo       | tengo           | ~350      |

**Rust ecosystem (by crates.io dependents):**

| Language    | Library         | Dependents |
|-------------|-----------------|------------|
| Lua         | mlua            | 284        |
| Rhai        | rhai            | 220        |
| JavaScript  | boa_engine      | ~100       |
| Starlark    | starlark        | 13         |
| Lua (older) | rlua            | ~50        |

**Observation:** The Rust embeddable language ecosystem is 10–30× smaller
than Go's across every language. This is not specific to Starlark — it
reflects Rust's younger ecosystem and smaller user base for this class of
application. Lua (`mlua` at 284 dependents) is the healthiest embeddable
language option in Rust, but still an order of magnitude behind Go's
options.

### 12.4 Lua as an Alternative: Evaluation and Rejection

Lua was evaluated as a potential replacement for Starlark, either as the
sole embedded language or as a second language alongside Starlark.

**Lua advantages over Starlark:**

- Richer feature set: coroutines, `pcall`/`xpcall` error handling,
  metatables, pattern matching, string library
- Stable specification (Lua 5.4, published standard)
- Larger ecosystem in both Go and Rust
- Battle-tested in gamedev, networking, and embedded systems

**Starlark advantages over Lua:**

- **Python syntax.** devlore targets DevOps and infrastructure engineers.
  Python is the lingua franca of this audience — far more familiar than
  Lua's `local`, `then`/`end`, 1-based indexing, and `~=` for inequality.
- **Determinism.** Starlark is hermetic by design: no I/O, no
  non-deterministic operations, frozen modules. This aligns with devlore's
  declarative provider model.
- **Frozen modules.** Loaded modules are immutable — no accidental global
  state mutation between providers.
- **Dict ordering.** Starlark dicts are insertion-ordered (like Python
  3.7+). Lua tables have undefined iteration order.

**Scenarios evaluated:**

| Scenario                        | Go | Rust | Assessment                                  |
|---------------------------------|----|------|---------------------------------------------|
| Starlark only (current)         | ✓  | —    | Healthy ecosystem, Python syntax, hermetic   |
| Lua only                        | ✓  | ✓    | Larger ecosystem but unfamiliar syntax        |
| Starlark + Lua                  | ✓  | ✓    | Maximum flexibility but doubled surface area  |
| Starlark only (Rust migration)  | —  | ✓    | Fragile dependency (13 dependents)            |
| Lua only (Rust migration)       | —  | ✓    | Healthy dependency but syntax trade-off       |

**Decision: familiarity wins.** devlore's users are infrastructure
engineers who think in Python. Starlark's Python syntax is a feature, not
an accident. Forcing users to learn Lua's idioms (1-based indexing,
`local` scoping, `then`/`end` blocks, `~=` for not-equal) adds friction
that outweighs Lua's technical advantages. The cost is borne by every user
on every script they write, forever.

Lua remains a viable fallback if `starlark-rust` proves unmaintainable
during a future migration attempt. The `mlua` crate (284 dependents) is
healthy enough for production use.

### 12.5 Language Adoption Context

Stack Overflow survey data (2019–2025) on professional developer usage
provides context for the Go-vs-Rust ecosystem size difference:

| Language | 2019   | 2021   | 2023   | 2025   | Trend      |
|----------|--------|--------|--------|--------|------------|
| Java     | 39.2%  | 35.4%  | 30.5%  | 28.0%  | Declining  |
| C#       | 31.4%  | 27.9%  | 27.6%  | 28.8%  | Stable     |
| Go       | 8.8%   | 9.4%   | 13.2%  | 16.1%  | Doubling   |
| Rust     | 3.2%   | 7.0%   | 13.1%  | 16.3%  | Quintupling|

Go and Rust have converged in developer adoption (~16% each in 2025), but
Go's 6-year head start means its library ecosystem is significantly more
mature. This explains why every embeddable language has 10–30× more Go
consumers than Rust consumers. The gap is closing but has not closed.

### 12.6 Remaining Go Work

The following features are planned or in progress in the Go codebase:

1. **Reconciliation** (`docs/architecture/5.1-reconciliation.md`):
   Changes `Action.Do` from 3 returns to 4, adds `ReconcilableAction`
   interface, moves `RecoveryStack` to `pkg/op`. This is a fundamental
   interface change that would be expensive to port mid-migration.

2. **Orchestration primitives** (`docs/architecture/2.3-orchestration-primitives.md`):
   `Gather`, `Choose`, `WaitUntil`, `SlotProxy`, `RuntimePredicate`.

3. **Graph convergence operations** (`docs/architecture/2.3-orchestration-primitives.md`):
   `Elevate` and related operations.

4. **Receipt integrity** (`docs/architecture/5-receipt-integrity.md`).

Completing these features in Go first means the Rust port has a stable,
proven API surface to target — rather than porting a moving target.

### 12.7 Decision

**Defer the Rust migration.** The rationale:

1. **Go Starlark is healthy.** 1,158 importers, active maintenance, stable
   API. There is no urgency to leave.

2. **starlark-rust is fragile.** 13 dependents, 15-month release gap,
   internal-only maintenance. Viable but risky for a production foundation.

3. **Familiarity wins.** Starlark's Python syntax is a core product
   feature for devlore's DevOps audience. No alternative language matches
   it.

4. **Finish Go features first.** Reconciliation, orchestration primitives,
   and convergence operations change fundamental interfaces. Port a stable
   API, not a moving target.

5. **The migration remains viable.** Nothing in this analysis makes the
   Rust migration impossible — only premature. When Go features stabilize,
   revisit with the Phase 1 spike (Section 11.8). If `starlark-rust` has
   improved by then, proceed. If not, evaluate `mlua` as a fallback.

**Estimated timeline:** Complete remaining Go features (~25–35 days of
active development), then reassess. The Rust migration itself is estimated
at ~55 days (Section 11.4) once started.
