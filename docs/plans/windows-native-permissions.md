---
title: "Windows native permissions: enforce restrictive modes, route every mutation through fsroot"
issue: https://github.com/NobleFactor/devlore-cli/issues/405
status: draft
created: 2026-08-13
updated: 2026-08-14
---

# Plan: Windows native permissions

Go's `os.Chmod` on Windows toggles only the read-only attribute, so **every restrictive mode this
codebase requests is silently unenforced there**. A `0600` private key lands accessible to anyone
with directory access. This plan makes Windows permissions real, and does it at a single seam by
routing every filesystem mutation through `fsroot`.

## Summary

Discovered while triaging the #373 windows leg: seven test failures reading `permission = 666,
want 600` are not "Unix-only assertions" — they are the product silently failing to honor its own
intent. Enumeration then showed the surface is far larger than the failures: **84 direct `os.*`
mutation calls** live outside `fsroot`, **31** of them passing a restrictive permission.

The worst is not the case originally filed. It is
[pkg/signing/signing.go:202](../../pkg/signing/signing.go) — a **private key** written at `0600`
into a `0700` directory, both unenforced on Windows.

## Rulings (2026-08-13)

1. **Windows permissions must be enforced.** Native support, not documented resignation. This
   supersedes the earlier "file an issue and scope the tests to Unix" disposition; the seven
   permission failures are reclassified **bucket 1 (product defect)**, not bucket 3.
2. **Every mutation goes through `fsroot`** — all 84 sites, test harnesses
   (`internal/e2e`, `cmd/devlore-test/devloretest`) and the build tool (`internal/tools/docgen`)
   included. **Amended the same day** (see ruling 6): the rule is not "no exemptions" but "no
   *unjustified* exemptions" — a direct `os.*` mutation is permitted only when it carries a
   `// Confinement:` comment stating why, which is mechanically checkable and cannot be added
   silently. The amendment came from the code: reading the 13 provider sites found five with
   principled reasons to bypass, three of which **already carried `// Confinement:` comments**.
   The convention existed; the guard adopts it.
3. **Mechanism: `golang.org/x/sys/windows`.** Already a direct dependency (v0.41.0), and
   `cmd/writ/writ/snapshot/protect_darwin.go` already sets the precedent of platform-specific file
   protection via `x/sys`. No shelling out to `icacls.exe` — no PATH dependency, no locale-sensitive
   output parsing.
4. **Mapping: private-vs-not.** When the requested mode denies **other** (`perm&0o007 == 0`), write
   a **protected** DACL (inheritance broken) granting the file owner, plus `SYSTEM` and
   `Administrators`. Otherwise leave the inherited default. Restricting SYSTEM and Administrators is
   theater — they can take ownership at will — and breaks backup and AV tooling for no defensive
   gain.

   **Corrected 2026-08-13** — as first written this ruling said `perm&0o077 == 0` while also
   claiming `0640` and `0600` were indistinguishable; those cannot both be true, and a test caught
   the seam. The boundary is **other**, not **group and other**: `0600`, `0640` and `0660` are all
   private on Windows. We can express "other is excluded"; we have no group principal to grant to,
   so the group bit is inexpressible and collapses into the owner. The rejected reading failed
   **open** — a file its author restricted to a group would have inherited a DACL readable by
   everyone, which is the failure class this issue exists to eliminate. Accepted consequence: a
   `0660` file intended for genuine group collaboration becomes owner-only on Windows.
5. **`file.Observe` reports the real mode plus an access fact.** `Mode.Perm()` stays truthful to
   what the OS reports (`0666`); a separate observed fact — `restricted`, derived by reading the
   DACL back — carries the enforcement state. Nothing is fabricated. Accepted consequence: a
   cross-platform mode comparison still sees `0666` vs `0600` for the same logical state unless it
   learns the new field.

6. **`fsroot.Root` reaches full `*os.Root` parity.** The interface exposes every method `*os.Root`
   provides; nine are missing today — `Chmod`, `Chown`, `Chtimes`, `Create`, `Lchown`, `Link`,
   `Mkdir`, `OpenRoot`, `RemoveAll`. Part of the 84 are not philosophical bypasses at all but code
   routing around a too-small interface: `os.RemoveAll` alone has **10** call sites with no
   `fsroot` equivalent. A guard that demands justification must first make the justified path
   available.
