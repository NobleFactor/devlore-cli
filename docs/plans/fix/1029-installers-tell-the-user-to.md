---
title: "The installers don't tell the user to register a layer writ already has, writ doesn't call an empty directory a layer, and the installers are tested against the programs beside them"
issue: https://github.com/NobleFactor/devlore-cli/issues/1029
status: active
created: 2026-10-04
updated: 2026-10-04
---

# Plan: the installers, writ's layers, and the installers tested against the programs beside them

Lane 24 of #949. #1002 merged as `f30c2092` (#1028), and its live check on this machine found #1029. Ruled
2026-10-04: "Fix it now in both installers. The PR is not complete until this work is done." So this branch also
closes the last box of #1002's plan.

#1029's tests found #1030, ruled the same day: "Fix writ in this pull request", "(a) with adopt's message corrected".
Fixing the two together found #1031, ruled: the suites test the programs in the same commit, and "Run them on every
pull request."

## Issue 1029

An install run on a machine whose layers are already registered ends with this, for each layer not given:

```
skipped: base; to register it later:
  writ repo set base <working-tree-root>|<repository-url>
```

At `f30c2092`, a layer with no `--base`, `--team` or `--personal` location counts as skipped (`install.sh:469-471`,
`install.ps1:834-837`). Neither installer asks writ whether it already holds the layer.

## Issue 1030

On a machine where no layer was ever registered, `writ repo list` reports all three as `registered`, each at writ's
own empty directory. `self install` creates `base`, `team` and `personal` as empty directories under
`WritLayersDir()` (`cmd/internal/cli/selfinstall.go:1153-1172`). `repoRegistration` (`cmd/writ/writ/repo_cmd.go:645-666`)
calls anything there that exists and resolves `registered`. The other readers of that directory decide for
themselves, each its own way: deploy's `getConfiguredRepo` (`cmd/writ/writ/commands.go`), `reconcile`'s
`layerStatuses`, the secrets code's `registeredLayers`, and adopt's layer check (`cmd/writ/writ/config.go:255-257`),
which tells the user to run `writ self install` "to create layers".

A layer is a git working tree: `writ repo set` refuses anything else, because "deploy pins layers from git history"
(`validateWorkingTree`, `cmd/writ/writ/repo_cmd.go:385-405`). An empty directory is never one.

## Issue 1031

The installer suites install the newest *published* release, so a pull request's installers are tested against the
last release's programs, never its own. And the Installers workflow runs only when an installer file changes, so a
program change alone never runs them. A fix needing an installer change and a program change together, as #1029 and
#1030 do, can't pass CI in one pull request.

## Requirements

### Requirement 1: A layer writ already has is reported as registered, not skipped (#1029)

For each layer not given, each installer asks the writ the run installed:

```bash
writ repo list --filter layer=<layer> --filter state=registered --jq '.[].root' --output value
```

`--filter`, not a `--jq` select: Windows PowerShell 5.1 strips the double quotes inside a native command's
arguments, and a select needs them.

- **It prints a root:** the summary's line after "Registered:" reads `Already registered: <layers>`, in base, team,
  personal order. The layer gets no "skipped" line.
- **It prints nothing, or the call fails:** the layer is skipped, as today. For a broken or unreadable layer,
  `writ repo set` is the repair, so the skipped line's advice holds.

The two installers behave identically.

### Requirement 2: A layer is registered only when it resolves to a git working tree (#1030)

- **One check.** `cmd/internal/devlore` gains the rule every reader uses: a layer's entry in `WritLayersDir()` is
  - `registered` when it resolves to a git working tree, the test `validateWorkingTree` makes;
  - `unregistered` when nothing is there, or a directory is there that is not a git working tree;
  - `broken` when a link is there that doesn't resolve, or resolves to something that is not a git working tree;
  - `unreadable` when a link is there that can't be read.
- **Every reader uses it:** `repo list` and `repo set`'s view of the previous registration, deploy's layer lookup,
  `reconcile` (`unregistered` reads as `absent`, `broken` and `unreadable` as `broken-link`), the secrets code, and
  adopt.
- **`self install` creates no layer directories.** `initWritLayers` and its summary lines go. `writ repo set` already
  creates `WritLayersDir()`, and replaces an empty directory in the layer's place.
- **adopt into an unregistered layer refuses** with `layer "<layer>" is not registered; register it with writ repo
  set <layer> <working-tree-root>|<repository-url>`.
- **Go tests** prove each state, each reader, `self install` creating nothing, and adopt's message. Tests that build
  layers from plain directories gain a `.git`, as a real layer has.

### Requirement 3: The installer suites install the programs in the same commit (#1031)

- **Build.** Each suite runs `make dist PLATFORM=<this machine> DEVLORE_VERSION=<the fixture's tag>`: the archive and
  checksums file the release job builds, packed the same way, for the machine the suite runs on.
- **Serve.** The suite's stand-in for GitHub serves them as the newest release. Every case that installs runs against
  it, the layer cases included, so the installer under test installs the writ under test.
