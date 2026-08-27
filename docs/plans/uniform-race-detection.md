---
title: "Uniform race detection across every platform leg"
issue: https://github.com/NobleFactor/devlore-cli/issues/XX
status: draft
created: 2026-08-27
updated: 2026-08-27
---

# Plan: Uniform race detection across every platform leg

## Summary

`ci.yaml` special-cased windows-arm64: a `race:` column in the matrix, two mutually exclusive
steps, `CGO_ENABLED` set on one path only, and Go's platform-support table transcribed into a
YAML comment. Only one fact forced any of it — Go ships no ThreadSanitizer runtime for
windows/arm64 — and encoding that fact in CI means it rots. This moves the decision into the
Makefile, where it is probed from the toolchain, and makes every leg run the identical command.

## Current State

| Component | Status |
| --- | --- |
| `matrix.race` column | ❌ Encodes a toolchain fact in CI |
| Two conditional test steps | ❌ Different job internals per platform |
| Go's support table in a comment | ❌ Drifts as ports gain support |
| Recovery when upstream ships the blob | ❌ Requires someone to remember |

## Requirements

### The probe must run with cgo enabled

Two distinct failures hide behind a rejected `-race`:

```
-race requires cgo                        CGO_ENABLED=0 — fixable
-race is not supported on windows/arm64   no TSan runtime — unfixable
```

Probing without cgo makes every platform report unsupported and silently drops race coverage
where it exists. `go list` compiles nothing, so the probe costs ~15ms.

### An uninstrumented leg must be loud

`make test-race` emits a `::warning::` annotation under GitHub Actions so the leg is visible on
the run summary, plus a plain banner locally. `GITHUB_ACTIONS` is dereferenced as
`${GITHUB_ACTIONS:-}` because `.SHELLFLAGS` carries `nounset`.

## Implementation Phases

### Phase 1: Makefile

- [x] `RACE_SUPPORTED` probe via `CGO_ENABLED=1 go list -race errors`
- [x] `test-race` branches on it, setting `race_flags` and `CGO_ENABLED`
- [x] Warning path verified for local, Actions, and supported cases

### Phase 2: ci.yaml

- [x] Drop the `race:` matrix column
- [x] Collapse the two steps into one `- name: Test / run: make test-race`
- [x] Remove the transcribed support table and the stale context-name example

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `Makefile` | Modify | Probe + branching in `test-race` |
| `.github/workflows/ci.yaml` | Modify | Uniform matrix and single step |

## Not in scope

`goleak` for goroutine-leak detection, which works on windows/arm64 where `-race` cannot. Worth
doing given the control plane and planned async sidecars, but it is a separate change.

## Open Questions

- [ ] Should a lost `-race` on a platform that previously had it fail the build rather than warn?
