---
title: "install.sh exits 1 after a successful install, and leaves its download directory behind: the exit trap names a local that is gone"
issue: https://github.com/NobleFactor/devlore-cli/issues/958
status: active
created: 2026-09-26
updated: 2026-09-26
---

# Plan: install.sh's cleanup outlives main

Lane 8 of #949. Reported by the owner 2026-09-26 ("fix this … that goes on the install-script list").

## Issue 958

`main` declares `local tmp_dir` and sets `trap 'rm -rf "$tmp_dir"' EXIT`. The trap fires after `main`
returns; the local is gone, and under `set -u` the expansion fails: `install.sh: line 1: tmp_dir:
unbound variable`, exit 1. The cleanup never runs, so the download directory is left in `$TMPDIR`.
Reproduced the same day against the installer the site publishes, in scratch directories: writ
installed and answered `--version`, the script exited 1, one directory left behind.

## Goals

1. A successful install exits 0; a failed one exits non-zero as it does today.
2. The download directory is removed on every exit, success or failure.
3. Nothing else in the script changes.

## Requirement

The path lives at script scope, set once `main` creates it; the EXIT trap calls a function that removes
it when it is set:

```bash
# The download directory, removed on every exit. Script scope, not local to main: the EXIT trap runs
# after main has returned, and under set -u a local that is gone is an error (#958).
TMP_DIR=""
cleanup() {
    if [[ -n "${TMP_DIR}" ]]; then
        rm -rf "${TMP_DIR}"
    fi
}
trap cleanup EXIT
```

`main`'s `local tmp_dir; tmp_dir=$(mktemp -d); trap … EXIT` becomes `TMP_DIR=$(mktemp -d)`, and its uses
read `${TMP_DIR}`. The trap is set before anything can fail, so an early `error` exit is covered too;
the `if` keeps the function's status 0 when nothing was created.

`rm -rf` and `mktemp -d` keep their short forms: the script runs on macOS, whose BSD `rm` and `mktemp`
have no long options (the open question on personal#228's plan, not settled here).

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, before the change

### Phase 2: The change

- [ ] Requirement, in `install.sh`

### Phase 3: Verify

- [ ] Before the change, in scratch `TMPDIR`, `XDG_*` and `--prefix`: exit 1, the unbound-variable line,
      one directory left (reproduced 2026-09-26)
- [ ] After: the same run exits 0, prints no unbound-variable line, leaves `TMPDIR` empty, and the
      installed writ answers `--version`
- [ ] A failing run exits non-zero and leaves `TMPDIR` empty: `DEVLORE_VERSION=v0.0.0-no-such-tag`
- [ ] `star lint shell .` (CI's shell gate, which reads `install.sh`) passes

### Phase 4: Merge, publish

- [ ] PR script written, shown, and handed over
- [ ] After the merge: the next pre-release's sync carries the fixed `install.sh` to the site's
      `develop`; the develop environment's copy is byte-identical to `develop`'s and a scratch run from
      it exits 0

## Out of Scope

- **`install.ps1`.** Its cleanup is a `finally` inside `Main`; not affected.
- **The layer flags** — lane 9, #950, which also edits `install.sh` and takes this fix by pulling
  `develop` before its merge.
