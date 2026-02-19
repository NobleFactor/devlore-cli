# Phase 4: Wiring and Cleanup

**Status**: COMPLETE — PR #151

## Summary

Update `bindings.go` to construct generated receivers with Provider instances.
Delete the `platform/` directory (4 files of dead code).

## Files

| File | Action |
|------|--------|
| `internal/starlark/bindings.go` | Modify: update receiver constructors |
| `internal/starlark/interfaces.go` | Modify: update PlanBindings interface |
| `internal/starlark/plan.go` | Modify: update planBindings implementation |
| `internal/starlark/platform/common.go` | Delete |
| `internal/starlark/platform/darwin.go` | Delete |
| `internal/starlark/platform/linux.go` | Delete |
| `internal/starlark/platform/windows.go` | Delete |
