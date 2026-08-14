---
title: "Windows phase 3e: the remaining 28"
issue: https://github.com/NobleFactor/devlore-cli/issues/373
status: draft
created: 2026-08-13
updated: 2026-08-13
---

# Plan: Windows phase 3e — the remaining 28

The #373 campaign's last grind. Every one of the 28 windows-leg failures on develop `99a0dbbd`
is enumerated below with its **actual error text**, its root cause read from the source, and its
campaign bucket. Classification lands here before any fix, per the campaign's phase-2 rule.

## Summary

28 `--- FAIL` lines, 0 `[build failed]`, 0 panics, 0 handle leaks. They resolve into **eleven
clusters**: **4 product defects** (one security-relevant, one — the permission cluster — large
enough to have earned its own plan), 4 Unix-assuming test clusters, 1 correctly-platform-scoped
cluster, and 6 singles that need reading before they can be classified honestly.

## Bucket key (from the campaign)

1. **Bucket 1** — a genuine defect on that platform. Fix the product.
2. **Bucket 2** — a Unix-assuming test. The product is fine; the test hardcodes a separator,
   a permission bit, or a message. Fix the test.
3. **Bucket 3** — correctly platform-scoped. The subject exists only on one platform; the test
   moves into a build-tagged file. **Not** a `t.Skip`.

## Classification

