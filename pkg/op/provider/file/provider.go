// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gitignore"
)

// Provider provides file system actions.
//
// Compensable forward methods return (string, map[string]any, error): the resource path, the compensation receipt, and
// an error. The map is opaque to the executor, meaningful only to the corresponding "Compensate*" backward method.
//
// +devlore:access=both
// +devlore:bind Root=WorkDir
type Provider struct {
	Root string // Working directory for Glob and WalkTree
}

// Actor returns a Reducer that calls the given function for each file or directory in a WalkTree operation.
//
// This is a convenience function for creating Reducers from simple functions that don't accumulate results or need to
// access the recovery stack.
func Actor(fn func(path string, dirEntry os.DirEntry) error) Reducer {
	return func(result any, path string, dirEntry os.DirEntry, stack *op.RecoveryStack) (any, error) {
		var zero any
		return zero, fn(path, dirEntry)
	}
}

// Reducer is a function called for each file or directory in a WalkTree operation.
// +devlore:callable swallow=stack
type Reducer func(initial any, path string, dirEntry os.DirEntry, stack *op.RecoveryStack) (result any, err error)

type Tombstone struct {
	RecoveryPath string // Where it is now
	OriginalPath string // Where it used to be
}

var (
	// SkipDir indicates that the current directory should be skipped.
	SkipDir = fs.SkipDir

	// SkipAll signals the walker to terminate immediately (success).
	SkipAll = fs.SkipAll
)

// ── Compensable Pairs ────────────────────────────────────────────────────────────────────────────────────────────────

// Backup moves the file at "path" to a timestamped backup location.
//
// Parameters:
//   - path: Absolute path to the file to back up
//   - backupSuffix: Suffix appended before the timestamp (default: .devlore-backup)
//
// Returns:
//   - The backup path and compensation state.
func (p *Provider) Backup(path, backupSuffix string) (result string, undo map[string]any, err error) {

	if backupSuffix == "" {
		backupSuffix = ".devlore-backup"
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := path + backupSuffix + "." + timestamp

	checksum := checksumFile(path)

	if err := os.Rename(path, backupPath); err != nil {
		return "", nil, err
	}

	undo = map[string]any{
		"original_path": path,
		"backup_path":   backupPath,
	}

	if checksum != "" {
		undo["written_checksum"] = checksum
	}

	return backupPath, undo, nil
}

// CompensateBackup undoes a Backup by moving the backup back to the original path.
//
// If a written_checksum is present, the backup file is verified before restoring; a mismatch indicates external
// modification and the compensation is skipped.
func (p *Provider) CompensateBackup(undo map[string]any) error {

	originalPath := op.StateString(undo, "original_path")
	backupPath := op.StateString(undo, "backup_path")

	if originalPath == "" || backupPath == "" {
		return nil
	}

	// Verify the backup hasn't been modified since we created it.

	if expected := op.StateString(undo, "written_checksum"); expected != "" {
		actual := checksumFile(backupPath)
		if actual == "" {
			return fmt.Errorf("cannot read %s for verification", backupPath)
		}
		if actual != expected {
			return fmt.Errorf("%s has been modified (checksum mismatch)", backupPath)
		}
	}

	return os.Rename(backupPath, originalPath)
}

// Copy copies a blob to the file at "destination" with the given mode.
//
// If the destination already exists, it is moved to the recovery path before writing.
//
// Parameters:
//   - destination: Absolute path to the file to write
//   - source: The content to copy (a Resource wrapping a file path)
//   - mode: The file mode to use (default: 0644)
//
// Returns:
//   - result: the path to the written file
//   - undo: a state map containing the tombstone for recovery
//   - err: any error that occurred during the copy
func (p *Provider) Copy(sourceFile Resource, destinationFilename string, destinationFileMode os.FileMode) (result Resource, undo map[string]any, err error) {

	result, undo, err = p.prepareWrite(destinationFilename)

	if err != nil {
		return Resource{}, nil, err
	}

	if destinationFileMode == 0 {
		destinationFileMode = 0o644
	}

	f, err := os.OpenFile(destinationFilename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, destinationFileMode)

	if err != nil {
		return Resource{}, nil, err
	}

	defer f.Close()

	if _, err := sourceFile.WriteTo(f); err != nil {
		return result, nil, err
	}

	return result, undo, nil
}

// CompensateCopy undoes a Copy action by restoring the original file from the recovery path.
//
// Parameters:
//   - undo: The state map returned by Copy
//
// Returns:
//   - err: any error that occurred during the compensation
func (p *Provider) CompensateCopy(undo map[string]any) error {
	return p.compensateWrite(undo)
}

// Link creates a symlink at a path pointing to a source file.
//
// Idempotent: if the symlink already points correctly, it's a no-op.
//
// Parameters:
//   - source: Absolute path to the symlink target
//   - path: Absolute path where the symlink will be created
//
// Returns:
//   - result: the path to the symlink
//   - undo: a state map with the following keys:
//     > path: the path to the symlink
//     > existed_before: true if the symlink already existed before
//     > previous_target: the target of the symlink before it was created if it existed
func (p *Provider) Link(source, path string) (result string, undo map[string]any, err error) {

	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			existing, readErr := os.Readlink(path)
			if readErr == nil && existing == source {
				return path, nil, nil // Already correct — no change
			}
			undo = map[string]any{
				"path":           path,
				"existed_before": true,
			}
			if readErr == nil {
				undo["previous_target"] = existing
			}
		} else {
			undo = map[string]any{
				"path":           path,
				"existed_before": true,
			}
		}
		if err := os.Remove(path); err != nil {
			return "", nil, err
		}
	} else {
		undo = map[string]any{
			"path":           path,
			"existed_before": false,
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", nil, err
	}

	if err := os.Symlink(source, path); err != nil {
		return "", nil, err
	}

	return path, undo, nil
}

