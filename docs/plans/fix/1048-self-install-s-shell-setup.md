---
title: "self install's shell setup instructions name ~/.local whatever the prefix"
issue: https://github.com/NobleFactor/devlore-cli/issues/1048
status: active
created: 2026-10-21
updated: 2026-10-21
---

# Plan: self install's shell setup instructions name the files this install wrote

Lane 37 of #949. One pull request.

## Summary

`printShellSetupInstructions` (`cmd/internal/cli/selfinstall.go:1224`) tells a PowerShell user to dot-source
`~/.local/share/powershell/completions/<tool>.ps1`, and a zsh user to put `~/.local/share/zsh/site-functions` on
`fpath`, whatever prefix was given. The install writes the completions under the prefix, by `shellCompletionPath`, so
`writ self install /usr/local` installs `/usr/local/share/zsh/site-functions/_writ` and points the user at `~/.local`.

## Current State

At `78881b09`:

| Shell | Written to, by `shellCompletionPath` | The instructions say |
| --- | --- | --- |
| bash | `<prefix>/share/bash-completion/completions/<tool>` | "ensure bash-completion is installed" |
| fish | `<prefix>/share/fish/vendor_completions.d/<tool>.fish` | "completions work automatically" |
| pwsh | `<prefix>/share/powershell/completions/<tool>.ps1` | `. ~/.local/share/powershell/completions/<tool>.ps1` |
| zsh | `<prefix>/share/zsh/site-functions/_<tool>` | `fpath=(~/.local/share/zsh/site-functions $fpath)` |

`printInstallSummary` has the prefix and calls `printShellSetupInstructions(installedShells, toolName)` without it.
No test covers the instructions.

## Requirements

### Requirement 1: The instructions name what this install wrote

- `printShellSetupInstructions` takes the prefix, and builds each path from `shellCompletionPath`, the function the
  install writes by: the PowerShell line dot-sources `<prefix>/share/powershell/completions/<tool>.ps1`, the zsh line
  puts `<prefix>/share/zsh/site-functions` on `fpath`.
- A test runs it with a prefix that is not `~/.local` and checks the narration names that prefix's files and never
  `~/.local`; it fails at `78881b09`.

### Requirement 2: bash and fish say where they look (ruled 2026-10-21: "fold them in")

- **bash** names the file written, and that bash-completion finds it once `<prefix>/bin` is on `PATH`: its loader
  looks in `<dir>/../share/bash-completion/completions` for every directory on `PATH` (read 2026-10-21 in the
  bash-completion installed on DANOBLE-UD24-1, `_comp_load`), so the summary's own PATH advice is all it needs.
- **fish** names the file written. Under a prefix whose `share` is `~/.local/share` or a directory of `XDG_DATA_DIRS`
  (`/usr/local/share:/usr/share` when unset), fish finds it on its own; under any other, the line to add to
  `~/.config/fish/config.fish` is `set --prepend fish_complete_path <prefix>/share/fish/vendor_completions.d`.
- The test covers both, under a prefix fish does not search and one it does.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/1048-self-install-s-shell-setup` and reviewed with the owner: "fold them in, commit
      the plan and go", 2026-10-21

### Phase 2: The fix (Requirements 1 and 2)

- [x] `printShellSetupInstructions` takes the prefix and names every shell's file; fish's search is `fishSearches`,
      over `xdg.DataHome` and `xdg.DataDirs`. The two tests fail at `78881b09`, the instructions naming `~/.local` and
      none of the prefix's files, and pass with the fix; `make test` passes

### Phase 3: Merge

- [x] `make check-all`, finished green as the PR script runs it, before the script is called ready: at `bda68bed`, with
      no prebuilt release
- [x] PR script written, shown, and handed over; the PR resolves #1048
- [ ] CI on the pull request, every leg; nothing acts on the checks while the script's gate watches them

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/internal/cli/selfinstall.go` | Modify | Requirements 1 and 2 |
| `cmd/internal/cli/selfinstall_test.go` | Modify | Requirements 1 and 2 |

## Open Questions

None. Question 1, bash and fish, ruled 2026-10-21: "fold them in" (Requirement 2).
