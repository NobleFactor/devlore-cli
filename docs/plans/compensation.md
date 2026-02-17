---
title: "Action Compensation"
issue: TBD
status: draft
created: 2026-02-17
updated: 2026-02-17
---

# Plan: Action Compensation

## Summary

Implement real compensation (undo) logic across all providers. Today every
action's `Undo` returns nil — the recovery stack unwinds but does nothing.
This plan adds Backward methods to Provider structs, state capture in Forward
methods, and wiring in generated actions so that `Undo` delegates to the
Provider's compensation logic. The generator template is updated last so that
`actions_gen.go` files become nuke-safe with full Do/Undo delegation.

Supersedes: `docs/plans/phase-execution.md` (retired — structural goals
achieved by resource-provider plan; compensation goals carried forward here).

## Goals

1. **State capture in Forward methods.** Provider Forward methods that are
   compensable return `(result, map[string]any, error)`. The state map carries
   what Backward needs to undo the operation.
2. **Backward methods on Providers.** Each compensable Forward method gets a
   `Compensate<Method>` pair on the Provider struct. Naming convention is the
   contract — no interface, no annotation.
3. **Action Do/Undo wiring.** Generated actions capture state from Forward in
   `Do` (as `UndoState`) and delegate to `Compensate<Method>` in `Undo`.
4. **Generator template update.** The `graph_actions` template detects Activity
   pairs and emits the wiring automatically. After this, `actions_gen.go` files
   are nuke-safe.
5. **Non-compensable actions stay nil.** Read-only actions (`file.source`,
   `encryption.decrypt`, `template.render`, `content.literal`), idempotent
   actions (`pkg.update`), and arbitrary commands (`shell.exec`,
   `shell.powershell`) return nil from `Undo`. No forced compensation.

## Current State

| Component | Status | Notes |
|---|---|---|
| Action interface (Do/Undo) | Implemented | `action.go` |
| RecoveryStack.Unwind | Implemented | Calls `Action.Undo()` per node in reverse |
| Provider Forward methods | Implemented | Return `(result, error)` — no state |
| Provider Backward methods | Missing | No `Compensate<Method>` methods |
| Action Undo wiring | Missing | All return nil |
| Generator Activity detection | Missing | Template emits nil Undo only |

## Compensation Inventory

### Compensable Actions (19)

Actions that modify system state and can be reversed.

**File Provider (7 compensable / 9 total):**

| Forward | Backward | State Captured |
|---|---|---|
| `Copy` | `CompensateCopy` | `{path, existed_before, previous_content?}` |
| `Link` | `CompensateLink` | `{path, existed_before, previous_target?}` |
| `Backup` | `CompensateBackup` | `{original_path, backup_path}` |
| `Write` | `CompensateWrite` | `{path, existed_before, previous_content?}` |
| `Move` | `CompensateMove` | `{source, path}` |
| `Unlink` | `CompensateUnlink` | `{path, target}` (symlink target for re-creation) |
| `Remove` | `CompensateRemove` | `{path, content, mode}` (file content for re-creation) |

Non-compensable: `Source` (read-only), `Mkdir` (idempotent, parents shared).

**Service Provider (5 compensable / 5 total):**

| Forward | Backward | State Captured |
|---|---|---|
| `Start` | `CompensateStart` | `{name, was_running}` |
| `Stop` | `CompensateStop` | `{name, was_running}` |
| `Restart` | `CompensateRestart` | `{name}` (no-op — service was running) |
| `Enable` | `CompensateEnable` | `{name, was_enabled}` |
| `Disable` | `CompensateDisable` | `{name, was_enabled}` |

**Package Provider (3 compensable / 4 total):**

| Forward | Backward | State Captured |
|---|---|---|
| `Install` | `CompensateInstall` | `{packages, manager, cask, already_installed}` |
| `Upgrade` | `CompensateUpgrade` | `{packages, manager, cask, previous_versions}` |
| `Remove` | `CompensateRemove` | `{packages, manager, cask}` |

Non-compensable: `Update` (idempotent index refresh).

**Net Provider (1 compensable / 1 total):**

| Forward | Backward | State Captured |
|---|---|---|
| `Download` | `CompensateDownload` | `{path}` (remove downloaded file; in-memory returns not compensable) |

**Archive Provider (1 compensable / 1 total):**

| Forward | Backward | State Captured |
|---|---|---|
| `Extract` | `CompensateExtract` | `{dest, created_files}` |

**Git Provider (1 compensable / 3 total):**

| Forward | Backward | State Captured |
|---|---|---|
| `Clone` | `CompensateClone` | `{path}` (remove cloned directory) |

Non-compensable: `Checkout` (would need to track previous ref — complex, low
value), `Pull` (same — would need to track previous HEAD).

