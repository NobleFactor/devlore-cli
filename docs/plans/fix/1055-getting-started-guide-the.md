---
title: "The getting-started guide, the repositories guide and the README say what is not so"
issue: https://github.com/NobleFactor/devlore-cli/issues/1055
status: active
created: 2026-10-21
updated: 2026-10-21
---

# Plan: the getting-started guide, the repositories guide and the README say what is so

Lane 41 of #949. One pull request, documentation only.

## Summary

Three published documents state things that are not so (found 2026-10-19 auditing #949).

## Current State

At `45a0dd43`:

| Where | What it says | What is so |
| --- | --- | --- |
| `docs/guides/getting-started.md`, Create your first project | `mkdir -p noblefactor/Home/.config/git`, then `mv ... noblefactor/.config/git/config` and `cp ~/.zshrc noblefactor/.zshrc` | A layer holds `Home/<project>/`, and a project's files are home-relative: `Home/noblefactor/.config/git/config` deploys to `~/.config/git/config` (the repositories guide, § Repository structure; personal's own `Home/common/`) |
| `docs/guides/writ/repositories.md`, § Repository structure | `noblefactor.Unix/local/bin/my-script`, `thenobles.Darwin/local/bin/Backup-TimeCapsule` | Without the leading dot these deploy to `~/local/bin`; `~/.local/bin` is meant |
| `README.md`, Building | Go 1.26+ | `go.mod` says `go 1.27.0`, and CI builds with 1.27 |

## Requirements

### Requirement 1: Each line says what is so

- Getting started: the project is made at `Home/noblefactor/`, and the files move and copy into it there; the
  sentence before the commands says a project is a directory under the layer's `Home/`.
- The repositories guide's two paths are `.local/bin/...`.
- The README's prerequisite is Go 1.27+.
- Getting started's later lines that the corrected layout contradicts, ruled 2026-10-21, "fix them here": the links
  `writ deploy` makes point into `~/my-environment/Home/noblefactor/`, as this machine's point into each layer's
  working tree; the example manifest is `Home/noblefactor/packages-manifest.yaml`, and lists each package as an object
  with `name` and `with`, the only form `schema/packages-manifest.json` takes (#796).

## Out of Scope

- **Getting started's `writ config set writ.repos.personal`,** which nothing reads: #1014's (amended 2026-10-19).
- **The getting-started sections after Install:** #894's replan (ruled 2026-09-30).

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/1055-getting-started-guide-the` and reviewed with the owner: "Yes, commit the plan
      and go", 2026-10-21

### Phase 2: The documents (Requirement 1)

- [x] The three, each line checked against what it describes: the layout against the repositories guide and
      personal's `Home/`; the links against this machine's (`~/.zshrc -> Workspace/Personal/Home/common.Unix/.zshrc`);
      the example manifest validated against `schema/packages-manifest.json`; Go against `go.mod`'s `go 1.27.0`

### Phase 3: Merge

- [x] `make check-all`, finished green as the PR script runs it, before the script is called ready: at `38e24af1`, with
      no prebuilt release
- [x] PR script written, shown, and handed over; the PR resolves #1055
- [ ] CI on the pull request, every leg; nothing acts on the checks while the script's gate watches them

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/guides/getting-started.md` | Modify | Requirement 1 |
| `docs/guides/writ/repositories.md` | Modify | Requirement 1 |
| `README.md` | Modify | Requirement 1 |

## Open Questions

None.
