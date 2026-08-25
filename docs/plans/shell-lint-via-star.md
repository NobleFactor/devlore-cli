---
title: "Shell lint runs on every check-in, through star"
issue: https://github.com/NobleFactor/devlore-cli/issues/672
status: draft
created: 2026-08-25
updated: 2026-08-25
---

# Plan: shell lint via star, on every check-in

## Goal

**Shell checks run on every commit, through `star`, with no path that reports green without checking.**
`.github/scripts/shell-lint.sh` is retired once nothing depends on it.

## Why this is not a one-line swap

Investigating the swap turned up three defects. Two of them make a check report success while doing
nothing, which is the failure this whole thread has been about.

1. **`star hook pre-commit` is dead.** It fails immediately with
   `lint.go: unexpected keyword argument "path" (did you mean paths?)` — `hook-pre-commit.star:52` passes
   `path=` where the parameter is `paths=`, and `:81` passes `path=` to `lint.shell` where the parameter is
   `files=`. The Go check dies first, so the shell check never runs at all. This is very likely why
   `.githooks/pre-commit` is hand-written bash and why pre-commit hooks were written off as buggy.

2. **`lint.Shell` reports success on an empty file list.**
   `if len(files) == 0 { return ShellResult{Passed: true}, nil }`. A discovery gap therefore surfaces as a
   pass, not as a failure.

3. **`star lint shell` discovers by extension only.** `lint-shell.star`'s `collect_files` globs `*.sh`,
   `*.bash`, `*.zsh` and hands `lint.shell` an explicit list. Combined with (2), a tree of extensionless
   scripts lints clean having checked nothing — and this project's convention is extensionless scripts.

The capability itself is not missing: `cmd/star/provider/shellcheck/provider.go`'s `isShellFile` already
matches the three extensions **or** a `#!/…/{bash,sh,zsh}` / `env {bash,sh,zsh}` shebang **or** a
`# shellcheck shell=` directive — a strict superset of what `shell-lint.sh` detects, plus `vendor` and
`node_modules` pruning the script lacks. It belongs to a different entry point than the one the extension
calls.

## Steps

| # | Step | Done |
| --- | --- | --- |
| 1 | `hook-pre-commit.star:52` — `lint.go(path=…)` becomes `paths=` | ☐ |
| 2 | `hook-pre-commit.star:81` — `lint.shell(path=…)` becomes `files=` | ☐ |
| 3 | `lint.Shell` stops returning `Passed: true` for an empty file set — an empty set is an error | ☐ |
| 4 | `lint-shell.star` — `collect_files` stops globbing; discovery uses the shebang/directive rule | ☐ |
| 5 | Discovery prunes `.devlore`, `build`, `dist` alongside `.git`, `vendor`, `node_modules` | ☐ |
| 6 | `make shell-lint` invokes `$(STAR) lint shell .` and depends on `$(STAR)` | ☐ |
| 7 | `.githooks/pre-commit` calls `make shell-lint` rather than `.github/scripts/shell-lint.sh` | ☐ |
| 8 | Delete `.github/scripts/shell-lint.sh` | ☐ |
| 9 | Pin it: a fixture with a deliberately bad extensionless script fails the lint | ☐ |

Steps 1–3 are the defect fixes and stand on their own merit. Steps 4–5 restore the discovery the provider
already implements. Steps 6–8 are the swap you asked for. Step 9 is what keeps 3 and 4 from regressing
quietly.

## Order matters

**Step 8 last, and only after 4.** Deleting the script before discovery is fixed would silently drop
`.githooks/pre-commit` — a live, tracked, extensionless script that `shell-lint.sh` lints today and
`star lint shell` does not — from every check. That is the invisible-removal pattern this campaign exists
to stop.

**Step 3 before 4.** If discovery is fixed while an empty set still reports green, a later regression in
discovery goes quiet rather than red.

## Verification

Each step: `make check`, `gofmt -l`. Step 9 is the real proof — a bad extensionless script must fail the
lint, and after step 7 must fail a commit.

Whole-plan exit: `.github/scripts/shell-lint.sh` no longer exists, `git commit` runs shell checks through
star, `star lint shell .` finds `.githooks/pre-commit`, and an empty discovery set is an error rather than
a pass.

## Related

- [#670](https://github.com/NobleFactor/devlore-cli/issues/670) — CI as a strict superset of local; this
  removes one of its three violations, the two shell-lint implementations
