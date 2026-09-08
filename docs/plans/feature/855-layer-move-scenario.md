---
title: "The layer journey as a scenario: self install, repo set, deploy, the move, on every platform"
issue: https://github.com/NobleFactor/devlore-cli/issues/855
status: in-progress
created: 2026-09-07
updated: 2026-09-08
---

# Plan: The layer journey as a scenario: self install, repo set, deploy, the move, on every platform

## Summary

One scenario, the real `writ` binary in a pristine sandbox on every platform we ship to, replaying the
whole journey a machine takes: `writ self install`; `writ repo set` for base, team and personal, by
path and by URL, in every combination; a bare `writ deploy` that converges the implicit set; `writ
deploy thenobles` that adds a named project and is remembered; and then the move of 2026-09-07 —
the base takes over `Declare-BashScript`, personal renames its project and moves 139 files, the links
dangle until the redeploy, orphans and the store's memory of them. Every step is the **ruled**
interface. Where today's binary lacks it, the step skips by name of the issue that ships it, so the
scenario is green the day it lands and is the writ lane's executable acceptance test until then.

## Goals

1. **The journey, start to finish**: from a binary that has never run to a converged machine, through
   a layer's own history moving under the deployed links.
2. **The ruled interface, not the shipped one**: `self install`, `repo set`/`unset`, a bare `deploy`,
   named projects remembered. Each unshipped step is a skip that names its issue, never a test
   rewritten to today's verbs.
3. **Every platform, every architecture we run on**: the five legs of the `scenario` job, and a ruling
   on `darwin-amd64`.
4. **No collision with the writ lane**: a new file, new fixtures, shared helpers untouched, and a
   skip-list the lane clears one issue at a time.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `cmd/writ/scenario_integration_test.go` | one scenario, `TestWritDeployScenario_*` | sandbox, `runWrit`, `initializeRepo`, `assertLinked/Rendered/Absent`, `segmentOS`; layers registered through the layers-dir symlink, `repo add` for personal |
| `cmd/scenario/selfinstall_scenario_test.go` | self install / uninstall per tool | the piece Part 0 starts from |
| Fixture | `cmd/writ/testdata/personal-repo` | one layer, 14 files, no scripts, no helper |
| CI `scenario` job | five legs, `fail-fast: false` | darwin-arm64, linux-amd64 (24.04), linux-arm64, windows-amd64, windows-arm64 |
| darwin/amd64 | built, never run | `arm64-build-and-test-matrix.md` Requirement 2: deliberate |
| `writ self install` | creates empty `layers/base`, `layers/team` | #840: `repo list` reads them as registrations, `repo add` refuses |
| `writ repo add`/`remove` | shipped | #791 rules `set`/`unset`; `set` replaces and narrates; a URL registration clones into writ's home, named as `git clone` names it (#793) |
| `writ deploy` | requires a project; `common` implicit | #843 (zero-argument refusal); #850 (implicit set = `common` + one project per configured layer repository; named projects added and remembered) |
| `writ decommission` | by name, no re-converge | #851 |
| What a move does to a deployment | measured 2026-09-07 | Part 3's table |

## Requirements

### Fixtures, `cmd/writ/testdata/layer-journey/`

