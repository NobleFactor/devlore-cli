---
step: 28
former_step: 25
title: "PowerShell naming standardization"
status: complete (2026-07-15) — landed on the phase-8 branch by user direction (the own-branch note superseded); groups A–E realized, the TestShellCompletionPath red closed from both sides; make test's FAIL set is now the step-33 writ builds only
proof_run: 2026-07-15 (make test — cmd/star/cli, internal/cli, internal/credentials, internal/pwsh, pkg/platform green)
parent: ../../phase-8.md
---

# Step 28 — PowerShell naming standardization

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

## Settled at implementation (2026-07-15)

1. **Ownership of group A** — resolved by direction: the user directed step 28 wholesale ("do the change on this
   branch"), covering the capability change.
2. **`internal/pwsh` package name** — **kept `internal/pwsh`.** It names the executable it locates and wraps (the exe
   role), while the Go-package rule in the table targets the provider (`pkg/op/provider/powershell`, untouched). If a
   rename is wanted later it is mechanical.
3. **Shell-selector key = `pwsh`** — confirmed and implemented: the selector arms and both test twins key on `pwsh`;
   detection is `LookPath("pwsh")` only.

## Landed (2026-07-15)

- **A (capability change):** `internal/pwsh/pwsh.go` `findPowerShell` requires `pwsh` (fallback deleted);
  `internal/credentials/helper.go` detects/keys/invokes `pwsh` (three case arms + three exec calls);
  `pkg/platform/windows_managers_windows.go` elevated path invokes `pwsh`; `internal/cli/selfinstall.go` detection is
  `pwsh`-only. No Windows-PowerShell invocation path remains.
- **C (completions + the red test):** `cmd/star/cli/selfinstall.go` dir → `share/powershell/completions`;
  the selector key is `pwsh` in both impl twins; both test twins re-keyed (`selfinstall_test.go` star + internal,
  including the `validShells` set). `TestShellCompletionPath` green.
- **B / D / E:** provider package untouched (already `powershell`); prose already reads `PowerShell`; package-manager
  identifiers (`brew install powershell`, apt `powershell`) and provider action names left as-is by role.

## Exit — met 2026-07-15

Standard applied across all five roles; `TestShellCompletionPath` green; `pwsh` required on every platform with no
Windows-PowerShell invocation path remaining; `make test`'s FAIL set is the step-33 writ builds only.
