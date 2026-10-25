---
title: "CI exercises os-release's lineage only on Ubuntu"
issue: https://github.com/NobleFactor/devlore-cli/issues/995
status: approved
created: 2026-10-25
updated: 2026-10-25
---

# Plan: CI deploys on Debian, Kali, Fedora, Rocky and Alpine, not Ubuntu alone

Lane 49 of #949. One pull request.

## Summary

CI reads a real os-release only on Ubuntu: both Linux legs of the `test` and `scenario` jobs run Ubuntu 24.04
(`ID=ubuntu`, `ID_LIKE=debian`), so the layer-journey scenario's lineage step asserts one thing, that what deploys to
Debian deploys to Ubuntu. No leg deploys on Debian itself, on the Fedora family, on an `ID` writ does not list, or on a
distribution with no listed ancestor, and no Linux leg checks that `common.Debian` stays off a host outside Debian's
lineage. A new CI job runs `make test-scenario` in one container per chain shape, and the fixture gains Fedora-family
consumers so the Rocky and Fedora legs have something to assert.

## Rulings

- 2026-10-25, of #995's kind: "make this a task and get it in now." #995 was a chore; it is a task under feature #463.

## Current State

At `6c2ba599`, `.github/workflows/ci.yaml`'s `scenario` job runs `make test-scenario` on six runners, the Linux two
`ubuntu-latest` and `ubuntu-24.04-arm`. `cmd/writ/scenario_layer_journey_test.go` lists its selector words, `Darwin`,
`Linux`, `Debian` and `Unix`, in four places: the consumers expected at commit A (`:563`), the move to personal-b
(`:619`), step 2.3's absence check (`:959`) and step 2.3b's lineage (`:977`). The fixture
(`cmd/writ/testdata/layer-journey/personal-a/Home/`) has `common.Debian`, eight files: `Get-DebianScenario` and
`Test-DebianScenario` under `local/bin`, their bash and zsh completions and their man pages; nothing for the Fedora
family. #995 sets out each chain shape and what the code does with it.

## Requirements

### Requirement 1: A job that runs the scenarios on five distributions

- `ci.yaml` gains a `distribution` job, one leg per image, each on `ubuntu-latest`: `debian:stable`,
  `kalilinux/kali-rolling`, `fedora:latest`, `rockylinux/rockylinux:10` and `alpine:latest`. Every leg reports
  independently (`fail-fast: false`).
- A leg runs `.github/scripts/scenario-in-container.sh <image>`, which runs the image with `docker run` over the
  checked-out tree, not as a container job: GitHub's JavaScript actions, `checkout` and `setup-go` among them, cannot
  run inside an Alpine image. In the container the script installs what `make test-scenario` needs and the image
  lacks, with the image's own package manager (apt, dnf or apk): bash, GNU make 3.82+, git, GNU getopt, curl and tar;
  installs Go 1.27.3 from go.dev, checked against the SHA-256 go.dev publishes for it; prints `/etc/os-release`; and
  runs `make test-scenario` as a user it creates, not as root.
- The job reports without blocking a merge: the "Cross-platform required checks" ruleset does not name it.

### Requirement 2: Fedora-family consumers in the fixture

- The fixture gains `common.Fedora` and `common.RHEL`, each the eight files `common.Debian` has, renamed:
  `Get-FedoraScenario`, `Test-FedoraScenario`, `Get-RHELScenario` and `Test-RHELScenario`, with their completions and
  man pages.
- Each of the scenario's four word lists gains `Fedora` and `RHEL`. Step 2.3 then asserts the Fedora-family consumers
  absent on the Debian-lineage hosts, and `common.Debian`'s absent on the Fedora-family ones; step 2.3b asserts
  `common.Fedora` and `common.RHEL` deployed on Rocky.

### Requirement 3: The evidence

- On this machine (Ubuntu, arm64), `make test-scenario` passes with the new fixture: the Fedora-family consumers
  absent, `common.Debian` deployed.
- On the pull request, each new leg's result is read from its log, with its `/etc/os-release` and the steps it ran.
  The expected results, from #995's reading of the code:

  | Image | Chain | Expected |
  | --- | --- | --- |
  | `debian:stable` | `Unix`, `Linux`, `Debian` | `common.Debian` deploys by `ID`; step 2.3b skips, no `ID_LIKE` |
  | `kalilinux/kali-rolling` | `Unix`, `Linux`, `Debian`, `Kali` | `common.Debian` deploys through `ID_LIKE`; the Fedora family absent |
  | `fedora:latest` | `Unix`, `Linux`, `Fedora` | `common.Fedora` deploys; `common.Debian` and `common.RHEL` absent |
  | `rockylinux/rockylinux:10` | `Unix`, `Linux`, `Fedora`, `CentOS`, `RHEL`, `Rocky` | `common.Fedora` and `common.RHEL` deploy; `common.Debian` absent |
  | `alpine:latest` | `Unix`, `Linux`, `Alpine` | none of the three; `platform.Detect` refuses the host, and what writ then does is measured |

- A leg that does not do what the table says is a finding, put to the owner before anything changes to make it pass.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `feature/995-ci-exercises-os-release-s` and reviewed with the owner

### Phase 2: The fixture (Requirement 2)

- [ ] `common.Fedora` and `common.RHEL`; the four word lists; `make test-scenario` passing on this machine

### Phase 3: The job (Requirement 1)

- [ ] `scenario-in-container.sh` and the `distribution` job; shellcheck and the shell lint passing

### Phase 4: Merge

- [ ] `make check-all`, finished green as the PR script runs it, read before this box is ticked
- [ ] PR script `~/Workspace/NobleFactor/devlore-cli.995-ci-exercises-os-release-s.bash` written, shown, and handed
      over; the PR resolves #995. This box's commit, the pull request's last, sets this plan `complete`, unless a
      leg's result is a finding still before the owner

CI on the pull request is the script's gate, not a box here, and the new job's legs are read there (Requirement 3).
The PR script merges only when every required check has passed; the new job is not required, so its legs are read
before the merge, from the pull request, and reported.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `.github/workflows/ci.yaml` | Modify | Requirement 1: the `distribution` job |
| `.github/scripts/scenario-in-container.sh` | Create | Requirement 1 |
| `cmd/writ/testdata/layer-journey/personal-a/Home/common.Fedora/**` | Create | Requirement 2 |
| `cmd/writ/testdata/layer-journey/personal-a/Home/common.RHEL/**` | Create | Requirement 2 |
| `cmd/writ/scenario_layer_journey_test.go` | Modify | Requirement 2: the four word lists |

## Open Questions

None.
