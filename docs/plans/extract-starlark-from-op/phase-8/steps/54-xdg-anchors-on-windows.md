---
step: 54
title: "XDG anchors resolve to relative paths on Windows, and two packages disagree about it"
status: complete — chartered 2026-08-14, exit criteria and open questions all closed 2026-08-15
proof_run: TBD — must include a case with both HOME and USERPROFILE controlled, including neither set
parent: ../../phase-8.md
---

# Step 54 — XDG anchors resolve to relative paths on Windows

**Status:** `charter` — chartered 2026-08-14 while planning
[windows-native-permissions](../../../windows-native-permissions.md) phase 3.1, which cannot proceed
over these anchors. The layout, the home-resolution ladder, and single ownership were **ruled the
same day**, after researching current practice rather than assuming it. The enumeration is done and
**tripled the step**: 30 resolutions across 12 files, ~18 defective, with `cmd/writ`'s deploy target
the most severe. What remains open is the owning package's *name* and whether any cwd-relative data
already written needs migrating.

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

## Enumerated 2026-08-14 — the defect is far wider than `xdg.go`

The charter said enumerate rather than sample, and doing so found **30 home resolutions across 12
files**, of which **about 18 carry the defect**:

| Where | Sites | Shape |
| --- | --- | --- |
| `cmd/writ` | **10** | bare `os.Getenv("HOME")` |
| `internal/cli/xdg.go` | 4 | bare `os.Getenv("HOME")` |
| `internal/cli/selfinstall.go` | 3 | `os.UserHomeDir()` then a bare `HOME` fallback |
| `cmd/lore/lore/builder.go` | 1 | `os.UserHomeDir()` then a bare `HOME` fallback |
| `signing` ×2, `sops`, `gitignore`, `credentials`, `lorepackage`, `star` ×2, `writ/identity` | 9 | `os.UserHomeDir()` with error handling — correct as far as rung 2, still short of the ladder |

**The most severe site is not the run index.** `cmd/writ/writ/config.go` sets
`cfg.TargetRoot = os.Getenv("HOME")` at **four** separate sites (lines 78, 130, 191, 242), and
`writ` *deploys dotfiles into that root*. On Windows without `HOME`, `TargetRoot` is `""` — so a
deploy targets the **working directory**. State landing in the wrong place is a nuisance; a deploy
unpacking someone's dotfiles into whatever directory they happened to be standing in is not.

`cmd/writ/writ/commands.go` and `adopt/batch.go` additionally implement `~` expansion off the same
bare read, so a tilde in user input expands to nothing rather than to a home directory.

### It was three implementations, not two

Migrating `cmd/writ` (2026-08-14) found a **third**: `cmd/writ/writ/deploy/templatedata.go` carried its own
`xdgPath(envVar, defaultPath)` helper feeding `{{.ConfigHome}}`, `{{.DataHome}}`, `{{.StateHome}}` and
`{{.CacheHome}}` to **user templates**. It was the weakest of the three — it accepted any non-empty value,
relative included, and built its defaults on the bare `HOME` — so a relative `XDG_*` would have been rendered
straight into a user's generated dotfiles. Deleted; those four now read `xdg.ConfigHome()` and friends.

Worth recording as method: the original "two packages disagree" came from reading imports. The third had no
import to find — it was a local helper. Only migrating the code surfaced it.

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

## Ruled 2026-08-14: **one place for XDG locations, one way to get home**

**A single package owns both.** There is exactly one place to ask for an XDG location and exactly one
way to resolve home; no other package resolves home for itself. That rules out `internal/cli` as the
owner — `pkg/signing`, `pkg/sops`, `pkg/gitignore`, `internal/credentials` and `internal/lorepackage`
all need it, so `pkg/*` would be importing `internal/cli`, and [phase 7](../../../windows-native-permissions.md)
moves that package to `cmd/internal/cli` where `pkg/*` could not import it at all. The owner is a
low-level package every layer may depend on.

Then two obligations, in order.

**First, find home** — by a ladder, because a single source cannot deliver "no failure". There is no
degraded mode at any rung: joining onto an empty string to produce a relative path, which is what
the code does today, is not a fallback but a defect wearing one.

| Rung | Source | Notes |
| --- | --- | --- |
| 1 | `XDG_<ROLE>_HOME`, **if absolute** | A relative value is **invalid and ignored**, per the spec (quoted below). It does not fall through to rung 2 as a *value* — it is discarded, and resolution continues as though unset. |
| 2 | `os.UserHomeDir()` | The environment answer, under **the platform's own variable name** — `%USERPROFILE%` on Windows, `$HOME` on Unix, `$home` on plan9. It is a `switch runtime.GOOS`, so it reads exactly one name per platform and never the others. |
| 3 | `os/user.Current().HomeDir` | The **operating system's** answer, independent of the environment. |
| 4 | — | Nothing left. Assert. |