7. **`fsroot.OpenScratch` — scratch is a `Root` confined to a temp directory.** Not an
   `os.CreateTemp` wrapper: a `fsroot.Root` anchored at a freshly created temp directory, whose
   `Close` removes the tree. This keeps the package's meaning intact (scratch is not an escape
   from confinement; it is its own confined tree with a self-destroying lifetime), removes the
   most common legitimate reason to reach for `os.*`, and brings scratch under the Windows ACL
   work automatically — which matters because scratch holds the most sensitive *transient* data
   (spooled archives, and decrypted plaintext if it ever spools).

**Load-bearing detail:** breaking inheritance is the whole game. Without a *protected* DACL the
file retains its parent's inherited ACEs and stays accessible no matter what is granted.

**Proof the scratch abstraction is wanted:** it already exists twice, hand-rolled.
`archive.spooledZipReader` is a type whose entire purpose is pairing a `Close` with a `Remove`,
and `cmd/devlore-test/devloretest/runner.go:232` hand-wires `os.MkdirTemp` + `defer os.RemoveAll`
+ `fsroot.OpenWritableUnconfined(tmpDir)` — a scratch root assembled from three parts.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Restrictive modes on Unix | ✅ honored | `os.WriteFile` / `Chmod` do what is asked |
| Restrictive modes on Windows | ❌ silently ignored | `Chmod` toggles read-only only; files report `0666` |
| Single write seam | ❌ absent | 84 mutation calls bypass `fsroot` |
| `fsroot.Root.Chmod` | ❌ missing | interface has no Chmod; `selfinstall.go:569` needs one |
| Enforcement observability | ❌ none | `Observe` cannot report Windows access state |
| Lint guard | ❌ none | nothing prevents a new direct `os.*` site |

### The 84 sites, by package

| Package | Sites | Package | Sites |
| --- | --- | --- | --- |
| `internal/cli` | 28 | `cmd/writ/writ/snapshot` | 3 |
| `cmd/star/config` | 6 | `cmd/star` | 3 |
| `cmd/devlore-test/devloretest` | 6 | `internal/tools/docgen` | 2 |
| `cmd/star/provider/setup` | 5 | `internal/registry` | 2 |
| `cmd/writ/writ` | 4 | `internal/e2e` | 2 |
| `pkg/signing` | 3 | `internal/document` | 2 |
| `pkg/op/provider/archive` | 3 | `cmd/writ/writ/migrate` | 2 |
| `internal/lorepackage` | 3 | `cmd/star/provider/lint` | 2 |
| `pkg/sink`, `pkg/op/provider/plan`, `pkg/op/provider/git`, `internal/credentials` | 1 each | | |

31 of the 84 pass a restrictive perm; the security-relevant ones are `pkg/signing` (private key +
its `0700` directory), `internal/cli` (state home `0700`, run index `0600`, user config `0600`,
self-install manifest `0600`), `internal/document` (`0600` writes), and the SOPS-decrypted output
reached via the file provider.

## Requirements

### R1 — `fsroot.Root` reaches full `*os.Root` parity

Nine methods are missing and all are thin delegations for `confinedRoot` (`r.inner.X(p.rel, …)`),
`os.*` calls for `unconfinedRootReaderWriter`, and `errReadOnly` for the read-only variant:

| Method | Call sites needing it today | Note |
| --- | --- | --- |
| `RemoveAll` | **10** | the largest single gap |
| `Create` | 4 | `OpenFile` covers it, but parity is the ruling |
| `Chmod` | 1 | **where Windows enforcement hooks for existing files** |
| `Chown` | 1 | pairs with the file provider's `applyChown` |
| `Chtimes`, `Lchown`, `Link`, `Mkdir`, `OpenRoot` | 0 today | parity per ruling 6 |

`OpenRoot(p Path) (Root, error)` returns a sub-root — confined for `confinedRoot`, a new
unconfined root at that path otherwise. `Chown`/`Lchown` fail on Windows by `os` design; that is
expected and must be asserted, not worked around.

This is a **sealed-interface change**: all three implementations live in `pkg/fsroot`, so the
change is mechanical, but it is a deliberate widening of the package's contract.

### R2 — Windows enforcement inside `fsroot`

A platform-split helper — `applyMode(path string, perm os.FileMode) error`, no-op on Unix (the
syscall already honored the mode), protected-DACL on Windows per ruling 4 — called from
`WriteFile`, `OpenFile` (on create), `Mkdir`/`MkdirAll`, and `Chmod`. Applies to directories as
well as files: `0700` state homes are in the restrictive set.

### R3 — migrate all 84 sites

Each site takes an `fsroot.Root` and calls its method. The sites that own no root today —
`internal/cli`'s 28 especially — need one plumbed in, most plausibly `OpenWritableUnconfined`
anchored at the relevant base (state home, config dir, install prefix). **Stated plainly:** an
unconfined writable root is `os.*` with a base path; the win is the single enforcement seam, not
confinement.

