---
step: 54
title: "XDG anchors resolve to relative paths on Windows, and two packages disagree about it"
status: charter — chartered 2026-08-14; blocks windows-native-permissions phase 3.1
proof_run: TBD — must include a case with both HOME and USERPROFILE controlled
parent: ../../phase-8.md
---

# Step 54 — XDG anchors resolve to relative paths on Windows

**Status:** `charter` — chartered 2026-08-14 while planning
[windows-native-permissions](../../../windows-native-permissions.md) phase 3.1, which cannot proceed
over these anchors. **The fix is not assumed**; the shape of the correct answer depends on a
question about platform convention that this charter states rather than settles.

## The defect

`internal/cli/xdg.go` resolves every anchor from `os.Getenv("HOME")` with no Windows fallback:

```go
func ConfigHome() string { … return filepath.Join(os.Getenv("HOME"), ".config") }   // xdg.go:19
func DataHome()   string { … return filepath.Join(os.Getenv("HOME"), ".local", "share") }  // :27
func CacheHome()  string { … return filepath.Join(os.Getenv("HOME"), ".cache") }    // :35
func StateHome()  string { … return filepath.Join(os.Getenv("HOME"), ".local", "state") }  // :43
```

Windows does not define `HOME` for a native process — it defines `USERPROFILE`. So with the
matching `XDG_*` variable unset, `filepath.Join("", ".local", "state")` yields **`.local\state`**, a
*relative* path, and `DevloreStateHome()` becomes `.local\state\devlore` **resolved against the
process's working directory**. The run index, user config, and cache then land wherever the user
happened to be standing.

## Two packages already disagree

`pkg/signing/configHome()` (`signing.go:229`) resolves the same convention **correctly** — and its
doc comment claims a parity that does not hold:

```go
// configHome resolves the user config directory per the devlore XDG convention (XDG_CONFIG_HOME, else
// ~/.config on every platform — matching internal/cli's xdg helpers; [os.UserConfigDir] diverges on darwin).
	home, err := os.UserHomeDir()
	if err != nil { return "", fmt.Errorf("resolve home dir: %w", err) }
```

`os.UserHomeDir()` returns `%USERPROFILE%` on Windows and **errors** when it cannot resolve, rather
than joining onto `""`. So on a Windows host without `HOME`:

- `pkg/signing` writes the signing key under `%USERPROFILE%\.config\devlore\signing\`.
- `internal/cli` looks for config under `.config\devlore\` relative to the working directory.

**The same logical location resolves to two different places**, and the comment asserting they match
is what would keep a reader from checking. `internal/cli/selfinstall.go:490` also uses
`os.UserHomeDir()`, so the correct API is already in the package — just not in `xdg.go`.

## Why CI cannot see it

Every path that would expose this is masked:

- `cmd/writ/scenario_integration_test.go` sets `HOME` **and** `USERPROFILE` in the child environment.
- 65 test sites across 19 files set `XDG_*` directly with `t.Setenv`, so the fallback branch never
  runs.

A leg that runs on `windows-latest` has therefore stayed green over a defect that affects every real
Windows user who has not set `HOME`.

## Why this blocks phase 3.1

Phase 3.1 anchors **writable roots** at these paths. A root anchored at a relative path holds
authority over an arbitrary working directory while presenting as scoped — strictly worse than the
`os.WriteFile` call it replaces, because it *looks* confined. The anchors must be sound before
anything is anchored at them.

## Open questions — do not assume the answer

1. **What happens when the home directory cannot be resolved at all?** `os.UserHomeDir` errors;
   today's code silently produces a relative path. An error changes these accessors' signatures
   (they return bare strings) and ripples to every caller. A panic is available under the
   repository's assert convention. Neither is obviously right, and the choice is a contract change.
2. **Should Windows use XDG-style paths at all?** The repository states an "XDG convention … on
   every platform", but that convention is stated, not tested, and Windows users expect
   `%LOCALAPPDATA%` / `%APPDATA%`. Re-examining it while fixing the platform is legitimate; keeping
   it is also legitimate. **Whichever is chosen, the residual must be named** — XDG-everywhere means
   Windows users get `~\.config`, native dirs mean the convention documented in `signing.go` and
   throughout the guides becomes platform-conditional.
3. **Is there data to migrate?** If any Windows user has run these tools without `HOME`, their state
   sits in a cwd-relative directory. Whether to detect and migrate it, or to leave it, is a decision
   about installed-base impact that nobody has looked at.
4. **How far does the divergence go?** `signing` and `cli` are the two found by inspection. The
   enumeration must cover every home-directory resolution in the repository rather than these two.

## Exit criteria

- [ ] Every home-directory resolution in the repository enumerated, not sampled.
- [ ] No anchor can resolve to a relative path — proved by a test that controls **both** `HOME` and
      `USERPROFILE`, including the case where neither is set.
- [ ] The failure mode is decided and documented (error, assert, or defined fallback).
- [ ] `pkg/signing` and `internal/cli` resolve identically, or the doc comment claiming they match
      is corrected to describe what is actually true.
- [ ] The platform-convention question is answered with its residual recorded.

## Related

- [windows-native-permissions.md](../../../windows-native-permissions.md) — phase 3.1 blocks on this.
- [step 53](53-network-dependent-tests.md) — the other charter where CI's signal was misleading.
