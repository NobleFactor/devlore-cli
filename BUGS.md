# Known Bugs

## #164: isSameDevice stats non-existent recovery path

`isSameDevice` in `pkg/op/provider/file/recovery_unix.go:52-59` calls `os.Stat` on a path that does not yet exist (`~/Library/Caches/devlore/recovery`). The stat fails, `isSameDevice` returns false, and `getRecoveryBase` falls back to `/.devlore_recovery` which is read-only on macOS (SIP).

**Blocks**: 8 tests (Remove, RemoveAll, Unlink round-trips and their compensations)

## #165: compensateWrite missing nil guard on undo parameter

`compensateWrite` in `pkg/op/provider/file/provider.go` does not guard against a nil `undo` map, unlike all other `Compensate*` methods which return nil early.

**Blocks**: 2 tests (CompensateWriteText_NilState, CompensateWriteBytes_NilState)

## #166: CompensateCopy does not restore file mode on existing files

`CompensateCopy` in `pkg/op/provider/file/provider.go:216` uses `os.WriteFile(path, prev, prevMode)` to restore content and permissions. When the file already exists, `os.WriteFile` truncates and writes but does not change permissions (the mode argument only applies when `O_CREATE` creates a new file). The restored file keeps the mode from the Copy operation instead of reverting to the original.

**Blocks**: 1 test (TestCopy_CompensateCopy_RoundTrip_Overwrite)