### R4 — the lint guard: mandatory `// Confinement:` justification

A direct `os.*` mutation outside `pkg/fsroot` is a finding **unless the call carries a
`// Confinement:` comment** explaining why the root cannot serve it. Six such justifications
already exist in the providers (one citing "§10 ruling 5"), so the guard formalizes a convention
rather than inventing one.

This is stronger than an exemption list: exemptions are granted once and forgotten, while a
justification is written at the call site, reviewed in the diff, and cannot be added silently.
`golangci-lint`'s `forbidigo` is the candidate mechanism (a `make check` grep is the fallback);
whichever is used must be able to see the comment. **Verify that before committing to
`forbidigo`** — if it cannot read adjacent comments, the grep fallback becomes the mechanism.

The guard cannot be switched on until R3 completes.

### R4a — `fsroot.OpenScratch`

`OpenScratch(pattern string) (Root, error)` — a `Root` anchored at a freshly created temp
directory; `Close` releases the handle **and removes the tree**. Retires `os.CreateTemp` (3 sites)
and `os.MkdirTemp` (1), the only calls in the inventory with no `*os.Root` equivalent, and folds
`archive.spooledZipReader` into the root itself.

Two residuals accepted with the ruling:

1. **`Close` carries a second meaning** here (release *and* delete) versus the other three modes.
   Overloading is the price of making cleanup un-forgettable; the alternative — a separate
   `Discard()` — restores leak-by-default, the exact failure #393 just cost a campaign to fix.
   Document it emphatically at the constructor and the method.
2. **A leaked scratch root is worse than a leaked temp file**, and worse specifically on Windows:
   the tree cannot be removed while any handle inside it is open, so #393's failure shape returns
   wearing new clothes. Same discipline, and now the same tests.

### R4b — the validation helper (`IsRestricted`), modeled on Win32-OpenSSH

**Prior art, researched 2026-08-13.** The two POSIX-on-Windows layers took opposite approaches, and
neither is the right model for us:

- **Cygwin** translates fully with `acl` mounts: owner → owner SID, group → group SID, other →
  `Everyone` (S-1-1-0), using deny *and* allow ACEs and **deliberately violating canonical ACE
  order** because "canonical ACLs are unable to reflect each possible combination of POSIX
  permissions." Upstream acknowledges the cost in a thread titled *"Cygwin's ACL handling is NOT
  interoperable with Windows."* It works because Cygwin maintains a POSIX-group-to-SID mapping —
  **which we do not have**, and inventing one would be policy dressed as fidelity.
- **MSYS2** declines to map at all: `noacl` by default, `chmod` silently ignored, permissions
  approximated from the DOS read-only attribute and the file extension. That is the
  "document the limitation" disposition already rejected for this work.
- **Win32-OpenSSH** — shipped and maintained by Microsoft, and facing our exact problem (protecting
  a private key on Windows) — requires that only the **owner, SYSTEM, and Administrators** have
  access, strictly enough to reject even an `OWNER RIGHTS` (S-1-3-4) entry. Decisively, it
  **validates rather than translates**: it reads the DACL and refuses an over-permissive key.

Adopt the validation half explicitly. `IsRestricted(p Path) (bool, error)` answers "is this object
actually private?" — on Windows by reading the DACL back (protected, and no trustee beyond owner /
SYSTEM / Administrators), on Unix by reading the mode bits. It is the mechanism R5's observed fact
reports, and it is what lets a future `writ doctor`-style check refuse to proceed over a secret
that is not actually protected, exactly as `ssh` refuses an unprotected key.

Note it departs from ruling 6's parity in the additive direction: `*os.Root` has no such method.
Parity means the interface *includes* everything `os.Root` offers, not that it may include nothing
else.

### R5 — `Observe` access fact

Add the DACL-derived `restricted` fact per ruling 5, and the read-back helper it needs.

### R6 — tests that prove enforcement

- `_windows_test.go`: read the DACL back and assert the ACE set — owner + SYSTEM + Administrators,
  inheritance protected — for a `0600` write, a `0700` mkdir, and a post-hoc `Chmod`.
- `_unix_test.go`: the existing mode-bit assertions, which are genuinely a Unix subject.
- Coverage therefore **increases** on both platforms rather than being scoped away.

## Implementation Phases