// CompensateLink undoes a Link action using the captured state.
//
// If the symlink existed before, it is removed. Otherwise, it is recreated.
//
// Parameters:
//   - state: The state map returned by Link
//
// Returns:
//   - err: any error that occurred during the compensation
func (p *Provider) CompensateLink(undo map[string]any) error {

	path := op.StateString(undo, "path")

	if path == "" {
		return nil
	}

	if !op.StateBool(undo, "existed_before") {
		return os.Remove(path)
	}

	prevTarget := op.StateString(undo, "previous_target")

	if prevTarget == "" {
		// Was a non-symlink — can't restore, just remove the new symlink.
		return os.Remove(path)
	}

	if err := os.Remove(path); err != nil {
		return err
	}

	return os.Symlink(prevTarget, path)
}

// Move moves a file from source to path using "os.Rename".
//
// Parameters:
//   - source: Absolute path to the source file
//   - destination: Absolute path to the destination file
//
// Returns:
//   - result: the path to the moved file
//   - undo: a state map with the following keys:
//     > source: the source path of the move
//     > path: the destination path of the move
//     > written_checksum: the checksum of the moved file (if available)
func (p *Provider) Move(source, destination string) (result string, undo map[string]any, err error) {

	if _, err := os.Stat(source); err != nil {
		return "", nil, err
	}

	// Capture content checksum before the rename.
	checksum := checksumFile(source)

	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return "", nil, err
	}

	if err := os.Rename(source, destination); err != nil {
		return "", nil, err
	}

	state := map[string]any{
		"source":      source,
		"destination": destination,
	}

	if checksum != "" {
		state["written_checksum"] = checksum
	}

	return destination, state, nil
}

