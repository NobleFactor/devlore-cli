---
title: "The shell gate follows Declare-BashScript, and discovers through the git tree"
issue: https://github.com/NobleFactor/devlore-cli/issues/863
status: in-progress
created: 2026-09-08
updated: 2026-09-08
---

# Plan: The shell gate follows Declare-BashScript, and discovers through the git tree

## Summary

`make check` fails locally on every script that sources `Declare-BashScript`, while CI is green, because
`.github/scripts/shell-lint.sh` passes no `-P` and so never follows the source: `shellcheck` reports the
helper's variables as unassigned. One line fixes it, the line `personal`'s copy of the same script already
carries. While measuring, a second defect surfaced: discovery is a filesystem walk, so in a built clone the
gate reads 55,153 files to find 43, and its answer depends on what is lying around rather than on what CI
checks out. Discovery becomes the git tree. The gate goes from minutes to 11.7 seconds and from
build-state-dependent to reproducible.

**What this plan does not do**, deliberately: swap `make shell-lint` to `star lint shell .` and delete the
script, as the issue proposed. Measured 2026-09-08, that would drop 41 of 52 shell scripts from every
check — `.githooks/pre-commit` among them — and `docs/plans/shell-lint-via-star.md` (#672) already warns
against exactly that ordering. See Decisions.

## Goals

1. **`make check` and CI agree** on every script that sources the base layer's helper.
2. **The gate checks what CI checks out**, not what a build left behind.
3. **No coverage is lost.** The set of files linted must not shrink.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `.github/scripts/shell-lint.sh` | no `-P` | `shellcheck -x --severity=warning "$f"`; a consumer's `source=Declare-BashScript` is never followed |
| its discovery | `find .` minus `.git` | 55,153 files in a built clone, 53,536 of them untracked; `build/` alone is 1.1 GB |
| `personal`'s copy of the same script | has `-P` since #174 | `-P "${DECLARE_BASHSCRIPT_DIR:-$HOME/.local/bin}"` |
| CI's Lint Shell step | `./build/star lint shell .` | discovers by extension: 11 of 52 tracked shell scripts |
| `.githooks/pre-commit` | calls the script directly | extensionless, invisible to `star lint shell` today |

## Requirements

Two lines of substance in one file.

```bash
# discovery: the git tree, not a filesystem walk
git ls-files -z | while IFS= read -r -d '' file; do … done

# the check: follow the helper
shellcheck -x --severity=warning -P "${DECLARE_BASHSCRIPT_DIR:-$HOME/.local/bin}" "$f"
```

`-z` and `read -d ''` so a path with a space or a newline survives. `[ -f "$file" ] || continue` so a
tracked-but-deleted path is skipped rather than read.

## Implementation Phases

### Phase 1: the gate

- [x] `-P "${DECLARE_BASHSCRIPT_DIR:-$HOME/.local/bin}"` on the `shellcheck` invocation, with the comment
      saying what fails without it
- [x] discovery through `git ls-files -z`, with the comment saying why: what CI lints is what CI checks out
- [x] **Acceptance:** the gate passes on develop's head with a deployed helper — 43 files, 0 failures,
      11.7 seconds, against minutes before; `.githooks/pre-commit` is still among them

### Phase 2: the record

- [ ] noblefactor-ops `docs/guides/pr-script-template.md`'s devlore-cli row names the shell gate — deferred
      to whichever noblefactor-ops PR touches that table next, since a PR cannot cross repositories
- [ ] #672 carries the discovery measurement, so the swap it plans is made with the numbers in hand

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `.github/scripts/shell-lint.sh` | Modify | follow the helper; discover through the git tree |

## Decisions

- **Not the swap the issue proposed.** Measured over tracked files: the script finds 52 shell scripts by
  shebang, `star lint shell` finds 11 by extension. Swapping and deleting would drop 41 — including
  `.githooks/pre-commit`, `New-DevloreKeyVaults`, and all of #855's fixtures — and `star lint shell .githooks`
  answers `No shell files found` with exit 0, so the loss would be silent. #672's plan says the same in its
  own words and orders the work accordingly; its steps 1–5 are undone.
- **CI is the weaker gate, not local.** The issue assumed the reverse. Local checks 52 files, CI checks 11,
  which is a violation of #670's "CI as a strict superset of local" pointing the other way. That is #672's
  to fix; this plan does not paper over it by weakening local to match.
- **Discovery is the git tree.** A filesystem walk lints build artifacts and scratch files, so the gate's
  answer depends on build state; it also reads 55,153 files to find 43. `git ls-files` is what CI checks
  out, and it is instant.
- **`DECLARE_BASHSCRIPT_DIR` with a deployed default**, exactly as `personal` does, so the two consumers of
  the base layer state their dependency the same way.

## Related Documents

- Issue #863; NobleFactor/noblefactor-ops#147 (the helper ships from the base) and #158
- #672 and `docs/plans/shell-lint-via-star.md` — the swap, its ordering, and the discovery defects
- #670 — CI as a strict superset of local

## Open Questions

None.
