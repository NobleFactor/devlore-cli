---
title: "Windows native permissions: enforce restrictive modes, route every mutation through fsroot"
issue: https://github.com/NobleFactor/devlore-cli/issues/405
status: in progress
created: 2026-08-13
updated: 2026-08-18
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

6. **`fsroot.Dir` reaches full `*os.Root` parity.** The interface exposes every method `*os.Root`
   provides; nine are missing today — `Chmod`, `Chown`, `Chtimes`, `Create`, `Lchown`, `Link`,
   `Mkdir`, `OpenRoot`, `RemoveAll`. Part of the 84 are not philosophical bypasses at all but code
   routing around a too-small interface: `os.RemoveAll` alone has **10** call sites with no
   `fsroot` equivalent. A guard that demands justification must first make the justified path
   available.
7. **`fsroot.OpenScratch` — scratch is a `Root` confined to a temp directory.** Not an
   `os.CreateTemp` wrapper: a `fsroot.Dir` anchored at a freshly created temp directory, whose
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
| Restrictive modes on Windows, **through `fsroot`** | ✅ enforced | protected DACL, proved on `test (windows-latest)` 2026-08-14 |
| Restrictive modes on Windows, **everywhere else** | ❌ silently ignored | the 84 direct `os.*` sites never reach the seam; `Chmod` toggles read-only only |
| Single write seam | ❌ absent | 84 mutation calls bypass `fsroot` |
| `fsroot.Dir` ↔ `*os.Root` parity | ✅ complete | 24 methods incl. `Chmod`, `RemoveAll`, `OpenRoot`, plus `OpenScratch` |
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
| `pkg/signing` | 3 | `pkg/document` | 2 |
| `pkg/op/provider/archive` | 3 | `cmd/writ/writ/migrate` | 2 |
| `internal/lorepackage` | 3 | `cmd/star/provider/lint` | 2 |
| `pkg/sink`, `pkg/op/provider/plan`, `pkg/op/provider/git`, `internal/credentials` | 1 each | | |

31 of the 84 pass a restrictive perm; the security-relevant ones are `pkg/signing` (private key +
its `0700` directory), `internal/cli` (state home `0700`, run index `0600`, user config `0600`,
self-install manifest `0600`), `pkg/document` (`0600` writes), and the SOPS-decrypted output
reached via the file provider.

## Requirements

### R1 — `fsroot.Dir` reaches full `*os.Root` parity

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

Each site takes an `fsroot.Dir` and calls its method. The sites that own no root today —
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

### Phase 2: Windows enforcement inside `fsroot` — branch `fsroot-windows-acl` — status: complete (2026-08-14)

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
- [x] **Exit gate, CI-side — MET 2026-08-14.** `pkg/fsroot` reports `ok` on `test
      (windows-latest)`, so all six `TestApplyMode_*` DACL tests pass on a real Windows runner and
      none appear in that leg's 28 known failures. This code cannot be executed on a development Mac
      — cross-compilation proves it *builds*, not that the DACL is right — so the leg was the only
      proof, and it is now green. **Call sites are cleared to migrate.**

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

#### Correction 2026-08-15 — enforcement was missing from the unconfined writable root

Phase 2 recorded `applyMode` as "wired into `WriteFile`, `OpenFile` (on `O_CREATE`), `Mkdir`, `MkdirAll`, and
`Chmod` **on both writable implementations**". That was true of every method **except `WriteFile` on
`unconfinedRootReaderWriter`**, which returned `os.WriteFile` directly and applied nothing.

**Impact, had it not been caught:** the campaign rules that CLI-side roots are `ModeWritableUnconfined`, so
*every* remaining phase-3 site — state home, run index, user config, self-install manifest,
`pkg/document`'s writes — would have gone through the one unenforced path. The migration would have
completed, the code would have looked right, and nothing would have been protected.

**Why the tests missed it.** Every DACL test in `applymode_windows_test.go` used `testConfinedRoot`. The
confined implementation was the one already enforcing; the tests proved the safe path and never touched the
one production actually uses outside an execution.

**What found it.** `pkg/signing`'s own read-back, on a real Windows runner, printing the descriptor:

```
D:AI(A;ID;FA;;;LA)(A;ID;FA;;;SY)(A;ID;FA;;;BA)
```

`AI` rather than `P`, and every ACE tagged `ID` — purely inherited. The key was private only because its
parent directory happened to be protected, which is a weaker guarantee that any later change to the parent
would quietly erode.

**Fixed**, with `TestApplyMode_BothWritableRootsProtect` running the same assertions across both writable
implementations so the asymmetry cannot return. `Create` remains unenforced on both **by design** — phase 1
ruled it produces an unrestricted `0666` file that must never carry sensitive content.

**The general lesson, for phases 4 and 5:** a test that exercises the safest implementation proves the least.
These assertions belong on the implementation production actually uses.

### Phase 2b: session-owned filesystem access — branch per PR below — status: in progress (2b.1 complete; 2b.2 next)

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
- [ ] **Both keep eager preflight validation** — existence and accessibility checked at environment
      build via an open-and-release probe, the pattern `plan.Provider.Spec` already uses: the root's
      anchor, and (ruled 2026-08-14) `os.TempDir()` for scratch. Purely lazy allocation would move a
      bad-anchor failure from preflight to first filesystem use, i.e. from "refused before dispatch"
      to "died halfway through a run", which is strictly worse than the `ReasonPreflightFailed`
      terminal phase 2 just built. Probing scratch is also what licenses its accessor to assert.
- [ ] **`Root` becomes an accessor that asserts.** `Root() fsroot.Dir` returns the root alone; a
      mint failure *after* successful validation is an invariant violation, not a condition to
      handle. This matches the repository's checksum trust boundary — corruption before verification
      is an error, any failure after verification is an assert — and keeps ~40 provider call sites
      as one-liners. Accepted residual: a genuine mid-run environmental failure (handle exhaustion,
      a yanked mount) panics rather than unwinding through compensation.
      **Completed by the `HasRoot` ruling below** — the assert covers the session whose root failed
      to mint; a session that never had an anchor is a separate, advertised case.