// CompensateMove undoes a Move by moving the file back from path to source.
//
// If a written_checksum is present, the destination file is verified before restoring; a mismatch indicates external
// modification and the compensation is skipped.
//
// Parameters:
//   - undo: The state map returned by Move
//
// Returns:
//   - err: any error that occurred during the compensation
func (p *Provider) CompensateMove(undo map[string]any) error {

	source := op.StateString(undo, "source")
	destination := op.StateString(undo, "destination")

	if source == "" || destination == "" {
		return nil
	}

	// Verify the file hasn't been modified since we moved it.
	if expected := op.StateString(undo, "written_checksum"); expected != "" {
		actual := checksumFile(destination)
		if actual == "" {
			return fmt.Errorf("cannot read %s for verification", destination)
		}
		if actual != expected {
			return fmt.Errorf("%s has been modified (checksum mismatch)", destination)
		}
	}

	if err := os.MkdirAll(filepath.Dir(source), 0o750); err != nil {
		return err
	}
	return os.Rename(destination, source)
}

// Remove deletes the file at "path".
//
// If prune is true and pruneBoundary is set, empty parent directories are removed up to the boundary.
//
// Parameters:
//   - path: Absolute path to the file to delete
//   - prune: If true, remove empty parent directories after deletion
//   - pruneBoundary: Stop pruning at this directory (prevents removing too much)
//
// Returns:
//   - result: the path to the deleted file
//   - compState: a state map with the following keys:
//     > path: the path to the deleted file
//     > content: the content of the deleted file (if available)
//     > mode: the file mode of the deleted file (if available)
func (p *Provider) Remove(path string, prune bool, pruneBoundary string) (result Tombstone, undo map[string]any, err error) {

	nonEmptyDirectory, err := isDirAndNotEmpty(path)

	if err != nil {
		if os.IsNotExist(err) {
			return Tombstone{}, nil, nil
		}
		return Tombstone{}, nil, err
	}

	if nonEmptyDirectory {
		return Tombstone{}, nil, fmt.Errorf("directory %s is not empty", path)
	}

	return p.moveToRecovery(path, prune, pruneBoundary)
}

// CompensateRemove undoes a Remove by re-creating the file with saved content and mode.
//
// If content is not available, the file is recreated with default permissions.
//
// Parameters:
//   - undo: The state map returned by Remove
//
// Returns:
//   - err: any error that occurred during the compensation
func (p *Provider) CompensateRemove(undo map[string]any) error {
	tombstone, err := op.ExtractUndo[Tombstone](undo, "tombstone")
	if err != nil {
		return err
	}
	return p.restoreFromRecovery(tombstone)
}

// RemoveAll removes the file at "path" and any children it contains.
//
// Parameters:
//   - path: Absolute path to the file or directory to remove
//   - prune: If true, remove empty parent directories after deletion
//   - pruneBoundary: Stop pruning at this directory (prevents removing too much)
//
// Returns:
//   - result: the path to the deleted file or directory
//   - undo: a state map with the following keys:
//     > path: the path to the deleted file or directory
//     > files_moved: the number of files moved to the recovery directory
//     > recovery_dir: the path to the recovery directory
//   - err: any error that occurred during the removal
func (p *Provider) RemoveAll(path string, prune bool, pruneBoundary string) (result Tombstone, undo map[string]any, err error) {
	return p.moveToRecovery(path, prune, pruneBoundary)
}

// CompensateRemoveAll is the compensating action for RemoveAll.
//
// It moves the files from the recovery site back to the original location.
//
// Parameters:
//   - undo: The state map returned by RemoveAll
//
// Returns:
//   - err: any error that occurred during the compensation
func (p *Provider) CompensateRemoveAll(undo map[string]any) error {
	tombstone, err := op.ExtractUndo[Tombstone](undo, "tombstone")
	if err != nil {
		return err
	}
	return p.restoreFromRecovery(tombstone)
}

