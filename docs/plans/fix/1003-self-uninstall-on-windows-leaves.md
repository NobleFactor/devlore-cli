---
title: "self uninstall finishes on Windows, keeps its record, and names a file it could not remove by its reason"
issue: https://github.com/NobleFactor/devlore-cli/issues/1003
status: draft
created: 2026-10-05
updated: 2026-10-05
---

# Plan: self uninstall finishes on Windows, and tells the truth about what it left

Lane 26 of #949.

## Issue 1003

`self uninstall`, run on Windows from the installed copy, leaves the program behind with no record of it. At
`71dc125b`:

- **It cannot remove the running `.exe`.** Windows refuses to delete a running image, so `removeRecordedFiles`
  (`cmd/internal/cli/selfinstall.go:631`) adds it to `skipped`.
- **It reports the file as modified.** `removeRecordedFiles` puts three unlike cases in one `skipped` list -- a file
  changed since it was recorded, a file the filesystem refused, and a file it could not read -- and the summary
  (`selfinstall.go:749`) calls them all "modified". The running `.exe` is none of those.
- **It then deletes the manifest** (`selfinstall.go:722`, best-effort), the record of what the tool owns. The `.exe`
  stays on disk and nothing names it, so no later `self install` or `self uninstall` can reach it.

The install path learned to replace a running image (#1001, #947): it renames one aside to `<tool>.exe.old` and
records it. Uninstall uses none of that, and in any case uninstall has no replacement to make -- it wants the file
gone, not set aside.

## The shape of the fix

Two layers, one shared by every platform and one for Windows alone.

- **Shared.** Keep the manifest while anything it records is still on disk, and name a file that was not removed by
  its real reason. This corrects Unix too: there, a permission-refused file is mislabelled "modified" today.
- **Windows only.** Delete the running image after this process exits, by handing its path to `cmd.exe`. Unix needs
  none of this: it removes a running binary's directory entry in place, and `removeRecordedFiles` already does.

## Requirements

### Requirement 1: A file that was not removed is named by its reason (all platforms)

- `removeRecordedFiles` reports, for each file it did not remove, which of these it was:
  - **changed since it was recorded** -- its hash no longer matches, so it may be the operator's own edit; left
    deliberately, as today;
  - **refused by the filesystem** -- a permission error (a prefix under `Program Files`, `/usr/local` or `/opt`
    without the rights to delete there), or any other refusal;
  - **unreadable** -- its hash could not be computed.
- The summary names each group for what it is. No group is called "modified" unless it changed.

### Requirement 2: The manifest is kept while it still records a file on disk (all platforms)

- The manifest is rewritten to the files not yet removed, never deleted outright while any remain. It is deleted only
  when nothing it recorded is left.
- So a later `self uninstall` or `self install` can finish what this one could not: the record still names every file
  on disk.

### Requirement 3: The running image is deleted after the process exits (Windows)

- The running image is the recorded `bin/<tool>.exe` that is this process's own executable, matched by
  [os.SameFile] as `installBinary` matches it (`selfinstall.go`), not by comparing paths -- a prefix reached through
  a link names the same file another way.
- It is not counted as refused. `self uninstall` hands its path to `cmd.exe` and exits; `cmd` deletes it once the
  image is released. `cmd.exe` is `%ComSpec%` -- always present, copied nowhere, left nowhere.
- The command is a retry-until-unlocked loop, not a wait on a process id, and not a fixed delay:

  ```
  cmd.exe /c "for /l %i in (1,1,<cap>) do (del /f /q "<path>" 2>nul & if not exist "<path>" exit & ping -n 2 127.0.0.1 >nul)"
  ```

  `del` fails while the image is mapped and succeeds the instant this process releases it; the loop then exits. The
  cap bounds a process that never exits. `ping`, not `timeout`, is the delay: `timeout` fails without a console,
  which is exactly the detached case.
- It is started hidden and detached (`CREATE_NO_WINDOW | DETACHED_PROCESS`), and Go builds the command line, so a
  path with spaces (`C:\Program Files\...`) is quoted once, by one builder.
- The running image stays in the rewritten manifest (Requirement 2): its deletion is scheduled, not confirmed, and
  this process is gone before it happens. If `cmd` fails, the record still names it, and a re-run -- no longer the
  running image -- removes it directly.
- The summary says the running program will be removed once it exits, not that it was skipped.

### Requirement 4: The tests

- **A Go test** of `removeRecordedFiles`' reasons: a changed file, a refused file (made unremovable by the harness),
  and an ordinary one, each landing in the right group, on every platform.
- **A Windows scenario,** on the two Windows CI legs: install writ into a scratch prefix, run `self uninstall` from
  the installed copy, wait for the process to exit, and assert that nothing it installed remains -- the `.exe`
  included -- and that no manifest is left. A Unix scenario asserts the same inline removal with no deferred step.

### Requirement 5: The documents

- `self uninstall`'s help and the getting-started guide say what an uninstall removes, and that on Windows the
  program itself goes once the command exits. The CLI reference is regenerated, never edited.

## Implementation Phases

### Phase 1: Commit the plan

- [ ] This plan committed on `fix/1003-self-uninstall-on-windows-leaves`, and reviewed with the owner

### Phase 2: The tests, failing (Requirements 1, 3, 4)

- [x] The Go test of the reasons, and the uninstall scenario, failing against `71dc125b`. `cmd/internal/cli`:
      a refused file is reported "modified", and the manifest is removed while that file remains (both fail now;
      Unix-only, since the setup refuses a delete through directory permissions, skipped as root). `cmd/scenario`:
      `self uninstall` run from the installed copy, so the running image is the file removed -- the Windows case the
      existing scenario misses by running the built binary. It polls for a deletion deferred past the process's exit

### Phase 3: The shared fix (Requirements 1, 2)

- [ ] `removeRecordedFiles` reports a reason per file; the manifest is rewritten to what remains and deleted only
      when nothing does; the summary names each group. `make test`, `make vet-all`, `make lint-all`, `star lint go`
      pass; `gofmt -l` is empty

### Phase 4: The Windows deletion (Requirement 3)

- [ ] `self uninstall` schedules the running image's deletion through `cmd.exe`, hidden and detached, and keeps it
      recorded. The PowerShell gate and the Windows scenario pass

### Phase 5: The documents (Requirement 5)

- [ ] The help, the guide, and the regenerated reference

### Phase 6: Merge

- [ ] PR script written, shown, and handed over. The PR resolves #1003
- [ ] After the merge, an install and uninstall on a Windows machine through the published installer leaves nothing
      behind (owed until a Windows machine is at hand; the Windows CI scenario stands in meanwhile)

## Out of Scope

- **Elevating to uninstall from a protected prefix.** A prefix under `Program Files`, `/usr/local` or `/opt` was
  installed into with elevation, and an unelevated uninstall cannot delete there, on any platform. This lane names
  those files as refused and keeps them recorded; offering to elevate waits on the elevation epic (#520), whose
  design is still nascent (ruled 2026-10-05).
- **The self-copying, `FILE_FLAG_DELETE_ON_CLOSE` deleter.** Considered and set aside for the `cmd.exe` loop, which
  copies nothing and leaves nothing (ruled 2026-10-05).

## Decisions

- **D1. `cmd.exe` deletes the running image, not a copy of writ.** `cmd.exe` is always present and is never left
  behind; a self-copy must be created, dispatched and cleaned up, and reads as malware to more antivirus engines.
- **D2. A retry-until-unlocked loop, not a wait on the process id.** `del` fails while the image is mapped and
  succeeds when it is released, so the loop waits exactly as long as the shutdown takes, with a cap for a process
  that never exits. A fixed delay would race the shutdown.
- **D3. `ping`, not `timeout`, for the delay.** `timeout` fails with "input redirection is not supported" when there
  is no console, which is the detached case; `ping -n 2 127.0.0.1 >nul` is a console-free ~1s delay.
- **D4. Uninstall does not rename the image aside.** The install path's `<tool>.exe.old` rename makes way for a
  replacement; uninstall has no replacement, so it deletes the image under its own name once it is released.
- **D5. The running image stays in the manifest until a run confirms it gone.** Its deletion is deferred past this
  process's exit and cannot be confirmed here, so the record keeps it; a re-run removes it and clears the record.
