---
title: "wiki/Releasing.md describes workflows, secrets, scripts and site environments that are not there"
issue: https://github.com/NobleFactor/devlore-cli/issues/1056
status: active
created: 2026-10-21
updated: 2026-10-21
---

# Plan: wiki/Releasing.md describes the release as it is

Lane 42 of #949. One pull request, documentation only.

## Summary

`wiki/Releasing.md` describes workflows, secrets, artifacts, versions and site environments that are not so (found
2026-10-19 auditing #949). Each is checked below against the workflows, the Makefile and the live site, at
`dbc4cabf`, 2026-10-21.

## Current State

| The page says | What is so |
| --- | --- |
| Four workflows, among them `sync-knowledge.yaml` | Six: `ci.yaml`, `docs-publish.yaml`, `installers.yaml`, `knowledge-extract.yaml`, `lkg-tag.yaml`, `release.yaml`; no `sync-knowledge.yaml` |
| `ci.yaml` runs on pushes and PRs to `develop`; "go build, go vet, golangci-lint, go test, shell lint" | It runs on pushes and PRs to `develop`, `main` and `release/*`: `quality-gate` (`make quality-gate`, since #1111), `test` (`make test-race`) and `scenario` (`make test-scenario`) on six platforms |
| — | `installers.yaml`: both installer suites against this checkout's programs, on Linux, macOS (MacPorts too) and Windows under 5.1 and 7, on the same pushes and PRs |
| — | `knowledge-extract.yaml`: builds and validates the knowledge base on the same pushes and PRs; a push opens a PR to devlore-registry, a PR reports the changes |
| — | `lkg-tag.yaml`: run by hand, admins alone, to tag a PR's build last-known-good |
| `docs-publish.yaml` uses `NOBLEFACTOR_AUTOMATION` | It uses the automation app, `NOBLEFACTOR_AUTOMATION_APP_ID` and `_APP_PRIVATE_KEY`, as `release.yaml` and `knowledge-extract.yaml` do |
| Secrets `NOBLEFACTOR_AUTOMATION` and `NOBLEFACTOR_CONTENT_ACCESS`, rotated with `scripts/Update-NFSecrets` | The workflows use `NOBLEFACTOR_AUTOMATION_APP_ID`, `NOBLEFACTOR_AUTOMATION_APP_PRIVATE_KEY` and `GITHUB_TOKEN`; nothing uses `NOBLEFACTOR_CONTENT_ACCESS`; `scripts/Update-NFSecrets` does not exist |
| GoReleaser builds `.deb`, `.rpm`, Homebrew and winget assets, and runs the Portfile hook | `release.yaml` runs `make dist`: a `.tar.gz` for darwin and linux, a `.zip` for windows, amd64 and arm64, and a SHA-256 checksums file. Nothing runs goreleaser (`Makefile`, `verify-ldflags`); `.goreleaser.yaml` and `packaging/macports/` are kept, unused |
| Versions `v0.1.0-draft`, `-alpha`, `-beta`, `-rc.1`, `v1.0.0` | `release.yaml`: develop `v0.1.0-dev.<UTC stamp>` (channel develop); `main` `v0.1.0-rc.<stamp>`; `release/X.Y` `vX.Y-rc.<stamp>` (channel release); a `v*` tag its own name, a full release. Every branch build is a pre-release. `v0.1.0-draft` is `make dist`'s default when no version is given |
| Production `devlore.noblefactor.com`, sign-in "currently yes"; the develop preview needs a sign-in | Measured 2026-10-21: both environments' pages redirect to a sign-in. The develop environment, `delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net`, serves `install.sh` and `install.ps1` anonymously, matching devlore-cli's develop, and is the installers' published address (`release.yaml`'s `INSTALLERS_URL`; ruled 2026-09-26). `devlore.noblefactor.com` serves an older `install.sh` anonymously, and `install.ps1` only after a sign-in |

## Requirements

### Requirement 1: The page says what is so

- The workflows' table and details: the six workflows, their triggers, jobs and secrets, as above.
- What happens on a push or a PR, per workflow.
- The secrets: the automation app's two, and `GITHUB_TOKEN`; no `NOBLEFACTOR_CONTENT_ACCESS`, no
  `scripts/Update-NFSecrets`.
- The release artifacts: what `make dist` makes; goreleaser and the MacPorts template named as kept and not run.
- The version numbers `release.yaml` gives, and the channel each carries.
- The website's environments as measured, the develop environment named as the installers' address.

## Out of Scope

- **The production domain's installers.** `devlore.noblefactor.com` serves an older `install.sh` and puts
  `install.ps1` behind a sign-in. That is the site's: NobleFactor/devlore.noblefactor.com#504, filed 2026-10-21 on the
  owner's word.
- **Branch strategy and creating a release.** The steps are the release process as designed; no release branch has
  been cut, so they are not checked against a run, and stay.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/1056-wiki-releasing-md-describes` and reviewed with the owner: "file the site
      issue, commit the plan and go", 2026-10-21

### Phase 2: The page (Requirement 1)

- [ ] `wiki/Releasing.md`, each line checked against what it describes

### Phase 3: Merge

- [ ] `make check-all`, finished green as the PR script runs it, before the script is called ready
- [ ] PR script written, shown, and handed over; the PR resolves #1056
- [ ] CI on the pull request, every leg; nothing acts on the checks while the script's gate watches them

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `wiki/Releasing.md` | Modify | Requirement 1 |

## Open Questions

None.