// Unlink removes the symlink at "path".
//
// If prune is true and pruneBoundary is set, empty parent directories are removed up to the boundary.
//
// Parameters:
//   - path: Absolute path to the symlink to remove
//   - prune: If true, remove empty parent directories after unlinking
//   - pruneBoundary: Stop pruning at this directory (prevents removing too much)
//
// Returns:
//   - result: the path to the deleted symlink
//   - compState: a state map with the following keys:
//     > path: the path to the deleted symlink
//     > target: the target of the deleted symlink
func (p *Provider) Unlink(path string, prune bool, pruneBoundary string) (result Tombstone, undo map[string]any, err error) {

	info, err := os.Lstat(path)

	if os.IsNotExist(err) {
		return Tombstone{}, nil, nil // Already gone — no change
	}

	if err != nil {
		return Tombstone{}, nil, err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return Tombstone{}, nil, fmt.Errorf("%s is not a symlink", path)
	}

	return p.moveToRecovery(path, prune, pruneBoundary)
}

// CompensateUnlink undoes an Unlink by re-creating the symlink.
//
// Parameters:
//   - state: The state map returned by Unlink
//
// Returns:
//   - err: any error that occurred during the compensation
func (p *Provider) CompensateUnlink(undo map[string]any) error {
	tombstone, err := op.ExtractUndo[Tombstone](undo, "tombstone")
	if err != nil {
		return err
	}
	return p.restoreFromRecovery(tombstone)
}

// WalkTree performs a depth-first traversal with an accumulator and a RecoveryStack for compensable operations.
//
// The visitor can push compensable operations onto the stack during traversal. On error mid-walk, the stack
// is unwound automatically and errors are joined. On success, the accumulated result and the stack are returned--the
// stack serves as the undo receipt.
//
// +devlore:defaults root="",honorGitignore=true
//
// Parameters:
//   - root: Root directory to start traversal from
//   - fn: Reducer function to call for each file or directory
//   - gitignore: If true, filter results using gitignore rules
//
// Returns:
//   - result: The accumulated result from the visitor function
//   - stack: The compensable operations stack
//   - err: The first error encountered during traversal, if any
func (p *Provider) WalkTree(root string, fn Reducer, honorGitignore bool) (result any, stack *op.RecoveryStack, err error) {

	stack = op.NewRecoveryStack()

	walkFn := func(path string, entry os.DirEntry) error {
		var err error
		result, err = fn(result, path, entry, stack)
		return err
	}

	var tracker *gitignore.Tracker

	if honorGitignore {
		value, err := gitignore.NewTracker(root)
		if err != nil {
			return nil, nil, err
		}
		tracker = value
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}

	if _, err := os.Stat(absoluteRoot); err != nil {
		return nil, nil, err
	}

	walkErr := filepath.WalkDir(absoluteRoot, func(path string, d fs.DirEntry, walkDirErr error) error {

		if walkDirErr != nil {
			return walkDirErr
		}

		relativePath, relativeErr := filepath.Rel(absoluteRoot, path)

		if relativeErr != nil {
			return relativeErr
		}

		if relativePath == "." {
			return nil
		}

		isDir := d.IsDir()

		if isDir && d.Name() == ".git" {
			return SkipDir
		}

		if tracker != nil {
			if isDir {
				tracker.Push(relativePath)
			}
			ignored, _ := tracker.IsIgnored(relativePath, isDir)
			if ignored && isDir {
				return SkipDir
			}
			if ignored {
				return nil
			}
		}

		return walkFn(relativePath, d)
	})

	if walkErr != nil {
		return nil, stack, walkErr
	}

	return result, stack, nil
}

// CompensateWalkTree unwinds the RecoveryStack returned by WalkTree in LIFO order.
//
// Best-effort: all entries are attempted, errors are joined.
//
// Parameters:
//   - stack: The stack returned by WalkTree
//
// Returns:
//   - err: The first error encountered during compensation, if any
func (p *Provider) CompensateWalkTree(stack *op.RecoveryStack) error {
	if stack == nil {
		return nil
	}
	return stack.Unwind()
}

// WriteBytes writes inline content to the file at "path" with the given mode.
//
// Parameters:
//   - content: String content to write to the file
//   - path: Absolute path where the file will be written
//   - mode: File permission bits (e.g., 0o644)
//
// Returns:
//   - result: the path to the written file
//   - undo: a state map with the following keys:
//     > tombstone: a [Tombstone]
//   - err: any error that occurred while writing
func (p *Provider) WriteBytes(destination, content string, mode os.FileMode) (result Resource, undo map[string]any, err error) {
	return p.write(destination, []byte(content), mode)
}

