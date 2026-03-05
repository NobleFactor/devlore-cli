# Rust Migration: Architecture and Design Decisions

This document captures the full design context for porting devlore-cli from
Go to idiomatic Rust. It records the reasoning behind every architectural
decision so that any future session can pick up the work with complete
understanding.

See also: [Rust Migration Plan](../plans/rust-migration.md) — phased
implementation plan with tasks, timelines, and file listings.

## 1. Why Rust

The Go codebase works. The architecture is proven. The motivation for Rust
is not dissatisfaction with Go's runtime or performance, but with specific
language limitations that force design compromises in this codebase:

### 1.1 The Type Erasure Problem

Go's `action.go` defines:

```go
type Result = any
type UndoState = any
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
`UndoState = any`. This erases provider-specific types. The Rust port uses
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