**Why rung 3 exists, and why it is not redundant.** `os.UserHomeDir` is a `Getenv` with a nicer name
(`os/file.go:605`): it reads `HOME` / `USERPROFILE` / `home`, special-cases android and ios, and
otherwise returns its single error, a bare `errors.New("$HOME is not defined")` — no sentinel,
nothing `errors.Is`-able. It never asks the operating system anything, and `%USERPROFILE%` is set
for interactive logons but not guaranteed for services, scheduled tasks, or some CI contexts.
`os/user` does ask:

- **Windows** — `Current()` reads the process token's profile directory via
  `GetUserProfileDirectory` (`os/user/lookup_windows.go:278`), a real OS call rather than an
  environment variable. A registry path (`ProfileList\<SID>\ProfileImagePath`) backs lookups by uid.
- **Unix with cgo** — `current()` → `lookupUnixUid(syscall.Getuid())` → the passwd entry.
- **Unix without cgo** — and this repository cross-compiles with `CGO_ENABLED=0` — `lookup_stubs.go`
  tries `/etc/passwd` first and then **falls back to `os.UserHomeDir()` itself**. On that build rung
  3 collapses into rung 2, so it adds nothing there. It is still worth carrying: it is the rung that
  saves Windows, which is the platform this step exists for.

**Rung 3 is mandatory, not a nicety.** When the platform's home variable is undefined, we ask the
**operating system's user account**, because that is the authoritative answer and the environment is
merely a hint someone can fail to set.

**Our code never names a home variable.** `os.UserHomeDir` already dispatches on `runtime.GOOS` to
the correct one, so naming `HOME` ourselves is what every defective site above has in common. After
this step, the names appear in exactly two places: documentation, and any diagnostic reporting what
was tried.

**Rung 4 is an assert, not an error.** Reaching it requires no home variable, no passwd entry, and no
cgo simultaneously — an environment in which no anchor of any kind can be honored. Threading an error
up from there would change every accessor's signature to describe a state in which the program
cannot function anyway.

**What the specification actually says.** It is **silent** on `$HOME` being unset — every default is
written as `$HOME/.local/share` with no guidance when that is unavailable, which is why the ladder
above is ours to define rather than to quote. It does rule on the shape of the present bug:

> All paths set in these environment variables must be absolute. If an implementation encounters a
> relative path in any of these variables it should consider the path invalid and ignore it.

That is written about `XDG_*`, not `$HOME`, but it settles rung 1 exactly and states the principle
the current code violates: **a relative anchor is invalid and must be ignored, never used.**

**Then, default to the XDG standard directory names rooted there** — `~/.config/devlore`,
`~/.local/share/devlore`, `~/.local/state/devlore`, `~/.cache/devlore` — with the `XDG_*` variables
overriding when set, on every platform, as they already do. The *names* are XDG's; the *root* is
home. **Windows gets no special case.** The objective is that people working across platforms do not
sweat Windows-isms, and this is the layout that delivers it.

### The evidence this was decided on

Two live cohorts, both defensible in 2026, and **no migration momentum in either direction**:

| | Approach | Examples |
| --- | --- | --- |
| **Home-rooted** | dotdirs under `%USERPROFILE%` | git (`.gitconfig`, plus `XDG_CONFIG_HOME`), **Microsoft's own OpenSSH port** (`%USERPROFILE%\.ssh\`), Docker CLI, kubectl, aws, cargo, **Starship** and **WezTerm** (`~/.config` on Windows, explicitly for cross-platform sameness) |
| **Known-Folder** | XDG *roles* mapped to AppData | Neovim (`%LOCALAPPDATA%\nvim`), gh (`%AppData%\GitHub CLI`), and the three dominant cross-platform directory libraries — Go `adrg/xdg`, Python `platformdirs`, Rust `dirs`/`directories` |

Three findings settled it:

1. **Nothing treats `%APPDATA%` as a substitute for `$HOME`.** The Known-Folder cohort maps the four
   *roles* onto native folders; none of them writes `%APPDATA%\.config`. So "use AppData" would not
   have simplified anything — it would have replaced one home with four unrelated destinations.
2. **Microsoft's own OpenSSH keeps private keys in `%USERPROFILE%\.ssh`** — same platform, same
   sensitivity, same vendor as the Known Folder API. That is the closest precedent to this
   repository's signing key, and it points home-ward.