Ordering rationale: the API must exist before anyone can be asked to use it (1); enforcement must
be *proved* before call sites move, or the migration cannot be said to have achieved anything (2);
the security-critical sites come next so the gap closes at the earliest possible moment (3); the
remainder follows (4); and the guard lands last because it cannot pass until the migration is
complete (5).

### Phase 1: `fsroot` API — parity + scratch — branch `fsroot-parity` — status: complete (2026-08-13)

Pure API growth. No call site moved, no behavior changed, nothing Windows-specific.

- [x] R1: the nine missing methods — `Chmod`, `Chown`, `Chtimes`, `Create`, `Lchown`, `Link`,
      `Mkdir`, `OpenRoot`, `RemoveAll` — across all three implementations. `Root` goes 15 → 24
      methods and now mirrors `*os.Root` in full.
- [x] R4a: `OpenScratch` + the unexported `scratchRoot`, whose `Close` joins the handle release
      with `RemoveAll` of the tree. Both halves run even when the first fails: a failed close must
      not orphan the tree, and a failed removal must still be reported.
- [x] Tests: `TestParity_WritableRootsImplementEveryMutation` exercises all nine against both
      writable implementations; `TestParity_ReadOnlyRootRefusesEveryMutation` pins `errReadOnly`
      and that `OpenRoot`'s sub-root **inherits** read-only; `TestOpenScratch_ClosesAndRemovesTheTree`
      pins the lifetime contract; `TestOpenScratch_IsConfined` proves scratch is a confined root,
      not a temp-dir escape hatch.
- [x] `make test` zero failures; `make build-all`, `make vet-all`, `make lint-all` green on linux,
      darwin, **and windows**; gofmt clean; style detectors clean.

**Decisions taken during implementation, for the record:**

- `Link` takes two `Path` values (like `Rename`) because a hard link must resolve inside the root;
  `Symlink` keeps `target string` because a symlink target may legally dangle.
- `OpenRoot` inherits the parent's access mode — a confined root yields a confined sub-root, a
  read-only root a read-only one — so a sub-root can never silently widen access.
- `Create`'s doc states plainly that it produces an **unrestricted** file (`0o666` before umask)
  and must never be used for sensitive content. Under ruling 4 it takes the inherited-DACL branch,
  so it gets no Windows protection by design.
- `Chown`/`Lchown` failures on Windows are surfaced, not masked; the parity test asserts the
  platform split rather than skipping it.

### Phase 2: Windows enforcement inside `fsroot` — branch `fsroot-windows-acl` — status: implemented, CI-gated (2026-08-13)

- [x] R2: the `applyMode` platform split — a no-op on Unix (the syscall already honored the mode),
      a protected DACL on Windows per ruling 4 — wired into `WriteFile`, `OpenFile` (on
      `O_CREATE`), `Mkdir`, `MkdirAll`, and `Chmod` on both writable implementations.
- [x] R6's `_windows_test.go`: six tests that read the security descriptor **back** and assert
      `SE_DACL_PROTECTED` plus the trustee set, printing the SDDL on failure. Read-back via SDDL
      rather than an ACE walk — no `unsafe`, and a failure prints the ACL the object actually
      carries.
- [x] `isPrivateMode` covered cross-platform (12 modes plus the type/setuid-bit cases), because the
      predicate decides enforcement for every call site at once and the rule is arithmetic, not a
      syscall.
- [x] `make test` zero failures; `make build-all`, `vet-all`, `lint-all` green on all three GOOS;
      gofmt and style detectors clean.
- [ ] **Exit gate, CI-side:** the Windows tests pass on `test (windows-latest)`. This code cannot
      be executed on a development Mac — cross-compilation proves it *builds*, not that the DACL is
      right — so the leg is the only proof, and no call site migrates until it is green.

#### First windows run — the DACL was right; the assertion was not (PR #408, 2026-08-13)

`test (windows-latest)` reported **29** (28 + one new), and the one new failure was
`TestApplyMode_PrivateFileGetsProtectedDACL`. The SDDL it printed is the whole story:

```
D:PAI(A;;FA;;;LA)(A;;FA;;;SY)(A;;FA;;;BA)
```

`P` is `SE_DACL_PROTECTED` — inheritance broken — followed by exactly three full-access ACEs: the
owner (`LA`, the runner's account), SYSTEM (`SY`), and Built-in Administrators (`BA`). **That is the
target DACL exactly, produced on the first attempt.** The control-bit assertion passed and five of
the six DACL tests passed, including the directory, `Chmod`, non-private, scratch, and `OpenFile`
cases.

The failure was the test's own trustee check: it asserted the **numeric** SIDs (`S-1-5-18`,
`S-1-5-32-544`) while SDDL renders well-known accounts as **aliases** (`SY`, `BA`). Fixed to match
either rendering, anchored on the ACE's trustee field (`;;;X)`) so an alias cannot match inside an
unrelated SID, and an exact-count assertion added — a DACL with a fourth allow ACE would still be
"protected" while granting someone else access, so the count is as load-bearing as the flag.

