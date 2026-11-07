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
(Product). Nothing files them; someone has to remember. This plan adds the two tasks' templates and a workflow that
files them with `star github issue file`, on the first of each month and on demand from the Actions page, filing
nothing when they already exist. The filing itself is star's, specified in Task NobleFactor/noblefactor-engprod#338;
this repository keeps no filing script of its own (ruled 2026-11-07: no more bash or PowerShell to maintain).

## Goals

1. **On time:** each month's two update tasks are filed on the first of the month, with nobody remembering.
2. **Filed right:** each is a Task under Feature #1019, with its Area, Priority Medium, Severity N/A, and a row on
   Backlog: devlore whose Status is Open, every value read back, as `star github issue file` does.
3. **Never twice:** a rerun, or a run by hand after the scheduled one, files nothing.
4. **On demand:** a run from the Actions page files any month's tasks, or shows what it would file.
5. **No filer here:** the repository holds templates and a workflow, and nothing that talks to GitHub's API itself.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Feature #1019's monthly tasks | Reshaped 2026-11-07 | Task #1020 and Task #1021 are 2026-11's, filed by hand |
| `star github issue file` | Specified, not built | Task NobleFactor/noblefactor-engprod#338, rewritten as its spec 2026-11-07; this task waits on it |
| `star github` | Broken today | By the issue data model's changes; Task #338 waits on its repair |
| The noblefactor-automation app | Present | Its secrets are set here, and `docs-publish`, `knowledge-extract` and `release` use it |
| The app's permissions | Issues write, organization projects write, **issue fields read** | It cannot set Area, Priority or Severity until the owner raises that permission (Open Questions) |
| star and its GitHub extension in CI | Available | CI builds star in-tree as `./build/star`, and checks out the base layer, where the extension lives |
| The default branch | `develop` | GitHub runs a schedule, and shows a workflow's Run button, only from the default branch |

## Requirements

### Requirement 1: the templates

One template per monthly task, in `.github/monthly-updates/`: `ci.md` and `go-modules.md`, in the template format Task
#338 specifies: a header naming the type, parent, title with `{month}`, Area, Priority, project, Status and
`unique: true`, then the body, in the task shape. The bodies of Task #1020 and Task #1021 seed them.

```markdown
---
type: Task
parent: NobleFactor/devlore-cli#1019
title: "Update CI, {month}: the workflows' actions and the golangci-lint pin"
area: Engineering
priority: Medium
project: "Backlog: devlore"
status: Open
unique: true
---
```

### Requirement 2: the workflow

`.github/workflows/monthly-updates.yaml`, run three ways:

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
  pull_request:
    paths:
      - ".github/monthly-updates/**"
      - ".github/workflows/monthly-updates.yaml"
```

One job: a token from the noblefactor-automation app for the NobleFactor organization, so it reaches the
organization's project; checkouts of this repository and of the base layer; star built, with the base layer's GitHub
extension where star finds it; and, for each template, `star github issue file --template <template> --set
month=<month>`. A pull request that changes a template or the workflow runs the same with `--dry-run`, so a broken
template fails its pull request rather than the first of a month.

## Implementation Phases

### Phase 1: the plan

- [ ] This plan committed and pushed on `chore/1212-monthly-update-trigger`, reviewed with the owner, and its status
      `approved`

### Phase 2: waits on Task NobleFactor/noblefactor-engprod#338

- [ ] `star github issue file` built, tested and on noblefactor-engprod's `develop`, with `star github` repaired

### Phase 3: the templates and the workflow

- [ ] `.github/monthly-updates/ci.md` and `go-modules.md`
- [ ] `.github/workflows/monthly-updates.yaml`, its pull-request dry run passing on this branch's pull request

### Phase 4: after the merge

- [ ] A run from the Actions page for 2026-11, which finds Task #1020 and Task #1021 and files nothing
- [ ] A run for 2026-12, by hand or on its schedule, which files that month's two tasks, read back; the comment on
      #1212 that its Evidence of completion asks for links both runs

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | Each template is one `star github issue file` accepts: every name resolves, no blank is left | the workflow's pull-request dry run | a template names a type, parent, project or option that does not exist, or leaves a blank unfilled |
| 2 | A month whose tasks exist gets none | Phase 4's run for 2026-11 | `unique` is dropped from a template |
| 3 | A month with neither task gets both, read back | Phase 4's run for 2026-12 | the workflow skips a template, or files under the wrong month |

Row 1 is made to fail once, by a template naming a parent that does not exist, before it is trusted.

**Covered by Task #338, not here:** the filing itself, its refusals, its duplicate check and its read-back, which the
command's own tests prove against a stand-in `gh`. **Not covered by a test:** GitHub's scheduler firing on the first of
the month, which Phase 4's scheduled run shows; and the app's permissions, an organization setting.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/chore/1212-monthly-update-trigger.md` | Create | This plan |
| `.github/monthly-updates/ci.md` | Create | The CI task's template |
| `.github/monthly-updates/go-modules.md` | Create | The Go modules task's template |
| `.github/workflows/monthly-updates.yaml` | Create | Files the month's tasks monthly and on demand, and dry-runs a changed template |

## Related Documents

- Feature #1019, dependencies are updated every month, under Epic #1210, Maintenance
- Task #1020 and Task #1021, 2026-11's tasks, whose bodies seed the templates
- Task NobleFactor/noblefactor-engprod#338, `star github issue file`, which this waits on

## Open Questions

- [ ] **Issue fields.** The noblefactor-automation app can read issue fields but not write them, so until the owner
      raises that permission to write, a filing cannot set a task's Area, Priority or Severity. What the command does
      then is Task #338's question.
- [ ] **The extension in CI.** How the workflow puts the base layer's GitHub extension where star finds it: copied from
      the base-layer checkout CI already makes, or deployed by writ from it.
- [x] **Where the trigger lives.** Ruled 2026-11-07: in this repository, which keeps the templates and the workflow.
      The work applies to a repository, not a team, because each repository's maintenance tasks differ. The filing is
      star's, so nothing is copied but templates; there is no filer here to copy (ruled 2026-11-07).
