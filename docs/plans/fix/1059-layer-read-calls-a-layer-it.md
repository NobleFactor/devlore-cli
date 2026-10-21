---
title: "layer.Read calls a layer it cannot stat unregistered, so the installers report it skipped"
issue: https://github.com/NobleFactor/devlore-cli/issues/1059
status: active
created: 2026-10-21
updated: 2026-10-21
---

# Plan: layer.Read calls a layer it cannot stat unreadable, not unregistered

Lane 44 of #949. One pull request.

## Summary

`layer.Read` (`cmd/writ/writ/layer/layer.go`) returns `Unregistered` for any error from `os.Lstat` on a layer's entry,
a permission error included. Its own states call a link that cannot be read `Unreadable`, and an entry that cannot be
stat'ed is known no better. `writ repo list` then reports the layer unregistered, and the installers (#1029) tell the
user to register a layer that may be registered.

## Current State

At `4992f580`:

- `Read` starts from `State: Unregistered` and returns it on any `Lstat` error.
- `Unreadable` is documented as "a link that cannot be read", set only when `os.Readlink` fails.
- `writ repo list` reports `layer.Unreadable` as `unreadable` (`repo_cmd.go:653`).
- Both installers ask `writ repo list --filter layer=<layer> --filter state=registered` (`install.sh:606`,
  `install.ps1:1268`): a layer not registered, for whatever reason, is reported skipped, with the command to register
  it.
- `layer_test.go` covers nothing there, a directory, a working tree, links, a dangling link and a file; no entry that
  cannot be stat'ed.

## Requirements

### Requirement 1: Only an entry that is not there is unregistered

- `Read` returns `Unregistered` when `Lstat` reports the entry does not exist (`errors.Is(err, fs.ErrNotExist)`), and
  `Unreadable` for any other error.
- `Unreadable`'s documentation says it: an entry that cannot be stat'ed, or a link that cannot be read.
- A test makes the layers directory unsearchable, so `Lstat` on an entry under it is refused, and checks `Read` says
  `Unreadable`; it fails at `4992f580`. Skipped on Windows, which has no such permission, and as root, which is not
  refused.

### Requirement 2: The installers say what writ cannot read (ruled 2026-10-21: "fold them in")

- Each installer asks `writ repo list` for a layer's state (`--filter layer=<layer> --jq '.[].state'`), not for a
  registered root alone: `registered` is already registered, as now; `unreadable` is its own line, naming the entry
  (`<XDG data>/devlore/writ/layers/<layer>`, as `devlore.WritLayersDir` places it on every platform) and saying writ
  cannot read it, to check its permissions and run the installer again, and is not offered `writ repo set`; every
  other state, or no answer, is skipped, as now.
- The `install.sh` suite makes the account's layers directory unsearchable and checks the run says the layer is
  unreadable and not skipped. The `install.ps1` suite does the same where Windows can be made to refuse the entry:
  measured on danoble-wd11-3 first, since Windows grants users traverse checking by default; if it cannot, the plan
  says so, and the PowerShell path is the same shape as the bash one.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/1059-layer-read-calls-a-layer-it` and reviewed with the owner: "fold them in,
      commit the plan and go", 2026-10-21

### Phase 2: The fix (Requirement 1)

- [x] `Read` and `Unreadable`'s documentation; `TestRead_EntryThatCannotBeStatedIsUnreadable` failed at `4992f580`
      ("State:unregistered ... want unreadable") and passes with the fix; `make test` passes

### Phase 3: The installers (Requirement 2)

- [ ] Both installers ask the state and say what writ cannot read; the `install.sh` suite's case fails before and
      passes after; the `install.ps1` suite's, or the measurement that Windows cannot refuse the entry

### Phase 4: Merge

- [ ] `make check-all`, finished green as the PR script runs it, before the script is called ready
- [ ] PR script written, shown, and handed over; the PR resolves #1059
- [ ] CI on the pull request, every leg; nothing acts on the checks while the script's gate watches them

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/writ/writ/layer/layer.go` | Modify | Requirement 1 |
| `cmd/writ/writ/layer/layer_test.go` | Modify | Requirement 1 |
| `install.sh`, `install.ps1` | Modify | Requirement 2 |
| `scripts/Test-InstallScript.sh`, `scripts/Test-InstallScript.ps1` | Modify | Requirement 2 |

## Open Questions

None. Question 1, the installers, ruled 2026-10-21: "fold them in" (Requirement 2).
