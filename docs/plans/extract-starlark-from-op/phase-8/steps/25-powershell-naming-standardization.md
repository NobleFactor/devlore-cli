---
step: 25
title: "PowerShell naming standardization"
status: not-started — standard settled 2026-06-17; awaiting go-ahead + own branch
proof_run: n/a (not started)
parent: ../../phase-8.md
---

# Step 25 — PowerShell naming standardization

**Status:** `not-started`. The naming standard is settled (user decision, 2026-06-17); implementation has not begun and
belongs on its own branch, separate from the phase-8 audit.

## The standard — same word, four roles

devlore supports **PowerShell 7+** (the cross-platform shell + scripting language + configuration-management framework,
executable `pwsh`). It does **NOT** support **Windows PowerShell** (5.x, executable `powershell.exe`). The word is
disambiguated by usage role:

| Role | Canonical | Note |
|---|---|---|
| **Executable** (the binary `exec` invokes) | `pwsh` | Hard-require on **every** platform. **Drop all Windows-PowerShell fallbacks** — this is a capability change, not a rename. |
| **Go package** (and `ProviderReceiverType.Name()`) | `powershell` | The provider package `pkg/op/provider/powershell` is **kept** — do not rename to `pwsh`. |
| **Completions directory** | `powershell` | Install path `share/powershell/completions`. |
| **Product / prose** (docs, comments where the product name is meant) | `PowerShell` | Not the exe/package/dir spelling. |
| **Arbitrary literal** (no PowerShell meaning) | leave | e.g. `"powershell"` as item data in `.star` gather fixtures. |

Blast radius: ≈65 `powershell` occurrences across ≈20 files (2026-06-17).

## Change-set

### A. Executable → `pwsh`; drop Windows-PowerShell fallbacks (behavior change)

- `internal/pwsh/pwsh.go:181` — remove the `exec.LookPath("powershell")` fallback in `findPowerShell()`; require `pwsh`,
  error if absent.
- `internal/credentials/helper.go:33` / `:155` / `:174` / `:187` (+ the `case "powershell"` arms at `:47` / `:61` /
  `:75`) — the credential helper detects and runs Windows PowerShell; switch to `pwsh`, drop the `powershell` branch.
- `pkg/platform/windows_managers_windows.go:279` — `exec.CommandContext(…, "powershell", "-Command", …)` → `pwsh`;
  refresh the `runWindowsCommand` doc at `:262`.

### B. Go package → `powershell` (no rename of the provider)

- `pkg/op/provider/powershell` is already correct (package + `ProviderReceiverType.Name()=="powershell"`); the generated
  `powershell` references in `*.gen.go` / inventory follow it. No change.

### C. Completions directory → `powershell`

- `cmd/star/cli/selfinstall.go:330` — `share/pwsh/completions` → `share/powershell/completions`.
- `internal/cli/selfinstall.go` — the same fix (twin copy).
- `cmd/star/cli/selfinstall_test.go:51` (+ the `internal/cli` twin) — shell-selector **key** `"powershell"` → `"pwsh"`;
  the expected dir `share/powershell/completions` is already correct.

### D. Product / prose → `PowerShell`

- Doc-comment and documentation mentions where the **product** is meant — e.g. the `pkg/op/provider/powershell/provider.go`
  header and `pkg/op/provider/shell/provider.go:7` — read `PowerShell`, not the exe/package/dir spelling.

### E. Leave

- `"powershell"` as arbitrary item data in `.star` gather fixtures; generated `powershell` package references.

## Relationship to step 18

Group C closes the `TestShellCompletionPath/powershell` red enumerated in [step 18](18-resolve-test-failures.md): the
impl's directory `share/pwsh/completions` is wrong (→ `share/powershell/completions`) and the test's shell key
`"powershell"` is wrong (→ `"pwsh"`). Both are corrected here.

## To settle at implementation (the standard is uniform; these are the edges)

1. **Ownership of group A.** Dropping the Windows-PowerShell fallbacks removes functionality (platform / credentials
   code) — outside the terminology-tidying lane. Confirm whether this step's group A is mine to make or belongs to the
   owner of that code.
2. **`internal/pwsh` package name.** It is package `pwsh`; does the Go-package rule (`powershell`) rename it, or does
   `internal/pwsh` stay?
3. **Shell-selector key = `pwsh`.** Interpreted from the existing keys (bash / fish / pwsh / zsh are exe names) and
   "executable name = `pwsh`"; confirm.

## Exit

Standard applied across all five roles; `TestShellCompletionPath` green; `pwsh` required on every platform with no
Windows-PowerShell invocation path remaining; full `make test` green (shared with the step-20/23 PR gate).
