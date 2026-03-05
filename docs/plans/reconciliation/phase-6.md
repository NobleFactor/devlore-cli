---
title: "Phase 6: Error prefix cleanup"
parent: ../reconciliation.md
status: draft
---

# Phase 6: Error prefix cleanup

## Summary

Remove boundary-layer context from provider error messages. Provider
methods return context-agnostic errors; the executor adds phase-aware
wrapping at the boundary.

## Rationale

The architecture document (§Provider Access Contexts) establishes that
the provider is context-agnostic. It does not know whether it is being
called as a forward action, a compensation, or a reconciliation. Error
prefixes like `"compensate disable (enable %s) failed"` embed executor
context in provider code.

The boundary layer (executor, Starlark receiver, CLI call site) is the
place that knows the execution phase. It adds the wrapping.

## Current violations

Service provider (`pkg/op/provider/service/provider.go`):

```go
// Line 63
return fmt.Errorf("compensate disable (enable %s) failed: %s", state.Name, r.Stderr)

// Line 100
return fmt.Errorf("compensate enable (disable %s) failed: %s", state.Name, r.Stderr)

// Line 165
return fmt.Errorf("compensate start (stop %s) failed: %s", state.Name, r.Stderr)

// Line 202
return fmt.Errorf("compensate stop (start %s) failed: %s", state.Name, r.Stderr)
```

These should become:

```go
return fmt.Errorf("enable %s: %s", state.Name, r.Stderr)
```

The word `"compensate"` is added by the executor when it wraps the error
in the `ExecutionEvent` envelope.

## Executor wrapping

The executor already wraps action errors:

```go
return fmt.Errorf("%s: %w", actionName, err)
```

This gains phase awareness:

```go
// During forward execution
return fmt.Errorf("%s: %w", actionName, err)

// During compensation (inside PushAction's closure)
return fmt.Errorf("compensate %s: %w", actionName, err)
```

## Tasks

- [ ] Audit all providers for phase-specific error prefixes (`"compensate"`,
  `"undo"`, `"rollback"`)
- [ ] Strip prefixes from service provider `CompensateX` methods
- [ ] Strip prefixes from any other providers found during audit
- [ ] Add phase-aware error wrapping in executor/`PushAction`

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/provider/service/provider.go` | Modify | Remove `"compensate ..."` prefixes |
| `pkg/op/recovery.go` | Modify | `PushAction` closure wraps with `"compensate"` prefix |
| `internal/execution/executor.go` | Verify | Confirm forward-path wrapping is sufficient |
