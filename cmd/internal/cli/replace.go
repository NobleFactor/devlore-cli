// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
)

// region HELPER FUNCTIONS

// Fallible actions

// stageBinary writes `source` to `staged`, executable, and on disk before anything renames it into place.
//
// The file is created at 0o750, the mode the installed binary carries, and set to it again: the umask may have
// narrowed it, and a stale copy keeps the mode it had. It is not [fsroot.Dir.CreateTemp], which creates 0o600
// under a unique name. The sync is there because a rename can reach the disk before the data it names, and a crash
// between the two would leave the program empty.
//
// Parameters:
//   - `prefixRoot`: the installation prefix.
//   - `source`: the binary to copy, outside the root.
//   - `staged`: the staged copy within `prefixRoot`; a file already there is overwritten.
//
// Returns:
//   - `error`: non-nil when the source cannot be read, or the staged copy cannot be written, synced, closed or
//     made executable.
func stageBinary(prefixRoot fsroot.Dir, source string, staged fsroot.Path) (err error) {

	// Unsandboxed: the source is wherever the operator launched it from, or the scratch tree an upgrade unpacked it
	// into — not ours to sandbox. The destination side goes through the root.
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer iox.Close(&err, src)

	dst, err := prefixRoot.OpenFile(staged, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o750)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", staged.Abs(), err)
	}
	defer iox.Close(&err, dst)

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to write %s: %w", staged.Abs(), err)
	}

	if err := dst.Sync(); err != nil {
		return fmt.Errorf("failed to sync %s: %w", staged.Abs(), err)
	}

	if err := prefixRoot.Chmod(staged, 0o750); err != nil {
		return fmt.Errorf("failed to make %s executable: %w", staged.Abs(), err)
	}

	return nil
}

// Actions

// retireSetAside removes the binary an earlier install set aside, unless something still runs it.
//
// Windows leaves one `<tool>.exe.old` beside a binary that replaced a running one (#947, D5), and the install after
// it removes it here, before placing its own binary. While it still runs, Windows refuses: the image of the
// `self upgrade` that placed a program runs from that name while the program's own `self install` runs. The refusal
// is expected, and not reported: the file stays on disk, [setAsideToRecord] records it, and a later install removes
// it once nothing runs it.
//
// It is removed by name, not through the record's hash guard, which spares an operator's edits: the name is the
// installer's own, and [replaceBinary] renames over it just the same.
//
// Parameters:
//   - `prefixRoot`: the installation prefix.
//   - `aside`: the set-aside binary's name in `bin/`, from [setAsideName]; empty where nothing is set aside.
func retireSetAside(prefixRoot fsroot.Dir, aside string) {

	// No name is no set-aside. The path below would be `bin/` itself, which an empty directory would lose.
	if aside == "" {
		return
	}

	_ = prefixRoot.Remove(prefixRoot.NewPath("bin", aside)) //nolint:errcheck // refused while it runs, then recorded
}

// setAsideToRecord returns the set-aside binary this install records in its manifest, when there is one on disk.
//
// The manifest is the record of what the tool owns (#933), and a binary set aside is the tool's: recorded, a later
// install removes it and `self uninstall` reaches it. [retireSetAside] removed any it could before this install
// placed its binary, so one on disk now is one this install set aside or one that still runs, and either way it is
// recorded. Its content cannot tell it from one the record already names: the same build set aside twice, by a
// repeated install or by an upgrade retried from the installed copy, is byte for byte that file.
//
// It is found in `bin/` rather than reported by [replaceBinary], because the process that sets it aside is not
// always the one that writes the record: `self upgrade` places each program before that program's own
// `self install` runs, and it is that install which records it.
//
// Parameters:
//   - `prefixRoot`: the installation prefix.
//   - `aside`: the set-aside binary's name in `bin/`, from [setAsideName]; empty where nothing is set aside.
//
// Returns:
//   - `[]string`: the set-aside binary's path relative to the prefix when there is one; otherwise empty.
func setAsideToRecord(prefixRoot fsroot.Dir, aside string) []string {

	if aside == "" {
		return nil
	}

	relative := filepath.Join("bin", aside)

	if _, err := prefixRoot.Stat(prefixRoot.NewPath(relative)); err != nil {
		// One that may be there and cannot be seen is left out of the record, as writeManifest would leave it; saying
		// so is the difference between that and a file on disk that nothing owns without anyone knowing.
		if !os.IsNotExist(err) {
			Warn("Cannot stat %s: %v (not recorded)", prefixRoot.NewPath(relative).Abs(), err)
		}
		return nil
	}

	return []string{relative}
}

// endregion