- [ ] **Scratch is a per-session directory inside the OS temp directory, with no spec field** — and
      therefore takes no `HasScratch` predicate; see the scratch ruling below. `os.MkdirTemp("")`
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
| **2b.1a** | First 6 test files onto the real constructor — branch `test-env-constructor` | ✅ **merged 2026-08-14 (#410)** |
| **2b.1b** | Remaining **26 sites / 12 files** — branch `test-env-constructor-remainder` | ✅ **complete 2026-08-14** |
| **2b.2a** | `Dir.CreateTemp` + `Dir.MkdirTemp` on the interface — branch `fsroot-temp-methods` | ✅ **merged 2026-08-14 (#413)** |
| **2b.2b** | Field → private, `HasRoot()` / `Root()` / `Scratch()`, preflight, lazy allocation, **105 rewrites / 25 files** | ✅ **complete 2026-08-14** |
| **2b.3** | Move `archive`'s spool and `devlore-test`'s runner off hand-rolled temp lifecycles | ✅ **complete 2026-08-14** |

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

Method: every non-test `.Root` reference (220), less `os.Root` / `fsroot.Dir` type names and
comments (133), then classified by receiver — cobra's `cmd.Root()`, `Graph`/`GraphSpec.Root`,
`text/template`'s `tree.Root`, go-git's `Filesystem.Root()`, `fsroot`-internal `decoded.Root`, and
the star providers' own `Provider.Root string` field are all unrelated. The star sites read
`ctx.Root`, where `ctx` is a `*op.RuntimeEnvironment` — a receiver-name grep misses them entirely.
(Separately: that parameter should be named `runtimeEnvironment`; `ctx` reads as a
`context.Context`. Fix it in the PR that touches those ten sites.)

**Exactly one production site writes the field** — the constructor at `runtime_environment.go:218`.
Privatizing is a one-line write change; the other 81 are reads.

#### Ruled 2026-08-14: `HasRoot() bool` + a panicking `Root()`, modeled on `reflect.Value`

**The question was what `Root()` does for a session that legitimately has no root.** Seven sites
nil-check the field today — the five star providers' `if ctx.Root != nil`, plus
`provider/mem/resource.go:337` and `provider/file/provider.go:62` — and they are right to: a spec
with an empty `RootPath` yields a nil root by design (`runtime_environment.go:786`, "Empty means no
root"), and `cmd/lore/lore/builder.go:108` builds exactly such a session **in production**, with
`WithStatus` / `WithModules` / `WithApplication` and no `WithRoot`. All seven treat absence as legal
degradation, not error: `file.Provider.Root()` returns `""`, `mem` returns a zero `fsroot.Path{}`,
and the star providers skip setting their own root field.

**The ruling:**

1. **`HasRoot() bool`** — reports whether the session was built with an anchor. Under lazy
   allocation it must answer from the **spec**, not from whether a handle happens to be minted yet,
   or it would change its answer mid-session.
2. **`Root() fsroot.Dir` panics when `HasRoot()` is false**, with a typed error naming the method
   and the cause, in the shape of `reflect.ValueError` ("reflect: call of reflect.Value.Interface on
   zero Value"). Not a string panic: identifiable under `recover`.
3. **`HasRoot`'s doc carries the rule**, exactly as `reflect.Value.IsValid` does — *if `HasRoot`
   returns false, `Root` panics* — so the contract is stated once rather than repeated at each
   accessor.
4. **The producer advertises.** `reflect`'s discipline is "most functions never return an invalid
   Value; if one does, its documentation states the conditions explicitly." So the constructor path
   that can yield a rootless session says so, making lore's builder an advertised case rather than a
   runtime discovery.

**Why not the alternatives.** `(fsroot.Dir, bool)` — the comma-ok shape of `context.Deadline` — was
weighed and rejected on call-site distribution: 75 of the 82 sites structurally have a root, so
comma-ok imposes ceremony on the many for the few, which is precisely why `reflect.Value` pairs a
predicate with panicking getters instead. Returning a bare nil was rejected because `fsroot.Dir` is
an **interface**: nil does not avoid the panic, it relocates it into `fsroot` internals with a stack
that never names the mistake. Making no-root sessions impossible was rejected because it contradicts
lazy allocation and would force an invented anchor on lore's builder.

**Accepted residual:** nothing compels a caller to ask `HasRoot` first, so a wrong assumption is a
runtime panic rather than a compile error. That is the same trust boundary the checksum rule already
draws, and `reflect` has shipped it for a decade.

#### Ruled 2026-08-14: `Scratch()` is an accessor to a per-session directory, preflighted like root

**No `HasScratch`.** Root's absence is *configured* — an empty `RootPath`, decided at construction
and knowable without touching the filesystem — which is what makes `HasRoot` both answerable and
necessary. Scratch has **no spec field**, so no session can be configured without it. A `HasScratch`
could only ever return `true`, advertising a state that does not exist and inviting dead
`if !HasScratch()` branches. Adding it would be renaming a sibling to match.

**`Scratch()` is an accessor, not a factory, and it is anchored at a per-session directory inside
the OS temp directory — never at the temp directory itself.** `os.MkdirTemp("")` yields
`<tempdir>/<random>`; `Close` releases the handle and removes that tree. Anchoring at `os.TempDir()`
directly fails three ways: `OpenScratch`'s `Close` contract would delete the shared system temp
directory; confinement to a directory every process on the machine writes to isolates nothing and
collides by name; and the mode is not ours to set, whereas a per-session directory is created
`0700` and therefore inherits phase 2's DACL protection — which matters because scratch holds the
most sensitive *transient* data in the system.

**Preflight applies to scratch too, and that changes its error contract.** `os.TempDir()`
accessibility is probed at environment build by the same open-and-release pattern root uses.
Preflighting supplies exactly what justified `Root()`'s panic — validation that already passed — so
a later `MkdirTemp` failure is a failure *after* verification, which by this repository's trust
boundary is an assert. **`Scratch() fsroot.Dir` therefore returns the directory alone, with no error
return, symmetric with `Root()`.**

*This supersedes an earlier ruling the same day* that gave `Scratch()` an `(fsroot.Dir, error)`
signature on the grounds that scratch had nothing to preflight. Preflighting removes the premise;
the earlier text is superseded rather than qualified, and the asymmetry it defended is gone.

**Rejected:** eagerly allocating the session directory at build rather than probing and releasing —
it makes a planning-only session that never scratches carry a tree it must remember to remove, which
is the leak surface #393 cost a campaign to close.

#### `fsroot.Dir` gains `CreateTemp` and `MkdirTemp` — landing in 2b.2

A shared per-session scratch directory means concurrent callers need unique names *within* it, and
`gather` dispatches its body concurrently by design. `os.CreateTemp` supplies that uniqueness free
today; a bare `Dir.Create` does not. So `Dir` gains `CreateTemp` and `MkdirTemp`, across all three
implementations — `errReadOnly` on the read-only one, like every other mutation — landing **in 2b.2
alongside the accessors** rather than as a separate `fsroot` PR.

**Both trees have the methods, and the choice between them is the whole design.** They are
mechanically identical; only the tree differs, and it differs along five axes:

1. **Atomicity.** `Root().CreateTemp` stages *inside the target tree*, so a stage-then-rename is
   atomic. Scratch is frequently a different device (tmpfs, separate mount), where the rename fails
   `EXDEV` and degrades to a copy. This is the cross-device staging case no `TMPDIR` setting can
   answer.
2. **Who sweeps.** Scratch's tree is removed wholesale at session `Close`. A temp file under root
   sits in the user's tree with nothing sweeping it — a crashed operation orphans it there.
3. **Visibility.** Root is usually anchored at a tree the workflow observes (`Observe`, the gitignore
   tracker, globbing, later plan steps). Temp files there are *in scope* mid-run; scratch is
   invisible to the resource model.
4. **Sensitivity.** The scratch directory is `0700` and DACL-protected. A temp file under root
   inherits that tree's permissions unless every call site remembers to ask.
5. **Availability.** Scratch always works once preflight passed. Root may not exist at all
   (see `HasRoot`) and may be **read-only**, where `CreateTemp` fails — so staging in a read-only
   session must use scratch.

**The rule, to appear verbatim in both doc comments:** *use `Scratch()` unless the bytes must end up
in the root's tree atomically; if the operation ends in a rename into that tree, stage with
`Root().CreateTemp` so the rename cannot cross a device boundary.* Applied to the known consumers:
the archive spool gives random access to a streamed zip whose bytes never enter the user's tree →
**scratch**; a file provider writing a target file atomically → **`Root().CreateTemp` + `Rename`**.

#### 2b.3 as delivered (2026-08-14) — and one correction to this plan

**`archive`'s spool** now lands in the session's scratch through `Scratch().CreateTemp`, threaded
down via `openArchiveStream`. Three consequences: **both `//nolint:gosec` suppressions disappear**
(no `os.Remove` of a variable path is left to justify), the spool is `0600` and therefore
DACL-protected on Windows instead of a bare temp file, and a crashed run leaks nothing — scratch is
swept at session close even if the reader never closes. The reader still removes its own file on
close, because an archive's bytes should not outlive the read that needed them and a long session
extracting many archives would otherwise accumulate them.

**`devlore-test`'s runner** collapses `os.MkdirTemp` + `defer os.RemoveAll` + its
`//nolint:errcheck` best-effort cleanup into a single `fsroot.OpenScratch("devlore-test-*")`, where
one `Close` both releases the handle and removes the tree.

**Correction to this plan's wording:** the runner cannot use `RuntimeEnvironment.Scratch()`. It
builds its `TestContext` *before* any session exists — the workspace is the tree the session is then
anchored at — so the correct tool is `fsroot.OpenScratch`, which is exactly what phase 1 built it
for. "Onto `Scratch()`" was the right idea named for the wrong API.

**Deliberately not changed:** `TestContext` keeps an **unconfined** writable root over that tree.
Scripts under test address absolute paths, which a confined root refuses by design, so handing it
the scratch root would smuggle a behavior change into a cleanup — and the Windows leg already has
`devlore-test` failures that would then be unattributable.

#### 2b.2 as delivered (2026-08-14)

Split in two: **2b.2a** put `CreateTemp` / `MkdirTemp` on the interface (#413), so a sealed-interface
addition was reviewed on its own rather than inside the migration diff. **2b.2b** is the rest.

**The production count held; the total did not.** 82 production sites was accurate, but the
migration touched **105 sites across 25 files** once test files were included — `triad_test.go`
alone held 20. The estimate counted production only, and said so; the lesson is that a migration's
size is the *repository's* count, not the subset that was enumerated.

**The mechanical rewrite over-matched once, and the compiler caught it.** `pkg/op/triad_test.go`
defines a local `triadEnv` fixture with its own `Root fsroot.Dir` **field**, so `env.Root` there is
not the session; 20 rewrites were reverted. It compiled loudly because a field is not callable —
but had that fixture been an interface with a `Root()` method, the same edit would have compiled and
silently changed meaning. **Phase 6's rename runs the same risk over a far larger surface.**

**One existing test specified the old contract** — `Root() != nil` on a rootless session — and was
rewritten to specify the new one, with `recover()` proving the panic. A new test pins scratch:
a private tree inside `os.TempDir()`, the same instance on every call, `0700`, removed on `Close`.

**Implementation notes for the reader:**

- `sync.Once` per resource, not a shared mutex: `gather` dispatches bodies concurrently, so two
  goroutines can reach `Root()` at the same moment.
- The assert is the repository's existing `assert.Failf` / `assert.NoError` (typed `AssertionError`,
  identifiable under `recover`) rather than a newly coined error type.
- `probeScratchAnchor` carries a `// Confinement:` comment: it is the one call that cannot route
  through a root, because it exists to prove the anchor a root would need.
- `Close` releases only what was minted, so a planning-only session closes nothing.

**Review hazard:** `file.Provider` already has `Root() string`
(`provider/file/provider.go:60`), itself a nil-tolerant wrapper returning `""`. Forty-four of the 82
sites live in that package, so `p.Root()` (a path string) will sit beside
`p.RuntimeEnvironment().Root()` (an `fsroot.Dir`) — two `Root()` methods of different types in one
file. Phase 6's `fsroot.Dir` rename does not resolve this; the collision is between two accessors,
not two types.

**PR 2b.1a — merged 2026-08-14 as [#410](https://github.com/NobleFactor/devlore-cli/pull/410),
squashed to `eaf80b5a`:** `pkg/op/recovery_site_test.go`, `pkg/op/triad_test.go`,
`pkg/op/inventory/seam_test.go`, `pkg/op/provider/encryption/{provider,receipt}_test.go`,
`pkg/op/provider/archive/provider_test.go`. `make test` and `make lint-all` verified green on
`develop` at the merge commit; `test (windows-latest)` held at the identical 28-failure set,
name for name, confirming the conversion moved nothing on that platform.

**It cost one CI round-trip, for a reason worth keeping.** `gofmt` was run and was clean, but
`gofmt` never reorders import groups — `goimports`, inside `golangci-lint`, does. `quality-gate`
failed on three files where `"context"` sat in a group of its own and `pkg/application` had been
dropped into the stdlib block. **The gate to run before handing off Go changes is `make lint-all`,
not `gofmt`.** (`golangci-lint fmt` will fix such files but leaves `pkg/application` in a group by
itself; place imports by hand to the stdlib / third-party / project convention.) This repository
runs no pre-commit hooks — `pre-commit` was evaluated and rejected 2026-08-14 — so `make lint-all`
before the commit and CI after it are the only gates that exist.

#### PR 2b.1b — branch `test-env-constructor-remainder` — status: complete (2026-08-14)

Completes 2b.1. One PR rather than five: the pattern was proven, the gotchas were known, and a
half-converted test suite is the worst state to leave 2b.2 waiting behind.

**Delivered — 26 sites across 12 files** (as converted, with the pre-flight estimate beside it):

| # | Target | Sites | Note |
| --- | --- | --- | --- |
| 1 | `pkg/op/provider/git/` — `provider` 5, `resource_input` 2, `checkout_pull_observe` 2, `receipt` 1, `resource` 1 | **11** (est. 10) | one shared helper; `checkout_pull_observe`'s narrating provider needed a bespoke spec for `WithStatus` and the dry-run flag |
| 2 | `cmd/star/provider/starcode/provider_test.go` | 6 | only 6 of its 12 `Root:` lines were the environment; the other six are `starcode.Provider.Root string` and were left untouched |
| 3 | `pkg/op/provider/{json,yaml,service,mem,function}/resource_test.go` | 6 | `mem` 2, the rest 1 each; five identical `newTestRuntimeEnvironment` helpers |
| 4 | `pkg/op/provider/file/provider_test.go` | 3 | `BackupSuffix` plus a catalog-free session |
| 5 | `pkg/op/starlarkbridge/runtime_test.go` | **0** (est. 1) | no `Root:` site at all — see the enumeration correction below |

**Two enumeration errors, which cancelled.** `git` had **11** sites, not 10: a bare
`&op.RuntimeEnvironment{}` that the `Root:.*fsroot\.` grep could not see, since it sets no field at
all. `starlarkbridge` had **0**, not 1: its literals set `Modules`, and the counting regex matched
the word "Root" inside fixture *names* like `bridgePlannedRootFixture`. The total held at 26 by
coincidence, and the file count was 12 rather than 13. Recorded because the cancellation is exactly
what makes such an error survive review — the headline number looked right.

**A second construction special case, beyond `BackupSuffix`.** `mem`'s
`TestNewResource_NilCatalogReturnsUnlinkedCandidate` builds its environment with a **nil
`ResourceCatalog` on purpose** — that is the test's whole subject — and the constructor defaults
one. Same shape in `file`'s `mustRegular`, whose "unlinked" result depends on the same nil. Both now
construct through the helper and then clear the catalog, with a comment saying why. **The general
rule: where the constructor's defaulting *is* the thing under test, construct and then undo, never
route around the constructor.**

**`BackupSuffix`'s old comment was wrong about mechanism** — it claimed `NewRuntimeEnvironment`
derives the value, and the constructor never sets the field at all
(`runtime_environment.go:211`). The value matches `NewRuntimeEnvironmentConfig`'s builtin floor. The
replacement comment says so.

**`make lint-all` earned its place again:** it flagged `file`'s `testRoot` helper as dead once its
last caller was rewritten. `make test` was perfectly happy. Deleted, not suppressed.

**Exit gate — met.** Zero `Root:`-setting literals remain in any `_test.go`; `make test` reports
zero failures; `make lint-all` reports zero issues on linux, darwin, and windows, run **before**
`../go` was written.

**Left alone deliberately:** `starlarkbridge`'s `&op.RuntimeEnvironment{Modules: …}` literals set no
root, so 2b.2 does not break them.

**Not unblocked by this PR:** nothing — 2b.2's design was ruled separately (#411). Finishing the
conversion removes the merge-conflict surface 2b.2 would otherwise fight.

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

### Phase 3: migrate the security-relevant sites — branch per slice — status: planned 2026-08-14

Planning this phase corrected two things the earlier outline got wrong. Both are recorded before the
slices because they change what phase 3 can claim.

#### Correction 1 — "`pkg/signing` first" is not executable as written

`signing.DefaultSigner()` derives its key path from `configHome()` and has **exactly one production
caller**, `internal/cli/store.go:197` (plus one test). Under the invariant, signing must *receive* a
root — so `internal/cli` must own a **config root** before signing can be handed one. The CLI
plumbing therefore comes first; signing is the first *consumer* of it, not the first change.

#### Correction 2 — the windows failures do NOT clear in phase 3, and cannot

The outline promised "the seven windows permission failures clear **by enforcement**". They cannot,
and ruling 5 already says why: on Windows `Mode().Perm()` reports `0666` no matter what the DACL
says. Every one of these tests asserts `os.Stat(path).Mode().Perm() == 0o600`, so **enforcement
makes the file genuinely private without moving the assertion one bit**.

Enumerated from `test (windows-latest)` on `develop` (2026-08-14) — **7 tests, 8 assertions**:

| Test | Package | Assertion | Clears when |
| --- | --- | --- | --- |
| `TestWrite_YAMLCreatesFileWith0o600` | `pkg/document` | `permission = 666, want 600` | phase 5 |
| `TestWrite_JSONCreatesFileWith0o600` | `pkg/document` | `permission = 666, want 600` | phase 5 |
| `TestWrite_WithPermOverridesPermission` | `pkg/document` | `permission = 666, want 644` | **never** — `0644` is not private; see below |
| `TestCopy_WritesNewFile` | `pkg/op/provider/file` | `file mode = 666, want 600` | phase 5 |
| `TestWriteBytes_WritesContentToNewFile` | `pkg/op/provider/file` | `file mode = 666, want 600` | phase 5 |
| `TestExecute_SopsChains` | `cmd/writ/…/deploy` | `secret mode`, `note mode` = `-rw-rw-rw-`, want `0600` | phase 5 |
| `TestObserve_ReportsFileFields` | `pkg/op/provider/file` | `Observe: Mode.Perm() = 666, want 640` | phase 5 (R5's own subject) |

`TestWrite_WithPermOverridesPermission` is the sharp one: `0644` grants **other**, so ruling 4's
boundary leaves it inheriting the parent DACL by design. It has no enforcement state to observe, and
must become a Unix-scoped assertion — the one case where scoping is the right answer rather than the
rejected one.

**So the windows leg stays at 28 through phases 3 and 4**, and moves in **phase 5**, when
`IsRestricted` (R4b) and `Observe`'s `restricted` fact (R5) give these tests something true to
assert on both platforms. Phase 3 still delivers the security fix — the private key becomes
genuinely protected, verifiable by reading the DACL back exactly as phase 2's own tests do. **Do not
read an unmoved 28 after phase 3 as a failed migration.**

#### The slices

- [x] **3.0 — the anchors are sound on every platform — COMPLETE 2026-08-15.** Every anchor 3.1
      needs — state, config, and the install prefix — now resolves through `pkg/xdg`. Eleven
      hand-rolled resolvers are gone, `internal/cli/xdg.go` with them; a repository-wide grep for
      `os.UserHomeDir`, `HOME`, `USERPROFILE` and `XDG_*` returns nothing outside `pkg/xdg`, save
      one `user.Current()` that reads a **username**. `defaultPrefix()` is `xdg.UserHomePath(".local")`
      and `expandTilde()` resolves through the same ladder. **3.1 is unblocked.** Chartered as
      [54-xdg-anchors-on-windows.md](../plans/extract-starlark-from-op/phase-8/steps/54-xdg-anchors-on-windows.md):
      `internal/cli/xdg.go` resolves every anchor from `os.Getenv("HOME")`, which Windows does not
      define, so with `XDG_*` unset `StateHome()` yields the **relative** `.local\state`. Anchoring
      a writable root there would hold authority over an arbitrary working directory while
      presenting as scoped — strictly worse than the `os.WriteFile` it replaces. `pkg/signing`
      already resolves correctly via `os.UserHomeDir()`, and its doc claims a parity with
      `internal/cli` that does not hold. **3.1 does not start until this lands.**

      **Ruled 2026-08-14, after researching current practice:** *we find home; we default to the XDG
      standard directory names rooted at home.* Home comes from a four-rung ladder — an **absolute**
      `XDG_<ROLE>_HOME`, else `os.UserHomeDir()`, else `os/user.Current().HomeDir` (the OS's own
      answer: the process token's profile directory on Windows, the passwd entry on Unix), else an
      assert. A **relative** `XDG_*` value is invalid and ignored, which is the one thing the
      specification does rule on. Rung 3 matters because `os.UserHomeDir` is a `Getenv` with a nicer
      name and `%USERPROFILE%` is not guaranteed for services or scheduled tasks. Layout stays
      `~/.config`, `~/.local/share`, `~/.local/state`, `~/.cache` on **every** platform: nothing in
      the AppData cohort treats `%APPDATA%` as a substitute for `$HOME`, Microsoft's own OpenSSH
      keeps private keys in `%USERPROFILE%\.ssh`, and both migration debates upstream sit open and
      unimplemented. Full evidence and residuals in the step doc.
- [ ] **3.1 — `cmd/internal/cli` opens its purpose-named roots.** The CLI is the session owner for
      CLI-side work (2b ruling). Its **29** `os.*` mutation sites are split across two PRs; the
      per-site assignment is this slice's real work — a mis-assignment anchors a root wider than
      intended with nothing to catch it.

      **Corrected 2026-08-16 by enumeration: five trees, not three.** This bullet said "state,
      config, install-prefix" and filed cache and writ's layers under the prefix. Cache is
      `devlore.CacheHome()` and the layers are `devlore.DataHome()` — anchoring either at the prefix
      would put a root over the wrong tree entirely, which is precisely the mis-assignment this
      slice exists to prevent. `man.go`'s temp file is a sixth case, belonging to scratch.

      | Root | Sites | Anchor |
      | --- | --- | --- |
      | state | 4 | `devlore.StateHome()` |
      | config | 7 | `devlore.ConfigHome()` |
      | cache | 2 | `devlore.CacheHome()` |
      | data | 2 | `devlore.DataHome()` |
      | scratch | 2 | `fsroot.OpenScratch` |
      | install prefix | 11 | operator-supplied argument |
      | polymorphic | 1 | `removeIfEmpty`, decided by its caller |

      **Ruled 2026-08-16: the caller owns the root.** A leaf that opens its own is the 2b invariant
      broken in miniature — and `WriteTrace` proved it concretely, writing three things into one tree
      (document, `latest` symlink, index line) while its callee opened a root for itself. The caller
      opens one root per store write and passes it down; `appendIndexEntry` takes `fsroot.Dir` as its
      first parameter and constructs nothing.

      **`removeIfEmpty` is the one site whose root cannot be decided by reading it** — it is called
      with directories in the config tree, the cache tree, and the install prefix. It takes a root and
      a path, which is what forced two prefix-tree callers into slice A ahead of schedule.

- [x] **3.1a — the XDG-anchored roots — COMPLETE 2026-08-16.** 18 sites: state 4, config 7, cache 2,
      data 2, scratch 2, plus `removeIfEmpty`'s signature. `make check` and the full suite green; the
      windows leg holds at 28, as phase 3 predicts.
- [x] **3.1b — the install-prefix root — COMPLETE 2026-08-16.** The remaining 11, and the only tree
      whose anchor is an operator-supplied argument rather than an XDG accessor — so its root is
      threaded from the command through `runSelfInstall`'s stages rather than opened from an
      accessor. `runSelfInstall` and `runSelfUninstall` each own one root now; the two sites that
      opened one locally in 3.1a are hoisted onto it.

      **`CopyDir` changed shape, and the asymmetry is the point.** It takes a destination root, a
      plain source path and a destination `fsroot.Path`: a source is whatever the operator points at
      — the running executable, a build tree — and is not ours to confine, while the destination is.
      `copyFile` carries the same split. `cmd/star`'s post-install hook opens a root and passes it,
      since the hook contract is still a prefix string (that contract is the campaign's LAST item).

      **Two `// Confinement:` justifications** were added rather than worked around: the running
      executable that `installBinary` copies from, and cobra's `doc.GenManTree`, which writes the
      page files itself given a directory path.

      **`cmd/internal/cli` now has zero `os.*` mutation sites.**

- [x] **3.1c — the install path gets tests, on every platform — COMPLETE 2026-08-16.** The 29 sites
      were converted with no end-to-end coverage of the thing they implement: the existing tests
      checked that subcommands exist and helpers behave, and nothing had ever installed anything.
      - Unit, in `cmd/internal/cli`: `runSelfInstall` into a temporary prefix with every `XDG_*`
        redirected, asserting the tree, the manifest, and that **every recorded path resolves**; then
        `runSelfUninstall` removes exactly those.
      - Scenario, in `cmd/scenario`: the same, through the **real binaries**, once per tool — `lore`,
        `star`, `writ`. It catches what an in-process test cannot, starting with the `.exe` suffix.
      - `make test-scenario` now runs both scenario groups, so CI's `scenario` job covers all three
        platforms with no workflow change.

      Modes are asserted nowhere in either: `Mode().Perm()` reports `0666` on Windows whatever the
      DACL says (ruling 5), so a mode check would be Unix-only or false. Enforcement is proved by the
      DACL read-back tests.

      **Ruled 2026-08-16 while sizing this slice — where a machine-wide install goes.** The per-user
      default stays `~/.local`. Machine-wide is the **usual directories within `/usr/local`** on Darwin
      and Linux — a GNU prefix, so `bin`, `share/man/man1` and the completion directories land where
      `PATH` and `xdg.DataDirs()` already look, with no symlink step. On Windows it is
      `%ProgramFiles%\<Vendor>\<Product>`; the exact spelling is open, with `NobleFactor` (no space,
      matching the org and module path) recommended over `Noble Factor`, and the product name's casing
      to settle against the lowercase `devlore` used everywhere else on disk.

      `/opt/local` was considered and **rejected**: it is not in FHS — which reserves only `/opt/bin`,
      `/opt/doc`, `/opt/include`, `/opt/info`, `/opt/lib` and `/opt/man` for local use — and its one
      real-world claimant is MacPorts, whose prefix it is.

      **`--all-users` selects it; the positional prefix remains the escape hatch.** A prefix argument
      cannot express machine-wide on Windows, because `%ProgramFiles%\<Vendor>\<Product>` is not a GNU
      prefix and carries obligations no path can state — the Uninstall registry key and machine `PATH`
      in `HKLM` rather than `HKCU`. A mode flag can, and each platform decides what it entails. The two
      are mutually exclusive: supplying both is a usage error, not a precedence puzzle. The flag is
      **not** `--system`, which would collide with writ's own layer vocabulary, where `System` names
      the `/` target root opposite `Home`.

      **Consequence this slice must handle:** the prefix root stops being reliably user-writable. A
      machine-wide install anchors it where elevation is required, so opening it can fail for
      permission reasons that `~/.local` never produces. The failure has to say *this needs
      administrator privileges*, not surface a bare `EACCES` from halfway through an install.

      **Not in this slice:** the Windows registry obligations are install *behavior*, not root
      plumbing, and want their own charter alongside the `--all-users` flag itself.

      *Paths: phase 7 moved this package on 2026-08-15. Text written before that date says
      `internal/cli`, and is left as written — it records what was true when it was ruled. Every
      file named below now lives under `cmd/internal/cli`.*
      - **state root** (`DevloreStateHome`) — `index.go` 2 (state home `0700`, run index `0600`),
        `store.go` 2 (the `latest` symlink).
      - **config root** (`DevloreConfigHome`) — `config.go` 2 (`0750` dir, `0600` user config),
        plus `selfinstall.go`'s config/config.d writes.
      - **install-prefix root** — the bulk of `selfinstall.go` (bin dir, cache, layers, manifest
        `0600`, the `0750` binary `Chmod`).
      - `man.go`'s `os.CreateTemp` becomes `fsroot.OpenScratch` — CLI-side, no session.
- [x] **3.2 — `pkg/signing` receives a root — COMPLETE 2026-08-15.** `DefaultSigner(configRoot
      fsroot.Dir)`, one production caller updated (`internal/cli/store.go`). All three sites now
      write through the root: the `0700` directory, the `0600` key, the `0644` public half.
      `pkg/signing` has **zero** `os.*` mutations left and the `TODO(#405)` is gone.

      **It needed almost none of the groundwork it appeared to.** No `Session` type, no `internal/cli`
      xdg migration, no `internal/` sweep, no curation — the CLI opens a writable-unconfined root at
      the existing `DevloreConfigHome()` and hands it over. Writable-unconfined because the config
      tree may not exist on first run and a confined root requires its anchor to exist.

      **The suite name left `pkg/signing` as a side effect:** the root arrives anchored at
      `<config>/devlore`, so signing joins `signing/ed25519` and no longer knows the string
      `"devlore"`.

      **`DefaultSigner`'s first branch keeps reading `~/.ssh/id_ed25519` directly**, with a
      `// Confinement:` comment: the user's SSH directory is not ours to confine, and a root anchored
      at our config tree cannot address it.

      **Proof is a DACL read-back, not a mode assertion** (`signing_windows_test.go`): mode bits
      report 0666 on Windows however private the object is (ruling 5), so the tests read the security
      descriptor off the key and assert `SE_DACL_PROTECTED` plus exactly three trustees — owner,
      SYSTEM, Administrators. A fourth allow ACE would leave the DACL "protected" while granting
      someone else read access to a signing key, so the count is as load-bearing as the flag. A third
      test pins the deliberate asymmetry: the `.pub` half must **not** be protected, since it exists
      to be copied into someone else's `allowed_signers`.

      **The 28 does not move.** No existing test asserts the key's mode, so these are additive.
- [ ] **3.3 — `pkg/document`** (2 sites: `0750` dir, `cfg.perm` write) — the package behind
      three of the seven tests above.

      **Re-shaped 2026-08-17, by enumeration and then by a design ruling.** Both sites live inside
      `document.Write`, which owns no root — so converting them changes its signature for **25 call
      sites**, nine of which hold no root today. The plan already knew this and filed it under
      phase 4's "shared libraries… each needs one supplied by its caller — the design question of
      phase 4"; it sized 3.3 as if the two questions were separate.

      Examining the package answered it differently. `document.Write` does three unrelated jobs:
      choose a rendering from the **file extension**, apply the permission policy, and wrap errors
      with the path. It never serialized anything — `Graph` has `MarshalJSON`/`MarshalYAML` and
      `Trace` marshals through its tags. Meanwhile `op` had `LoadGraph` and `LoadTrace` but no
      `Save*`: deserialization was a trust boundary owned by the artifact, serialization was not.

      **Ruled: the artifacts save themselves.**

- [x] **3.3a — `op.SaveGraph` / `op.SaveTrace` — COMPLETE 2026-08-17.** The write-side complements of
      `LoadGraph` / `LoadTrace`, taking a root and a path, writing `0600`.
      - **Format is stated, not inferred from a filename suffix** — `SaveGraph(…, "yaml")` mirrors
        `LoadGraph(env, data, format)`. `SaveTrace` is YAML-only because `LoadTrace` is.
      - **`SaveTrace` stamps the checksum itself.** `LoadTrace` refuses a document without one, so
        leaving the stamp to the caller allowed writing a document nothing could read — a failure
        that surfaced at the next load rather than at the write. Stamping is idempotent because the
        canonical bytes exclude the field.
      - **Signing stays with the caller**: the checksum belongs to the artifact, the key belongs to
        whoever publishes it.
      - **`Save*` does not create directories.** A save that invents store layout would decide store
        policy on the caller's behalf, so `WriteGraph`/`WriteTrace` create their own trees — which is
        what `document.Write`'s implicit `MkdirAll` had been hiding.

      **Delivered** — PR [#427](https://github.com/NobleFactor/devlore-cli/pull/427), merged
      `07d9210e`. Seven checks green; `test (windows-latest)` failed with the inherited baseline,
      verified **name-for-name** against `develop`'s own leg at `d6abacbc`: 28 versus 28, zero new
      and zero cleared. The three `scenario` legs — the self-install scenario's first run under
      branch protection — passed on all platforms.

- [ ] **3.3b — `document.WriteFile` takes a root.** The 2026-08-17 deferral is superseded by the
      2026-08-20 rulings on [#558](https://github.com/NobleFactor/devlore-cli/issues/558): both steps
      land now, as two PRs. The work-done-twice concern is answered rather than waited out — the
      stream form `Write(w io.Writer, format Format, v any, …)` takes its format **explicitly** (the
      `SaveGraph` rule: format is stated, not inferred), which IS the single-codec seam a protobuf
      rendering plugs into; extension-sniffing survives only in the path form as a file-boundary
      convenience.
      - [x] **PR-A — the codec split.** `Write` → `WriteFile` (the `Read`/`ReadFile` symmetry the
        write side was missing), `Write(w, format, v, …)` added, `WithPerm` documented as
        [WriteFile]-only. Ten production call sites renamed mechanically; no behavior change; the
        windows leg holds.
      - [x] **PR-B — the root.** `WriteFile(dir fsroot.Dir, p fsroot.Path, v, …)`; the ten call
        sites take roots threaded from the CLI layer that owns the purpose-named tree (ruled: no
        library opens a root for itself). Two sites earned documented exceptions: the git transport
        opens at its own `.sync-info.yaml` write because its subprocess creates and replaces the
        cache tree wholesale — there is nothing for a caller to open before Sync, and a held handle
        would block the replacement on Windows; and `e2e.WriteReport`'s raw `summary.md` write rode
        along through the same root. The two 0o600 mode assertions gate to unix, and
        `document_windows_test.go` DACL read-backs prove protection and the 0o644 boundary, in
        phase 2's shape. The windows leg moves **6 → 4**: `TestWrite_JSONCreatesFileWith0o600`,
        `TestWrite_YAMLCreatesFileWith0o600`.
- [ ] **3.4 — the writ deploy/sops output path** ([#433](https://github.com/NobleFactor/devlore-cli/issues/433)) — behind `TestExecute_SopsChains`, and the one
      that writes **decrypted plaintext**, which makes it the second-most security-relevant site in
      the campaign after the private key.
- [ ] Exit gate: every restrictive-perm write in these areas flows through a root; a **DACL
      read-back test** proves the private key is protected on Windows, in the shape phase 2's
      `_windows_test.go` established. The 28 does **not** move here.

### Phase 4: migrate the remainder, by area — branch per area — status: pending — [#434](https://github.com/NobleFactor/devlore-cli/issues/434)

Areas in ascending order of ambiguity, each its own PR:

- [ ] **Providers (13 → 11 migrate, 2 stay justified).** `archive`'s 3 spool sites become
      `OpenScratch` users rather than bypasses; `star`'s 8 migrate onto the env's root. `git`'s
      `RemoveAll` (clone tree owned by an external subprocess) and `plan.SaveDefinition`'s
      `os.Create` (caller-named store document) keep their existing `// Confinement:` comments.
- [ ] **Shared libraries (12).** `pkg/signing` already done in phase 3; `pkg/document`,
      `internal/lorepackage`, `internal/registry`, `internal/credentials`, `pkg/sink` remain.
      These own no root, so each needs one supplied by its caller — the design question of phase 4.
- [ ] **Tooling / harness (11).** `devlore-test` (6, and its `MkdirTemp` becomes `OpenScratch`),
      `internal/e2e` (2), `docgen` (2), `devlore-inventory` (1).
- [ ] **CLI (48, minus those done in phase 3).** Preceded by the call-path analysis: enumerate
      every path that reaches an `os.*` call, so the migrate-or-justify decision is made against
      evidence rather than per call site. `internal/cli`'s 28 may warrant their own PR.

### Phase 5: guard + observability — branch `fsroot-guard` — status: pending — [#435](https://github.com/NobleFactor/devlore-cli/issues/435)

- [ ] Verify the mechanism can see `// Confinement:` comments before committing to `forbidigo`.
- [ ] R4: the guard, wired into `make check` and `quality-gate`.
- [ ] **R4b: `IsRestricted`** — the Win32-OpenSSH-style validation helper. Lands before R5, which
      consumes it.
- [ ] R5: `Observe`'s DACL-derived `restricted` fact.
- [ ] Exit gate: a deliberately reintroduced unjustified `os.WriteFile` fails the build.

### Phase 6: rename `fsroot.Root` → `fsroot.Dir` — status: **complete 2026-08-15, out of order**

Mechanical, no behavior change, and ~~deliberately last~~ **run early**, alongside phase 7 — the
conflict argument for going last evaporated once phases 3–4 had not started.

- [x] `fsroot.Root` → `fsroot.Dir` across the repository; `Root` disappears as a type name inside
      `pkg/fsroot` too.
- [x] The `RuntimeEnvironment.Root` **field keeps its name** — it names the role, and renaming it
      is a far larger blast radius than the type. The declaration becomes `Root fsroot.Dir`, field
      naming the role and type naming the thing.
- [x] Interface doc keeps stating the `*os.Root` method-set mirror explicitly, since the name no
      longer carries it.

**Done semantically, not textually.** `gopls rename` drove the 54 call sites and the `[fsroot.Root]`
doc links, which is what kept the field and `Path.Root()` untouched — a `sed` on `\bRoot\b` would
have hit both. Two gaps it could not see, each fixed by hand and each worth recording:

1. **Prose, not doc links.** Four `.go` files mentioned the old name in running comment text rather
   than in `[…]` link syntax, which a semantic rename does not read. **The same gap spans the
   documentation**: 38 references across eight `.md` files, which no code tool touches at all.
2. **The build-tag blind spot.** `pkg/fsroot/applymode_windows_test.go` uses the type twice and is
   excluded from the darwin build, so the rename never saw it. This is the same class of escape the
   full-configuration recount rule exists for — and `go build` would not have caught it either,
   because `go build` does not compile tests. `make lint-all` does, and reports **0 issues** under
   all three GOOS values.

**What deliberately kept the name `Root`**, none of it in scope above:

- `Path.Root()` — an accessor returning the anchor path as a `string`. It names what it returns.
- The unexported implementations — `rootBase`, `scratchRoot`, `confinedRoot`,
  `unconfinedRootReader`, `unconfinedRootReaderWriter`. Package-internal, no stutter at any call
  site, and renaming them buys nothing the exported rename already bought.

**Why rename:** `fsroot.Root` was textbook package-name stutter, which Go's own guidance says to
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

### Phase 7: move `internal/cli` → `cmd/internal/cli` — status: **complete 2026-08-15, out of order**

**Ruled 2026-08-14 to run last; it ran first instead**, pulled forward by `cmd/internal/devlore` —
the package that now owns the application's path names. Being under `cmd/internal/`, it is
unreachable from `internal/`, so every consumer of those names had to sit under `cmd/` before the
tree would build. The ordering residual this section accepted below is therefore **avoided**: phases
3–5 will plumb their roots at the new path, with no second mechanical pass.

**The move is wider than one package.** The import closure was enumerated, not estimated — anything
importing `cli` or `config` had to come too:

| Moved | Files | Why |
| --- | --- | --- |
| `cmd/internal/cli` | 15 | needs `devlore` for its path anchors |
| `cmd/internal/config` | 6 | imports `cli`, and needs `devlore.ConfigHome()` |
| `cmd/internal/model` | 9 | imports `cli` and `config` |
| `cmd/internal/lorepackage` | 11 | imports `config` |
| `cmd/internal/e2e` | 3 | imports `config`, `lorepackage`, `model` |

`internal/console`, `credentials`, `document`, `manifest`, `pwsh`, `registry` and `tools` stay —
none of them imports a moved package, and nothing in `pkg/` did either. 53 files had import paths
rewritten; `make vet`, `make build` and `make test` are clean.

**Two of this phase's three goals are unmet, and relocation is why.** Moving the dependents under
`cmd/` satisfied the compiler without correcting the layering the move was meant to expose:

the two `internal/` libraries that reached **up** into the CLI facade still do. Relocating them under
`cmd/` made the compiler stop objecting; it did not make the dependency point the right way.

- [x] Move the package; `cmd/internal/cli` is importable only from within `cmd/…`, which turns that
      layering violation into a **compile error** rather than a convention.
- [ ] `model` stops narrating — values or an injected narrator, not `cli.Note` ×8 and `cli.Error`.
      A library printing "Is Ollama running?" to a terminal is a layering problem wherever the
      package sits, and moving it to `cmd/internal/model` changed only the address.
- [ ] `config` receives its config anchor instead of calling for it — the same *receive, don't
      construct* shape phases 2b and 3 apply to filesystem access. It now calls
      `devlore.ConfigHome()` where it called `cli.DevloreConfigHome()`: a shorter reach up, still a
      reach up.
- [x] Exit gate: `cmd/internal/cli` has no importer outside `cmd/…`, and the build proves it.

#### Follow-up chartered 2026-08-15: `devlore` must not know its callers

Moved to the closure list — see [`devlore` must not know its callers](#last-devlore-must-not-know-its-callers)
under Verification. It is the campaign's final item rather than phase 7's loose end, because it turns
out to carry an uninstall-semantics decision rather than being a refactor.

## Verification

`make test` green; `make lint-all` and `make build-all` green on all three GOOS; DACL assertions
passing on `test (windows-latest)`; the guard failing a deliberately reintroduced direct
`os.WriteFile`.

**`make test-scenario` is part of this gate, and was not treated as such.** It is not in `make
check` — it drives the real binaries end to end and is the only test that deploys to a home
directory at all. On 2026-08-15 a change passed `make check` and `make lint-all` on three platforms,
was committed, and only then failed `scenario` on macOS and ubuntu — having deployed into the
developer's actual home directory, because the home ladder had stopped honoring the sandbox's
`HOME`. A gate that only CI runs is a gate that reports after the commit.

**The campaign is not done while [#392](https://github.com/NobleFactor/devlore-cli/issues/392) is open.**
The System target root is the literal `/` on every platform, which on Windows is
drive-relative — the same ambient-resolution defect as the XDG anchors, one level up. Exposure is currently
zero because nothing deploys through the System scope, and that is exactly why it must be closed
with tests rather than declared harmless: the campaign's claim is that Windows paths resolve
absolutely, and today one of them does not.

**Nor while [#438](https://github.com/NobleFactor/devlore-cli/issues/438) is open**, chartered
2026-08-17 out of the 3.3a review. `WriteTrace` creates `latest.yaml` with `Symlink`, which an ordinary
Windows user — no Developer Mode, not an Administrator — cannot do. It belongs to this campaign for the
same reason #392 does: the claim is that these tools write correctly on Windows, and this is a write that
does not. It also carries the sharper version of the campaign's own lesson — the Actions runner **holds**
the symlink privilege, so no amount of CI coverage can see the defect, and the proof run has to deny the
privilege rather than merely decline to grant it.

**One of its six exit criteria is met (2026-08-18, PR #514).** `appendIndexEntry` used to sit *after* the
link, so any link failure discarded the index entry for a trace already durable on disk — silently, because
all nine callers warn rather than fail. That ordering was wrong on **every** platform; Windows is only where
it fires for ordinary users. The append now precedes the link, with a regression test that forces a link
failure portably by occupying `latest.yaml` with a non-empty directory. The `latest` disposition and the
privilege-denying Windows proof remain open.

### LAST: `devlore` must not know its callers — [#436](https://github.com/NobleFactor/devlore-cli/issues/436)

**The final item before the campaign closes.** It is last because everything above it either protects
a file or proves that protection; this one repairs the layering the campaign disturbed, and it should
not delay a security fix.

`cmd/internal/devlore` names the locations the tools share, yet two of its accessors are writ's alone:
`WritLayersDir()` and `WritReposDir()`. A package every command depends on for shared anchors must not
carry per-command knowledge — today `cmd/lore` links a function describing writ's layer registry. Eight
call sites in `cmd/writ` then write `filepath.Join(devlore.WritLayersDir(), layer)`, the
missing-joiner smell corrected everywhere else on 2026-08-16.

- [ ] The two anchors move to `cmd/writ/writ`, where all eight call sites already live, as variadic
      `LayersDir(elem ...string)` / `ReposDir(elem ...string)` over `devlore.DataPath`.
- [ ] `selfinstall.go`'s `toolName != "writ"` branch and `initWritLayers` go with them. That branch is
      the reason the accessors sit in `devlore` at all: the shared CLI package creates writ's
      directories, so moving the accessors without moving the creation would make `cmd/internal/cli`
      import a specific tool — a worse inversion than the one being fixed.
- [ ] Nothing else command-specific enters `devlore` — the four base accessors, the man page
      directory, and the three completion directories are the whole of what is genuinely shared.

**The decision this hides, and the reason it is not a pure refactor.** `PostInstallHooks` is the
existing mechanism for tool-specific install work — `func(prefix string) []string`, and
`cmd/star/main.go` returns `cli.CollectFiles(prefix, …)`, i.e. paths **relative to the prefix**, which
the uninstall manifest then tracks. Writ's layers are not under the prefix; they are under XDG data.
So a writ hook would either return paths relative to a prefix they do not live under — corrupting the
manifest — or return nothing and stay untracked, which is what the code does today.

Whether the manifest should track directories outside the prefix is a question about `writ self
uninstall`, not about layering: a layer directory holds symlinks to the user's own repositories, so
removing it on uninstall may be exactly wrong. **Answer that before writing the hook.**

**Corrected 2026-08-14 — the windows leg moves in phase 5, not phase 3.** The original wording
promised the seven permission failures would clear "by enforcement, not by scoping". Enforcement
cannot move them: `Mode().Perm()` reports `0666` on Windows whatever the DACL says (ruling 5), and
every one of those tests asserts mode bits. They clear when they learn `IsRestricted` / `Observe`'s
`restricted` fact — **six of the seven**. The seventh, `TestWrite_WithPermOverridesPermission`,
asserts `0644`, which grants *other* and therefore has no enforcement state to observe; it becomes a
Unix-scoped assertion, the one place where scoping is correct rather than evasive. See phase 3.

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

- [x] ~~What does `Root()` return for a session that legitimately has no root?~~ **Ruled
      2026-08-14:** `HasRoot() bool` plus a `Root()` that panics when it is false, modeled on
      `reflect.Value.IsValid`. Detailed under the phase 2b delivery plan.
- [ ] Which root instance serves `internal/cli` (one per base — state home, config, install prefix —
      or one anchored higher)? Settle in phase 3, not by accident in the first migrated file.
- [ ] Does `fsroot` need anything beyond `Chmod` (R1's audit answers this)?
- [ ] Do we enforce on *existing* files at deploy time (a chmod pass), or only at creation?

## Related Documents

- Issue [#405](https://github.com/NobleFactor/devlore-cli/issues/405) — the gap
- [windows-phase-3e.md](./windows-phase-3e.md) — cluster 4, reclassified to bucket 1 by ruling 1
- [platform-test-matrix.md](./platform-test-matrix.md) — the #373 campaign
- `cmd/writ/writ/snapshot/protect_darwin.go` — the existing `x/sys` platform-protection precedent
