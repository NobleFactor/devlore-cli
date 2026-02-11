# WASM Extension Architecture

## Overview

The star runtime executes extension code in sandboxed WASM modules using
[wazero](https://wazero.io/) (pure Go WebAssembly runtime). All WASM modules target
`wasm32-wasip1` and run as **shared memory reactors**: a single persistent instance per
module, with function calls via named WASM exports and data exchange through linear memory.

All modules — regardless of source language (Rust, Go) — use the same protocol:
export `alloc` and `dealloc` for memory management, plus named function exports with
`(ptr, len) -> packed_result` signatures.

## Shared Memory Reactor Protocol

### How it works

1. **First call**: Runtime instantiates the module, calls `_initialize` for setup, caches
   the instance (mutex-protected for concurrent use).
2. **Each call**: Runtime writes JSON args to module memory via `alloc`, calls the named
   export with `(ptr, len)`, reads the packed result `(result_ptr << 32 | result_len)`,
   frees both pointers via `dealloc`.
3. **On trap**: Instance is discarded; next call creates a fresh one.

### Function signature convention

```
fn(json_ptr: i32, json_len: i32) -> packed_result: i64
```

The return value packs two 32-bit values: `(result_ptr << 32) | result_len`. The result
bytes are JSON.

### Memory management exports

| Export | Signature | Purpose |
|---|---|---|
| `alloc` | `(size: i32) -> ptr: i32` | Allocate in WASM heap for host to write into |
| `dealloc` | `(ptr: i32, size: i32)` | Free previously allocated memory |

The export name `dealloc` (not `free`) avoids a symbol conflict with the C standard
library's `free()` that affected Rust `cdylib` WASM builds.

### Reactor detection

All WASM receivers use the shared memory reactor protocol. The runtime requires
`alloc` and `dealloc` exports as part of the reactor contract.

## Language-Specific Implementation

### Rust modules

Rust uses `std::alloc::alloc` / `std::alloc::dealloc` for memory management. Named
exports are bare `extern "C"` functions with `#[no_mangle]`. The `#[cfg_attr]` pattern
limits WASM-specific export names to the `wasm32` target so native tests compile cleanly.

```rust
#[cfg_attr(target_arch = "wasm32", export_name = "alloc")]
pub extern "C" fn wasm_alloc(size: u32) -> *mut u8 { ... }

#[cfg_attr(target_arch = "wasm32", export_name = "dealloc")]
pub extern "C" fn wasm_free(ptr: *mut u8, size: u32) { ... }

#[no_mangle]
pub extern "C" fn matches(json_ptr: *const u8, json_len: u32) -> u64 { ... }
```

**Example: Gitignore module** — the matcher is built lazily on first call via
`OnceLock<Gitignore>` and reused across all subsequent calls on the persistent instance.

```
star/extensions/com.noblefactor.star.Gitignore/
├── Cargo.toml          # [lib] crate-type = ["cdylib"]
├── src/lib.rs          # Named exports: matches(), filter(), alloc(), dealloc()
├── extension.yaml      # capabilities only (functions auto-discovered)
└── receivers/
    └── gitignore.wasm  # Compiled artifact (committed)
```

### Go modules

Go uses `go:wasmexport` (Go 1.24+) for named exports. Memory management uses Go's heap
with a global `map[uintptr][]byte` to prevent GC collection of host-accessible buffers.

```go
var allocations = make(map[uintptr][]byte)

//go:wasmexport alloc
func wasmAlloc(size int32) int32 {
    buf := make([]byte, size)
    ptr := uintptr(unsafe.Pointer(&buf[0]))
    allocations[ptr] = buf
    return int32(ptr)
}

//go:wasmexport dealloc
func wasmDealloc(ptr int32, size int32) {
    delete(allocations, uintptr(ptr))
}

//go:wasmexport parse_devlore_api
func wasmParseDevloreAPI(jsonPtr int32, jsonLen int32) int64 { ... }
```

`go:wasmexport` only supports scalar types (`int32`, `int64`, `float32`, `float64`,
`unsafe.Pointer`), which is exactly the `(i32, i32) -> i64` signature the protocol requires.

An empty `func main() {}` is required by the Go compiler for `-buildmode=c-shared`.

**Example: Go AST parsing module**

```
star/extensions/com.noblefactor.devlore.registry.BuildKnowledge/
├── go.mod              # stdlib only
├── src/
│   ├── main.go         # Reactor protocol: alloc, dealloc, named exports
│   ├── devlore_api.go  # parse_devlore_api handler
│   ├── migrate.go      # parse_migrate_knowledge handler
│   └── execution.go    # parse_execution_ops, parse_execution_schema
├── extension.yaml      # capabilities only (functions auto-discovered)
└── receivers/
    └── go.wasm         # Built: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared
```

## Function Auto-Discovery

Functions are always discovered from WASM exports. The `functions:` field in extension.yaml
is rejected by validation — it was a source of drift between the YAML declaration and the
actual binary.

### Infrastructure export filtering

These exports are internal to the runtime protocol and filtered from `Functions()`:

| Export | Purpose |
|---|---|
| `_initialize` | WASI reactor entry point |
| `_start` | WASI command entry point (rejected at load time) |
| `alloc` | Shared memory allocation |
| `dealloc` | Shared memory deallocation |
| `memory` | WASM linear memory |

Everything else returned by `Functions()` is exposed to Starlark as callable attributes
on the receiver module.

## Production Validation

`LoadModule()` validates the reactor contract before caching:

1. **`_initialize` must be exported** — modules without it fail with a clear error (not a
   silent no-op).
2. **`memory` must be exported** — required for shared memory reads/writes.
3. **`_start` must NOT be exported** — WASI commands are rejected (test guard, not runtime
   check).

## Capabilities and Sandboxing

Each WASM receiver declares its sandbox permissions in extension.yaml:

```yaml
capabilities:
  fs:
    read:
      - "/workspace"    # Expands to cwd at runtime
    write: []
  host_calls: []        # e.g., shell.run, http.get, fs.read, fs.write
```

The runtime enforces these via:
- **Filesystem**: wazero `FSConfig` with read-only or read-write directory mounts
- **Host calls**: `CapabilityChecker` validates each host callback before execution

## Build Instructions

### Rust modules

```bash
PATH="$HOME/.rustup/toolchains/stable-aarch64-apple-darwin/bin:$PATH" \
    cargo build --target wasm32-wasip1 --release
cp target/wasm32-wasip1/release/MODULE.wasm receivers/
```

### Go modules

```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o receivers/go.wasm ./src/
```

## WASI Migration Path

| Timeline | Status | Action |
|---|---|---|
| **Now** | Go: wasip1 only. Rust: wasip1 + wasip2 (Tier 2) | Shared memory reactor on wasip1 |
| **Mid 2026** | Go: wasip3 proposal under review. Rust: wasip3 Tier 3 | Monitor |
| **Late 2026** | Both may have P3 support | Port to P3 typed function signatures |

When WASI P3 arrives with typed function signatures, the shared memory protocol
(alloc/dealloc + packed pointer returns) can be replaced with native parameter passing.
The `IsReactor()` detection in `module.go` is designed to be extended for additional
protocol variants.

## Key Files

### noblefactor-ops (star runtime)

| File | Role |
|---|---|
| `internal/wasm/host.go` | WasmHost: runtime lifecycle, module loading, export validation |
| `internal/wasm/module.go` | WasmModule: Call() dispatch, reactor instance caching, shared memory helpers |
| `internal/wasm/errors.go` | WasmError types and error codes |
| `internal/wasm/capabilities.go` | CapabilityChecker, host callback validation |
| `internal/wasm/host_module.go` | Host function imports (shell.run, http.get, fs.read/write) |
| `internal/extension/spec.go` | ExtensionSpec, ReceiverSpec parsing and validation |
| `internal/starlark/runtime.go` | Extension loading, function auto-discovery |
| `internal/starlark/wasm_receiver.go` | WasmReceiver: Starlark HasAttrs wrapper, JSON-to-struct conversion |

### devlore-cli (extension source)

| File | Role |
|---|---|
| `star/extensions/.../src/main.go` | Go WASM receiver: alloc/dealloc + named exports |
| `star/extensions/.../receivers/go.wasm` | Compiled Go WASM binary |

### noblefactor-ops (extension source)

| File | Role |
|---|---|
| `star/extensions/.../src/lib.rs` | Rust WASM receiver: alloc/dealloc + named exports |
| `star/extensions/.../receivers/gitignore.wasm` | Compiled Rust WASM binary |
