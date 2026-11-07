---
title: "A trigger files each month's update tasks from a template"
issue: https://github.com/NobleFactor/devlore-cli/issues/1212
status: draft
created: 2026-11-07
updated: 2026-11-07
---

# Plan: a trigger files each month's update tasks from a template

## Summary

Feature #1019 updates devlore-cli's dependencies every month, as two tasks that each close when their month's pull
request merges: one for CI's actions and the golangci-lint pin (Engineering), one for the Go modules and toolchain
(Product). Nothing files them; someone has to remember. This plan adds a workflow that files the month's two tasks from
templates kept in the repository, on the first of each month and on demand from the Actions page, and files nothing
when they already exist.

## Goals

1. **On time:** each month's two update tasks are filed on the first of the month, with nobody remembering.
2. **Filed right:** each is a Task under Feature #1019, with its Area, Priority Medium, Severity N/A, and a row on
   Backlog: devlore whose Status is Open, and the run reads every value back.
3. **Never twice:** a rerun, or a run by hand after the scheduled one, files nothing.
4. **On demand:** a run from the Actions page files any month's tasks, or shows what it would file.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Feature #1019's monthly tasks | Reshaped 2026-11-07 | Task #1020 and Task #1021 are 2026-11's, filed by hand |
| A workflow that files issues | Missing | No workflow here runs on a schedule; `lkg-tag.yaml` is the one run by hand |
| The noblefactor-automation app | Present | Its secrets are set here, and `docs-publish`, `knowledge-extract` and `release` use it |
| The app's permissions | Issues write, organization projects write, **issue fields read** | It can file the tasks and add their rows, but cannot set Area, Priority or Severity (Open Questions) |
| The default branch | `develop` | GitHub runs a schedule, and shows a workflow's Run button, only from the default branch |

## Requirements

### Requirement 1: the templates

One template per monthly task, in `.github/monthly-updates/`: `ci.md` and `go-modules.md`. Each opens with a header
naming the title, with `{month}` where the month goes, and the Area; the rest is the task's body, which the bodies of
Task #1020 and Task #1021 seed.

```markdown
---
title: "Update CI, {month}: the workflows' actions and the golangci-lint pin"
area: Engineering
---
```

### Requirement 2: the script

`.github/scripts/monthly-update-tasks.sh [--month <yyyy-mm>] [--dry-run]`, beside the release scripts and in their
manner:

- The month defaults to the current month in UTC; anything that is not `yyyy-mm` is refused with status 64.
- It reads Feature #1019's sub-issues, open and closed. A template whose title for the month is already among them is
  skipped, and the run says so.
- Otherwise it files the task: type Task, parent Feature #1019, a row on Backlog: devlore, Area from the template,
  Priority Medium and Severity N/A. Every type, project, field and option is looked up by name at run time; no node ID
  is written into the script.
- It waits for the project row, which GitHub lists a moment after the issue is made, then sets its Status to Open.
- It reads every filed task back and fails when a value differs.
- `--dry-run` prints what it would file and writes nothing.

### Requirement 3: the workflow

`.github/workflows/monthly-updates.yaml`:

```yaml
on:
  schedule:
    - cron: "0 14 1 * *" # 14:00 UTC on the first of the month, 06:00 Pacific
  workflow_dispatch:
    inputs:
      month:
        description: "The month, yyyy-mm; empty for this month"
        required: false
        type: string
      dry-run:
        description: "Show what would be filed, and file nothing"
        required: false
        type: boolean
        default: false
```

One job: a token from the noblefactor-automation app for the NobleFactor organization, so it reaches the organization's
project; a checkout; and the script, given the inputs.

### Requirement 4: the test

`scripts/Test-MonthlyUpdateTasks.sh` runs the script with a stand-in `gh` first on `PATH`, which answers GitHub's
queries from fixtures and records every call; nothing reaches GitHub. `make test-monthly-updates` runs it, and the
quality gate runs that target, as it runs `test-release`.

## Implementation Phases

### Phase 1: the plan

- [ ] This plan committed and pushed on `chore/1212-monthly-update-trigger`, reviewed with the owner, and its status
      `approved`

### Phase 2: the test, then the templates and the script

- [ ] `scripts/Test-MonthlyUpdateTasks.sh`, written first and failing for the script's absence
- [ ] `.github/monthly-updates/ci.md` and `go-modules.md`
- [ ] `.github/scripts/monthly-update-tasks.sh`, and the test passing

### Phase 3: the workflow and the gate

- [ ] `.github/workflows/monthly-updates.yaml`
- [ ] `make test-monthly-updates`, run by the quality gate

### Phase 4: after the merge

- [ ] A run from the Actions page for 2026-11, which finds Task #1020 and Task #1021 and files nothing
- [ ] A run for 2026-12, by hand or on its schedule, which files that month's two tasks, read back; the comment on
      #1212 that its Evidence of completion asks for links both runs and the test

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | A month with neither task gets both, each with its template's title, Area and body, type Task, parent Feature #1019, Priority Medium, Severity N/A and its Backlog: devlore row set Open | unit, stand-in `gh` | a title, field, parent or row is wrong, or a task is skipped |
| 2 | A month whose two tasks exist, open or closed, gets neither | unit | the duplicate check is removed, or matches a title other than the month's |
| 3 | A month with one of the two gets only the other | unit | the check treats the pair as one |
| 4 | `--dry-run` writes nothing | unit | any mutation is called under `--dry-run` |
| 5 | A month that is not `yyyy-mm` is refused with status 64 | unit | the check is removed |
| 6 | A filed value that reads back different fails the run | unit | the read-back is removed |

Each row is made to fail once, by the change in its last column, before it is trusted.

**Not covered by the test:** GitHub's scheduler firing on the first of the month, which Phase 4's scheduled run shows;
and the app's permissions, an organization setting the test cannot see.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/chore/1212-monthly-update-trigger.md` | Create | This plan |
| `.github/monthly-updates/ci.md` | Create | The CI task's template |
| `.github/monthly-updates/go-modules.md` | Create | The Go modules task's template |
| `.github/scripts/monthly-update-tasks.sh` | Create | Files the month's tasks from the templates |
| `.github/workflows/monthly-updates.yaml` | Create | Runs the script monthly and on demand |
| `scripts/Test-MonthlyUpdateTasks.sh` | Create | The script's test, with a stand-in `gh` |
| `Makefile` | Modify | `test-monthly-updates`, and the quality gate running it |

## Related Documents

- Feature #1019, dependencies are updated every month, under Epic #1210, Maintenance
- Task #1020 and Task #1021, 2026-11's tasks, whose bodies seed the templates
- Feature NobleFactor/noblefactor-engprod#461, the GitHub enablers: the triggers this may later join

## Open Questions

- [ ] **Issue fields.** The noblefactor-automation app can read issue fields but not write them, so as things stand the
      trigger cannot set a task's Area, Priority or Severity. Proposed: the owner raises the app's Issue fields
      permission to write, in the organization's settings for the app. Until then, should the script file the tasks
      without the fields and fail the run loudly, so the month's work is not lost, or file nothing?
- [ ] **Where the trigger lives.** Proposed: devlore-cli's own workflow and templates now, moved under Feature
      NobleFactor/noblefactor-engprod#461 once the GitHub enablers are designed.
