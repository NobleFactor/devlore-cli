---
title: "Get-EpicReport retires; star gh issues report is the report"
issue: https://github.com/NobleFactor/devlore-cli/issues/797
status: complete
created: 2026-09-06
updated: 2026-09-06
---

# Plan: Get-EpicReport retires; star gh issues report is the report

## Summary

The last phase of noblefactor-ops#140. The extension is deployed by writ from the base layer and
answers from the user slot with no environment variable, so the bash script it replaced can go.
This repository declares its `gh:` block in `star/config.yaml` and deletes `scripts/Get-EpicReport`.

## Goals

1. **One report, not two.** The script is deleted; the process documents in `noblefactor-ops`
   already name the extension (noblefactor-ops#177).
2. **This repository's config declares the set** — `gh.repositories` and `gh.exempt` — so the
   report run here spans both repositories and exempts #65 as the script hard-coded.
3. **Parity, one last time, before the deletion.** Byte for byte, on this repository, with the
   exemption coming from project config rather than a scratch user file.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `star gh` from the user slot | ✅ Deployed | `writ deploy common` on 2026-09-06, base layer registered |
| `star/config.yaml` `gh:` block | ❌ Missing | the script hard-coded `exempt='[65]'` |
| `scripts/Get-EpicReport` | present | 350 lines of bash around ~200 of jq |
| Process documents | ✅ Point at the extension | noblefactor-ops#177 |

## Implementation Phases

### Phase 1 — complete

- [x] `star/config.yaml` gains `gh: { repositories: [devlore-cli, noblefactor-ops], exempt: [65] }`
- [x] Parity: `Get-EpicReport --epic ResourceModel --view table --state all` against
      `star gh issues report --repo NobleFactor/devlore-cli …` — byte-identical, 63 lines, exemption
      from project config
- [x] `git rm scripts/Get-EpicReport`
- [x] Nothing else invokes it: `shell-lint.sh` discovers files by shebang; the three remaining
      mentions are plan documents, which are records and stay

**Files**: `star/config.yaml` — Modify; `scripts/Get-EpicReport` — Delete

## Observed on the way

`star gh issues audit --repo NobleFactor/devlore-cli -o json` returns `null` when there are no
faults — #825, an empty Starlark list marshalling as `null`, seen in production for the first time.
The audit was clean; the JSON says nothing.

## Related Documents

- Issue #797 — this task
- `NobleFactor/noblefactor-ops#140` — the feature this closes out
- `NobleFactor/noblefactor-ops#177` — the process documents that stopped naming the script
- `docs/plans/chore/809-report-renders-chores.md` — the script's last change, and Phase 1's parity target
