---
title: "Rerunning a release run doesn't reproduce its release"
issue: https://github.com/NobleFactor/devlore-cli/issues/980
status: active
created: 2026-10-23
updated: 2026-10-23
---

# Plan: a rerun of a release run produces the release the first run made

Lane 48 of #949. One pull request.

## Summary

A branch run of `.github/workflows/release.yaml` takes its version from the clock, so a rerun publishes a second
pre-release of the same commit; a tag run takes its version from the tag, and its rerun fails because the release
exists. Under the ruling that every operation is idempotent, a rerun is to produce the release the first run made.

## Rulings

- 2026-10-23, of a rerun that finds its release published: "A", then "Change of heart: B". A complete release is
  left as it is; one missing an asset has every asset replaced from the rerun's build; and the build is reproducible,
  so the rerun's assets are the first run's bytes.

## Current State

At `fcc43546`, the "Determine version and release type" step sets the version of a run on `main` to
`v0.1.0-rc.$(date +%Y%m%d%H%M%S)`, on `release/<x>` to `v<x>-rc.<the same>`, and on `develop` to
`v0.1.0-dev.<the same>` (`release.yaml:57-70`); a `v*` tag's version is the tag. "Create GitHub Release" runs
`gh release create <version> --target <sha> ... dist/*` (`:88-102`), which refuses a release that exists.

A rebuild of the same commit is not the same bytes: `BUILD_DATE` is the clock (`Makefile:58`), stamped into every
binary; `tar -czf` and `zip` record each file's modification time, the build's and the checkout's; and the checksums
file hashes those archives (`Makefile`, `dist-all` and `checksums`). The one rerun on record, after the upload of
`v0.1.0-dev.20261005021254` timed out (lane 24), published a release of its own.

## Requirements

### Requirement 1: The version comes from the commit

- A branch run's timestamp is the commit's committer time in UTC, `%Y%m%d%H%M%S`, read from git, not the clock. A
  rerun of a run computes the version the first run did. A tag run's version is the tag, as now.

### Requirement 2: The release step creates or completes, never duplicates or fails

- When no release has the version, the step creates it, as now.
- When the release exists and has every asset this run built, by name, the step changes nothing and says so.
- When the release exists and lacks one, the step uploads every asset this run built, replacing those there, so the
  archives and the checksums file come from one build.
- It never fails on the release existing. The step's logic moves to `.github/scripts/release-publish.sh`, so it can be
  run outside Actions.

### Requirement 3: A rebuild of a commit is the same bytes

- `BUILD_DATE` is the commit's committer time in UTC, not the clock, wherever `make` stamps it.
- The binaries are built with `-trimpath`, so no build directory is recorded in them.
- The archives are packed with every file's time set to the commit's, in name order, owned by 0:0: `tar --sort=name
  --mtime=<commit time> --owner=0 --group=0 --numeric-owner`, compressed with `gzip -n`, which records no name or time;
  `zip -X` over files touched to the commit's time, added in name order.
- The Release workflow builds with one exact Go version, written once, rather than `1.27`, which `setup-go` resolves
  to the newest 1.27 patch on the day of the run.

### Requirement 4: The tests

- The version step's logic moves to a script the workflow runs, `.github/scripts/release-version.sh`, so it can be run
  outside Actions. A test, `scripts/Test-Release.sh`, run by `make test-release`, a step of `make quality-gate` and so
  of CI's quality-gate job and `make check-all`, runs it for each ref kind against a fixed commit and compares the
  version, exactly, with the one the commit's time gives, twice, the second run equal to the first. It fails at
  `fcc43546`, where the version is the clock's.
- The test runs `make dist` twice, a few seconds apart, for this machine's platform and for windows/amd64, so a
  `.tar.gz` and a `.zip`, and compares the two checksums files byte for byte, and so every archive. It fails at
  `fcc43546`, where the build date and file times differ.
- The same test runs `release-publish.sh` against a stand-in `gh` on `PATH` that records each call and answers as a
  release would: none, one with every asset, one missing an asset. It compares the calls, exactly, with create;
  nothing; and an upload of every asset, replacing. At `fcc43546` the step is inline, so the test fails for want of
  the script.
- After the merge: the Release run of the merge commit is rerun, and the release list shows one pre-release for that
  commit, not two. This box closes after the merge, so this plan stays `active` at its last commit, as #199 allows,
  and the pull request says so.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/980-rerunning-a-release-run-doesn-t` and reviewed with the owner

### Phase 2: The tests, failing (Requirement 4)

- [x] The test of both scripts and of two builds, failing at `fcc43546` through `make test-release`: all 12 checks
      fail, the eight version checks and the three release-step checks for want of the scripts, and the build check
      because two builds seconds apart give different archive hashes

### Phase 3: The fix (Requirements 1, 2 and 3)

- [x] The version from the commit; the release step as ruled; the build reproducible; both scripts; the test
      passing. `make test-release` exits 0, all 12 checks. The build check was widened in this phase to build
      windows/amd64 as well as this machine's platform, so a `.zip` is compared as well as a `.tar.gz`; both come back
      the same bytes. `make dist` now refuses a `tar` that is not GNU tar, which `--sort` and `--mtime` need. The
      Release workflow's Go is 1.27.3, what `1.27` resolved to for the last release, and its version step runs under
      `shell: bash`, so a failed script fails the step

### Phase 4: Merge

- [ ] `make check-all`, finished green as the PR script runs it, read before this box is ticked
- [ ] PR script `~/Workspace/NobleFactor/devlore-cli.980-rerunning-a-release-run-doesn-t.bash` written, shown, and
      handed over; the PR resolves #980
- [ ] After the merge: the merge commit's Release run rerun, and one pre-release for that commit

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `.github/workflows/release.yaml` | Modify | Requirements 1, 2 and 3 (the exact Go version) |
| `.github/scripts/release-version.sh` | Create | Requirements 1 and 4 |
| `.github/scripts/release-publish.sh` | Create | Requirements 2 and 4 |
| `scripts/Test-Release.sh` | Create | Requirement 4 |
| `Makefile` | Modify | Requirement 3: the build date, `-trimpath`, the archives; Requirement 4: the test's target |

## Open Questions

None.
