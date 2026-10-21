---
title: "copyFile discards the destination's close error, so a failed write of star's extensions passes"
issue: https://github.com/NobleFactor/devlore-cli/issues/1049
status: active
created: 2026-10-21
updated: 2026-10-21
---

# Plan: copyFile returns the destination's close error

Lane 38 of #949. One pull request.

## Summary

`copyFile` (`cmd/internal/cli/selfinstall.go`) closes its destination with `defer func() { _ = dest.Close() }()`. A
write the operating system reports only at close, a full disk or a network filesystem's, leaves a short file and
returns nil. `CopyDir` copies with it, and star's post-install hook copies its extensions with `CopyDir`. The rest of
the file closes with `iox.Close(&err, ...)`.

## Current State

At `c8a31a44`: `copyFile` opens the source, creates the destination through `fsroot.Dir.Create`, which returns an
`*os.File`, copies with `io.Copy`, and discards both close errors. Its tests cover a copy and a missing source.

## Requirements

### Requirement 1: The destination's close error is returned

- The copy and the destination's close move into `copyAndClose(dest io.WriteCloser, source io.Reader) (err error)`,
  which closes with `defer iox.Close(&err, dest)`, as the rest of the file closes; `copyFile` calls it.
- The source's close stays discarded: it was only read, and a read-only close reports nothing about the copy.
- A test gives `copyAndClose` a writer whose writes succeed and whose close fails, and checks the close error comes
  back; it fails against the discarding close and passes with `iox.Close`.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/1049-copyfile-discards-the` and reviewed with the owner: "commit the plan and go",
      2026-10-21

### Phase 2: The fix (Requirement 1)

- [x] `copyAndClose`, and `copyFile` through it; `TestCopyAndClose_ReturnsTheCloseError` failed against the
      discarding close ("copyAndClose returned <nil>; the destination's close failed") and passes with `iox.Close`;
      `make test` passes

### Phase 3: Merge

- [x] `make check-all`, finished green as the PR script runs it, before the script is called ready: at `b5f4e74c`, with
      no prebuilt release
- [x] PR script written, shown, and handed over; the PR resolves #1049
- [ ] CI on the pull request, every leg; nothing acts on the checks while the script's gate watches them

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/internal/cli/selfinstall.go` | Modify | Requirement 1 |
| `cmd/internal/cli/selfinstall_test.go` | Modify | Requirement 1 |

## Decisions

- **D1. A helper over an `io.WriteCloser`, not a fake `fsroot.Dir`.** `Dir.Create` returns an `*os.File`, and a real
  file's close cannot be made to fail in a test without its writes failing first, which `io.Copy` would report on its
  own. The helper is the seam where a closer that fails only at close can be handed in.

## Open Questions

None.
