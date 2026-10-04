---
title: "The installers don't tell the user to register a layer writ already has"
issue: https://github.com/NobleFactor/devlore-cli/issues/1029
status: draft
created: 2026-10-04
updated: 2026-10-04
---

# Plan: the installers don't tell the user to register a layer writ already has

Lane 23 of #949, continued. #1002 merged as `f30c2092` (#1028), and its live check on this machine found this. Ruled
2026-10-04: "Fix it now in both installers. The PR is not complete until this work is done." So this branch also
closes the last box of #1002's plan.

## Issue 1029

An install run on a machine whose layers are already registered ends with this, for each layer not given:

```
skipped: base; to register it later:
  writ repo set base <working-tree-root>|<repository-url>
```

At `f30c2092`, a layer with no `--base`, `--team` or `--personal` location counts as skipped (`install.sh:469-471`,
`install.ps1:834-837`). Neither installer asks writ whether it already holds the layer. So the summary tells the user
to register what is registered: on this machine, `writ repo list` shows base, team and personal all `registered`.

## Requirements

### Requirement 1: A layer writ already has is reported as registered, not skipped

For each layer not given, each installer asks the writ the run installed:

```bash
writ repo list --jq '.[] | select(.layer == "<layer>" and .state == "registered") | .root' --output value
```

- **It prints a root:** the layer is already registered. The summary's line after "Registered:" reads
  `Already registered: <layers>`, in base, team, personal order. The layer gets no "skipped" line.
- **It prints nothing, or the call fails:** the layer is skipped, as today. This covers no writ installed by this run,
  a writ that can't be run, and the states `unregistered`, `broken` and `unreadable`. For a broken or unreadable layer,
  `writ repo set` is the repair, so the skipped line's advice still holds.

The two installers behave identically.

### Requirement 2: The tests prove it, in both suites

- **`scripts/Test-InstallScript.sh`:** a third run in the `layers` account, with no flags. Its summary says
  `Already registered: team personal`. Its last lines are base's skipped lines, and only base's.
- **`scripts/Test-InstallScript.ps1`:** the same case.
- The existing cases (first run, second run, no flags) are unchanged and still pass.

### Requirement 3: The documents

- Each installer's usage text says a layer not given is skipped *unless writ already has it*:
  `install.sh`'s comment and help, and `install.ps1`'s comment-based help and its usage message.
- `docs/guides/getting-started.md:66` says the same.
- #1002's plan: its last Phase 7 box is ticked, citing the live check of 2026-10-04 (`v0.1.0-dev.20261004061541`,
  build `f30c2092`, "Checksum verified"), and its status becomes `complete`.

## Implementation Phases

### Phase 1: Commit the plan

- [ ] This plan committed on `fix/1029-installers-tell-the-user-to`, and reviewed with the owner

### Phase 2: The tests, failing (Requirement 2)

- [ ] The new case in both suites, run against `f30c2092`'s installers, fails for the reason stated

### Phase 3: Both installers (Requirement 1)

- [ ] `install.sh` and `install.ps1` ask writ about each layer not given. Both suites pass: bash 5 and bash 3.2, and
      pwsh. The PowerShell gate and shell lint pass

### Phase 4: The documents (Requirement 3)

- [ ] The usage texts, the guide, and #1002's plan

### Phase 5: Merge

- [ ] PR script written, shown, and handed over; its `git add` list proven with `git add --dry-run`. The PR resolves
      #1029
- [ ] After the merge, the site serves the merged installers. An install on this machine through the published
      one-liner prints `Already registered: base team personal` and no "skipped" line

## Out of Scope

- **Re-registering a layer given on the command line** that writ already has. `writ repo set` already reports it
  unchanged, and the second-run case proves it.
- **Adding `writ deploy` to "Next steps" for an already-registered layer.** A run that changed no layer has nothing
  new to deploy.

## Decisions

- **D1. Ask the writ this run installed, not one on PATH.** It is the writ whose registrations the user just
  upgraded. When this run installed none (`DEVLORE_TOOLS=lore`), the layer is skipped, as today.
- **D2. Only `registered` counts.** Any other state needs `writ repo set` to work, which is exactly what the skipped
  line says.
- **D3. One summary line, with the layers' names and not their roots.** It matches the "Registered:" line beside it.
  `writ repo list` shows the roots.