Worth recording as evidence for the campaign's method: the read-back assertion did its job. Had the
test checked only that the call returned nil, it would have passed and told us nothing.

**Ruling 4's contradiction, found by a test.** As merged, ruling 4 gave the formula
`perm&0o077 == 0` while also asserting `0640` and `0600` were indistinguishable. Those cannot both
hold: the formula leaves `0640` inheriting its parent's DACL. The implementation followed the
formula, the test followed the prose, and `make test` failed. **Corrected to `perm&0o007 == 0`** —
see ruling 4. The rejected reading failed *open*, which is the failure class this issue exists to
eliminate.

**Deliberate divergence from Unix, recorded:** enforcement is applied whenever a private mode is
requested, including on an object that already exists — where Unix would ignore the perm argument.
This fails closed. A caller asking for `0600` gets a private file whether or not it was the creator.

**Residual, not hidden:** enforcement is applied by path, not by handle, so a privileged attacker
able to swap the path between the write and the DACL call could redirect it. Closing that needs
handle-based `SetSecurityInfo` at every creation site; tracked on #405 rather than solved here.

### Phase 2b: session-owned filesystem access — branch `session-owned-fs` — status: pending

**Inserted before the migration, 2026-08-13, after phase 3's first call site exposed a wrong
pattern.** Migrating `pkg/signing` by having it construct its own root drew the ruling that makes
this phase necessary; the change was reverted rather than left as precedent for 29 more sites.

**The invariant: code never constructs filesystem access — it receives it.** The root is set by
whoever starts a starlark or graph execution, carried on the `RuntimeEnvironment`, and reached
downstream through the activation record (`ActivationRecord.RuntimeEnvironment`, documented as
"always set during dispatch"). Every provider already does this —
`p.RuntimeEnvironment().Root` in `encryption`, `archive`, `file`. **A provider constructing its own
root bypasses the session that owns it.**

- [ ] **Scratch joins root on the session.** `RuntimeEnvironment` carries both; providers reach
      scratch the same way they reach root. Scratch has the sharper lifetime hazard — an unclosed
      root leaks a handle, an unclosed scratch leaks a *tree* that Windows cannot remove while any
      handle inside it is open — so concentrating ownership in the one place with a proven teardown
      matters more, not less, than for root.
- [ ] **Both allocate lazily.** A planning-only session that never touches the filesystem then
      holds no confined-root handle at all, shrinking the exact surface #393 was about.
- [ ] **Root keeps eager preflight validation** — existence and accessibility checked at
      environment build via an open-and-release probe, the pattern `plan.Provider.Spec` already
      uses. Purely lazy allocation would move a bad-anchor failure from preflight to first
      filesystem use, i.e. from "refused before dispatch" to "died halfway through a run", which is
      strictly worse than the `ReasonPreflightFailed` terminal phase 2 just built.
- [ ] **`Root` becomes an accessor that asserts.** `Root() fsroot.Root` returns the root alone; a
      mint failure *after* successful validation is an invariant violation, not a condition to
      handle. This matches the repository's checksum trust boundary — corruption before verification
      is an error, any failure after verification is an assert — and keeps ~40 provider call sites
      as one-liners. Accepted residual: a genuine mid-run environmental failure (handle exhaustion,
      a yanked mount) panics rather than unwinding through compensation.
      **Incomplete as written** — it does not cover the session that legitimately has *no* root, a
      case seven existing call sites depend on; see the open question under the 2b delivery plan.
- [ ] **Scratch is anchored at the OS temp directory, with no spec field.** `os.MkdirTemp("")`
      resolves through `os.TempDir()`, which already honors `TMPDIR` on Unix and `TMP`/`TEMP` on
      Windows — every environment-configuration reason to relocate scratch (small tmpfs, encrypted
      volume, constrained container) is already served by the standard mechanism, and a spec field
      would reinvent it. The one non-configuration case is **cross-device staging**: a
      stage-then-rename into a target tree is only atomic on the same filesystem, and `TMPDIR`
      cannot know the target's device. That is a per-operation need, answered by a scratch *inside*
      the target root, not by a session-level anchor.

#### Phase 2b delivery plan — three PRs, revised 2026-08-14