### Non-Compensable Actions (9)

| Action | Reason |
|---|---|
| `file.source` | Read-only — no side effects |
| `file.mkdir` | Idempotent, parents may be shared |
| `encryption.decrypt` | Pure transform — no side effects |
| `template.render` | Pure transform — no side effects |
| `content.literal` | Pure value producer — no side effects |
| `pkg.update` | Idempotent index refresh |
| `shell.exec` | Arbitrary command — no auto-compensation |
| `shell.powershell` | Arbitrary command — no auto-compensation |
| `git.checkout` | Complex state tracking, low value |

Non-compensable actions continue returning nil from `Undo`.

### Flow Actions (4) — Already Done

| Action | Undo Behavior |
|---|---|
| `flow.gather` | GatherUndoState — walks iterations in reverse |
| `flow.choose` | ChooseUndoState — delegates to selected branch |
| `flow.elevate` | Stub (full implementation in elevation plan) |
| `flow.wait_until` | No-op (observation only) |

## Provider Method Signature Change

### Compensable Forward Methods

Current:
```go
func (p *Provider) Copy(path string, mode os.FileMode, content []byte) (string, error)
```

New:
```go
func (p *Provider) Copy(path string, mode os.FileMode, content []byte) (string, map[string]any, error)
```

The `map[string]any` state is the compensation receipt — opaque to the executor,
meaningful only to the Provider's Backward method.

### Non-Compensable Forward Methods

No change. Methods like `Source(path string) ([]byte, error)` keep their
current signature.

### Backward Methods

```go
func (p *Provider) CompensateCopy(state map[string]any) error {
    path, _ := state["path"].(string)
    existed, _ := state["existed_before"].(bool)
    if existed {
        return nil // File existed before — don't remove
    }
    return os.Remove(path)
}
```

## Action Do/Undo Wiring

### Compensable Action (new pattern)

```go
func (o *Copy) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
    path := slots["path"].(string)
    mode, _ := slots["mode"].(os.FileMode)
    content, _ := slots["content"].([]byte)

    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] copy %v\n", path)
        return nil, nil, nil
    }
    checksum, state, err := o.Impl.Copy(path, mode, content)
    if err != nil {
        return nil, nil, err
    }
    ctx.TargetChecksum = checksum
    return nil, state, nil  // state becomes UndoState
}

func (o *Copy) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
    s, _ := state.(map[string]any)
    if s == nil {
        return nil
    }
    return o.Impl.CompensateCopy(s)
}
```

### Non-Compensable Action (unchanged)

```go
func (o *Source) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
    return nil
}
```

## Implementation Phases

### Phase 1: File Provider (Reference Implementation)

The largest provider (9 actions, 7 compensable). Establishes the pattern.

- [ ] Add state capture to compensable Forward methods (7 methods):
      `Copy`, `Link`, `Backup`, `Write`, `Move`, `Unlink`, `Remove`
- [ ] Add Backward methods (7 methods):
      `CompensateCopy`, `CompensateLink`, `CompensateBackup`,
      `CompensateWrite`, `CompensateMove`, `CompensateUnlink`,
      `CompensateRemove`
- [ ] Pre-action state query: check `existed_before`, save `previous_target`
      (for link), save `content` + `mode` (for remove)
- [ ] Update action Do to capture state as UndoState (7 actions)
- [ ] Update action Undo to delegate to CompensateXxx (7 actions)
- [ ] Non-compensable actions (`Source`, `Mkdir`) unchanged
- [ ] Tests: Forward+state round-trip, Backward restores previous state

**Files:**

| File | Action | Purpose |
|---|---|---|
| `provider/file/provider.go` | Modify | Add state returns to Forward, add Backward methods |
| `provider/file/actions_gen.go` | Modify | Wire Do state capture + Undo delegation |
| `provider/file/provider_test.go` | Create | Forward/Backward tests per Activity pair |

### Phase 2: Service Provider

Natural inverse pairs. Requires querying service state before acting.

- [ ] Add pre-action state query: is service running? is service enabled?
      (platform-specific: `systemctl is-active`, `launchctl list`, `sc query`)
