---
title: "Writ Deploy Scenario"
issue: https://github.com/NobleFactor/devlore-cli/issues/346
status: draft
created: 2026-08-08
updated: 2026-08-08
---

# Plan: Writ Deploy Scenario

## Summary

An end-to-end user-journey integration test on all three platforms: a fresh user installs
devlore-cli, configures writ with only a personal repo (base and team nil), and deploys
two projects with `writ deploy noblefactor thenobles`. Migration is out of scope (ruled
2026-08-08). The scenario's source of truth is a properly structured branch on the real
`~/Workspace/Personal` repo — the dogfooding staging ground — mirrored by a checked-in
fixture for CI.

## The personal-repo branch

`~/Workspace/Personal` is a live git repo. The branch `devlore-cli/writ-layer` (name
ruled 2026-08-08: the `devlore-cli/` namespace marks it as serving this project;
`writ-layer` names what the branch makes the repo be — writ's personal layer) presents
the writ-conventional layout: **`Home/` at the top, projects under it, segment variants
as dot-suffixed siblings** (ruled 2026-08-08 — the earlier `<project>/Home/` sketch was
upside down). The dot separator is the code's convention
(`segment.ParseDirName`: `"noblefactor.Darwin.arm64" → "noblefactor", ["Darwin",
"arm64"]`); the repo's current `Home/Configs/*` names use dashes, so the scaffold maps
dash → dot:

| `Home/Configs/` (dash) | Branch `Home/` (dot) |
|---|---|
| `all`, `all-Darwin`, `all-Debian`, `all-Linux`, `all-Unix`, `all-Windows` | `all`, `all.Darwin`, `all.Debian`, `all.Linux`, `all.Unix`, `all.Windows` |
| `microsoft`, `microsoft-Unix`, `microsoft-Windows` | `microsoft`, `microsoft.Unix`, `microsoft.Windows` |
| `noblefactor`, `noblefactor-Unix` | `noblefactor`, `noblefactor.Unix` |
| `thenobles`, `thenobles-Darwin` | `thenobles`, `thenobles.Darwin` |

Four projects — `all`, `microsoft`, `noblefactor`, `thenobles` — with real content copied
from `Home/Configs/*`; the scenario deploys `noblefactor thenobles`, the extra projects
exercising project selection. Content observations from the mapping (2026-08-08):

- `noblefactor/` and `thenobles/` base directories carry **dot-prefixed content only**
  (`noblefactor/.ssh`, `thenobles/.Personal-secrets` + `.ssh`) — an initial plain-`ls`
  survey misread them as empty (corrected 2026-08-08 post-scaffold; survey directories
  with `ls -a`). The visible content is variant-only, so segment matching is still
  exercised: on macOS both variants contribute; on Linux `thenobles` contributes only
  its base dot-content. The secrets stayed encrypted at the new paths — verified by the
  `GITCRYPT` magic on the committed blobs, with structural pattern parity between
  `/Home/Configs/*/…` and `/Home/*/…` attribute rules.
- `microsoft/` carries five loose `.md` notes at its top level — home-relative, they
  would land directly in `~/` if that project were ever deployed. Inert for this
  scenario; noted for dogfooding.
- Platform-rooted trees appear exactly where expected: `all.Darwin/Library/…`,
  `all.Windows/Documents/…`, everything else under `local/…`.

## Segment conventions (verified in code, 2026-08-08)

**The separator is dots**, verified at three levels: `segment.ParseDirName` splits on
`"."` only (`segment.go:75`); no code in `segment/` or `tree/` splits directory names on
dashes; and the deploy flow routes through it (`tree/builder.go` →
`segment.MatchDirectories` → the matcher → `ParseDirName`). A dash-named directory like
`all-Darwin` parses as a *base project named* `all-Darwin` with no suffixes — and
suffix-less names match on **every** platform, so dashes would deploy everywhere if
selected, not merely fail to match.

**Segment values are not raw GOOS** (`segment.DetectSegments`):

| Segment | Derivation | Example values |
|---|---|---|
| OS | `capitalizeOS(runtime.GOOS)` | `Darwin`, `Linux`, `Windows`, `FreeBSD` — never `darwin` |
| Family | `OSFamily(OS)`, added to matchables | `Unix` (matches on both Darwin and Linux) |
| DISTRO | `/etc/os-release` ID → `capitalizeDistro`, Linux only | `Debian`, `Ubuntu`, `Fedora`, `CentOS`, `RHEL`, `Arch` |
| ARCH | `runtime.GOARCH` **raw** | `arm64`, `amd64` — lowercase, the one uncapitalized segment |
| Custom | config-defined; `WRIT_SEGMENT_<NAME>` env or `--segment` | e.g. `ROLE=desktop` |

An arch-specific variant is therefore `noblefactor.Darwin.arm64` — capitalized OS,
lowercase arch — per `ParseDirName`'s own doc example. **git-crypt**: the repo's encryption patterns are
path-scoped to `Home/Configs/*` (`.Personal-secrets/**`, `.ssh/*`), so the scaffold
extends `.gitattributes` with the `Home/*` equivalents BEFORE staging the copies —
otherwise secret files would land unencrypted at the new paths.

The branch does double duty: the scenario's local source now, and the incremental staging
ground for dogfooding — content grows on this branch until it becomes the repo's real
shape. Scaffolding it is a script the repo owner runs (phase 0); this plan's sessions
never touch the personal repo directly.

**Chartered in passing**: `docs/guides/writ/repositories.md`'s repository-structure
example shows `<project>/Home/` — upside down versus the code (`layer.go` reads
`<repo>/Home`; the segment matcher parses `Home/*` dirnames). The guide needs the
corrected shape.

## Scenario → test phases

| Scenario step | Test realization |
|---|---|
| Create a new user account | Pristine sandbox: `t.TempDir()` as `HOME` (`USERPROFILE` on Windows) + fresh `XDG_{CONFIG,STATE,DATA}_HOME` — the established house pattern |
| Install devlore-cli | Real binaries built once, placed on the sandbox `PATH`; the test shells `writ` as a subprocess |
| Configure writ (personal only) | `writ config set writ.repos.personal <repo>` (or the config file directly) — base and team unset |
| `writ deploy noblefactor thenobles` | Deploy both projects into the sandbox home |
| Verify | Links/copies land; `writ status` reports all ✓; the store holds the graph (once) + one trace + index entries; a second deploy is idempotent |

## Source resolution: branch locally, fixture in CI

The test resolves the personal repo from an environment variable
(`WRIT_SCENARIO_REPO`); when unset it falls back to the checked-in fixture
`cmd/writ/testdata/personal-repo/` that mirrors the branch's shape. Locally, the harness
materializes the branch without disturbing the owner's checkout:
`git -C $WRIT_SCENARIO_REPO archive devlore-cli/writ-layer | tar -x` into the sandbox
(branch overridable via `WRIT_SCENARIO_BRANCH`). CI always uses the fixture; when the
branch's content evolves, the fixture is refreshed deliberately.

## Test architecture

1. **Location**: `cmd/writ/scenario_integration_test.go`, subprocess-driving, its own
   make target (`make test-scenario`) so `make test` stays fast.
2. **Assertions read public surfaces only**: the sandbox filesystem, `writ status`
   output, and the execution store (`graphs/`, `traces/`, `index.jsonl`).
3. **CI**: a scenario matrix job — ubuntu + macos first, windows per Q3 — running build +
   the scenario test only; the full gate stays ubuntu.

## Open questions (rulings requested)

1. **Branch content — resolved by the phase-0 scaffold (2026-08-08, revised).** The
   branch carries the real `Home/Configs/*` content mapped to dotted project-variant
   directories; no `packages-manifest.yaml` yet (a manifest would plan real
   package-manager operations — the packaging leg gets its own scenario). The CI
   fixture mirrors the SHAPE with neutral synthetic content — real dotfiles never enter
   the public devlore-cli repo.
2. **Configuration mechanism in the test.** Drive `writ config set` (exercises the
   config surface — recommended) vs writing the config file directly (fewer moving
   parts).
3. **Windows staging.** (a) darwin+linux first, Windows as a follow-on phase riding the
   #91 audit (recommended); (b) immediate windows-latest with `continue-on-error` for
   signal.

## Phases

0. **Branch scaffold** — a script for the repo owner: creates
   `devlore-cli/writ-layer` on `~/Workspace/Personal`, extends `.gitattributes` with the
   `Home/*` git-crypt patterns, maps all thirteen `Home/Configs/*` directories to their
   dotted `Home/*` project-variant equivalents (real content), commits, and returns the
   checkout to the original branch. Staging is `git add .gitattributes Home`, safe under
   the clean-tree guard. (Owner-run; nothing automated touches the personal repo.)
   **Executed 2026-08-08** — thirteen directories mapped, secrets verified encrypted at
   the new paths.
1. **Harness + fixture** — sandbox helper (env redirect, binaries on PATH), the
   `personal-repo` fixture mirroring the branch, `WRIT_SCENARIO_REPO` resolution.
   Deliverable: `writ --help` runs green in the sandbox.
2. **Deploy leg** — configure, deploy, assert (filesystem, status, store, idempotent
   re-deploy).
3. **CI matrix** — the scenario job on ubuntu + macos.
4. **Windows** — per Q3's ruling.

## Acceptance criteria

- The scenario test passes on darwin and linux in CI (Windows per Q3 staging), driving
  only public surfaces.
- Locally, the same test runs against `~/Workspace/Personal`'s `devlore-cli/writ-layer`
  branch via `WRIT_SCENARIO_REPO`.
- No network beyond the sandbox; no LLM; no package-manager mutations (per Q1's v1
  scope).
- `make test` runtime unaffected.