// CompensateWriteBytes restores the previous state of written bytes using the provided undo map data.
//
// It delegates to the internal [compensateWrite] method to perform the actual compensation operation.
//
// Parameters:
//   - undo: The state map returned by [WriteBytes]
//
// Returns:
//   - err: any error that occurred during the compensation
func (p *Provider) CompensateWriteBytes(undo map[string]any) error {
	return p.compensateWrite(undo)
}

// WriteText writes inline content to the file at "path" with the given mode.
//
// Parameters:
//   - content: String content to write to the file
//   - path: Absolute path where the file will be written
//   - mode: File permission bits (e.g., 0o644)
//
// Returns:
//   - result: the path to the written file
//   - undo: a state map with the following keys:
//     > tombstone: a [Tombstone]
//   - err: any error that occurred while writing
func (p *Provider) WriteText(destination, content string, mode os.FileMode) (result Resource, undo map[string]any, err error) {
	return p.write(destination, []byte(content), mode)
}

// CompensateWriteText undoes a WriteText action using the captured state.
//
// It delegates to the internal [compensateWrite] method to perform the actual compensation operation.
//
// Parameters:
//   - undo: The state map returned by [WriteText]
//
// Returns:
//   - err: any error that occurred during the compensation
func (p *Provider) CompensateWriteText(undo map[string]any) error {
	return p.compensateWrite(undo)
}

// ── Non-compensable Methods (pure functions) ─────────────────────────────────────────────────────────────────────────

// Exists returns true if the file at "path" exists.
//
// Parameters:
//   - path: Absolute path to check
//
// Returns:
//   - bool: true if the file at "path" exists, false otherwise
func (p *Provider) Exists(blob Resource) bool {
	_, err := os.Lstat(blob.SourcePath)
	return err == nil
}

