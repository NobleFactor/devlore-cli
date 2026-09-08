---
title: "New-DevloreKeyVaults lives in devlore-cli's own Home/noblefactor-ops.Unix and sources the base's Declare-BashScript"
issue: https://github.com/NobleFactor/devlore-cli/issues/476
status: in-progress
created: 2026-09-07
updated: 2026-09-07
---

# Plan: New-DevloreKeyVaults lives in devlore-cli's own Home/noblefactor-ops.Unix and sources the base's Declare-BashScript

## Summary

Phase 3 of noblefactor-ops#147. devlore-cli's one bash script that sources `Declare-BashScript`,
`scripts/New-DevloreKeyVaults`, does so by PATH — `source "Declare-BashScript"` — and lives outside any
project, so nothing about where it sits says what it depends on. The rule ruled 2026-09-07: code that
depends on a repository's stack goes into a directory named for that repository; every bash script
sources `Declare-BashScript`. devlore-cli is a writ layer with a `Home` of its own, so the script moves to
`Home/noblefactor-ops.Unix/.local/bin/`, sources the sibling the merged `~/.local/bin` provides, carries the
bare directive, and is brought to spec on the way: `--help`, a man page, both completions. #476's original
exit — "the shared library ships from base; nothing outside the personal layer sources a file from it" — is
met by this and noblefactor-ops#182 together.

## Goals

1. **The script says what it depends on by where it lives**: `Home/noblefactor-ops.Unix/.local/bin`.
2. **The standard shape**: header, `# shellcheck source=Declare-BashScript`, the sibling `source`, `--help`
   through the shared parsing, a man page, bash and zsh completions.
3. **Nothing else changes**: the Azure work the script does is untouched; `scripts/` keeps its two CI helpers.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `scripts/New-DevloreKeyVaults` | 77 lines, MIT header, PATH-form source, no options | uses `require_nix`, `error`, `note`, `success`, `EX_UNAVAILABLE` |
| devlore-cli's `Home/` | `common/packages-manifest.yaml` only | the team layer's whole contribution today |
| Who names the script | nothing in the repository | no docs, no Makefile target |
| `star lint shell .` | the gate | the bare directive is unresolved there (no `-P`), which is info-level under `--severity warning` |
| noblefactor-ops#147's open question | "devlore-cli's own `Home/noblefactor-ops.Unix`, rather than the base's" | this plan answers it: devlore-cli's, because devlore-cli is a layer with a `Home` |

## Implementation Phases

### Phase 1: the move, one PR

- [x] `git mv scripts/New-DevloreKeyVaults Home/noblefactor-ops.Unix/.local/bin/New-DevloreKeyVaults`
- [x] header to Apache-2.0, as everything in this repository is; the PATH-form `source` becomes
      `source "$(dirname "$0")/Declare-BashScript" "$0" "help" "h" "$@"` with the bare directive above it;
      an Arguments section with `--help` through `usage`
- [x] `Home/noblefactor-ops.Unix/.local/share/man/man1/New-DevloreKeyVaults.1`, and completions under
      `bash-completion/completions/` and `zsh/site-functions/`
- [x] **Acceptance (lint):** `star lint shell .` clean; `shellcheck -x -P <noblefactor-ops>/Home/common/.local/bin`
      clean with the source followed; `mandoc -T lint` clean
- [ ] **Acceptance (deploy):** after `writ deploy` on this machine with devlore-cli registered as the team
      layer, `New-DevloreKeyVaults --help` answers from `~/.local/bin`

### Phase 2: the record

- [x] #476's title and body state the current ruling rather than "move the shared bin"
- [ ] noblefactor-ops#147's plan: Phase 3 ticked, the open question closed; #147 closes when the Windows
      deploy is done and the two remaining held questions (the six context scripts, the git hook) are ruled
      or filed as their own chores

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `scripts/New-DevloreKeyVaults` → `Home/noblefactor-ops.Unix/.local/bin/New-DevloreKeyVaults` | Move + modify | the placement and the shape |
| `Home/noblefactor-ops.Unix/.local/share/man/man1/New-DevloreKeyVaults.1` | Create | the man page |
| `Home/noblefactor-ops.Unix/.local/share/bash-completion/completions/New-DevloreKeyVaults` | Create | bash completion |
| `Home/noblefactor-ops.Unix/.local/share/zsh/site-functions/_New-DevloreKeyVaults` | Create | zsh completion |

## Decisions

- **devlore-cli's own `Home`, not the base's.** devlore-cli is a layer (the team layer in the worked
  example of devlore-cli#850). Content that depends on the base belongs in *its* repository-named project,
  as personal's does; the base's `Home` holds what the base ships.
- **`.Unix`.** The script needs `az` and bash and runs nowhere that git alone provides; it is not a
  git-supporting script, so it is not `common`.
- **To spec on the way.** Every bash script sources `Declare-BashScript`, and a script that does has
  `--help`, a man page and exit codes. Moving it without those would move a below-spec script.
- **Short-term the move, longer-term the port.** Ruled 2026-09-07 when the question came up: a `star devlore
  keyvault create` command would serve every platform from one implementation and needs no PowerShell twin;
  it waits on a command execution that reaches `az` on Windows without a bash. Filed as #861; this move is
  what runs until then.
- **The CI helpers stay in `scripts/`.** `Invoke-WindowsDevloreTest.sh` and `Test-GuideFrontmatter.sh` are
  repository tooling, not deployed content, and source nothing.

## Related Documents

- Issue #476 — this task; feature #475, epic #466
- noblefactor-ops#147 and its plan, `docs/plans/chore/147-declare-bashscript.md` — Phases 1, 2 and 4 done;
  this is Phase 3
- #861 — the port to `star devlore keyvault create`, the longer-term shape
- noblefactor-ops#182 — the base ships the file; personal#174 — personal's side of the same rule
- `docs/guides/development-process.md` in noblefactor-ops, "Where a script lives" — the rule

## Open Questions

None.
