# Plan: Worker 5 - Wasm Runtime Phase 2

## Summary

Implement the WebAssembly host runtime package (`internal/wasm/`) using wazero for sandboxed extension execution. This phase establishes the foundation for loading and executing Wasm modules with capability-based security.

## Branch

`feat/ext-wasm-phase-2`

## Files to Create

| File | Purpose |
|------|---------|
| `internal/wasm/doc.go` | Package documentation |
| `internal/wasm/host.go` | wazero runtime setup, module compilation/caching |
| `internal/wasm/module.go` | WasmModule wrapper with Call() |
| `internal/wasm/capabilities.go` | Capability validation and path checking |
| `internal/wasm/protocol.go` | Request/Response types for extension-host communication |
| `internal/wasm/errors.go` | Custom error types |
| `internal/wasm/host_test.go` | Unit tests for host functionality |
| `internal/wasm/capabilities_test.go` | Unit tests for capability validation |

## Dependencies

Add to `go.mod`:
```
github.com/tetratelabs/wazero
```

## Key Design Decisions

1. **Compilation Caching**: Use `wazero.NewCompilationCacheWithDir` with `~/.cache/star/wasm/wazero` for 10x faster repeated module loads

2. **Data Passing**: Use stdin/stdout JSON serialization (simpler than malloc/free, works with WASI modules)

3. **Module Lifecycle**: Reactor pattern with `WithStartFunctions("_initialize")` for multiple calls per instance

4. **Error Handling**: Handle `sys.ExitError`, never double-close on error (causes SIGSEGV)

5. **Capabilities**: Import `Capabilities` from `internal/extension` (already defined in spec.go:112-129)

## Interface Contract

Worker 5 implements, Worker 3 consumes:

```go
// host.go
type WasmHost struct { ... }

func NewHost(ctx context.Context, caps extension.Capabilities) (*WasmHost, error)
func (h *WasmHost) LoadModule(wasmPath string) (*WasmModule, error)
func (h *WasmHost) Close() error

// module.go
type WasmModule struct { ... }

func (m *WasmModule) Call(ctx context.Context, function string, args []byte) ([]byte, error)
func (m *WasmModule) Path() string
func (m *WasmModule) ExportedFunctions() []string

// capabilities.go
func ValidateCapabilities(caps extension.Capabilities) error

// protocol.go
type Request struct {
    ID     uint64          `json:"id"`
    Method string          `json:"method"`
    Params json.RawMessage `json:"params,omitempty"`
}

type Response struct {
    ID     uint64          `json:"id"`
    Result json.RawMessage `json:"result,omitempty"`
    Error  *WasmError      `json:"error,omitempty"`
}
```

## Implementation Steps

### 1. Add wazero dependency
```bash
go get github.com/tetratelabs/wazero@latest
```

### 2. Create `internal/wasm/doc.go`
Package documentation explaining the wasm host runtime.

### 3. Create `internal/wasm/errors.go`
```go
type WasmError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}
```

### 4. Create `internal/wasm/protocol.go`
Request/Response envelope types for JSON over stdin/stdout.

### 5. Create `internal/wasm/capabilities.go`
- `ValidateCapabilities(caps extension.Capabilities) error`
- Path validation (must be absolute or `/workspace`)
- Block sensitive paths (`/etc`, `/var`, `/usr`, etc.)
- Host call format validation (`namespace.method`)

### 6. Create `internal/wasm/host.go`
```go
func NewHost(ctx context.Context, caps extension.Capabilities) (*WasmHost, error) {
    if err := ValidateCapabilities(caps); err != nil {
        return nil, fmt.Errorf("invalid capabilities: %w", err)
    }

    cacheDir := filepath.Join(os.UserCacheDir(), "star", "wasm", "wazero")
    cache, _ := wazero.NewCompilationCacheWithDir(cacheDir)

    config := wazero.NewRuntimeConfig().
        WithCompilationCache(cache).
        WithCloseOnContextDone(true)

    rt := wazero.NewRuntimeWithConfig(ctx, config)
    wasi_snapshot_preview1.Instantiate(ctx, rt)

    return &WasmHost{runtime: rt, cache: cache, caps: caps, ctx: ctx}, nil
}
```

Key features:
- Compilation cache in user cache directory
- WASI instantiated for all modules
- `WithCloseOnContextDone` for timeout handling
- Module cache with `sync.Map`

### 7. Create `internal/wasm/module.go`
```go
func (m *WasmModule) Call(ctx context.Context, function string, args []byte) ([]byte, error) {
    var stdout, stderr bytes.Buffer

    req := Request{ID: 1, Method: function, Params: args}
    reqBytes, _ := json.Marshal(req)

    config := wazero.NewModuleConfig().
        WithName("").
        WithStdin(bytes.NewReader(reqBytes)).
        WithStdout(&stdout).
        WithStderr(&stderr).
        WithStartFunctions("_initialize")  // Reactor mode

    // Apply FS config from capabilities
    result, err := m.host.runtime.InstantiateModule(ctx, m.compiled, config)
    if err != nil {
        return nil, m.handleError(err, stderr.String())
    }
    defer result.Close(ctx)

    var resp Response
    json.Unmarshal(stdout.Bytes(), &resp)
    if resp.Error != nil {
        return nil, resp.Error
    }
    return resp.Result, nil
}
```

### 8. Create unit tests
- `capabilities_test.go`: Path validation, host call validation
- `host_test.go`: NewHost with valid/invalid caps, LoadModule, Close

## Critical Files (read before implementing)

- `internal/extension/spec.go:112-129` - Capabilities types (import, don't redefine)
- `internal/starlark/receiver.go` - Pattern for Starlark integration (Phase 4)
- `go.mod` - Add wazero dependency

## Verification

```bash
go get github.com/tetratelabs/wazero@latest
go test ./internal/wasm/...
go build ./...
```

## Notes

- Phase 4 will add host callbacks using `runtime.NewHostModuleBuilder`
- The `/workspace` placeholder expands to the current working directory
- Empty capabilities = no access allowed (strict sandboxing)