// Glob returns file paths matching a pattern relative to Root.
//
// Parameters:
//   - pattern: Glob pattern (e.g., "*.go", "**/*.yaml")
//   - honorGitignore: If true, filter results using gitignore rules
//
// Returns:
//   - []string: List of matching file paths
func (p *Provider) Glob(pattern string, honorGitignore bool) ([]string, error) {

	matches, err := filepath.Glob(pattern)

	if err != nil {
		return nil, err
	}

	if !honorGitignore || p.Root == "" {
		return matches, nil
	}

	tracker, trackerErr := gitignore.NewTracker(p.Root)

	if trackerErr != nil {
		return matches, nil //nolint:nile`rr // graceful degradation: return unfiltered if gitignore unavailable
	}

	var filtered []string
	for _, m := range matches {
		relPath, relErr := filepath.Rel(tracker.Root(), m)
		if relErr != nil {
			filtered = append(filtered, m)
			continue
		}
		info, statErr := os.Stat(m)
		isDir := statErr == nil && info.IsDir()
		ignored, _ := tracker.IsIgnored(relPath, isDir)

		if !ignored {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}

// IsDir returns true if the file at "path" exists and is a directory.
//
// Parameters:
//   - path: Absolute path to check
//
// Returns:
//   - bool: true if the file at "path "is a directory, false otherwise
func (p *Provider) IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// IsFile returns true if the file at "path" exists and is a regular file.
//
// Parameters:
//   - path: Absolute path to check
//
// Returns:
//   - bool: true if the file at "path" is a regular file, false otherwise
func (p *Provider) IsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// Join joins path components using the OS path separator.
//
// Parameters:
//   - parts: Path components to join
//
// Returns:
//   - string: The joined path or an empty string, if no parts are provided or all parts are empty
func (p *Provider) Join(parts ...string) string {
	return filepath.Join(parts...)
}

// Mkdir creates a directory (and parents) with the given mode.
//
// Parameters:
//   - path: Absolute path of the directory to create
//   - mode: Directory permission bits (e.g., 0o755)
//
// Returns:
//   - string: The absolute path of the created directory
func (p *Provider) Mkdir(path string, mode os.FileMode) (string, error) {
	return path, os.MkdirAll(path, mode)
}

// Name returns the last element of "path" (a file or directory name).
//
// Parameters:
//   - path: Path to extract the name from
//
// Returns:
//   - string: The name of the file or directory

func (p *Provider) Name(path string) string {
	return filepath.Base(path)
}

// Parent returns the directory containing the file at "path".
//
// Parameters:
//   - path: Path to a file
//
// Returns:
//   - string: The parent directory of the file
func (p *Provider) Parent(path string) string {
	return filepath.Dir(path)
}

// Read creates a Resource from the file at "path" for reading the contents of the file at "path".
//
// Parameters:
//   - path: Absolute path to the file to read
//
// Returns:
//   - result: the contents of the file
func (p *Provider) Read(path string) (result Resource, err error) {
	return NewResource(path)
}

// region Internal

// compensateWrite reverts a write operation by restoring the original file state from the tombstone record.
//
// Parameters:
//   - undo: The state map returned by Write
//
// Returns:
//   - err: Any error that occurred during the compensation
func (p *Provider) compensateWrite(undo map[string]any) error {

	tombstone, err := op.ExtractUndo[Tombstone](undo, "tombstone")

	if err != nil {
		return err
	}

	if err := os.Remove(tombstone.OriginalPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	if tombstone.RecoveryPath != "" {
		return os.Rename(tombstone.RecoveryPath, tombstone.OriginalPath)
	}

	return nil
}

// prepareWrite handles the "Undo" logic for destructive actions.
//
// It moves the entity to a recovery site before performing the action.
//
// Parameters:
//   - path: The path to the file to write
//
// Returns:
//   - undo: A state map with the following keys:
//     > action: The action that was performed (e.g., "overwrite", "create")
//     > tombstone: The tombstone returned by RemoveAll
//   - err: Any error that occurred during the preparation
func (p *Provider) prepareWrite(path string) (result Resource, undo map[string]any, err error) {

	result, err = NewResource(path)

	if err != nil {
		return Resource{}, nil, err
	}

	if !result.Exists() {

		err = os.MkdirAll(filepath.Dir(result.SourcePath), 0o750)

		if err != nil {
			return Resource{}, nil, errors.Join(os.ErrNotExist, err)
		}

		undo = map[string]any{"tombstone": Tombstone{OriginalPath: result.SourcePath}}
		return result, undo, nil
	}

	tombstone, _, err := p.Remove(result.SourcePath, false, "")

	if err != nil {
		return Resource{}, nil, fmt.Errorf("failed to backup existing file: %w", err)
	}

	undo = map[string]any{"tombstone": tombstone}
	return result, undo, nil
}

// write writes data to the specified path after preparing the write operation.
//
// Parameters:
//   - path: The path to the file to write
//   - data: The data to write to the file
//
// Returns:
func (p *Provider) write(path string, data []byte, mode os.FileMode) (result Resource, undo map[string]any, err error) {

	// Prepare the write operation

	result, undo, err = p.prepareWrite(path)

	if err != nil {
		return result, nil, err
	}

	if mode == 0 {
		mode = 0o644
	}

	// Perform the write operation with a hasher and a MultiWriter

	f, err := os.OpenFile(result.SourcePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)

	if err != nil {
		return result, undo, err
	}
	defer f.Close()

	var size int
	hasher := sha256.New()
	mw := io.MultiWriter(f, hasher)

	size, err = mw.Write(data)

	if err != nil {
		return result, undo, err
	}

	// Finalize

	if err = f.Sync(); err != nil {
		return result, undo, err
	}

	// Update metadata using the pre-computed hash

	err = result.RefreshMetadataWith(hex.EncodeToString(hasher.Sum(nil)), int64(size))
	return result, undo, err
}

// endregion