- **GitHub itself** is asked only by the cases that test GitHub's own answers: a release it doesn't have, and the name
  its download host serves. The guide's checksum line is checked against the built release.
- **Both suites,** `scripts/Test-InstallScript.sh` and `scripts/Test-InstallScript.ps1`. Each says in its help that it
  needs Go and GNU make.

### Requirement 4: The Installers workflow runs on every pull request, with Go (#1031)

- `paths:` goes from both triggers in `.github/workflows/installers.yaml`.
- Each of its seven jobs sets up Go as `ci.yaml` does, and on macOS GNU make 3.82+, before its tests.
- The workflow's header comment says what the suites now test.

### Requirement 5: The tests for #1029

- **`scripts/Test-InstallScript.sh`:** a third run in the `layers` account, with no flags. Its summary says
  `Already registered: team personal`. Its last lines are base's skipped lines, and only base's.
- **`scripts/Test-InstallScript.ps1`:** the same case.
- The existing cases are unchanged and pass. Against the built writ, the no-flags case in a fresh account again ends
  with three skipped layers, which proves #1030 end to end.

### Requirement 6: The documents

- Each installer's usage text says a layer not given is skipped *unless writ already has it*: `install.sh`'s help,
  and `install.ps1`'s comment-based help and usage message. `docs/guides/getting-started.md:66` says the same.
- Any guide or reference that says `self install` creates the layer directories, or that adopt needs them, is
  corrected. The CLI reference is regenerated, never edited.
- #1002's plan: its last Phase 7 box is ticked, citing the live check of 2026-10-04 (`v0.1.0-dev.20261004061541`,
  build `f30c2092`, "Checksum verified"), and its status becomes `complete`.

## Implementation Phases

### Phase 1: Commit the plan

- [x] The plan committed on `fix/1029-installers-tell-the-user-to`, and reviewed with the owner: #1029 approved
      2026-10-04 ("we'll address the issue while it's fresh in my mind"), #1030 and #1031 ruled the same day

### Phase 2: The tests, failing (Requirements 2, 5)

- [x] #1029's case in both suites, run against `f30c2092`'s installers: fails for the reason stated (no "Already
      registered" line; personal's lines last)
- [x] #1030's Go tests, failing against `f30c2092`'s writ for the reason stated: `repo list` calls empty
      directories and a link to a tree with no `.git` registered; `self install` leaves the layers directory; adopt names
      `self install` and accepts an empty directory; deploy, `reconcile` and the secrets code take an empty directory as a
      layer. The fixtures that registered plain directories as layers gain a `.git`

### Phase 3: writ (Requirement 2)

- [ ] The one check, its readers, `self install`, and adopt. `make test`, `make vet-all`, `make lint-all` and
      `star lint go` pass; `gofmt -l` lists nothing

### Phase 4: The suites build what they test (Requirements 3, 4)

- [ ] Both suites build and serve the commit's programs; the workflow runs on every pull request, with Go

### Phase 5: Both installers (Requirements 1, 5)

- [ ] `install.sh` and `install.ps1` ask writ about each layer not given. Both suites pass: bash 5, bash 3.2, and
      pwsh. The PowerShell gate and shell lint pass

### Phase 6: The documents (Requirement 6)

- [ ] The usage texts, the guides, the regenerated reference, and #1002's plan

### Phase 7: Merge

- [ ] PR script written, shown, and handed over; its `git add` list proven with `git add --dry-run`. The PR resolves
      #1029, #1030 and #1031
- [ ] After the merge: the Installers workflow ran on the pull request; the site serves the merged installers; and an
      install on this machine through the published one-liner prints `Already registered: base team personal` and no
      "skipped" line

## Out of Scope

- **Re-registering a layer given on the command line** that writ already has. `writ repo set` already reports it
  unchanged, and the second-run case proves it.
- **Adding `writ deploy` to "Next steps" for an already-registered layer.** A run that changed no layer has nothing
  new to deploy.
- **Removing the empty layer directories `self install` left on existing machines.** writ reads them as
  `unregistered`, and `writ repo set` replaces one in place.

## Decisions

- **D1. The installers ask the writ this run installed, not one on PATH.** It is the writ whose registrations the
  user just upgraded. When this run installed none (`DEVLORE_TOOLS=lore`), the layer is skipped, as today.
- **D2. Only `registered` counts as already registered.** Any other state needs `writ repo set`, which is what the
  skipped line says.
- **D3. One summary line, with the layers' names and not their roots.** It matches the "Registered:" line beside it.
- **D4. The one check lives in `cmd/internal/devlore`, beside `WritLayersDir()`.** `reconcile` and `secret` are
  packages under `cmd/writ/writ`, which imports them, so they can't import a check that lives there.
- **D5. The suites build with `make dist`,** the release job's own target, so the archive under test is packed exactly
  as a release is. A hand-built archive would test a packing no user receives.