| # | Cluster | Tests | Lines | Bucket |
| --- | --- | --- | --- | --- |
| 1 | `file.Name` / `file.Parent` return OS-native separators | 4 | 4 | **1** |
| 2 | `mem.SourcePath` — test expects native, product is canonical | 1 | 1 | 2 |
| 3 | `migrate.commonAncestor` splits on `filepath.Separator` | 1 | 1 | 2 |
| 4 | Permission-bit assertions — **product defect; see [windows-native-permissions.md](./windows-native-permissions.md)** | 7 | 7 | **1** |
| 5 | chown identity — `os.Getuid()` is −1 on Windows | 2 | 2 | 3 |
| 6 | `filepath.IsAbs` misses rooted paths — symlink escape accepted | 1 | 1 | **1 (security)** |
| 7 | Onboard manifest path built by string concatenation | 1 | 1 | **1** |
| 8 | git argv narration — substring coincidence on Unix | 2 | 2 | 2 |
| 9 | CLI error text / path form | 2 | 2 | 2 |
| 10 | Starlark path escape (#376) | 1 | 1 | **1** |
| 11 | Undiagnosed singles | 6 | 6 | ? |

Total: 28.

### Cluster 1 — `file.Name` / `file.Parent` return OS-native separators (bucket 1)

```
Name("/")             = "\"      want "/"
Name("///")           = "\"      want "/"
Parent("/a/b/c.txt")  = "\a\b"   want "/a/b"
Parent("/a/b/.config")= "\a\b"   want "/a/b"
```

`provider.go:1355` and `:1366` are `filepath.Base(path)` / `filepath.Dir(path)` verbatim. Both are
Starlark-exposed (`file.name`, `file.parent`); their returns land in graph slots and therefore in
serialized documents, which is exactly the condition that made `fsroot.Path.Rel()` canonical-slash
under **#377**. A slash-form input coming back backslashed also silently changes the caller's
separator style mid-pipeline.

**Ruled (Q1): slash-native** — `path.Base` / `path.Dir`, matching `Rel` (#377) and the `Find`
matcher (#395). See Rulings below.

### Cluster 2 — `mem.SourcePath` (bucket 2)

```
SourcePath = ".devlore/mem/resource/sha256/e2/e29154…"
      want   ".devlore\mem\resource\sha256\e2\e29154…"
```

The **reverse** of cluster 1: the product returns the canonical slash form and the *test* builds
its expectation with `filepath.Join`. Under #377 the product is correct; the expectation is
Unix-assuming (it only ever matched because `filepath.Join` uses `/` on Unix). Fix the test.

### Cluster 3 — `migrate.commonAncestor` (bucket 2, pending a ruling)

```
commonAncestor("/home/user/repo", "/home/user/.local/…") = "\home\user"  want "/home/user"
```

`register.go:202` splits and rejoins on `filepath.Separator`, so Unix-literal inputs come back
backslashed. Its production inputs are real host paths, where OS-native output is correct — which
makes this a test that feeds literals rather than a product defect.

**Ruled (Q2): bucket 2.** Traced — the result reaches only `cli.Note` and `migrateSpec(root)`'s
confinement anchor, never document bytes. The helper stays OS-native and states so in its doc
comment; the test builds inputs with `filepath.Join` / `t.TempDir()`.

### Cluster 4 — permission-bit assertions (bucket 3)

```
internal/document: TestWrite_YAMLCreatesFileWith0o600   permission = 666, want 600
internal/document: TestWrite_JSONCreatesFileWith0o600   permission = 666, want 600
internal/document: TestWrite_WithPermOverridesPermission permission = 666, want 644
file:  TestCopy_WritesNewFile                            file mode = 666, want 600
file:  TestWriteBytes_WritesContentToNewFile             file mode = 666, want 600
file:  TestObserve_ReportsFileFields                     Mode.Perm() = 666, want 640
deploy: TestExecute_SopsChains                           secret/note mode = -rw-rw-rw-, want 0600
```

Windows has no Unix permission bits: Go's `Chmod` toggles only the read-only attribute and every
writable file reports `0666`. The assertions are meaningless there, not wrong. Move each into a
`_unix_test.go` so the build constraint states it — never a `t.Skip`.

**Ruled (Q3), then REVERSED the same day — this cluster is bucket 1, not bucket 3.** The first
ruling was "file the gap, scope the assertions to Unix." It was superseded within the hour:
**Windows permissions must be enforced natively**, so a product that accepts `0600` and silently
produces an unrestricted file is *wrong on Windows*, not correctly platform-scoped. These seven
failures are product defects and are cleared by enforcement, not by scoping.

The work moved to its own plan — [windows-native-permissions.md](./windows-native-permissions.md),
issue [#405](https://github.com/NobleFactor/devlore-cli/issues/405) — because enumeration showed
the surface is far larger than these tests: **84 direct `os.*` mutation calls** bypass `fsroot`,
31 of them passing a restrictive perm, including a **private key** at
`pkg/signing/signing.go:202`. Phase B of this plan therefore no longer moves the permission
assertions; it keeps only cluster 5 (chown), and the permission work is tracked there.

### Cluster 5 — chown identity (bucket 3)

```
parseChown("-1"):     invalid ownership "-1": at least one of user or group must be present
applyChown("-1:-1"):  invalid ownership "-1:-1": …
```

Root cause found by reading, not inference: **`os.Getuid()` returns −1 on Windows** (documented).
`helpers_test.go:72` builds its spec from `strconv.Itoa(os.Getuid())`, so the spec literally
becomes `"-1"`, and `parseChown` correctly rejects a spec naming neither user nor group. The
product is right on every platform; the tests' *subject* (uid/gid ownership) is Unix-only. Move to
`helpers_unix_test.go` — the same disposition PR #388 already applied to the sibling chown test.

### Cluster 6 — `filepath.IsAbs` misses rooted paths (bucket 1, security-relevant)

```
TestExtract_SymlinkTargetEscapes: absolute target = <nil>; want the symlink-target refusal
```

`containedLinkTarget` (`archive/provider.go:968`) refuses absolute symlink targets with
`filepath.IsAbs(linkname)`. On Windows `filepath.IsAbs("/etc/passwd")` is **false** — a rooted path
without a drive letter is not "absolute" there — so **the refusal never fires and the escaping
symlink is extracted**. This is the same defect class as #400's `resolveFindRoot` rooted-pattern
fix, and it is a containment check, so it is security-relevant.

**`containedTarget` (`provider.go:933`) has the identical hole** for entry names, currently
untested. Fix both; add the missing entry-name regression test rather than only the one the
failure named.

### Cluster 7 — onboard manifest path (bucket 1)

```
narration: … \001/packages-manifest.yaml     (mixed separators)
```

`cmd/lore/lore/commands.go:803` builds the path by concatenation —
`outputDir + "/packages-manifest.yaml"` — instead of `filepath.Join`. The test compares against a
`filepath.Join` result, so the mismatch is real and the emitted path is non-canonical.

### Cluster 8 — git argv narration (bucket 2)

```
narrated: "…\git.exe -C …\checkout-repo checkout release-1.2"
want contains: "git -C …\checkout-repo checkout release-1.2"
```

The narrator prints the **resolved** binary, identically on both platforms. On Unix the assertion
passes by coincidence — `/usr/bin/git -C …` happens to *contain* `git -C …`. On Windows the
resolved name carries `.exe`, breaking the substring. Product behavior is the same; the assertion
is accidental. Fix the test to assert on the argv tail (or the resolved binary), not a substring
that only works when the suffix is empty.

### Cluster 9 — CLI error text and path form (bucket 2)

```
TestCLI_RunMissingFile: output missing "no such file"        (Windows: "The system cannot find the file specified.")
TestCLI_ConfigPath:     output missing "devlore/config.yaml" (Windows: backslash form)
```

Assert on the OS-independent part (`errors.Is(fs.ErrNotExist)` semantics at the CLI boundary, or a
separator-normalized comparison) rather than on a Unix message and a slash path.

### Cluster 10 — Starlark path escape (bucket 1, issue #376)

```
invalid escape sequence \Users\RUN
```

A Windows path interpolated into generated `.star` source: `\U` is an invalid Starlark escape.
Already filed as **#376**; it lands here or in its own PR, not silently.

### Cluster 11 — undiagnosed, six singles

Listed with their evidence; each needs reading before it earns a bucket. **No bucket is asserted
for these.**

| Test | Evidence |
| --- | --- |
| `TestCompensation` (devloretest) | `compensated.txt` exists but should not; `error("file.copy")` — execution succeeded, expected error |
| `TestShellExec` (devloretest) | `shell_output.txt` not found |
| `TestSource` (devloretest) | `file.read_text: openat source_input.txt: The system cannot find the file specified` |
| `TestGatherFailureUnwind_ViaPublicAPI` | run error names `mkdirat blocker: file exists`, not the failing write — the blocker fixture may not reproduce the intended failure on Windows |
| `TestDiscoverRegular_ForeignSchemeString_IsAPath` | identity carries `file://C:\…\https:\example.com\x` — a foreign-scheme string joined as a path |
| `TestSymbolicLinkDigest_LiteralTarget` | digest ≠ hash of the literal target |

The first three are all devlore-test `.star` fixtures failing on file operations, which suggests
one shared cause rather than three; that hypothesis is to be tested, not assumed.

## Implementation Phases

### Phase A: product defects — branch `windows-3e-product` — status: pending

- [ ] Cluster 1 (Q1 ruled slash-native): `file.Name` / `file.Parent` → `path.Base` / `path.Dir`,
      doc comments stating the slash-form contract.
- [ ] Cluster 6: `containedLinkTarget` **and** `containedTarget` rooted-path containment, with the
      missing entry-name regression test.
- [ ] Cluster 7: `filepath.Join` for the onboard manifest path.
- [ ] Each with a regression test that fails on Windows before the fix.

### Phase B: platform-scoped tests — branch `windows-3e-scoping` — status: pending

- [ ] Cluster 5: two chown tests into `helpers_unix_test.go` (`os.Getuid()` is −1 on Windows).
- [x] Cluster 4 **removed from this phase** — reclassified bucket 1 by the Q3 reversal; the seven
      permission failures are cleared by [windows-native-permissions.md](./windows-native-permissions.md),
      which lands before the complexity work.

### Phase C: Unix-assuming tests — branch `windows-3e-test-fixes` — status: pending

- [ ] Cluster 2 (`mem.SourcePath` expectation), cluster 3 (Q2 ruled bucket 2 — fix the test, state
      the OS-native contract in the doc comment), cluster 8 (git argv), cluster 9 (CLI text).

### Phase D: diagnose the six — branch `windows-3e-singles` — status: pending

- [ ] Read each; classify in this document **before** fixing; test the shared-cause hypothesis for
      the three devlore-test fixtures.
- [ ] Cluster 10 (#376) lands here or separately.

### Phase E: gate — no branch

- [ ] Windows green, then #373 phase 4: add the three `test (…)` legs to ruleset `12426847` and
      prove the gate refuses a deliberate failure.

## Rulings (2026-08-13)

**Q1 — `file.Name` / `file.Parent` are slash-native.** They become `path.Base` / `path.Dir`,
returning the canonical slash form on every platform, joining `fsroot.Path.Rel()` (#377) and the
slash-native `Find` matcher (#395) under one contract: these helpers describe *logical* paths, and
logical paths are a slash-form language repository-wide. Consequence a caller must respect: code
handing these helpers a genuine Windows host path gets a slash-form answer back and must convert
at its own boundary — the same rule `Rel` already imposes.

**Q2 — `commonAncestor` stays OS-native; the test is wrong (bucket 2).** Resolved by tracing
rather than judgment: the result flows only to `cli.Note` narration and to `migrateSpec(root)`,
where it becomes the confinement anchor `fsroot.Open` mints from. It never reaches document bytes,
so OS-native is correct and `register_test.go` must build its inputs with `filepath.Join` /
`t.TempDir()` instead of Unix literals. Residual accepted: nothing in the signature states the
OS-native contract, so a future caller passing a slash-form logical path would get a subtly wrong
answer — phase C states it in the doc comment to close that.

**Q3 — file the gap, scope the tests (issue [#405](https://github.com/NobleFactor/devlore-cli/issues/405)).**
The seven permission assertions move to `_unix_test.go`; the unenforced-`0600`-on-Windows problem
— sharpest for `writ secret`'s decrypted output — is recorded in #405 with the inherited-ACL
premise named as an assumption to test, not to rely on. 3e stays a classification phase; designing
an ACL path here would block the gate behind unrelated product work.

## Related Documents

- [platform-test-matrix.md](./platform-test-matrix.md) — the #373 campaign
- [env-minted-root.md](./env-minted-root.md) — #393, the cluster that preceded this one
- Issues #373, #376, #377 (precedent), #395 / #400 (rooted-path precedent)