- [ ] Add state capture to Forward methods (5 methods)
- [ ] Add Backward methods:
      `CompensateStart` (stop if wasn't running),
      `CompensateStop` (start if was running),
      `CompensateRestart` (no-op),
      `CompensateEnable` (disable if wasn't enabled),
      `CompensateDisable` (enable if was enabled)
- [ ] Update action wiring (5 actions)
- [ ] Tests: mock service state query, verify Backward logic

**Files:**

| File | Action | Purpose |
|---|---|---|
| `provider/service/provider.go` | Modify | Add state query, state returns, Backward methods |
| `provider/service/actions_gen.go` | Modify | Wire Do/Undo |
| `provider/service/provider_test.go` | Create | Forward/Backward tests |

### Phase 3: Package Provider

Requires querying installed-package state before acting.

- [ ] Add pre-action state query: which packages are already installed?
      (platform-specific: `dpkg -l`, `rpm -q`, `brew list`, etc.)
- [ ] Add state capture to Forward methods (3 methods)
- [ ] Add Backward methods:
      `CompensateInstall` (remove packages that weren't already installed),
      `CompensateUpgrade` (downgrade to saved versions),
      `CompensateRemove` (re-install removed packages)
- [ ] Update action wiring (3 actions)
- [ ] Tests

**Files:**

| File | Action | Purpose |
|---|---|---|
| `provider/pkg/provider.go` | Modify | Add state query, state returns, Backward methods |
| `provider/pkg/actions_gen.go` | Modify | Wire Do/Undo |
| `provider/pkg/provider_test.go` | Create | Forward/Backward tests |

### Phase 4: Remaining Providers (Net, Archive, Git)

Smaller scope — 3 compensable actions across 3 providers.

- [ ] `net.Download`: save path, CompensateDownload removes file
- [ ] `archive.Extract`: save dest + created file list, CompensateExtract
      removes created files
- [ ] `git.Clone`: save path, CompensateClone removes directory
- [ ] Update action wiring (3 actions)
- [ ] Tests

**Files:**

| File | Action | Purpose |
|---|---|---|
| `provider/net/provider.go` | Modify | Add state return, Backward method |
| `provider/net/actions_gen.go` | Modify | Wire Do/Undo |
| `provider/archive/provider.go` | Modify | Add state return, Backward method |
| `provider/archive/actions_gen.go` | Modify | Wire Do/Undo |
| `provider/git/provider.go` | Modify | Add state return, Backward method |
| `provider/git/actions_gen.go` | Modify | Wire Do/Undo |

### Phase 5: Generator Template Update

Update the `graph_actions` template in noblefactor-ops to detect Activity
pairs and emit the wiring automatically. After this phase, all `actions_gen.go`
files are nuke-safe: `rm *_gen.go` + regenerate produces identical code.

- [ ] Update `methodInfo` analysis to detect `Compensate<Method>` pairs
- [ ] Gate 3 validation: verify Backward signature is
      `func(state map[string]any) error`
- [ ] Update `graph_actions.go.template` to emit:
      - Do: capture state from Forward's `map[string]any` return → UndoState
      - Undo: delegate to `Impl.CompensateXxx(state)` for compensable ops
      - Undo: return nil for non-compensable ops
- [ ] Filter `Compensate*` methods from Forward method list in `generate.star`
- [ ] Verify: regenerate all `actions_gen.go`, diff against hand-written
- [ ] Update `plan_receiver.go.template` if needed (unlikely — plan receivers
      don't touch compensation)

**Files (noblefactor-ops):**

| File | Action | Purpose |
|---|---|---|
| `internal/starlark/receiver_go_gen.go` | Modify | Template + pair detection |
| `star/extensions/.../commands/generate.star` | Modify | Filter Compensate* methods |

**Files (devlore-cli):**

| File | Action | Purpose |
|---|---|---|
| `star/extensions/.../templates/graph_actions.go.template` | Modify | Emit Do/Undo wiring |

### Phase 6: Compensation Integration Tests

End-to-end tests verifying the full compensation cycle: build graph → execute
→ fail mid-phase → unwind → verify compensation occurred.

- [ ] Test: file phase fails mid-execution, completed file ops are compensated
      (created files removed, moved files restored)
- [ ] Test: service phase fails, started services are stopped
- [ ] Test: package phase fails, installed packages are removed
- [ ] Test: gather with 5 items, item 3 fails — items 1-2 compensated,
      items 4-5 never started
- [ ] Test: nested phase compensation (gather body contains choose with
      compensable branch)
- [ ] Test: dry-run produces no UndoState (nil), Undo is no-op

**Files:**

| File | Action | Purpose |
|---|---|---|
| `internal/execution/compensation_test.go` | Create | Integration tests |

## Migration Path

No migration. Greenfield product — no deployed graphs to support.

## Related Documents

- [Phase Execution Architecture](../architecture/devlore-phase-execution.md) —
  Saga pattern, Activities, compensation ownership, two-layer model
- [Orchestration Primitives Architecture](../architecture/devlore-orchestration-primitives.md) —
  GatherUndoState, ChooseUndoState (already implemented)
- [Resource-Provider Plan](resource-provider.md) — Parent plan, action inventory
- [Star Gen Receiver Plan](star-gen-receiver.md) — Code generation pipeline

## Open Questions

None.
