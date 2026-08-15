---
step: 54
title: "XDG anchors resolve to relative paths on Windows, and two packages disagree about it"
status: charter — chartered 2026-08-14; layout and home-resolution ladder RULED the same day; blocks windows-native-permissions phase 3.1
proof_run: TBD — must include a case with both HOME and USERPROFILE controlled, including neither set
parent: ../../phase-8.md
---

# Step 54 — XDG anchors resolve to relative paths on Windows

**Status:** `charter` — chartered 2026-08-14 while planning
[windows-native-permissions](../../../windows-native-permissions.md) phase 3.1, which cannot proceed
over these anchors. The layout and the home-resolution ladder were **ruled the same day**, after
researching current practice rather than assuming it; what remains open is migration of any
cwd-relative data already written, and the full enumeration of home resolutions across the
repository.

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

## Ruled 2026-08-14: **we find home; we default to XDG standard directory names rooted at home**

Two obligations, in that order.

**First, find home** — by a ladder, because a single source cannot deliver "no failure". There is no
degraded mode at any rung: joining onto an empty string to produce a relative path, which is what
the code does today, is not a fallback but a defect wearing one.

| Rung | Source | Notes |
| --- | --- | --- |
| 1 | `XDG_<ROLE>_HOME`, **if absolute** | A relative value is **invalid and ignored**, per the spec (quoted below). It does not fall through to rung 2 as a *value* — it is discarded, and resolution continues as though unset. |
| 2 | `os.UserHomeDir()` | The environment answer: `%USERPROFILE%` on Windows, `$HOME` elsewhere. |
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

**Rung 4 is an assert, not an error.** Reaching it requires no `$HOME`, no passwd entry, and no cgo
simultaneously — an environment in which no anchor of any kind can be honored. Threading an error
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

1. **Is there data to migrate?** If any Windows user has run these tools without `HOME`, their state
   sits in a cwd-relative directory. Whether to detect and migrate it, or to leave it, is a decision
   about installed-base impact that nobody has looked at.
3. **How far does the divergence go?** `signing` and `cli` are the two found by inspection. The
   enumeration must cover every home-directory resolution in the repository rather than these two.

## Exit criteria

- [ ] Every home-directory resolution in the repository enumerated, not sampled.
- [ ] No anchor can resolve to a relative path — proved by a test that controls **both** `HOME` and
      `USERPROFILE`, including the case where neither is set.
- [x] The failure mode is decided: the four-rung ladder above, with an assert only at rung 4.
- [ ] `pkg/signing` and `internal/cli` resolve identically — which the ruling above makes achievable
      by making `internal/cli` match what `signing` already does, rather than the reverse.
- [x] The platform-convention question is answered with its residuals recorded (see the ruling).

## Related

- [windows-native-permissions.md](../../../windows-native-permissions.md) — phase 3.1 blocks on this.
- [step 53](53-network-dependent-tests.md) — the other charter where CI's signal was misleading.
