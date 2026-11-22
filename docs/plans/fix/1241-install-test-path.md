---
title: "The installer test's install folder stays inside Windows' path limit"
issue: https://github.com/NobleFactor/devlore-cli/issues/1241
status: active
created: 2026-11-22
updated: 2026-11-22
documents:
  - kind: Schedule
    url: https://github.com/NobleFactor/noblefactor-engprod/issues/588
    text: "Issue tracking, iteration 2"
  - kind: Related work
    url: https://github.com/NobleFactor/devlore-cli/pull/1240
    text: "The landing of the credentials check, which this unblocks"
---

# The installer test's install folder stays inside Windows' path limit

<a id="role-assignments"></a>

## 1. Role assignments

- Owner: David Noble.
- Participant: the Claude Code session assigned to Schedule NobleFactor/noblefactor-engprod#588, as coder.
- Reviewer: Codex.
- Compliance auditor: agy, the Antigravity CLI, of the Gemini model family.

<a id="summary-and-promised-result"></a>

## 2. Summary and promised result

`scripts/Test-InstallScript.ps1` installs into a scratch directory in Windows' temp folder, named `devlore-install-test-` and a 32-character ID, and `writ` clones noblefactor-engprod under it. Since noblefactor-engprod#604 added a file whose path in the repository is 118 characters, the deepest path came to 263 on GitHub's machines, past Windows' 260, and both required `install.ps1` jobs fail on every pull request. At the end, the scratch directory is `dlt-` and the commit's 7-character hash, the deepest path 221.

<a id="current-state"></a>

## 3. Current state

Read on 2026-11-22, at devlore-cli's develop 0c7e151: line 84 names the scratch directory; `Use-Account` puts each account's XDG homes under it; `writ` clones the team layer to `<scratch>\layers\data\devlore\writ\repos\noblefactor-engprod`, 144 characters on GitHub's machines. A normal install's clone directory is about 73.

<a id="design-impact"></a>

## 4. Design impact

None: the test's own layout. No design document describes it.

<a id="proposed-implementation"></a>

## 5. Proposed implementation

The scratch directory is named `dlt-` and `git -C <checkout> rev-parse --short=7 HEAD`, which tells a leftover directory's commit at a glance; when that gives no hash, outside a git checkout, `dlt-` and 7 characters of a new ID. A directory of that name left by an earlier run of the same commit is removed before the run makes it. On GitHub's machines each job is a fresh machine, so two runs never share one; on the Windows test box, two runs of one commit at once would, and we don't run two at once.

<a id="alternatives-and-challenges"></a>

## 6. Alternatives and challenges

- **A short random name alone.** Not taken: the commit names a leftover directory's run.
- **`RUNNER_TEMP` as the root, or long paths on for the test's git.** Not taken now: the shorter name alone leaves 39 characters to spare; the long-path setting is for real installs, which [David-Noble-at-work/personal#268](https://github.com/David-Noble-at-work/personal/issues/268) takes up in the Windows git configuration.

<a id="acceptance-criteria-and-evidence"></a>

## 7. Acceptance criteria and evidence

| Criterion | Observable result | Evidence/method | Environment/artifact |
| --- | --- | --- | --- |
| The test's clone fits Windows' limit | Both `install.ps1` jobs pass | This pull request's CI | GitHub's Windows machines |

<a id="required-completion-milestones"></a>

## 8. Required completion milestones

| Milestone | Required? | Expected evidence | State/result |
| --- | --- | --- | --- |
| Implementation | Yes | The commit of § 9.2 | Pending |
| Merge | Yes | The pull request and the commit it lands | Pending |
| Deployment | No: a test | None | Not required |
| Verification | Yes | Both `install.ps1` jobs passing on the pull request, and on #1240 after it merges develop | Pending |

<a id="implementation-phases"></a>

## 9. Implementation phases

<a id="phase-1-the-plan"></a>

### 9.1 Phase 1: the plan

- [x] This plan approved on 2026-11-22 (§ 12).

<a id="phase-2-the-fix"></a>

### 9.2 Phase 2: the fix

- [x] `scripts/Test-InstallScript.ps1` as § 5 sets out; the PowerShell lint clean, 4 scripts checked, none with findings.

<a id="files-and-related-assets"></a>

## 10. Files and related assets

- `scripts/Test-InstallScript.ps1`.

<a id="dependencies-overlaps-and-blockers"></a>

## 11. Dependencies, overlaps and blockers

- **NobleFactor/devlore-cli#1240,** the landing of noblefactor-engprod#613's credentials check, can't merge until this does; it takes develop once this lands.

<a id="approval-and-departures"></a>

## 12. Approval and departures

- **Approved, 2026-11-22.** David Noble directed the fix, the scratch directory named for the commit, and its resolution at once, in the session `NobleFactor/noblefactor-engprod#588 | Schedule: issue tracking, iteration 2 | Coder`. #1240 stays open, blocked, beside this pull request.

<a id="completion-review"></a>

## 13. Completion review

Pending.
