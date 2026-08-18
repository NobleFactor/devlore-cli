---
step: 58
issue: https://github.com/NobleFactor/devlore-cli/issues/392
title: "The System target root is `/` on every platform, which on Windows means the current drive"
status: charter — chartered 2026-08-16; the windows campaign is not done until this closes
proof_run: TBD — must include a Windows case proving the System root is volume-absolute, and a case run from a working directory on a non-system volume
parent: ../../phase-8.md
---

# Step 58 — The System target root is `/` on every platform

**Status:** `charter` — chartered 2026-08-16 while designing the `writ.targets.*` configuration
override, which needs a correct default before it can have an override.

**Ruled at charter time, because the analogue is not in question:** the Windows counterpart of `/` is
the **system drive root** — `%SystemDrive%\`, conventionally `C:\`. What is open is how it is
resolved and what the System scope means on a platform where writing there needs elevation; see Open
questions.

## The defect

[`cmd/writ/writ/layer.go:29`](../../../../../cmd/writ/writ/layer.go) is the only place the System
root is decided, and it is a literal:

```go
func TargetOrder() []TargetSpec {
	home := xdg.UserHomeDir()
	return []TargetSpec{
		{SourceDir: "System", TargetRoot: "/"},
		{SourceDir: "Home", TargetRoot: home},
	}
}
```

`/` is not an absolute path on Windows. Go treats a leading separator with no volume as
**drive-relative**: `filepath.Join("/", "ProgramData", "x")` yields `\ProgramData\x`, which the OS
resolves against the process's *current* drive. A System deploy therefore lands on `C:\` or `D:\`
depending on where the operator happened to be standing when they ran `writ`.

**This is the same defect family step 54 just closed** — a path that resolves against ambient state
rather than an absolute anchor — and it is why this step belongs to the campaign rather than to a
general Windows-polish backlog. Step 54's anchors were relative to the working *directory*; this one
is relative to the working *volume*.

## The documentation describes a different thing, and that thing is also wrong

Two places document the System scope as `%SystemRoot%`:

- [`cmd/writ/writ/adopt_cmd.go:41`](../../../../../cmd/writ/writ/adopt_cmd.go) — help text: *"Items
  under / (Unix) or %SystemRoot% (Windows) are adopted into System/"*.
- [`cmd/writ/writ/adopt/batch.go:276`](../../../../../cmd/writ/writ/adopt/batch.go) — comment:
  *"paths under `%SystemRoot%` are System"*.

`%SystemRoot%` is `C:\Windows` — a subdirectory of the drive root, not the root. Scoping System
there would admit only files destined for the Windows directory itself, which is nothing like `/`.
**No code reads either variable**, so these describe behavior that was never implemented, and the
correction is part of this step rather than a separate documentation task.

## Enumerated 2026-08-16 — the exposure is currently zero

That is why it has survived, and it is also why fixing it is cheap:

| Site | What it is |
| --- | --- |
| `layer.go:29` | The only assignment of the System root anywhere |
| `layer_test.go:18`, `layer_test.go:20` | `"/"` as a fixture literal, asserting `PartitionByScope` grouping — never deployed through |
| `snapshot_test.go:207`, `:267`, `:460` | Same: `TargetName: "System"` fixtures for snapshot grouping |
| The deploy scenario | Its fixture has **no `System/` directory**, so `CollectLayerSources`'s `dirExists` check skips the scope entirely |

So no test deploys through the System scope on any platform, and the writ-deploy scenario — the only
end-to-end gate — never exercises it. The bug is latent. It surfaces the first time anyone puts a
file under a layer's `System/` directory on Windows, and it surfaces as files written to the wrong
volume rather than as an error.

## Why `%SystemDrive%\` and not `%ProgramData%`

**writ is path-preserving.** A file at `System/etc/foo` deploys to `/etc/foo`; the layer author
writes the destination as the source tree, and the Home scope behaves identically. Mapping System to
`%ProgramData%` would silently rewrite paths, making `System\ProgramData\app\x` land at
`C:\ProgramData\ProgramData\app\x` or forcing a translation table nobody asked for. With the drive
root as the anchor, a Windows-segment layer writes `System\ProgramData\app\x` and gets
`C:\ProgramData\app\x` — the same rule the tool already follows everywhere else.

## Resolution, mirroring step 54's ladder

Ask the operating system before the environment, for the same reason home does:

1. `GetSystemWindowsDirectory` (via `golang.org/x/sys/windows`) yields `C:\Windows`;
   [`filepath.VolumeName`](https://pkg.go.dev/path/filepath#VolumeName) of that plus a separator
   yields `C:\`.
2. `%SystemDrive%` — the environment's answer, as the rung beneath it.
3. Assert. An environment with neither is one where no System anchor can be honored.

On Unix the resolver returns `/` unchanged, so this is a Windows-only implementation behind one
accessor.

## Exit criteria

- [ ] `TargetOrder`'s System root comes from a platform resolver, not a literal. `/` remains the
      Unix answer; Windows resolves to the system **volume** root.
- [ ] Both documentation sites corrected — `%SystemRoot%` replaced by the drive root in
      `adopt_cmd.go`'s help text and `batch.go`'s comment — since they currently promise a scope the
      code never implemented.
- [ ] **A Windows test proves the root is volume-absolute**: `filepath.VolumeName(root) != ""` and
      the path does not begin with a bare separator. This is the assertion that would have caught the
      defect, and it is cheap on every platform.
- [ ] **A Windows test proves it does not follow the working directory** — resolve the System root
      with the process's working directory on a non-system volume, or with the current drive
      changed, and assert the answer is unmoved. Without this case the fix is untested against the
      actual failure mode.
- [ ] The deploy scenario grows a `System/` fixture, so the scope stops being the one deployment
      path with no end-to-end coverage. Scoping it to a writable subtree (rather than requiring
      elevation) is part of the step's design work.
- [ ] `writ.targets.system` — should the configuration override land with this step — defaults to
      the resolver rather than to a literal.

## Open questions

1. ~~**What does the System scope mean on Windows without elevation?**~~ **Answered 2026-08-16: it
   means you run writ with administrator privileges.** Deploying under `C:\` outside user-writable
   subtrees requires an elevated process, exactly as `/` does on Unix, and the answer is the same —
   run it with the privileges the target needs. The difference is mechanical, not conceptual:
   elevation on Windows is a separate process launch under UAC rather than a `sudo` prefix, so
   "elevate this one command" is not available to an operator mid-session.

   **Three consequences for this step:**
   - It does **not** block on [step 38](38-elevation-policy.md). A non-elevated System deploy fails
     with access denied, which is correct behavior and needs no policy to express. Resolving the
     root correctly is independent of holding the rights to write there.
   - The failure must read as a permissions problem, not as a path problem. Today a non-elevated
     Windows deploy could just as easily fail because the root was the wrong volume, and the two are
     indistinguishable from the error alone — which is its own argument for fixing the root first.
   - **`writ.targets.system` is what makes the scope testable.** Pointing it at a temp directory
     exercises the whole System path — classification, ordering, deployment — without elevation and
     without touching a real system drive. That is how the scenario's `System/` fixture should run,
     rather than by requiring an admin runner. (GitHub's Windows runners are widely reported to be
     administrative; the step should not depend on that without verifying it, and does not need to.)
2. **Multi-volume and UNC targets.** A layer that wants `D:\data\...` cannot express it under a
   `System/` tree anchored at `C:\`. Whether that is a real need or a hypothetical one should be
   answered before inventing syntax for it; the `writ.targets.system` override may already be the
   answer.
3. **Does `adopt` need the same resolver?** `batch.go` classifies an adopted path as System or Home
   by prefix. It currently has no Windows implementation of that test, so the classification is
   presumably as wrong as the deployment root — needs enumerating rather than assuming.

## Related

- [step 54](54-xdg-anchors-on-windows.md) — the same defect family: a path resolved against ambient
  state. Closed 2026-08-15.
- [step 59](59-xdg-search-paths-on-windows.md) — the third member of that family, found the next day:
  `ConfigDirs`/`DataDirs` return `/etc/xdg` and `/usr/local/share` on Windows, drive-relative and
  nonexistent. Also zero-exposure, also cheap now.
- [windows-native-permissions.md](../../../windows-native-permissions.md) — the campaign this gates.
- [writ-deploy-scenario.md](../../../writ-deploy-scenario.md) — records the `HOME`-only `TargetOrder`
  read as a known product gap on #91; this is its System-scope sibling.
