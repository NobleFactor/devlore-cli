---
step: 59
title: "ConfigDirs and DataDirs return Unix paths on every platform, including Windows"
status: charter — chartered 2026-08-16; sibling of step 58, same defect family
proof_run: TBD — must include a Windows case asserting every returned entry is volume-absolute
parent: ../../phase-8.md
---

# Step 59 — The XDG search paths are Unix-only

**Status:** `charter` — chartered 2026-08-16 while summarizing system-wide install conventions for
`writ self install`. The system-wide default is the other end of this mechanism, so the defect had to
be named before anything is built on top of it.

## The defect

[`pkg/xdg/xdg.go:121`](../../../../../pkg/xdg/xdg.go) and
[`pkg/xdg/xdg.go:142`](../../../../../pkg/xdg/xdg.go):

```go
func ConfigDirs() []string { return dirs(envConfigDirs, "/etc/xdg") }
func DataDirs() []string   { return dirs(envDataDirs, "/usr/local/share", "/usr/share") }
```

Those are the specification's defaults, and they are correct on Linux and Darwin. On Windows they are
wrong twice over:

1. **`/etc/xdg` and `/usr/share` do not exist**, and nothing on Windows searches them.
2. **They are not absolute.** Go treats a leading separator with no volume as drive-relative, so
   `/usr/share` resolves against whatever drive the process is standing on — the same
   ambient-resolution defect as [step 58](58-windows-system-target-root.md)'s `/`, and the same
   family as [step 54](54-xdg-anchors-on-windows.md)'s relative `.local\state`.

The package's own documentation states the invariant these violate: *"no anchor can resolve to a
relative path."* `TestNoAnchorIsEverRelative` proves it for the five **base** accessors. The two
search-path accessors are not in that test, and would fail it on Windows.

## Exposure is zero today, which is why it survived

Enumerated 2026-08-16: `ConfigDirs()` and `DataDirs()` have **no callers** anywhere in the repository
outside `pkg/xdg`'s own tests. They exist because the specification defines them and the package
implements the specification completely.

That is the same position step 58 is in, and the same argument applies: a defect with no consumers is
cheap to fix and expensive to inherit. The moment a system-wide install lands — which is what
prompted this charter — `DataDirs()` becomes the natural place to look for machine-wide completions,
man pages and package data, and it will silently return two nonexistent drive-relative paths on
Windows.

## What the answer probably is, and what has to be decided

The Windows convention has no direct counterpart to a *search path*: machine-wide payload lives in
`%ProgramFiles%\<Company>\<Product>` and machine-wide data in `%ProgramData%\<Company>\<Product>`.
There is no established list of directories that Windows software scans in preference order the way
`XDG_DATA_DIRS` is scanned.

So the decision is not "which paths" but **what these functions mean on a platform without the
concept**:

1. **`%ProgramData%` as a one-entry list.** `DataDirs()` returns `[%ProgramData%]`, `ConfigDirs()`
   returns `[%ProgramData%]` — machine-wide, absolute, and where Windows software genuinely keeps
   shared data. Residual: it maps a search *path* onto a single directory, so preference ordering
   stops being expressible.
2. **Empty list on Windows.** Honest about the absence: there is no system-wide search path, so
   callers fall back to the per-user home. Residual: a caller written on Unix silently finds nothing
   on Windows rather than finding the machine-wide copy.
3. **Derive from the install prefix.** If the tool is installed system-wide, its own prefix is the
   search path. Residual: `pkg/xdg` would have to know about installation, which it deliberately does
   not.

**Answer the same way step 54's layout question was answered — by researching what comparable tools
actually do on Windows, not by picking the tidiest option.** The `%USERPROFILE%\.config` ruling came
from git, OpenSSH, Docker and cargo agreeing; this needs the same evidence.

## Exit criteria

- [ ] `ConfigDirs()` and `DataDirs()` return only volume-absolute paths on every platform.
- [ ] `TestNoAnchorIsEverRelative` — or a sibling — covers **all seven** accessors, not the five
      bases. The gap in that test is what let this ship.
- [ ] The Windows semantics are decided against evidence from comparable tools, with residuals
      recorded, exactly as the layout ruling in step 54 was.
- [ ] The package documentation states what these mean on Windows; today it describes only the
      specification's Unix defaults.

## Related

- [step 58](58-windows-system-target-root.md) — the System target root, `/` on every platform. Same
  family, same zero-exposure argument, chartered one day earlier.
- [step 54](54-xdg-anchors-on-windows.md) — the closed original: anchors resolving relative on
  Windows.
- [windows-native-permissions.md](../../../windows-native-permissions.md) — whether this gates the
  campaign the way step 58 does is **open**; it is the same class of defect, but the campaign's claim
  is about paths the tools actually write to, and nothing writes through these two yet.