The split exists because making the field private breaks every test literal that sets it, and the
test constructor must land **first** or nothing is actually split.

| PR | Scope | Status |
| --- | --- | --- |
| **2b.1** | Move test literals that set `Root:` onto the real constructor | **in progress — 6 of 19 files** |
| **2b.2** | Field → private, `Root()` / `Scratch()`, lazy allocation, **82 sites / 20 files** (1 write, 81 reads) | pending |
| **2b.3** | Move `archive`'s spool and `devlore-test`'s runner onto `Scratch()` (optional; may fold into 2b.2) | pending |

**A codegen PR was planned and proved unnecessary.** The `receiver_type.gen_test.go` template emits
`Application` / `Context` / `Status` and **never `Root`**, so no generated file breaks. Verified by
enumeration, after an earlier estimate wrongly assumed every `RuntimeEnvironment` literal in the
tests was affected.

**Scope, re-enumerated 2026-08-14.** The earlier "28 sites in 18 files" was itself a filtered-grep
figure and does not survive counting: the **13 unconverted files alone hold 26 sites**, and every
converted file held at least one, so the total is **19 files** and at least 32 sites. The trap that
inflates a naive count is the reverse one — `cmd/star/provider/starcode/provider_test.go` shows 12
`Root:` lines but only **6** are `RuntimeEnvironment`; the other six are `starcode.Provider.Root`,
an unrelated `string` field. Counts here are of `RuntimeEnvironment` literals specifically.

**PR 2b.2's production surface, enumerated 2026-08-14 — 82 sites in 20 files.** The `~85` estimate
survived counting roughly intact, unlike the test-side figures.

| Area | Sites |
| --- | --- |
| `pkg/op/provider/file` (6 files) | 44 |
| `pkg/op/recovery_site.go` | 13 |
| `cmd/star/provider/{starcode,staranalysis,starcomplexity,starindex,starstats}` | 10 |
| `pkg/op/provider/{encryption 4, mem 4, archive 2, function 2, git 1, plan 1}` | 14 |
| `pkg/op/runtime_environment.go` | 1 |

Method: every non-test `.Root` reference (220), less `os.Root` / `fsroot.Root` type names and
comments (133), then classified by receiver — cobra's `cmd.Root()`, `Graph`/`GraphSpec.Root`,
`text/template`'s `tree.Root`, go-git's `Filesystem.Root()`, `fsroot`-internal `decoded.Root`, and
the star providers' own `Provider.Root string` field are all unrelated. The star sites read
`ctx.Root`, where `ctx` is a `*op.RuntimeEnvironment` — a receiver-name grep misses them entirely.
(Separately: that parameter should be named `runtimeEnvironment`; `ctx` reads as a
`context.Context`. Fix it in the PR that touches those ten sites.)

**Exactly one production site writes the field** — the constructor at `runtime_environment.go:218`.
Privatizing is a one-line write change; the other 81 are reads.

