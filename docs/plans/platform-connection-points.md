---
title: "Platform connection points: run the execution matrix on real machines"
issue: https://github.com/NobleFactor/devlore-cli/issues/671
status: draft
created: 2026-08-25
updated: 2026-08-25
---

# Plan: platform connection points

## Summary

Declare, per developer, which machines stand in for which **platforms**, so the checks that must *execute*
can run somewhere real. On a darwin workstation that means naming a linux box and a windows box; the
matrix then builds locally, ships, runs remotely, and reports back.

**Keyed by platform, not by OS.** `windows/amd64` and `windows/arm64` are different targets — Parallels on
Apple Silicon makes that concrete — and `PLATFORM`, `build/<goos>-<goarch>/`, and `dist` already share that
spelling. Reusing it means no translation layer between "what I built" and "where I can run it".

## Why now

[#670](https://github.com/NobleFactor/devlore-cli/issues/670) rules that **CI is a strict superset of the
local checks**, with a local opt-in to everything. Measuring the gap showed the matrix is not one thing:

| Check | Needs a remote machine | Runs on darwin today |
| --- | --- | --- |
| `make build-all` | no | yes — 12s |
| `make vet-all` | no | yes — 5s |
| `make lint-all` | no | yes — 59s |
| `make test`, `make test-race` | **yes** | host only |
| `make test-scenario` | **yes** | host only |

Cross-*compilation* is pure Go and already works. Only cross-*execution* needs machines. So the static half
of `make check-all` can land without this plan, and this plan is what grows the execution half.

## The mechanism

### Where it lives

A new announced section, `connections`, in `pkg/devconfig` — the same mechanism the `runtime` section uses.
No new file format, no new discovery path, and `star config show` describes it without extra work.

It belongs to the **user's** config, never the repository's. Hostnames, accounts, and network topology are
per-developer facts; committing them would rot immediately and leak detail that does not belong in a repo.

### The shape

```yaml
connections:
  linux/amd64:   { host: build-linux }
  windows/arm64: { host: build-win, shell: pwsh }
```

**`host` is an `~/.ssh/config` alias, not a hostname.** SSH already solves user, port, identity, jump host,
agent forwarding, and multiplexing, and the developer already maintains that file. Restating any of it here
would create a second source of truth that drifts out of step with the first.

The section carries only what SSH config cannot express. Today that is the remote shell: Windows needs
`pwsh` where the default would be `cmd`.

### The rule that matters most

**An unreachable connection point fails loudly. It never skips.**

A matrix that silently drops a platform because a box is down reproduces exactly the defect this campaign
has been unwinding — a gate reporting green while not running. The two states are different and must stay
different:

- **no entry for a platform** — not configured; reported as such, not an error
- **an entry that will not connect** — a failure, not an omission

### A companion command

`star connections check` (name provisional) proves every configured point is reachable and can run a
binary, so the matrix never discovers a dead box mid-run. It is also the thing a developer runs once after
editing their config, rather than learning by way of a failed test sweep.

## Phases

### Phase 1 — the config section — status: pending

`connections` announced in `pkg/devconfig`, platform-keyed, validated: keys must parse as `<goos>/<goarch>`
and name a supported platform; `host` is required; `shell` is optional and defaults per platform.

### Phase 2 — reachability verification — status: pending

`star connections check`. Connects to each configured point, confirms it can execute a trivial binary
built for that platform, and reports per-platform. Exit non-zero if any configured point fails.

### Phase 3 — the execution matrix consumes it — status: pending

`make check-all` grows its execution half: for each selected platform that is not the host, ship the
binaries already in `build/<goos>-<goarch>/`, run the suite there, report back. Builds on
[#598](https://github.com/NobleFactor/devlore-cli/issues/598)'s build-once/test-many, which is what makes
the binaries available in the first place.

## Open questions

1. **Does an entry name a platform or a machine?** One box able to emulate might serve `linux/amd64` and
   `linux/arm64`. Do we allow one entry to claim several platforms, or keep the mapping one-to-one?
2. **Where do binaries land remotely** — a fixed path, a per-run temp directory, or XDG on the remote?
3. **Is `shell` the only thing SSH config cannot express?** A remote working directory, an environment
   prelude, or a sudo policy may follow.
4. **Does this belong to devlore-cli or to `writ`?** `writ` already knows about machines. The instinct is
   devlore-cli — this is about testing the product, not deploying dotfiles — but the boundary is a USER
   call.

## Verification

Every phase: `make check`, `make vet` under GOOS windows and linux, `gofmt -l`. Phase 2 and 3 additionally
need a configured point to test against, which is the first thing in this campaign that cannot be verified
from a single machine.
