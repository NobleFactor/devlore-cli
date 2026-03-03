# Plan: Worker 5 Phase 4 - Wasm Host Callbacks

## Summary

Implement host callbacks for the Wasm runtime, enabling sandboxed Wasm extensions to invoke privileged host operations (`shell.run`, `http.get`, `fs.read`, `fs.write`) through capability-checked mechanisms.

## Branch Setup

```bash
# Start from develop (current branch is behind origin/develop)
git checkout develop
git pull origin develop

# Create Phase 4 branch
git checkout -b feat/ext-wasm-phase-4
```

## Implementation

### 1. Create `internal/wasm/callbacks.go`

**Purpose:** Define `HostCallbacks` interface and `DefaultCallbacks` implementation.

**Types:**
```go
type HostCallbacks interface {
    ShellRun(ctx context.Context, cmd string, args []string, dir string) (stdout, stderr string, exitCode int, err error)
    HTTPGet(ctx context.Context, url string, headers map[string]string) (body []byte, statusCode int, err error)
    FSRead(ctx context.Context, path string) ([]byte, error)
    FSWrite(ctx context.Context, path string, data []byte) error
}

type DefaultCallbacks struct {
    checker      *CapabilityChecker
    ShellTimeout time.Duration  // 60s
    HTTPTimeout  time.Duration  // 30s
    MaxFileSize  int64          // 10MB
}
```

**Each method:**
1. Validates capability via `checker.CheckHostCall()`
2. Validates paths via `checker.CheckRead()`/`CheckWrite()` where applicable
3. Executes operation with timeout
4. Returns result or `WasmError`

### 2. Create `internal/wasm/hostmodule.go`

**Purpose:** Build wazero host module that Wasm modules can import.

**Host Module Name:** `star_host`

**Exported Functions:**
| Function | Signature | Description |
|----------|-----------|-------------|
| `shell_run` | `(req_ptr, req_len, resp_ptr, resp_cap) -> resp_len` | Execute shell command |
| `http_get` | `(req_ptr, req_len, resp_ptr, resp_cap) -> resp_len` | HTTP GET request |
| `fs_read` | `(req_ptr, req_len, resp_ptr, resp_cap) -> resp_len` | Read file |
| `fs_write` | `(req_ptr, req_len) -> error_code` | Write file |
| `get_last_error` | `(error_ptr, error_cap) -> error_len` | Get last error message |

**Request/Response Protocol:** JSON via linear memory pointers.

**Return Values:**
- `>= 0`: Success (response length)
- `-1`: Context canceled
- `-2`: Timeout
- `-3`: Internal error
- `-4`: Capability violation
- `-5`: Protocol error

### 3. Modify `internal/wasm/host.go`

**Add to `WasmHost` struct:**
```go
callbacks  HostCallbacks
hostModule api.Module
```

**Modify `NewHost()`:**
- Create `DefaultCallbacks` with `CapabilityChecker`
- Call `InstantiateHostModule()` before returning

**Modify `Close()`:**
- Close `hostModule` before runtime

**Add accessor:**
```go
func (h *WasmHost) Callbacks() HostCallbacks
```

### 4. Create Tests

**`internal/wasm/callbacks_test.go`:**
- Test each callback with valid capabilities
- Test capability denial
- Test timeout handling
- Test path validation

**`internal/wasm/hostmodule_test.go`:**
- Integration test with test Wasm module
- Test memory-based argument passing
- Test error code returns

### 5. Update `internal/wasm/doc.go`

Document Phase 4 additions.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/wasm/callbacks.go` | CREATE | HostCallbacks interface + DefaultCallbacks |
| `internal/wasm/hostmodule.go` | CREATE | wazero host module builder |
| `internal/wasm/host.go` | MODIFY | Add callbacks and host module to WasmHost |
| `internal/wasm/callbacks_test.go` | CREATE | Callback unit tests |
| `internal/wasm/hostmodule_test.go` | CREATE | Host module integration tests |
| `internal/wasm/doc.go` | MODIFY | Update documentation |

## Verification

```bash
go test ./internal/wasm/...
go build ./...
```

## Security Notes

1. All callbacks validate capabilities before execution
2. Paths resolved to absolute and checked against allowed directories
3. Timeouts prevent resource exhaustion
4. Commands executed without shell interpretation