**Open question 2b.2 must answer first: what does `Root()` do when the session has no root?** Seven
sites nil-check it today — the five star providers' `if ctx.Root != nil`, plus
`provider/mem/resource.go:337` and `provider/file/provider.go:62`. They exist because a spec with an
empty `RootPath` legitimately yields a nil root (`runtime_environment.go:786`: "Empty means no root:
the environment's `Root` stays nil"), which is exactly the planning-only session lazy allocation is
meant to serve. So the asserting accessor ruled above **cannot serve these seven** as written: it
would panic for a session that never had a root, as distinct from one whose mint failed after
validation. Decide the split — a second `HasRoot()` predicate, an `(fsroot.Root, bool)` return, or
making no-root sessions impossible — before the mechanical pass starts, not during it.

**Review hazard:** `file.Provider` already has `Root() string`
(`provider/file/provider.go:60`), itself a nil-tolerant wrapper returning `""`. Forty-four of the 82
sites live in that package, so `p.Root()` (a path string) will sit beside
`p.RuntimeEnvironment().Root()` (an `fsroot.Root`) — two `Root()` methods of different types in one
file. Phase 6's `fsroot.Dir` rename does not resolve this; the collision is between two accessors,
not two types.

**PR 2b.1 — converted and green (2026-08-14):** `pkg/op/recovery_site_test.go`,
`pkg/op/triad_test.go`, `pkg/op/inventory/seam_test.go`,
`pkg/op/provider/encryption/{provider,receipt}_test.go`,
`pkg/op/provider/archive/provider_test.go`.

**Remaining (13 files, 26 sites):** `pkg/op/provider/git/` 10 (`provider` 4, `resource_input` 2,
`checkout_pull_observe` 2, `receipt` 1, `resource` 1), `cmd/star/provider/starcode/provider_test.go`
6, `pkg/op/provider/{json,yaml,service,mem,function}/resource_test.go` 6 (`mem` 2, the rest 1 each),
`pkg/op/provider/file/provider_test.go` 3, `pkg/op/starlarkbridge/runtime_test.go` 1.

**The pattern** — a local `testEnvironment(t, dir)` helper per package:

```go
runtimeEnvironment, err := op.NewRuntimeEnvironment(context.Background(),
    op.NewRuntimeEnvironmentSpec("test").
        WithRoot(dir, fsroot.ModeWritableUnconfined).
        WithApplication(&application.Application{Name: "test"}))
if err != nil { t.Fatalf("op.NewRuntimeEnvironment: %v", err) }
t.Cleanup(func() { _ = runtimeEnvironment.Close() })
```

**Three gotchas, each already hit once:**

1. The constructor **already wires `RecoverySite` and defaults `ResourceCatalog`** — those
   hand-assembled lines are deleted, not ported.
2. `Application` must be non-nil; `NewRuntimeEnvironment` asserts on it.
3. Removing the `fsroot.Open*` call often **orphans the `fsroot` import** — this broke the build
   once already in `encryption/receipt_test.go`.

One special case: `pkg/op/provider/file/provider_test.go` also sets `BackupSuffix`, which the
constructor does **not** default. Assign it after construction or the test loses its meaning.

**Why before the migration:** this changes `RuntimeEnvironment.Root` from a field to an accessor,
touching many of the files the migration would touch. The same argument that puts the
`fsroot.Dir` rename last runs the other way here — the rename does not change the API the migration
targets; this does.

**CLI root ownership — ruled 2026-08-13: `internal/cli` owns purpose-named roots.**

`internal/cli` holds 20 of the 29 remaining restrictive sites (state home `0700`, run index `0600`,
user config `0600`, self-install manifest and binaries) plus `signing`'s key via
`store.go:197`, and has no `RuntimeEnvironment` anywhere — none of it runs inside an execution.
Those paths span three different trees, so no single anchor covers them.

The package exposes lazily-opened, purpose-named roots for its known bases — a state root, a config
root, and an install-prefix root handed to self-install — closed at command exit. Everything
downstream, `pkg/signing` included, **receives** one.

This keeps the invariant intact rather than carving an exception out of it: **the session owner
constructs, everyone else receives**, and for CLI-side work the CLI layer *is* the session owner.
The rejected alternatives were giving the CLI a full `RuntimeEnvironment` (heavy machinery to write
a config file, and it inverts an existing dependency — config loading precedes environment
construction, so the session would need the config it is meant to write) and letting each command
open what it needs (which makes the rule area-dependent, and the boundary between areas is exactly
where a contributor will guess wrong).

Accepted residual: each of the 20 sites must be assigned to the correct base, and a mis-assignment
anchors a root wider than intended with nothing to catch it. Assignments are recorded per site in
phase 3's PR rather than decided in passing.

`pkg/signing` therefore takes a root from its caller. Its `generateLocalKey` currently carries a
`TODO(#405)` and still writes through `os.*` — the private key is **not** enforced on Windows until
that lands.

### Phase 3: migrate the security-relevant sites — branch `fsroot-migrate-secure` — status: blocked on 2b

- [ ] The 31 restrictive-perm sites, **`pkg/signing` first** (the private key).
- [ ] Then `internal/cli`'s restrictive set (state home, run index, user config, self-install
      manifest), `internal/document`, and the lore/writ sites.
- [ ] Exit gate: the seven windows permission failures clear **by enforcement**, not by scoping.

### Phase 4: migrate the remainder, by area — branch per area — status: pending

Areas in ascending order of ambiguity, each its own PR:

- [ ] **Providers (13 → 11 migrate, 2 stay justified).** `archive`'s 3 spool sites become
      `OpenScratch` users rather than bypasses; `star`'s 8 migrate onto the env's root. `git`'s
      `RemoveAll` (clone tree owned by an external subprocess) and `plan.SaveDefinition`'s
      `os.Create` (caller-named store document) keep their existing `// Confinement:` comments.
- [ ] **Shared libraries (12).** `pkg/signing` already done in phase 3; `internal/document`,
      `internal/lorepackage`, `internal/registry`, `internal/credentials`, `pkg/sink` remain.
      These own no root, so each needs one supplied by its caller — the design question of phase 4.
