---
title: "Lanes 8 and 9: repo set and unset, and the clone that goes with its registration"
issue: https://github.com/NobleFactor/devlore-cli/issues/791
status: complete
created: 2026-09-19
updated: 2026-09-19
---

# Plan: Lanes 8 and 9 of the command line schedule

## Summary

Lanes 8 and 9 of [#887](https://github.com/NobleFactor/devlore-cli/issues/887), in one pull request. A layer
has exactly one registration, so re-pointing it is the obvious intent behind registering it again — and today
that is refused, after the clone has already completed. The verbs become `set` and `unset`, the vocabulary the
shared root already uses for a keyed value, and unsetting a layer takes the clone writ made for it.

**This plan covers these two lanes and nothing else.** Anything found while working them stops the work and
goes to the owner for placement.

## Issue 791

Lane 8. `writ repo add` refuses a layer that is already registered and names `writ repo remove` in the
refusal; re-pointing a layer is therefore two commands, and the refusal arrives after a clone has been made.
Ruled 2026-09-04: a keyed registration is `set` and `unset`. Four further rulings, 2026-09-18: `unset` is
idempotent, `set` to the target a layer already has narrates `unchanged`, `--branch` stays on the URL form,
and the result record carries what changed.

## Issue 792

Lane 9. `repo add <layer> <url>` clones into `XDG_DATA_HOME/devlore/writ/repos/<layer>`; `repo remove` drops
the registration and leaves that clone behind, so the next `add` from a URL finds the directory occupied and
the operator is cleaning up writ's own state by hand. Ruled 2026-09-04: a clone writ made in its own home is
writ's to remove; a working tree the user registered by path is never touched.

Measured on `danoble-ud24-1.local`, 2026-09-18: every one of base, team and personal failed re-registration by
URL with `clone destination … already exists`, from orphans a previous session's `remove` left behind.

## Goals

- [x] Lane 8: `writ repo set` registers or re-points a layer in one command; `writ repo unset` removes a
      registration and is safe to run twice; `add`, `remove`, `rm` and `ls` are unknown commands.
- [x] Lane 9: unsetting or displacing a writ-owned clone removes it and says so; a path-registered working
      tree is never touched; `--dry-run` says what would go.
- [x] The result record carries `layer`, `state`, `source`, `root`, `owner`, `branch` and `previous`, and
      `repo list` carries `source` and `owner` on every row.

## Current State

Measured 2026-09-19 in `/Users/david-noble/Workspace/NobleFactor/devlore-cli.791-repo-set-and-unset`, at the
merge of pull request #896.

| Component | Lane | Status | Notes |
| --- | --- | --- | --- |
| `newRepoAddCmd` (`repo_cmd.go:51`) | 8 | ❌ | `add`, with a three-argument clone grammar and `--branch` |
| `newRepoRemoveCmd` (`repo_cmd.go:84`) | 8 | ❌ | `remove`, aliased `rm` |
| `newRepoListCmd` (`repo_cmd.go:98`) | 8 | ✅ | `list`, aliased `ls`; the alias goes with the others |
| `runRepoAdd` (`repo_cmd.go:142`) | 8 | ❌ | refuses an existing registration, after `resolveWorkingTreeRoot` has cloned |
| `runRepoRemove` (`repo_cmd.go:297`) | 8, 9 | ❌ | removes the symlink; errors when unregistered; leaves any clone |
| `repoRegistration` (`repo_cmd.go:339`) | 8 | ❌ | emits `{layer, root, state}`; no `source`, no `owner` |
| `cloneRepository` (`repo_cmd.go:258`) | 9 | ✅ | shells to `git` directly; the precedent for reading a remote |
| `devlore.WritReposDir()` | 9 | ✅ | the writ-owned home; the whole test for `owner` |
| anything reading a remote's URL | 8 | ❌ | nothing in the tree does: `source` needs a mechanism |
| `config set` / `config unset` (`config.go:122`, `:170`) | 8 | ✅ | the vocabulary this rename adopts, already shipping |
| §3 of the specification | 8 | ❌ | #791 says it records the set/unset rule; it does not. The rule is added there |
| `docs/guides/writ/repositories.md:54` | 9 | ❌ | "`writ repo remove` never deletes repository files" — which #792 reverses for writ-owned clones |

## Requirements

### Requirement 1: the verbs are `set` and `unset` (lane 8)

`writ repo set <layer> <working-tree-root>|<repository-url> [<working-tree-root>]` registers or re-points.
`writ repo unset <layer>` removes a registration. `writ repo list` stays. `add`, `remove`, `rm` and `ls` are
gone, not aliased: this is a greenfield product and there is no legacy to keep.

`--branch` stays, on the repository-url form only, refused on the path form as it is today.

### Requirement 2: `set` re-points, and says what changed (lane 8)

Registering a layer that is already registered is a replacement, not an error. It narrates what changed —
`personal: was ~/Workspace/Personal, now ~/env` — and emits the record of Requirement 4.

**`set` to the target a layer already has narrates `unchanged`** and emits its record. Not an error, and not
silent.

**A refusal precedes the clone.** Today `resolveWorkingTreeRoot` clones and `runRepoAdd` then refuses; every
refusal this command can make — an unknown layer, a malformed argument combination, `--branch` on the path
form — is decided before any network or filesystem work.

### Requirement 3: `unset` is idempotent, and takes writ's own clone with it (lanes 8 and 9)

Unsetting a layer that is not registered succeeds and emits `state: "unregistered"`. A command that is safe to
run twice is a command a script can use.

When the registration pointed at a **writ-owned** clone — a root under `devlore.WritReposDir()` — that clone is
removed with the registration, and the narration says so. When it pointed at a path the user registered, only
the registration goes; their tree is never touched. The same distinction governs `set` displacing a
registration.

`--dry-run` emits the record it would have emitted and removes nothing.

### Requirement 4: the record says what changed (lanes 8 and 9)

```json
{
  "layer": "personal",
  "state": "registered",
  "source": "git@github.com:me/personal.git",
  "root": "/Users/me/.local/share/devlore/writ/repos/personal",
  "owner": "writ",
  "branch": "devlore-cli/writ-layer",
  "previous": {
    "source": "git@github.com:me/personal.git",
    "root": "/Users/me/Workspace/Personal",
    "owner": "user"
  }
}
```

| Field | Meaning |
| --- | --- |
| `layer` | the layer name |
| `state` | `registered`, `unregistered`, `broken`, `unreadable` — unchanged |
| `source` | the URL writ would fetch from |
| `root` | where the working tree is, which is what writ reads |
| `owner` | `writ` or `user`: who cloned the tree, and so who may delete it |
| `branch` | present when `--branch` was given |
| `previous` | the displaced registration, same shape; absent when nothing was displaced |

Narration is stderr (§5), so `--output json` never sees "was X, now Y": whatever a script needs is in the
record. There is no `removed` field — writ clones only into `<data home>/devlore/writ/repos/<name>`, so a
removed clone is always `previous.root` with `previous.owner == "writ"`.

`repo list` gains `source` and `owner` on every row.

### Requirement 5: `source` and `owner` are derived, not stored (lane 8)

**`owner`** is `writ` when the root lies under `devlore.WritReposDir()` and `user` otherwise, decided with
`filepath.Rel` so a path that merely begins with the same characters does not qualify.

**`source`** is the URL writ would fetch from, resolved by running `git` in the working tree, as
`cloneRepository` already does:

1. the checked-out branch's upstream remote — `git -C <root> rev-parse --abbrev-ref --symbolic-full-name @{upstream}`, whose `<remote>/<branch>` names the remote;
2. else `origin`;
3. else empty.

Then `git -C <root> remote get-url <remote>`. Empty is a real answer, not a failure: a repository made by
`git init` and never pushed has no remote, and writ's own refusal text tells the user to create exactly that.
A tree that is not readable by git leaves `source` empty rather than failing the command.

### Requirement 6: the documents say set and unset (lanes 8 and 9)

- **§3 of the specification** gains the rule `#791` claims it already carries: a keyed registration is `set`
  and `unset`, on every program, and `config set`/`config unset` are its precedent.
- **`docs/guides/writ/repositories.md`** loses `add`/`remove` throughout, and its sentence "`writ repo remove`
  never deletes repository files" becomes the ownership rule: writ removes the clones it made, and never a
  tree you registered by path.
- **`docs/architecture/2.4-hermeticity-guarantees.status.md`** names the new verbs.
- **The nine documents under `docs/plans/`** that name `repo add` keep their text: they record work as it was
  done, and rewriting history to match today's vocabulary would make them lie about what shipped. Stated here
  for the owner's approval rather than done silently.

## Design

```
  writ repo set <layer> <location> [<destination>]
        |
        |  1. validate: layer, argument shape, --branch placement        <- every refusal lives here
        |  2. read the current registration, if any                      <- becomes `previous`
        |  3. resolve the root: a path, or a clone made now
        |  4. write the symlink, replacing any that was there
        |  5. displaced writ-owned clone? remove it and narrate
        |  6. emit the record
        v
  {layer, state, source, root, owner, branch, previous}
```

`unset` is steps 2, 5 and 6 with the symlink removed rather than written.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-09-19. The two open questions stand: the nine historical
      plan documents are left as they are, and `writ-e2e.ps1` was rewritten to the new verbs on the owner's
      word, though it still lives outside version control.

### Phase 2: The verbs

- [x] `newRepoSetCmd` and `newRepoUnsetCmd` replace the add/remove constructors; `list` loses its `ls` alias.
- [x] Validation precedes any clone; `set` replaces rather than refuses; `unset` is idempotent. `plannedRoot`
      answers where the tree will be without touching the network, so every refusal comes before a clone.
- [x] `repo_cmd_test.go` follows. Ten tests renamed or replaced; three added for re-pointing, `unchanged`, and
      a refusal preceding the clone; the alias test became the retirement test. The harness now runs the
      sequence `main` runs -- validate, then execute -- and separates stdout from stderr.
- [x] **Lane 8, #897**: `cli.ValidateCommandLine` in every program's `main` (star's `run`), `operandsOf` and
      its table test, §3 and §14 amended. Measured on the rebuilt binaries: every retired spelling and every
      unknown verb under every group exits 64 with cobra's own message; bare and flagged groups still print
      help at 0. `make check` exit 0, 2026-09-19.

### Phase 3: The record

- [x] `RepoRegistration` gains `Source`, `Owner`, `Branch` and `Previous`; `repoRegistration` derives the
      first two per Requirement 5. `RepoTarget{Source, Root, Owner}` is embedded, so the record reads layer,
      state, source, root, owner, branch, previous, and `Previous` is the same shape. `sourceOf` runs git for
      the upstream's remote, else `origin`, else empty; `ownerOf` is `filepath.Rel` against the writ home.
- [x] `repo list` carries them. Four tests pin owner, previous, source (no remote, unreadable tree, and a
      fixture with an `origin`), and branch. Green 2026-09-19.

### Phase 4: The clone

- [x] `unset` and a displacing `set` remove a writ-owned clone and narrate it; a user path is untouched.
      Removal goes through `removeWritClone`, which decides again from the path rather than trusting the
      record's `Owner`, and `ownerOf` calls the writ home itself nobody's -- only a strict child is writ's.
- [x] `--dry-run` emits the record and removes nothing -- no clone, no symlink, no removal -- read through
      `viper.GetBool("writ.dry-run")` as every other writ command reads it. Four tests, green 2026-09-19.

### Phase 5: The rest of the tree

- [x] The five Go sites outside `repo_cmd.go`: `secret_cmd.go:55`, `secret/encrypt.go:95` and `:121`,
      `secret/encrypt_test.go:126`, `scenario_integration_test.go:83`, and the comment at
      `cmd/internal/devlore/devlore.go:121`. The only `repo add` left in Go is in comments about its retirement.
- [x] The three live documents of Requirement 6. The guide's "`writ repo remove` never deletes repository
      files" became the ownership rule; §3 carries the set/unset sentence #791 said it already had.

### Phase 6: The three numbers

- [x] Coverage per package and total, code size, and the complexity figures themselves. Measured 2026-09-19 at
      `make check` exit 0:

      | Number | Value |
      | --- | --- |
      | Coverage, `cmd/internal/cli` | 68.3% |
      | Coverage, `cmd/writ/writ` | 40.7% package-wide; every function this branch wrote or rewrote 75-100% |
      | Coverage, total | 62.7% |
      | Complexity, highest touched | `runRepoSet` 14, down from 17 once `writeRegistration` came out; `operandsOf` 11; `runRepoUnset` 9 |
      | Complexity, over the ceiling | none |
      | Size | `groups.go` 187, `groups_test.go` 153, `repo_cmd.go` 660, `repo_cmd_test.go` 812 -- 1,812 lines across the four |

      The one thin spot named: `writeRegistration` at 75%, its `MkdirAll` failure branch untested. The guard on
      the file's only deletion, `removeWritClone`, has its own test for the refusal, the home itself, and a
      true clone.

### Phase 7: Installed and exercised

- [x] `make install`, and `writ version` names the build under test: `v0.1.0-dev.20260918183856-dirty` on
      `9aa570b7`. On this machine, installed: `repo add x /tmp` refused with `unknown command "add" for
      "writ repo"`; `list` shows base and team as `user` at their `~/Workspace` trees and personal as `writ`
      at its clone, every `source` resolved; `set personal <its root>` says `unchanged`.
- [x] **End to end on the Linux virtual machine**, `danoble-ud24-1.local`, snapshotted first (`{d8e69a4a}`),
      2026-09-19. The layers began registered by path to clones *under the writ home* -- owner `writ` -- so the
      first `unset` of each is #792 live: the clone went with the registration, and `set` by URL re-cloned
      where "clone destination already exists" had blocked every layer the day before.

      | Step | base, team | personal |
      | --- | --- | --- |
      | `unset`, twice | 0, 0 -- the second a no-op | 0, 0 |
      | `set <url>` | 0, a real clone over https | **hung**: a private repository, and the machine's credential manager answered with a browser OAuth flow that no `GIT_TERMINAL_PROMPT=0` suppresses. Killed. |
      | `set <url>` again | 0, `unchanged` | -- |
      | `set <path to the clone>` | 0, `unchanged` | -- |
      | `set ~/Workspace/Personal` | -- | 0, owner `user`, source `git@github.com:David-Noble-at-work/personal.git` from its `origin`; again, `unchanged` |
      | `deploy`, bare | 64 ([#843](https://github.com/NobleFactor/devlore-cli/issues/843)) | |
      | `deploy common --dry-run` | 0, four source collisions narrated | |
      | `upgrade` | 0, "No copied files to upgrade" | |

      Two facts from it, neither this lane's to fix. **Registering a private repository by URL on an unattended
      machine is not possible** where the credential manager opens a browser; that is the credential manager's
      behaviour, not writ's. **And I left an orphan**: killing writ along with its hung git child pre-empted
      `cloneRepository`'s own cleanup, so `repos/personal` exists there with no registration pointing at it and
      will block a future `set personal <url>`. The snapshot predates it.
- [x] **The Windows half is carried to the next pull request's sequence.** `prlctl snapshot DANOBLE-WD11-3`
      refused: the host is at 100%, 25 GB free of 3.7 TB, and a snapshot of the running machine needs its 32 GB
      of memory on disk. Nothing touched that machine. `C:\Users\david-noble\writ-e2e.ps1` carries the sequence
      in the new verbs. Ruled 2026-09-19: open the pull request as it stands; the Windows run happens on the next
      pull request, against a build that also carries the #822 fix, so deploy is expected to work there, not to
      stop at `file.mkdir`.
- [x] **The owner's row.** `writ deploy` or `writ upgrade` before the pull request opens: waived by the owner's
      order of 2026-09-19 to open it. Bare `writ deploy` exits 64 on every platform ([#843](https://github.com/NobleFactor/devlore-cli/issues/843));
      a real deploy on either virtual machine stops at `file.mkdir` on a non-directory occupant
      ([#822](https://github.com/NobleFactor/devlore-cli/issues/822)), as it did on 2026-09-18 -- a defect this
      lane does not touch and the next pull request takes.

### Phase 8: Closure

- [x] #791's, #792's and #897's acceptance boxes ticked 2026-09-19; lanes 8, 9 and 10 close when the pull
      request's `Closes` lines close them; this plan `complete` in the last commit under noblefactor-ops#199.

## Test Plan

| # | Lane | What it proves | Level | Fails when |
| --- | --- | --- | --- | --- |
| 1 | 8 | `set` registers a layer that was not registered | unit, `writ` | the new verb does not work at all |
| 2 | 8 | `set` re-points a registered layer and reports `previous` | unit, `writ` | re-pointing still refuses |
| 3 | 8 | `set` to the same target narrates `unchanged` and exits 0 | unit, `writ` | it errors, or says nothing |
| 4 | 8 | `unset` twice: both exit 0, both emit `unregistered` | unit, `writ` | the second run errors |
| 5 | 8 | `add`, `remove`, `rm`, `ls` are unknown commands, exit 64 | subprocess, `writ` | an alias survives |
| 6 | 8 | `--branch` on the path form is refused before anything is written | unit, `writ` | it clones, then refuses |
| 7 | 8 | `source` is the upstream's remote, else `origin`, else empty | unit, `writ` | a no-remote tree fails the command |
| 8 | 8 | `owner` is `writ` under the writ home and `user` elsewhere, by `filepath.Rel` | unit, `writ` | a sibling path is misread as writ's |
| 9 | 9 | `unset` of a URL-registered layer removes the clone and narrates it | unit, `writ` | the orphan of #792 survives |
| 10 | 9 | `unset` of a path-registered layer leaves the tree | unit, `writ` | a user's working tree is deleted |
| 11 | 9 | a displacing `set` removes the displaced writ-owned clone only | unit, `writ` | replacement orphans or over-deletes |
| 12 | 9 | `--dry-run` emits the record and removes nothing | unit, `writ` | the dry run acts |
| 13 | 8 | `repo list` carries `source` and `owner` | unit, `writ` | the row shape drifts from `set`'s |

## Files to Create/Modify

| File | Lane | Action |
| --- | --- | --- |
| `docs/plans/feature/791-repo-set-and-unset.md` | — | Create |
| `cmd/writ/writ/repo_cmd.go` | 8, 9 | Modify: the verbs, the record, the clone |
| `cmd/writ/writ/repo_cmd_test.go` | 8, 9 | Modify: read first, then follow |
| `cmd/writ/writ/secret_cmd.go`, `secret/encrypt.go`, `secret/encrypt_test.go` | 8 | Modify: the refusal names `repo set` |
| `cmd/writ/scenario_integration_test.go` | 8 | Modify |
| `cmd/internal/devlore/devlore.go` | 8 | Modify: one comment |
| `docs/architecture/10-command-line-interface.md` | 8 | Modify, §3 |
| `docs/guides/writ/repositories.md` | 8, 9 | Modify |
| `docs/architecture/2.4-hermeticity-guarantees.status.md` | 8 | Modify |

Measured 2026-09-19: 59 sites in 18 files name `repo add`, `repo remove` or `repo rm`. Nine of those files are
plan documents recording past work and are left alone (Requirement 6).

## Lane 8, added 2026-09-19: a group refuses a verb it does not have

Found while writing the retirement test, filed as
[#897](https://github.com/NobleFactor/devlore-cli/issues/897), and ruled into this pull request the same day.

`writ repo add team ~/x` on the renamed tree exited **0** and printed the group's help. So did
`writ repo nosuchverb`, `writ secret nosuchverb` and `writ config nosuchverb`. Measured across the built
binaries: **writ 9 of 12**, **lore 6 of 18**, **star 10 of 11** top-level commands. Only the root and leaves
exited 64.

**Two cobra decisions produce it.** `Find` consults its validator only at the root -- `legacyArgs` returns nil
for anything with a parent -- and `execute` abandons a command it cannot run *before* validating arguments:
`if !c.Runnable() { return flag.ErrHelp }` precedes `ValidateArgs` by thirteen lines, and `ExecuteC` turns
`ErrHelp` into help and a nil error.

**Two fixes were tried and rejected before the third.** Setting `cobra.NoArgs` on every group made it worse:
`Find` consults `legacyArgs` only when `Args == nil`, so a validator there disables the root-level check and is
itself never reached -- measured, not reasoned. Giving each group a `RunE` works but contradicts §3 and §14's
invariant 7, which require that a group take no action of its own.

**What ships is the order the specification already assumes: parse, validate, then run.** `Find` is exported
and free of side effects, so each program resolves the line, refuses what cobra would have accepted, and only
then executes:

```go
cmd := writ.NewRootCmd()
if err := cli.ValidateCommandLine(cmd, os.Args[1:]); err != nil {
	os.Exit(cli.ExitCode(err))
}
if err := cmd.Execute(); err != nil {
	os.Exit(cli.ExitCode(err))
}
```

`ValidateCommandLine` refuses exactly one thing: an operand standing where a group's verb belongs. The message
is cobra's own, which `isUsageError` already maps to 64. Groups keep `Args == nil`, so the root-level check
still runs, and no group gains a `Run` or `RunE`, so invariant 7 stands untouched. star validates inside `run`,
after `loadStarlarkCommands`, because an extension's groups are part of the tree.

`operandsOf` is our copy of cobra's unexported `stripFlags`, and the table test beside it is what keeps the two
in step: `writ repo --output json` must reach help, `writ repo add` must not.

**The refusal is cobra's, not a copy of cobra's.** `ValidateCommandLine` calls [cobra.NoArgs] -- the validator
`execute` would have reached had the group been runnable -- so the message under a group is the message a reader
met at the root by construction, and a future cobra rewording carries to both at once. The
"Did you mean this?" block is reproduced from [cobra.Command.SuggestionsFor], since only the four-line formatting
around it is unexported.

`runRepo`, the test harness, runs the same sequence `main` does: validate, then execute. A harness that called
only `Execute` would exercise a path no user takes -- which is how the retirement test came to fail against a
fix that was working. It also separates stdout from stderr, because `git clone` narrates to stderr and a result
assertion that decodes both reads `Cloning into ...` as JSON.

### What the binaries do, measured 2026-09-19

| Invocation | Before | After |
| --- | --- | --- |
| `writ repo add personal <tree>` | 0, help | **64** |
| `writ repo remove`, `rm`, `ls` | 0, help | **64** |
| `writ repo set`, `unset`, `list` | — | 0 |
| `writ repo`, `writ secret`, `writ config` bare | 0, help | 0, help |
| the same, `--output json` | 0, help | 0, help |
| the same, `nosuchverb` | 0, help | **64** |
| `lore manifest nosuchverb`, `star key nosuchverb` | 0, help | **64** |

star's groups include the ones its extensions contribute, since `run` validates after `loadStarlarkCommands`.

## Open questions

1. **The historical plan documents.** Nine under `docs/plans/` name the old verbs. This plan leaves them,
   treating them as the record of what shipped. The owner's to confirm.
2. **`writ-e2e.ps1` on the Windows machine** names `repo add`. It is rewritten to `set`/`unset` in Phase 7,
   but it is not in the repository — it was placed there by hand and has no home in version control.

## Related Documents

- [#887](https://github.com/NobleFactor/devlore-cli/issues/887) — the schedule; this plan is lanes 8 and 9
- [#791](https://github.com/NobleFactor/devlore-cli/issues/791), [#792](https://github.com/NobleFactor/devlore-cli/issues/792) — the lanes
- [#463](https://github.com/NobleFactor/devlore-cli/issues/463) — the feature both belong to
- [#840](https://github.com/NobleFactor/devlore-cli/issues/840) — the phantom registrations `set` must not be fooled by; lane 1 of #894
- `docs/architecture/10-command-line-interface.md` §3, §5 — the rulings applied
