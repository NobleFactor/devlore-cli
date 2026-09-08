---
title: "The Makefile finds Git for Windows' bash itself when run from PowerShell or cmd"
issue: https://github.com/NobleFactor/devlore-cli/issues/865
status: in-progress
created: 2026-09-08
updated: 2026-09-08
---

# Plan: The Makefile finds Git for Windows' bash itself when run from PowerShell or cmd

## Summary

`make install` on DANOBLE-WD11-3 from PowerShell died in cmd.exe on its first recipe, because the Makefile
says `SHELL := bash` and Git's `bin` was not on that machine's PATH. The same command built under Git Bash.
Ruled 2026-09-08: the build system makes the choice on Windows. Before anything else in the Makefile, on
`Windows_NT`, `SHELL` is set to Git for Windows' bash by its space-free 8.3 path unless a `bash` is already
reachable; a machine without Git for Windows gets an error that names the remedy. Proved on the machine that
failed, from a PowerShell session, without touching its PATH.

## Goals

1. **`make` works from PowerShell and cmd on a Windows machine with Git for Windows installed**, PATH or no PATH.
2. **Nothing changes anywhere else**: Git Bash, macOS, Linux, CI's Windows legs (already `shell: bash`).
3. **A machine without Git for Windows is told so** in one line, instead of failing on `'GOOS' is not recognized`.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `Makefile:4-6` | `SHELL := bash`, `.SHELLFLAGS`, `.ONESHELL:` | POSIX recipes throughout; `HOST_GO` is an env-prefix form |
| Windows from PowerShell/cmd without Git's `bin` on PATH | fails at `inventory` | `'GOOS' is not recognized as an internal or external command` |
| Windows from Git Bash | works | 580s for `make install` on arm64, measured 2026-09-08 |
| CI Windows legs | `shell: bash` | unaffected either way |

## Requirements

### The shell selection, before `SHELL := bash`

```make
ifeq ($(OS),Windows_NT)
  # GNU make's SHELL cannot carry a space, so Git for Windows' bash is named by its 8.3 path.
  GIT_BASH := $(firstword $(wildcard C:/PROGRA~1/Git/bin/bash.exe C:/PROGRA~1/Git/usr/bin/bash.exe \
                                     $(subst \,/,$(LOCALAPPDATA))/Programs/Git/bin/bash.exe))
  ifneq ($(GIT_BASH),)
    SHELL := $(GIT_BASH)
  else ifeq ($(wildcard C:/Program?Files/Git/bin/bash.exe),)
    $(error This Makefile runs under bash. Install Git for Windows, or put a bash on PATH)
  endif
endif
SHELL ?= bash
```

`make` evaluates this before any recipe or `$(shell)`, so `HOST_GOOS := $(shell go env GOHOSTOS)` and every
recipe run under the chosen bash. When `bash` is already on PATH — a Git Bash session, or after personal#176 —
the 8.3 path is still what runs on Windows, which is the same binary. On macOS and Linux `$(OS)` is empty
and the block is skipped.

## Implementation Phases

### Phase 1: the Makefile

- [x] the block above at the top of the Makefile, with its comment: what fails without it, verbatim
- [x] **Acceptance:** `make inventory` from a PowerShell session on DANOBLE-WD11-3 with Git's `bin` absent
      from PATH: exit 0 in 9s, both inventories written, recipes running as `GOOS=windows GOARCH=arm64`;
      the same from Git Bash; `make help` and `make -n inventory` on this Mac unchanged (2026-09-08)

### Phase 2: the record

- [x] personal#176 re-scoped: the PATH declaration is for the prompt, not the build (done 2026-09-08)
- [x] the Windows-machine memory names the fix

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `Makefile` | Modify | the shell selection on Windows |

## Decisions

- **The 8.3 path, not a quoted one.** GNU make hands `SHELL` to the OS as a bare string; `C:\Program Files\…`
  splits. `PROGRA~1` exists on every default `C:` volume, and a volume with 8.3 names disabled falls into the
  error branch with the remedy.
- **Detect, don't require PATH.** The build's prerequisites are the build's to find; PATH declarations are for
  people at prompts.
- **Two red lines per recipe on Windows from PowerShell are Git for Windows', and stay.** Their bash is
  patched to source `/etc/bash.bashrc` even for `bash -c`, and that file's line 13 reads
  `${CYG_SYS_BASHRC}` without a default, so under this Makefile's long-standing `-o nounset` it prints
  `CYG_SYS_BASHRC: unbound variable` and returns; the recipe then runs correctly. `--norc` would silence
  it by not running their rc, and defining the variable would be the same workaround in another coat —
  both set aside 2026-09-08 ("you seem to be fixing a problem by disabling a feature … don't do that").
  The fix is upstream's, one default, `${CYG_SYS_BASHRC:-}`, in MSYS2's `filesystem` package.
- **Prove it where it failed.** The Makefile is copied to DANOBLE-WD11-3's checkout and run from PowerShell,
  then the checkout is restored; CI's Windows legs cannot show this, since they already run under bash.

## Related Documents

- Issue #865 — this chore; feature #91, epic WindowsCampaign
- personal#176 — the PATH declaration, re-scoped
- `.github/workflows/ci.yaml` — `shell: bash` on the Windows legs

## Open Questions

- [ ] Whether to file the rc's `nounset`-unsafety with Git for Windows / MSYS2. The draft is one paragraph.