3. **The migration debates went nowhere.** [starship#896](https://github.com/starship/starship/issues/896)
   (asking for native dirs) and [neovim#24009](https://github.com/neovim/neovim/issues/24009)
   (asking to move `%LOCALAPPDATA%` → `%APPDATA%`) are both still open and unimplemented. Projects
   keep what they started with, so this is a free choice rather than swimming against a tide.

**Roaming is a question this ruling avoids entirely.** `%APPDATA%` syncs at logon in domain
environments and `%LOCALAPPDATA%` does not, so anyone choosing AppData must answer
roaming-vs-local per role — note `adrg/xdg` sidesteps it by putting even *config* in Local. A dotdir
under `%USERPROFILE%` never raises the question.

### Accepted residuals

1. **`~\.local\state` looks alien on Windows.** Nothing else on the machine writes there, and
   Explorer does not hide dot-directories, so `.config`, `.local` and `.cache` are visible in the
   profile folder. This is precisely why the Known-Folder cohort chose AppData.
2. **No roaming.** A domain user's config does not follow them between machines.
3. **We are choosing the Starship/WezTerm shape over the Neovim/gh shape.** Both are current; the
   tiebreaker is the cross-platform-sameness objective, which picks this one unambiguously.

## Open questions — still to answer

1. ~~**Package name for the single owner.**~~ **Answered: `pkg/xdg`** (#419).
2. ~~**Is there data to migrate?**~~ **Closed — nothing to migrate.**
3. ~~**How far does the divergence go?**~~ **Answered by the enumeration**: 30 sites in 12 files,
   eleven separate hand-rolled resolvers, all now on `pkg/xdg`.

## Exit criteria

- [x] Every home-directory resolution in the repository enumerated, not sampled — **30 sites in 12
      files, ~18 defective**; see the enumeration above.
- [x] All 30 route through the one package: no other package resolves home or names an XDG location.
      **Closed 2026-08-15**, verified by a grep for `os.UserHomeDir`, `HOME`, `USERPROFILE`,
      `user.Current` and `XDG_*` across `cmd/`, `internal/` and `pkg/` that returns nothing outside
      `pkg/xdg` — see the closing note below for the one deliberate exception.
- [x] `cmd/writ`'s `TargetRoot` and `~` expansion fixed — the most severe sites, and the reason this
      step is no longer just about `xdg.go`. `TargetRoot` landed in #420; `identity.expandPath`
      followed 2026-08-15.
- [x] Discarded relative `XDG_*` values carry the `diagnose-ignored-error` marker, so they enumerate with
      every other pending diagnostic (2.8 §"Ignored errors are diagnostics" → "Discarded values owe the same
      debt"). The specification requires the warning; the stream does not exist yet, so the debt is recorded
      rather than paid. Carried on `xdg.base`.
- [x] No anchor can resolve to a relative path — proved by a test that controls **both** `HOME` and
      `USERPROFILE`, including the case where neither is set. `TestNoAnchorIsEverRelative` does
      exactly this: `homeVariable()` returns `USERPROFILE` on Windows and `HOME` elsewhere, the test
      clears it, and sets every `XDG_*` to a relative value.
- [x] The failure mode is decided: the four-rung ladder above, with an assert only at rung 4.
- [x] `pkg/signing` and `cmd/internal/cli` resolve identically — both now call `pkg/xdg` and neither
      names a home variable. `signing.configHome` is deleted, and with it the doc comment claiming a
      parity with `internal/cli` that did not hold.
- [x] The platform-convention question is answered with its residuals recorded (see the ruling).

## Closing note — 2026-08-15

**What the migration removed.** Eleven hand-rolled resolvers, each a partial copy of the ladder:
`internal/cli/xdg.go`'s four base accessors, `signing.configHome`, `sops.xdgConfigPath`,
`lore.userHomeDir`, `lorepackage.defaultCacheDir`, `star.userConfigPath`, star's extension-path
block, and `credentials.credentialsPath`. Five of them accepted a **relative** `XDG_*` value, which
the specification says to ignore; none had the Windows fallback.

**Four signatures got simpler**, because a resolver that cannot fail owes no error:
`credentialsPath`, `defaultCacheDir`, `userConfigPath` and `resolveGlobalIgnore`'s helpers dropped
their error returns, and the dead `!= ""` guards at their call sites went with them.

**`pkg/xdg` gained `UserHomePath`** — the home-directory analog of `ConfigPath` and its siblings, for
the locations the specification does not name (`~/.ssh`, `~/.local`, `~/.gitconfig`, and the tail of
any path a user wrote with a leading `~`). Every `filepath.Join(home, …)` now goes through it.

**The one deliberate exception** is `deploy/templatedata.go`'s `user.Current()`, which reads the
account's **username** for template data, not its home directory. Home in that file already comes
from `xdg.UserHomeDir()`.

**One package, one way.** `pkg/xdg` is the only code in the repository that resolves a home
directory, and it never names `HOME`: `os.UserHomeDir` answers, `user.Current` answers when the
environment is silent, and an assert catches an environment where neither can. Every XDG location in
the repository is reached through its accessors.

## Related

- [windows-native-permissions.md](../../../windows-native-permissions.md) — phase 3.1 blocks on this.
- [step 53](53-network-dependent-tests.md) — the other charter where CI's signal was misleading.
