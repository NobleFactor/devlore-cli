---
title: "The release run's summary says what was built, where each platform's products are, and where the installers are, with no unused token"
issue: https://github.com/NobleFactor/devlore-cli/issues/962
status: approved
created: 2026-09-29
updated: 2026-09-30
---

# Plan: the release run's summary

Lane 14 of #949, PR A of four (ruled 2026-09-29: "We'll four PRs: A, B, #959, and then C."). It touches
`.github/workflows/release.yaml`, plus the plan documents.

## Issue 962

Every release run publishes a pre-release on GitHub, and the installers download from it. They ask GitHub's
API for the newest release, or for the tag in `DEVLORE_VERSION`, and fetch its archive and checksums
(`install.sh:140-170`, `install.ps1:213-236`). What's wrong is the run's summary, `release.yaml:89-120` at
`e2653443`, which says none of that:
- **An unused token.** It prints `GITHUB_TOKEN=<token>` and `$env:GH_TOKEN = '<token>'` in its commands.
- **A dead site.** Every command names `devlore.noblefactor.com`, which is frozen and unexposed.
- **A false line.** It says a pre-release "requires explicit version".
- **No products.** It ends with `ls -la dist/`.

The owner's four requirements, 2026-09-27
([comment](https://github.com/NobleFactor/devlore-cli/issues/962#issuecomment-5896631745)), are this plan's
requirements, in the owner's order. Ruled 2026-09-29: "we need to take care of those other 4 bullets, starting
with the GITHUB_TOKEN purge."

## The summary this plan produces

This is what it would have read for the run of 2026-09-29:

```text
devlore-cli v0.1.0-dev.20260929155253, built from develop at e2653443
Release: https://github.com/NobleFactor/devlore-cli/releases/tag/v0.1.0-dev.20260929155253

Build products: lore, star and writ, in one archive per platform
  darwin/amd64   https://github.com/NobleFactor/devlore-cli/releases/download/v0.1.0-dev.20260929155253/devlore-cli_v0.1.0-dev.20260929155253_darwin_amd64.tar.gz
  darwin/arm64   https://github.com/NobleFactor/devlore-cli/releases/download/v0.1.0-dev.20260929155253/devlore-cli_v0.1.0-dev.20260929155253_darwin_arm64.tar.gz
  linux/amd64    https://github.com/NobleFactor/devlore-cli/releases/download/v0.1.0-dev.20260929155253/devlore-cli_v0.1.0-dev.20260929155253_linux_amd64.tar.gz
  linux/arm64    https://github.com/NobleFactor/devlore-cli/releases/download/v0.1.0-dev.20260929155253/devlore-cli_v0.1.0-dev.20260929155253_linux_arm64.tar.gz
  windows/amd64  https://github.com/NobleFactor/devlore-cli/releases/download/v0.1.0-dev.20260929155253/devlore-cli_v0.1.0-dev.20260929155253_windows_amd64.zip
  windows/arm64  https://github.com/NobleFactor/devlore-cli/releases/download/v0.1.0-dev.20260929155253/devlore-cli_v0.1.0-dev.20260929155253_windows_arm64.zip
  checksums      https://github.com/NobleFactor/devlore-cli/releases/download/v0.1.0-dev.20260929155253/devlore-cli_v0.1.0-dev.20260929155253_checksums.txt

Installers
  install.sh     https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net/install.sh
  install.ps1    https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net/install.ps1

Install this release
  curl --fail --silent --show-error --location https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net/install.sh | DEVLORE_VERSION=v0.1.0-dev.20260929155253 bash
  $env:DEVLORE_VERSION = 'v0.1.0-dev.20260929155253'; irm https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net/install.ps1 | iex
```

On the run's page the lists are tables and the commands are code blocks.

## Requirements

### Requirement 1: Purge the unused `GITHUB_TOKEN`

Every `GITHUB_TOKEN` in devlore-cli at `e2653443`:

| Where | What | Fate |
| --- | --- | --- |
| `release.yaml:102`, `:110` | `GITHUB_TOKEN=<token>` in the summary's install commands. `install.sh` never reads it (it reads `GH_TOKEN`, `install.sh:67`), and the repository is public | removed |
| `release.yaml:105`, `:113` | `$env:GH_TOKEN = '<token>'` in the same commands | removed |
| `release.yaml:87`; `lkg-tag.yaml:21`, `:32` | `GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}`, the workflow's own token, which `gh release create` and the tag steps authenticate with | used; stays |
| `install.sh:12` | the private-repository instruction `GH_TOKEN=$(unset GITHUB_TOKEN GH_TOKEN; gh auth token)` | obsolete since the repository went public; PR B, which rewrites the installers' headers (#950) |
| `cmd/internal/model/provider.go:136`; `docs/architecture/7.2-e2e-testing.md` | the AI-model provider's key; the end-to-end tests | unrelated to releases; stays |
| `wiki/Releasing.md:33`, `:128` | "`GITHUB_TOKEN` for releases" | true; stays |

The new summary names no token at all.

### Requirement 2: Straighten out the build output

- The summary opens with what was built: the tag (the version step's output), the branch and the commit, then
  the release page.
- Today's whole Summary step goes: the two-branch install block with its false "requires explicit version" line,
  the `devlore.noblefactor.com` commands, and the `ls -la dist/` listing.
- So does the "List artifacts" step (`:67-68`), which prints the same listing to the log.
- The false comment at `:41-44` is corrected: pre-releases aren't served by GitHub's `/releases/latest`, but the
  installers' `latest` includes them.

### Requirement 3: Where the build products are, for each platform

- One line per archive in `dist/`, with its platform and its download address. The list is taken from `dist/`
  itself, which is exactly what the release uploads, not from a second copy of the Makefile's platforms.
- The checksums file's address.
- One line saying each archive holds lore, star and writ. The Makefile's archive check enforces that
  (`Makefile:552-560`).

### Requirement 4: Where the installers are

- The addresses of `install.sh` and `install.ps1` on the site, which releases from `develop` (ruled 2026-09-26).
  The base address is stated once, in the workflow's `env:`.
- The two commands that install exactly this build, pinned to its tag, with `curl`'s long options.

The step's values reach the shell through its `env:`, not through `${{ }}` inside `run:`.

### Requirement 5: Lane 11's owed tick

`docs/plans/feature/974-writ-s-templates-end-in-tmpl.md`: Phase 4's last box is ticked with DANOBLE-UD24-1's
convergence of 2026-09-29, and the plan is set `complete`. What was checked:
- the official `v0.1.0-dev.20260929155253` installed;
- the certificate template deployed as a link;
- the rendered copy, identical to its source, removed;
- `New-LocationConfig` filled every placeholder.

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, before the change

### Phase 2: The change

- [ ] Requirements 1–4 in `release.yaml`
- [ ] Requirement 5

### Phase 3: Verify

- [ ] A dry run of the new Summary step. Extract its `run:` block from the branch's `release.yaml` and give it the
      run of 2026-09-29's values through its `env:`, with a scratch `dist/` holding that release's seven file
      names. Run it under `bash -o errexit -o nounset -o pipefail`, with `GITHUB_STEP_SUMMARY` in scratch.
      - The output matches "The summary this plan produces".
      - It contains no `GITHUB_TOKEN`, `GH_TOKEN`, `<token>`, `devlore.noblefactor.com` or short `curl` option.
- [ ] `bash -n` and `shellcheck` on the extracted block, since no CI gate reads workflow `run:` blocks
- [ ] Every address in that output answers 200. The bash command, run as printed in a scratch home and prefix,
      installs `v0.1.0-dev.20260929155253`
- [ ] CI's gate on the branch

### Phase 4: Merge and prove

- [ ] PR script written, shown, and handed over. The PR resolves #962
- [ ] After the merge: the merge commit's own release run shows the summary, naming its own tag, and its addresses
      answer 200

## Out of Scope

- **The release notes** (`:80-83`), which stay three lines.
- **Linking a PR and its pre-release,** in either direction.
- **The installers' own headers and the `No GH_TOKEN set` warning.** PR B owns them.
- **Rerunning a release run.** A branch run's rerun recomputes the timestamp and publishes a second pre-release,
  and a tag run's rerun fails because the release exists. Found drafting this plan; filed as #980.