- [ ] **Tooling / harness (11).** `devlore-test` (6, and its `MkdirTemp` becomes `OpenScratch`),
      `internal/e2e` (2), `docgen` (2), `devlore-inventory` (1).
- [ ] **CLI (48, minus those done in phase 3).** Preceded by the call-path analysis: enumerate
      every path that reaches an `os.*` call, so the migrate-or-justify decision is made against
      evidence rather than per call site. `internal/cli`'s 28 may warrant their own PR.

### Phase 5: guard + observability — branch `fsroot-guard` — status: pending

- [ ] Verify the mechanism can see `// Confinement:` comments before committing to `forbidigo`.
- [ ] R4: the guard, wired into `make check` and `quality-gate`.
- [ ] **R4b: `IsRestricted`** — the Win32-OpenSSH-style validation helper. Lands before R5, which
      consumes it.
- [ ] R5: `Observe`'s DACL-derived `restricted` fact.
- [ ] Exit gate: a deliberately reintroduced unjustified `os.WriteFile` fails the build.

### Phase 6: rename `fsroot.Root` → `fsroot.Dir` — branch `fsroot-dir-rename` — status: pending

Mechanical, no behavior change, and **deliberately last**.

- [ ] `fsroot.Root` → `fsroot.Dir` across the repository; `Root` disappears as a type name inside
      `pkg/fsroot` too.
- [ ] The `RuntimeEnvironment.Root` **field keeps its name** — it names the role, and renaming it
      is a far larger blast radius than the type. The declaration becomes `Root fsroot.Dir`, field
      naming the role and type naming the thing.
- [ ] Interface doc keeps stating the `*os.Root` method-set mirror explicitly, since the name no
      longer carries it.

**Why rename:** `fsroot.Root` is textbook package-name stutter, which Go's own guidance says to
avoid. The counter-argument — that the name advertises the `*os.Root` mirror — does not survive
inspection: the mirror is a property of the *method set* and is stated in the doc comment, so it
survives the rename, while the stutter is paid at every call site forever. `Sandbox` was rejected
because two of the three modes are explicitly **unconfined** — it would name a thing that
sandboxes nothing. `Dir` follows Go's own habit of naming an accessor for the noun it accesses
(`os.File`, `os.Root`).

**Why last:** the rename is a single mechanical pass whose cost does not scale with the number of
references, but a rename landing *during* phases 3–4 would conflict with every in-flight migration
branch. Running it after the migration settles trades a larger diff — which costs nothing, since
it is automated — for zero conflicts.

## Verification

`make test` green; `make lint-all` and `make build-all` green on all three GOOS; the windows leg's
seven permission failures cleared **by enforcement, not by scoping**; DACL assertions passing on
`test (windows-latest)`; the guard failing a deliberately reintroduced direct `os.WriteFile`.

## Risks, stated

1. **Failing open.** A subtly wrong DACL — missing the protected flag, wrong ACE order — leaves the
   file accessible while every test that only checks "no error" passes. R6's assertions must read
   the DACL back and check the ACE set, never merely that the call returned nil.
2. **CI environment.** The windows runner executes as its own user; DACL assertions must key off
   the *current* owner rather than a hardcoded principal, or they will pass locally and fail in CI
   (or worse, the reverse).
3. **Churn without benefit.** ~53 of the 84 sites carry no security consequence and are migrated for
   the guard's sake. Reviewers should expect mechanical diffs there and reserve scrutiny for phases
   1–2.
4. **Plumbing pressure.** `internal/cli`'s 28 sites may tempt a package-level singleton root. That
   is a structure decision, not a mechanical one — surface it before adopting.

## Open Questions

- [ ] What does `Root()` return for a session that legitimately has no root? Blocks PR 2b.2; seven
      call sites nil-check the field today. Detailed under the phase 2b delivery plan.
- [ ] Which root instance serves `internal/cli` (one per base — state home, config, install prefix —
      or one anchored higher)? Settle in phase 3, not by accident in the first migrated file.
- [ ] Does `fsroot` need anything beyond `Chmod` (R1's audit answers this)?
- [ ] Do we enforce on *existing* files at deploy time (a chmod pass), or only at creation?

## Related Documents

- Issue [#405](https://github.com/NobleFactor/devlore-cli/issues/405) — the gap
- [windows-phase-3e.md](./windows-phase-3e.md) — cluster 4, reclassified to bucket 1 by ruling 1
- [platform-test-matrix.md](./platform-test-matrix.md) — the #373 campaign
- `cmd/writ/writ/snapshot/protect_darwin.go` — the existing `x/sys` platform-protection precedent
