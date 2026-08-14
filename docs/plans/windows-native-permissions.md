---
title: "Windows native permissions: enforce restrictive modes, route every mutation through fsroot"
issue: https://github.com/NobleFactor/devlore-cli/issues/405
status: draft
created: 2026-08-13
updated: 2026-08-13
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
4. **Mapping: private-vs-not.** When the requested mode denies group and other
   (`perm&0o077 == 0`), write a **protected** DACL (inheritance broken) granting the file owner,
   plus `SYSTEM` and `Administrators`. Otherwise leave the inherited default. Restricting SYSTEM and
   Administrators is theater — they can take ownership at will — and breaks backup and AV tooling
   for no defensive gain. Accepted consequence: `0640` and `0600` are indistinguishable on Windows;
   Windows has no honest analogue for the Unix group bit.
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

### Phase 1: `fsroot` API — parity + scratch — branch `fsroot-parity` — status: pending

Pure API growth. No call site moves, no behavior changes, nothing Windows-specific.

- [ ] R1: the nine missing methods across all three implementations.
- [ ] R4a: `OpenScratch`, with the `Close`-also-removes contract documented at both the
      constructor and the method.
- [ ] Tests per method per implementation, including the read-only variant's `errReadOnly` and
      `Chown`/`Lchown` failing as `os` designs them on Windows.

### Phase 2: Windows enforcement inside `fsroot` — branch `fsroot-windows-acl` — status: pending

- [ ] R2: the `applyMode` platform split; protected DACL per ruling 4; wired into `WriteFile`,
      `OpenFile` (on create), `Mkdir`, `MkdirAll`, and `Chmod`.
- [ ] R6's `_windows_test.go`: read the DACL **back** and assert the ACE set — never merely that
      the call returned nil, since a wrong DACL fails open while a nil-check passes.
- [ ] Exit gate: a `0600` write, a `0700` mkdir, and a post-hoc `Chmod` are provably restricted on
      the windows leg **before** any call site migrates.

### Phase 3: migrate the security-relevant sites — branch `fsroot-migrate-secure` — status: pending

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
- [ ] R5: `Observe`'s DACL-derived `restricted` fact.
- [ ] Exit gate: a deliberately reintroduced unjustified `os.WriteFile` fails the build.

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

- [ ] Which root instance serves `internal/cli` (one per base — state home, config, install prefix —
      or one anchored higher)? Settle in phase 3, not by accident in the first migrated file.
- [ ] Does `fsroot` need anything beyond `Chmod` (R1's audit answers this)?
- [ ] Do we enforce on *existing* files at deploy time (a chmod pass), or only at creation?

## Related Documents

- Issue [#405](https://github.com/NobleFactor/devlore-cli/issues/405) — the gap
- [windows-phase-3e.md](./windows-phase-3e.md) — cluster 4, reclassified to bucket 1 by ruling 1
- [platform-test-matrix.md](./platform-test-matrix.md) — the #373 campaign
- `cmd/writ/writ/snapshot/protect_darwin.go` — the existing `x/sys` platform-protection precedent
