---
title: "writ detects the Linux distribution: detectDistro reads /etc/os-release before it closes it"
issue: https://github.com/NobleFactor/devlore-cli/issues/959
status: active
created: 2026-09-30
updated: 2026-09-30
---

# Plan: writ detects the Linux distribution

Lane 18 of #949, alone (ruled 2026-09-29: "We'll four PRs: A, B, #959, and then C."). Lane 19, #944, follows it:
"we'll do lane 18 followed by lane 22. two prs and at the end we'll be done with selectors. we'll have common selectors
for writ and lore." (ruled 2026-09-30; #944 was lane 22 when the ruling was made). It's the first PR in `cmd/writ/writ`
that the other session waits on.

## Issue 959

`cmd/writ/writ/segment/detect.go:130` calls `iox.Close(&err, file)` directly, not deferred, between `os.Open` and
`bufio.NewScanner(file)`. The scan reads a closed file and `detectDistro` returns `""`. So the DISTRO segment has
been empty on every Linux host since `57a9fd20` (#288, 2026-07-24), and no directory with a distribution suffix has
matched anywhere. Two tests let it through: `TestDetectSegments` never checks DISTRO
(`cmd/writ/writ/segment/segment_test.go:214-231`), and `cmd/writ/writ/adopt/adopt_integration_test.go:341-343`
asserts that `ValidatePlatform("Ubuntu")` is refused, which holds on Ubuntu only while DISTRO is empty.

## Requirements

### Requirement 1: `detectDistro` reads the file, then closes it

- **A function that takes the path.** `detectDistro()` becomes one call, `readDistro("/etc/os-release")`. The new
  unexported `readDistro(path string) string` opens the file, defers `iox.Close(&err, file)` on the next line (the
  Go style guide, § 8), scans for `ID=`, and returns `capitalizeDistro` of it. It returns `""` when the file can't be
  opened or carries no `ID`, as today. Its doc comment has Parameters and Returns (§ 4).
- **Why the path.** A test has to drive the open, the scan and the close, because the bug sat between them. A parser
  that takes a string would have passed with the bug in place.
- **Nothing else changes.** DISTRO is the `ID` field, capitalized, as today. The lineage (`ID_LIKE`, so that Ubuntu
  also matches `.Debian`) and the selector grammar are #944's, the next PR. The rest of `detect.go` stays as it is.

### Requirement 2: Tests that would have caught it

- **`readDistro` against fixtures,** written into `t.TempDir()` (§ 9), in `segment_test.go` under
  `// --- readDistro ---`:
  - `ID=ubuntu` with `ID_LIKE=debian`: `Ubuntu`;
  - `ID=debian`: `Debian`;
  - a quoted `ID="fedora"`: `Fedora`;
  - an ID the table doesn't know, `ID=nixos`: `Nixos`;
  - no `ID` line: `""`;
  - no file: `""`.
- **`TestDetectSegments` checks DISTRO on Linux.** Where `/etc/os-release` carries an `ID`, DISTRO is non-empty and
  equals `readDistro("/etc/os-release")`. Every Linux CI leg is Ubuntu, so CI proves it against a real os-release.
- **They fail first.** Run against the current `detectDistro`, the fixture tests and the Linux check fail; after the
  fix they pass. `make test`, never bare `go test` (§ 9).

### Requirement 3: The adopt test that pinned the bug

`adopt_integration_test.go:341-343` means "a word the layer tree never matches is refused". It uses `Ubuntu`, which
is now matched on Ubuntu. It uses `NoSuchPlatform` instead, which no host detects.

### Requirement 4: What changes on a Linux host

- **DISTRO carries the os-release `ID`,** capitalized: `Ubuntu` on DANOBLE-UD24-1 and on every Linux CI leg.
- **A project suffixed with that ID deploys there:** `.Ubuntu` on Ubuntu, `.Debian` on Debian. personal's
  `Home/common.Debian` (#224) starts deploying on a Debian host. On Ubuntu it waits for #944's lineage.
- **`writ adopt --platform Ubuntu` is accepted on Ubuntu.**
- **Each graph records DISTRO among its segments** (`cmd/writ/writ/deploy/plan.go:378`).
- **DANOBLE-UD24-1 gains no link.** No project in base, team or personal carries `.Ubuntu`; the only suffixed
  project in the three layers is personal's `Home/common.Debian`.

### Requirement 5: PR B's owed ticks

`docs/plans/feature/950-installers-take-base-team-and.md`:

- **Phase 5's last box:** every installers job passed on #984, on all six runners and under both PowerShell editions.
- **Phase 6's last box:** release run 36653137431 synced both installers; on 2026-09-30 the site served `develop`'s
  copies (`e3b501d7`), and the documented bash line, run as printed in scratch, registered all three layers.
- **The plan is set `complete`.**

### Requirement 6: #959's documents table

The issue's `## Plan and design documents` row links this plan on `develop`, when the plan is committed.

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, before the change; #959's table links it (Requirement 6)

### Phase 2: The fix and its tests (#959)

- [ ] Requirement 2's tests, run against the current code: they fail
- [ ] Requirement 1 in `detect.go`: the tests pass
- [ ] Requirement 3
- [ ] `gofmt`; `make test`; CI's quality gate (`make vet-all`, `./build/star lint go ./...`, `make lint-all`)

### Phase 3: PR B's owed ticks

- [ ] Requirement 5

### Phase 4: Merge

- [ ] PR script written, shown, and handed over. The PR resolves #959
- [ ] After the merge, DANOBLE-UD24-1 converges (agent rule 5): the official pre-release from the site's
      `install.sh`, then `writ deploy`; the new graph records `DISTRO: Ubuntu`, and no link is added
- [ ] The other session is told that #959 has merged, and that #944 touches `cmd/writ/writ/segment` and `pkg/platform`
      next

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/writ/writ/segment/detect.go` | Modify | `readDistro`, and `detectDistro` calls it |
| `cmd/writ/writ/segment/segment_test.go` | Modify | `readDistro`'s fixtures; DISTRO on Linux |
| `cmd/writ/writ/adopt/adopt_integration_test.go` | Modify | The refused word is `NoSuchPlatform` |
| `docs/plans/feature/950-installers-take-base-team-and.md` | Modify | PR B's owed ticks; `complete` |

## Out of Scope

- **The lineage, the selector grammar, and one selector API for writ and lore:** #944, which absorbs #860. It's the
  next PR, lane 19.
- **The Go-style sweep of `cmd/writ/writ`:** the other session's.
- **`pkg/platform`'s `readOSRelease`:** it already defers its close (`pkg/platform/detect_linux.go:99`), so lore's
  detection doesn't have this bug.

## Open Questions

None.