| Tree | Holds |
| --- | --- |
| `base/` | `Home/common/.local/bin/Declare-BashScript` — the real file from noblefactor-ops at `dee8e84e5`, Apache-2.0 — with man page and bash and zsh completions; `Home/common/.local/bin/git-scenario`, git-supporting, sources the sibling, prints its arguments |
| `team/` | `Home/common/.config/scenario/team.conf`; no packages (#762) |
| `personal-a/` | `Home/common/.local/bin/Declare-BashScript` (same bytes) and the `local/bin` bridge symlink; two consumers each under `common.{Darwin,Linux,Debian,Unix}/local/bin` with man page and completions; `noblefactor/.local/bin/git-a`; `noblefactor.Unix/.local/bin/nf-unix`; `thenobles.Darwin/local/bin/tn` and `thenobles/.config/scenario/tn.conf`; `microsoft.Unix/local/bin/ms`; `common.Windows/local/bin/w.ps1`; `Inventory/host.txt` outside `Home/` |
| `personal-b/` | the same repository after the move: `noblefactor-ops{,.Unix}`; every consumer under `noblefactor-ops.<selector>/.local/{bin,share}`; helper and bridge gone; `tn` and `ms` self-contained; `w.ps1` and `Inventory/` untouched |

Every consumer is the standard shape: header, `# shellcheck source=Declare-BashScript`, the sibling
`source`, `--help` through the shared parsing.

### Two modes, one test body

The scenario reads its layers from a table the sandbox builds; the steps never know which mode filled it.

**Fixture mode — the default and the PR gate.** Each tree becomes a repository *named as the real one*
— `noblefactor-ops`, `devlore-cli`, `personal` — so #793's clone naming yields the real project names
and personal's `Home/noblefactor-ops*` is implicit under #850. Each gets `git init` with the isolated
config the first scenario uses, one commit, and a bare clone under `<sandbox>/remotes/<name>.git`:
every layer has a path and a `file://` URL. Commit B is `personal-b` committed on top of A in the same
repository and pushed to its bare remote, so a by-path run sees the working tree advance and a by-URL
run sees the remote advance (#812). No environment beyond `WRIT_SCENARIO_RUN=1`, no network, runs on
forks; the five-leg `scenario` job runs it through `make test-scenario`.

**Real mode — `WRIT_SCENARIO_REAL=1`, never on the PR gate.** Defaults, each overridable by
environment: base `https://github.com/NobleFactor/noblefactor-ops.git` pinned to `dee8e84e5`; team the
running checkout by path at HEAD (`--allow-dirty` until #852); personal `~/Workspace/Personal` by path
locally, its GitHub URL in CI, at `27e53171` (A) and `54de61d6` (B) — the real move, one commit apart.
Pinning is a checkout inside the registered working tree or inside writ's clone. Real mode asserts
invariants, not file lists: the helper's provenance flipping from personal to base, the collision count
before and after, the dangling count between commits equal to the moved-plus-deleted count of
`git diff --name-status A B`, reconcile's state counts. Personal's 80 git-crypt files arrive as
ciphertext; the sandbox holds no key and nothing under `.Personal-secrets` is invoked. A second
workflow, `scenario-real.yaml`, on a schedule and on `workflow_dispatch`, the same five legs, with the
token `PERSONAL_LAYER_TOKEN` (fine-grained, contents read, that one repository) as a secret; when the
secret is absent the personal layer falls back to the fixture so a dispatch without secrets still runs.

### Part 0: `writ self install`

| # | Step | Assertion | Ships it |
| --- | --- | --- | --- |
| 0.1 | `writ self install` from the built binary, sandbox HOME and XDG homes | the binary is at `~/.local/bin/writ`; completions and the config skeleton are where `self install` says | — |
| 0.2 | `writ repo list` | empty: no registrations, no placeholders read as registrations | #840 |
| 0.3 | every later `writ` invocation is the self-installed one | `command -v writ` resolves under the sandbox HOME | — |

### Part 1: `writ repo set`, all combinations in turn

For each layer `L` in base, team, personal, and for each of path and URL:

| # | Step | Assertion | Ships it |
| --- | --- | --- | --- |
| 1.1 | `writ repo set L <path>` on an empty slot | registered; `repo list` shows the path; a working tree is required (#463) | #791 |
| 1.2 | `writ repo set L <url>` over it | replaced, narrated as "was …, now …"; the clone lands in writ's home named as `git clone` names it | #791, #793 |
| 1.3 | `writ repo set L <path>` over the clone | replaced; the writ-owned clone it displaces is removed; a user path never is | #791, #792 |
| 1.4 | `writ repo unset L` | gone from `repo list`; `add`, `remove`, `rm`, `ls` are unknown commands | #791 |
| 1.5 | `writ repo set` with a non-repository path | refused, naming the path | — |
| 1.6 | personal by URL; the bare remote advances one commit; the refresh verb #812 rules | the clone is brought forward and the next deploy plans from the new commit; today the clone stays where `clone` left it | #812 |

Then the **layer subsets**: each of the seven non-empty subsets of {base, team, personal} is
registered (path for odd positions, URL for even, so both kinds recur), a bare `writ deploy` runs,
and the deployed inventory equals the union of `common*` from the registered layers plus each
registered repository's own-named project — and nothing from an unregistered one. `personal` alone,
the state this machine lived in for months, is one of the seven; `base` alone, from a fresh self
install, is #477's proof that deploying base requires no base.

### Part 2: `writ deploy`, and `writ deploy thenobles`

Registered: all three layers, personal at commit A.

| # | Step | Assertion | Ships it |
| --- | --- | --- | --- |
| 2.1 | `writ deploy` (bare) | exit 0; the implicit set converges: `common*` from all three, `noblefactor` from personal; `thenobles` and `microsoft` absent | #843, #850 |
| 2.2 | collisions | exactly four source collisions narrated — the helper and its three assets, personal over base | #470 |
| 2.3 | selectors | `.Darwin` consumers on macOS only; `.Linux` and `.Debian` on Ubuntu only; `.Windows` on Windows only; `common` everywhere | — |
| 2.4 | every deployed consumer answers `--help` | exit 0 and the usage text; on Windows under Git for Windows' bash | — |
| 2.5 | `writ deploy thenobles` | adds `thenobles` and `thenobles.Darwin` (on macOS) on top; nothing else changes | #850 |
| 2.6 | `writ deploy` (bare) again | `thenobles` is still deployed — the selection is remembered | #850, persistence |
| 2.7 | `writ decommission thenobles` | its entries go; the rest re-converge; a following bare deploy does not bring it back | #851 |
| 2.8 | `writ decommission common`, `writ decommission noblefactor` | refused by name, pointing at `writ repo unset` | #851 |
| 2.9 | `writ reconcile -o json` | every entry `linked` | — |

### Part 3: the move

Personal advances from commit A to commit B, deployed. Run twice: with personal registered **by path**
(the advance is a commit in the working tree writ links into — this machine's case) and **by URL** (the
advance is a push to the bare remote, and step 3.1 first needs the clone refreshed — #812's case; until
that verb ships, the URL run skips at 3.1 by its name).

| # | Step | Assertion | Ships it / names |
| --- | --- | --- | --- |
| 3.1 | commit B, no deploy | the helper link, every consumer link and `git-a` dangle; the count equals the moved-plus-deleted files; `writ reconcile` reports them (state recorded as observed) | the merge's cost |
| 3.2 | `writ deploy --dry-run` | exit 0 and no word about occupied targets | #853 |
| 3.3 | `writ deploy --conflict=replace` | the helper resolves to the base's; no collision narrated; every consumer resolves from `~/.local/bin` and answers `--help`; the old links under `~/local` remain, dangling | #845 |
| 3.4 | `writ reconcile -o json` | `missing` equals the number of old targets | #845 |
| 3.5 | delete the dangling links by hand | `writ reconcile` still reports the same `missing` count | #845 |
| 3.6 | `writ deploy` under the default policy | exit 0: writ's own links are recognized | — |
| 3.7 | `touch Inventory/x`; `writ deploy` | refused: "layers have uncommitted changes: [personal]"; `--allow-dirty` succeeds | #852 |
| 3.8 | `writ upgrade` | selects as deploy does; regenerates the template only | #845, #850 |

Part 3's "today" rows (3.2–3.5, 3.7) assert what writ does now and carry the issue number in the
failure message: when a fix lands, the scenario is what fails, and the one-line update is the proof.

### The skip discipline

A step whose verb or behaviour is not shipped runs as `t.Skip("needs devlore-cli#<n>: <what>")`, keyed
on a capability probe (`writ repo set --help` exits 0; `writ deploy --help` shows no `MinimumNArgs`
refusal), never on a version number. The scenario prints its skip-list at the end so a run on any
platform states which rulings are still outstanding there. Nothing is rewritten to today's verbs.

### Platforms

The five legs of the `scenario` job. **Ruling needed:** `darwin-amd64` on `macos-15-intel`. The arm64
plan ruled it out deliberately (10× billing; the Intel label set closed at `macos-26-intel`); "all
architectures" reads as overriding that for this job. Both shapes are carried: five legs, or six with
`{ platform: darwin-amd64, os: macos-15-intel }` added to the `scenario` matrix only.

Windows: symlinks and `shell: bash` are already proved by the first scenario; the `common` scripts run
under Git for Windows' bash in 2.4 and 3.3. Ubuntu: `.Debian` matches through the Debian family
(`segment/detect.go`), asserted rather than assumed.

## Implementation Phases

### Phase 1: fixtures and the sandbox

- [x] `cmd/writ/testdata/layer-journey/{base,team,personal-a}`; the helper and its assets from noblefactor-ops
      `dee8e84e5` (`base/PIN`); `personal-b` is **derived, not checked in** — `applyMove` in the scenario
      performs the move on `personal-a` in the sandbox and commits it, so the two cannot drift and the
      regeneration script the draft named is unnecessary
- [x] every fixture script passes `star lint shell` and, directly, `shfmt -d -i 4 -ci` and
      `shellcheck -x -P <base>/Home/common/.local/bin` (30 scripts)
- [x] `cmd/writ/scenario_layer_journey_test.go`: a three-layer sandbox on the first scenario's pieces, each
      layer a working tree named as the real repository plus a bare remote, the bridge symlink made by the
      harness, capability probes read from help text (cobra answers `repo set --help` with exit 0 when `set`
      is unknown, so an exit code is not a probe), and the skip-list printed at the end
- [x] **Acceptance:** `TestWritLayerJourneyScenario_Harness` green on Darwin; `make test-scenario` runs the file

### Phase 2: Parts 0 and 1

- [x] self install into the sandbox (0.1 passes; 0.2 skips: self install leaves *three* placeholders,
      `base`, `personal`, `team` — recorded on #840); the registration matrix (skips as a whole on #791,
      carrying #792 and #793 inside it); the seven subsets, all green, the base-only one being #477's proof
- [x] **Acceptance:** green locally where shipped, skipping by issue where not; the skip-list is
      #791, #812, #840, #850, #851 — #843 is the shim's log line rather than a skip, #792 and #793 ride
      inside #791's step

### Phase 3: Parts 2 and 3

- [x] Part 2 as per-step subtests: 2.1–2.2 (four collisions, personal's helper wins), 2.3, 2.4 (every
      consumer answers `--help`), 2.5, 2.9 green; 2.6 skips on #850, 2.7–2.8 on #851
- [x] Part 3 by path green end to end: the dangling set after the move equals exactly the links whose
      sources moved or vanished (29 on Darwin; reconcile calls them `orphan` between the commits); the
      dry run says nothing (#853); the redeploy relinks everything to the base's helper with no collision;
      the old `~/local` links remain as orphans (#845), become `missing` in the store after the hand
      cleanup (#845), the default policy accepts writ's own links, and a file outside `Home/` forces
      `--allow-dirty` (#852). Part 3 by URL skips at 3.1 on #812
- [x] **Acceptance:** green on the six CI legs with the same skip-list on each — #859's second run, 2026-09-08,
      after its first taught #860 and the LF pin

### Phase 4a: the matrix and the record — this PR

- [x] `darwin-amd64` on `macos-15-intel` added to the `scenario` job (ruled), with a GNU getopt step for the
      macOS legs: `Declare-BashScript` parses with `getopt --long`, which macOS's BSD getopt lacks, and the
      scenario runs the deployed consumers with `--help`
- [x] `docs/plans/writ-deploy-scenario.md`, `platform-test-matrix.md` and the `test-scenario` target's
      comment name the second scenario
- [ ] #470's fixture item and #850's integration-test item point here; each skipped issue gains a comment:
      "the layer-journey scenario step <n> is your acceptance test" (#840 has its first)
- [x] **Acceptance:** the `scenario` job green on all six legs on #859, the same skip-list on each; the
      epic report ran at the end

### Phase 4b: real mode — the next PR under this issue

- [ ] `WRIT_SCENARIO_REAL=1`: the layer table from the real repositories at their pins; the invariant
      assertions; a local run against `~/Workspace/Personal` green
- [ ] `.github/workflows/scenario-real.yaml`: schedule + `workflow_dispatch`, the six legs, `PERSONAL_LAYER_TOKEN`
      optional with the fixture fallback; the token created by the owner (fine-grained, contents read,
      `David-Noble-at-work/personal` only) — a manual step, named here
- [ ] **Acceptance:** a dispatched run green on every leg with the token, and green with the fixture
      fallback without it

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/writ/scenario_layer_journey_test.go` | Create | the scenario |
| `cmd/writ/testdata/layer-journey/**` | Create | four fixture trees and the regeneration script |
| `Makefile` | Modify | `test-scenario` runs it |
| `.github/workflows/scenario-real.yaml` | Create | real mode on a schedule and on dispatch |
| `.github/workflows/ci.yaml` | Modify (if ruled) | the sixth leg |
| `docs/plans/writ-deploy-scenario.md`, `docs/plans/platform-test-matrix.md` | Modify | name the second scenario |

## Decisions

- **The ruled interface, skipped by issue where unshipped.** A scenario written to today's verbs
  would be rewritten five times; one written to the rulings is red for nothing and green the day
  each lands. The skip-list is the writ lane's to-do list, executable.
- **Assert today's behaviour in Part 3, name the issue.** Where the ruling is about what deploy
  *leaves behind* rather than a verb, the current behaviour is asserted with the issue in the
  message, so the fix's landing is visible as the scenario failing at that step.
- **Fixtures are the gate; the real repositories are a scheduled check.** Ruled 2026-09-07 ("I like the
  hybrid approach"). The PR gate must not depend on a private repository, a token, the network, or what
  personal happens to carry that day; the truth-check against real content runs on its own clock, and
  special-purpose GitHub repositories would add three things to keep in step for what the two public
  real ones already give — a clone over the network.
- **Fixture repositories carry the real names.** `noblefactor-ops`, `devlore-cli`, `personal`, so the
  implicit-project mechanism is tested with the names it will see.
- **The real helper, not a stand-in.** The scenario replays a real move; the file at its centre is the
  real file, at a named commit.
- **URLs are `file://` bare clones in the sandbox.** No network, and every platform's `git clone`
  handles them.
- **Two fixture trees, one repository, two commits.** A regeneration script keeps `personal-b` honest.
- **Real mode asserts invariants, not inventories.** Real content changes; provenance, counts and
  states do not.
- **A new file, new fixtures.** The writ lane edits the first scenario; this one shares helpers and
  touches nothing of theirs.
- **No packages.** The team layer contributes content only (#762).

## Related Documents

- Issue #855 — this task; feature #463, epic #447
- #346 and `docs/plans/writ-deploy-scenario.md` — the first scenario and its harness;
  `cmd/scenario/selfinstall_scenario_test.go` — Part 0's starting point
- `docs/plans/platform-test-matrix.md`, `docs/plans/arm64-build-and-test-matrix.md` — the matrix and the Intel ruling
- noblefactor-ops#147, personal#172 — what Part 3 replays
- #840, #791, #792, #793, #812, #843, #850, #851 — the rulings Parts 0–2 are the acceptance test for;
  #477 — the base-only subset is its proof; #470, #845, #852, #853 — the behaviours Part 3 names

## Rulings, 2026-09-07

- **darwin-amd64 runs.** "Test on darwin-amd64 on macos-15-intel for scenario and scenario-real jobs,
  overriding the arm64 plan's exclusion." Six legs for both scenario jobs; the unit-test matrix and the
  arm64 plan's build-only stance for every other job are untouched.
- **The scenario lands first.** "The scenario lands first with its skip-list and the writ lane clears it."
  The skip-list — #840, #791, #792, #793, #812, #843, #850, #851 — is the lane's executable to-do list.
- **The Linux chain follows the lineage** (2026-09-07, after the first six-leg run showed `common.Debian`
  absent on Ubuntu): "Debian is the base for Ubuntu so what deploys to Debian should deploy to Ubuntu.
  Similarly, Fedora is the base for RHEL so what deploys to Fedora should deploy to RHEL." Filed as #860:
  `Unix → Linux → ID_LIKE reversed → ID`. The scenario's selector table is the ruled chain; step 2.3b
  asserts the lineage members and skips by #860 until writ reads `ID_LIKE`. #860 joins the skip-list.

## Open Questions

- [ ] **Packages in real mode.** devlore-cli's team manifest is `packages: []` today (#821); when #796,
      #818 and #820 restore it, a real-team deploy installs packages on runners and writ has no flag to
      skip package nodes. A rule for then, not now.
